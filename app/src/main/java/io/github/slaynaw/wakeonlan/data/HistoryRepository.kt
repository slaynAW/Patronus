package io.github.slaynaw.wakeonlan.data

import android.content.Context
import io.github.slaynaw.wakeonlan.core.history.HistoryData
import io.github.slaynaw.wakeonlan.diagnostics.DataKind
import io.github.slaynaw.wakeonlan.diagnostics.DataNotices
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
import java.io.File
import java.io.IOException
import java.security.GeneralSecurityException

/**
 * Historique des démarrages et extinctions (30 jours), chiffré comme la configuration par
 * [KeystoreCipher] (clé distincte). Écritures atomiques ; un fichier illisible repart de zéro :
 * c'est un simple journal, et celui des agents est de toute façon relu sur chaque PC.
 */
class HistoryRepository(context: Context, scope: CoroutineScope) {

    private val file = File(context.applicationContext.filesDir, "history/history.bin")
    private val cipher = KeystoreCipher(alias = "wakeonlan-history-v1", aad = "wakeonlan/history")
    private val mutex = Mutex()
    private val loaded = CompletableDeferred<Unit>()
    private val _data = MutableStateFlow(HistoryData.EMPTY)

    val data: StateFlow<HistoryData> = _data.asStateFlow()

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
    suspend fun update(transform: (HistoryData) -> HistoryData) {
        loaded.await()
        mutex.withLock {
            val next = transform(_data.value)
            if (next == _data.value) return
            _data.value = next
            withContext(Dispatchers.IO) { save(next) }
        }
    }

    private fun load(): HistoryData {
        if (!file.exists()) return HistoryData.EMPTY
        return try {
            HistoryData.decode(String(cipher.decrypt(file.readBytes()), Charsets.UTF_8)) ?: HistoryData.EMPTY
        } catch (e: GeneralSecurityException) {
            unreadable(e)
        } catch (e: IOException) {
            unreadable(e)
        } catch (e: RuntimeException) {
            // Keystore défaillant sur certains appareils (ProviderException…) : l'historique repart de zéro.
            unreadable(e)
        }
    }

    /** Historique illisible : copie gardée de côté et incident noté, puis historique vide. */
    private fun unreadable(e: Throwable): HistoryData {
        DataNotices.unreadable(DataKind.HISTORY, file, e, notify = false)
        return HistoryData.EMPTY
    }

    private fun save(data: HistoryData) {
        try {
            val dir = file.parentFile ?: return
            dir.mkdirs()
            val temp = File(dir, file.name + ".tmp")
            temp.writeBytes(cipher.encrypt(HistoryData.encode(data).toByteArray(Charsets.UTF_8)))
            if (!temp.renameTo(file)) throw IOException("Écriture de l'historique impossible")
        } catch (e: GeneralSecurityException) {
            DiagnosticLog.w("historique", "non enregistré", e)
        } catch (e: IOException) {
            DiagnosticLog.w("historique", "non enregistré", e)
        } catch (e: RuntimeException) {
            DiagnosticLog.w("historique", "non enregistré", e)
        }
    }
}
