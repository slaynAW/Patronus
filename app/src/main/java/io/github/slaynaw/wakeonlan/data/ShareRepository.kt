package io.github.slaynaw.wakeonlan.data

import android.content.Context
import android.util.Log
import androidx.datastore.core.CorruptionException
import androidx.datastore.core.DataStore
import androidx.datastore.core.DataStoreFactory
import androidx.datastore.core.Serializer
import androidx.datastore.core.handlers.ReplaceFileCorruptionHandler
import io.github.slaynaw.wakeonlan.core.config.ConfigCodec
import io.github.slaynaw.wakeonlan.core.share.ShareState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.serialization.SerializationException
import java.io.File
import java.io.InputStream
import java.io.OutputStream
import java.security.GeneralSecurityException

/**
 * État du partage (clés privées, jeton GitHub, accès reçus) : fichier DataStore chiffré par
 * [KeystoreCipher] avec une clé distincte de celle de la configuration.
 */
class ShareRepository(context: Context) {

    private val store: DataStore<ShareState> = DataStoreFactory.create(
        serializer = EncryptedShareSerializer(KeystoreCipher(alias = "wakeonlan-share-v1", aad = "wakeonlan/share")),
        corruptionHandler = ReplaceFileCorruptionHandler { e ->
            Log.e(TAG, "Partage illisible, réinitialisation", e)
            ShareState()
        },
        scope = CoroutineScope(Dispatchers.IO + SupervisorJob()),
        produceFile = { File(context.applicationContext.filesDir, "datastore/share.bin") },
    )

    val data: Flow<ShareState> = store.data

    suspend fun current(): ShareState = store.data.first()

    suspend fun update(transform: (ShareState) -> ShareState): ShareState = store.updateData(transform)

    private companion object {
        const val TAG = "ShareRepository"
    }
}

private class EncryptedShareSerializer(private val cipher: KeystoreCipher) : Serializer<ShareState> {
    override val defaultValue: ShareState = ShareState()

    override suspend fun readFrom(input: InputStream): ShareState {
        val bytes = input.readBytes()
        if (bytes.isEmpty()) return defaultValue
        return try {
            ConfigCodec.json.decodeFromString(ShareState.serializer(), String(cipher.decrypt(bytes), Charsets.UTF_8))
        } catch (e: GeneralSecurityException) {
            throw CorruptionException("Déchiffrement impossible", e)
        } catch (e: SerializationException) {
            throw CorruptionException("État du partage invalide", e)
        } catch (e: IllegalArgumentException) {
            throw CorruptionException("État du partage invalide", e)
        }
    }

    override suspend fun writeTo(t: ShareState, output: OutputStream) {
        output.write(cipher.encrypt(ConfigCodec.json.encodeToString(ShareState.serializer(), t).toByteArray(Charsets.UTF_8)))
    }
}
