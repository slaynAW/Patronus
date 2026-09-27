package io.github.slaynaw.wakeonlan.core.agent

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.security.MessageDigest
import java.util.Base64
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/**
 * Protocole « wolagent/1 » entre l'application et l'agent installé sur le PC.
 * Spécification complète : docs/PROTOCOLE.md. Vecteurs de test partagés : protocol/test-vectors.json.
 *
 * Résumé (TCP, une ligne JSON par message) :
 * 1. agent → app : `{"proto":"wolagent/1","nonce":"<32 octets aléatoires>"}`
 * 2. app → agent : `{"cnonce":"<16 octets>","body":"<JSON>","mac":"<HMAC>"}`
 * 3. agent → app : `{"body":"<JSON>","mac":"<HMAC>"}` ou `{"error":"unauthorized"}`
 *
 * Chaque message est authentifié par HMAC-SHA256 avec la clé partagée, et lié aux deux nonces :
 * un message intercepté ne peut ni être rejoué ni être modifié. La clé ne transite jamais.
 */
object AgentProtocol {
    const val PROTO = "wolagent/1"
    const val SERVER_NONCE_BYTES = 32
    const val CLIENT_NONCE_BYTES = 16
    const val MAX_LINE_BYTES = 8 * 1024

    private val b64 = Base64.getUrlEncoder().withoutPadding()
    private val b64Decoder = Base64.getUrlDecoder()

    fun requestMac(key: ByteArray, nonce: String, cnonce: String, body: String): String =
        hmac(key, "$PROTO\nrequest\n$nonce\n$cnonce\n$body")

    fun responseMac(key: ByteArray, nonce: String, cnonce: String, body: String): String =
        hmac(key, "$PROTO\nresponse\n$nonce\n$cnonce\n$body")

    /** Comparaison en temps constant (évite les attaques par mesure du temps de réponse). */
    fun macEquals(expected: String, received: String): Boolean {
        val a = runCatching { b64Decoder.decode(expected) }.getOrNull() ?: return false
        val b = runCatching { b64Decoder.decode(received) }.getOrNull() ?: return false
        return MessageDigest.isEqual(a, b)
    }

    fun encodeNonce(bytes: ByteArray): String = b64.encodeToString(bytes)

    fun decodedLength(value: String): Int = runCatching { b64Decoder.decode(value).size }.getOrDefault(-1)

    private fun hmac(key: ByteArray, message: String): String {
        val mac = Mac.getInstance("HmacSHA256")
        mac.init(SecretKeySpec(key, "HmacSHA256"))
        return b64.encodeToString(mac.doFinal(message.toByteArray(Charsets.UTF_8)))
    }
}

/** Actions d'alimentation que l'agent sait exécuter. */
enum class PowerAction(val wire: String) {
    SHUTDOWN("shutdown"),
    REBOOT("reboot"),
    SLEEP("sleep"),
}

/** Informations renvoyées par l'agent (commande `status`). */
@Serializable
data class AgentStatus(
    val hostname: String = "",
    val os: String = "",
    val arch: String = "",
    val version: String = "",
    @SerialName("uptime") val uptimeSeconds: Long = 0,
)

@Serializable
internal data class HelloMessage(val proto: String, val nonce: String)

@Serializable
internal data class RequestBody(
    val cmd: String,
    val delay: Int? = null,
    val force: Boolean? = null,
)

@Serializable
internal data class RequestMessage(val cnonce: String, val body: String, val mac: String)

@Serializable
internal data class ResponseMessage(
    val body: String? = null,
    val mac: String? = null,
    val error: String? = null,
)

@Serializable
internal data class ResponseBody(
    val ok: Boolean,
    val code: String,
    val message: String = "",
    val hostname: String = "",
    val os: String = "",
    val arch: String = "",
    val version: String = "",
    val uptime: Long = 0,
) {
    fun toStatus() = AgentStatus(hostname, os, arch, version, uptime)
}
