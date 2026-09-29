package io.github.slaynaw.wakeonlan

import android.content.Context
import android.os.Build
import android.provider.Settings
import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.status.HostProber
import io.github.slaynaw.wakeonlan.core.status.ProbeAvailability
import io.github.slaynaw.wakeonlan.core.status.StatusMonitor
import io.github.slaynaw.wakeonlan.core.status.UnknownReason
import io.github.slaynaw.wakeonlan.core.wol.WakeOnLanSender
import io.github.slaynaw.wakeonlan.data.ConfigRepository
import io.github.slaynaw.wakeonlan.data.HistoryRepository
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import io.github.slaynaw.wakeonlan.network.LanNetworkMonitor
import io.github.slaynaw.wakeonlan.network.LocalNetworkAccess
import io.github.slaynaw.wakeonlan.share.ShareManager
import io.github.slaynaw.wakeonlan.update.AppUpdater
import kotlinx.coroutines.CoroutineExceptionHandler
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
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
    /**
     * Tâches de fond de l'application (enregistrement de l'historique, lecture du journal des agents,
     * publication du partage). Une erreur imprévue y est notée au journal de diagnostic au lieu de
     * fermer l'application.
     */
    private val scope = CoroutineScope(
        SupervisorJob() + Dispatchers.Default + CoroutineExceptionHandler { _, e ->
            DiagnosticLog.e("tâche", "erreur imprévue dans une tâche de fond", e)
        },
    )

    val network = LanNetworkMonitor(context)
    val repository = ConfigRepository(context)

    /** Partage des PC entre personnes (PC reçus, accès accordés). */
    val share = ShareManager(context, scope, repository)

    /** PC du téléphone puis PC reçus d'autres personnes : ce que surveille et affiche l'application. */
    val allDevices: Flow<AppConfig> = combine(repository.config, share.sharedDevices) { config, shared ->
        if (shared.isEmpty()) config else config.copy(devices = config.devices + shared.map { it.device })
    }

    /** Le nom du téléphone accompagne chaque demande : les agents le notent dans l'historique commun. */
    private val agentClient = AgentClient(binder = network, deviceName = ::deviceName)
    val statusMonitor = StatusMonitor(prober = HostProber(binder = network, agentClient = agentClient))
    val history = HistoryRepository(context, scope)
    val historyTracker = HistoryTracker(allDevices, statusMonitor.statuses, agentClient, history, scope)
    val actions = DeviceActions(network, WakeOnLanSender(binder = network), agentClient, statusMonitor, historyTracker)

    /** Mises à jour intégrées (versions officielles publiées sur GitHub). */
    val updater = AppUpdater(context, scope)

    private val _localNetworkGranted = MutableStateFlow(LocalNetworkAccess.isGranted(context))

    /** Autorisation Android 17 « réseau local » accordée (toujours vrai avant Android 17). */
    val localNetworkGranted: StateFlow<Boolean> = _localNetworkGranted.asStateFlow()

    /** Nom de ce téléphone (réglages Android « Nom de l'appareil »), à défaut son modèle. */
    private fun deviceName(): String =
        Settings.Global.getString(context.contentResolver, Settings.Global.DEVICE_NAME)?.takeIf { it.isNotBlank() } ?: Build.MODEL

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
