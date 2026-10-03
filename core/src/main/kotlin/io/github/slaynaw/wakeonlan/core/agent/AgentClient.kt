package io.github.slaynaw.wakeonlan.core.agent

import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.net.NetworkBinder
import io.github.slaynaw.wakeonlan.core.net.connectBound
import io.github.slaynaw.wakeonlan.core.net.useCancellable
import kotlinx.serialization.json.Json
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.io.InputStream
import java.net.ConnectException
import java.net.InetSocketAddress
import java.net.Socket
import java.net.UnknownHostException
import java.security.SecureRandom

/** Causes d'échec d'un échange avec l'agent. */
enum class AgentError {
    /** Aucune clé configurée pour ce PC. */
    NO_KEY,

    /** Nom d'hôte introuvable. */
    UNKNOWN_HOST,

    /** Pas de réponse (PC éteint, pare-feu, mauvais réseau...). */
    UNREACHABLE,

    /** Le PC répond mais l'agent n'écoute pas sur ce port (PC allumé, agent arrêté). */
    REFUSED,

    /** Clé refusée par l'agent. */
    UNAUTHORIZED,

    /** Trop d'échecs d'authentification : l'agent bloque temporairement ce téléphone. */
    RATE_LIMITED,

    /** Réponse illisible ou signature invalide (mauvaise clé côté téléphone, ou usurpation). */
    PROTOCOL,

    /** L'agent a compris mais n'a pas pu exécuter la commande. */
    REJECTED,
}

sealed interface AgentResult<out T> {
    data class Success<T>(val value: T) : AgentResult<T>
    data class Failure(val error: AgentError, val detail: String? = null) : AgentResult<Nothing>

    /** Vrai si l'échec prouve malgré tout que la machine est allumée (elle a répondu). */
    val hostAnswered: Boolean
        get() = this is Success || (this is Failure && error in HOST_ANSWERED)

    companion object {
        private val HOST_ANSWERED = setOf(
            AgentError.REFUSED,
            AgentError.UNAUTHORIZED,
            AgentError.RATE_LIMITED,
            AgentError.PROTOCOL,
            AgentError.REJECTED,
        )
    }
}

/**
 * Client du protocole « wolagent/1 » (voir [AgentProtocol]). [deviceName] : nom de cet appareil, noté
 * par l'agent (1.4.0 ou plus) avec chaque demande, pour l'historique commun.
 */
