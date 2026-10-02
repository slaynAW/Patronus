package io.github.slaynaw.wakeonlan.backup

import io.github.slaynaw.wakeonlan.data.KeystoreCipher
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import java.io.File
import java.io.IOException
import java.security.GeneralSecurityException

/** Compte GitHub des sauvegardes. */
@Serializable
data class BackupGitHub(val token: String, val user: String, val gist: String = "")

/**
 * Réglages des sauvegardes de ce téléphone. Le mot de passe et le jeton GitHub sont des secrets : le
 * fichier est chiffré par une clé du Keystore Android, comme la configuration.
 */
@Serializable
data class BackupSettings(
    val enabled: Boolean = false,
    val password: String = "",
    /** Identifiant du téléphone dans les noms de fichiers (BackupNames.deviceId). */
    val device: String = "",
    val github: BackupGitHub? = null,
    /** Dossier choisi (URI d'arborescence du sélecteur Android) et son nom affiché. */
    val folder: String = "",
    val folderLabel: String = "",
    val lastGithub: Long = 0,
    val lastFolder: Long = 0,
    /** Fichiers de ce téléphone présents dans le Gist (pour ne garder que KEEP versions). */
    val uploaded: List<String> = emptyList(),
) {
    override fun toString(): String =
        "BackupSettings(enabled=$enabled, password=${if (password.isEmpty()) "<vide>" else "***"}, device=$device, " +
            "github=${github?.let { "@${it.user}" }}, folder=$folderLabel)"
}

/** Fichier des réglages (backup.bin, chiffré). */
class BackupStore(private val file: File) {
    private val cipher = KeystoreCipher(alias = "patronus-backup-v1", aad = "patronus/backup")
    private val json = Json { ignoreUnknownKeys = true }

    /** Réglages enregistrés ; un fichier illisible est mis de côté (exception pour le signaler). */
    fun load(): BackupSettings {
        if (!file.exists()) return BackupSettings()
        return try {
            json.decodeFromString(BackupSettings.serializer(), String(cipher.decrypt(file.readBytes()), Charsets.UTF_8))
        } catch (e: GeneralSecurityException) {
            unreadable(e)
        } catch (e: SerializationException) {
            unreadable(e)
        } catch (e: IllegalArgumentException) {
            unreadable(e)
        } catch (e: RuntimeException) {
            unreadable(e)
        }
    }

    private fun unreadable(e: Exception): Nothing {
        file.renameTo(File(file.parentFile, "${file.name}.illisible-${System.currentTimeMillis()}"))
        throw IOException("réglages des sauvegardes illisibles, mis de côté (${e.javaClass.simpleName})", e)
    }

    fun save(settings: BackupSettings) {
        val tmp = File(file.parentFile, "${file.name}.tmp")
        tmp.writeBytes(cipher.encrypt(json.encodeToString(BackupSettings.serializer(), settings).toByteArray(Charsets.UTF_8)))
        if (!tmp.renameTo(file)) {
            tmp.delete()
            throw IOException("réglages des sauvegardes non enregistrés")
        }
    }
}
