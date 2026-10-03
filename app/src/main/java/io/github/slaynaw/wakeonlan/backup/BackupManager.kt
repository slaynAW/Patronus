package io.github.slaynaw.wakeonlan.backup

import android.content.Context
import android.net.Uri
import android.provider.DocumentsContract
import io.github.slaynaw.wakeonlan.BuildConfig
import io.github.slaynaw.wakeonlan.core.archive.ArchiveCodec
import io.github.slaynaw.wakeonlan.core.archive.ArchiveException
import io.github.slaynaw.wakeonlan.core.backup.BackupNames
import io.github.slaynaw.wakeonlan.core.config.ConfigCodec
import io.github.slaynaw.wakeonlan.core.config.ConfigException
import io.github.slaynaw.wakeonlan.core.config.ExportCodec
import io.github.slaynaw.wakeonlan.core.history.HistoryData
import io.github.slaynaw.wakeonlan.core.share.ExportedShareOwner
import io.github.slaynaw.wakeonlan.core.share.GistFile
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
import java.security.MessageDigest
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
    /** Compte du partage dont GitHub refuse la connexion : la connexion par code le reconnecte aussi. */
    val loginShareUser: String? = null,
    /** Changement du mot de passe en cours (null : aucun). */
    val rotation: RotationUi? = null,
    /** Mot de passe changé sur un autre appareil : nouveau mot de passe à saisir. */
    val stale: Boolean = false,
    /** Fin du dernier changement de mot de passe (millisecondes). */
    val rotated: Long = 0,
)

/** Changement du mot de passe des sauvegardes en cours : étape affichée, erreur (reprise automatique). */
data class RotationUi(val running: Boolean, val step: String?, val error: String?)

