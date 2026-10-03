package io.github.slaynaw.wakeonlan.archive

import io.github.slaynaw.wakeonlan.BuildConfig
import io.github.slaynaw.wakeonlan.backup.BackupManager
import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentHistoryEvent
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.MetricsRow
import io.github.slaynaw.wakeonlan.core.archive.ArchiveCodec
import io.github.slaynaw.wakeonlan.core.archive.ArchiveDay
import io.github.slaynaw.wakeonlan.core.archive.ArchiveException
import io.github.slaynaw.wakeonlan.core.archive.ArchiveJournal
import io.github.slaynaw.wakeonlan.core.archive.ArchiveKey
import io.github.slaynaw.wakeonlan.core.history.HistoryData
import io.github.slaynaw.wakeonlan.core.history.HistoryEvent
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.share.ShareException
import io.github.slaynaw.wakeonlan.core.share.ShareGitHub
import io.github.slaynaw.wakeonlan.data.ConfigRepository
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import java.io.IOException
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.util.concurrent.ConcurrentHashMap
import kotlin.math.roundToLong

/** État des archives pour l'interface. */
data class ArchiveUiState(
    val available: Boolean = false,
    val enabled: Boolean = false,
    val running: Boolean = false,
    val last: Long = 0,
    val error: String? = null,
)

/** Résultat d'un archivage. */
data class ArchiveResult(val pcs: Int, val minutes: Int, val files: Int, val skipped: List<String>)

/** Point des graphiques (moyenne et maximum sur sa durée) ; [t] en millisecondes. */
data class MetricsPoint(
    val t: Long,
    val ct: Double? = null,
    val ctx: Double? = null,
    val gt: Double? = null,
    val gtx: Double? = null,
    val cl: Double? = null,
    val clx: Double? = null,
    val gl: Double? = null,
    val glx: Double? = null,
)

/** Mesures d'une période pour l'écran « Mesures » ; [step] : durée d'un point (secondes). */
data class MetricsRange(
    val from: Long,
    val to: Long,
    val step: Long,
    val points: List<MetricsPoint>,
    val summary: MetricsPoint,
    val minutes: Int,
    val agent: Boolean,
    val archive: Boolean,
    val notes: List<String>,
)

/**
 * Archives chiffrées (docs/ARCHIVES.md), mêmes règles que l'application Windows : l'agent de chaque PC
 * enregistre ses mesures en continu (90 jours) ; tant que l'application est ouverte, elle les rattrape
 * toutes les 30 minutes et les range, avec le journal des démarrages et arrêts, dans un Gist secret par
 * mois chiffré par le mot de passe des sauvegardes (même compte GitHub). L'écran « Mesures » lit l'agent
 * et, au-delà ou PC éteint, les archives.
 */
