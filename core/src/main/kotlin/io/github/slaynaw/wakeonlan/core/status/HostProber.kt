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
import java.security.MessageDigest
import java.util.Base64
import java.util.concurrent.ConcurrentHashMap
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
 *
 * Après un refus de l'agent (identité inconnue, clé refusée), il n'est recontacté qu'après un délai, doublé à chaque
 * nouveau refus : chaque refus compte comme un échec, et l'agent bloque une adresse après 5 échecs en une minute
 * (docs/PROTOCOLE-V2.md, §3.3). Entre-temps, une simple connexion à son port dit si le PC est allumé.
 */
class HostProber(
    private val binder: NetworkBinder = NetworkBinder.Default,
    private val agentClient: AgentClient = AgentClient(binder),
    private val timeoutMs: Long = DEFAULT_TIMEOUT_MS,
    private val ping: suspend (InetAddress, Int) -> Boolean = ::defaultPing,
    tcp: (suspend (InetAddress, Int, Int) -> Boolean)? = null,
    private val now: () -> Long = System::currentTimeMillis,
) : DeviceProber {
    private val tcp: suspend (InetAddress, Int, Int) -> Boolean = tcp ?: ::tcpAnswers

    /** Derniers refus de l'agent, par PC. */
    private val refusals = ConcurrentHashMap<String, Refusal>()

    private data class Refusal(val settings: String, val error: AgentError, val count: Int, val until: Long)

    /** Oublie le dernier refus de l'agent d'un PC : il sera recontacté à la prochaine sonde (action réussie, appairage). */
    fun forget(deviceId: String) {
        refusals.remove(deviceId)
    }

    override suspend fun probe(device: Device): ProbeResult = coroutineScope {
        if (!device.hasHost) return@coroutineScope ProbeResult.UNREACHABLE
        val start = TimeSource.Monotonic.markNow()
        val address = withTimeoutOrNull(timeoutMs) {
            runCatching { awaitBlocking { binder.resolve(device.host) } }.getOrNull()
        } ?: return@coroutineScope ProbeResult.UNREACHABLE

        val agentSettings = device.agent?.takeIf { it.ready }
        // Agent qui refuse cet appareil : pas de nouvelle demande avant le délai, mais une simple connexion à son
        // port (qu'il ne compte pas comme un échec) dit si le PC est allumé.
        val refused = agentSettings?.let { refusedRecently(device) }
        val ports = if (refused != null) device.probePorts + agentSettings.port else device.probePorts
        val agentJob = agentSettings?.takeIf { refused == null }?.let {
            async { withTimeoutOrNull(timeoutMs) { agentClient.status(address.hostAddress, it) } }
        }
        val fallbackJob = async {
            withTimeoutOrNull(timeoutMs) { firstReachable(address, ports, start) }
        }

        val agentResult = agentJob?.await()
        if (agentResult != null) noteAgent(device, agentResult)
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
            refused != null -> refused
            agentResult == null -> AgentError.UNREACHABLE
            else -> (agentResult as? AgentResult.Failure)?.error
        }
        (fallbackJob.await() ?: ProbeResult.UNREACHABLE).copy(agentError = agentError)
    }

    /** Refus récent de l'agent d'un PC, avec les mêmes réglages, dont le délai court encore ; null sinon. */
    private fun refusedRecently(device: Device): AgentError? {
        val r = refusals[device.id] ?: return null
        return r.error.takeIf { r.settings == settingsKey(device) && now() < r.until }
    }

    /** Retient un refus de l'agent (délai doublé à chaque refus), ou l'oublie après une réponse. */
    private fun noteAgent(device: Device, result: AgentResult<*>) {
        val error = (result as? AgentResult.Failure)?.error
        if (error == null) {
            refusals.remove(device.id)
            return
        }
        if (error != AgentError.UNREGISTERED && error != AgentError.UNAUTHORIZED && error != AgentError.RATE_LIMITED) return
        val key = settingsKey(device)
        val previous = refusals[device.id]?.takeIf { it.settings == key }
        val next = if (error == AgentError.RATE_LIMITED) {
            Refusal(key, error, previous?.count ?: 0, now() + RATE_LIMITED_PAUSE_MS)
        } else {
            val count = (previous?.count ?: 0) + 1
            Refusal(key, error, count, now() + minOf(REFUSAL_MAX_DELAY_MS, REFUSAL_MIN_DELAY_MS shl minOf(count - 1, 5)))
        }
        refusals[device.id] = next
    }

    /** Adresse et réglages de l'agent d'un PC, résumés (sans garder la clé) : un changement remet le délai à zéro. */
    private fun settingsKey(device: Device): String {
        val a = device.agent
        val text = listOf(device.host, a?.port?.toString().orEmpty(), a?.key.orEmpty(), a?.pin.orEmpty()).joinToString("\u0000")
        return Base64.getEncoder().encodeToString(MessageDigest.getInstance("SHA-256").digest(text.toByteArray(Charsets.UTF_8)))
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

        /** Délai avant de recontacter un agent qui a refusé cet appareil, doublé à chaque refus. */
        const val REFUSAL_MIN_DELAY_MS = 60_000L
        const val REFUSAL_MAX_DELAY_MS = 30 * 60_000L

        /** Une adresse bloquée par l'agent l'est 5 minutes. */
        const val RATE_LIMITED_PAUSE_MS = 5 * 60_000L

        suspend fun defaultPing(address: InetAddress, timeoutMs: Int): Boolean =
            awaitBlocking { runCatching { address.isReachable(timeoutMs) }.getOrDefault(false) }
    }
}
