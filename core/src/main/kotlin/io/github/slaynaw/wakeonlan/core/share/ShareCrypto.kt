package io.github.slaynaw.wakeonlan.core.share

import io.github.slaynaw.wakeonlan.core.config.ConfigCodec
import io.github.slaynaw.wakeonlan.core.config.ConfigException
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.Device
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import java.io.IOException
import java.math.BigInteger
import java.security.GeneralSecurityException
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.MessageDigest
import java.security.PrivateKey
import java.security.PublicKey
import java.security.SecureRandom
import java.security.Signature
import java.security.interfaces.ECPrivateKey
import java.security.interfaces.ECPublicKey
import java.security.spec.ECFieldFp
import java.security.spec.ECGenParameterSpec
import java.security.spec.ECParameterSpec
import java.security.spec.ECPoint
import java.security.spec.ECPrivateKeySpec
import java.security.spec.ECPublicKeySpec
import java.security.spec.EllipticCurve
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.KeyAgreement
import javax.crypto.Mac
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/** Échec lié au partage, avec un message destiné à l'utilisateur. */
class ShareException(val reason: Reason, message: String, cause: Throwable? = null) : IOException(message, cause) {
    enum class Reason {
        /** Fichier d'accès ou lien mal formé. */
        INVALID,

        /** Fichier non signé par la personne qui partage. */
        SIGNATURE,

        /** Fichier destiné à un autre appareil. */
        RECIPIENT,

        /** Fichier plus ancien que celui déjà reçu. */
        OLDER,

        /** Espace de partage (Gist) introuvable. */
        NOT_FOUND,

        /** Connexion GitHub expirée ou refusée. */
        UNAUTHORIZED,

        /** Limite de requêtes de GitHub atteinte. */
        RATE_LIMITED,

        /** Connexion refusée par l'utilisateur, ou code expiré. */
        DENIED,

        /** Réseau indisponible, réponse inattendue. */
        NETWORK,
    }

    /**
     * Échec passager (Internet coupé, GitHub momentanément indisponible, réponse tronquée) : l'opération
     * peut être retentée. Les réponses refusées par GitHub (4xx) ne le sont pas.
     */
    val isTransient: Boolean get() = reason == Reason.NETWORK && cause != null
}

/** Paire de clés P-256 brute : scalaire privé (32 octets) et point public non compressé (65 octets). */
class ShareKeyPair(val privateKey: ByteArray, val publicKey: ByteArray) {
    val publicEncoded: String get() = ShareCrypto.encodeKey(publicKey)
    val privateEncoded: String get() = ShareCrypto.encodeKey(privateKey)

    override fun toString(): String = "ShareKeyPair(public=$publicEncoded, private=***)"
}

/** Ce que reçoit l'appareil autorisé (même format que Windows). */
@Serializable
data class ShareContent(
    val ownerName: String,
    val recipientName: String = "",
    val devices: List<Device> = emptyList(),
)

/** Fichier d'accès vérifié et déchiffré. */
data class OpenedAccess(val revision: Long, val issuedAt: String, val content: ShareContent)

@Serializable
internal data class ShareFile(val format: String, val version: Int, val payload: String, val signature: String)

@Serializable
internal data class SharePayload(
    val owner: String,
    val device: String,
    val revision: Long,
    val issuedAt: String,
    val epk: String,
    val iv: String,
    val data: String,
)

/**
 * Format de partage « wakeonlan-share/1 » (docs/PARTAGE.md), identique à l'application Windows :
 * fichier d'accès chiffré pour la clé ECDH P-256 d'un seul appareil (HKDF-SHA256 + AES-256-GCM) et
 * signé par la clé ECDSA P-256 de la personne qui partage. Vecteurs communs : protocol/share-vectors.json.
 */
object ShareCrypto {
    const val FORMAT = "wakeonlan-share"
    const val VERSION = 1
    const val MAX_NAME_LENGTH = 40
    const val MAX_FILE_SIZE = 256 shl 10
    const val MAX_DEVICES = 100

    private const val DOMAIN = "wakeonlan-share/1"
    private const val SIGN_PREFIX = "$DOMAIN\n"
    private const val HKDF_INFO = "$DOMAIN access"
    private const val VERIFY_INFO = "$DOMAIN verify"
    private const val PUBLIC_LEN = 65
    private const val IV_LEN = 12

