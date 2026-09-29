package io.github.slaynaw.wakeonlan.core.diagnostics

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.security.GeneralSecurityException
import java.security.SecureRandom
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.PBEKeySpec
import javax.crypto.spec.SecretKeySpec

/** Chiffrement décrit dans un rapport (valeurs en Base64 standard). */
@Serializable
data class DiagnosticEncryption(
    val kdf: String,
    val iterations: Int,
    val salt: String,
    val cipher: String,
    val iv: String,
)

/** Fichier d'un rapport de diagnostic (seuls [app] et [createdAt] restent lisibles). */
@Serializable
data class DiagnosticEnvelope(
    val format: String,
    val version: Int,
    val app: String,
    val createdAt: String,
    val encryption: DiagnosticEncryption,
    val data: String,
)

/**
 * Rapports de diagnostic (format « patronus-diagnostic/1 », docs/DIAGNOSTIC.md, identique à
 * l'application Windows et à l'agent) : texte chiffré par un mot de passe choisi par l'utilisateur,
 * PBKDF2-HMAC-SHA256 puis AES-256-GCM, comme les sauvegardes complètes.
 */
object DiagnosticCodec {
    const val FORMAT = "patronus-diagnostic"
    const val VERSION = 1
    const val KDF = "PBKDF2WithHmacSHA256"
    const val CIPHER = "AES-256-GCM"
    const val DEFAULT_ITERATIONS = 600_000
    const val MIN_ITERATIONS = 100_000
    const val MAX_ITERATIONS = 10_000_000
    const val MIN_PASSWORD_LENGTH = 8
    const val MAX_REPORT_CHARS = 4_000_000

    private val AAD = "$FORMAT/$VERSION".toByteArray()
    private val json = Json { prettyPrint = true; ignoreUnknownKeys = true }

    /** Chiffre un rapport ; [app] (« Patronus Android 1.5.4 ») et [createdAt] restent lisibles. */
    fun seal(
        report: String,
        password: CharArray,
        app: String,
        createdAt: String,
        iterations: Int = DEFAULT_ITERATIONS,
        random: SecureRandom = SecureRandom(),
    ): String {
        require(password.size >= MIN_PASSWORD_LENGTH) { "mot de passe trop court ($MIN_PASSWORD_LENGTH caractères au moins)" }
        val text = if (report.length > MAX_REPORT_CHARS) report.takeLast(MAX_REPORT_CHARS) else report
        val salt = ByteArray(16).also(random::nextBytes)
        val iv = ByteArray(12).also(random::nextBytes)
        val data = cipher(Cipher.ENCRYPT_MODE, password, salt, iv, iterations).doFinal(text.toByteArray(Charsets.UTF_8))
        val b64 = Base64.getEncoder()
        val envelope = DiagnosticEnvelope(
            format = FORMAT,
            version = VERSION,
            app = app,
            createdAt = createdAt,
            encryption = DiagnosticEncryption(KDF, iterations, b64.encodeToString(salt), CIPHER, b64.encodeToString(iv)),
            data = b64.encodeToString(data),
        )
        return json.encodeToString(DiagnosticEnvelope.serializer(), envelope)
    }

    /** Déchiffre un rapport ; exception explicite si le fichier ou le mot de passe ne convient pas. */
    fun open(text: String, password: CharArray): Pair<DiagnosticEnvelope, String> {
        val env = runCatching { json.decodeFromString(DiagnosticEnvelope.serializer(), text) }.getOrNull()
            ?.takeIf { it.format == FORMAT }
            ?: throw IllegalArgumentException("ce fichier n'est pas un rapport de diagnostic Patronus")
        val e = env.encryption
        require(env.version == VERSION && e.kdf == KDF && e.cipher == CIPHER) { "rapport de version ${env.version} non pris en charge" }
        require(e.iterations in MIN_ITERATIONS..MAX_ITERATIONS) { "paramètres de chiffrement invalides" }
        val b64 = Base64.getDecoder()
        val salt = b64.decode(e.salt)
        val iv = b64.decode(e.iv)
        require(salt.size >= 16 && iv.size == 12) { "rapport abîmé" }
        val plain = try {
            cipher(Cipher.DECRYPT_MODE, password, salt, iv, e.iterations).doFinal(b64.decode(env.data))
        } catch (ex: GeneralSecurityException) {
            throw IllegalArgumentException("mot de passe incorrect ou rapport modifié", ex)
        }
        return env to String(plain, Charsets.UTF_8)
    }

    private fun cipher(mode: Int, password: CharArray, salt: ByteArray, iv: ByteArray, iterations: Int): Cipher {
        val spec = PBEKeySpec(password, salt, iterations, 256)
        val keyBytes = try {
            SecretKeyFactory.getInstance(KDF).generateSecret(spec).encoded
        } finally {
            spec.clearPassword()
        }
        return Cipher.getInstance("AES/GCM/NoPadding").apply {
            init(mode, SecretKeySpec(keyBytes, "AES"), GCMParameterSpec(128, iv))
            updateAAD(AAD)
        }.also { keyBytes.fill(0) }
    }
}