class ArchiveManager(
    scope: CoroutineScope,
    private val config: ConfigRepository,
    private val devices: Flow<AppConfig>,
    private val backups: BackupManager,
    private val agent: AgentClient,
) {
    private val github = ShareGitHub(BuildConfig.GITHUB_CLIENT_ID, "Patronus-Android/${BuildConfig.VERSION_NAME}")

    private data class Runtime(val running: Boolean = false, val attemptAt: Long = 0, val failed: Boolean = false, val error: String? = null)

    private val runtime = MutableStateFlow(Runtime())

    // Clés dérivées une fois par session (PBKDF2 coûteux) et données lues récemment.
    private val keys = ConcurrentHashMap<String, ArchiveKey>()
    private val gists = ConcurrentHashMap<String, Pair<Long, Map<String, String>>>()
    private val dayLists = ConcurrentHashMap<String, Pair<Long, AgentResult<List<String>>>>()
    private val agentRows = ConcurrentHashMap<String, Pair<Long, List<MetricsRow>>>()
    private val journals = ConcurrentHashMap<String, Pair<Long, List<AgentHistoryEvent>>>()

    @Volatile
    private var monthsCache: Pair<Long, Map<String, List<String>>>? = null

    val state: StateFlow<ArchiveUiState> = combine(backups.current, runtime) { st, rt ->
        ArchiveUiState(
            available = st != null,
            enabled = st?.archive?.enabled == true,
            running = rt.running,
            last = st?.archive?.last ?: 0,
            error = rt.error,
        )
    }.stateIn(scope, SharingStarted.Eagerly, ArchiveUiState())

    init {
        // Autre mot de passe, autre compte ou Gists recopiés (changement du mot de passe) : clés et
        // données lues oubliées.
        scope.launch {
            backups.current.map { Triple(it?.password, it?.github?.token, it?.rotation == null) }.distinctUntilChanged().collect { reset() }
        }
    }

    private fun reset() {
        keys.clear()
        gists.clear()
        monthsCache = null
    }

    /** Échange avec un agent borné dans le temps : un délai dépassé compte comme un PC injoignable. */
    private suspend fun <T> timed(block: suspend () -> AgentResult<T>): AgentResult<T> =
        withTimeoutOrNull(AGENT_TIMEOUT_MS) { block() } ?: AgentResult.Failure(AgentError.UNREACHABLE, "délai dépassé")

    private data class Access(val password: String, val token: String)

    private fun access(): Access? {
        val st = backups.current.value ?: return null
        val token = st.github?.token.orEmpty()
        if (!st.enabled || st.password.isEmpty() || token.isEmpty()) return null
        return Access(st.password, token)
    }

    /** Tant que l'application est visible : archivage toutes les 30 minutes. */
    suspend fun run() {
        delay(START_MS)
        while (true) {
            try {
                if (isDue()) archiveNow()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                DiagnosticLog.w(AREA, "archivage automatique en échec", e)
            }
            delay(TICK_MS)
        }
    }

    private fun isDue(): Boolean {
        val st = backups.current.value ?: return false
        val rt = runtime.value
        val now = System.currentTimeMillis()
        if (!st.archive.enabled || access() == null || rt.running || backups.rotationPending()) return false
        if (rt.failed && now - rt.attemptAt < RETRY_MS) return false
        return now - st.archive.last >= EVERY_MS
    }

    // --- Réglages ---

    suspend fun enable(enabled: Boolean) {
        val st = backups.current.value ?: throw IOException("réglages des sauvegardes pas encore lus")
        if (enabled && (!st.enabled || st.password.isEmpty())) {
            throw IOException("activez d'abord la sauvegarde automatique : les archives sont chiffrées par son mot de passe")
        }
        if (enabled && st.github?.token.isNullOrEmpty()) {
            throw IOException("connectez d'abord GitHub dans la sauvegarde automatique : les archives y sont rangées")
        }
        // Activation : premier archivage tout de suite.
        backups.updateArchive { it.copy(enabled = enabled, last = if (enabled) 0 else it.last) }
        runtime.update { it.copy(failed = false, error = null) }
        DiagnosticLog.i(AREA, "archives ${if (enabled) "activées" else "désactivées"}")
    }

    // --- Archivage ---

    private class PcWork(val name: String) {
        val rows = HashMap<String, List<MetricsRow>>()
        var events: List<AgentHistoryEvent> = emptyList()
        var last = 0L
    }

    /** Rattrape les mesures et le journal des PC joignables et les range dans les Gists. */
    suspend fun archiveNow(): ArchiveResult {
        val st = backups.current.value ?: throw IOException("réglages des sauvegardes pas encore lus")
        if (!st.archive.enabled) throw IOException("archivage désactivé")
        val acc = access() ?: throw IOException("activez la sauvegarde automatique et connectez GitHub")
        if (runtime.value.running) throw IOException("archivage déjà en cours")
        runtime.update { it.copy(running = true, attemptAt = System.currentTimeMillis()) }
        try {
            // Mot de passe changé sur un autre appareil, ou changement en cours : rien n'est écrit.
            backups.checkPassword()
            if (backups.rotationPending()) throw IOException("changement du mot de passe des sauvegardes en cours : l'archivage suivra")
            val gistIds = st.archive.gists.toMutableMap()
            val synced = HashMap<String, Long>()
            val work = LinkedHashMap<String, PcWork>()
            val skipped = ArrayList<String>()
            for (d in config.current().devices) {
                val settings = d.agent ?: continue
                if (!settings.hasKey || !d.hasHost) continue
                val mac = d.mac.toString()
                val since = st.archive.synced[mac] ?: 0L
                when (val w = collect(d, since)) {
                    is AgentResult.Success -> {
                        work[mac] = w.value
                        synced[mac] = w.value.last
                    }
                    is AgentResult.Failure -> skipped += "${d.name} (${describe(w.error)})"
                }
            }
            val monthsTouched = work.values.flatMap { w -> w.rows.keys.map { it.take(7) } + w.events.map { ArchiveCodec.monthOfTime(it.t) } }.toSortedSet()
            var files = 0
            var minutes = 0
            for (month in monthsTouched) {
                if (backups.rotationPending()) throw IOException("changement du mot de passe des sauvegardes en cours : l'archivage suivra")
                val (f, m) = archiveMonth(acc, month, work, gistIds)
                files += f
                minutes += m
            }
            val now = System.currentTimeMillis()
            backups.updateArchive { it.copy(synced = it.synced + synced, gists = gistIds, last = now) }
            runtime.update { it.copy(failed = false, error = null) }
            DiagnosticLog.i(AREA, "archives : ${work.size} PC, $minutes minute(s), $files fichier(s) mis à jour" +
                (if (skipped.isNotEmpty()) " ; ignorés : ${skipped.joinToString()}" else ""))
            return ArchiveResult(work.size, minutes, files, skipped)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            val message = (e as? ShareException)?.message ?: e.message ?: e.javaClass.simpleName
            runtime.update { it.copy(failed = true, error = message) }
            DiagnosticLog.w(AREA, "archivage impossible", e)
            throw if (e is IOException) e else IOException(message, e)
        } finally {
            runtime.update { it.copy(running = false) }
        }
    }

    private suspend fun collect(d: Device, since: Long): AgentResult<PcWork> {
        val settings = d.agent ?: return AgentResult.Failure(AgentError.NO_KEY)
        val list = when (val r = timed { agent.metrics(d.host, settings) }) {
            is AgentResult.Success -> r.value
            is AgentResult.Failure -> return r
        }
        val w = PcWork(d.name).apply { last = since }
        val from = if (since > 0) ArchiveCodec.dayOfTime(since) else ""
        for (day in list.days) {
            if (day < from || ArchiveCodec.dayStart(day) == null) continue
            val rows = when (val r = timed { agent.metrics(d.host, settings, day) }) {
                is AgentResult.Success -> r.value.rows.filter { it.t > since }
                is AgentResult.Failure -> return r
            }
            if (rows.isNotEmpty()) {
                w.rows[day] = rows
                w.last = maxOf(w.last, rows.maxOf { it.t })
            }
        }
        val history = timed { agent.history(d.host, settings) }
        if (history is AgentResult.Success) w.events = history.value.events
        return AgentResult.Success(w)
    }

    /** Fusionne le travail d'un mois dans son Gist ; renvoie les fichiers écrits et les minutes ajoutées. */
    private suspend fun archiveMonth(acc: Access, month: String, work: Map<String, PcWork>, gistIds: MutableMap<String, String>): Pair<Int, Int> {
        val (gistId, key, files) = monthGist(acc, month, gistIds, create = true) ?: throw IOException("Gist d'archives introuvable")
        val updates = HashMap<String, String?>()
        var minutes = 0
        withContext(Dispatchers.Default) {
            val days = work.values.flatMap { it.rows.keys }.filter { it.startsWith(month) }.toSortedSet()
            for (day in days) {
                val name = ArchiveCodec.dayFile(day)
                var content = files[name]?.let { text ->
                    try {
                        key.open(name, text, ArchiveDay.serializer())
                    } catch (e: ArchiveException) {
                        DiagnosticLog.w(AREA, "archives : $name illisible, réécrit", e)
                        null
                    }
                } ?: ArchiveDay(day)
                var changed = false
                for ((mac, w) in work.toSortedMap()) {
                    val rows = w.rows[day] ?: continue
                    val (next, c) = content.addRows(mac, w.name, rows)
                    content = next
                    if (c) {
                        changed = true
                        minutes += rows.size
                    }
                }
                if (changed) updates[name] = key.seal(name, ArchiveDay.serializer(), content)
            }
            val name = ArchiveCodec.journalFile(month)
            var journal = files[name]?.let { text ->
                try {
                    key.open(name, text, ArchiveJournal.serializer())
                } catch (e: ArchiveException) {
                    DiagnosticLog.w(AREA, "archives : $name illisible, réécrit", e)
                    null
                }
            } ?: ArchiveJournal(month)
            var changed = false
            for ((mac, w) in work.toSortedMap()) {
                val (next, c) = journal.addEvents(mac, w.name, w.events)
                journal = next
                changed = changed || c
            }
            if (changed) updates[name] = key.seal(name, ArchiveJournal.serializer(), journal)
        }
        if (updates.isEmpty()) return 0 to 0
        github.updateGist(acc.token, gistId, updates)
        gists.remove(gistId)
        return updates.size to minutes
    }

    private data class MonthGist(val id: String, val key: ArchiveKey, val files: Map<String, String>)

    /** Gist d'archives du mois que le mot de passe ouvre ([create] : le crée s'il n'existe pas). */
    private suspend fun monthGist(acc: Access, month: String, gistIds: MutableMap<String, String>, create: Boolean): MonthGist? {
        val tried = HashSet<String>()
        suspend fun attempt(id: String): MonthGist? {
            tried += id
            return try {
                openGist(acc, id, month)
            } catch (e: ShareException) {
                if (e.reason != ShareException.Reason.NOT_FOUND) throw e
                null
            } catch (_: ArchiveException) {
                null
            }
        }
        gistIds[month]?.let { id -> attempt(id)?.let { return it } }
        gistIds.remove(month)
        for (id in months(acc, refresh = false)[month].orEmpty()) {
            if (id in tried) continue
            attempt(id)?.let {
                gistIds[month] = id
                return it
            }
        }
        if (!create) return null
        val password = acc.password.toCharArray()
        val (manifest, key) = try {
            withContext(Dispatchers.Default) { ArchiveCodec.newManifest(month, password) }
        } finally {
            password.fill(' ')
        }
        val manifestJson = ArchiveCodec.manifestJson(manifest)
        val id = github.createGist(acc.token, ArchiveCodec.description(month), mapOf(
            ArchiveCodec.MANIFEST_FILE to manifestJson,
            ArchiveCodec.README_FILE to ArchiveCodec.README,
        ))
        DiagnosticLog.i(AREA, "archives : Gist ${id.take(8)}… créé pour $month")
        keys[id] = key
        monthsCache = monthsCache?.let { (at, map) -> at to (map + (month to (map[month].orEmpty() + id))) }
        gistIds[month] = id
        return MonthGist(id, key, mapOf(ArchiveCodec.MANIFEST_FILE to manifestJson))
    }

    /** Ouvre un Gist d'archives du mois : manifeste vérifié, clé (dérivée une fois), fichiers. */
    private suspend fun openGist(acc: Access, id: String, month: String): MonthGist {
        val files = gistFiles(acc, id)
        val manifest = ArchiveCodec.parseManifest(files[ArchiveCodec.MANIFEST_FILE] ?: throw ArchiveException("manifeste absent"))
        if (manifest.month != month) throw ArchiveException("Gist d'un autre mois")
        val key = keys[id] ?: run {
            val password = acc.password.toCharArray()
            try {
                withContext(Dispatchers.Default) { ArchiveCodec.unlock(manifest, password) }
            } finally {
                password.fill(' ')
            }
        }.also { keys[id] = it }
        return MonthGist(id, key, files)
    }

    private suspend fun gistFiles(acc: Access, id: String): Map<String, String> {
        gists[id]?.let { (at, files) -> if (System.currentTimeMillis() - at < GIST_TTL_MS) return files }
        val files = github.readGist(acc.token, id, ArchiveCodec.MAX_FILE_BYTES).associate { it.name to it.content }
        gists[id] = System.currentTimeMillis() to files
        return files
    }

    /** Gists d'archives du compte, par mois. */
    private suspend fun months(acc: Access, refresh: Boolean): Map<String, List<String>> {
        monthsCache?.let { (at, map) -> if (!refresh && System.currentTimeMillis() - at < MONTHS_TTL_MS) return map }
        val map = github.listGists(acc.token, ArchiveCodec.DESCRIPTION_PREFIX)
            .mapNotNull { g -> ArchiveCodec.monthOf(g.description)?.let { it to g.id } }
            .groupBy({ it.first }, { it.second })
        monthsCache = System.currentTimeMillis() to map
        return map
    }

    private fun describe(error: AgentError) = when (error) {
        AgentError.UNREACHABLE, AgentError.REFUSED, AgentError.UNKNOWN_HOST -> "injoignable"
        AgentError.REJECTED -> "agent antérieur à 1.8.0"
        AgentError.UNAUTHORIZED -> "clé refusée"
        else -> error.name.lowercase()
    }

    // --- Écran « Mesures » ---

    /** Mois archivés sur GitHub, du plus récent au plus ancien (vide si les archives ne sont pas configurées). */
    suspend fun archivedMonths(refresh: Boolean = false): List<String> {
        val acc = access() ?: return emptyList()
        return months(acc, refresh).keys.sortedDescending()
    }

    private suspend fun device(id: String): Pair<Device, Boolean>? {
        config.current().devices.firstOrNull { it.id == id }?.let { return it to true }
        return devices.first().devices.firstOrNull { it.id == id }?.let { it to false }
    }

    /** Mesures d'un PC entre [fromMs] et [toMs] (au plus [points] points). */
    suspend fun range(deviceId: String, fromMs: Long, toMs: Long, points: Int): MetricsRange {
        val (d, own) = device(deviceId) ?: throw IOException("PC introuvable")
        val from = fromMs / 1000
        val to = toMs / 1000
        require(to > from && to - from <= MAX_RANGE_DAYS * 86_400L) { "Période invalide" }
        val notes = LinkedHashSet<String>()
        val days = agentDays(d)
        if (days is AgentResult.Failure && days.error != AgentError.NO_KEY) notes += agentNote(days.error)
        val agentList = (days as? AgentResult.Success)?.value.orEmpty()
        val acc = if (own) access() else null
        val rows = ArrayList<MetricsRow>()
        var fromAgent = false
        var fromArchive = false
        var day = LocalDate.ofEpochDay(Math.floorDiv(from, 86_400L))
        while (day.atStartOfDay(ZoneOffset.UTC).toEpochSecond() < to) {
            val name = day.toString()
            var done = false
            if (name in agentList) {
                when (val r = agentDay(d, name)) {
                    is AgentResult.Success -> {
                        rows += r.value
                        fromAgent = true
                        done = true
                    }
                    is AgentResult.Failure -> notes += agentNote(r.error)
                }
            }
            if (!done && acc != null) {
                try {
                    archiveDay(acc, d.mac.toString(), name)?.let {
                        rows += it
                        fromArchive = true
                    }
                } catch (e: ShareException) {
                    notes += "Archives illisibles : ${e.message}"
                }
            }
            day = day.plusDays(1)
        }
        if (own && acc == null) notes += "Archives GitHub non configurées : seules les mesures gardées par le PC (90 jours) sont disponibles."
        return withContext(Dispatchers.Default) {
            downsample(rows, from, to, points.coerceIn(10, 1500)).let { (step, pts, summary, minutes) ->
                MetricsRange(fromMs, toMs, step, pts, summary, minutes, fromAgent, fromArchive, notes.filter { it.isNotEmpty() })
            }
        }
    }

    private fun agentNote(error: AgentError) = when (error) {
        AgentError.UNREACHABLE, AgentError.REFUSED, AgentError.UNKNOWN_HOST -> "PC injoignable : mesures lues dans les archives GitHub."
        AgentError.REJECTED -> "Agent antérieur à 1.8.0 : mettez-le à jour pour enregistrer les mesures en continu."
        AgentError.NO_KEY -> ""
        else -> "Agent : ${error.name.lowercase()}"
    }

    private suspend fun agentDays(d: Device): AgentResult<List<String>> {
        val settings = d.agent
        if (settings == null || !settings.hasKey || !d.hasHost) return AgentResult.Failure(AgentError.NO_KEY)
        dayLists[d.id]?.let { (at, r) -> if (System.currentTimeMillis() - at < AGENT_DAYS_TTL_MS) return r }
        val r = when (val m = timed { agent.metrics(d.host, settings) }) {
            is AgentResult.Success -> AgentResult.Success(m.value.days)
            is AgentResult.Failure -> m
        }
        dayLists[d.id] = System.currentTimeMillis() to r
        return r
    }

    private suspend fun agentDay(d: Device, day: String): AgentResult<List<MetricsRow>> {
        val key = "${d.id}|$day"
        val ttl = if (day == LocalDate.now(ZoneOffset.UTC).toString()) TODAY_TTL_MS else PAST_DAY_TTL_MS
        agentRows[key]?.let { (at, rows) -> if (System.currentTimeMillis() - at < ttl) return AgentResult.Success(rows) }
        val settings = d.agent ?: return AgentResult.Failure(AgentError.NO_KEY)
        return when (val r = timed { agent.metrics(d.host, settings, day) }) {
            is AgentResult.Success -> {
                agentRows[key] = System.currentTimeMillis() to r.value.rows
                AgentResult.Success(r.value.rows)
            }
            is AgentResult.Failure -> r
        }
    }

    /** Un jour de mesures d'un PC dans les archives (null s'il n'y est pas). */
    private suspend fun archiveDay(acc: Access, mac: String, day: String): List<MetricsRow>? {
        var merged = ArchiveDay(day)
        var found = false
        for (id in months(acc, refresh = false)[day.take(7)].orEmpty()) {
            val gist = try {
                openGist(acc, id, day.take(7))
            } catch (_: ArchiveException) {
                continue // autre mot de passe, ou Gist abîmé : ignoré
            }
            val text = gist.files[ArchiveCodec.dayFile(day)] ?: continue
            val content = try {
                withContext(Dispatchers.Default) { gist.key.open(ArchiveCodec.dayFile(day), text, ArchiveDay.serializer()) }
            } catch (_: ArchiveException) {
                continue
            }
            val rows = content.rows(mac)
            if (rows.isNotEmpty()) {
                merged = merged.addRows(mac, "", rows).first
                found = true
            }
        }
        return if (found) merged.rows(mac) else null
    }

    /** Journal d'un PC entre [fromMs] et [toMs] : celui de l'agent et celui des archives. */
    suspend fun journal(deviceId: String, fromMs: Long, toMs: Long): List<HistoryEvent> {
        val (d, own) = device(deviceId) ?: throw IOException("PC introuvable")
        val raw = ArrayList<AgentHistoryEvent>()
        val settings = d.agent
        if (settings != null && settings.hasKey && d.hasHost) {
            val cached = journals[d.id]?.takeIf { System.currentTimeMillis() - it.first < AGENT_DAYS_TTL_MS }?.second
            if (cached != null) {
                raw += cached
            } else {
                val r = timed { agent.history(d.host, settings) }
                if (r is AgentResult.Success) {
                    journals[d.id] = System.currentTimeMillis() to r.value.events
                    raw += r.value.events
                }
            }
        }
        val acc = if (own) access() else null
        if (acc != null) {
            try {
                val byMonth = months(acc, refresh = false)
                var month = Instant.ofEpochMilli(fromMs).atOffset(ZoneOffset.UTC).toLocalDate().withDayOfMonth(1)
                val last = Instant.ofEpochMilli(toMs).atOffset(ZoneOffset.UTC).toLocalDate()
                while (!month.isAfter(last)) {
                    val name = month.toString().take(7)
                    for (id in byMonth[name].orEmpty()) {
                        val gist = try {
                            openGist(acc, id, name)
                        } catch (_: ArchiveException) {
                            continue
                        }
                        val text = gist.files[ArchiveCodec.journalFile(name)] ?: continue
                        try {
                            raw += withContext(Dispatchers.Default) {
                                gist.key.open(ArchiveCodec.journalFile(name), text, ArchiveJournal.serializer())
                            }.events(d.mac.toString())
                        } catch (_: ArchiveException) {
                            // Journal illisible : ignoré.
                        }
                    }
                    month = month.plusMonths(1)
                }
            } catch (e: ShareException) {
                DiagnosticLog.w(AREA, "journal archivé illisible", e)
            }
        }
        return raw.distinct()
            .filter { it.t * 1000 in fromMs until toMs }
            .mapNotNull { HistoryData.fromAgent(d.id, it) }
            .sortedByDescending { it.time }
    }

    /** Résumé pour le rapport de diagnostic (sans secret). */
    fun describe(): String {
        val st = backups.current.value ?: return "réglages pas encore lus"
        val a = st.archive
        return "activées ${a.enabled}, dernière réussite ${if (a.last > 0) Instant.ofEpochMilli(a.last) else "-"}, ${a.synced.size} PC suivis, " +
            "${a.gists.size} mois${runtime.value.error?.let { ", erreur : $it" }.orEmpty()}"
    }

    private companion object {
        const val AREA = "archives"
        const val START_MS = 2 * 60_000L
        const val TICK_MS = 30_000L
        const val EVERY_MS = 30 * 60_000L
        const val RETRY_MS = 10 * 60_000L
        const val AGENT_TIMEOUT_MS = 20_000L
        const val GIST_TTL_MS = 10 * 60_000L
        const val MONTHS_TTL_MS = 10 * 60_000L
        const val AGENT_DAYS_TTL_MS = 60_000L
        const val PAST_DAY_TTL_MS = 10 * 60_000L
        const val TODAY_TTL_MS = 30_000L
        const val MAX_RANGE_DAYS = 93
    }
}

