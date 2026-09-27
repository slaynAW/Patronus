package io.github.slaynaw.wakeonlan.core.agent

import java.security.SecureRandom
import java.util.Base64

/**
 * Clé secrète partagée entre l'application et l'agent : 32 octets aléatoires (256 bits),
 * échangés en Base64 URL-safe sans remplissage (43 caractères).
 */
object AgentKey {
    const val SIZE_BYTES = 32

    private val encoder = Base64.getUrlEncoder().withoutPadding()
    private val decoder = Base64.getUrlDecoder()

    fun generate(random: SecureRandom = SecureRandom()): String =
        encoder.encodeToString(ByteArray(SIZE_BYTES).also(random::nextBytes))

    /** Décode une clé ; tolère les espaces et le remplissage `=` éventuels (copier-coller). */
    fun decodeOrNull(value: String): ByteArray? {
        val cleaned = value.filterNot { it.isWhitespace() }.trimEnd('=')
        val bytes = runCatching { decoder.decode(cleaned) }.getOrNull() ?: return null
        return bytes.takeIf { it.size == SIZE_BYTES }
    }

    fun decode(value: String): ByteArray =
        requireNotNull(decodeOrNull(value)) { "Clé d'agent invalide (32 octets Base64 attendus)" }

    fun encode(bytes: ByteArray): String = encoder.encodeToString(bytes)
}
