package io.github.slaynaw.wakeonlan.ui.hub

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.github.slaynaw.wakeonlan.AdminChange
import io.github.slaynaw.wakeonlan.AppContainer
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent2.Commands
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.agent2.Role
import io.github.slaynaw.wakeonlan.core.hub.HubAdmin
import io.github.slaynaw.wakeonlan.core.hub.HubCreatedInvite
import io.github.slaynaw.wakeonlan.core.hub.HubLinkResult
import io.github.slaynaw.wakeonlan.core.hub.HubProbe
import io.github.slaynaw.wakeonlan.core.hub.HubRequest
import io.github.slaynaw.wakeonlan.core.hub.HubService
import io.github.slaynaw.wakeonlan.core.hub.HubState
import io.github.slaynaw.wakeonlan.core.hub.PairOutcome
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Résultat à montrer : dialogue (échec, retrait incomplet) ou message bref. */
sealed interface HubNotice {
    data class Paired(val name: String, val role: String, val already: Boolean) : HubNotice

    /** Lien refusé avant tout échange (lien de PC, expiré ou invalide). */
    data class BadLink(val reason: HubLinkResult) : HubNotice

    data object OtherHub : HubNotice

    data class PairFailed(val failure: AgentResult.Failure) : HubNotice

    /** Modification refusée par le hub ou impossible. */
    data class Failed(val failure: AgentResult.Failure) : HubNotice

    /** Hub oublié ; [revoked] : ce téléphone s'en est aussi retiré. */
    data class Forgotten(val revoked: Boolean) : HubNotice

    /** PC retiré du hub ; [kept] : le hub n'a pas pu être retiré de ses appareils autorisés ([hubPin] : empreinte courte). */
    data class Removed(val name: String, val kept: Boolean, val hubPin: String) : HubNotice

    data class DeviceAdded(val name: String) : HubNotice

    data class Retention(val days: Int) : HubNotice
}

data class HubUiState(
    val busy: Boolean = false,
    /** Appareils autorisés, invitations et conservation (rôle « admin ») ; null : pas encore lus. */
    val admin: HubAdmin? = null,
    /** Lecture de l'administration impossible. */
    val adminFailure: AgentResult.Failure? = null,
    val created: HubCreatedInvite? = null,
    val notice: HubNotice? = null,
)

/**
 * Écran « Hub sur le NAS » (docs/HUB.md, §12) : appairage, état du hub, PC et appareils confiés, administration
 * (appareils autorisés, invitations, conservation de l'historique) et oubli. Le hub vérifie de toute façon chaque
 * demande selon le rôle de ce téléphone.
 */
class HubViewModel(private val container: AppContainer) : ViewModel() {
    private val manager = container.hub
    private val _state = MutableStateFlow(HubUiState())
    val state: StateFlow<HubUiState> = _state.asStateFlow()
    val hub: StateFlow<HubState> = manager.state

    init {
        // Administration relue quand ce téléphone devient administrateur d'un hub ; oubliée sinon.
        viewModelScope.launch {
            manager.state.map { s -> s.hub?.takeIf { it.isAdmin }?.pin }.distinctUntilChanged().collect { pin ->
                if (pin == null) _state.update { it.copy(admin = null, adminFailure = null) } else loadAdmin()
            }
        }
    }

    private suspend fun loadAdmin() {
        if (manager.state.value.hub?.isAdmin != true) return
        when (val r = manager.admin()) {
            is AgentResult.Success -> _state.update { it.copy(admin = r.value, adminFailure = null) }
            is AgentResult.Failure -> _state.update { it.copy(adminFailure = r) }
        }
    }

    /** Relit l'état du hub et, pour un administrateur, ses appareils autorisés. */
    fun refresh() {
        viewModelScope.launch {
            manager.refresh()
            loadAdmin()
        }
    }

    /** Une action à la fois ; [run] rend le résultat à montrer (null : rien). */
    private fun act(run: suspend () -> HubNotice?) {
        if (_state.value.busy) return
        _state.update { it.copy(busy = true) }
        viewModelScope.launch {
            val notice = try {
                run()
            } finally {
                _state.update { it.copy(busy = false) }
            }
            if (notice != null) _state.update { it.copy(notice = notice) }
        }
    }