// --- Réduction des mesures pour les graphiques (mêmes règles que l'application Windows) ---

private class Agg {
    var sum = 0.0
    var weight = 0.0
    var max = 0.0
    var n = 0

    fun add(avg: Double?, maxValue: Double?, w: Int) {
        if (avg == null) return
        val weight = maxOf(w, 1).toDouble()
        sum += avg * weight
        this.weight += weight
        val m = maxValue ?: avg
        if (n == 0 || m > max) max = m
        n++
    }

    fun avg(): Double? = if (n == 0) null else round1(sum / weight)
    fun max(): Double? = if (n == 0) null else round1(max)
}

private fun round1(v: Double) = (v * 10).roundToLong() / 10.0

private class PointAgg {
    val ct = Agg()
    val gt = Agg()
    val cl = Agg()
    val gl = Agg()

    fun add(r: MetricsRow) {
        ct.add(r.ct, r.ctx, r.n)
        gt.add(r.gt, r.gtx, r.n)
        cl.add(r.cl, r.clx, r.n)
        gl.add(r.gl, r.glx, r.n)
    }

    fun point(t: Long) = MetricsPoint(t, ct.avg(), ct.max(), gt.avg(), gt.max(), cl.avg(), cl.max(), gl.avg(), gl.max())
}

/** Résultat de [downsample] : pas (s), points, résumé de la période, minutes couvertes. */
internal data class Downsampled(val step: Long, val points: List<MetricsPoint>, val summary: MetricsPoint, val minutes: Int)

/**
 * Regroupe les minutes de [from, to[ (secondes) en au plus [points] points : moyenne pondérée par le
 * nombre de relevés, maximum des maximums.
 */
internal fun downsample(rows: List<MetricsRow>, from: Long, to: Long, points: Int): Downsampled {
    var step = maxOf(60L, (to - from + points - 1) / points)
    step = (step + 59) / 60 * 60
    val buckets = java.util.TreeMap<Long, PointAgg>()
    val total = PointAgg()
    val seen = HashSet<Long>()
    for (r in rows) {
        if (r.t < from || r.t >= to || !seen.add(r.t)) continue
        val bucket = buckets.getOrPut((r.t - from) / step) { PointAgg() }
        bucket.add(r)
        total.add(r)
    }
    return Downsampled(step, buckets.map { (i, b) -> b.point((from + i * step) * 1000) }, total.point(from * 1000), seen.size)
}
