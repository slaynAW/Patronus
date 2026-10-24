package io.github.slaynaw.wakeonlan.data

import android.content.Context
import io.github.slaynaw.wakeonlan.core.hub.HubStore
import io.github.slaynaw.wakeonlan.core.hub.PairedHub
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import java.io.File
import java.io.IOException
import java.security.GeneralSecurityException

@Serializable
private data class HubFile(val version: Int = VERSION, val hub: PairedHub? = null)

private const val VERSION = 1

/**
 * Hub auquel ce téléphone est appairé (docs/HUB.md, §5.2), chiffré comme l'historique par [KeystoreCipher] (clé
 * distincte). Comme l'identité du téléphone, il n'est ni exporté ni sauvegardé : un fichier illisible (restauré sur un
 * autre téléphone, Keystore effacé) est oublié, il suffit d'appairer de nouveau.
 */
class HubRepository(context: Context, scope: CoroutineScope) : HubStore {

    private val file = File(context.applicationContext.filesDir, "hub/hub.bin")
    private val cipher = KeystoreCipher(alias = "patronus-hub-v1", aad = "patronus/hub")
    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
    private val mutex = Mutex()
    private val loaded = CompletableDeferred<Unit>()
    private val _hub = MutableStateFlow<PairedHub?>(null)

    override val hub: StateFlow<PairedHub?> = _hub.asStateFlow()

    init {
        scope.launch(Dispatchers.IO) {
            try {
                _hub.value = load()
            } finally {
                loaded.complete(Unit)
            }
        }
    }

    /** Le hub appairé, une fois le fichier lu. */
    override suspend fun current(): PairedHub? {
        loaded.await()
        return _hub.value
    }

    /** Applique une modification (null : oublie le hub) puis l'enregistre. */
    override suspend fun update(transform: (PairedHub?) -> PairedHub?) {
        loaded.await()
        mutex.withLock {
            val next = transform(_hub.value)
            if (next == _hub.value) return
            _hub.value = next
            withContext(Dispatchers.IO) { save(next) }
        }
    }

    private fun load(): PairedHub? {
        if (!file.exists()) return null
        return try {
            val f = json.decodeFromString(HubFile.serializer(), String(cipher.decrypt(file.readBytes()), Charsets.UTF_8))
            if (f.version == VERSION) f.hub else unreadable(IOException("version ${f.version} inconnue"))
        } catch (e: GeneralSecurityException) {
            unreadable(e)
        } catch (e: IOException) {
            unreadable(e)
        } catch (e: SerializationException) {
            unreadable(e)
        } catch (e: RuntimeException) {
            // Keystore défaillant sur certains appareils (ProviderException…).
            unreadable(e)
        }
    }

    private fun unreadable(e: Throwable): PairedHub? {
        DiagnosticLog.w("hub", "hub appairé illisible, à appairer de nouveau", e)
        file.delete()
        return null
    }

    private fun save(hub: PairedHub?) {
        try {
            if (hub == null) {
                file.delete()
                return
            }
            val dir = file.parentFile ?: return
            dir.mkdirs()
            val temp = File(dir, file.name + ".tmp")
            temp.writeBytes(cipher.encrypt(json.encodeToString(HubFile.serializer(), HubFile(VERSION, hub)).toByteArray(Charsets.UTF_8)))
            if (!temp.renameTo(file)) throw IOException("Écriture du hub impossible")
        } catch (e: GeneralSecurityException) {
            DiagnosticLog.w("hub", "hub non enregistré", e)
        } catch (e: IOException) {
            DiagnosticLog.w("hub", "hub non enregistré", e)
        } catch (e: RuntimeException) {
            DiagnosticLog.w("hub", "hub non enregistré", e)
        }
    }
}
