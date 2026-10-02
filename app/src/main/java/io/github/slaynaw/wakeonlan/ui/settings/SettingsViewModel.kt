package io.github.slaynaw.wakeonlan.ui.settings

import android.content.ContentResolver
import android.content.Context
import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.github.slaynaw.wakeonlan.AppContainer
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.backup.FullBackup
import io.github.slaynaw.wakeonlan.core.config.ConfigCodec
import io.github.slaynaw.wakeonlan.core.config.ConfigException
import io.github.slaynaw.wakeonlan.core.config.ExportCodec
import io.github.slaynaw.wakeonlan.core.diagnostics.DiagnosticCodec
import io.github.slaynaw.wakeonlan.core.history.HistoryData
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.AppSettings
import io.github.slaynaw.wakeonlan.core.share.ExportedShareOwner
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticReport
import io.github.slaynaw.wakeonlan.ui.devices.UiMessage
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.IOException
import java.time.Instant

data class SettingsUiState(
    val settings: AppSettings = AppSettings(),
    val deviceCount: Int = 0,
    val hasSecrets: Boolean = false,
)

/** Étapes de l'import d'une sauvegarde. */
sealed interface ImportStep {
    data class NeedPassword(val text: String, val wrongPassword: Boolean = false) : ImportStep
    data class Confirm(val config: AppConfig, val sharing: ExportedShareOwner? = null, val history: HistoryData? = null) : ImportStep {
        /** Sauvegarde « sans secrets » : certaines clés d'agent devront être ressaisies. */
        val missingKeys: Boolean get() = config.devices.any { it.agent?.hasKey == false }
    }
}

class SettingsViewModel(private val container: AppContainer) : ViewModel() {

    val state: StateFlow<SettingsUiState> = container.repository.config.map { config ->
        SettingsUiState(
            settings = config.settings,
            deviceCount = config.devices.size,
            hasSecrets = config.devices.any { it.agent?.hasKey == true || it.secureOnPassword != null },
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), SettingsUiState())

    private val _busy = MutableStateFlow(false)
    val busy: StateFlow<Boolean> = _busy.asStateFlow()

    private val _importStep = MutableStateFlow<ImportStep?>(null)
    val importStep: StateFlow<ImportStep?> = _importStep.asStateFlow()

    private val _messages = Channel<UiMessage>(Channel.BUFFERED)
    val messages: Flow<UiMessage> = _messages.receiveAsFlow()

    /** Partage repris d'une sauvegarde : il faut se reconnecter à GitHub (message affiché une fois). */
    private val _sharingImported = MutableStateFlow(false)
    val sharingImported: StateFlow<Boolean> = _sharingImported.asStateFlow()

    fun dismissSharingImported() {
        _sharingImported.value = false
    }

    fun updateSettings(transform: (AppSettings) -> AppSettings) = viewModelScope.launch {
        container.repository.updateSettings(transform)
    }

    /** Écrit la sauvegarde ; [password] nul = export lisible sans aucun secret. */
    fun export(resolver: ContentResolver, uri: Uri, password: CharArray?) = work {
        val backup = try {
            FullBackup.build(container.repository, container.share, container.history, password)
        } finally {
            password?.fill(' ')
        }
        withContext(Dispatchers.IO) {
            (resolver.openOutputStream(uri, "wt") ?: throw IOException("Fichier inaccessible")).use {
                it.write(backup.text.toByteArray(Charsets.UTF_8))
            }
        }
        DiagnosticLog.i("sauvegarde", "export ${if (password != null) "complet (chiffré, partage : ${backup.sharing})" else "sans secrets"} : ${backup.devices} PC")
        _messages.send(UiMessage(R.string.message_export_done, listOf(backup.devices)))
    }

    /** Sauvegarde automatique choisie dans la liste de GitHub : l'import habituel prend le relais. */
    fun restoreBackup(name: String) = work {
        val text = container.backups.content(name)
        _importStep.value = if (ExportCodec.isEncrypted(text)) ImportStep.NeedPassword(text) else ImportStep.Confirm(ExportCodec.import(text, null))
    }

    /** Sauvegarde demandée par l'utilisateur. */
    fun backupNow() = work {
        val result = container.backups.backupNow(manual = true)
        if (result.errors.isEmpty()) _messages.send(UiMessage(R.string.backup_done)) else throw IOException(result.errors.joinToString("\n"))
    }

    /**
     * Écrit le rapport de diagnostic chiffré par [password] (format « patronus-diagnostic/1 ») : à
     * transmettre avec le mot de passe, par un autre canal, pour analyse.
     */
    fun exportDiagnostic(context: Context, uri: Uri, password: CharArray) = work {
        try {
            DiagnosticLog.i("diagnostic", "export du rapport")
            val report = DiagnosticReport.build(context, container)
            val sealed = withContext(Dispatchers.Default) {
                DiagnosticCodec.seal(report, password, DiagnosticReport.app(), Instant.now().toString())
            }
            withContext(Dispatchers.IO) {
                (context.contentResolver.openOutputStream(uri, "wt") ?: throw IOException("Fichier inaccessible")).use {
                    it.write(sealed.toByteArray(Charsets.UTF_8))
                }
            }
            _messages.send(UiMessage(R.string.message_diagnostic_done))
        } finally {
            password.fill(' ')
        }
    }