    /** Appaire ce téléphone au hub d'un lien scanné ou collé. */
    fun pair(text: String) = act {
        when (val r = manager.pair(text)) {
            is PairOutcome.Paired -> HubNotice.Paired(r.hub.name, r.hub.role, r.already)
            is PairOutcome.BadLink -> HubNotice.BadLink(r.reason)
            PairOutcome.OtherHub -> HubNotice.OtherHub
            is PairOutcome.Failed -> HubNotice.PairFailed(r.failure)
        }
    }

    fun forget() = act { HubNotice.Forgotten(manager.forget()) }

    /** Rôle, nom ou retrait d'un appareil autorisé, ou annulation d'une invitation. */
    fun change(change: AdminChange) = act {
        val request = when (change) {
            is AdminChange.Revoke -> HubRequest(cmd = Commands.DEVICES_REVOKE, device = change.device)
            is AdminChange.SetRole -> HubRequest(cmd = Commands.DEVICES_ROLE, device = change.device, role = change.role)
            is AdminChange.Rename -> HubRequest(cmd = Commands.DEVICES_RENAME, device = change.device, name = change.name)
            is AdminChange.CancelInvite -> HubRequest(cmd = Commands.INVITES_CANCEL, invite = change.id)
        }
        val r = manager.change(request)
        loadAdmin()
        (r as? AgentResult.Failure)?.let { HubNotice.Failed(it) }
    }

    /** Invitation d'un autre appareil sur le hub ([role] : view, wake ou admin). */
    fun invite(role: String, ttlSeconds: Long, name: String) = act {
        val parsed = Role.parse(role) ?: return@act HubNotice.Failed(AgentResult.Failure(AgentError.REJECTED, code = ResponseCode.BAD_REQUEST))
        val r = manager.invite(parsed, ttlSeconds, name.trim())
        loadAdmin()
        when (r) {
            is AgentResult.Success -> {
                _state.update { it.copy(created = r.value) }
                null
            }
            is AgentResult.Failure -> HubNotice.Failed(r)
        }
    }

    fun retention(days: Int) = act {
        when (val r = manager.retention(days)) {
            is AgentResult.Success -> {
                _state.update { s -> s.copy(admin = s.admin?.copy(retention = r.value)) }
                HubNotice.Retention(r.value)
            }
            is AgentResult.Failure -> {
                loadAdmin()
                HubNotice.Failed(r)
            }
        }
    }

    /** Retire un PC ou un appareil du hub (et, pour un PC de ce téléphone, le hub de ses appareils autorisés). */
    fun remove(pcId: String) = act {
        val devices = container.repository.current().devices
        when (val r = manager.remove(pcId, devices)) {
            is AgentResult.Success -> HubNotice.Removed(r.value.pc.name, kept = r.value.agentRevoked == false, hubPin = manager.state.value.hub?.shortPin.orEmpty())
            is AgentResult.Failure -> HubNotice.Failed(r)
        }
    }

    /** Ajoute un appareil sans agent, déjà vérifié par [HubService.validateDevice]. */
    fun addDevice(name: String, host: String, mac: String, probe: HubProbe) = act {
        if (HubService.validateDevice(name, host, mac, probe) != null) {
            return@act HubNotice.Failed(AgentResult.Failure(AgentError.REJECTED, code = ResponseCode.BAD_REQUEST))
        }
        when (val r = manager.addDevice(name, host, mac, probe)) {
            is AgentResult.Success -> HubNotice.DeviceAdded(r.value.name)
            is AgentResult.Failure -> HubNotice.Failed(r)
        }
    }

    fun dismissNotice() = _state.update { it.copy(notice = null) }

    fun dismissCreated() = _state.update { it.copy(created = null) }

    companion object {
        /** Rôles d'un appareil sur le hub (§4). */
        val ROLES: List<String> = listOf(Role.VIEW.wire, Role.WAKE.wire, Role.ADMIN.wire)

        /** Durées de conservation proposées, en jours. */
        val RETENTIONS: List<Int> = listOf(90, 182, 365, 730, 1095)
    }
}
