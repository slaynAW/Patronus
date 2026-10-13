package io.github.slaynaw.wakeonlan.core.agent2

import java.security.MessageDigest
import java.security.SecureRandom
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/**
 * Preuves de la migration depuis `wolagent/1` (§9) :
 * HMAC-SHA256(K, libellé ‖ 0x00 ‖ empreinte(agent) ‖ empreinte(appareil) ‖ Nc ‖ Na). Chaque côté y met les empreintes
 * vues dans sa propre poignée de main TLS : un intermédiaire, qui présente sa propre identité de chaque côté, ne peut
 * pas obtenir des preuves qui concordent. Les libellés, distincts l'un de l'autre, commencent par « wolagent/2 » ;
 * les messages signés en wolagent/1, par « wolagent/1 » suivi d'un retour à la ligne.
 */
object Migration {
    private const val AGENT_LABEL = "wolagent/2 migrate agent"
    private const val DEVICE_LABEL = "wolagent/2 migrate device"

    fun newNonce(random: SecureRandom = SecureRandom()): ByteArray = ByteArray(Wolagent2.NONCE_BYTES).also(random::nextBytes)

    /** Preuve de l'agent, Pa, à vérifier avant d'envoyer quoi que ce soit d'autre. */
    fun agentProof(key: ByteArray, agent: Pin, device: Pin, nc: ByteArray, na: ByteArray): ByteArray =
        proof(key, AGENT_LABEL, agent, device, nc, na)

    /** Preuve de l'appareil, Pd, envoyée une fois celle de l'agent vérifiée. */
    fun deviceProof(key: ByteArray, agent: Pin, device: Pin, nc: ByteArray, na: ByteArray): ByteArray =
        proof(key, DEVICE_LABEL, agent, device, nc, na)

    /** Comparaison en temps constant. */
    fun proofsEqual(a: ByteArray, b: ByteArray): Boolean = MessageDigest.isEqual(a, b)

    /** Lit un nonce ou une preuve écrits en B64 (32 octets) ; null s'ils sont mal écrits. */
    fun decode(text: String): ByteArray? = B64.decodeExact(text, Wolagent2.NONCE_BYTES)

    fun encode(bytes: ByteArray): String = B64.encode(bytes)

    private fun proof(key: ByteArray, label: String, agent: Pin, device: Pin, nc: ByteArray, na: ByteArray): ByteArray {
        require(nc.size == Wolagent2.NONCE_BYTES && na.size == Wolagent2.NONCE_BYTES) { "nonce de 32 octets attendu" }
        val mac = Mac.getInstance("HmacSHA256")
        mac.init(SecretKeySpec(key, "HmacSHA256"))
        mac.update(label.toByteArray(Charsets.US_ASCII))
        mac.update(0)
        mac.update(agent.toBytes())
        mac.update(device.toBytes())
        mac.update(nc)
        mac.update(na)
        return mac.doFinal()
    }
}
