package io.github.slaynaw.wakeonlan.core.agent

import io.github.slaynaw.wakeonlan.core.agent2.Commands
import io.github.slaynaw.wakeonlan.core.agent2.DeviceIdentity
import io.github.slaynaw.wakeonlan.core.agent2.PairingCode
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.agent2.Request2
import io.github.slaynaw.wakeonlan.core.agent2.Response2
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Client
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Exception
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Session
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.net.NetworkBinder
import io.github.slaynaw.wakeonlan.core.net.connectBound
import io.github.slaynaw.wakeonlan.core.net.useCancellable
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import java.io.ByteArrayOutputStream
import java.io.Closeable
import java.io.IOException
import java.io.InputStream
import java.net.ConnectException
import java.net.InetSocketAddress
import java.net.Socket
import java.net.UnknownHostException
import java.security.SecureRandom

/** Causes d'échec d'un échange avec l'agent. */
enum class AgentError {
    /** Ni clé ni empreinte configurée pour ce PC. */
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

    /** Cet appareil n'est pas (ou plus) autorisé par l'agent (wolagent/2) : il faut l'appairer. */
    UNREGISTERED,

    /**
     * L'agent ne présente pas l'empreinte enregistrée (autre machine à cette adresse, agent réinstallé, ou
     * interception) : connexion refusée avant tout envoi.
     */
    PIN_MISMATCH,

    /** L'identité wolagent/2 de cet appareil est indisponible (Android 9 ou moins, Keystore en panne). */
    NO_IDENTITY,

    /** Code d'appairage refusé par l'agent (invalide, expiré ou déjà utilisé : la même réponse dans tous les cas). */
    PAIRING_FAILED,
}

/** Résultat d'un appairage avec un code : rôle donné par l'agent ; [already] si cet appareil était déjà enregistré. */
data class AgentPairing(val role: String, val already: Boolean = false)

sealed interface AgentResult<out T> {
    data class Success<T>(val value: T) : AgentResult<T>
    /** [code] : code donné par l'agent pour une commande refusée (« forbidden », « busy »…). */
    data class Failure(val error: AgentError, val detail: String? = null, val code: String? = null) : AgentResult<Nothing>

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
            AgentError.UNREGISTERED,
            AgentError.PIN_MISMATCH,
            AgentError.PAIRING_FAILED,
        )
    }
}

/**
 * Client des agents. Il parle « wolagent/2 » (TLS 1.3, identités épinglées : docs/PROTOCOLE-V2.md) aux PC dont il
 * connaît l'empreinte, sans jamais revenir à « wolagent/1 » avec eux, et « wolagent/1 » (clé partagée : voir
 * [AgentProtocol]) aux autres. [deviceName] : nom de cet appareil, noté par l'agent (1.4.0 ou plus) avec chaque
 * demande wolagent/1, et enregistré par l'agent à la migration. [identity] : identité wolagent/2 de cet appareil
 * (null : wolagent/2 indisponible) ; elle peut être lente à créer la première fois (appelée hors du fil principal).
 */
