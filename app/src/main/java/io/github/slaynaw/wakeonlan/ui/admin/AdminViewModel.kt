package io.github.slaynaw.wakeonlan.ui.admin

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.github.slaynaw.wakeonlan.AdminChange
import io.github.slaynaw.wakeonlan.AppContainer
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent2.DeviceInfo
import io.github.slaynaw.wakeonlan.core.agent2.InviteInfo
import io.github.slaynaw.wakeonlan.core.agent2.PairingCode
import io.github.slaynaw.wakeonlan.core.agent2.PairingLinkV2
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.agent2.Role
import io.github.slaynaw.wakeonlan.core.model.Device
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Invitation qui vient d'être créée : lien `v=2` (QR code) et code à transmettre à l'autre appareil. */
data class CreatedInvite(val link: String, val code: String, val role: String, val expires: Long) {
    override fun toString(): String = "CreatedInvite(role=$role, expires=$expires)"
}

/** Refus de l'agent : cause, et code de l'agent (« last_admin », « insufficient_role »…). */
data class AdminFailure(val error: AgentError, val code: String?)

data class AdminUiState(
    val loading: Boolean = true,
    val deviceName: String = "",
    val devices: List<DeviceInfo> = emptyList(),
    val invites: List<InviteInfo> = emptyList(),
    /** Lecture impossible (PC injoignable, rôle retiré…). */
    val error: AdminFailure? = null,
    val busy: Boolean = false,
    val created: CreatedInvite? = null,
    /** Modification refusée, affichée dans un dialogue. */
    val failure: AdminFailure? = null,
)

/**
 * Appareils autorisés par un PC (docs/PROTOCOLE-V2.md, §6.2 et §7.5) : rôles, noms, retraits, invitations. Réservé aux
 * appareils de rôle « admin » ; l'agent vérifie de toute façon chaque demande.
 */
class AdminViewModel(private val container: AppContainer, private val deviceId: String) : ViewModel() {
    private val _state = MutableStateFlow(AdminUiState())
    val state: StateFlow<AdminUiState> = _state.asStateFlow()

    init {
        load()
    }

    private suspend fun device(): Device? = container.repository.current().device(deviceId)

    fun load() {
        viewModelScope.launch {
            val device = device()
            if (device == null) {
                _state.update { it.copy(loading = false, error = AdminFailure(AgentError.NO_KEY, null)) }
                return@launch
            }
            _state.update { it.copy(loading = true, deviceName = device.name) }
            when (val r = container.actions.agentAdmin(device)) {
                is AgentResult.Success -> _state.update {
                    it.copy(loading = false, error = null, devices = r.value.devices, invites = r.value.invites)
                }
                is AgentResult.Failure -> _state.update { it.copy(loading = false, error = AdminFailure(r.error, r.code)) }
            }
        }
    }

    fun change(change: AdminChange) {
        if (_state.value.busy) return
        viewModelScope.launch {
            val device = device() ?: return@launch
            _state.update { it.copy(busy = true) }
            val r = container.actions.agentAdminChange(device, change)
            _state.update { it.copy(busy = false, failure = (r as? AgentResult.Failure)?.let { f -> AdminFailure(f.error, f.code) }) }
            load()
        }
    }

    /** Crée une invitation ([role], [ttlSeconds], [name] facultatif) et prépare son lien `v=2` pour le QR code. */
    fun invite(role: String, ttlSeconds: Long, name: String) {
        if (_state.value.busy) return
        viewModelScope.launch {
            val device = device() ?: return@launch
            val agent = device.agent ?: return@launch
            val pin = Pin.parse(agent.pin) ?: return@launch
            _state.update { it.copy(busy = true) }
            when (val r = container.actions.agentInvite(device, role, ttlSeconds, name.trim())) {
                is AgentResult.Success -> {
                    val invite = r.value
                    val code = PairingCode.normalize(invite.code)
                    if (code == null || invite.agent != pin.toString()) {
                        _state.update { it.copy(busy = false, failure = AdminFailure(AgentError.PROTOCOL, null)) }
                        return@launch
                    }
                    // Adresse MAC pour le réveil, sauf valeur qu'un lien refuse (§7.1).
                    val mac = device.mac.toString().takeUnless { it == "00:00:00:00:00:00" || it == "FF:FF:FF:FF:FF:FF" }.orEmpty()
                    val link = PairingLinkV2(device.name, device.host, agent.port, mac, pin, code, Role.parse(invite.role), invite.expires).text()
                    _state.update { it.copy(busy = false, created = CreatedInvite(link, PairingCode.format(code), invite.role, invite.expires)) }
                }
                is AgentResult.Failure -> _state.update { it.copy(busy = false, failure = AdminFailure(r.error, r.code)) }
            }
            load()
        }
    }

    fun dismissCreated() = _state.update { it.copy(created = null) }

    fun dismissFailure() = _state.update { it.copy(failure = null) }
}
