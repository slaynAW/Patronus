package io.github.slaynaw.wakeonlan

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.data.SpecsRepository
import io.github.slaynaw.wakeonlan.data.StoredSpecs
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import java.util.concurrent.ConcurrentHashMap

/**
 * Lit la fiche de chaque PC (agent 1.10.0) quand il répond par son agent, si elle manque, a plus
 * d'un jour ou vient d'un agent depuis mis à jour (mêmes règles que l'application Windows).
 */
class SpecsTracker(
    private val config: Flow<AppConfig>,
    private val statuses: StateFlow<Map<String, DeviceStatus>>,
    private val agentClient: AgentClient,
    private val specs: SpecsRepository,
    private val scope: CoroutineScope,
    private val clock: () -> Long = System::currentTimeMillis,
) {
    private class Fetch {
        var running = false
        var last = 0L
        var gap = 0L

        /** Version d'un agent qui ne connaît pas la commande (redemandée s'il est mis à jour). */
        var unsupported: String? = null
    }

    private val fetches = ConcurrentHashMap<String, Fetch>()

    @Volatile
    private var devices: Map<String, Device> = emptyMap()

    /** Suit la surveillance tant que l'application est visible (voir MainActivity). */
    suspend fun run(): Unit = coroutineScope {
        launch {
            config.collect { c ->
                val byId = c.devices.associateBy { it.id }
                devices = byId
                fetches.keys.retainAll(byId.keys)
                specs.update { stored -> stored.filterKeys { it in byId } }
            }
        }
        statuses.collect { current -> current.forEach { (id, status) -> fetch(id, status) } }
    }

    private fun fetch(id: String, status: DeviceStatus) {
        val agentStatus = status.agent ?: return
        if (status.state != PowerState.ONLINE) return
        val device = devices[id] ?: return
        val settings = device.agent?.takeIf { it.hasKey && device.hasHost } ?: return
        val version = agentStatus.version
        val now = clock()
        val stored = specs.data.value[id]
        val state = fetches.getOrPut(id) { Fetch() }
        synchronized(state) {
            val fresh = stored != null && stored.agent == version && now - stored.fetched < MAX_AGE_MS
            if (state.running || fresh || state.unsupported == version || (state.last != 0L && now - state.last < state.gap)) return
            state.running = true
            state.last = now
            state.gap = RETRY_MS
        }
        scope.launch {
            val result = try {
                withTimeoutOrNull(TIMEOUT_MS) { agentClient.specs(device.host, settings) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: RuntimeException) {
                DiagnosticLog.w("fiche", "« ${device.name} » : réponse inattendue", e)
                null
            }
            when {
                result is AgentResult.Success -> if (devices.containsKey(id)) {
                    specs.update { it + (id to StoredSpecs(result.value, clock(), version)) }
                }
                result is AgentResult.Failure && result.code == "busy" -> synchronized(state) { state.gap = BUSY_RETRY_MS }
                result is AgentResult.Failure && result.error == AgentError.REJECTED -> {
                    // Agent antérieur à 1.10.0 : redemandée dès qu'il change de version.
                    synchronized(state) {
                        state.unsupported = version
                        state.gap = 0
                    }
                    DiagnosticLog.i("fiche", "« ${device.name} » : agent $version trop ancien")
                }
                result is AgentResult.Failure -> DiagnosticLog.w("fiche", "« ${device.name} » : ${result.error}${result.detail?.let { " — $it" }.orEmpty()}")
                else -> DiagnosticLog.w("fiche", "« ${device.name} » : pas de réponse")
            }
            synchronized(state) { state.running = false }
        }
    }

    private companion object {
        const val MAX_AGE_MS = 24 * 3_600_000L
        const val RETRY_MS = 5 * 60_000L
        const val BUSY_RETRY_MS = 15_000L
        const val TIMEOUT_MS = 8_000L
    }
}
