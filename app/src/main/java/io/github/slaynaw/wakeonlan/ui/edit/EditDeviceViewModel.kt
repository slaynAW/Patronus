package io.github.slaynaw.wakeonlan.ui.edit

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.github.slaynaw.wakeonlan.AppContainer
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.PairingLink
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.DEFAULT_AGENT_PORT
import io.github.slaynaw.wakeonlan.core.model.DEFAULT_PROBE_PORTS
import io.github.slaynaw.wakeonlan.core.model.DEFAULT_WOL_PORT
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.DeviceField
import io.github.slaynaw.wakeonlan.core.model.DeviceValidator
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.util.UUID

/** Contenu brut du formulaire (tel que saisi). */
data class DeviceForm(
    val name: String = "",
    val mac: String = "",
    val host: String = "",
    val agentEnabled: Boolean = false,
    val agentPort: String = DEFAULT_AGENT_PORT.toString(),
    val agentKey: String = "",
    val broadcast: String = "",
    val wolPort: String = DEFAULT_WOL_PORT.toString(),
    val probePorts: String = DEFAULT_PROBE_PORTS.joinToString(", "),
    val secureOn: String = "",
) {
    companion object {
        fun from(device: Device) = DeviceForm(
            name = device.name,
            mac = device.mac.toString(),
            host = device.host,
            agentEnabled = device.agent != null,
            agentPort = (device.agent?.port ?: DEFAULT_AGENT_PORT).toString(),
            agentKey = device.agent?.key.orEmpty(),
            broadcast = device.broadcastAddress.orEmpty(),
            wolPort = device.wolPort.toString(),
            probePorts = device.probePorts.joinToString(", "),
            secureOn = device.secureOnPassword.orEmpty(),
        )
    }
}

sealed interface AgentTestResult {
    data class Success(val status: AgentStatus) : AgentTestResult
    data class Failure(val error: AgentError, val detail: String?) : AgentTestResult
    data class Invalid(val message: String) : AgentTestResult
}

data class EditUiState(
    val loading: Boolean = true,
    val isNew: Boolean = true,
    val form: DeviceForm = DeviceForm(),
    val errors: Map<DeviceField, String> = emptyMap(),
    val testing: Boolean = false,
    val testResult: AgentTestResult? = null,
    /** Erreur de lecture d'un lien / QR code d'appairage. */
    val pairingError: String? = null,
    val pairingApplied: Boolean = false,
    val done: Boolean = false,
)

class EditDeviceViewModel(
    private val container: AppContainer,
    private val deviceId: String?,
) : ViewModel() {

    private val _state = MutableStateFlow(EditUiState())
    val state: StateFlow<EditUiState> = _state.asStateFlow()

    init {
        viewModelScope.launch {
            val existing = deviceId?.let { container.repository.current().device(it) }
            _state.value = EditUiState(
                loading = false,
                isNew = existing == null,
                form = existing?.let(DeviceForm::from) ?: DeviceForm(),
            )
        }
    }

    fun onFormChange(form: DeviceForm) = _state.update { it.copy(form = form, errors = emptyMap(), testResult = null) }

    /** Remplit le formulaire à partir d'un lien `wolagent://` (QR code scanné ou lien collé). */
    fun applyPairing(text: String) {
        val info = try {
            PairingLink.parse(text)
        } catch (e: IllegalArgumentException) {
            _state.update { it.copy(pairingError = e.message, pairingApplied = false) }
            return
        }
        _state.update { s ->
            val f = s.form
            s.copy(
                form = f.copy(
                    name = f.name.ifBlank { info.name },
                    mac = info.mac?.toString() ?: f.mac,
                    host = info.host,
                    agentEnabled = true,
                    agentPort = info.port.toString(),
                    agentKey = info.key,
                ),
                errors = emptyMap(),
                pairingError = null,
                pairingApplied = true,
                testResult = null,
            )
        }
    }

    fun dismissPairingFeedback() = _state.update { it.copy(pairingError = null, pairingApplied = false) }

    fun save() {
        val device = buildDevice() ?: return
        viewModelScope.launch {
            container.repository.upsert(device)
            _state.update { it.copy(done = true) }
        }
    }

    fun delete() {
        val id = deviceId ?: return
        viewModelScope.launch {
            container.repository.delete(id)
            _state.update { it.copy(done = true) }
        }
    }

    fun testAgent() {
        val form = _state.value.form
        val port = form.agentPort.trim().toIntOrNull()
        val host = form.host.trim()
        if (!DeviceValidator.isValidHost(host) || port == null || !DeviceValidator.isValidPort(port)) {
            _state.update { it.copy(testResult = AgentTestResult.Invalid("Renseignez une adresse et un port valides")) }
            return
        }
        _state.update { it.copy(testing = true, testResult = null) }
        viewModelScope.launch {
            val result = container.actions.testAgent(host, AgentSettings(port, form.agentKey.trim()))
            _state.update {
                it.copy(
                    testing = false,
                    testResult = when (result) {
                        is AgentResult.Success -> AgentTestResult.Success(result.value)
                        is AgentResult.Failure -> AgentTestResult.Failure(result.error, result.detail)
                    },
                )
            }
        }
    }

    /** Convertit le formulaire en [Device], ou affiche les erreurs champ par champ. */
    private fun buildDevice(): Device? {
        val f = _state.value.form
        val errors = linkedMapOf<DeviceField, String>()

        val mac = MacAddress.parseOrNull(f.mac)
        if (mac == null) errors[DeviceField.MAC] = "Adresse MAC invalide (ex. AA:BB:CC:DD:EE:FF)"
        val wolPort = f.wolPort.trim().toIntOrNull()
        if (wolPort == null) errors[DeviceField.WOL_PORT] = "Nombre attendu"
        val probePorts = f.probePorts.split(',', ';', ' ').filter { it.isNotBlank() }.map { it.trim().toIntOrNull() }
        if (probePorts.any { it == null }) errors[DeviceField.PROBE_PORTS] = "Liste de ports séparés par des virgules"
        val agentPort = f.agentPort.trim().toIntOrNull()
        if (f.agentEnabled && agentPort == null) errors[DeviceField.AGENT_PORT] = "Nombre attendu"

        if (mac != null && errors.isEmpty()) {
            val device = Device(
                id = deviceId ?: UUID.randomUUID().toString(),
                name = f.name.trim(),
                mac = mac,
                host = f.host.trim(),
                broadcastAddress = f.broadcast.trim().ifEmpty { null },
                wolPort = wolPort ?: DEFAULT_WOL_PORT,
                secureOnPassword = f.secureOn.trim().ifEmpty { null },
                probePorts = probePorts.filterNotNull().distinct(),
                agent = if (f.agentEnabled) AgentSettings(agentPort ?: DEFAULT_AGENT_PORT, f.agentKey.trim()) else null,
            )
            DeviceValidator.validate(device).forEach { errors.putIfAbsent(it.field, it.message) }
            if (errors.isEmpty()) return device
        }
        _state.update { it.copy(errors = errors) }
        return null
    }
}
