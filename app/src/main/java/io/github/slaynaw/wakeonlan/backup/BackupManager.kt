package io.github.slaynaw.wakeonlan.backup

import android.content.Context
import android.net.Uri
import android.provider.DocumentsContract
import io.github.slaynaw.wakeonlan.BuildConfig
import io.github.slaynaw.wakeonlan.core.backup.BackupNames
import io.github.slaynaw.wakeonlan.core.config.ConfigCodec
import io.github.slaynaw.wakeonlan.core.config.ExportCodec
import io.github.slaynaw.wakeonlan.core.history.HistoryData
import io.github.slaynaw.wakeonlan.core.share.ExportedShareOwner
import io.github.slaynaw.wakeonlan.core.share.ShareException
import io.github.slaynaw.wakeonlan.core.share.ShareGitHub
import io.github.slaynaw.wakeonlan.data.ConfigRepository
import io.github.slaynaw.wakeonlan.data.HistoryRepository
import io.github.slaynaw.wakeonlan.diagnostics.DataKind
import io.github.slaynaw.wakeonlan.diagnostics.DataNotices
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import io.github.slaynaw.wakeonlan.share.ShareLogin
import io.github.slaynaw.wakeonlan.share.ShareManager
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonElement
import java.io.File
import java.io.IOException
import java.time.Instant
import java.time.LocalDate

/** Destination des sauvegardes affichée par l'interface. */
data class BackupTarget(val label: String, val last: Long, val error: String? = null)

/** Sauvegarde disponible sur GitHub. */
data class BackupEntryView(val name: String, val device: String, val date: LocalDate, val size: Int, val mine: Boolean)

/** État des sauvegardes pour l'interface (sans mot de passe ni jeton). */
data class BackupUiState(
    val loaded: Boolean = false,
    val canLogin: Boolean = false,
    val enabled: Boolean = false,
    val running: Boolean = false,
    /** Sauvegardes automatiques en pause (données illisibles au démarrage) : la raison. */
    val paused: String? = null,
    val loadError: String? = null,
    val github: BackupTarget? = null,
    val folder: BackupTarget? = null,
    val login: ShareLogin? = null,
)

/** Résultat d'une sauvegarde. */
data class BackupResult(val github: String? = null, val folder: String? = null, val skipped: String? = null, val errors: List<String> = emptyList())

/** Sauvegarde complète (PC, clés, clé de partage, historique), chiffrée par [password] (sans secret si null). */
data class FullBackup(val text: String, val devices: Int, val sharing: Boolean) {
    companion object {
        suspend fun build(config: ConfigRepository, share: ShareManager, history: HistoryRepository, password: CharArray?): FullBackup {
            val cfg = config.current()
            // Sauvegarde complète : la clé de partage suit, pour changer d'appareil sans réinviter, et
            // l'historique, pour le retrouver sur le nouvel appareil.
            val extra = buildMap<String, JsonElement> {
                if (password == null) return@buildMap
                share.exportOwner()?.let { put("sharing", ConfigCodec.json.encodeToJsonElement(ExportedShareOwner.serializer(), it)) }
                val events = history.data.value.forBackup(System.currentTimeMillis())
                put("history", ConfigCodec.json.parseToJsonElement(HistoryData.encode(events)))
            }
            val text = withContext(Dispatchers.Default) {
                ExportCodec.export(cfg, password, Instant.now().toString(), "Patronus ${BuildConfig.VERSION_NAME}", extra = extra)
            }
            return FullBackup(text, cfg.devices.size, extra.containsKey("sharing"))
        }
    }
}

/**
 * Sauvegardes automatiques (docs/SAUVEGARDE.md), mêmes règles que l'application Windows : après chaque
 * changement (PC, réglages, partage) et au moins une fois par jour tant que l'application est ouverte,
 * une sauvegarde complète chiffrée par le mot de passe des sauvegardes est écrite dans le Gist secret du
 * compte GitHub et/ou dans le dossier choisi ([BackupNames.KEEP] versions par appareil).
 */
