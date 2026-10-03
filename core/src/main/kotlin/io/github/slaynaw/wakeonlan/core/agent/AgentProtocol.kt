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

    /** Taille maximale de la réponse à la commande `history` (journal de 30 jours). */
    const val MAX_HISTORY_BYTES = 512 * 1024

    /** Démarrages signalés en une fois (commande « wakes »). */
    const val MAX_WAKES = 50

    /** Longueur maximale du nom de l'appareil noté par l'agent. */
    const val MAX_BY_LENGTH = 40

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
    /** Températures du PC (agent 1.5.0 ou plus ; `null` : aucun capteur lisible). */
    val temperatures: AgentTemperatures? = null,
)

/**
 * Températures du PC en °C et utilisation du processeur et de la carte graphique en % ([cpuLoad],
 * [gpuLoad] : mesurées comme le Gestionnaire des tâches, agent 1.7.0) ; un capteur illisible est `null`. [cpuHint] explique l'absence de la
 * température du processeur : [CPU_HINT_LHM] = LibreHardwareMonitor ne la fournit pas (Windows), et
 * [lhm] précise pourquoi (agent 1.6.0 ou plus ; vide avec un agent plus ancien).
 */
@Serializable
data class AgentTemperatures(
    val cpu: Double? = null,
    val gpu: Double? = null,
    val cpuLoad: Double? = null,
    val gpuLoad: Double? = null,
    val gpuName: String = "",
    /** Carte graphique intégrée au processeur, sans sonde à part : [gpu] est la température de la puce (agent 1.6.1). */
    val gpuShared: Boolean = false,
    val cpuHint: String = "",
    val lhm: String = "",
) {
    companion object {
        const val CPU_HINT_LHM = "lhm"

        /** LibreHardwareMonitor ne tourne pas. */
        const val LHM_NOT_RUNNING = "not-running"

        /** Il tourne, mais son serveur web (Options → Remote Web Server → Run) est désactivé. */
        const val LHM_WEB_OFF = "web-off"

        /** Son serveur web demande un mot de passe. */
        const val LHM_AUTH = "auth"

        /** Il répond sans température du processeur (pilote PawnIO absent…). */
        const val LHM_NO_SENSOR = "no-sensor"

        /** Seuils d'affichage : chaud, puis très chaud. */
        const val WARM = 80.0
        const val HOT = 90.0
    }
}

/**
 * Journal du PC renvoyé par la commande `history` : 30 jours au plus, heures en secondes (Unix).
 * [from] : début de la période couverte (installation de l'agent, ou 30 jours).
 */
@Serializable
data class AgentHistory(val from: Long = 0, val events: List<AgentHistoryEvent> = emptyList())

/**
 * Évènement du journal : [t] heure (s), [k] type (`boot`, `shutdown`, `lost`, `sleep`, `resume`,
 * `cmd`, `wake`), [a] action d'une commande reçue, [c] adresse de l'appareil à l'origine d'une
 * demande et [b] son nom (agent 1.4.0 ou plus).
 */
@Serializable
data class AgentHistoryEvent(val t: Long, val k: String, val a: String? = null, val c: String? = null, val b: String? = null)

@Serializable
internal data class HelloMessage(val proto: String, val nonce: String)

@Serializable
internal data class RequestBody(
    val cmd: String,
    val delay: Int? = null,
    val force: Boolean? = null,
    /** Nom de cet appareil, noté par l'agent avec la demande (ignoré avant l'agent 1.4.0). */
    val by: String? = null,
    /** Heures (s) des démarrages demandés, pour la commande « wakes ». */
    val wakes: List<Long>? = null,
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
    val history: AgentHistory? = null,
    val temperatures: AgentTemperatures? = null,
) {
    fun toStatus() = AgentStatus(hostname, os, arch, version, uptime, temperatures)
}
