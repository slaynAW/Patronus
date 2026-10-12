package io.github.slaynaw.wakeonlan.core.agent2

import io.github.slaynaw.wakeonlan.core.agent.AgentDisks
import io.github.slaynaw.wakeonlan.core.agent.AgentHistory
import io.github.slaynaw.wakeonlan.core.agent.AgentMetrics
import io.github.slaynaw.wakeonlan.core.agent.AgentSpecs
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.AgentTemperatures
import io.github.slaynaw.wakeonlan.core.net.NetworkBinder
import io.github.slaynaw.wakeonlan.core.net.connectBound
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.io.ByteArrayOutputStream
import java.io.Closeable
import java.io.IOException
import java.io.InputStream
import java.net.InetSocketAddress
import java.net.Socket
import java.security.Principal
import java.security.PrivateKey
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLEngine
import javax.net.ssl.SSLSocket
import javax.net.ssl.X509ExtendedKeyManager
import javax.net.ssl.X509ExtendedTrustManager

/** Identité de cet appareil (§2) : clé ECDSA P-256 (Keystore sur Android) et le certificat qui la présente. */
class DeviceIdentity(val privateKey: PrivateKey, val certificate: X509Certificate) {
    val pin: Pin = Pin.fromCertificate(certificate.encoded)
}

/** Requête (§4.2) ; son identifiant est attribué par la session. */
@Serializable
data class Request2(
    val id: Long = 0,
    val cmd: String,
    val delay: Int? = null,
    val force: Boolean? = null,
    val wakes: List<Long>? = null,
    val day: String? = null,
    val device: String? = null,
    val role: String? = null,
    val name: String? = null,
    val ttl: Long? = null,
    val invite: String? = null,
    val code: String? = null,
    val nonce: String? = null,
    val proof: String? = null,
)

/** Appareil enregistré sur un PC (commande devices). */
@Serializable
data class DeviceInfo(
    val device: String,
    val name: String,
    val role: String,
    val added: Long = 0,
    val via: String = "",
    val seen: Long = 0,
    val self: Boolean = false,
)

/** Invitation ; [code] et [agent] ne figurent que dans la réponse à invite. */
@Serializable
data class InviteInfo(
    val id: String,
    val code: String = "",
    val role: String,
    val name: String = "",
    val expires: Long = 0,
    val device: String = "",
    val agent: String = "",
)

/** Réponse (§4.3) : celle de wolagent/1, plus l'identifiant de la requête et les champs de wolagent/2. */
@Serializable
data class Response2(
    val id: Long = 0,
    val ok: Boolean = false,
    val code: String = "",
    val message: String = "",
    val hostname: String = "",
    val os: String = "",
    val arch: String = "",
    val version: String = "",
    val uptime: Long = 0,
    val history: AgentHistory? = null,
    val temperatures: AgentTemperatures? = null,
    val metrics: AgentMetrics? = null,
    val disks: AgentDisks? = null,
    val specs: AgentSpecs? = null,
    val protocols: List<String> = emptyList(),
    val role: String = "",
    val device: String = "",
    val devices: List<DeviceInfo>? = null,
    val invite: InviteInfo? = null,
    val invites: List<InviteInfo>? = null,
    val nonce: String = "",
    val proof: String = "",
) {
    fun toStatus() = AgentStatus(hostname, os, arch, version, uptime, temperatures, disks)
}

/** Échec d'une session wolagent/2. */
class Wolagent2Exception(val reason: Reason, message: String, cause: Throwable? = null) : IOException(message, cause) {
    enum class Reason {
        /** Ce qui répond n'est pas un agent Patronus. */
        NOT_AGENT,

        /** L'agent bloque cette adresse après trop d'échecs. */
        RATE_LIMITED,

        /** L'agent ne présente pas l'empreinte épinglée : rien n'a été envoyé. */
        PIN_MISMATCH,

        /** Preuve de l'agent fausse pendant la migration (autre clé, ou intermédiaire) : rien d'autre n'a été envoyé. */
        MIGRATION_PROOF,

        /** Empreinte de l'agent pas encore prouvée : seule la migration est permise. */
        UNPINNED,

        /** Échange interrompu ou réponse illisible. */
        PROTOCOL,
    }
}

/**
 * Client wolagent/2 (§3) : lecture du salut de l'agent, TLS 1.3 avec l'empreinte de l'agent vérifiée pendant la
 * poignée de main (avant tout envoi), puis requêtes et réponses en trames. Bloquant : à appeler hors du fil
 * principal.
 */