    fun startImport(resolver: ContentResolver, uri: Uri) = work {
        val text = withContext(Dispatchers.IO) {
            (resolver.openInputStream(uri) ?: throw IOException("Fichier inaccessible")).use { input ->
                val bytes = input.readNBytesCompat(MAX_IMPORT_BYTES + 1)
                if (bytes.size > MAX_IMPORT_BYTES) throw IOException("Fichier trop volumineux")
                String(bytes, Charsets.UTF_8)
            }
        }
        DiagnosticLog.i("sauvegarde", "import : fichier de ${text.length} caractères, chiffré : ${runCatching { ExportCodec.isEncrypted(text) }.getOrNull()}")
        _importStep.value = if (ExportCodec.isEncrypted(text)) {
            ImportStep.NeedPassword(text)
        } else {
            ImportStep.Confirm(ExportCodec.import(text, null))
        }
    }

    fun submitPassword(password: CharArray) {
        val step = _importStep.value as? ImportStep.NeedPassword ?: return
        work {
            try {
                val (config, extra) = withContext(Dispatchers.Default) { ExportCodec.importWithExtra(step.text, password) }
                val sharing = extra["sharing"]?.let { element ->
                    runCatching { ConfigCodec.json.decodeFromJsonElement(ExportedShareOwner.serializer(), element).also { it.toOwner() } }.getOrNull()
                }
                val history = extra["history"]?.let { HistoryData.decode(it.toString()) }?.takeIf { it.events.isNotEmpty() }
                _importStep.value = ImportStep.Confirm(config, sharing, history)
            } catch (e: ConfigException) {
                if (e.reason != ConfigException.Reason.WRONG_PASSWORD) throw e
                DiagnosticLog.i("sauvegarde", "import : mot de passe incorrect")
                _importStep.value = step.copy(wrongPassword = true)
            } finally {
                password.fill(' ')
            }
        }
    }

    fun confirmImport(replace: Boolean) {
        val step = _importStep.value as? ImportStep.Confirm ?: return
        _importStep.value = null
        work {
            DiagnosticLog.i(
                "sauvegarde",
                "import confirmé (${if (replace) "remplacement" else "ajout"}) : ${step.config.devices.size} PC, " +
                    "partage : ${step.sharing?.let { "${it.people.size} personne(s)" } ?: "non"}, historique : ${step.history?.events?.size ?: 0} évènement(s)",
            )
            if (replace) container.repository.replaceWith(step.config) else container.repository.mergeWith(step.config)
            // Historique de la sauvegarde ajouté à celui du téléphone (les PC absents sont écartés ensuite).
            step.history?.let { imported ->
                val ids = container.allDevices.first().devices.mapTo(HashSet()) { it.id }
                container.history.update { it.merge(imported, System.currentTimeMillis()).keep(ids) }
            }
            _messages.send(UiMessage(R.string.message_import_done, listOf(step.config.devices.size)))
            val sharing = step.sharing
            if (sharing != null && container.share.importOwner(sharing)) _sharingImported.value = true
        }
    }

    fun cancelImport() {
        _importStep.value = null
    }

    /** Efface l'historique noté par le téléphone ; le journal des agents sera relu. */
    fun clearHistory() = viewModelScope.launch {
        container.history.update { HistoryData.EMPTY }
        container.historyTracker.reset()
        _messages.send(UiMessage(R.string.message_history_cleared))
    }

    /** Exécute une opération longue en affichant l'indicateur d'activité et les erreurs. */
    private fun work(block: suspend () -> Unit) = viewModelScope.launch {
        _busy.value = true
        try {
            block()
        } catch (e: ConfigException) {
            DiagnosticLog.w("réglages", "opération refusée (${e.reason})", e)
            _importStep.value = null
            _messages.send(UiMessage(R.string.message_error, listOf(e.message.orEmpty())))
        } catch (e: IOException) {
            DiagnosticLog.w("réglages", "opération impossible", e)
            _messages.send(UiMessage(R.string.message_error, listOf(e.message.orEmpty())))
        } catch (e: IllegalArgumentException) {
            DiagnosticLog.w("réglages", "opération refusée", e)
            _messages.send(UiMessage(R.string.message_error, listOf(e.message.orEmpty())))
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            // Jamais de fermeture de l'application : l'erreur est notée et affichée.
            DiagnosticLog.e("réglages", "erreur imprévue", e)
            _messages.send(UiMessage(R.string.message_error, listOf(e.javaClass.simpleName + (e.message?.let { " : $it" } ?: ""))))
        } finally {
            _busy.value = false
        }
    }

    private fun java.io.InputStream.readNBytesCompat(limit: Int): ByteArray {
        val out = java.io.ByteArrayOutputStream()
        val buffer = ByteArray(8 * 1024)
        while (out.size() < limit) {
            val n = read(buffer, 0, minOf(buffer.size, limit - out.size()))
            if (n < 0) break
            out.write(buffer, 0, n)
        }
        return out.toByteArray()
    }

    private companion object {
        const val MAX_IMPORT_BYTES = 1024 * 1024
    }
}
