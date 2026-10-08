package io.github.slaynaw.wakeonlan.ui.devices

import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.github.slaynaw.wakeonlan.AppContainer
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.model.AppSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.core.status.LatencySample
import io.github.slaynaw.wakeonlan.core.status.ProbeAvailability
import io.github.slaynaw.wakeonlan.core.status.TempSample
import io.github.slaynaw.wakeonlan.data.StoredSpecs
import io.github.slaynaw.wakeonlan.network.LanState
import io.github.slaynaw.wakeonlan.ui.common.label
import io.github.slaynaw.wakeonlan.ui.common.sentMessage
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/**
 * Un PC, son état et ses mesures de latence récentes (tracé en direct). [sharedBy] : nom de la
 * personne qui le partage (PC reçu, non modifiable).
 */
data class DeviceItem(
    val device: Device,
    val status: DeviceStatus,
    val latency: List<LatencySample> = emptyList(),
    val sharedBy: String? = null,
    /** Températures des 5 dernières minutes (agent) et fiche du PC (agent 1.10.0), si connues. */
    val temps: List<TempSample> = emptyList(),
    val specs: StoredSpecs? = null,
) {
    val editable: Boolean get() = sharedBy == null
}

data class DevicesUiState(
    val loaded: Boolean = false,
    val items: List<DeviceItem> = emptyList(),
    val settings: AppSettings = AppSettings(),
    val lan: LanState = LanState(),
    val availability: ProbeAvailability = ProbeAvailability.AVAILABLE,
    val localNetworkGranted: Boolean = true,
)

/** Message ponctuel à afficher (snackbar) : ressource + arguments. */
data class UiMessage(@StringRes val text: Int, val args: List<Any> = emptyList())

/** Argument de message qui est lui-même une ressource texte (résolue à l'affichage). */
data class ResArg(@StringRes val id: Int)

class DevicesViewModel(private val container: AppContainer) : ViewModel() {

    private val _messages = Channel<UiMessage>(Channel.BUFFERED)
    val messages: Flow<UiMessage> = _messages.receiveAsFlow()

    val state: StateFlow<DevicesUiState> = combine(
        combine(container.repository.config, container.share.sharedDevices, container.specs.data) { c, shared, specs -> Triple(c, shared, specs) },
        combine(container.statusMonitor.statuses, container.statusMonitor.latency, container.statusMonitor.temps) { s, l, t -> Triple(s, l, t) },
        container.network.state,
        container.probeAvailability,
        container.localNetworkGranted,
    ) { (config, shared, specs), (statuses, latency, temps), lan, availability, granted ->
        fun item(d: Device, sharedBy: String? = null) =
            DeviceItem(d, statuses[d.id] ?: DeviceStatus(), latency[d.id].orEmpty(), sharedBy, temps[d.id].orEmpty(), specs[d.id])
        DevicesUiState(
            loaded = true,
            items = config.devices.map { item(it) } + shared.map { item(it.device, it.ownerName) },
            settings = config.settings,
            lan = lan,
            availability = availability,
            localNetworkGranted = granted,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), DevicesUiState())

    fun wake(device: Device) = viewModelScope.launch {
        val result = runCatching { container.actions.wake(device) }
        val wake = result.getOrNull()
        _messages.send(
            when {
                wake == null -> UiMessage(R.string.message_wake_error, listOf(result.exceptionOrNull()?.message.orEmpty()))
                wake.success -> UiMessage(R.string.message_wake_sent, listOf(device.name))
                else -> UiMessage(R.string.message_wake_error, listOf(wake.errors.firstOrNull().orEmpty()))
            },
        )
    }

    fun power(device: Device, action: PowerAction, force: Boolean) = viewModelScope.launch {
        when (val result = container.actions.power(device, action, force)) {
            is AgentResult.Success -> _messages.send(UiMessage(action.sentMessage(), listOf(device.name)))
            is AgentResult.Failure -> _messages.send(
                UiMessage(R.string.message_agent_error, listOf(device.name, ResArg(result.error.label()))),
            )
        }
    }

    fun refresh() = container.statusMonitor.refresh()

    fun clearNotice(device: Device) = container.statusMonitor.clearNotice(device.id)

    fun move(device: Device, offset: Int) = viewModelScope.launch { container.repository.move(device.id, offset) }

    fun delete(device: Device) = viewModelScope.launch {
        container.repository.delete(device.id)
        _messages.send(UiMessage(R.string.message_deleted, listOf(device.name)))
    }

    fun onPermissionResult() = container.refreshPermissions()
}
