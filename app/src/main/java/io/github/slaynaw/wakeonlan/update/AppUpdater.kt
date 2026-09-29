package io.github.slaynaw.wakeonlan.update

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.content.pm.PackageManager
import android.content.pm.Signature
import android.os.Build
import androidx.core.content.edit
import io.github.slaynaw.wakeonlan.BuildConfig
import io.github.slaynaw.wakeonlan.core.update.UpdateClient
import io.github.slaynaw.wakeonlan.core.update.UpdateException
import io.github.slaynaw.wakeonlan.core.update.UpdateManifest
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import java.io.File
import java.io.IOException
import java.security.PublicKey
import java.security.cert.CertificateException
import java.security.cert.CertificateFactory

/** Étape d'une installation en cours. */
enum class UpdateStage { IDLE, DOWNLOADING, INSTALLING }

/** État des mises à jour intégrées, pour l'interface. */
data class UpdateUiState(
    /** Mises à jour disponibles pour cette version de l'application (signée par la clé officielle). */
    val enabled: Boolean = false,
    val auto: Boolean = true,
    val checking: Boolean = false,
    val lastCheck: Long = 0,
    val error: String? = null,
    val available: UpdateManifest? = null,
    /** « Plus tard » choisi pour cette version : ne plus la proposer d'elle-même pendant 24 h. */
    val postponed: Boolean = false,
    val stage: UpdateStage = UpdateStage.IDLE,
    val progress: Float = 0f,
)

/**
 * Mises à jour intégrées (mêmes règles que l'application Windows) : le manifeste de la dernière
 * version officielle est authentifié avec le certificat de signature de l'application installée,
 * l'APK téléchargé doit avoir la taille et l'empreinte annoncées, puis il est installé par
 * l'installateur d'Android, qui vérifie de nouveau que la signature est la même. Une mise à jour
 * conserve toutes les données (PC, clés, réglages, historique).
 */
class AppUpdater(context: Context, private val scope: CoroutineScope) {
    private val app = context.applicationContext
    private val prefs = app.getSharedPreferences("updates", Context.MODE_PRIVATE)
    private val client: UpdateClient? = signingKey()?.let { UpdateClient(it, userAgent = "Patronus-Android") }
    private val mutex = Mutex()
    private val _state = MutableStateFlow(
        UpdateUiState(
            enabled = client != null,
            auto = prefs.getBoolean(KEY_AUTO, true),
            lastCheck = prefs.getLong(KEY_LAST_CHECK, 0),
        ),
    )

    val state: StateFlow<UpdateUiState> = _state.asStateFlow()

    /** Au retour dans l'application : recherche automatique si la dernière date d'au moins 12 h. */
    fun checkIfDue() {
        val s = _state.value
        if (client == null || !s.auto || s.checking || s.stage != UpdateStage.IDLE) return
        if (System.currentTimeMillis() - s.lastCheck < CHECK_INTERVAL_MS) return
        scope.launch { check() }
    }

    /** Recherche une nouvelle version ; renvoie l'état obtenu. */
    suspend fun check(): UpdateUiState = mutex.withLock {
        val updater = client ?: return _state.value
        _state.update { it.copy(checking = true, error = null) }
        try {
            val manifest = updater.latest()
            val newer = manifest.takeIf { it.code > BuildConfig.VERSION_CODE && it.file(PLATFORM) != null }
            val now = System.currentTimeMillis()
            prefs.edit { putLong(KEY_LAST_CHECK, now) }
            DiagnosticLog.i(AREA, "recherche : dernière version ${manifest.version} (code ${manifest.code}), ${if (newer != null) "proposée" else "rien de plus récent"}")
            _state.update { it.copy(checking = false, lastCheck = now, available = newer, postponed = isPostponed(newer)) }
        } catch (e: UpdateException) {
            if (e.reason == UpdateException.Reason.NOT_PUBLISHED) {
                val now = System.currentTimeMillis()
                prefs.edit { putLong(KEY_LAST_CHECK, now) }
                _state.update { it.copy(checking = false, lastCheck = now, available = null) }
            } else {
                DiagnosticLog.w(AREA, "recherche impossible (${e.reason})", e)
                _state.update { it.copy(checking = false, error = e.message) }
            }
        }
        _state.value
    }

    fun setAuto(enabled: Boolean) {
        prefs.edit { putBoolean(KEY_AUTO, enabled) }
        _state.update { it.copy(auto = enabled) }
    }

    /** « Plus tard » : la version n'est plus proposée d'elle-même pendant 24 h. */
    fun postpone() {
        val version = _state.value.available?.version ?: return
        prefs.edit {
            putString(KEY_POSTPONED_VERSION, version)
            putLong(KEY_POSTPONED_AT, System.currentTimeMillis())
        }
        _state.update { it.copy(postponed = true) }
    }