class AgentClient(
    private val binder: NetworkBinder = NetworkBinder.Default,
    private val connectTimeoutMs: Int = 2_000,
    private val readTimeoutMs: Int = 4_000,
    private val random: SecureRandom = SecureRandom(),
    private val deviceName: () -> String? = { null },
) {
    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }

    suspend fun status(host: String, agent: AgentSettings): AgentResult<AgentStatus> =
        when (val r = exchange(host, agent, RequestBody(cmd = "status"))) {
            is AgentResult.Success -> AgentResult.Success(r.value.toStatus())
            is AgentResult.Failure -> r
        }

    /**
     * Demande une action d'alimentation. L'agent répond AVANT d'exécuter l'action,
     * après le délai [delaySeconds].
     */
    suspend fun power(
        host: String,
        agent: AgentSettings,
        action: PowerAction,
        delaySeconds: Int = 0,
        force: Boolean = false,
    ): AgentResult<String> {
        require(delaySeconds in 0..3600) { "Délai invalide" }
        val body = RequestBody(cmd = action.wire, delay = delaySeconds, force = force, by = name())
        return when (val r = exchange(host, agent, body)) {
            is AgentResult.Success -> AgentResult.Success(r.value.message)
            is AgentResult.Failure -> r
        }
    }

    /** Lit le journal du PC (30 jours). Un agent trop ancien répond [AgentError.REJECTED]. */
    suspend fun history(host: String, agent: AgentSettings): AgentResult<AgentHistory> =
        when (val r = exchange(host, agent, RequestBody(cmd = "history"), AgentProtocol.MAX_HISTORY_BYTES)) {
            is AgentResult.Success ->
                r.value.history?.let { AgentResult.Success(it) } ?: protocolError("journal absent de la réponse")
            is AgentResult.Failure -> r
        }

    /**
     * Lit les mesures enregistrées par le PC ([day] : jour UTC « 2026-10-03 », `null` pour la seule
     * liste des jours). Un agent antérieur à 1.8.0 répond [AgentError.REJECTED].
     */
    suspend fun metrics(host: String, agent: AgentSettings, day: String? = null): AgentResult<AgentMetrics> =
        when (val r = exchange(host, agent, RequestBody(cmd = "metrics", day = day), AgentProtocol.MAX_METRICS_BYTES)) {
            is AgentResult.Success ->
                r.value.metrics?.let { AgentResult.Success(it) } ?: protocolError("mesures absentes de la réponse")
            is AgentResult.Failure -> r
        }

    /**
     * Signale au journal du PC les démarrages demandés depuis cet appareil ([times] en secondes), et
     * renvoie le journal à jour. Un agent antérieur à 1.4.0 répond [AgentError.REJECTED].
     */
    suspend fun reportWakes(host: String, agent: AgentSettings, times: List<Long>): AgentResult<AgentHistory> {
        require(times.size in 1..AgentProtocol.MAX_WAKES) { "Nombre de démarrages invalide" }
        val body = RequestBody(cmd = "wakes", wakes = times, by = name())
        return when (val r = exchange(host, agent, body, AgentProtocol.MAX_HISTORY_BYTES)) {
            is AgentResult.Success ->
                r.value.history?.let { AgentResult.Success(it) } ?: protocolError("journal absent de la réponse")
            is AgentResult.Failure -> r
        }
    }

    private fun name(): String? = deviceName()?.trim()?.take(AgentProtocol.MAX_BY_LENGTH)?.takeIf { it.isNotEmpty() }

    private suspend fun exchange(
        host: String,
        agent: AgentSettings,
        request: RequestBody,
        maxResponseBytes: Int = AgentProtocol.MAX_LINE_BYTES,
    ): AgentResult<ResponseBody> {
        val key = AgentKey.decodeOrNull(agent.key) ?: return AgentResult.Failure(AgentError.NO_KEY)
        var connected = false
        return try {
            Socket().useCancellable { socket ->
                val address = InetSocketAddress(binder.resolve(host), agent.port)
                socket.connectBound(binder, address, connectTimeoutMs)
                connected = true
                socket.soTimeout = readTimeoutMs
                converse(socket, key, request, maxResponseBytes)
            }
        } catch (e: UnknownHostException) {
            AgentResult.Failure(AgentError.UNKNOWN_HOST, e.message)
        } catch (e: ConnectException) {
            val refused = e.message?.contains("refused", ignoreCase = true) == true
            AgentResult.Failure(if (refused) AgentError.REFUSED else AgentError.UNREACHABLE, e.message)
        } catch (e: IOException) {
            // Connexion établie puis coupée / muette : la machine est allumée mais l'agent ne répond pas bien.
            AgentResult.Failure(if (connected) AgentError.PROTOCOL else AgentError.UNREACHABLE, e.message)
        }
    }

    private fun converse(socket: Socket, key: ByteArray, request: RequestBody, maxResponseBytes: Int): AgentResult<ResponseBody> {
        val input = socket.getInputStream().buffered()
        val output = socket.getOutputStream()

        val helloLine = readLine(input, AgentProtocol.MAX_LINE_BYTES)
        // Un agent qui bloque ce téléphone envoie directement une erreur à la place de la salutation.
        val hello = decode<HelloMessage>(helloLine) ?: return earlyError(helloLine)
        if (hello.proto != AgentProtocol.PROTO) return protocolError("version de protocole inconnue : ${hello.proto}")
        if (AgentProtocol.decodedLength(hello.nonce) != AgentProtocol.SERVER_NONCE_BYTES) {
            return protocolError("nonce invalide")
        }

        val cnonce = AgentProtocol.encodeNonce(ByteArray(AgentProtocol.CLIENT_NONCE_BYTES).also(random::nextBytes))
        val body = json.encodeToString(RequestBody.serializer(), request)
        val mac = AgentProtocol.requestMac(key, hello.nonce, cnonce, body)
        val message = json.encodeToString(RequestMessage.serializer(), RequestMessage(cnonce, body, mac))
        output.write((message + "\n").toByteArray(Charsets.UTF_8))
        output.flush()

        val response = decode<ResponseMessage>(readLine(input, maxResponseBytes)) ?: return protocolError("réponse illisible")
        if (response.error != null) return errorFrom(response.error)
        val responseBody = response.body ?: return protocolError("réponse sans contenu")
        val responseMac = response.mac ?: return protocolError("réponse non signée")
        val expected = AgentProtocol.responseMac(key, hello.nonce, cnonce, responseBody)
        if (!AgentProtocol.macEquals(expected, responseMac)) return protocolError("signature de la réponse invalide")

        val parsed = decode<ResponseBody>(responseBody) ?: return protocolError("contenu de réponse illisible")
        return if (parsed.ok) AgentResult.Success(parsed) else AgentResult.Failure(AgentError.REJECTED, parsed.message)
    }

    private fun earlyError(line: String?): AgentResult.Failure =
        decode<ResponseMessage>(line)?.error?.let(::errorFrom) ?: protocolError("salutation illisible")

    private fun errorFrom(error: String): AgentResult.Failure = when (error) {
        "unauthorized" -> AgentResult.Failure(AgentError.UNAUTHORIZED)
        "rate_limited" -> AgentResult.Failure(AgentError.RATE_LIMITED)
        else -> protocolError("erreur de l'agent : $error")
    }

    private inline fun <reified T> decode(text: String?): T? =
        text?.let { runCatching { json.decodeFromString<T>(it) }.getOrNull() }

    private fun protocolError(detail: String) = AgentResult.Failure(AgentError.PROTOCOL, detail)

    /** Lit une ligne terminée par `\n`, de taille bornée (protection contre un pair malveillant). */
    private fun readLine(input: InputStream, maxBytes: Int): String? {
        val buffer = ByteArrayOutputStream()
        while (true) {
            val b = input.read()
            if (b == -1) return if (buffer.size() == 0) null else buffer.toString(Charsets.UTF_8.name())
            if (b == '\n'.code) return buffer.toString(Charsets.UTF_8.name()).trimEnd('\r')
            if (buffer.size() >= maxBytes) throw IOException("message trop long")
            buffer.write(b)
        }
    }
}
