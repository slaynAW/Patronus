package io.github.slaynaw.wakeonlan.diagnostics

import android.content.Context
import android.os.Build
import io.github.slaynaw.wakeonlan.BuildConfig
import java.io.File
import java.time.Instant

/**
 * Enregistre tout plantage dans un fichier local (jamais envoyé automatiquement), pour pouvoir
 * l'afficher au lancement suivant et le partager si l'utilisateur le souhaite.
 */
object CrashReporter {
    private const val MAX_LENGTH = 64_000

    fun install(context: Context) {
        val app = context.applicationContext
        val previous = Thread.getDefaultUncaughtExceptionHandler()
        Thread.setDefaultUncaughtExceptionHandler { thread, error ->
            runCatching { write(app, thread, error) }
            // Comportement normal d'Android ensuite (fermeture de l'application).
            previous?.uncaughtException(thread, error)
        }
    }

    /** Rapport du dernier plantage, ou `null`. */
    fun pending(context: Context): String? =
        runCatching { file(context).takeIf { it.exists() }?.readText() }.getOrNull()

    fun clear(context: Context) {
        runCatching { file(context).delete() }
    }

    private fun write(context: Context, thread: Thread, error: Throwable) {
        val report = buildString {
            appendLine("Wake On LAN ${BuildConfig.VERSION_NAME} (${BuildConfig.VERSION_CODE})")
            appendLine("Android ${Build.VERSION.RELEASE} (API ${Build.VERSION.SDK_INT}) - ${Build.MANUFACTURER} ${Build.MODEL}")
            appendLine("Date : ${Instant.now()}")
            appendLine("Thread : ${thread.name}")
            appendLine()
            append(error.stackTraceToString())
        }
        val target = file(context)
        target.parentFile?.mkdirs()
        target.writeText(report.take(MAX_LENGTH))
    }

    private fun file(context: Context) = File(context.filesDir, "crash/last-crash.txt")
}
