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
 * pendant un réveil / une extinction pour un retour quasi instantané, ainsi que pour le terminal
 * affiché en détail ([watch]) : sa latence est alors tracée en direct.
 */
class StatusMonitor(
    private val prober: DeviceProber,
    private val clock: () -> Long = System::currentTimeMillis,
) {
    private val trackers = ConcurrentHashMap<String, StatusTracker>()
    private val refreshSignals = ConcurrentHashMap<String, Channel<Unit>>()
    private val _statuses = MutableStateFlow<Map<String, DeviceStatus>>(emptyMap())
    private val _latency = MutableStateFlow<Map<String, List<LatencySample>>>(emptyMap())
    private val _temps = MutableStateFlow<Map<String, List<TempSample>>>(emptyMap())
    private val watchers = HashMap<String, Int>()

    /** État courant de chaque terminal, par identifiant. */
    val statuses: StateFlow<Map<String, DeviceStatus>> = _statuses.asStateFlow()

    /** Mesures de latence des dernières minutes de chaque terminal (voir [LatencyLog]). */
    val latency: StateFlow<Map<String, List<LatencySample>>> = _latency.asStateFlow()

    /** Relevés de températures des dernières minutes de chaque terminal (voir [TempLog]). */
    val temps: StateFlow<Map<String, List<TempSample>>> = _temps.asStateFlow()

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

    /**
     * Le terminal est affiché en détail : il est sondé toutes les secondes jusqu'à l'appel de
     * [unwatch] correspondant (plusieurs écrans peuvent le suivre en même temps).
     */
    fun watch(deviceId: String) {
        val first = synchronized(watchers) {
            val count = watchers[deviceId] ?: 0
            watchers[deviceId] = count + 1
            count == 0
        }
        if (first) refresh(deviceId)
    }

    fun unwatch(deviceId: String) {
        synchronized(watchers) {
            val count = (watchers[deviceId] ?: return) - 1
            if (count > 0) watchers[deviceId] = count else watchers.remove(deviceId)
        }
    }

    private fun isWatched(deviceId: String): Boolean = synchronized(watchers) { deviceId in watchers }

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
                else -> {
                    val result = prober.probe(device)
                    val now = clock()
                    record(device.id, LatencySample(now, result.latencyMs.takeIf { result.reachable }))
                    TempLog.sampleOf(now, result.agent)?.let { recordTemp(device.id, it) }
                    tracker.onProbe(result)
                }
            }
            publish(device.id, status)
            val fast = status.state.isTransitional || isWatched(device.id)
            val wait = if (fast) minOf(interval, FAST_INTERVAL_MS) else interval
            withTimeoutOrNull(wait) { signal.receive() }
        }
    }

    private fun tracker(id: String) = trackers.getOrPut(id) { StatusTracker(clock) }

    private fun publish(id: String, status: DeviceStatus) = _statuses.update { it + (id to status) }

    private fun record(id: String, sample: LatencySample) =
        _latency.update { it + (id to LatencyLog.append(it[id].orEmpty(), sample)) }

    private fun recordTemp(id: String, sample: TempSample) =
        _temps.update { it + (id to TempLog.append(it[id].orEmpty(), sample)) }

    private fun prune(ids: Set<String>) {
        trackers.keys.retainAll(ids)
        refreshSignals.keys.retainAll(ids)
        _statuses.update { map -> map.filterKeys { it in ids } }
        _latency.update { map -> map.filterKeys { it in ids } }
        _temps.update { map -> map.filterKeys { it in ids } }
    }

    companion object {
        const val FAST_INTERVAL_MS = LatencyLog.LIVE_INTERVAL_MS
    }
}
