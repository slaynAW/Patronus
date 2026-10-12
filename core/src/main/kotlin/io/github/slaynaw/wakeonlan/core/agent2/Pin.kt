package io.github.slaynaw.wakeonlan.core.agent2

import java.io.ByteArrayInputStream
import java.math.BigInteger
import java.security.MessageDigest
import java.security.cert.CertificateFactory

/** Refus d'un certificat présenté dans TLS (§2.2). */
class CertificateRejectedException(val reason: Reason) : Exception(
    when (reason) {
        Reason.CERTIFICATE -> "certificat illisible"
        Reason.KEY_TYPE -> "clé autre que ECDSA P-256"
    },
) {
    enum class Reason { CERTIFICATE, KEY_TYPE }
}

/**
 * Empreinte d'une identité (§2.1) : SHA-256 de sa clé publique (SubjectPublicKeyInfo en DER), soit la valeur
 * `pin-sha256` de la RFC 7469. Écrite en B64 (43 caractères) ; empreinte courte pour la comparaison à l'œil.
 */
class Pin private constructor(private val bytes: ByteArray) {
    fun toBytes(): ByteArray = bytes.copyOf()

    /** Écriture B64, celle des messages, des liens et des fichiers. */
    override fun toString(): String = B64.encode(bytes)

    /** Empreinte courte : 60 premiers bits en base32 de Crockford, par groupes de quatre (« 7KQ2-9MXP-4D1Z »). */
    fun short(): String {
        var bits = 0L
        for (i in 0 until 8) bits = (bits shl 8) or (bytes[i].toLong() and 0xFF)
        bits = bits ushr 4
        val chars = CharArray(12) { i -> PairingCode.ALPHABET[((bits ushr (55 - 5 * i)) and 31).toInt()] }
        return String(chars, 0, 4) + "-" + String(chars, 4, 4) + "-" + String(chars, 8, 4)
    }

    override fun equals(other: Any?): Boolean = other is Pin && MessageDigest.isEqual(bytes, other.bytes)

    override fun hashCode(): Int = bytes.contentHashCode()

    companion object {
        const val SIZE = 32

        /** Début de tout SubjectPublicKeyInfo P-256 standard (courbe nommée, point non compressé). */
        private val P256_SPKI_PREFIX = hex("3059301306072a8648ce3d020106082a8648ce3d030107034200" + "04")
        private const val P256_SPKI_LENGTH = 91
        private val P = BigInteger("ffffffff00000001000000000000000000000000ffffffffffffffffffffffff", 16)
        private val B = BigInteger("5ac635d8aa3a93e7b3ebbd55769886bc651d06b0cc53b0f63bce3c3e27d2604b", 16)
        private val THREE = BigInteger.valueOf(3)

        /** Empreinte d'une clé publique encodée (SubjectPublicKeyInfo en DER). */
        fun of(spki: ByteArray): Pin = Pin(MessageDigest.getInstance("SHA-256").digest(spki))

        /** Lit une empreinte écrite en B64 ; null si elle est mal écrite. */
        fun parse(text: String): Pin? = B64.decodeExact(text, SIZE)?.let(::Pin)

        /**
         * Vérifie le certificat présenté par l'autre côté et renvoie son empreinte (§2.2). Seule la clé compte :
         * ECDSA P-256 sous sa forme standard, point sur la courbe. L'émetteur, les dates, les extensions et la
         * signature sont ignorés : la possession de la clé est prouvée par TLS.
         *
         * @throws CertificateRejectedException
         */
        fun fromCertificate(der: ByteArray): Pin {
            runCatching { CertificateFactory.getInstance("X.509").generateCertificate(ByteArrayInputStream(der)) }
                .onFailure { throw CertificateRejectedException(CertificateRejectedException.Reason.CERTIFICATE) }
            val spki = Der.subjectPublicKeyInfo(der)
                ?: throw CertificateRejectedException(CertificateRejectedException.Reason.CERTIFICATE)
            if (spki.size != P256_SPKI_LENGTH || !spki.copyOf(P256_SPKI_PREFIX.size).contentEquals(P256_SPKI_PREFIX)) {
                throw CertificateRejectedException(CertificateRejectedException.Reason.KEY_TYPE)
            }
            if (!onCurve(spki)) throw CertificateRejectedException(CertificateRejectedException.Reason.CERTIFICATE)
            return of(spki)
        }

        /** Le point de la clé est-il sur la courbe P-256 (y² = x³ − 3x + b mod p) ? */
        private fun onCurve(spki: ByteArray): Boolean {
            val x = BigInteger(1, spki.copyOfRange(27, 59))
            val y = BigInteger(1, spki.copyOfRange(59, 91))
            if (x >= P || y >= P) return false
            return y.multiply(y).mod(P) == x.pow(3).subtract(x.multiply(THREE)).add(B).mod(P)
        }

        private fun hex(text: String): ByteArray = ByteArray(text.length / 2) { text.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
    }
}

/** Lecture minimale du DER d'un certificat X.509, pour en extraire la clé telle qu'écrite. */
internal object Der {
    private class Element(val tag: Int, val start: Int, val end: Int)

    /** SubjectPublicKeyInfo du certificat, ou null si le DER est mal formé. */
    fun subjectPublicKeyInfo(der: ByteArray): ByteArray? = runCatching {
        val certificate = element(der, 0)
        require(certificate.tag == 0x30 && certificate.end == der.size)
        val tbs = element(der, certificate.start)
        require(tbs.tag == 0x30)
        var pos = tbs.start
        val first = element(der, pos)
        if (first.tag == 0xA0) pos = first.end // version
        repeat(5) { pos = element(der, pos).end } // numéro de série, signature, émetteur, validité, sujet
        val spki = element(der, pos)
        require(spki.tag == 0x30)
        der.copyOfRange(pos, spki.end)
    }.getOrNull()

    private fun element(data: ByteArray, at: Int): Element {
        var pos = at
        val tag = data[pos++].toInt() and 0xFF
        var length = data[pos++].toInt() and 0xFF
        if (length and 0x80 != 0) {
            val count = length and 0x7F
            require(count in 1..3)
            length = 0
            repeat(count) { length = (length shl 8) or (data[pos++].toInt() and 0xFF) }
        }
        require(pos + length <= data.size)
        return Element(tag, pos, pos + length)
    }
}
