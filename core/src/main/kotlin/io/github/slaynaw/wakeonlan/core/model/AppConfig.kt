package io.github.slaynaw.wakeonlan.core.model

import kotlinx.serialization.Serializable

/**
 * Configuration complète de l'application : c'est ce qui est stocké (chiffré) sur le téléphone
 * et ce qui est exporté / importé.
 *
 * [schemaVersion] permet de faire évoluer le format sans casser les anciennes sauvegardes :
 * voir [io.github.slaynaw.wakeonlan.core.config.ConfigCodec] pour les migrations.
 */
@Serializable
data class AppConfig(
    val schemaVersion: Int = CURRENT_SCHEMA_VERSION,
    val devices: List<Device> = emptyList(),
    val settings: AppSettings = AppSettings(),
) {
    fun device(id: String): Device? = devices.firstOrNull { it.id == id }

    /** Ajoute ou remplace (même identifiant) un terminal, en conservant l'ordre existant. */
    fun upsert(device: Device): AppConfig {
        val index = devices.indexOfFirst { it.id == device.id }
        val updated = if (index >= 0) devices.toMutableList().apply { set(index, device) } else devices + device
        return copy(devices = updated)
    }

    fun remove(id: String): AppConfig = copy(devices = devices.filterNot { it.id == id })

    fun move(id: String, offset: Int): AppConfig {
        val index = devices.indexOfFirst { it.id == id }
        val target = index + offset
        if (index < 0 || target !in devices.indices) return this
        val list = devices.toMutableList()
        list.add(target, list.removeAt(index))
        return copy(devices = list)
    }

    /**
     * Fusionne une configuration importée : les terminaux de même identifiant sont remplacés,
     * les nouveaux sont ajoutés à la fin. Les réglages locaux sont conservés.
     */
    fun mergeDevicesFrom(other: AppConfig): AppConfig = other.devices.fold(this) { acc, d -> acc.upsert(d) }

    companion object {
        const val CURRENT_SCHEMA_VERSION = 1
    }
}

/** Réglages généraux de l'application. */
@Serializable
data class AppSettings(
    /** Intervalle entre deux vérifications d'état, en secondes. */
    val pollIntervalSeconds: Int = DEFAULT_POLL_INTERVAL_SECONDS,
    /** Demander une confirmation avant d'éteindre / redémarrer / mettre en veille. */
    val confirmPowerActions: Boolean = true,
    /** Durée maximale d'attente du démarrage après l'envoi du paquet magique, en secondes. */
    val wakeTimeoutSeconds: Int = DEFAULT_WAKE_TIMEOUT_SECONDS,
) {
    companion object {
        const val DEFAULT_POLL_INTERVAL_SECONDS = 3
        const val DEFAULT_WAKE_TIMEOUT_SECONDS = 180
        val POLL_INTERVAL_RANGE = 1..60
        val WAKE_TIMEOUT_RANGE = 30..900
    }
}
