package io.github.slaynaw.wakeonlan

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentUpgrade
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.core.status.HostProber
import io.github.slaynaw.wakeonlan.data.ConfigRepository
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import io.github.slaynaw.wakeonlan.share.ShareManager
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import java.util.concurrent.ConcurrentHashMap

/**
 * Fait passer les PC du téléphone en wolagent/2 dès que leur agent l'annonce ([AgentUpgrade], même règle que
 * l'application Windows) : empreinte vérifiée, téléphone enregistré par l'agent, puis clé effacée. Les PC reçus d'une
 * autre personne ne sont pas concernés.
 */
class AgentUpgradeTracker(
    private val repository: ConfigRepository,
    private val statuses: StateFlow<Map<String, DeviceStatus>>,
    private val agentClient: AgentClient,
    private val prober: HostProber,
    private val share: ShareManager,
    private val scope: CoroutineScope,
    private val clock: () -> Long = System::currentTimeMillis,
) {
    private class Attempt {
        var running = false
        var failures = 0
        var next = 0L
    }

    private val attempts = ConcurrentHashMap<String, Attempt>()

    @Volatile
    private var devices: Map<String, Device> = emptyMap()

    /** Suit la surveillance tant que l'application est visible (voir MainActivity). */
    suspend fun run(): Unit = coroutineScope {
        launch {
            repository.config.collect { c: AppConfig ->
                devices = c.devices.associateBy { it.id }
                attempts.keys.retainAll(devices.keys)
            }
        }
        statuses.collect { current -> current.forEach { (id, status) -> consider(id, status) } }
    }

    private fun consider(id: String, status: DeviceStatus) {
        val device = devices[id] ?: return
        // Tri rapide : un PC partagé avec le droit « démarrer et éteindre » n'est jamais plus à tenter qu'un autre.
        // Sans clé (PC déjà passé, ou retiré par le PC), rien n'est jamais tenté : pas de réenregistrement automatique.
        if (!AgentUpgrade.worthTrying(device, status, sharedWithControl = false)) return
        val attempt = attempts.getOrPut(id) { Attempt() }
        synchronized(attempt) {
            if (attempt.running || clock() < attempt.next) return
            attempt.running = true
        }
        scope.launch {
            try {
                val shared = share.sharedWithControl(id)
                if (AgentUpgrade.worthTrying(device, status, shared)) upgrade(device, shared, attempt)
            } catch (e: CancellationException) {
                throw e
            } catch (e: RuntimeException) {
                DiagnosticLog.w("agent", "passage en wolagent/2 de « ${device.name} » : erreur imprévue", e)
            } finally {
                synchronized(attempt) { attempt.running = false }
            }
        }
    }

    private suspend fun upgrade(device: Device, shared: Boolean, attempt: Attempt) {
        val before = device.agent ?: return
        val outcome = withTimeoutOrNull(TIMEOUT_MS) { AgentUpgrade.run(agentClient, device, keepKey = shared) }
        when (outcome) {
            is AgentUpgrade.Outcome.Upgraded -> {
                synchronized(attempt) {
                    attempt.failures = 0
                    attempt.next = 0
                }
                prober.forget(device.id)
                outcome.role?.let { DiagnosticLog.i("agent", "« ${device.name} » : ce téléphone est enregistré par l'agent (rôle $it)") }
                // La clé reste si le PC vient d'être partagé avec le droit « démarrer et éteindre ».
                val next = if (share.sharedWithControl(device.id)) outcome.settings.copy(key = before.key) else outcome.settings
                if (next == before) return
                var applied = false
                repository.update { c ->
                    val current = c.device(device.id)
                    // Fiche modifiée entre-temps (nouvelle clé, import) : l'état suivant décidera.
                    if (current == null || current.agent != before) return@update c
                    applied = true
                    c.upsert(current.copy(agent = next))
                }
                if (applied) {
                    DiagnosticLog.i(
                        "agent",
                        "« ${device.name} » : passé en wolagent/2 (empreinte ${next.shortPin})" +
                            if (next.hasKey) ", clé gardée pour le partage" else ", clé wolagent/1 effacée",
                    )
                }
            }
            AgentUpgrade.Outcome.Unchanged -> Unit
            is AgentUpgrade.Outcome.Failed, null -> {
                val wait = synchronized(attempt) {
                    attempt.failures++
                    AgentUpgrade.retryDelayMs(attempt.failures).also { attempt.next = clock() + it }
                }
                val why = (outcome as? AgentUpgrade.Outcome.Failed)?.let { "${it.error}${it.detail?.let { d -> " — $d" }.orEmpty()}" } ?: "pas de réponse"
                DiagnosticLog.w("agent", "passage en wolagent/2 de « ${device.name} » : $why, nouvel essai dans ${wait / 60_000} min")
            }
        }
    }

    private companion object {
        /** Un passage complet : trois échanges au plus. */
        const val TIMEOUT_MS = 30_000L
    }
}
