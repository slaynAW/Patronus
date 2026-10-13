package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent2.DeviceIdentity
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.File
import java.math.BigInteger
import java.security.KeyFactory
import java.security.KeyPair
import java.security.KeyPairGenerator
import java.security.Signature
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.security.spec.ECGenParameterSpec
import java.security.spec.PKCS8EncodedKeySpec
import java.security.spec.X509EncodedKeySpec
import java.util.Base64

/**
 * Identités wolagent/2 pour les tests sur la JVM. Sur Android, le Keystore crée la clé et son certificat ; ici, un
 * certificat autosigné minimal est écrit à la main (l'agent ne regarde que la clé).
 */
object TestIdentities {
    fun newDevice(): DeviceIdentity {
        val pair = KeyPairGenerator.getInstance("EC").apply { initialize(ECGenParameterSpec("secp256r1")) }.generateKeyPair()
        return DeviceIdentity(pair.private, selfSigned(pair))
    }

    /**
     * Identité de l'agent lui-même, lue dans son fichier identity.key (PKCS#8 en PEM, Linux et macOS) : c'est celle
     * de wol-agent sur le PC, acceptée comme administrateur depuis l'adresse de bouclage.
     */
    fun agent(identityKey: File): DeviceIdentity {
        val der = Base64.getMimeDecoder().decode(
            identityKey.readText().lines().filterNot { it.startsWith("-----") }.joinToString(""),
        )
        val privateKey = KeyFactory.getInstance("EC").generatePrivate(PKCS8EncodedKeySpec(der))
        // Clé publique : champ [1] de l'ECPrivateKey, dans le PKCS#8 écrit par Go.
        val pkcs8 = children(der, 0)
        val ecPrivateKey = children(der, pkcs8[2].start)
        val publicKey = ecPrivateKey.first { it.tag == 0xA1 }
        val bitString = der.copyOfRange(publicKey.start, publicKey.end)
        val point = bitString.copyOfRange(bitString.size - 65, bitString.size)
        val spki = P256_SPKI_PREFIX + point.copyOfRange(1, 65)
        val pub = KeyFactory.getInstance("EC").generatePublic(X509EncodedKeySpec(spki))
        return DeviceIdentity(privateKey, selfSigned(KeyPair(pub, privateKey)))
    }

    private val P256_SPKI_PREFIX = hexBytes("3059301306072a8648ce3d020106082a8648ce3d030107034200" + "04")

    /** Certificat X.509 v3 autosigné minimal : sujet CN=wolagent, ECDSA avec SHA-256, sans extension. */
    private fun selfSigned(pair: KeyPair): X509Certificate {
        val algorithm = tlv(0x30, oid(1, 2, 840, 10045, 4, 3, 2))
        val name = tlv(0x30, tlv(0x31, tlv(0x30, oid(2, 5, 4, 3) + tlv(0x0C, "wolagent".toByteArray()))))
        val validity = tlv(0x30, tlv(0x17, "250101000000Z".toByteArray()) + tlv(0x18, "99991231235959Z".toByteArray()))
        val serial = tlv(0x02, BigInteger(64, java.security.SecureRandom()).add(BigInteger.ONE).toByteArray())
        val tbs = tlv(0x30, tlv(0xA0, tlv(0x02, byteArrayOf(2))) + serial + algorithm + name + validity + name + pair.public.encoded)
        val signature = Signature.getInstance("SHA256withECDSA").apply {
            initSign(pair.private)
            update(tbs)
        }.sign()
        val der = tlv(0x30, tbs + algorithm + tlv(0x03, byteArrayOf(0) + signature))
        return CertificateFactory.getInstance("X.509").generateCertificate(ByteArrayInputStream(der)) as X509Certificate
    }

    private fun tlv(tag: Int, content: ByteArray): ByteArray {
        val out = ByteArrayOutputStream()
        out.write(tag)
        when {
            content.size < 0x80 -> out.write(content.size)
            content.size < 0x100 -> out.write(byteArrayOf(0x81.toByte(), content.size.toByte()))
            else -> out.write(byteArrayOf(0x82.toByte(), (content.size shr 8).toByte(), content.size.toByte()))
        }
        out.write(content)
        return out.toByteArray()
    }

    private fun oid(vararg arcs: Long): ByteArray {
        val out = ByteArrayOutputStream()
        out.write((arcs[0] * 40 + arcs[1]).toInt())
        for (arc in arcs.drop(2)) {
            val groups = mutableListOf<Int>()
            var v = arc
            do {
                groups.add(0, (v and 0x7F).toInt())
                v = v shr 7
            } while (v > 0)
            groups.forEachIndexed { i, g -> out.write(if (i < groups.size - 1) g or 0x80 else g) }
        }
        return tlv(0x06, out.toByteArray())
    }

    private class Element(val tag: Int, val start: Int, val end: Int)

    /** Éléments DER contenus dans celui qui commence à [at]. */
    private fun children(der: ByteArray, at: Int): List<Element> {
        val parent = element(der, at)
        val out = mutableListOf<Element>()
        var pos = parent.start
        while (pos < parent.end) {
            val child = element(der, pos)
            out += child
            pos = child.end
        }
        return out
    }

    private fun element(der: ByteArray, at: Int): Element {
        var pos = at
        val tag = der[pos++].toInt() and 0xFF
        var length = der[pos++].toInt() and 0xFF
        if (length and 0x80 != 0) {
            val count = length and 0x7F
            length = 0
            repeat(count) { length = (length shl 8) or (der[pos++].toInt() and 0xFF) }
        }
        return Element(tag, pos, pos + length)
    }

    private fun hexBytes(text: String) = ByteArray(text.length / 2) { text.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
}
