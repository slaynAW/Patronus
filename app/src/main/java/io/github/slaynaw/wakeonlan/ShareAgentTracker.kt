package io.github.slaynaw.wakeonlan

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.agent2.Role
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.data.ConfigRepository
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import io.github.slaynaw.wakeonlan.share.ShareManager
import io.github.slaynaw.wakeonlan.share.isPinned
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import java.util.concurrent.ConcurrentHashMap

/**
 * Partage v2, côté propriétaire (docs/PARTAGE.md, « Partage v2 »), mêmes règles que l'application Windows : quand ce
 * téléphone administre un PC partagé et que ce PC répond, il y crée les invitations réservées aux appareils des
 * personnes autorisées, aligne leurs rôles et applique les retraits ([ShareManager.syncAgent]). Au plus une session à la
 * fois par PC ; les invitations en attente sont vérifiées toutes les [POLL_MS].
 */
class ShareAgentTracker(
    private val repository: ConfigRepository,
    private val statuses: StateFlow<Map<String, DeviceStatus>>,
    private val agentClient: AgentClient,
    private val share: ShareManager,
    private val scope: CoroutineScope,
    /** Empreinte courte de ce téléphone, pour l'aide « devenir administrateur » (null si indisponible). */
    private val selfPin: () -> String?,
    private val clock: () -> Long = System::currentTimeMillis,
) {
    private class Attempt {
        var running = false
        var failures = 0
        var next = 0L
        var last = 0L
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
        val agentStatus = status.agent ?: return
        if (!device.isPinned || status.state != PowerState.ONLINE || agentStatus.proto != Wolagent2.ALPN) return
        val now = clock()
        val (work, poll) = share.agentWork(id, now / 1000)
        val notAdmin = (work || poll) && agentStatus.role != Role.ADMIN.wire
        share.setNotAdmin(id, notAdmin, selfPin)
        if (notAdmin || !(work || poll)) return
        val attempt = attempts.getOrPut(id) { Attempt() }
        synchronized(attempt) {
            if (attempt.running || now < attempt.next || (!work && now - attempt.last < POLL_MS)) return
            attempt.running = true
        }
        scope.launch {
            try {
                sync(device, attempt)
            } catch (e: CancellationException) {
                throw e
            } catch (e: RuntimeException) {
                DiagnosticLog.w(AREA, "partage v2 sur « ${device.name} » : erreur imprévue", e)
            } finally {
                synchronized(attempt) { attempt.running = false }
            }
        }
    }

    private suspend fun sync(device: Device, attempt: Attempt) {
        // null : réussite, ou délai dépassé ; finished les distingue.
        var finished = false
        val failure = withTimeoutOrNull(TIMEOUT_MS) { share.syncAgent(device, agentClient).also { finished = true } }
        when {
            finished && failure == null -> synchronized(attempt) {
                attempt.failures = 0
                attempt.next = 0
                attempt.last = clock()
            }
            failure?.error == AgentError.REJECTED && failure.code == ResponseCode.INSUFFICIENT_ROLE ->
                share.setNotAdmin(device.id, true, selfPin)
            else -> {
                val wait = synchronized(attempt) {
                    attempt.failures++
                    minOf(RETRY_MAX_MS, RETRY_MIN_MS shl minOf(attempt.failures - 1, 6)).also { attempt.next = clock() + it }
                }
                val why = failure?.let { "${it.error}${it.code?.let { c -> " ($c)" }.orEmpty()}" } ?: "pas de réponse"
                DiagnosticLog.w(AREA, "partage v2 sur « ${device.name} » : $why, nouvel essai dans ${wait / 60_000} min")
            }
        }
    }

    private companion object {
        const val AREA = "partage"

        /** Une session : lecture de l'état, puis quelques commandes. */
        const val TIMEOUT_MS = 30_000L

        /** Relecture de l'état d'un PC allumé dont des invitations attendent d'être utilisées (session légère). */
        const val POLL_MS = 2 * 60_000L

        /** Délais entre deux essais après un échec : doublés à chaque fois, d'une minute à une heure. */
        const val RETRY_MIN_MS = 60_000L
        const val RETRY_MAX_MS = 3_600_000L
    }
}
