package io.github.slaynaw.wakeonlan.ui.edit

import android.os.Build
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.github.slaynaw.wakeonlan.AppContainer
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.PairingLink
import io.github.slaynaw.wakeonlan.core.agent.ParsedPairing
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.DEFAULT_AGENT_PORT
import io.github.slaynaw.wakeonlan.core.model.DEFAULT_PROBE_PORTS
import io.github.slaynaw.wakeonlan.core.model.DEFAULT_WOL_PORT
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.DeviceField
import io.github.slaynaw.wakeonlan.core.model.DeviceValidator
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
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

/**
 * Appairage wolagent/2 en attente, lu dans un lien `v=2` : il se fait à l'enregistrement. [agent] : empreinte complète
 * de l'agent, [code] : code d'appairage normalisé, [role] et [expires] (secondes Unix, 0 si absente) : annoncés par le
 * lien, pour l'affichage.
 */
data class PendingPairing(val agent: String, val agentShort: String, val code: String, val role: String?, val expires: Long) {
    override fun toString(): String = "PendingPairing(agent=$agentShort, code=***, role=$role, expires=$expires)"
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
    /** Empreinte courte de l'agent d'un PC passé en wolagent/2 (la clé n'est alors plus utile). */
    val agentPin: String? = null,
    /** Appairage wolagent/2 à faire à l'enregistrement (lien `v=2`). */
    val pairing: PendingPairing? = null,
    /** Enregistrement en cours (appairage compris). */
    val saving: Boolean = false,
    /** Échec de l'appairage à l'enregistrement : la fiche n'est pas enregistrée. */
    val pairingFailure: AgentError? = null,
)

class EditDeviceViewModel(
    private val container: AppContainer,
    private val deviceId: String?,
) : ViewModel() {

    private val _state = MutableStateFlow(EditUiState())
    val state: StateFlow<EditUiState> = _state.asStateFlow()

    /** Réglages de l'agent enregistrés : l'empreinte wolagent/2 n'est pas dans le formulaire. */
    private var savedAgent: AgentSettings? = null

    init {
        viewModelScope.launch {
            val existing = deviceId?.let { container.repository.current().device(it) }
            savedAgent = existing?.agent
            _state.value = EditUiState(
                loading = false,
                isNew = existing == null,
                form = existing?.let(DeviceForm::from) ?: DeviceForm(),
                agentPin = existing?.agent?.shortPin,
            )
        }
    }

    /** Réglages de l'agent pour la clé saisie, avec l'empreinte connue si elle reste valable. */
    private fun agentSettings(port: Int, key: String) = AgentSettings(port, key, pin = savedAgent?.pinKeptWith(key).orEmpty())

    fun onFormChange(form: DeviceForm) = _state.update {
        // Agent désactivé : l'appairage en attente n'a plus lieu d'être.
        it.copy(form = form, errors = emptyMap(), testResult = null, pairing = it.pairing.takeIf { form.agentEnabled })
    }

    /**
     * Remplit le formulaire à partir d'un lien `wolagent://` (QR code scanné ou lien collé) : `v=1` avec la clé de
     * l'agent, `v=2` avec son empreinte et un code à usage unique, envoyé à l'enregistrement.
     */
    fun applyPairing(text: String) {
        val parsed = try {
            PairingLink.parseAny(text)
        } catch (e: IllegalArgumentException) {
            _state.update { it.copy(pairingError = e.message, pairingApplied = false) }
            return
        }
        if (parsed is ParsedPairing.V2 && Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) {
            _state.update { it.copy(pairingError = ANDROID_9_V2, pairingApplied = false) }
            return
        }
        _state.update { s ->
            val f = s.form
            val next = when (parsed) {
                is ParsedPairing.V1 -> {
                    val info = parsed.info
                    s.copy(
                        form = f.copy(
                            name = f.name.ifBlank { info.name },
                            mac = info.mac?.toString() ?: f.mac,
                            host = info.host,
                            agentEnabled = true,
                            agentPort = info.port.toString(),
                            agentKey = info.key,
                        ),
                        pairing = null,
                    )
                }
                is ParsedPairing.V2 -> {
                    val link = parsed.link
                    s.copy(
                        form = f.copy(
                            name = f.name.ifBlank { link.name },
                            mac = link.mac.ifEmpty { f.mac },
                            host = link.host,
                            agentEnabled = true,
                            agentPort = link.port.toString(),
                            agentKey = "",
                        ),
                        pairing = PendingPairing(link.agent.toString(), link.agent.short(), link.code, link.role?.wire, link.expires),
                    )
                }
            }
            next.copy(errors = emptyMap(), pairingError = null, pairingApplied = true, testResult = null, pairingFailure = null)
        }
    }

    fun dismissPairingFeedback() = _state.update { it.copy(pairingError = null, pairingApplied = false) }

    /** Annule l'appairage en attente ; la clé enregistrée revient dans le formulaire. */
    fun cancelPairing() = _state.update {
        it.copy(pairing = null, form = it.form.copy(agentKey = it.form.agentKey.ifEmpty { savedAgent?.key.orEmpty() }))
    }

    fun dismissPairingFailure() = _state.update { it.copy(pairingFailure = null) }

    fun save() {
        if (_state.value.saving) return
        val device = buildDevice() ?: return
        val pairing = _state.value.pairing
        val agent = device.agent
        if (pairing == null || agent == null) {
            viewModelScope.launch {
                container.repository.upsert(device)
                _state.update { it.copy(done = true) }
            }
            return
        }
        _state.update { it.copy(saving = true, pairingFailure = null) }
        viewModelScope.launch {
            // Lien v=2 : ce téléphone s'appaire d'abord ; la fiche n'est enregistrée qu'en cas de succès, avec
            // l'empreinte de l'agent et sans clé (gardée seulement pour un partage « démarrer et éteindre », §10).
            val settings = AgentSettings(agent.port, key = "", pin = pairing.agent)
            when (val r = container.actions.pair(device.host, settings, pairing.code, device.id)) {
                is AgentResult.Success -> {
                    val kept = savedAgent?.key?.takeIf { it.isNotEmpty() && container.share.sharedWithControl(device.id) }
                    container.repository.upsert(device.copy(agent = settings.copy(key = kept.orEmpty())))
                    DiagnosticLog.i("agent", "« ${device.name} » : appairé en wolagent/2 (rôle ${r.value.role})")
                    _state.update { it.copy(saving = false, done = true) }
                }
                is AgentResult.Failure -> _state.update { it.copy(saving = false, pairingFailure = r.error) }
            }
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
            // Comme à l'enregistrement : un PC passé en wolagent/2 est testé en wolagent/2.
            val result = container.actions.testAgent(host, agentSettings(port, form.agentKey.trim()), deviceId)
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

    private companion object {
        const val ANDROID_9_V2 = "Ce QR code utilise la connexion chiffrée, que ce téléphone (Android 8 ou 9) ne permet pas. " +
            "Sur le PC, lancez « wol-agent pair --v1 » pour afficher l'ancien QR code."
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
                agent = if (f.agentEnabled) agentSettings(agentPort ?: DEFAULT_AGENT_PORT, f.agentKey.trim()) else null,
            )
            DeviceValidator.validate(device).forEach { errors.putIfAbsent(it.field, it.message) }
            if (errors.isEmpty()) return device
        }
        _state.update { it.copy(errors = errors) }
        return null
    }
}
