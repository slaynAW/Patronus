package io.github.slaynaw.wakeonlan.core.config

import io.github.slaynaw.wakeonlan.core.model.AppConfig
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import java.security.SecureRandom
import java.util.Base64
import javax.crypto.AEADBadTagException
import javax.crypto.Cipher
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.PBEKeySpec
import javax.crypto.spec.SecretKeySpec

/** Enveloppe d'un fichier d'export (`*.wol.json`). */
@Serializable
data class ExportEnvelope(
    val format: String,
    val version: Int,
    val exportedAt: String,
    val app: String? = null,
    /** Configuration en clair (export SANS secrets). */
    val config: JsonElement? = null,
    /** Paramètres de chiffrement (export AVEC secrets). */
    val encryption: EncryptionInfo? = null,
    /** Configuration chiffrée, en Base64. */
    val data: String? = null,
)

@Serializable
data class EncryptionInfo(
    val kdf: String,
    val iterations: Int,
    val salt: String,
    val cipher: String,
    val iv: String,
)

/**
 * Export / import de la configuration.
 *
 * - Sans mot de passe : export lisible, **sans aucun secret** (clés d'agent retirées).
 * - Avec mot de passe : export complet, chiffré en AES-256-GCM avec une clé dérivée du mot de passe
 *   par PBKDF2-HMAC-SHA256 (600 000 itérations, sel aléatoire). Toute modification du fichier est détectée.
 */
object ExportCodec {
    const val FORMAT = "wakeonlan-config"
    const val VERSION = 1
    const val KDF = "PBKDF2WithHmacSHA256"
    const val CIPHER = "AES-256-GCM"
    const val DEFAULT_ITERATIONS = 600_000
    const val MIN_ITERATIONS = 100_000
    const val MAX_ITERATIONS = 10_000_000
    const val MIN_PASSWORD_LENGTH = 8

    private val AAD = "$FORMAT/$VERSION".toByteArray()
    private val b64 = Base64.getEncoder()
    private val b64Decoder = Base64.getDecoder()

    /**
     * @param password `null` pour un export en clair sans secrets.
     * @param exportedAt date ISO-8601 (fournie par l'appelant pour rester testable).
     */
    fun export(
        config: AppConfig,
        password: CharArray?,
        exportedAt: String,
        appVersion: String? = null,
        iterations: Int = DEFAULT_ITERATIONS,
        random: SecureRandom = SecureRandom(),
        /** Données ajoutées à une sauvegarde chiffrée (ex. « sharing »), ignorées des anciennes versions. */
        extra: Map<String, JsonElement> = emptyMap(),
    ): String {
        val envelope = if (password == null) {
            val stripped = config.copy(devices = config.devices.map { it.withoutSecrets() })
            ExportEnvelope(
                format = FORMAT,
                version = VERSION,
                exportedAt = exportedAt,
                app = appVersion,
                config = ConfigCodec.json.encodeToJsonElement(AppConfig.serializer(), stripped),
            )
        } else {
            require(password.size >= MIN_PASSWORD_LENGTH) { "Mot de passe trop court" }
            val salt = ByteArray(16).also(random::nextBytes)
            val iv = ByteArray(12).also(random::nextBytes)
            val cipher = cipher(Cipher.ENCRYPT_MODE, password, salt, iv, iterations)
            var root = ConfigCodec.json.encodeToJsonElement(AppConfig.serializer(), config) as JsonObject
            if (extra.isNotEmpty()) root = JsonObject(root + extra.filterKeys { it !in root })
            val encrypted = cipher.doFinal(root.toString().toByteArray(Charsets.UTF_8))
            ExportEnvelope(
                format = FORMAT,
                version = VERSION,
                exportedAt = exportedAt,
                app = appVersion,
                encryption = EncryptionInfo(KDF, iterations, b64.encodeToString(salt), CIPHER, b64.encodeToString(iv)),
                data = b64.encodeToString(encrypted),
            )
        }
        return ConfigCodec.prettyJson.encodeToString(ExportEnvelope.serializer(), envelope)
    }

    /** Lit l'enveloppe sans la déchiffrer (pour savoir s'il faut demander un mot de passe). */
    fun inspect(text: String): ExportEnvelope {
        val envelope = try {
            ConfigCodec.json.decodeFromString(ExportEnvelope.serializer(), text)
        } catch (e: Exception) {
            throw ConfigException(ConfigException.Reason.NOT_A_BACKUP, "Ce fichier n'est pas une sauvegarde Wake On LAN", e)
        }
        if (envelope.format != FORMAT) {
            throw ConfigException(ConfigException.Reason.NOT_A_BACKUP, "Ce fichier n'est pas une sauvegarde Wake On LAN")
        }
        if (envelope.version > VERSION) {
            throw ConfigException(
                ConfigException.Reason.NEWER_VERSION,
                "Sauvegarde créée par une version plus récente de l'application : mettez-la à jour.",
            )
        }
        return envelope
    }

    fun isEncrypted(text: String): Boolean = inspect(text).encryption != null

    fun import(text: String, password: CharArray?): AppConfig = importWithExtra(text, password).first

    /** Comme [import], avec les données ajoutées à une sauvegarde chiffrée (voir [export]). */
    fun importWithExtra(text: String, password: CharArray?): Pair<AppConfig, Map<String, JsonElement>> {
        val envelope = inspect(text)
        val encryption = envelope.encryption
        if (encryption == null) {
            val config = envelope.config as? JsonObject
                ?: throw ConfigException(ConfigException.Reason.INVALID_DATA, "Sauvegarde vide")
            return ConfigCodec.fromJson(config) to emptyMap()
        }
        if (password == null || password.isEmpty()) {
            throw ConfigException(ConfigException.Reason.PASSWORD_REQUIRED, "Cette sauvegarde est protégée par un mot de passe")
        }
        if (encryption.kdf != KDF || encryption.cipher != CIPHER || encryption.iterations !in MIN_ITERATIONS..MAX_ITERATIONS) {
            throw ConfigException(ConfigException.Reason.INVALID_DATA, "Paramètres de chiffrement non pris en charge")
        }
        val plain = try {
            val salt = b64Decoder.decode(encryption.salt)
            val iv = b64Decoder.decode(encryption.iv)
            val data = b64Decoder.decode(envelope.data ?: "")
            cipher(Cipher.DECRYPT_MODE, password, salt, iv, encryption.iterations).doFinal(data)
        } catch (e: AEADBadTagException) {
            throw ConfigException(ConfigException.Reason.WRONG_PASSWORD, "Mot de passe incorrect ou fichier modifié", e)
        } catch (e: IllegalArgumentException) {
            throw ConfigException(ConfigException.Reason.INVALID_DATA, "Sauvegarde corrompue", e)
        }
        val text = String(plain, Charsets.UTF_8)
        val extra = try {
            ConfigCodec.json.parseToJsonElement(text).jsonObject.filterKeys { it in EXTRA_KEYS }
        } catch (e: Exception) {
            emptyMap()
        }
        return ConfigCodec.decode(text) to extra
    }

    /** Données ajoutées reconnues dans une sauvegarde chiffrée : partage, historique. */
    private val EXTRA_KEYS = setOf("sharing", "history")

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
