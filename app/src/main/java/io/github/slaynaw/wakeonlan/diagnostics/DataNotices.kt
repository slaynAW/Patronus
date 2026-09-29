package io.github.slaynaw.wakeonlan.diagnostics

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import java.io.File
import java.time.LocalDateTime
import java.time.format.DateTimeFormatter

/** Données enregistrées qui peuvent devenir illisibles (fichier abîmé, clé Keystore perdue…). */
enum class DataKind { CONFIG, SHARE, HISTORY }

/**
 * Incidents sur les données enregistrées, signalés à l'utilisateur au lieu d'être passés sous
 * silence : le fichier illisible est mis de côté (jamais écrasé), l'incident noté au journal.
 */
object DataNotices {
    private val _notices = MutableStateFlow<List<DataKind>>(emptyList())
    val notices: StateFlow<List<DataKind>> = _notices.asStateFlow()

    /**
     * Garde une copie du fichier illisible avant sa réinitialisation, note l'incident et, si [notify],
     * le signale à l'utilisateur.
     */
    fun unreadable(kind: DataKind, file: File, error: Throwable, notify: Boolean = true) {
        val stamp = LocalDateTime.now().format(DateTimeFormatter.ofPattern("yyyyMMdd-HHmmss"))
        val kept = runCatching { file.copyTo(File(file.parentFile, "${file.name}.illisible-$stamp"), overwrite = true) }
        DiagnosticLog.e(
            "données",
            "$kind illisible (${file.name}, ${file.length()} octets) : réinitialisé ; copie " +
                (kept.getOrNull()?.name ?: "impossible (${kept.exceptionOrNull()?.javaClass?.simpleName})"),
            error,
        )
        if (notify) _notices.update { if (kind in it) it else it + kind }
    }

    fun dismiss() {
        _notices.value = emptyList()
    }
}