class Wolagent2Client(
    private val binder: NetworkBinder = NetworkBinder.Default,
    private val connectTimeoutMs: Int = 2_000,
    private val readTimeoutMs: Int = 6_000,
) {
    /**
     * Ouvre une session avec l'agent [host]:[port]. [agent] est l'empreinte épinglée ; null seulement pour une
     * migration, dont la preuve vérifiera l'empreinte vue.
     *
     * @throws Wolagent2Exception ou une [IOException] de connexion.
     */
    fun connect(host: String, port: Int, identity: DeviceIdentity, agent: Pin?): Wolagent2Session {
        val socket = Socket()
        try {
            socket.connectBound(binder, InetSocketAddress(binder.resolve(host), port), connectTimeoutMs)
            socket.soTimeout = readTimeoutMs
            readHello(socket.getInputStream())
            val trust = PinningTrustManager(agent)
            val context = SSLContext.getInstance("TLSv1.3")
            context.init(arrayOf(IdentityKeyManager(identity)), arrayOf(trust), null)
            val ssl = context.socketFactory.createSocket(socket, host, port, true) as SSLSocket
            ssl.sslParameters = ssl.sslParameters.apply {
                protocols = arrayOf("TLSv1.3")
                applicationProtocols = arrayOf(Wolagent2.ALPN)
                serverNames = emptyList() // pas de SNI : l'agent est désigné par son adresse
            }
            try {
                ssl.startHandshake()
            } catch (e: IOException) {
                if (trust.mismatch) {
                    throw Wolagent2Exception(Wolagent2Exception.Reason.PIN_MISMATCH, "l'empreinte de l'agent ne correspond pas à celle enregistrée", e)
                }
                throw e
            }
            if (ssl.applicationProtocol != Wolagent2.ALPN) {
                throw Wolagent2Exception(Wolagent2Exception.Reason.PROTOCOL, "protocole applicatif refusé")
            }
            val seen = trust.seen ?: throw Wolagent2Exception(Wolagent2Exception.Reason.PROTOCOL, "certificat de l'agent non vérifié")
            return Wolagent2Session(ssl, seen, identity.pin, pinned = agent != null)
        } catch (e: Throwable) {
            runCatching { socket.close() }
            throw e
        }
    }

    /** Lit la ligne de salut que l'agent envoie avant TLS (§3.1), sans rien lire au-delà. */
    private fun readHello(input: InputStream) {
        val line = ByteArrayOutputStream()
        while (true) {
            val b = input.read()
            if (b < 0) throw Wolagent2Exception(Wolagent2Exception.Reason.NOT_AGENT, "connexion fermée avant le salut")
            if (b == '\n'.code) break
            if (line.size() == Wolagent2.MAX_HELLO_BYTES) throw Wolagent2Exception(Wolagent2Exception.Reason.NOT_AGENT, "salut trop long")
            line.write(b)
        }
        val hello = runCatching { json.decodeFromString<Hello>(line.toString(Charsets.UTF_8.name())) }.getOrNull()
            ?: throw Wolagent2Exception(Wolagent2Exception.Reason.NOT_AGENT, "salut illisible")
        when {
            hello.error == ResponseCode.RATE_LIMITED ->
                throw Wolagent2Exception(Wolagent2Exception.Reason.RATE_LIMITED, "trop d'échecs depuis cette adresse")
            hello.error.isNotEmpty() || hello.proto.isEmpty() ->
                throw Wolagent2Exception(Wolagent2Exception.Reason.NOT_AGENT, "salut inattendu")
        }
    }

    @Serializable
    private data class Hello(val proto: String = "", val error: String = "")

    /** Vérifie le certificat de l'agent (§2.2) et son empreinte, pendant la poignée de main. */
    private class PinningTrustManager(private val expected: Pin?) : X509ExtendedTrustManager() {
        @Volatile var seen: Pin? = null

        @Volatile var mismatch = false

        private fun check(chain: Array<X509Certificate>) {
            if (chain.size != 1) throw CertificateException("l'agent doit présenter un seul certificat")
            val pin = try {
                Pin.fromCertificate(chain[0].encoded)
            } catch (e: CertificateRejectedException) {
                throw CertificateException(e.message, e)
            }
            if (expected != null && pin != expected) {
                mismatch = true
                throw CertificateException("empreinte de l'agent différente de celle enregistrée")
            }
            seen = pin
        }

        override fun checkServerTrusted(chain: Array<X509Certificate>, authType: String) = check(chain)

        override fun checkServerTrusted(chain: Array<X509Certificate>, authType: String, socket: Socket?) = check(chain)

        override fun checkServerTrusted(chain: Array<X509Certificate>, authType: String, engine: SSLEngine?) = check(chain)

        override fun checkClientTrusted(chain: Array<X509Certificate>, authType: String) = throw CertificateException("client")

        override fun checkClientTrusted(chain: Array<X509Certificate>, authType: String, socket: Socket?) = throw CertificateException("client")

        override fun checkClientTrusted(chain: Array<X509Certificate>, authType: String, engine: SSLEngine?) = throw CertificateException("client")

        override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
    }

    /** Présente l'identité de l'appareil, demandée par l'agent (certificat client obligatoire). */
    private class IdentityKeyManager(private val identity: DeviceIdentity) : X509ExtendedKeyManager() {
        override fun chooseClientAlias(keyType: Array<String>?, issuers: Array<Principal>?, socket: Socket?) = ALIAS

        override fun chooseEngineClientAlias(keyType: Array<String>?, issuers: Array<Principal>?, engine: SSLEngine?) = ALIAS

        override fun getClientAliases(keyType: String?, issuers: Array<Principal>?) = arrayOf(ALIAS)

        override fun getCertificateChain(alias: String?): Array<X509Certificate> = arrayOf(identity.certificate)

        override fun getPrivateKey(alias: String?): PrivateKey = identity.privateKey

        override fun chooseServerAlias(keyType: String?, issuers: Array<Principal>?, socket: Socket?): String? = null

        override fun getServerAliases(keyType: String?, issuers: Array<Principal>?): Array<String>? = null

        private companion object {
            const val ALIAS = "wolagent2"
        }
    }

    internal companion object {
        val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
    }
}