class AgentClient(
    private val binder: NetworkBinder = NetworkBinder.Default,
    private val connectTimeoutMs: Int = 2_000,
    private val readTimeoutMs: Int = 4_000,
    private val random: SecureRandom = SecureRandom(),
    private val deviceName: () -> String? = { null },
    private val identity: () -> DeviceIdentity? = { null },
) {
    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
    private val v2Client = Wolagent2Client(binder, connectTimeoutMs, readTimeoutMs)

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
     * Lit la fiche du PC (processeur, mémoire, cartes graphiques, carte mère, système). Un agent
     * antérieur à 1.10.0 répond [AgentError.REJECTED] ; un agent qui la lit encore aussi, avec le code
     * « busy » (à redemander).
     */
    suspend fun specs(host: String, agent: AgentSettings): AgentResult<AgentSpecs> =
        when (val r = exchange(host, agent, RequestBody(cmd = "specs"), AgentProtocol.MAX_SPECS_BYTES)) {
            is AgentResult.Success ->
                r.value.specs?.let { AgentResult.Success(it) } ?: protocolError("fiche absente de la réponse")
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

    /**
     * Enregistre cet appareil auprès de l'agent (docs/PROTOCOLE-V2.md, §9), sur une session wolagent/2 où
     * l'empreinte [AgentSettings.pin] est exigée ; la clé wolagent/1 [AgentSettings.key] prouve ses droits. L'agent
     * prouve d'abord qu'il connaît la clé : sinon, rien d'autre n'est envoyé. Renvoie le rôle donné par l'agent.
     */
    suspend fun migrate(host: String, agent: AgentSettings): AgentResult<String> {
        val key = AgentKey.decodeOrNull(agent.key)
        if (key == null || !agent.hasPin) return AgentResult.Failure(AgentError.NO_KEY)
        return session(host, agent) { session ->
            val response = session.migrate(key, name() ?: "Android")
            when (val body = fromV2(response)) {
                is AgentResult.Success -> AgentResult.Success(response.role)
                is AgentResult.Failure -> body
            }
        }
    }

    /**
     * Enregistre cet appareil auprès de l'agent avec un code d'appairage (docs/PROTOCOLE-V2.md, §6.3 et §7.3), sur une
     * session où l'empreinte de l'agent lue dans le lien, [AgentSettings.pin], est exigée : le code n'est envoyé qu'à cet
     * agent. Un code refusé donne [AgentError.PAIRING_FAILED] ; un appareil déjà enregistré, [AgentPairing.already].
     */
    suspend fun pair(host: String, agent: AgentSettings, code: String): AgentResult<AgentPairing> {
        val normalized = PairingCode.normalize(code)
        if (normalized == null || !agent.hasPin) {
            return AgentResult.Failure(AgentError.PAIRING_FAILED, "code d'appairage ou empreinte de l'agent invalide")
        }
        return session(host, agent) { session ->
            val response = session.request(Request2(cmd = Commands.PAIR, code = normalized, name = name() ?: "Android"))
            when {
                !response.ok && response.code == ResponseCode.PAIRING_FAILED ->
                    AgentResult.Failure(AgentError.PAIRING_FAILED, response.message)
                !response.ok && response.code == ResponseCode.ALREADY_PAIRED ->
                    AgentResult.Success(AgentPairing(response.role, already = true))
                else -> when (val body = fromV2(response)) {
                    is AgentResult.Success -> AgentResult.Success(AgentPairing(response.role))
                    is AgentResult.Failure -> body
                }
            }
        }
    }

    private fun name(): String? = deviceName()?.trim()?.take(AgentProtocol.MAX_BY_LENGTH)?.takeIf { it.isNotEmpty() }

    private suspend fun exchange(
        host: String,
        agent: AgentSettings,
        request: RequestBody,
        maxResponseBytes: Int = AgentProtocol.MAX_LINE_BYTES,
    ): AgentResult<ResponseBody> {
        if (agent.hasPin) {
            // Empreinte connue : wolagent/2 uniquement, jamais de retour à wolagent/1 (§9, R8).
            return session(host, agent) { session ->
                fromV2(
                    session.request(
                        Request2(cmd = request.cmd, delay = request.delay, force = request.force, wakes = request.wakes, day = request.day),
                    ),
                )
            }
        }
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

    /**
     * Ouvre une session wolagent/2 avec l'agent (empreinte [AgentSettings.pin] exigée pendant la poignée de main TLS),
     * y mène [run], puis la ferme. Annulable à tout moment : la connexion est alors fermée.
     */
    private suspend fun <T> session(host: String, agent: AgentSettings, run: (Wolagent2Session) -> AgentResult<T>): AgentResult<T> {
        val pin = Pin.parse(agent.pin) ?: return protocolError("empreinte enregistrée invalide")
        val id = withContext(Dispatchers.IO) { runCatching { identity() }.getOrNull() }
            ?: return AgentResult.Failure(AgentError.NO_IDENTITY)
        val call = SessionCall()
        return try {
            call.useCancellable { c ->
                v2Client.connect(host, agent.port, id, pin, onSocket = c::attach).use(run)
            }
        } catch (e: Wolagent2Exception) {
            when (e.reason) {
                Wolagent2Exception.Reason.PIN_MISMATCH -> AgentResult.Failure(AgentError.PIN_MISMATCH, e.message)
                Wolagent2Exception.Reason.RATE_LIMITED -> AgentResult.Failure(AgentError.RATE_LIMITED, e.message)
                Wolagent2Exception.Reason.MIGRATION_PROOF ->
                    protocolError("preuve de l'agent invalide (clé changée, ou interception)")
                else -> AgentResult.Failure(AgentError.PROTOCOL, e.message)
            }
        } catch (e: UnknownHostException) {
            AgentResult.Failure(AgentError.UNKNOWN_HOST, e.message)
        } catch (e: ConnectException) {
            val refused = e.message?.contains("refused", ignoreCase = true) == true
            AgentResult.Failure(if (refused) AgentError.REFUSED else AgentError.UNREACHABLE, e.message)
        } catch (e: IOException) {
            // Connexion établie puis refus de TLS, coupure ou silence : la machine est allumée.
            AgentResult.Failure(if (call.connected) AgentError.PROTOCOL else AgentError.UNREACHABLE, e.message)
        }
    }

    /** Connexion d'une session wolagent/2 en cours, fermée si l'appel est annulé. */
    private class SessionCall : Closeable {
        @Volatile private var socket: Socket? = null

        @Volatile private var closed = false

        val connected: Boolean get() = socket?.isConnected == true

        fun attach(s: Socket) {
            socket = s
            if (closed) runCatching { s.close() }
        }

        override fun close() {
            closed = true
            socket?.let { runCatching { it.close() } }
        }
    }

    /** Traduit une réponse wolagent/2 dans la forme commune aux deux protocoles. */
    private fun fromV2(r: Response2): AgentResult<ResponseBody> = when {
        r.code.isEmpty() -> protocolError("réponse sans code")
        !r.ok && r.code == ResponseCode.UNKNOWN_DEVICE -> AgentResult.Failure(AgentError.UNREGISTERED, r.message)
        !r.ok -> AgentResult.Failure(AgentError.REJECTED, r.message, r.code)
        else -> AgentResult.Success(
            ResponseBody(
                ok = true, code = r.code, message = r.message, hostname = r.hostname, os = r.os, arch = r.arch,
                version = r.version, uptime = r.uptime, history = r.history, temperatures = r.temperatures,
                metrics = r.metrics, disks = r.disks, specs = r.specs, protocols = r.protocols,
                proto = Wolagent2.ALPN, role = r.role,
            ),
        )
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
        return if (parsed.ok) AgentResult.Success(parsed) else AgentResult.Failure(AgentError.REJECTED, parsed.message, parsed.code)
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
