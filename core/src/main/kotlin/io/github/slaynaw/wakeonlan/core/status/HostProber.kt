package io.github.slaynaw.wakeonlan.core.status

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.net.NetworkBinder
import io.github.slaynaw.wakeonlan.core.net.awaitBlocking
import io.github.slaynaw.wakeonlan.core.net.connectBound
import io.github.slaynaw.wakeonlan.core.net.useCancellable
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.async
import kotlinx.coroutines.cancelChildren
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.joinAll
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import java.io.IOException
import java.net.ConnectException
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Socket
import kotlin.time.TimeSource

/** Méthode qui a permis de détecter la machine. */
enum class ProbeMethod { AGENT, TCP, PING }

/** Résultat d'une sonde ponctuelle. */
data class ProbeResult(
    val reachable: Boolean,
    val latencyMs: Long? = null,
    val method: ProbeMethod? = null,
    val agent: AgentStatus? = null,
    val agentError: AgentError? = null,
) {
    companion object {
        val UNREACHABLE = ProbeResult(reachable = false)
    }
}

/** Abstraction de la sonde, pour pouvoir la simuler dans les tests. */
fun interface DeviceProber {
    suspend fun probe(device: Device): ProbeResult
}

/**
 * Détermine si une machine est allumée en combinant plusieurs méthodes, lancées en parallèle :
 *
 * - **Agent** (si configuré) : échange authentifié, qui donne en plus le nom et l'uptime du PC ;
 * - **TCP** sur des ports courants : une connexion acceptée *ou refusée* prouve que la machine répond
 *   (une machine éteinte ne répond pas du tout) ;
 * - **Ping ICMP**, souvent bloqué par le pare-feu Windows mais utile pour les autres systèmes.
 *
 * La première réponse positive l'emporte ; le tout est borné par [timeoutMs].
 */
class HostProber(
    private val binder: NetworkBinder = NetworkBinder.Default,
    private val agentClient: AgentClient = AgentClient(binder),
    private val timeoutMs: Long = DEFAULT_TIMEOUT_MS,
    private val ping: suspend (InetAddress, Int) -> Boolean = ::defaultPing,
    tcp: (suspend (InetAddress, Int, Int) -> Boolean)? = null,
) : DeviceProber {
    private val tcp: suspend (InetAddress, Int, Int) -> Boolean = tcp ?: ::tcpAnswers

    override suspend fun probe(device: Device): ProbeResult = coroutineScope {
        if (!device.hasHost) return@coroutineScope ProbeResult.UNREACHABLE
        val start = TimeSource.Monotonic.markNow()
        val address = withTimeoutOrNull(timeoutMs) {
            runCatching { awaitBlocking { binder.resolve(device.host) } }.getOrNull()
        } ?: return@coroutineScope ProbeResult.UNREACHABLE

        val agentSettings = device.agent?.takeIf { it.hasKey }
        val agentJob = agentSettings?.let {
            async { withTimeoutOrNull(timeoutMs) { agentClient.status(address.hostAddress, it) } }
        }
        val fallbackJob = async {
            withTimeoutOrNull(timeoutMs) { firstReachable(address, device.probePorts, start) }
        }

        val agentResult = agentJob?.await()
        if (agentResult != null && agentResult.hostAnswered) {
            fallbackJob.cancel()
            val latency = start.elapsedNow().inWholeMilliseconds
            return@coroutineScope when (agentResult) {
                is AgentResult.Success -> ProbeResult(true, latency, ProbeMethod.AGENT, agent = agentResult.value)
                is AgentResult.Failure -> ProbeResult(true, latency, ProbeMethod.TCP, agentError = agentResult.error)
            }
        }
        val agentError = when {
            agentSettings == null -> null
            agentResult == null -> AgentError.UNREACHABLE
            else -> (agentResult as? AgentResult.Failure)?.error
        }
        (fallbackJob.await() ?: ProbeResult.UNREACHABLE).copy(agentError = agentError)
    }

    private suspend fun firstReachable(
        address: InetAddress,
        ports: List<Int>,
        start: TimeSource.Monotonic.ValueTimeMark,
    ): ProbeResult? = coroutineScope {
        val winner = CompletableDeferred<ProbeResult?>()
        val connectTimeout = timeoutMs.toInt()
        val jobs = ports.distinct().map { port ->
            launch {
                if (tcp(address, port, connectTimeout)) {
                    winner.complete(ProbeResult(true, start.elapsedNow().inWholeMilliseconds, ProbeMethod.TCP))
                }
            }
        } + launch {
            if (runCatching { ping(address, connectTimeout) }.getOrDefault(false)) {
                winner.complete(ProbeResult(true, start.elapsedNow().inWholeMilliseconds, ProbeMethod.PING))
            }
        }
        launch {
            jobs.joinAll()
            winner.complete(null)
        }
        val result = winner.await()
        coroutineContext.cancelChildren()
        result
    }

    /** Vrai si la machine répond sur ce port, que la connexion soit acceptée ou refusée. */
    private suspend fun tcpAnswers(address: InetAddress, port: Int, timeout: Int): Boolean = try {
        Socket().useCancellable { socket ->
            socket.connectBound(binder, InetSocketAddress(address, port), timeout)
            true
        }
    } catch (e: ConnectException) {
        e.message?.contains("refused", ignoreCase = true) == true
    } catch (_: IOException) {
        false
    }

    companion object {
        const val DEFAULT_TIMEOUT_MS = 1_500L

        suspend fun defaultPing(address: InetAddress, timeoutMs: Int): Boolean =
            awaitBlocking { runCatching { address.isReachable(timeoutMs) }.getOrDefault(false) }
    }
}