/** Session wolagent/2 établie : une requête à la fois (§4.3). */
class Wolagent2Session internal constructor(
    private val socket: SSLSocket,
    /** Empreinte de l'agent vue dans la poignée de main (prouvée si la session a été ouverte avec elle). */
    val agentPin: Pin,
    /** Empreinte de cet appareil. */
    val devicePin: Pin,
    private var pinned: Boolean,
) : Closeable {
    private var lastId = 0L
    private var broken = false

    /** Envoie une requête et lit sa réponse. */
    @Synchronized
    fun request(request: Request2): Response2 {
        if (!pinned && request.cmd != Commands.MIGRATE) {
            throw Wolagent2Exception(Wolagent2Exception.Reason.UNPINNED, "empreinte de l'agent non vérifiée : seule la migration est permise")
        }
        return exchange(request)
    }

    /**
     * Fait passer en wolagent/2 un appareil appairé en wolagent/1 (§9), sur une session ouverte sans empreinte ;
     * [key] est la clé wolagent/1 du PC. La preuve de l'agent est vérifiée avant d'envoyer celle de l'appareil : si
     * elle est fausse, la session est fermée sans rien envoyer d'autre. En cas de succès, [agentPin] est prouvée.
     */
    @Synchronized
    fun migrate(key: ByteArray, name: String): Response2 {
        val nc = Migration.newNonce()
        val first = exchange(Request2(cmd = Commands.MIGRATE, nonce = Migration.encode(nc), name = name))
        if (!first.ok) return first
        val na = Migration.decode(first.nonce)
        val pa = Migration.decode(first.proof)
        if (na == null || pa == null || !Migration.proofsEqual(Migration.agentProof(key, agentPin, devicePin, nc, na), pa)) {
            fail()
            throw Wolagent2Exception(Wolagent2Exception.Reason.MIGRATION_PROOF, "preuve de l'agent invalide : migration refusée")
        }
        pinned = true
        val pd = Migration.deviceProof(key, agentPin, devicePin, nc, na)
        return exchange(Request2(cmd = Commands.MIGRATE, proof = Migration.encode(pd)))
    }

    private fun exchange(request: Request2): Response2 {
        if (broken) throw Wolagent2Exception(Wolagent2Exception.Reason.PROTOCOL, "session interrompue")
        val sent = request.copy(id = ++lastId)
        try {
            val payload = Wolagent2Client.json.encodeToString(Request2.serializer(), sent).toByteArray(Charsets.UTF_8)
            Frames.write(socket.outputStream, payload, Wolagent2.MAX_REQUEST_BYTES)
            val raw = Frames.read(socket.inputStream, Wolagent2.MAX_RESPONSE_BYTES)
                ?: throw Wolagent2Exception(Wolagent2Exception.Reason.PROTOCOL, "connexion fermée par l'agent")
            val response = runCatching { Wolagent2Client.json.decodeFromString(Response2.serializer(), raw.toString(Charsets.UTF_8)) }
                .getOrElse { throw Wolagent2Exception(Wolagent2Exception.Reason.PROTOCOL, "réponse illisible", it) }
            if (response.id != sent.id) throw Wolagent2Exception(Wolagent2Exception.Reason.PROTOCOL, "réponse à une autre requête")
            return response
        } catch (e: IOException) {
            fail()
            throw e
        }
    }

    private fun fail() {
        broken = true
        close()
    }

    override fun close() {
        runCatching { socket.close() }
    }
}