    /** Télécharge, vérifie puis confie l'APK à l'installateur d'Android (en arrière-plan). */
    fun install() {
        val updater = client ?: return
        val manifest = _state.value.available ?: return
        val file = manifest.file(PLATFORM) ?: return
        if (_state.value.stage != UpdateStage.IDLE) return
        _state.update { it.copy(stage = UpdateStage.DOWNLOADING, progress = 0f, error = null) }
        DiagnosticLog.i(AREA, "installation de la version ${manifest.version} : téléchargement (${file.size} octets)")
        scope.launch {
            try {
                val dir = File(app.cacheDir, "updates").apply { mkdirs() }
                dir.listFiles()?.forEach { it.delete() }
                val apk = File(dir, "Patronus-update.apk")
                updater.download(manifest, file, apk) { done, total ->
                    _state.update { it.copy(progress = done.toFloat() / total) }
                }
                checkArchive(apk, manifest)
                _state.update { it.copy(stage = UpdateStage.INSTALLING, progress = 1f) }
                DiagnosticLog.i(AREA, "fichier vérifié ; confié à l'installateur d'Android")
                withContext(Dispatchers.IO) { commit(apk) }
            } catch (e: IOException) {
                DiagnosticLog.w(AREA, "installation impossible", e)
                fail(e.message)
            } catch (e: SecurityException) {
                DiagnosticLog.w(AREA, "installation impossible", e)
                fail(e.message)
            } catch (e: IllegalStateException) {
                DiagnosticLog.w(AREA, "installation impossible", e)
                fail(e.message)
            }
        }
    }

    /** Résultat de l'installateur d'Android (annulation, échec) ; en cas de succès, l'application est relancée par Android. */
    fun onInstallFailed(message: String?) = fail(message ?: "installation annulée")

    private fun fail(message: String?) {
        DiagnosticLog.w(AREA, "mise à jour impossible : $message")
        _state.update { it.copy(stage = UpdateStage.IDLE, progress = 0f, error = message) }
    }

    private fun isPostponed(manifest: UpdateManifest?): Boolean =
        manifest != null && prefs.getString(KEY_POSTPONED_VERSION, null) == manifest.version &&
            System.currentTimeMillis() - prefs.getLong(KEY_POSTPONED_AT, 0) < POSTPONE_MS

    /** L'APK doit être cette application, dans la version annoncée. */
    private fun checkArchive(apk: File, manifest: UpdateManifest) {
        val info = packageArchiveInfo(apk) ?: throw IOException("fichier de mise à jour illisible")
        val code = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) info.longVersionCode else @Suppress("DEPRECATION") info.versionCode.toLong()
        if (info.packageName != app.packageName || code != manifest.code) {
            throw IOException("le fichier téléchargé n'est pas la version annoncée")
        }
    }

    private fun packageArchiveInfo(apk: File) = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
        app.packageManager.getPackageArchiveInfo(apk.path, PackageManager.PackageInfoFlags.of(0))
    } else {
        @Suppress("DEPRECATION")
        app.packageManager.getPackageArchiveInfo(apk.path, 0)
    }

    private fun commit(apk: File) {
        val installer = app.packageManager.packageInstaller
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
            setAppPackageName(app.packageName)
            setSize(apk.length())
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                // Plus de confirmation une fois que l'application s'est elle-même mise à jour.
                setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED)
            }
        }
        val sessionId = installer.createSession(params)
        installer.openSession(sessionId).use { session ->
            session.openWrite("base.apk", 0, apk.length()).use { out ->
                apk.inputStream().use { it.copyTo(out) }
                session.fsync(out)
            }
            val intent = Intent(app, UpdateInstallReceiver::class.java).setPackage(app.packageName)
            val flags = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE
            } else {
                PendingIntent.FLAG_UPDATE_CURRENT
            }
            session.commit(PendingIntent.getBroadcast(app, sessionId, intent, flags).intentSender)
        }
    }

    /** Clé publique du certificat qui a signé l'application installée. */
    private fun signingKey(): PublicKey? = try {
        val pm = app.packageManager
        val signatures: Array<out Signature> = when {
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU ->
                pm.getPackageInfo(app.packageName, PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES.toLong()))
                    .signingInfo?.apkContentsSigners.orEmpty()
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.P ->
                @Suppress("DEPRECATION")
                pm.getPackageInfo(app.packageName, PackageManager.GET_SIGNING_CERTIFICATES).signingInfo?.apkContentsSigners.orEmpty()
            else ->
                @Suppress("DEPRECATION")
                pm.getPackageInfo(app.packageName, PackageManager.GET_SIGNATURES).signatures.orEmpty()
        }
        signatures.singleOrNull()?.let { signature ->
            CertificateFactory.getInstance("X.509").generateCertificate(signature.toByteArray().inputStream()).publicKey
        }
    } catch (e: PackageManager.NameNotFoundException) {
        null
    } catch (e: CertificateException) {
        null
    }

    private companion object {
        const val AREA = "mise-à-jour"
        const val PLATFORM = "android"
        const val KEY_AUTO = "auto"
        const val KEY_LAST_CHECK = "lastCheck"
        const val KEY_POSTPONED_VERSION = "postponedVersion"
        const val KEY_POSTPONED_AT = "postponedAt"
        const val CHECK_INTERVAL_MS = 12 * 60 * 60 * 1000L
        const val POSTPONE_MS = 24 * 60 * 60 * 1000L
    }
}
