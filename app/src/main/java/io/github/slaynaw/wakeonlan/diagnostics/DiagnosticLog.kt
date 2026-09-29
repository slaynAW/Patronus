package io.github.slaynaw.wakeonlan.diagnostics

import android.content.Context
import android.os.Build
import android.util.Log
import io.github.slaynaw.wakeonlan.BuildConfig
import io.github.slaynaw.wakeonlan.core.share.ShareCrypto
import io.github.slaynaw.wakeonlan.data.KeystoreCipher
import java.io.File
import java.nio.ByteBuffer
import java.security.SecureRandom
import java.time.Instant
import java.util.Base64
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/**
 * Journal de diagnostic : évènements, avertissements et erreurs de l'application (avec leur trace
 * complète), gardés sur le téléphone et joints au rapport que l'utilisateur exporte lui-même
 * (Réglages → Diagnostic). Rien n'est envoyé automatiquement.
 *
 * Chaque évènement est une ligne chiffrée en AES-256-GCM avec une clé de données aléatoire, elle-même
 * chiffrée par une clé du Keystore Android (non extractible). Deux fichiers de 512 Kio au plus : le
 * plus ancien est remplacé. N'y écrire aucun secret (clés d'agent, jetons, clés privées, mots de
 * passe) : [keyId] donne une empreinte courte pour identifier une clé publique.
 */
object DiagnosticLog {
    enum class Level(val letter: Char) { DEBUG('D'), INFO('I'), WARN('W'), ERROR('E') }

    private const val TAG = "Patronus"
    private const val MAX_FILE_BYTES = 512L * 1024
    private const val MAX_RECORD_CHARS = 16_000
    private const val FILE_CURRENT = "journal-0.log"
    private const val FILE_PREVIOUS = "journal-1.log"

    private val executor = Executors.newSingleThreadExecutor { r -> Thread(r, "journal").apply { isDaemon = true } }
    private val random = SecureRandom()

    @Volatile
    private var store: Store? = null

    /** Ouvre le journal (au démarrage de l'application). */
    fun init(context: Context) {
        if (store != null) return
        store = runCatching { Store.open(File(context.applicationContext.filesDir, "diagnostics")) }
            .onFailure { Log.e(TAG, "Journal de diagnostic indisponible", it) }
            .getOrNull()
        i(
            "appli",
            "démarrage : Patronus ${BuildConfig.VERSION_NAME} (code ${BuildConfig.VERSION_CODE}), " +
                "Android ${Build.VERSION.RELEASE} (API ${Build.VERSION.SDK_INT}), ${Build.MANUFACTURER} ${Build.MODEL}",
        )
    }

    fun d(area: String, message: String) = log(Level.DEBUG, area, message, null)

    fun i(area: String, message: String) = log(Level.INFO, area, message, null)

    fun w(area: String, message: String, error: Throwable? = null) = log(Level.WARN, area, message, error)

    fun e(area: String, message: String, error: Throwable? = null) = log(Level.ERROR, area, message, error)

    /** Écrit immédiatement (plantage : le processus va s'arrêter). */
    fun now(level: Level, area: String, message: String, error: Throwable?) {
        val text = format(level, area, message, error)
        logcat(level, text)
        runCatching { executor.submit { store?.append(text) }.get(2, TimeUnit.SECONDS) }
    }

    /** Tous les évènements, du plus ancien au plus récent (pour le rapport). */
    fun records(): List<String> =
        runCatching { executor.submit<List<String>> { store?.readAll().orEmpty() }.get(20, TimeUnit.SECONDS) }
            .getOrElse { listOf("[journal illisible : ${it.javaClass.simpleName}]") }

    /**
     * Empreinte courte d'une clé publique (identifie un appareil sans la recopier) : début de
     * l'identifiant du protocole, le même dans les rapports Windows et dans les noms des fichiers
     * d'accès du Gist (« acces-<identifiant>.json »).
     */
    fun keyId(key: String): String {
        val id = ShareCrypto.keyId(key)
        return if (id.isEmpty()) "-" else "k:" + id.take(8)
    }