class BackupManager(
    private val context: Context,
    private val scope: CoroutineScope,
    private val config: ConfigRepository,
    private val share: ShareManager,
    private val history: HistoryRepository,
) {
    private val store = BackupStore(File(context.filesDir, "backup.bin"))
    private val github = ShareGitHub(BuildConfig.GITHUB_CLIENT_ID, "Patronus-Android/${BuildConfig.VERSION_NAME}")
    private val mutex = Mutex()
    private val settings = MutableStateFlow<BackupSettings?>(null)

    /** Réglages (lecture seule) : l'archivage utilise le même compte GitHub et le même mot de passe. */
    internal val current: StateFlow<BackupSettings?> get() = settings

    private data class Runtime(
        val running: Boolean = false,
        val paused: String? = null,
        val loadError: String? = null,
        val errGithub: String? = null,
        val errFolder: String? = null,
        val login: ShareLogin? = null,
        val dirty: Boolean = false,
        val changedAt: Long = 0,
        val attemptAt: Long = 0,
        val failed: Boolean = false,
    )

    private val runtime = MutableStateFlow(Runtime())
    private var loginJob: Job? = null
    private val loginWake = Channel<Unit>(Channel.CONFLATED)
    @Volatile
    private var listing: Map<String, String> = emptyMap()
    @Volatile
    private var listedAt = 0L

    val state: StateFlow<BackupUiState> = combine(settings, runtime) { st, rt ->
        BackupUiState(
            loaded = st != null,
            canLogin = BuildConfig.GITHUB_CLIENT_ID.isNotEmpty(),
            enabled = st?.enabled == true,
            running = rt.running,
            paused = rt.paused,
            loadError = rt.loadError,
            github = st?.github?.let { BackupTarget("@${it.user}", st.lastGithub, rt.errGithub) },
            folder = st?.takeIf { it.folder.isNotEmpty() }?.let { BackupTarget(it.folderLabel, it.lastFolder, rt.errFolder) },
            login = rt.login,
        )
    }.stateIn(scope, SharingStarted.Eagerly, BackupUiState())

    init {
        scope.launch(Dispatchers.IO) {
            val loaded = try {
                store.load()
            } catch (e: IOException) {
                DiagnosticLog.e(AREA, "réglages illisibles", e)
                runtime.update { it.copy(loadError = e.message) }
                BackupSettings()
            }
            settings.value = loaded
        }
        // Données illisibles au démarrage : une sauvegarde automatique remplacerait la dernière bonne.
        scope.launch {
            DataNotices.notices.collect { kinds ->
                val reason = when {
                    DataKind.CONFIG in kinds -> "liste des PC illisible au démarrage"
                    DataKind.SHARE in kinds -> "partage illisible au démarrage"
                    else -> null
                }
                if (reason != null) runtime.update { it.copy(paused = reason) }
            }
        }
    }

    /** Tant que l'application est visible : sauvegardes après changement et quotidiennes. */
    suspend fun run(): Unit = coroutineScope {
        launch { config.config.drop(1).collect { markDirty() } }
        launch {
            share.state.filter { it.loaded }.map { it.owner }.distinctUntilChanged().drop(1).collect { markDirty() }
        }
        delay(START_MS)
        while (true) {
            try {
                if (isDue()) backupNow(manual = false)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                DiagnosticLog.e(AREA, "sauvegarde automatique en échec", e)
            }
            delay(TICK_MS)
        }
    }

    fun onResume() {
        if (loginJob?.isActive == true) loginWake.trySend(Unit)
    }

    private fun markDirty() = runtime.update { it.copy(dirty = true, changedAt = System.currentTimeMillis()) }

    private fun hasTarget(st: BackupSettings) = st.github?.gist?.isNotEmpty() == true || st.folder.isNotEmpty()

    private fun isDue(): Boolean {
        val st = settings.value ?: return false
        val rt = runtime.value
        val now = System.currentTimeMillis()
        if (!st.enabled || st.password.isEmpty() || rt.running || rt.paused != null || !hasTarget(st)) return false
        if (rt.failed && now - rt.attemptAt < RETRY_MS) return false
        if (rt.dirty && now - rt.changedAt >= DELAY_MS) return true
        val lasts = listOfNotNull(st.lastGithub.takeIf { st.github?.gist?.isNotEmpty() == true }, st.lastFolder.takeIf { st.folder.isNotEmpty() })
        return now - (lasts.minOrNull() ?: 0) >= DAILY_MS
    }

    /** Met à jour les réglages des archives (enregistrés avec ceux des sauvegardes). */
    internal suspend fun updateArchive(transform: (ArchiveSettings) -> ArchiveSettings) {
        save { it.copy(archive = transform(it.archive)) }
    }

    private suspend fun save(transform: (BackupSettings) -> BackupSettings): BackupSettings = mutex.withLock {
        val next = transform(settings.value ?: BackupSettings())
        withContext(Dispatchers.IO) { store.save(next) }
        settings.value = next
        next
    }

    /** Sauvegarde vers chaque destination ; [manual] : demandée par l'utilisateur (lève la pause). */
    suspend fun backupNow(manual: Boolean): BackupResult {
        val st = settings.value ?: throw IOException("réglages des sauvegardes pas encore lus")
        if (!st.enabled || st.password.isEmpty()) throw IOException("sauvegarde automatique désactivée")
        if (!hasTarget(st)) throw IOException("choisissez d'abord où sauvegarder (GitHub ou un dossier)")
        if (runtime.value.running) throw IOException("sauvegarde déjà en cours")
        runtime.update { it.copy(running = true, dirty = false, attemptAt = System.currentTimeMillis(), paused = if (manual) null else it.paused) }
        try {
            val password = st.password.toCharArray()
            val backup = try {
                FullBackup.build(config, share, history, password)
            } finally {
                password.fill(' ')
            }
            if (!manual && backup.devices == 0 && !backup.sharing) {
                // Rien à sauvegarder (données perdues ?) : ne pas remplacer une bonne sauvegarde.
                DiagnosticLog.w(AREA, "sauvegarde automatique ignorée : aucun PC ni partage")
                return BackupResult(skipped = "aucun PC ni partage à sauvegarder")
            }
            val name = BackupNames.fileName(st.device, LocalDate.now())
            var result = BackupResult()
            var errGithub: String? = null
            var errFolder: String? = null
            var uploaded = st.uploaded
            val gh = st.github
            if (gh != null && gh.gist.isNotEmpty()) {
                val names = uploaded.filter { it != name } + name
                val old = BackupNames.outdated(names, st.device)
                try {
                    val files = buildMap<String, String?> {
                        put(name, backup.text)
                        old.forEach { put(it, null) }
                    }
                    github.updateGist(gh.token, gh.gist, files)
                    uploaded = names - old.toSet()
                    result = result.copy(github = name)
                    DiagnosticLog.i(AREA, "sauvegarde GitHub : $name (${backup.devices} PC, ${backup.text.length} octets, ${old.size} ancienne(s) version(s) supprimée(s))")
                } catch (e: ShareException) {
                    errGithub = if (e.reason == ShareException.Reason.NOT_FOUND) {
                        "Gist des sauvegardes introuvable (supprimé ?) : reconnectez GitHub pour en créer un."
                    } else {
                        e.message.orEmpty()
                    }
                    result = result.copy(errors = result.errors + "GitHub : $errGithub")
                    DiagnosticLog.w(AREA, "sauvegarde GitHub impossible (${e.reason})", e)
                }
            }
            if (st.folder.isNotEmpty()) {
                try {
                    withContext(Dispatchers.IO) { writeFolder(Uri.parse(st.folder), st.device, name, backup.text.toByteArray(Charsets.UTF_8)) }
                    result = result.copy(folder = st.folderLabel)
                    DiagnosticLog.i(AREA, "sauvegarde dans le dossier ${st.folderLabel} : $name (${backup.devices} PC)")
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    errFolder = "dossier inaccessible (${e.javaClass.simpleName}) : choisissez-le à nouveau"
                    result = result.copy(errors = result.errors + "Dossier : $errFolder")
                    DiagnosticLog.w(AREA, "sauvegarde dans le dossier impossible", e)
                }
            }
            val now = System.currentTimeMillis()
            save { current ->
                current.copy(
                    lastGithub = if (result.github != null) now else current.lastGithub,
                    lastFolder = if (result.folder != null) now else current.lastFolder,
                    uploaded = if (result.github != null) uploaded else current.uploaded,
                )
            }
            runtime.update { it.copy(errGithub = errGithub, errFolder = errFolder, failed = result.errors.isNotEmpty()) }
            if (result.github == null && result.folder == null && result.errors.isNotEmpty()) throw IOException(result.errors.joinToString(" ; "))
            return result
        } finally {
            runtime.update { it.copy(running = false) }
        }
    }

    /** Écrit la sauvegarde dans le dossier (remplace celle du jour) et supprime les versions en trop. */
    private fun writeFolder(tree: Uri, device: String, name: String, content: ByteArray) {
        val resolver = context.contentResolver
        val treeId = DocumentsContract.getTreeDocumentId(tree)
        val parent = DocumentsContract.buildDocumentUriUsingTree(tree, treeId)
        val existing = HashMap<String, Uri>()
        resolver.query(
            DocumentsContract.buildChildDocumentsUriUsingTree(tree, treeId),
            arrayOf(DocumentsContract.Document.COLUMN_DOCUMENT_ID, DocumentsContract.Document.COLUMN_DISPLAY_NAME),
            null, null, null,
        )?.use { c ->
            while (c.moveToNext()) existing[c.getString(1)] = DocumentsContract.buildDocumentUriUsingTree(tree, c.getString(0))
        } ?: throw IOException("dossier illisible")
        val target = existing[name] ?: DocumentsContract.createDocument(resolver, parent, "application/json", name)
            ?: throw IOException("fichier non créé")
        (resolver.openOutputStream(target, "wt") ?: throw IOException("fichier inaccessible")).use { it.write(content) }
        for (old in BackupNames.outdated(existing.keys + name, device)) {
            existing[old]?.let { runCatching { DocumentsContract.deleteDocument(resolver, it) } }
        }
    }

    // --- Réglages ---

    /** Active les sauvegardes avec ce mot de passe (vidé ensuite). */
    suspend fun enable(password: CharArray) {
        try {
            save { st ->
                val newPassword = String(password)
                st.copy(
                    enabled = true,
                    password = newPassword,
                    device = st.device.ifEmpty { BackupNames.deviceId("android", android.os.Build.MODEL) },
                    // Nouveau mot de passe : les archives repartent dans des Gists qu'il ouvre (rattrapage complet).
                    archive = if (st.password != newPassword) st.archive.resetProgress() else st.archive,
                )
            }
        } finally {
            password.fill(' ')
        }
        DiagnosticLog.i(AREA, "sauvegarde automatique activée (appareil ${settings.value?.device})")
        runtime.update { it.copy(dirty = true, changedAt = 0) }
    }

    /** Désactive les sauvegardes et oublie le mot de passe (les sauvegardes existantes restent). */
    suspend fun disable() {
        save { it.copy(enabled = false, password = "", archive = it.archive.copy(enabled = false)) }
        DiagnosticLog.i(AREA, "sauvegarde automatique désactivée (archives arrêtées)")
    }

    /** Dossier choisi (URI d'arborescence, droit d'accès déjà conservé par l'appelant). */
    suspend fun setFolder(tree: Uri) {
        val label = runCatching { DocumentsContract.getTreeDocumentId(tree).substringAfter(':').ifEmpty { "dossier" } }.getOrDefault("dossier")
        save { it.copy(folder = tree.toString(), folderLabel = label, lastFolder = 0) }
        runtime.update { it.copy(errFolder = null, dirty = true, changedAt = 0) }
        DiagnosticLog.i(AREA, "dossier des sauvegardes : $label")
    }

    suspend fun removeFolder() {
        val previous = settings.value?.folder.orEmpty()
        save { it.copy(folder = "", folderLabel = "", lastFolder = 0) }
        runtime.update { it.copy(errFolder = null) }
        if (previous.isNotEmpty()) {
            runCatching {
                context.contentResolver.releasePersistableUriPermission(
                    Uri.parse(previous),
                    android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION or android.content.Intent.FLAG_GRANT_WRITE_URI_PERMISSION,
                )
            }
        }
    }

    // --- GitHub ---

    /**
     * Connecte GitHub : compte du partage s'il est connecté sur ce téléphone (renvoie son nom), sinon
     * connexion par code (renvoie null ; l'état porte le code, puis la connexion).
     */
    suspend fun connect(): String? {
        val owner = share.state.value.owner
        if (owner != null && owner.token.isNotEmpty() && owner.user.isNotEmpty()) {
            attach(owner.token, owner.user)
            return owner.user
        }
        loginJob?.cancel()
        val code = github.startLogin()
        runtime.update { it.copy(login = ShareLogin(code.userCode, code.verificationUri)) }
        DiagnosticLog.i(AREA, "connexion GitHub des sauvegardes : code affiché")
        loginJob = scope.launch {
            try {
                val token = github.waitLogin(code, loginWake)
                val user = github.user(token)
                attach(token, user)
                runtime.update { it.copy(login = null) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                DiagnosticLog.w(AREA, "connexion GitHub des sauvegardes en échec", e)
                runtime.update { rt -> rt.copy(login = rt.login?.copy(error = e.message ?: e.javaClass.simpleName)) }
            }
        }
        return null
    }

    fun cancelLogin() {
        loginJob?.cancel()
        loginJob = null
        runtime.update { it.copy(login = null) }
    }

    /** Enregistre le compte et retrouve (ou crée) le Gist des sauvegardes. */
    private suspend fun attach(token: String, user: String) {
        val gist = github.findGist(token, BackupNames.GIST_DESCRIPTION)
            ?: github.createGist(token, BackupNames.GIST_DESCRIPTION, mapOf("LISEZMOI.md" to BackupNames.GIST_NOTE))
        save {
            // Autre compte : archives à rattraper dans ses propres Gists.
            val archive = if (it.github?.user != user) it.archive.resetProgress() else it.archive
            it.copy(github = BackupGitHub(token, user, gist), archive = archive)
        }
        listing = emptyMap()
        runtime.update { it.copy(errGithub = null, dirty = true, changedAt = 0) }
        DiagnosticLog.i(AREA, "GitHub connecté pour les sauvegardes : @$user, gist ${gist.take(8)}…")
    }

    suspend fun disconnect() {
        save { it.copy(github = null, lastGithub = 0, uploaded = emptyList(), archive = it.archive.resetProgress().copy(enabled = false)) }
        listing = emptyMap()
        runtime.update { it.copy(errGithub = null) }
        DiagnosticLog.i(AREA, "GitHub déconnecté des sauvegardes")
    }

    // --- Restauration ---

    /** Sauvegardes du Gist, de la plus récente à la plus ancienne (null : GitHub non connecté). */
    suspend fun list(): List<BackupEntryView>? {
        val st = settings.value ?: return null
        val gh = st.github ?: return null
        val files = github.readGist(gh.token, gh.gist, MAX_BACKUP_BYTES)
        listing = files.associate { it.name to it.content }
        listedAt = System.currentTimeMillis()
        val sizes = files.associate { it.name to it.size }
        return BackupNames.sorted(files.map { it.name }).map {
            BackupEntryView(it.name, it.device, it.date, sizes[it.name] ?: 0, mine = it.device == st.device)
        }.also { DiagnosticLog.i(AREA, "restauration : ${it.size} sauvegarde(s) sur GitHub") }
    }

    /** Contenu d'une sauvegarde de la dernière liste (l'import habituel prend le relais). */
    fun content(name: String): String {
        if (System.currentTimeMillis() - listedAt > LIST_TTL_MS) throw IOException("liste des sauvegardes expirée : rouvrez-la")
        DiagnosticLog.i(AREA, "restauration de $name")
        return listing[name] ?: throw IOException("sauvegarde introuvable")
    }

    /** Résumé pour le rapport de diagnostic (sans secret). */
    fun describe(): String {
        val st = settings.value ?: return "réglages pas encore lus"
        val rt = runtime.value
        return "activées ${st.enabled}, mot de passe ${if (st.password.isNotEmpty()) "présent" else "absent"}, appareil ${st.device.ifEmpty { "-" }}, " +
            "en cours ${rt.running}${rt.paused?.let { ", EN PAUSE : $it" }.orEmpty()}${rt.loadError?.let { ", erreur : $it" }.orEmpty()}\n" +
            "GitHub : ${st.github?.let { "@${it.user}, gist ${it.gist.take(8)}, jeton ${if (it.token.isNotEmpty()) "présent" else "absent"}, dernière réussite ${instant(st.lastGithub)}, ${st.uploaded.size} version(s)" } ?: "non connecté"}" +
            "${rt.errGithub?.let { ", erreur : $it" }.orEmpty()}\n" +
            "Dossier : ${if (st.folder.isNotEmpty()) "${st.folderLabel}, dernière réussite ${instant(st.lastFolder)}" else "aucun"}${rt.errFolder?.let { ", erreur : $it" }.orEmpty()}"
    }

    private fun instant(millis: Long) = if (millis <= 0) "-" else Instant.ofEpochMilli(millis).toString()

    private companion object {
        const val AREA = "sauvegarde"
        const val START_MS = 30_000L
        const val TICK_MS = 10_000L
        const val DELAY_MS = 20_000L
        const val RETRY_MS = 10 * 60_000L
        const val DAILY_MS = 24 * 3_600_000L
        const val LIST_TTL_MS = 10 * 60_000L
        /** Taille maximale d'une sauvegarde relue (comme l'import d'un fichier). */
        const val MAX_BACKUP_BYTES = 1024 * 1024
    }
}
