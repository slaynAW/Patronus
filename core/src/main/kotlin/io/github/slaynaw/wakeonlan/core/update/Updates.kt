package io.github.slaynaw.wakeonlan.core.update

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import java.io.File
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URI
import java.security.GeneralSecurityException
import java.security.KeyFactory
import java.security.MessageDigest
import java.security.PublicKey
import java.security.Signature
import java.security.spec.X509EncodedKeySpec
import java.util.Base64

/** Un fichier de la version, pour une plateforme (« android », « windows-amd64 »…). */
@Serializable
data class UpdateFile(
    val platform: String,
    val name: String,
    val size: Long,
    val sha256: String,
)

/** Manifeste d'une version publiée (« update.json »), mêmes règles que l'application Windows. */
@Serializable
data class UpdateManifest(
    val format: Int,
    val version: String,
    /** Croît à chaque build (numéro d'exécution de la CI) : c'est lui qui est comparé. */
    val code: Long,
    val date: String = "",
    val notes: String = "",
    val files: List<UpdateFile> = emptyList(),
) {
    fun file(platform: String): UpdateFile? = files.firstOrNull { it.platform == platform }
}

/** Échec d'une recherche ou d'un téléchargement de mise à jour. */
class UpdateException(val reason: Reason, message: String, cause: Throwable? = null) : IOException(message, cause) {
    enum class Reason {
        /** Aucune version publiée ne contient encore de manifeste. */
        NOT_PUBLISHED,

        /** Signature absente ou invalide : manifeste refusé. */
        SIGNATURE,

        /** Manifeste mal formé. */
        INVALID,

        /** Fichier téléchargé différent de celui annoncé. */
        CORRUPT,

        /** Réseau indisponible, serveur en erreur. */
        NETWORK,
    }
}

/**
 * Vérification des mises à jour. Chaque version officielle publiée par la CI contient
 * « update.json » signé avec la clé de signature de l'APK (« update.json.sig » : RSA PKCS#1 v1.5 /
 * SHA-256, en base64). Un manifeste n'est accepté que si sa signature est valide, et un fichier que
 * s'il a exactement la taille et l'empreinte annoncées.
 */
object Updates {
    /** Versions publiées (GitHub Releases du dépôt public). */
    const val DEFAULT_BASE = "https://github.com/slaynAW/Patronus/releases"
    const val MANIFEST_NAME = "update.json"
    const val FORMAT = 1
    const val MAX_FILE_SIZE = 100L shl 20
    private const val MAX_MANIFEST = 64 shl 10

