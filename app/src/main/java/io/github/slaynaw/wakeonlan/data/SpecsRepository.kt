package io.github.slaynaw.wakeonlan.data

import android.content.Context
import io.github.slaynaw.wakeonlan.core.agent.AgentSpecs
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

/** Fiche d'un PC, lue le [fetched] (ms) sur son agent [agent] (version). */
@Serializable
data class StoredSpecs(val specs: AgentSpecs, val fetched: Long, val agent: String = "")

@Serializable
private data class SpecsFile(val version: Int = VERSION, val devices: Map<String, StoredSpecs> = emptyMap())

private const val VERSION = 1

/**
 * Fiche de chaque PC (commande « specs » de l'agent 1.10.0), chiffrée comme l'historique par
 * [KeystoreCipher] (clé distincte) : elle reste affichée PC éteint. Un fichier illisible repart de
 * zéro : les fiches se relisent sur les agents.
 */
class SpecsRepository(context: Context, scope: CoroutineScope) {

    private val file = File(context.applicationContext.filesDir, "specs/specs.bin")
    private val cipher = KeystoreCipher(alias = "wakeonlan-specs-v1", aad = "wakeonlan/specs")
    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
    private val mutex = Mutex()
    private val loaded = CompletableDeferred<Unit>()
    private val _data = MutableStateFlow<Map<String, StoredSpecs>>(emptyMap())

    val data: StateFlow<Map<String, StoredSpecs>> = _data.asStateFlow()

    init {
        scope.launch(Dispatchers.IO) {
            try {
                _data.value = load()
            } finally {
                loaded.complete(Unit)
            }
        }
    }

    /** Applique une modification puis l'enregistre (après la lecture du fichier au démarrage). */
    suspend fun update(transform: (Map<String, StoredSpecs>) -> Map<String, StoredSpecs>) {
        loaded.await()
        mutex.withLock {
            val next = transform(_data.value)
            if (next == _data.value) return
            _data.value = next
            withContext(Dispatchers.IO) { save(next) }
        }
    }

    private fun load(): Map<String, StoredSpecs> {
        if (!file.exists()) return emptyMap()
        return try {
            val f = json.decodeFromString(SpecsFile.serializer(), String(cipher.decrypt(file.readBytes()), Charsets.UTF_8))
            if (f.version == VERSION) f.devices else unreadable(IOException("version ${f.version} inconnue"))
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

    private fun unreadable(e: Throwable): Map<String, StoredSpecs> {
        DiagnosticLog.w("fiche", "fiches des PC illisibles, relues sur les agents", e)
        file.delete()
        return emptyMap()
    }

    private fun save(devices: Map<String, StoredSpecs>) {
        try {
            val dir = file.parentFile ?: return
            dir.mkdirs()
            val temp = File(dir, file.name + ".tmp")
            temp.writeBytes(cipher.encrypt(json.encodeToString(SpecsFile.serializer(), SpecsFile(VERSION, devices)).toByteArray(Charsets.UTF_8)))
            if (!temp.renameTo(file)) throw IOException("Écriture des fiches impossible")
        } catch (e: GeneralSecurityException) {
            DiagnosticLog.w("fiche", "non enregistrée", e)
        } catch (e: IOException) {
            DiagnosticLog.w("fiche", "non enregistrée", e)
        } catch (e: RuntimeException) {
            DiagnosticLog.w("fiche", "non enregistrée", e)
        }
    }
}
