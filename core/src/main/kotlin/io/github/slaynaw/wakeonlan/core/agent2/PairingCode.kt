package io.github.slaynaw.wakeonlan.core.agent2

import java.security.SecureRandom

/** Code d'appairage (§7.2) : 8 caractères base32 de Crockford, soit 40 bits, affiché « 7KQ2-9MXP ». */
object PairingCode {
    const val LENGTH = 8
    const val ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

    fun generate(random: SecureRandom = SecureRandom()): String = encode(ByteArray(5).also(random::nextBytes))

    /** Écrit 40 bits en 8 caractères, bits de poids fort d'abord. */
    fun encode(bytes: ByteArray): String {
        require(bytes.size == 5)
        var bits = 0L
        for (b in bytes) bits = (bits shl 8) or (b.toLong() and 0xFF)
        return String(CharArray(LENGTH) { i -> ALPHABET[((bits ushr (35 - 5 * i)) and 31).toInt()] })
    }

    /**
     * Lit un code saisi ou lu dans un lien : espaces et tirets retirés, lettres ASCII en majuscules, O lu 0, I et L
     * lus 1 ; il doit rester exactement 8 caractères de l'alphabet. Aucun caractère non ASCII n'est transformé (un
     * « ı » ne devient pas « I ») : il rend le code invalide. Null si le code est invalide.
     */
    fun normalize(input: String): String? {
        val out = StringBuilder(LENGTH)
        for (c in input) {
            if (c == ' ' || c == '-') continue
            if (c.code > 0x7F) return null
            val upper = if (c in 'a'..'z') c - ('a' - 'A') else c
            val read = when (upper) {
                'O' -> '0'
                'I', 'L' -> '1'
                else -> upper
            }
            if (out.length == LENGTH || ALPHABET.indexOf(read) < 0) return null
            out.append(read)
        }
        return out.toString().takeIf { it.length == LENGTH }
    }

    /** Affiche un code normalisé en deux groupes. */
    fun format(code: String): String = if (code.length == LENGTH) code.substring(0, 4) + "-" + code.substring(4) else code
}