    private val random = SecureRandom()
    private val b64url = Base64.getUrlEncoder().withoutPadding()
    private val b64urlDecoder = Base64.getUrlDecoder()
    private val b64 = Base64.getEncoder()
    private val b64Decoder = Base64.getDecoder()
    private val json = Json(ConfigCodec.json) { }

    // Courbe P-256 (secp256r1), pour contrôler les points reçus et reconstruire les clés.
    private val P = BigInteger("ffffffff00000001000000000000000000000000ffffffffffffffffffffffff", 16)
    private val A = P - BigInteger.valueOf(3)
    private val B = BigInteger("5ac635d8aa3a93e7b3ebbd55769886bc651d06b0cc53b0f63bce3c3e27d2604b", 16)
    private val N = BigInteger("ffffffff00000000ffffffffffffffffbce6faada7179e84f3b9cac2fc632551", 16)
    private val G = ECPoint(
        BigInteger("6b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c296", 16),
        BigInteger("4fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f5", 16),
    )
    private val curve = ECParameterSpec(EllipticCurve(ECFieldFp(P), A, B), G, N, 1)

    // --- Clés ---

    /** Nouvelle paire de clés (signature de la personne qui partage, ou réception d'un appareil). */
    fun newKeyPair(): ShareKeyPair {
        val pair = KeyPairGenerator.getInstance("EC").apply { initialize(ECGenParameterSpec("secp256r1"), random) }.generateKeyPair()
        val w = (pair.public as ECPublicKey).w
        return ShareKeyPair(fixed((pair.private as ECPrivateKey).s), byteArrayOf(4) + fixed(w.affineX) + fixed(w.affineY))
    }

    /** Relit une paire enregistrée ([privateKey] et [publicKey] encodées en Base64 URL). */
    fun keyPair(privateKey: String, publicKey: String): ShareKeyPair {
        val priv = decodeKey(privateKey)
        val pub = decodeKey(publicKey)
        if (priv == null || priv.size != 32 || pub == null || point(pub) == null) invalid("clé de partage invalide")
        return ShareKeyPair(priv, pub)
    }

    fun encodeKey(bytes: ByteArray): String = b64url.encodeToString(bytes)

    fun decodeKey(text: String): ByteArray? = try {
        b64urlDecoder.decode(text)
    } catch (e: IllegalArgumentException) {
        null
    }

    /** Vrai si [text] est une clé publique P-256 valide (point non compressé sur la courbe). */
    fun isValidPublicKey(text: String): Boolean = decodeKey(text)?.let { point(it) } != null

    /** Identifiant court d'une clé publique : 32 caractères hexadécimaux. */
    fun keyId(publicKey: String): String {
        val raw = decodeKey(publicKey) ?: return ""
        return hex(sha256(raw).copyOf(16))
    }

    /** Nom du fichier d'accès d'un appareil. */
    fun fileName(devicePublic: String): String = "acces-${keyId(devicePublic)}.json"

    /** Code à 6 chiffres affiché des deux côtés d'une demande (« 123 456 »). */
    fun verificationCode(ownerPublic: String, devicePublic: String): String {
        val owner = decodeKey(ownerPublic) ?: return ""
        val device = decodeKey(devicePublic) ?: return ""
        val sum = sha256(VERIFY_INFO.toByteArray() + owner + device)
        val n = (BigInteger(1, sum.copyOf(4)).toLong() % 1_000_000).toString().padStart(6, '0')
        return n.substring(0, 3) + " " + n.substring(3)
    }

    /** Nom de personne valide : 1 à 40 caractères, sans caractère de contrôle ni espace autour. */
    fun isValidName(name: String): Boolean {
        val count = name.codePointCount(0, name.length)
        if (name.trim() != name || count == 0 || count > MAX_NAME_LENGTH) return false
        return name.none { it.code < 0x20 || it.code == 0x7f }
    }

    // --- Fichier d'accès ---

    /** Fabrique le fichier d'accès d'un appareil ; [revision] doit croître à chaque publication. */
    fun seal(owner: ShareKeyPair, devicePublic: String, revision: Long, issuedAt: String, content: ShareContent): String {
        val plain = json.encodeToString(ShareContent.serializer(), content).toByteArray()
        val iv = ByteArray(IV_LEN).also(random::nextBytes)
        return sealRaw(owner, devicePublic, revision, issuedAt, plain, newKeyPair(), iv)
    }

