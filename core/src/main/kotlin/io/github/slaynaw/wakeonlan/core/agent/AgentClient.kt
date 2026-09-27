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

/** Client du protocole « wolagent/1 » (voir [AgentProtocol]). */
class AgentClient(
    private val binder: NetworkBinder = NetworkBinder.Default,
    private val connectTimeoutMs: Int = 2_000,
    private val readTimeoutMs: Int = 4_000,
    private val random: SecureRandom = SecureRandom(),
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
        val body = RequestBody(cmd = action.wire, delay = delaySeconds, force = force)
        return when (val r = exchange(host, agent, body)) {
            is AgentResult.Success -> AgentResult.Success(r.value.message)
            is AgentResult.Failure -> r
        }
    }

    private suspend fun exchange(host: String, agent: AgentSettings, request: RequestBody): AgentResult<ResponseBody> {
        val key = AgentKey.decodeOrNull(agent.key) ?: return AgentResult.Failure(AgentError.NO_KEY)
        var connected = false
        return try {
            Socket().useCancellable { socket ->
                val address = InetSocketAddress(binder.resolve(host), agent.port)
                socket.connectBound(binder, address, connectTimeoutMs)
                connected = true
                socket.soTimeout = readTimeoutMs
                converse(socket, key, request)
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

    private fun converse(socket: Socket, key: ByteArray, request: RequestBody): AgentResult<ResponseBody> {
        val input = socket.getInputStream().buffered()
        val output = socket.getOutputStream()

        val hello = decode<HelloMessage>(readLine(input)) ?: return protocolError("salutation illisible")
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

        val response = decode<ResponseMessage>(readLine(input)) ?: return protocolError("réponse illisible")
        when (response.error) {
            null -> Unit
            "unauthorized" -> return AgentResult.Failure(AgentError.UNAUTHORIZED)
            "rate_limited" -> return AgentResult.Failure(AgentError.RATE_LIMITED)
            else -> return protocolError("erreur de l'agent : ${response.error}")
        }
        val responseBody = response.body ?: return protocolError("réponse sans contenu")
        val responseMac = response.mac ?: return protocolError("réponse non signée")
        val expected = AgentProtocol.responseMac(key, hello.nonce, cnonce, responseBody)
        if (!AgentProtocol.macEquals(expected, responseMac)) return protocolError("signature de la réponse invalide")

        val parsed = decode<ResponseBody>(responseBody) ?: return protocolError("contenu de réponse illisible")
        return if (parsed.ok) AgentResult.Success(parsed) else AgentResult.Failure(AgentError.REJECTED, parsed.message)
    }

    private inline fun <reified T> decode(text: String?): T? =
        text?.let { runCatching { json.decodeFromString<T>(it) }.getOrNull() }

    private fun protocolError(detail: String) = AgentResult.Failure(AgentError.PROTOCOL, detail)

    /** Lit une ligne terminée par `\n`, de taille bornée (protection contre un pair malveillant). */
    private fun readLine(input: InputStream): String? {
        val buffer = ByteArrayOutputStream()
        while (true) {
            val b = input.read()
            if (b == -1) return if (buffer.size() == 0) null else buffer.toString(Charsets.UTF_8.name())
            if (b == '\n'.code) return buffer.toString(Charsets.UTF_8.name()).trimEnd('\r')
            if (buffer.size() >= AgentProtocol.MAX_LINE_BYTES) throw IOException("message trop long")
            buffer.write(b)
        }
    }
}
