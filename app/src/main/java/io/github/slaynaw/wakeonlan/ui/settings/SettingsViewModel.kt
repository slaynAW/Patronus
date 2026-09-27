package io.github.slaynaw.wakeonlan.ui.settings

import android.content.ContentResolver
import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.github.slaynaw.wakeonlan.AppContainer
import io.github.slaynaw.wakeonlan.BuildConfig
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.config.ConfigException
import io.github.slaynaw.wakeonlan.core.config.ExportCodec
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.AppSettings
import io.github.slaynaw.wakeonlan.ui.devices.UiMessage
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
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
    data class Confirm(val config: AppConfig) : ImportStep {
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

    fun updateSettings(transform: (AppSettings) -> AppSettings) = viewModelScope.launch {
        container.repository.updateSettings(transform)
    }

    /** Écrit la sauvegarde ; [password] nul = export lisible sans aucun secret. */
    fun export(resolver: ContentResolver, uri: Uri, password: CharArray?) = work {
        val config = container.repository.current()
        val text = withContext(Dispatchers.Default) {
            ExportCodec.export(config, password, Instant.now().toString(), "WakeOnLan ${BuildConfig.VERSION_NAME}")
        }
        password?.fill(' ')
        withContext(Dispatchers.IO) {
            (resolver.openOutputStream(uri, "wt") ?: throw IOException("Fichier inaccessible")).use {
                it.write(text.toByteArray(Charsets.UTF_8))
            }
        }
        _messages.send(UiMessage(R.string.message_export_done, listOf(config.devices.size)))
    }

    fun startImport(resolver: ContentResolver, uri: Uri) = work {
        val text = withContext(Dispatchers.IO) {
            (resolver.openInputStream(uri) ?: throw IOException("Fichier inaccessible")).use { input ->
                val bytes = input.readNBytesCompat(MAX_IMPORT_BYTES + 1)
                if (bytes.size > MAX_IMPORT_BYTES) throw IOException("Fichier trop volumineux")
                String(bytes, Charsets.UTF_8)
            }
        }
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
                val config = withContext(Dispatchers.Default) { ExportCodec.import(step.text, password) }
                _importStep.value = ImportStep.Confirm(config)
            } catch (e: ConfigException) {
                if (e.reason != ConfigException.Reason.WRONG_PASSWORD) throw e
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
            if (replace) container.repository.replaceWith(step.config) else container.repository.mergeWith(step.config)
            _messages.send(UiMessage(R.string.message_import_done, listOf(step.config.devices.size)))
        }
    }

    fun cancelImport() {
        _importStep.value = null
    }

    /** Exécute une opération longue en affichant l'indicateur d'activité et les erreurs. */
    private fun work(block: suspend () -> Unit) = viewModelScope.launch {
        _busy.value = true
        try {
            block()
        } catch (e: ConfigException) {
            _importStep.value = null
            _messages.send(UiMessage(R.string.message_error, listOf(e.message.orEmpty())))
        } catch (e: IOException) {
            _messages.send(UiMessage(R.string.message_error, listOf(e.message.orEmpty())))
        } catch (e: IllegalArgumentException) {
            _messages.send(UiMessage(R.string.message_error, listOf(e.message.orEmpty())))
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