    internal fun sealRaw(
        owner: ShareKeyPair, devicePublic: String, revision: Long, issuedAt: String,
        plain: ByteArray, ephemeral: ShareKeyPair, iv: ByteArray,
    ): String {
        val payload = encrypt(owner.publicEncoded, devicePublic, revision, issuedAt, plain, ephemeral, iv)
        val body = json.encodeToString(SharePayload.serializer(), payload).toByteArray()
        val signature = Signature.getInstance("SHA256withECDSA").run {
            initSign(privateKey(owner.privateKey))
            update(SIGN_PREFIX.toByteArray() + body)
            sign()
        }
        val file = ShareFile(FORMAT, VERSION, b64.encodeToString(body), b64.encodeToString(signature))
        return ConfigCodec.prettyJson.encodeToString(ShareFile.serializer(), file)
    }

    internal fun encrypt(
        ownerPublic: String, devicePublic: String, revision: Long, issuedAt: String,
        plain: ByteArray, ephemeral: ShareKeyPair, iv: ByteArray,
    ): SharePayload {
        val device = decodeKey(devicePublic)?.takeIf { point(it) != null } ?: invalid("clé de l'appareil invalide")
        val secret = agree(ephemeral.privateKey, device)
        val cipher = cipher(Cipher.ENCRYPT_MODE, secret, ephemeral.publicKey, device, iv)
        cipher.updateAAD(additionalData(ownerPublic, devicePublic))
        return SharePayload(
            owner = ownerPublic, device = devicePublic, revision = revision, issuedAt = issuedAt,
            epk = ephemeral.publicEncoded, iv = b64.encodeToString(iv), data = b64.encodeToString(cipher.doFinal(plain)),
        )
    }

    /**
     * Vérifie puis déchiffre un fichier d'accès : signature de [ownerPublic] (celle de l'invitation),
     * destiné à [device], révision au moins égale à [minRevision], PC valides.
     */
    fun open(text: String, ownerPublic: String, device: ShareKeyPair, minRevision: Long): OpenedAccess {
        if (text.length > MAX_FILE_SIZE) invalid("fichier d'accès trop volumineux")
        val file = try {
            json.decodeFromString(ShareFile.serializer(), text)
        } catch (e: SerializationException) {
            invalid("fichier d'accès illisible", e)
        } catch (e: IllegalArgumentException) {
            invalid("fichier d'accès illisible", e)
        }
        if (file.format != FORMAT) invalid("ce n'est pas un fichier d'accès")
        if (file.version != VERSION) invalid("version ${file.version} non prise en charge : mettez l'application à jour")
        val body = decode64(file.payload)
        val signature = decode64(file.signature)
        val owner = decodeKey(ownerPublic)?.takeIf { point(it) != null } ?: invalid("clé de la personne qui partage invalide")
        val valid = try {
            Signature.getInstance("SHA256withECDSA").run {
                initVerify(publicKey(owner))
                update(SIGN_PREFIX.toByteArray() + body)
                verify(signature)
            }
        } catch (e: GeneralSecurityException) {
            false
        }
        if (!valid) throw ShareException(ShareException.Reason.SIGNATURE, "fichier non signé par la personne qui partage")
        val payload = try {
            json.decodeFromString(SharePayload.serializer(), body.decodeToString())
        } catch (e: SerializationException) {
            invalid("contenu signé illisible", e)
        } catch (e: IllegalArgumentException) {
            invalid("contenu signé illisible", e)
        }
        if (payload.owner != ownerPublic) throw ShareException(ShareException.Reason.SIGNATURE, "fichier non signé par la personne qui partage")
        if (payload.device != device.publicEncoded) throw ShareException(ShareException.Reason.RECIPIENT, "fichier destiné à un autre appareil")
        if (payload.revision < minRevision) throw ShareException(ShareException.Reason.OLDER, "fichier plus ancien que celui déjà reçu")
        val epk = decodeKey(payload.epk)?.takeIf { point(it) != null } ?: invalid("clé éphémère invalide")
        val iv = decode64(payload.iv)
        if (iv.size != IV_LEN) invalid("IV invalide")
        val plain = try {
            val cipher = cipher(Cipher.DECRYPT_MODE, agree(device.privateKey, epk), epk, device.publicKey, iv)
            cipher.updateAAD(additionalData(payload.owner, payload.device))
            cipher.doFinal(decode64(payload.data))
        } catch (e: GeneralSecurityException) {
            invalid("données chiffrées invalides", e)
        }
        val content = try {
            json.decodeFromString(ShareContent.serializer(), plain.decodeToString())
        } catch (e: SerializationException) {
            invalid("contenu illisible", e)
        } catch (e: IllegalArgumentException) {
            invalid("contenu illisible", e)
        }
        if (!isValidName(content.ownerName) || content.devices.size > MAX_DEVICES) invalid("contenu invalide")
        // Mêmes contrôles qu'un import : ne jamais faire confiance au contenu reçu.
        val devices = try {
            ConfigCodec.sanitize(AppConfig(devices = content.devices)).devices
        } catch (e: ConfigException) {
            invalid(e.message ?: "PC invalide", e)
        }
        return OpenedAccess(payload.revision, payload.issuedAt, content.copy(devices = devices))
    }

