package io.github.slaynaw.wakeonlan.data

import android.content.Context
import android.util.Log
import androidx.datastore.core.CorruptionException
import androidx.datastore.core.DataStore
import androidx.datastore.core.DataStoreFactory
import androidx.datastore.core.Serializer
import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import io.github.slaynaw.wakeonlan.core.config.ConfigCodec
import io.github.slaynaw.wakeonlan.core.config.ConfigException
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.AppSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import java.io.File
import java.io.InputStream
import java.io.OutputStream
import java.security.GeneralSecurityException

/**
 * Stockage de la configuration : un fichier DataStore entièrement chiffré par [KeystoreCipher].
 * Toutes les écritures sont atomiques (DataStore) et validées ([ConfigCodec.sanitize]).
 */
class ConfigRepository(context: Context) {

    private val store: DataStore<AppConfig> = DataStoreFactory.create(
        serializer = EncryptedConfigSerializer(KeystoreCipher()),
        corruptionHandler = ReplaceFileCorruptionHandler { e ->
            // Clé Keystore perdue ou fichier abîmé : on repart d'une configuration vide
            // plutôt que de bloquer l'application. L'export sert de sauvegarde.
            Log.e(TAG, "Configuration illisible, réinitialisation", e)
            AppConfig()
        },
        scope = CoroutineScope(Dispatchers.IO + SupervisorJob()),
        produceFile = { File(context.applicationContext.filesDir, "datastore/config.bin") },
    )

    val config: Flow<AppConfig> = store.data

    suspend fun current(): AppConfig = store.data.first()

    suspend fun update(transform: (AppConfig) -> AppConfig): AppConfig =
        store.updateData { ConfigCodec.sanitize(transform(it)) }

    suspend fun upsert(device: Device) = update { it.upsert(device) }

    suspend fun delete(id: String) = update { it.remove(id) }

    suspend fun move(id: String, offset: Int) = update { it.move(id, offset) }

    suspend fun updateSettings(transform: (AppSettings) -> AppSettings) =
        update { it.copy(settings = transform(it.settings)) }

    /** Remplace tous les terminaux par ceux importés (les réglages importés sont aussi appliqués). */
    suspend fun replaceWith(imported: AppConfig) = update { imported }

    /** Ajoute les terminaux importés ; ceux qui existent déjà (même identifiant) sont mis à jour. */
    suspend fun mergeWith(imported: AppConfig) = update { it.mergeDevicesFrom(imported) }

    private companion object {
        const val TAG = "ConfigRepository"
    }
}

private class EncryptedConfigSerializer(private val cipher: KeystoreCipher) : Serializer<AppConfig> {
    override val defaultValue: AppConfig = AppConfig()

    override suspend fun readFrom(input: InputStream): AppConfig {
        val bytes = input.readBytes()
        if (bytes.isEmpty()) return defaultValue
        return try {
            ConfigCodec.decode(String(cipher.decrypt(bytes), Charsets.UTF_8))
        } catch (e: GeneralSecurityException) {
            throw CorruptionException("Déchiffrement impossible", e)
        } catch (e: ConfigException) {
            throw CorruptionException("Configuration invalide", e)
        }
    }

    override suspend fun writeTo(t: AppConfig, output: OutputStream) {
        output.write(cipher.encrypt(ConfigCodec.encode(t).toByteArray(Charsets.UTF_8)))
    }
}
