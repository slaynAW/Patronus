package io.github.slaynaw.wakeonlan

import android.content.Context
import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.status.HostProber
import io.github.slaynaw.wakeonlan.core.status.ProbeAvailability
import io.github.slaynaw.wakeonlan.core.status.StatusMonitor
import io.github.slaynaw.wakeonlan.core.status.UnknownReason
import io.github.slaynaw.wakeonlan.core.wol.WakeOnLanSender
import io.github.slaynaw.wakeonlan.data.ConfigRepository
import io.github.slaynaw.wakeonlan.network.LanNetworkMonitor
import io.github.slaynaw.wakeonlan.network.LocalNetworkAccess
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine

/**
 * Assemble les composants de l'application (injection de dépendances manuelle, volontairement
 * simple). Une seule instance, créée par [WolApplication].
 */
class AppContainer(private val context: Context) {
    val network = LanNetworkMonitor(context)
    val repository = ConfigRepository(context)
    private val agentClient = AgentClient(binder = network)
    val statusMonitor = StatusMonitor(prober = HostProber(binder = network, agentClient = agentClient))
    val actions = DeviceActions(network, WakeOnLanSender(binder = network), agentClient, statusMonitor)

    private val _localNetworkGranted = MutableStateFlow(LocalNetworkAccess.isGranted(context))

    /** Autorisation Android 17 « réseau local » accordée (toujours vrai avant Android 17). */
    val localNetworkGranted: StateFlow<Boolean> = _localNetworkGranted.asStateFlow()

    fun refreshPermissions() {
        _localNetworkGranted.value = LocalNetworkAccess.isGranted(context)
    }

    /** Peut-on vérifier l'état des PC en ce moment ? Sinon on affiche « inconnu » (jamais un faux « éteint »). */
    val probeAvailability: Flow<ProbeAvailability> =
        combine(network.state, localNetworkGranted) { lan, granted ->
            when {
                !granted -> ProbeAvailability(false, UnknownReason.NO_PERMISSION)
                lan.connected || lan.vpnActive -> ProbeAvailability.AVAILABLE
                else -> ProbeAvailability(false, UnknownReason.NO_NETWORK)
            }
        }
}