    // --- Primitives ---

    private fun cipher(mode: Int, secret: ByteArray, ephemeralPublic: ByteArray, devicePublic: ByteArray, iv: ByteArray): Cipher {
        val key = hkdf(secret, ephemeralPublic + devicePublic, HKDF_INFO.toByteArray())
        return Cipher.getInstance("AES/GCM/NoPadding").apply {
            init(mode, SecretKeySpec(key, "AES"), GCMParameterSpec(128, iv))
        }
    }

    private fun additionalData(ownerPublic: String, devicePublic: String): ByteArray {
        val owner = decodeKey(ownerPublic) ?: invalid("clé invalide")
        val device = decodeKey(devicePublic) ?: invalid("clé invalide")
        return DOMAIN.toByteArray() + owner + device
    }

    /** HKDF-SHA256 (RFC 5869) pour une sortie de 32 octets (un seul bloc). */
    private fun hkdf(secret: ByteArray, salt: ByteArray, info: ByteArray): ByteArray {
        val prk = Mac.getInstance("HmacSHA256").run {
            init(SecretKeySpec(salt, "HmacSHA256"))
            doFinal(secret)
        }
        return Mac.getInstance("HmacSHA256").run {
            init(SecretKeySpec(prk, "HmacSHA256"))
            update(info)
            doFinal(byteArrayOf(1))
        }
    }

    private fun agree(privateKey: ByteArray, publicKey: ByteArray): ByteArray =
        KeyAgreement.getInstance("ECDH").run {
            init(privateKey(privateKey))
            doPhase(publicKey(publicKey), true)
            generateSecret()
        }

    private fun privateKey(raw: ByteArray): PrivateKey =
        KeyFactory.getInstance("EC").generatePrivate(ECPrivateKeySpec(BigInteger(1, raw), curve))

    private fun publicKey(raw: ByteArray): PublicKey =
        KeyFactory.getInstance("EC").generatePublic(ECPublicKeySpec(point(raw) ?: invalid("clé invalide"), curve))

    /** Point non compressé vérifié (sur la courbe), ou null. */
    private fun point(raw: ByteArray): ECPoint? {
        if (raw.size != PUBLIC_LEN || raw[0].toInt() != 4) return null
        val x = BigInteger(1, raw.copyOfRange(1, 33))
        val y = BigInteger(1, raw.copyOfRange(33, 65))
        if (x >= P || y >= P) return null
        val left = y.multiply(y).mod(P)
        val right = x.multiply(x).multiply(x).add(A.multiply(x)).add(B).mod(P)
        return if (left == right) ECPoint(x, y) else null
    }

    private fun fixed(value: BigInteger): ByteArray {
        val bytes = value.toByteArray()
        return when {
            bytes.size == 32 -> bytes
            bytes.size > 32 -> bytes.copyOfRange(bytes.size - 32, bytes.size)
            else -> ByteArray(32 - bytes.size) + bytes
        }
    }

    private fun decode64(text: String): ByteArray = try {
        b64Decoder.decode(text)
    } catch (e: IllegalArgumentException) {
        invalid("Base64 invalide", e)
    }

    private fun sha256(data: ByteArray): ByteArray = MessageDigest.getInstance("SHA-256").digest(data)

    internal fun hex(bytes: ByteArray): String = bytes.joinToString("") { "%02x".format(it) }

    private fun invalid(message: String, cause: Throwable? = null): Nothing =
        throw ShareException(ShareException.Reason.INVALID, message, cause)
}