/** Le mot de passe des sauvegardes a été changé sur un autre appareil. */
class StalePasswordException :
    IOException("le mot de passe des sauvegardes a été changé sur un autre appareil : saisissez le nouveau mot de passe")

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
        val loginShareUser: String? = null,
        /** Mot de passe changé sur un autre appareil ; Gist dont le mot de passe a été vérifié. */
        val stale: Boolean = false,
        val checkedGist: String? = null,
        val rotating: Boolean = false,
        val rotateStep: String? = null,
        val rotateError: String? = null,
        val rotateAt: Long = 0,
        val rotated: Long = 0,
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
            loginShareUser = rt.loginShareUser?.takeIf { rt.login != null },
            rotation = st?.rotation?.let { RotationUi(rt.rotating, rt.rotateStep, rt.rotateError) },
            stale = rt.stale,
            rotated = rt.rotated,
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
                if (rotationDue()) runRotation()
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
        if (!st.enabled || st.password.isEmpty() || rt.running || rt.paused != null || !hasTarget(st) || st.rotation != null || rt.stale) return false
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
        val initial = settings.value ?: throw IOException("réglages des sauvegardes pas encore lus")
        if (!initial.enabled || initial.password.isEmpty()) throw IOException("sauvegarde automatique désactivée")
        if (!hasTarget(initial)) throw IOException("choisissez d'abord où sauvegarder (GitHub ou un dossier)")
        if (runtime.value.running) throw IOException("sauvegarde déjà en cours")
        if (initial.rotation != null) throw IOException("changement du mot de passe en cours : la sauvegarde suivra")
        if (runtime.value.stale) throw StalePasswordException()
        runtime.update { it.copy(running = true, dirty = false, attemptAt = System.currentTimeMillis(), paused = if (manual) null else it.paused) }
        try {
            // Mot de passe changé sur un autre appareil ? Puis identifiant anonyme (fichiers renommés).
            try {
                checkPassword()
            } catch (e: ShareException) {
                DiagnosticLog.w(AREA, "vérification du mot de passe sur GitHub impossible (${e.reason})", e)
            }
            migrateDevice()
            val st = settings.value ?: initial
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
                // Envoi dans le Gist ; renvoie les fichiers de ce téléphone qui y restent.
                suspend fun upload(gist: String, present: List<String>): List<String> {
                    val names = present.filter { it != name } + name
                    val old = BackupNames.outdated(names, st.device)
                    github.updateGist(gh.token, gist, buildMap<String, String?> {
                        put(name, backup.text)
                        old.forEach { put(it, null) }
                    })
                    DiagnosticLog.i(AREA, "sauvegarde GitHub : $name (${backup.devices} PC, ${backup.text.length} octets, ${old.size} ancienne(s) version(s) supprimée(s))")
                    return names - old.toSet()
                }
                try {
                    uploaded = try {
                        upload(gh.gist, uploaded)
                    } catch (e: ShareException) {
                        if (e.reason != ShareException.Reason.NOT_FOUND) throw e
                        // Gist recopié par un autre appareil (mot de passe changé, fichiers renommés) : retrouvé
                        // sur le compte, mot de passe vérifié, puis nouvel essai.
                        val found = gistFiles(gh.token, gh.gist, strict = false)?.first
                        if (found == null || found == gh.gist) throw e
                        checkPassword()
                        upload(found, settings.value?.uploaded.orEmpty())
                    }
                    result = result.copy(github = name)
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

    /** Fichiers du dossier choisi (nom → document). */
    private fun folderChildren(tree: Uri): Map<String, Uri> {
        val treeId = DocumentsContract.getTreeDocumentId(tree)
        val existing = HashMap<String, Uri>()
        context.contentResolver.query(
            DocumentsContract.buildChildDocumentsUriUsingTree(tree, treeId),
            arrayOf(DocumentsContract.Document.COLUMN_DOCUMENT_ID, DocumentsContract.Document.COLUMN_DISPLAY_NAME),
            null, null, null,
        )?.use { c ->
            while (c.moveToNext()) existing[c.getString(1)] = DocumentsContract.buildDocumentUriUsingTree(tree, c.getString(0))
        } ?: throw IOException("dossier illisible")
        return existing
    }

    private fun writeDocument(target: Uri, content: ByteArray) {
        (context.contentResolver.openOutputStream(target, "wt") ?: throw IOException("fichier inaccessible")).use { it.write(content) }
    }

    private fun readDocument(source: Uri): String =
        (context.contentResolver.openInputStream(source) ?: throw IOException("fichier illisible")).use { it.readBytes().toString(Charsets.UTF_8) }

    /** Écrit la sauvegarde dans le dossier (remplace celle du jour) et supprime les versions en trop. */
    private fun writeFolder(tree: Uri, device: String, name: String, content: ByteArray) {
        val resolver = context.contentResolver
        val parent = DocumentsContract.buildDocumentUriUsingTree(tree, DocumentsContract.getTreeDocumentId(tree))
        val existing = folderChildren(tree)
        val target = existing[name] ?: DocumentsContract.createDocument(resolver, parent, "application/json", name)
            ?: throw IOException("fichier non créé")
        writeDocument(target, content)
        for (old in BackupNames.outdated(existing.keys + name, device)) {
            existing[old]?.let { runCatching { DocumentsContract.deleteDocument(resolver, it) } }
        }
    }

    // --- Changement du mot de passe et identifiant anonyme (docs/SAUVEGARDE.md) ---
    //
    // Changer le mot de passe rechiffre tout ce que l'ancien ouvre : sauvegardes de tous les appareils
    // (Gist et dossier) et archives des mesures. Chaque Gist est recopié dans un Gist neuf, vérifié, puis
    // l'ancien est supprimé : son historique (versions chiffrées par l'ancien mot de passe) disparaît avec
    // lui. Rien n'est perdu ; un changement interrompu reprend où il s'était arrêté. Les autres appareils
    // s'en aperçoivent (leurs sauvegardes ne s'ouvrent plus avec leur mot de passe) et demandent le
    // nouveau. Les fichiers portaient le nom du téléphone avant la 1.9.0 : ils sont renommés avec un
    // identifiant anonyme, dans un Gist recopié pour que l'ancien nom disparaisse de l'historique.

    /** Changement de mot de passe à terminer, ou nouveau mot de passe attendu : sauvegardes et archives attendent. */
    fun rotationPending(): Boolean = settings.value?.rotation != null || runtime.value.stale

    private fun rotationDue(): Boolean {
        val st = settings.value ?: return false
        val rt = runtime.value
        return st.rotation != null && st.password.isNotEmpty() && !rt.rotating &&
            (rt.rotateError == null || System.currentTimeMillis() - rt.rotateAt >= ROTATE_RETRY_MS)
    }

    /**
     * Fichiers du Gist des sauvegardes ; s'il a disparu (recopié par un autre appareil), celui du compte
     * est retrouvé par sa description et enregistré. null : aucun Gist sur le compte.
     */
    private suspend fun gistFiles(token: String, gist: String, strict: Boolean): Pair<String, List<GistFile>>? {
        suspend fun read(id: String) = if (strict) github.readGistAll(token, id, COPY_LIMIT) else github.readGist(token, id, MAX_BACKUP_BYTES)
        try {
            return gist to read(gist)
        } catch (e: ShareException) {
            if (e.reason != ShareException.Reason.NOT_FOUND) throw e
        }
        val found = github.findGist(token, BackupNames.GIST_DESCRIPTION)
        if (found == null || found == gist) return null
        val files = read(found)
        save { st ->
            val gh = st.github
            if (gh != null && gh.gist == gist) {
                st.copy(github = gh.copy(gist = found), uploaded = files.mapNotNull { f -> BackupNames.parse(f.name)?.takeIf { it.device == st.device }?.name })
            } else {
                st
            }
        }
        DiagnosticLog.i(AREA, "Gist des sauvegardes ${gist.take(8)} introuvable, ${found.take(8)} retrouvé sur le compte")
        return found to files
    }

    /** Sauvegarde la plus récente de ce téléphone (ancien identifiant compris). */
    private fun latestOwn(files: List<GistFile>, device: String, legacy: String): GistFile? {
        val entry = BackupNames.sorted(files.map { it.name }).firstOrNull { it.device == device || (legacy.isNotEmpty() && it.device == legacy) }
            ?: return null
        return files.first { it.name == entry.name }
    }

    /**
     * Vérifie (une fois par session et par Gist) que le mot de passe enregistré ouvre encore la dernière
     * sauvegarde de ce téléphone sur GitHub. Sinon, il a été changé sur un autre appareil : sauvegardes
     * et archives s'arrêtent jusqu'à la saisie du nouveau ([StalePasswordException]).
     */
    internal suspend fun checkPassword() {
        if (runtime.value.stale) throw StalePasswordException()
        val st = settings.value ?: return
        val gh = st.github ?: return
        if (gh.gist.isEmpty() || gh.token.isEmpty() || st.password.isEmpty() || runtime.value.checkedGist == gh.gist) return
        val (id, files) = gistFiles(gh.token, gh.gist, strict = false) ?: return
        val own = latestOwn(files, st.device, st.legacyDevice)
        val stale = own != null && try {
            withContext(Dispatchers.Default) { ExportCodec.checkPassword(own.content, st.password.toCharArray()) }
            false
        } catch (e: ConfigException) {
            e.reason == ConfigException.Reason.WRONG_PASSWORD
        }
        if (stale) {
            runtime.update { it.copy(stale = true, errGithub = StalePasswordException().message) }
            DiagnosticLog.w(AREA, "mot de passe des sauvegardes changé sur un autre appareil : sauvegardes et archives arrêtées")
            throw StalePasswordException()
        }
        runtime.update { it.copy(checkedGist = id) }
    }

    /** Répartit les fichiers (triés par nom) en lots d'environ [BATCH_BYTES]. */
    private fun batches(files: Map<String, String>): List<Map<String, String>> {
        val out = ArrayList<Map<String, String>>()
        var current = LinkedHashMap<String, String>()
        var size = 0
        for (name in files.keys.sorted()) {
            val content = files.getValue(name)
            if (current.isNotEmpty() && size + content.length > BATCH_BYTES) {
                out += current
                current = LinkedHashMap()
                size = 0
            }
            current[name] = content
            size += content.length
        }
        if (current.isNotEmpty()) out += current
        return out
    }

    /**
     * Recopie [files] dans un Gist secret neuf (même description), vérifie la copie, puis supprime
     * l'ancien Gist : son historique disparaît avec lui. Un remplaçant déjà créé par une recopie
     * interrompue est réutilisé. Renvoie l'identifiant du nouveau Gist.
     */
    private suspend fun replaceGist(token: String, oldId: String, description: String, files: Map<String, String>): String {
        if (files.isEmpty()) throw IOException("Gist vide : rien à recopier")
        val all = batches(files)
        var reused = settings.value?.replacing?.get(oldId)
        var extra = emptyList<String>()
        if (reused != null) {
            try {
                extra = github.readGist(token, reused, COPY_LIMIT).map { it.name }.filter { it !in files }
            } catch (e: ShareException) {
                if (e.reason != ShareException.Reason.NOT_FOUND) throw e
                reused = null
            }
        }
        val newId: String
        var pending = all
        if (reused == null) {
            val created = github.createGist(token, description, all.first())
            save { it.copy(replacing = it.replacing + (oldId to created)) }
            newId = created
            pending = all.drop(1)
            DiagnosticLog.i(AREA, "recopie du Gist ${oldId.take(8)} dans ${created.take(8)} (${files.size} fichier(s))")
        } else {
            newId = reused
        }
        pending.forEachIndexed { i, batch ->
            val update = HashMap<String, String?>(batch)
            if (i == 0) extra.forEach { update[it] = null }
            github.updateGist(token, newId, update)
        }
        // Vérification : la copie doit être complète et identique avant de supprimer l'original.
        val copied = github.readGistAll(token, newId, COPY_LIMIT).associate { it.name to it.content }
        for ((name, content) in files) {
            if (copied[name]?.trim() != content.trim()) throw IOException("copie du Gist incomplète ($name) : l'original est gardé")
        }
        if (copied.size != files.size) throw IOException("copie du Gist différente de l'original : l'original est gardé")
        try {
            github.deleteGist(token, oldId)
        } catch (e: ShareException) {
            if (e.reason != ShareException.Reason.NOT_FOUND) throw e
        }
        save { it.copy(replacing = it.replacing - oldId) }
        DiagnosticLog.i(AREA, "Gist ${oldId.take(8)} remplacé par ${newId.take(8)} (ancien supprimé avec son historique)")
        return newId
    }

    /**
     * Recopie le Gist des sauvegardes dans un Gist neuf : sauvegardes que [oldPassword] ouvre rechiffrées
     * par [newPassword] (simple recopie si les deux sont égaux), fichiers de l'ancien identifiant [legacy]
     * renommés. Renvoie le Gist (null : aucun Gist sur le compte).
     */
    private suspend fun rebuildBackupGist(token: String, gist: String, oldPassword: String, newPassword: String, legacy: String, device: String): String? {
        val (id, files) = gistFiles(token, gist, strict = true) ?: return null
        val present = files.map { it.name }.toSet()
        val out = linkedMapOf("LISEZMOI.md" to BackupNames.GIST_NOTE)
        var reencrypted = 0
        var kept = 0
        var renamed = 0
        for (f in files) {
            var name = f.name
            var content = f.content
            if (legacy.isNotEmpty()) {
                val to = BackupNames.renamed(name, legacy, device)
                if (to != null) {
                    if (to in present) continue // sauvegarde du même jour déjà faite sous le nouvel identifiant
                    name = to
                    renamed++
                }
            }
            if (oldPassword != newPassword && BackupNames.parse(f.name) != null) {
                try {
                    content = withContext(Dispatchers.Default) {
                        ExportCodec.reencrypt(content, oldPassword.toCharArray(), newPassword.toCharArray())
                    }
                    reencrypted++
                } catch (e: ConfigException) {
                    kept++ // autre mot de passe (autre appareil) ou déjà rechiffrée : gardée telle quelle
                }
            }
            out[name] = content
        }
        if (oldPassword == newPassword && renamed == 0) return id
        val newId = replaceGist(token, id, BackupNames.GIST_DESCRIPTION, out)
        DiagnosticLog.i(AREA, "Gist des sauvegardes recopié : $reencrypted rechiffrée(s), $kept gardée(s) telle(s) quelle(s), $renamed renommée(s)")
        save { st ->
            val gh = st.github
            if (gh != null && (gh.gist == gist || gh.gist == id)) {
                st.copy(github = gh.copy(gist = newId), uploaded = out.keys.filter { BackupNames.parse(it)?.device == device }.sorted())
            } else {
                st
            }
        }
        runtime.update { it.copy(checkedGist = newId) }
        return newId
    }

    /** Renomme les sauvegardes de l'ancien identifiant dans le dossier. */
    private fun migrateFolder(tree: Uri, legacy: String, device: String) {
        val resolver = context.contentResolver
        val children = folderChildren(tree)
        for ((name, uri) in children) {
            val to = BackupNames.renamed(name, legacy, device) ?: continue
            if (to in children) {
                runCatching { DocumentsContract.deleteDocument(resolver, uri) }
                continue
            }
            DocumentsContract.renameDocument(resolver, uri, to) ?: throw IOException("fichier non renommé")
        }
    }

    /**
     * Remplace l'identifiant de ce téléphone (avec son nom, avant la 1.9.0) par un identifiant anonyme et
     * renomme ses sauvegardes (Gist recopié, dossier). Rien n'est perdu ; un échec est retenté à la
     * sauvegarde suivante.
     */
    private suspend fun migrateDevice() {
        var st = settings.value ?: return
        if (st.device.isNotEmpty() && !BackupNames.isAnonymous(st.device)) {
            st = save { s ->
                val legacy = s.legacyDevice.ifEmpty { s.device }
                val device = BackupNames.deviceId("android")
                s.copy(device = device, legacyDevice = legacy, uploaded = s.uploaded.map { BackupNames.renamed(it, legacy, device) ?: it })
            }
            DiagnosticLog.i(AREA, "identifiant anonyme du téléphone : ${st.device} (fichiers à renommer)")
        }
        val legacy = st.legacyDevice
        if (legacy.isEmpty()) return
        var ok = true
        if (st.folder.isNotEmpty()) {
            try {
                withContext(Dispatchers.IO) { migrateFolder(Uri.parse(st.folder), legacy, st.device) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                ok = false
                DiagnosticLog.w(AREA, "renommage des sauvegardes du dossier impossible", e)
            }
        }
        val gh = st.github
        if (gh != null && gh.gist.isNotEmpty()) {
            try {
                rebuildBackupGist(gh.token, gh.gist, st.password, st.password, legacy, st.device)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                ok = false
                DiagnosticLog.w(AREA, "renommage des sauvegardes sur GitHub impossible, nouvel essai plus tard", e)
            }
        }
        if (ok) {
            save { it.copy(legacyDevice = "") }
            DiagnosticLog.i(AREA, "sauvegardes renommées : le nom du téléphone n'apparaît plus")
        }
    }

    /** Démarre le changement du mot de passe ([current] : mot de passe actuel) ; les deux sont vidés ensuite. */
    suspend fun changePassword(current: CharArray, password: CharArray) {
        try {
            val st = settings.value ?: throw IOException("réglages des sauvegardes pas encore lus")
            val rt = runtime.value
            when {
                !st.enabled || st.password.isEmpty() -> throw IOException("sauvegarde automatique désactivée")
                st.rotation != null -> throw IOException("changement du mot de passe déjà en cours")
                rt.stale -> throw StalePasswordException()
                rt.running -> throw IOException("sauvegarde en cours : réessayez dans un instant")
                !MessageDigest.isEqual(String(current).toByteArray(), st.password.toByteArray()) -> throw IOException("mot de passe actuel incorrect")
                password.size < ExportCodec.MIN_PASSWORD_LENGTH ->
                    throw IOException("mot de passe trop court (${ExportCodec.MIN_PASSWORD_LENGTH} caractères au moins)")
                String(password) == st.password -> throw IOException("le nouveau mot de passe est identique à l'actuel")
            }
            startRotation(st.password, String(password))
        } finally {
            current.fill(' ')
            password.fill(' ')
        }
        DiagnosticLog.i(AREA, "changement du mot de passe des sauvegardes demandé")
        launchRotation()
    }

    /**
     * Enregistre le nouveau mot de passe choisi sur un autre appareil (vérifié sur la dernière sauvegarde
     * de ce téléphone), puis rechiffre ce qui ne l'est pas encore (dossier…). [password] est vidé ensuite.
     */
    suspend fun updatePassword(password: CharArray) {
        try {
            val st = settings.value ?: throw IOException("réglages des sauvegardes pas encore lus")
            val gh = st.github
            if (!runtime.value.stale || gh == null || st.rotation != null) throw IOException("aucun nouveau mot de passe attendu")
            val files = gistFiles(gh.token, gh.gist, strict = false)?.second.orEmpty()
            val own = latestOwn(files, st.device, st.legacyDevice)
            if (own != null) {
                try {
                    withContext(Dispatchers.Default) { ExportCodec.checkPassword(own.content, password) }
                } catch (e: ConfigException) {
                    throw IOException("mot de passe incorrect : saisissez celui choisi sur l'autre appareil", e)
                }
            }
            startRotation(st.password, String(password))
        } finally {
            password.fill(' ')
        }
        DiagnosticLog.i(AREA, "nouveau mot de passe des sauvegardes saisi (changé sur un autre appareil)")
        launchRotation()
    }

    private suspend fun startRotation(old: String, password: String) {
        save { it.copy(rotation = BackupRotation(old, started = System.currentTimeMillis()), password = password) }
        runtime.update { it.copy(stale = false, checkedGist = null, errGithub = null, rotateError = null, rotateAt = 0) }
    }

    private fun launchRotation() {
        scope.launch {
            try {
                runRotation()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                DiagnosticLog.w(AREA, "changement du mot de passe interrompu, reprise automatique", e)
            }
        }
    }

    /** Poursuit le changement du mot de passe (étapes déjà faites sautées) ; exception si interrompu. */
    suspend fun runRotation() {
        while (true) {
            val current = runtime.value
            if (current.rotating) return
            if (runtime.compareAndSet(current, current.copy(rotating = true, rotateError = null))) break
        }
        try {
            val initial = settings.value ?: return
            val rotation = initial.rotation ?: return
            val old = rotation.old
            val password = initial.password
            if (password.isEmpty()) return
            val done = rotation.done.toMutableSet()
            suspend fun markDone(vararg keys: String) {
                done.addAll(keys)
                save { s -> s.rotation?.let { s.copy(rotation = it.copy(done = it.done + keys)) } ?: throw IOException("changement du mot de passe annulé") }
            }
            fun step(text: String) = runtime.update { it.copy(rotateStep = text) }
            val token = initial.github?.token.orEmpty()
            val gist = initial.github?.gist.orEmpty()
            val legacy = initial.legacyDevice
            val device = initial.device
            val folder = initial.folder

            if (token.isNotEmpty() && gist.isNotEmpty() && BackupRotation.BACKUPS !in done) {
                step("sauvegardes sur GitHub")
                rebuildBackupGist(token, gist, old, password, legacy, device)
                markDone(BackupRotation.BACKUPS)
            }
            if (token.isNotEmpty()) {
                step("archives des mesures")
                for (g in github.listGists(token, ArchiveCodec.DESCRIPTION_PREFIX).sortedBy { it.description }) {
                    val month = ArchiveCodec.monthOf(g.description) ?: continue
                    if (g.id in done) continue
                    step("archives de $month")
                    val files = try {
                        github.readGistAll(token, g.id, COPY_LIMIT).associate { it.name to it.content }
                    } catch (e: ShareException) {
                        if (e.reason == ShareException.Reason.NOT_FOUND) continue else throw e
                    }
                    val (out, unreadable) = try {
                        withContext(Dispatchers.Default) { ArchiveCodec.reencrypt(files, old.toCharArray(), password.toCharArray()) }
                    } catch (e: ArchiveException) {
                        // Autre mot de passe, déjà rechiffré, ou pas une archive : laissé tel quel.
                        if (!e.wrongPassword) DiagnosticLog.w(AREA, "archives $month : Gist ${g.id.take(8)} laissé tel quel", e)
                        markDone(g.id)
                        continue
                    }
                    if (unreadable.isNotEmpty()) DiagnosticLog.w(AREA, "archives $month : ${unreadable.size} fichier(s) abîmé(s) recopié(s) tel(s) quel(s)")
                    val newId = replaceGist(token, g.id, g.description, out)
                    save { s -> s.copy(archive = s.archive.copy(gists = s.archive.gists.mapValues { (_, id) -> if (id == g.id) newId else id })) }
                    markDone(g.id, newId)
                    DiagnosticLog.i(AREA, "archives $month rechiffrées (Gist ${newId.take(8)})")
                }
            }
            if (folder.isNotEmpty() && BackupRotation.FOLDER !in done) {
                step("dossier des sauvegardes")
                val count = withContext(Dispatchers.IO) {
                    val tree = Uri.parse(folder)
                    if (legacy.isNotEmpty()) migrateFolder(tree, legacy, device)
                    var n = 0
                    for ((name, uri) in folderChildren(tree)) {
                        if (BackupNames.parse(name) == null) continue
                        val next = try {
                            ExportCodec.reencrypt(readDocument(uri), old.toCharArray(), password.toCharArray())
                        } catch (e: ConfigException) {
                            continue // autre mot de passe ou déjà rechiffrée
                        }
                        writeDocument(uri, next.toByteArray(Charsets.UTF_8))
                        n++
                    }
                    n
                }
                DiagnosticLog.i(AREA, "dossier : $count sauvegarde(s) rechiffrée(s)")
                markDone(BackupRotation.FOLDER)
            }
            // Archives : tout ce que les agents gardent est fusionné de nouveau (rien de ce qu'un autre
            // appareil aurait écrit pendant la recopie n'est perdu).
            save { s ->
                s.copy(
                    rotation = null,
                    legacyDevice = if (legacy.isNotEmpty() && s.legacyDevice == legacy && token.isNotEmpty()) "" else s.legacyDevice,
                    archive = s.archive.resetProgress(),
                )
            }
            runtime.update { it.copy(rotated = System.currentTimeMillis(), dirty = true, changedAt = 0) }
            DiagnosticLog.i(AREA, "changement du mot de passe des sauvegardes terminé")
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            val message = e.message ?: e.javaClass.simpleName
            runtime.update { it.copy(rotateError = "Changement du mot de passe interrompu, reprise automatique : $message") }
            throw e
        } finally {
            runtime.update { it.copy(rotating = false, rotateStep = null, rotateAt = System.currentTimeMillis()) }
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
                    device = st.device.ifEmpty { BackupNames.deviceId("android") },
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
        if (settings.value?.rotation != null) throw IOException("changement du mot de passe en cours : attendez qu'il se termine")
        save { it.copy(enabled = false, password = "", archive = it.archive.copy(enabled = false)) }
        runtime.update { it.copy(stale = false, checkedGist = null) }
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
        // Compte du partage à reconnecter par la même occasion (jeton refusé ou déjà oublié).
        var shareUser: String? = null
        if (owner != null && owner.user.isNotEmpty()) {
            if (owner.token.isNotEmpty()) {
                try {
                    attach(owner.token, owner.user)
                    return owner.user
                } catch (e: ShareException) {
                    // Jeton expiré ou révoqué : connexion par code, dont le jeton servira aussi au partage.
                    if (e.reason != ShareException.Reason.UNAUTHORIZED) throw e
                    DiagnosticLog.w(AREA, "jeton GitHub du partage refusé (@${owner.user}) : connexion par code", e)
                    share.tokenRejected(owner.token, e.message)
                }
            }
            shareUser = owner.user
        }
        loginJob?.cancel()
        val code = github.startLogin()
        runtime.update { it.copy(login = ShareLogin(code.userCode, code.verificationUri), loginShareUser = shareUser) }
        DiagnosticLog.i(AREA, "connexion GitHub des sauvegardes : code affiché")
        loginJob = scope.launch {
            try {
                val token = github.waitLogin(code, loginWake)
                val user = github.user(token)
                attach(token, user)
                runtime.update { it.copy(login = null) }
                share.adoptToken(token, user)
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
        if (settings.value?.rotation != null) throw IOException("changement du mot de passe en cours : attendez qu'il se termine")
        save { it.copy(github = null, lastGithub = 0, uploaded = emptyList(), archive = it.archive.resetProgress().copy(enabled = false)) }
        listing = emptyMap()
        runtime.update { it.copy(errGithub = null, stale = false, checkedGist = null) }
        DiagnosticLog.i(AREA, "GitHub déconnecté des sauvegardes")
    }

    // --- Restauration ---

    /** Sauvegardes du Gist, de la plus récente à la plus ancienne (null : GitHub non connecté). */
    suspend fun list(): List<BackupEntryView>? {
        val st = settings.value ?: return null
        val gh = st.github ?: return null
        val files = gistFiles(gh.token, gh.gist, strict = false)?.second.orEmpty()
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
            "Dossier : ${if (st.folder.isNotEmpty()) "${st.folderLabel}, dernière réussite ${instant(st.lastFolder)}" else "aucun"}${rt.errFolder?.let { ", erreur : $it" }.orEmpty()}" +
            (if (st.legacyDevice.isNotEmpty()) "\nAncien identifiant (avec le nom du téléphone) : fichiers à renommer" else "") +
            (st.rotation?.let { "\nChangement du mot de passe en cours depuis ${instant(it.started)}, ${it.done.size} étape(s) faite(s), en cours ${rt.rotating}${rt.rotateError?.let { e -> ", erreur : $e" }.orEmpty()}" }.orEmpty()) +
            (if (rt.stale) "\nMOT DE PASSE CHANGÉ SUR UN AUTRE APPAREIL : nouveau mot de passe à saisir" else "")
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
        const val ROTATE_RETRY_MS = 2 * 60_000L
        /** Volume envoyé par requête lors de la recopie d'un Gist ; taille maximale d'un fichier recopié. */
        const val BATCH_BYTES = 900 * 1024
        const val COPY_LIMIT = 16 shl 20
    }
}
