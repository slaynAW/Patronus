package io.github.slaynaw.wakeonlan

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import io.github.slaynaw.wakeonlan.share.ReceivedTarget
import io.github.slaynaw.wakeonlan.share.ShareManager
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import java.util.concurrent.ConcurrentHashMap

/**
 * Partage v2, côté destinataire (docs/PARTAGE.md, « Partage v2 »), mêmes règles que l'application Windows : dès qu'un PC
 * reçu répond, ce téléphone utilise l'invitation qui lui est réservée, une seule fois par invitation, puis joint ce PC en
 * wolagent/2 comme ses propres PC. Retiré par le PC, il le traite de nouveau en « démarrer ».
 */
class ReceivedPairingTracker(
    private val statuses: StateFlow<Map<String, DeviceStatus>>,
    private val agentClient: AgentClient,
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

    /** Suit la surveillance tant que l'application est visible (voir MainActivity). */
    suspend fun run() {
        statuses.collect { current -> current.forEach { (id, status) -> consider(id, status) } }
    }

    private fun consider(id: String, status: DeviceStatus) {
        val now = clock()
        val target = share.receivedTarget(id, status, now / 1000) ?: return
        val attempt = attempts.getOrPut(id) { Attempt() }
        synchronized(attempt) {
            if (attempt.running || (!target.unpair && now < attempt.next)) return
            attempt.running = true
        }
        scope.launch {
            try {
                if (target.unpair) share.unpairReceived(target) else pair(target, attempt)
            } catch (e: CancellationException) {
                throw e
            } catch (e: RuntimeException) {
                DiagnosticLog.w(AREA, "PC reçu « ${target.device.name} » : erreur imprévue", e)
            } finally {
                synchronized(attempt) { attempt.running = false }
            }
        }
    }

    private suspend fun pair(target: ReceivedTarget, attempt: Attempt) {
        // null : réussite (ou refus définitif, invitation marquée), ou délai dépassé ; finished les distingue.
        var finished = false
        val failure = withTimeoutOrNull(TIMEOUT_MS) { share.pairReceived(target, agentClient).also { finished = true } }
        synchronized(attempt) {
            if (finished && failure == null) {
                attempt.failures = 0
                attempt.next = 0
            } else {
                attempt.failures++
                attempt.next = clock() + minOf(RETRY_MAX_MS, RETRY_MIN_MS shl minOf(attempt.failures - 1, 6))
            }
        }
    }

    private companion object {
        const val AREA = "partage"

        /** Un appairage avec une invitation reçue. */
        const val TIMEOUT_MS = 20_000L

        /** Délais entre deux essais après une erreur réseau : doublés à chaque fois, d'une minute à une heure. */
        const val RETRY_MIN_MS = 60_000L
        const val RETRY_MAX_MS = 3_600_000L
    }
}