    private fun log(level: Level, area: String, message: String, error: Throwable?) {
        val text = format(level, area, message, error)
        logcat(level, text)
        val s = store ?: return
        runCatching { executor.execute { s.append(text) } }
    }

    private fun format(level: Level, area: String, message: String, error: Throwable?): String {
        val text = buildString {
            append(Instant.now()).append(' ').append(level.letter).append(' ').append(area).append(" : ").append(message)
            if (error != null) append('\n').append(error.stackTraceToString().trimEnd())
        }
        return if (text.length > MAX_RECORD_CHARS) text.take(MAX_RECORD_CHARS) + "…" else text
    }

    private fun logcat(level: Level, text: String) {
        val priority = when (level) {
            Level.DEBUG -> Log.DEBUG
            Level.INFO -> Log.INFO
            Level.WARN -> Log.WARN
            Level.ERROR -> Log.ERROR
        }
        if (BuildConfig.DEBUG || level >= Level.WARN) Log.println(priority, TAG, text)
    }

    /** Fichiers chiffrés du journal (utilisés uniquement depuis le fil « journal »). */
    private class Store(private val dir: File, private val key: SecretKeySpec) {
        private val current = File(dir, FILE_CURRENT)
        private val previous = File(dir, FILE_PREVIOUS)

        fun append(text: String) {
            try {
                val iv = ByteArray(12).also(random::nextBytes)
                val cipher = Cipher.getInstance("AES/GCM/NoPadding")
                cipher.init(Cipher.ENCRYPT_MODE, key, GCMParameterSpec(128, iv))
                val sealed = cipher.doFinal(text.toByteArray(Charsets.UTF_8))
                val line = Base64.getEncoder().encodeToString(ByteBuffer.allocate(iv.size + sealed.size).put(iv).put(sealed).array())
                current.appendText(line + "\n")
                if (current.length() > MAX_FILE_BYTES) {
                    previous.delete()
                    current.renameTo(previous)
                }
            } catch (e: Exception) {
                Log.e(TAG, "Écriture du journal impossible", e)
            }
        }

        fun readAll(): List<String> = listOf(previous, current).filter { it.exists() }.flatMap { file ->
            file.readLines().filter { it.isNotBlank() }.map { line ->
                try {
                    val bytes = Base64.getDecoder().decode(line)
                    val cipher = Cipher.getInstance("AES/GCM/NoPadding")
                    cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, bytes, 0, 12))
                    String(cipher.doFinal(bytes, 12, bytes.size - 12), Charsets.UTF_8)
                } catch (e: Exception) {
                    "[ligne illisible : ${e.javaClass.simpleName}]"
                }
            }
        }

        companion object {
            private const val KEY_FILE = "journal.key"

            /** Ouvre le dossier ; clé de données créée ou relue (journaux effacés si elle est perdue). */
            fun open(dir: File): Store {
                dir.mkdirs()
                val wrapper = KeystoreCipher(alias = "patronus-diagnostics-v1", aad = "patronus/diagnostics")
                val keyFile = File(dir, KEY_FILE)
                val raw = runCatching { wrapper.decrypt(keyFile.readBytes()) }.getOrNull()?.takeIf { it.size == 32 }
                    ?: ByteArray(32).also { fresh ->
                        random.nextBytes(fresh)
                        // Clé absente ou perdue : les anciennes lignes sont illisibles, on repart de zéro.
                        File(dir, FILE_CURRENT).delete()
                        File(dir, FILE_PREVIOUS).delete()
                        val tmp = File(dir, "$KEY_FILE.tmp")
                        tmp.writeBytes(wrapper.encrypt(fresh))
                        if (!tmp.renameTo(keyFile)) error("clé du journal non enregistrée")
                    }
                return Store(dir, SecretKeySpec(raw, "AES")).also { raw.fill(0) }
            }
        }
    }
}
