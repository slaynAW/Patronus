package io.github.slaynaw.wakeonlan.core.config

import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.AppSettings
import io.github.slaynaw.wakeonlan.core.model.DeviceValidator
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject

/** Erreur de lecture d'une configuration, avec un message destiné à l'utilisateur. */
class ConfigException(val reason: Reason, message: String, cause: Throwable? = null) : Exception(message, cause) {
    enum class Reason { NOT_A_BACKUP, NEWER_VERSION, INVALID_DATA, PASSWORD_REQUIRED, WRONG_PASSWORD }
}

/**
 * Sérialisation JSON de [AppConfig], avec migrations de schéma et validation stricte.
 *
 * Pour faire évoluer le format : incrémenter [AppConfig.CURRENT_SCHEMA_VERSION] et ajouter une
 * migration `n → n+1` dans [migrations]. Les anciennes sauvegardes restent ainsi lisibles.
 */
object ConfigCodec {
    const val MAX_DEVICES = 200

    val json = Json {
        ignoreUnknownKeys = true
        encodeDefaults = true
        explicitNulls = false
    }

    val prettyJson = Json(json) { prettyPrint = true }

    /** Migrations successives : la clé est la version SOURCE. Vide tant qu'il n'y a qu'un schéma. */
    private val migrations: Map<Int, (JsonObject) -> JsonObject> = emptyMap()

    fun encode(config: AppConfig, pretty: Boolean = false): String =
        (if (pretty) prettyJson else json).encodeToString(AppConfig.serializer(), config)

    fun decode(text: String): AppConfig {
        val root = try {
            json.parseToJsonElement(text).jsonObject
        } catch (e: Exception) {
            throw ConfigException(ConfigException.Reason.INVALID_DATA, "Fichier JSON illisible", e)
        }
        return fromJson(root)
    }

    fun fromJson(root: JsonObject): AppConfig {
        val version = (root["schemaVersion"] as? JsonPrimitive)?.intOrNull ?: 1
        if (version > AppConfig.CURRENT_SCHEMA_VERSION) {
            throw ConfigException(
                ConfigException.Reason.NEWER_VERSION,
                "Cette configuration vient d'une version plus récente de l'application : mettez-la à jour.",
            )
        }
        var migrated = root
        for (v in version until AppConfig.CURRENT_SCHEMA_VERSION) {
            val step = migrations[v] ?: error("Migration manquante depuis la version $v")
            migrated = step(migrated)
        }
        val config = try {
            json.decodeFromJsonElement(AppConfig.serializer(), migrated)
        } catch (e: Exception) {
            throw ConfigException(ConfigException.Reason.INVALID_DATA, "Configuration invalide : ${e.message}", e)
        }
        return sanitize(config)
    }

    /** Valide chaque terminal et remet les réglages dans des bornes raisonnables. */
    fun sanitize(config: AppConfig): AppConfig {
        if (config.devices.size > MAX_DEVICES) invalid("Trop de terminaux (max $MAX_DEVICES)")
        val ids = HashSet<String>()
        config.devices.forEach { device ->
            if (device.id.isBlank() || device.id.length > 64) invalid("Identifiant de terminal invalide")
            if (!ids.add(device.id)) invalid("Identifiant de terminal en double : ${device.id}")
            DeviceValidator.validate(device).firstOrNull()?.let { invalid("« ${device.name} » : ${it.message}") }
        }
        val s = config.settings
        return config.copy(
            schemaVersion = AppConfig.CURRENT_SCHEMA_VERSION,
            settings = s.copy(
                pollIntervalSeconds = s.pollIntervalSeconds.coerceIn(AppSettings.POLL_INTERVAL_RANGE),
                wakeTimeoutSeconds = s.wakeTimeoutSeconds.coerceIn(AppSettings.WAKE_TIMEOUT_RANGE),
            ),
        )
    }

    private fun invalid(message: String): Nothing = throw ConfigException(ConfigException.Reason.INVALID_DATA, message)
}