    private val versionPattern = Regex("^[0-9]{1,4}\\.[0-9]{1,4}\\.[0-9]{1,4}$")
    private val namePattern = Regex("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
    private val datePattern = Regex("^[0-9]{4}-[0-9]{2}-[0-9]{2}$")
    private val sha256Pattern = Regex("^[0-9a-f]{64}$")
    private val json = Json { ignoreUnknownKeys = true }

    /** Clé publique RSA (SubjectPublicKeyInfo DER, en base64). */
    fun publicKey(base64: String): PublicKey =
        KeyFactory.getInstance("RSA").generatePublic(X509EncodedKeySpec(Base64.getDecoder().decode(base64)))

    /** Vrai si [signature] (base64) authentifie le manifeste brut [data]. */
    fun verify(key: PublicKey, data: ByteArray, signature: String): Boolean = try {
        val bytes = Base64.getDecoder().decode(signature.trim())
        Signature.getInstance("SHA256withRSA").run {
            initVerify(key)
            update(data)
            verify(bytes)
        }
    } catch (e: IllegalArgumentException) {
        false
    } catch (e: GeneralSecurityException) {
        false
    }

    /** Lit et contrôle un manifeste (déjà authentifié par [verify]). */
    fun parse(data: ByteArray): UpdateManifest {
        val m = try {
            json.decodeFromString<UpdateManifest>(data.decodeToString())
        } catch (e: SerializationException) {
            throw invalid("manifeste illisible", e)
        } catch (e: IllegalArgumentException) {
            throw invalid("manifeste illisible", e)
        }
        when {
            m.format != FORMAT -> throw invalid("format de manifeste ${m.format} non pris en charge")
            !versionPattern.matches(m.version) -> throw invalid("numéro de version invalide")
            m.code <= 0 -> throw invalid("code de version invalide")
            m.date.isNotEmpty() && !datePattern.matches(m.date) -> throw invalid("date invalide")
            m.notes.length > 32 * 1024 -> throw invalid("nouveautés trop longues")
        }
        val seen = HashSet<String>()
        for (f in m.files) {
            when {
                f.platform.isEmpty() || !seen.add(f.platform) -> throw invalid("plateforme absente ou en double")
                !namePattern.matches(f.name) -> throw invalid("nom de fichier invalide")
                f.size <= 0 || f.size > MAX_FILE_SIZE -> throw invalid("taille invalide")
                !sha256Pattern.matches(f.sha256) -> throw invalid("empreinte invalide")
            }
        }
        return m
    }

    /** Adresse de téléchargement d'un fichier de la version. */
    fun fileUrl(base: String, manifest: UpdateManifest, file: UpdateFile): String =
        "$base/download/v${manifest.version}/${file.name}"

    private fun invalid(message: String, cause: Throwable? = null) =
        UpdateException(UpdateException.Reason.INVALID, message, cause)
}

/** Recherche et téléchargement des versions publiées sous [base], authentifiées par [key]. */
class UpdateClient(
    private val key: PublicKey,
    private val base: String = Updates.DEFAULT_BASE,
    private val userAgent: String = "Patronus",
) {
    /** Manifeste de la dernière version officielle, authentifié et contrôlé. */
    suspend fun latest(): UpdateManifest = withContext(Dispatchers.IO) {
        val url = "$base/latest/download/${Updates.MANIFEST_NAME}"
        val data = get(url, 64 * 1024)
        val signature = get("$url.sig", 4096).decodeToString()
        if (!Updates.verify(key, data, signature)) {
            throw UpdateException(UpdateException.Reason.SIGNATURE, "signature de la mise à jour invalide")
        }
        Updates.parse(data)
    }

    /**
     * Télécharge [file] dans [dest] ; [dest] n'est écrit que si le fichier est complet et conforme
     * à l'empreinte annoncée. [progress] reçoit le nombre d'octets reçus.
     */
    suspend fun download(manifest: UpdateManifest, file: UpdateFile, dest: File, progress: (Long, Long) -> Unit = { _, _ -> }) =
        withContext(Dispatchers.IO) {
            val connection = open(Updates.fileUrl(base, manifest, file))
            val temp = File(dest.parentFile, dest.name + ".part")
            try {
                if (connection.responseCode != HttpURLConnection.HTTP_OK) {
                    throw UpdateException(UpdateException.Reason.NETWORK, "téléchargement : réponse ${connection.responseCode}")
                }
                val digest = MessageDigest.getInstance("SHA-256")
                var done = 0L
                connection.inputStream.use { input ->
                    temp.outputStream().use { output ->
                        val buffer = ByteArray(64 * 1024)
                        while (true) {
                            ensureActive()
                            val n = input.read(buffer)
                            if (n < 0) break
                            done += n
                            if (done > file.size) break
                            output.write(buffer, 0, n)
                            digest.update(buffer, 0, n)
                            progress(done, file.size)
                        }
                    }
                }
                val hex = digest.digest().joinToString("") { "%02x".format(it) }
                if (done != file.size || hex != file.sha256) {
                    throw UpdateException(UpdateException.Reason.CORRUPT, "le fichier téléchargé ne correspond pas à la version publiée")
                }
                if (!temp.renameTo(dest)) throw IOException("écriture impossible : ${dest.name}")
            } catch (e: UpdateException) {
                throw e
            } catch (e: IOException) {
                throw UpdateException(UpdateException.Reason.NETWORK, e.message ?: "téléchargement interrompu", e)
            } finally {
                temp.delete()
                connection.disconnect()
            }
        }

    private fun open(url: String): HttpURLConnection =
        (URI(url).toURL().openConnection() as HttpURLConnection).apply {
            connectTimeout = 15_000
            readTimeout = 30_000
            instanceFollowRedirects = true
            setRequestProperty("User-Agent", userAgent)
        }

    private fun get(url: String, limit: Int): ByteArray {
        val connection = open(url)
        try {
            when (connection.responseCode) {
                HttpURLConnection.HTTP_OK -> Unit
                HttpURLConnection.HTTP_NOT_FOUND -> throw UpdateException(UpdateException.Reason.NOT_PUBLISHED, "aucune mise à jour publiée")
                else -> throw UpdateException(UpdateException.Reason.NETWORK, "réponse ${connection.responseCode} du serveur des mises à jour")
            }
            // Lecture bornée (InputStream.readNBytes n'existe qu'à partir d'Android 13).
            val out = java.io.ByteArrayOutputStream()
            connection.inputStream.use { input ->
                val buffer = ByteArray(8 * 1024)
                while (true) {
                    val n = input.read(buffer)
                    if (n < 0) break
                    out.write(buffer, 0, n)
                    if (out.size() > limit) throw UpdateException(UpdateException.Reason.INVALID, "réponse trop volumineuse")
                }
            }
            return out.toByteArray()
        } catch (e: UpdateException) {
            throw e
        } catch (e: IOException) {
            throw UpdateException(UpdateException.Reason.NETWORK, e.message ?: "réseau indisponible", e)
        } finally {
            connection.disconnect()
        }
    }
}
