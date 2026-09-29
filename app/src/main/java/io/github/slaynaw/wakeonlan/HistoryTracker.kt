package io.github.slaynaw.wakeonlan

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentHistory
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.history.HistoryEvent
import io.github.slaynaw.wakeonlan.core.history.HistoryKind
import io.github.slaynaw.wakeonlan.core.history.HistoryRecorder
import io.github.slaynaw.wakeonlan.core.history.HistorySource
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.data.HistoryRepository
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import java.util.concurrent.ConcurrentHashMap

/** Lecture du journal de l'agent d'un PC, pour l'interface. */
enum class AgentJournalState {
    /** Journal lu. */
    OK,

    /** Agent trop ancien : à mettre à jour pour un historique complet. */
    OUTDATED,

    /** Agent injoignable pour l'instant. */
    UNREACHABLE,
}

/**
 * Tient l'historique à jour (mêmes règles que l'application Windows) : changements d'état constatés
 * par la surveillance, demandes faites depuis le téléphone ([record]) et journal de l'agent de chaque
 * PC, relu dès que le PC répond (ce qui s'est passé pendant que l'application était fermée).
 * Les démarrages demandés depuis le téléphone, que l'agent ne peut pas voir, lui sont ensuite
 * signalés (agent 1.4.0 ou plus) : tous les appareils affichent le même historique.
 */
