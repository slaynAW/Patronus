package io.github.slaynaw.wakeonlan.core.status

import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.AppSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import java.util.concurrent.ConcurrentHashMap

/** Capacité du téléphone à vérifier l'état des terminaux à un instant donné. */
data class ProbeAvailability(val canProbe: Boolean, val reason: UnknownReason? = null) {
    companion object {
        val AVAILABLE = ProbeAvailability(true)
    }
}

/**
 * Surveille en continu l'état de tous les terminaux.
 *
 * [run] est à exécuter tant que l'écran est visible (inutile de vider la batterie en arrière-plan) :
 * une boucle par terminal sonde la machine toutes les `pollIntervalSeconds`, et toutes les secondes
 * pendant un réveil / une extinction pour un retour quasi instantané.
 */
class StatusMonitor(
    private val prober: DeviceProber,
    private val clock: () -> Long = System::currentTimeMillis,
) {
    private val trackers = ConcurrentHashMap<String, StatusTracker>()
    private val refreshSignals = ConcurrentHashMap<String, Channel<Unit>>()
    private val _statuses = MutableStateFlow<Map<String, DeviceStatus>>(emptyMap())

    /** État courant de chaque terminal, par identifiant. */
    val statuses: StateFlow<Map<String, DeviceStatus>> = _statuses.asStateFlow()

    suspend fun run(config: Flow<AppConfig>, availability: Flow<ProbeAvailability>) {
        combine(config.distinctUntilChanged(), availability.distinctUntilChanged()) { c, a -> c to a }
            .collectLatest { (cfg, avail) ->
                prune(cfg.devices.map { it.id }.toSet())
                coroutineScope {
                    cfg.devices.forEach { device -> launch { pollLoop(device, cfg.settings, avail) } }
                }
            }
    }

    /** Demande une vérification immédiate (tous les terminaux si [deviceId] est nul). */
    fun refresh(deviceId: String? = null) {
        val targets = if (deviceId == null) refreshSignals.values else listOfNotNull(refreshSignals[deviceId])
        targets.forEach { it.trySend(Unit) }
    }

    fun onWakeSent(deviceId: String) = act(deviceId) { onWakeSent() }

    fun onPowerActionSent(deviceId: String, action: PowerAction) = act(deviceId) {
        if (action == PowerAction.REBOOT) onRestartSent() else onShutdownSent()
    }

    fun clearNotice(deviceId: String) = act(deviceId, refresh = false) { clearNotice() }

    private fun act(deviceId: String, refresh: Boolean = true, block: StatusTracker.() -> DeviceStatus) {
        publish(deviceId, tracker(deviceId).block())
        if (refresh) refresh(deviceId)
    }

    private suspend fun pollLoop(device: Device, settings: AppSettings, availability: ProbeAvailability) {
        val tracker = tracker(device.id).apply { wakeTimeoutMs = settings.wakeTimeoutSeconds * 1_000L }
        val signal = refreshSignals.getOrPut(device.id) { Channel(Channel.CONFLATED) }
        val interval = settings.pollIntervalSeconds.coerceIn(AppSettings.POLL_INTERVAL_RANGE) * 1_000L
        while (true) {
            val status = when {
                !device.hasHost -> tracker.onUnavailable(UnknownReason.NO_HOST)
                !availability.canProbe -> tracker.onUnavailable(availability.reason ?: UnknownReason.NO_NETWORK)
                else -> tracker.onProbe(prober.probe(device))
            }
            publish(device.id, status)
            val wait = if (status.state.isTransitional) minOf(interval, FAST_INTERVAL_MS) else interval
            withTimeoutOrNull(wait) { signal.receive() }
        }
    }

    private fun tracker(id: String) = trackers.getOrPut(id) { StatusTracker(clock) }

    private fun publish(id: String, status: DeviceStatus) = _statuses.update { it + (id to status) }

    private fun prune(ids: Set<String>) {
        trackers.keys.retainAll(ids)
        refreshSignals.keys.retainAll(ids)
        _statuses.update { map -> map.filterKeys { it in ids } }
    }

    companion object {
        const val FAST_INTERVAL_MS = 1_000L
    }
}