class HistoryTracker(
    private val config: Flow<AppConfig>,
    private val statuses: StateFlow<Map<String, DeviceStatus>>,
    private val agentClient: AgentClient,
    private val history: HistoryRepository,
    private val scope: CoroutineScope,
    private val clock: () -> Long = System::currentTimeMillis,
) {
    private class Fetch {
        var last = 0L
        var running = false

        /** Agent antérieur à 1.4.0 : les démarrages ne lui sont plus signalés. */
        var wakesUnsupported = false
    }

    private val fetches = ConcurrentHashMap<String, Fetch>()

    @Volatile
    private var devices: Map<String, Device> = emptyMap()

    private val _agentJournal = MutableStateFlow<Map<String, AgentJournalState>>(emptyMap())

    /** État de la dernière lecture du journal de chaque agent. */
    val agentJournal: StateFlow<Map<String, AgentJournalState>> = _agentJournal.asStateFlow()

    /** Suit la surveillance tant que l'application est visible (voir MainActivity). */
    suspend fun run(): Unit = coroutineScope {
        launch {
            config.collect { c ->
                val byId = c.devices.associateBy { it.id }
                devices = byId
                fetches.keys.retainAll(byId.keys)
                _agentJournal.update { states -> states.filterKeys { it in byId } }
                history.update { it.keep(byId.keys) }
            }
        }
        var previous: Map<String, DeviceStatus>? = null
        statuses.collect { current ->
            val before = previous
            previous = current
            if (before != null) onStatuses(before, current)
        }
    }

    private suspend fun onStatuses(before: Map<String, DeviceStatus>, current: Map<String, DeviceStatus>) {
        val now = clock()
        val events = HistoryRecorder.transitions(before, current, now)
        if (events.isNotEmpty()) history.update { data -> events.fold(data) { acc, e -> acc.add(e, now) } }
        for ((id, status) in current) {
            val old = before[id]
            if (old?.state != status.state || old?.agentError != status.agentError) {
                DiagnosticLog.i(
                    "état",
                    "« ${devices[id]?.name ?: id} » : ${old?.state ?: "-"} → ${status.state}" +
                        (status.method?.let { " (via $it" + (status.latencyMs?.let { ms -> ", $ms ms" } ?: "") + ")" } ?: "") +
                        (status.agentError?.let { " ; agent : $it" } ?: "") +
                        (status.unknownReason?.let { " ; raison : $it" } ?: "") +
                        (status.notice?.let { " ; avis : $it" } ?: ""),
                )
            }
            if (status.state == PowerState.ONLINE && old?.state != PowerState.ONLINE) refreshAgent(id, force = false)
        }
    }

    /** Enregistre une demande faite depuis le téléphone (démarrage, extinction...). */
    suspend fun record(deviceId: String, kind: HistoryKind) {
        val now = clock()
        history.update { it.add(HistoryEvent(deviceId, now, kind, HistorySource.APP), now) }
    }

    /**
     * Relit le journal de l'agent d'un PC ([id]) ou de tous (`null`), en arrière-plan : sans effet
     * sans agent, ou si une lecture récente existe (5 min, 20 s si [force]).
     */
    fun refreshAgent(id: String?, force: Boolean) {
        val targets = if (id == null) devices.values.toList() else listOfNotNull(devices[id])
        targets.forEach { fetch(it, force) }
    }

    /** Après l'effacement de l'historique : le journal des agents sera relu à la prochaine occasion. */
    fun reset() {
        fetches.clear()
        _agentJournal.value = emptyMap()
    }

    private fun fetch(device: Device, force: Boolean) {
        val agent = device.agent?.takeIf { it.hasKey && device.hasHost } ?: return
        val state = fetches.getOrPut(device.id) { Fetch() }
        val now = clock()
        synchronized(state) {
            val gap = if (force) FORCED_GAP_MS else AUTO_GAP_MS
            if (state.running || (state.last != 0L && now - state.last < gap)) return
            state.running = true
            state.last = now
        }
        scope.launch {
            val result = try {
                withTimeoutOrNull(TIMEOUT_MS) { agentClient.history(device.host, agent) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: RuntimeException) {
                DiagnosticLog.w("journal-agent", "« ${device.name} » : réponse inattendue", e)
                null // réponse inattendue : traitée comme un agent injoignable
            }
            if (result is AgentResult.Failure) {
                DiagnosticLog.w("journal-agent", "« ${device.name} » (${device.host}:${agent.port}) : ${result.error}${result.detail?.let { " — $it" }.orEmpty()}")
            } else if (result == null) {
                DiagnosticLog.w("journal-agent", "« ${device.name} » (${device.host}:${agent.port}) : pas de réponse")
            }
            val journal = when {
                result is AgentResult.Success -> {
                    if (devices.containsKey(device.id)) {
                        val at = clock()
                        history.update { it.replaceAgent(device.id, result.value, at, at) }
                        reportWakes(device, agent, state, result.value)
                    }
                    AgentJournalState.OK
                }
                result is AgentResult.Failure && result.error == AgentError.REJECTED -> AgentJournalState.OUTDATED
                else -> AgentJournalState.UNREACHABLE
            }
            synchronized(state) { state.running = false }
            if (devices.containsKey(device.id)) _agentJournal.update { it + (device.id to journal) }
        }
    }

    /** Signale à l'agent les démarrages demandés d'ici qu'il ne connaît pas encore. */
    private suspend fun reportWakes(device: Device, agent: AgentSettings, state: Fetch, journal: AgentHistory) {
        if (state.wakesUnsupported) return
        val wakes = history.data.value.unreportedWakes(device.id, journal)
        if (wakes.isEmpty()) return
        val result = try {
            withTimeoutOrNull(TIMEOUT_MS) { agentClient.reportWakes(device.host, agent, wakes) }
        } catch (e: CancellationException) {
            throw e
        } catch (e: RuntimeException) {
            DiagnosticLog.w("journal-agent", "« ${device.name} » : envoi des démarrages impossible", e)
            null
        }
        when {
            result is AgentResult.Success && devices.containsKey(device.id) -> {
                val at = clock()
                history.update { it.replaceAgent(device.id, result.value, at, at) }
            }
            result is AgentResult.Failure && result.error == AgentError.REJECTED -> state.wakesUnsupported = true
        }
    }

    private companion object {
        const val AUTO_GAP_MS = 5 * 60_000L
        const val FORCED_GAP_MS = 20_000L
        const val TIMEOUT_MS = 8_000L
    }
}
