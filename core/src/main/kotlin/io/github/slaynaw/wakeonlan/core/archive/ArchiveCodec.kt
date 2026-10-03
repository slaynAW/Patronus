package io.github.slaynaw.wakeonlan.core.archive

import io.github.slaynaw.wakeonlan.core.agent.AgentHistoryEvent
import io.github.slaynaw.wakeonlan.core.agent.MetricsRow
import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.security.GeneralSecurityException
import java.security.SecureRandom
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeParseException
import java.util.Base64
import java.util.zip.GZIPInputStream
import java.util.zip.GZIPOutputStream
import javax.crypto.Cipher
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.PBEKeySpec
import javax.crypto.spec.SecretKeySpec

/** Erreur de lecture d'une archive ; [wrongPassword] : le mot de passe ne correspond pas à ce mois. */
class ArchiveException(message: String, val wrongPassword: Boolean = false, cause: Throwable? = null) : Exception(message, cause)

/**
 * Format des archives chiffrées « patronus-archive/1 » (identique à desktop/internal/archive côté
 * Windows) : mesures minute par minute et journal des démarrages et arrêts des PC, rangés sur GitHub
 * dans un Gist secret par mois (description « Patronus – archives chiffrées AAAA-MM »).
 *
 * Fichiers d'un Gist : [MANIFEST_FILE] (sel et paramètres, en clair), [README_FILE], un fichier de
 * mesures par jour UTC ([dayFile]) et le journal du mois ([journalFile]). Chaque fichier de données
 * est du JSON compressé (gzip) puis chiffré en AES-256-GCM (nonce de 12 octets en tête, Base64), clé
 * dérivée du mot de passe des sauvegardes par PBKDF2-HMAC-SHA256 (600 000 itérations, sel du mois) ;
 * le nom du fichier est authentifié. Seules les dates apparaissent en clair.
 */
object ArchiveCodec {
    const val FORMAT = "patronus-archive"
    const val VERSION = 1
    const val KDF = "PBKDF2WithHmacSHA256"
    const val ITERATIONS = 600_000
    const val MIN_ITERATIONS = 100_000
    const val MAX_ITERATIONS = 10_000_000
    const val MANIFEST_FILE = "patronus-archive.json"
    const val README_FILE = "LISEZMOI.md"
    const val DESCRIPTION_PREFIX = "Patronus – archives chiffrées "
    const val MAX_FILE_BYTES = 4 shl 20
    const val MAX_PLAIN_BYTES = 16 shl 20
    private const val CHECK_NAME = "check"

    private val monthPattern = Regex("^\\d{4}-\\d{2}$")
    private val dayFilePattern = Regex("^mesures-(\\d{4}-\\d{2}-\\d{2})\\.txt$")
    private val random = SecureRandom()

    internal val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
    private val pretty = Json { prettyPrint = true; prettyPrintIndent = "  " }

    fun description(month: String) = DESCRIPTION_PREFIX + month

    /** Mois d'une description de Gist d'archives (`null` si ce n'en est pas une). */
    fun monthOf(description: String): String? =
        description.removePrefix(DESCRIPTION_PREFIX).takeIf { description.startsWith(DESCRIPTION_PREFIX) && monthPattern.matches(it) }

    fun dayFile(day: String) = "mesures-$day.txt"

    /** Jour d'un nom de fichier de mesures (`null` si ce n'en est pas un). */
    fun dayOf(name: String): String? = dayFilePattern.find(name)?.groupValues?.get(1)

    fun journalFile(month: String) = "journal-$month.txt"

    /** Début (secondes Unix) d'un jour UTC « 2026-10-03 », `null` s'il est invalide. */
    fun dayStart(day: String): Long? = try {
        LocalDate.parse(day, DateTimeFormatter.ISO_LOCAL_DATE).atStartOfDay(ZoneOffset.UTC).toEpochSecond()
    } catch (_: DateTimeParseException) {
        null
    }

    /** Jour UTC (« 2026-10-03 ») d'une heure en secondes. */
    fun dayOfTime(seconds: Long): String = Instant.ofEpochSecond(seconds).atOffset(ZoneOffset.UTC).toLocalDate().toString()

    /** Mois UTC (« 2026-10 ») d'une heure en secondes. */
    fun monthOfTime(seconds: Long): String = dayOfTime(seconds).substring(0, 7)

    /** Crée le manifeste (sel aléatoire) et la clé d'un nouveau mois. */
    fun newManifest(month: String, password: CharArray): Pair<ArchiveManifest, ArchiveKey> {
        require(monthPattern.matches(month)) { "Mois invalide : $month" }
        val salt = ByteArray(16).also(random::nextBytes)
        val key = deriveKey(password, salt, ITERATIONS)
        val check = key.seal(CHECK_NAME, String.serializer(), FORMAT)
        val manifest = ArchiveManifest(FORMAT, VERSION, month, KDF, ITERATIONS, Base64.getEncoder().encodeToString(salt), check)
        return manifest to key
    }

    /** Lit un manifeste et vérifie ses paramètres. */
    fun parseManifest(text: String): ArchiveManifest {
        val m = try {
            json.decodeFromString(ArchiveManifest.serializer(), text)
        } catch (e: SerializationException) {
            throw ArchiveException("Manifeste illisible", cause = e)
        } catch (e: IllegalArgumentException) {
            throw ArchiveException("Manifeste illisible", cause = e)
        }
        when {
            m.format != FORMAT -> throw ArchiveException("Ce Gist n'est pas une archive Patronus")
            m.version != VERSION -> throw ArchiveException("Archive de version ${m.version} : mettez l'application à jour")
            m.kdf != KDF || m.iterations !in MIN_ITERATIONS..MAX_ITERATIONS -> throw ArchiveException("Paramètres de chiffrement non pris en charge")
            !monthPattern.matches(m.month) -> throw ArchiveException("Mois invalide dans le manifeste")
        }
        return m
    }

    fun manifestJson(m: ArchiveManifest): String = pretty.encodeToString(ArchiveManifest.serializer(), m)

    /** Vérifie le mot de passe et renvoie la clé du mois. */
    fun unlock(m: ArchiveManifest, password: CharArray): ArchiveKey {
        val salt = try {
            Base64.getDecoder().decode(m.salt)
        } catch (e: IllegalArgumentException) {
            throw ArchiveException("Sel invalide dans le manifeste", cause = e)
        }
        if (salt.size < 16) throw ArchiveException("Sel invalide dans le manifeste")
        val key = deriveKey(password, salt, m.iterations)
        val check = try {
            key.open(CHECK_NAME, m.check, String.serializer())
        } catch (e: ArchiveException) {
            throw ArchiveException("Mot de passe des archives incorrect", wrongPassword = true, cause = e)
        }
        if (check != FORMAT) throw ArchiveException("Mot de passe des archives incorrect", wrongPassword = true)
        return key
    }

    /**
     * Rechiffre les fichiers d'un Gist d'archives de [oldPassword] vers [newPassword] : nouveau
     * manifeste (sel neuf), même contenu. Exception (wrongPassword) si l'ancien mot de passe n'ouvre pas
     * ce mois. Renvoie les fichiers, et ceux que l'ancienne clé n'ouvre pas (abîmés, recopiés tels quels).
     */
    fun reencrypt(files: Map<String, String>, oldPassword: CharArray, newPassword: CharArray): Pair<Map<String, String>, List<String>> {
        val text = files[MANIFEST_FILE] ?: throw ArchiveException("Manifeste absent")
        val manifest = parseManifest(text)
        val oldKey = unlock(manifest, oldPassword)
        val (next, newKey) = newManifest(manifest.month, newPassword)
        val out = linkedMapOf(MANIFEST_FILE to manifestJson(next), README_FILE to README)
        val unreadable = ArrayList<String>()
        for (name in files.keys.sorted()) {
            if (name == MANIFEST_FILE || name == README_FILE) continue
            val content = files.getValue(name)
            val value = try {
                oldKey.open(name, content, JsonElement.serializer())
            } catch (e: ArchiveException) {
                out[name] = content
                unreadable += name
                continue
            }
            out[name] = newKey.seal(name, JsonElement.serializer(), value)
        }
        return out to unreadable
    }

    private fun deriveKey(password: CharArray, salt: ByteArray, iterations: Int): ArchiveKey {
        require(password.isNotEmpty()) { "Mot de passe des sauvegardes requis" }
        val spec = PBEKeySpec(password, salt, iterations, 256)
        try {
            return ArchiveKey(SecretKeySpec(SecretKeyFactory.getInstance(KDF).generateSecret(spec).encoded, "AES"))
        } finally {
            spec.clearPassword()
        }
    }

    internal fun aad(name: String) = "$FORMAT/$VERSION:$name".toByteArray(Charsets.UTF_8)

    internal fun nonce() = ByteArray(12).also(random::nextBytes)

    const val README = """# Patronus – archives chiffrées

Ce Gist secret contient les mesures (températures, utilisation du processeur et de la carte
graphique, minute par minute) et le journal des démarrages et arrêts de vos PC pour un mois,
envoyés par l'application Patronus.

Tout est **chiffré** avec le mot de passe des sauvegardes (AES-256-GCM, clé dérivée par
PBKDF2-HMAC-SHA256, 600 000 itérations) : sans lui, ces fichiers sont illisibles. Seules les dates
apparaissent dans les noms des fichiers.

Consultation : application Patronus → fiche d'un PC → Mesures. Ne modifiez pas ces fichiers ; vous
pouvez supprimer le Gist pour effacer ce mois d'archives.
"""
}

/** Manifeste d'un Gist d'archives (en clair). */
@Serializable
data class ArchiveManifest(
    val format: String,
    val version: Int,
    val month: String,
    val kdf: String,
    val iterations: Int,
    val salt: String,
    /** Valeur chiffrée connue, pour reconnaître le bon mot de passe. */
    val check: String,
)

/** Clé d'un mois d'archives. */
class ArchiveKey internal constructor(private val key: SecretKeySpec) {
    /** Chiffre [value] (JSON compressé) pour le fichier [name]. */
    fun <T> seal(name: String, serializer: KSerializer<T>, value: T): String {
        val plain = ArchiveCodec.json.encodeToString(serializer, value).toByteArray(Charsets.UTF_8)
        val zipped = ByteArrayOutputStream().also { out -> GZIPOutputStream(out).use { it.write(plain) } }.toByteArray()
        val nonce = ArchiveCodec.nonce()
        val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply {
            init(Cipher.ENCRYPT_MODE, key, GCMParameterSpec(128, nonce))
            updateAAD(ArchiveCodec.aad(name))
        }
        return Base64.getEncoder().encodeToString(nonce + cipher.doFinal(zipped))
    }

    /** Déchiffre le fichier [name]. */
    fun <T> open(name: String, text: String, serializer: KSerializer<T>): T {
        if (text.length > ArchiveCodec.MAX_FILE_BYTES) throw ArchiveException("Fichier d'archive trop volumineux")
        val sealed = try {
            Base64.getDecoder().decode(text.trim())
        } catch (e: IllegalArgumentException) {
            throw ArchiveException("Fichier d'archive illisible", cause = e)
        }
        if (sealed.size < 12 + 16) throw ArchiveException("Fichier d'archive illisible")
        val zipped = try {
            Cipher.getInstance("AES/GCM/NoPadding").run {
                init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, sealed, 0, 12))
                updateAAD(ArchiveCodec.aad(name))
                doFinal(sealed, 12, sealed.size - 12)
            }
        } catch (e: GeneralSecurityException) {
            throw ArchiveException("Fichier d'archive illisible ou modifié", cause = e)
        }
        val plain = try {
            GZIPInputStream(ByteArrayInputStream(zipped)).use { input ->
                val out = ByteArrayOutputStream()
                val buffer = ByteArray(16 * 1024)
                while (true) {
                    val n = input.read(buffer)
                    if (n < 0) break
                    out.write(buffer, 0, n)
                    if (out.size() > ArchiveCodec.MAX_PLAIN_BYTES) throw ArchiveException("Fichier d'archive trop volumineux")
                }
                out.toByteArray()
            }
        } catch (e: java.io.IOException) {
            throw ArchiveException("Fichier d'archive illisible", cause = e)
        }
        return try {
            ArchiveCodec.json.decodeFromString(serializer, plain.toString(Charsets.UTF_8))
        } catch (e: SerializationException) {
            throw ArchiveException("Fichier d'archive illisible", cause = e)
        } catch (e: IllegalArgumentException) {
            throw ArchiveException("Fichier d'archive illisible", cause = e)
        }
    }
}

/** Mesures d'un jour (UTC) de tous les PC. */
@Serializable
data class ArchiveDay(val day: String, val pcs: List<ArchiveDayPc> = emptyList()) {
    /**
     * Ajoute les mesures d'un PC : une minute déjà présente est remplacée, les minutes hors du jour
     * ignorées. Le second élément indique si le contenu a changé.
     */
    fun addRows(mac: String, name: String, rows: List<MetricsRow>): Pair<ArchiveDay, Boolean> {
        val start = ArchiveCodec.dayStart(day) ?: return this to false
        val pc = pcs.firstOrNull { it.mac == mac } ?: ArchiveDayPc(mac, name)
        val byMinute = LinkedHashMap<Long, MetricsRow>()
        pc.rows.forEach { byMinute[it.t] = it }
        var changed = false
        for (r in rows) {
            if (r.t < start || r.t >= start + 86_400) continue
            if (byMinute[r.t] != r) {
                byMinute[r.t] = r
                changed = true
            }
        }
        val newName = name.ifBlank { pc.name }
        if (newName != pc.name) changed = true
        val updated = ArchiveDayPc(mac, newName, byMinute.values.sortedBy { it.t })
        return copy(pcs = (pcs.filter { it.mac != mac } + updated).sortedBy { it.mac }) to changed
    }

    fun rows(mac: String): List<MetricsRow> = pcs.firstOrNull { it.mac == mac }?.rows.orEmpty()
}

/** Mesures d'un PC, reconnu par son adresse MAC (la même sur tous les appareils). */
@Serializable
data class ArchiveDayPc(val mac: String, val name: String = "", val rows: List<MetricsRow> = emptyList())

/** Journal d'un mois de tous les PC. */
@Serializable
data class ArchiveJournal(val month: String, val pcs: List<ArchiveJournalPc> = emptyList()) {
    /**
     * Ajoute le journal d'un PC : évènements déjà présents ou d'un autre mois ignorés. Le second
     * élément indique si le contenu a changé.
     */
    fun addEvents(mac: String, name: String, events: List<AgentHistoryEvent>): Pair<ArchiveJournal, Boolean> {
        val pc = pcs.firstOrNull { it.mac == mac } ?: ArchiveJournalPc(mac, name)
        val all = pc.events.toMutableList()
        var changed = false
        for (e in events) {
            if (ArchiveCodec.monthOfTime(e.t) != month || e in all) continue
            all += e
            changed = true
        }
        val newName = name.ifBlank { pc.name }
        if (newName != pc.name) changed = true
        val updated = ArchiveJournalPc(mac, newName, all.sortedBy { it.t })
        return copy(pcs = (pcs.filter { it.mac != mac } + updated).sortedBy { it.mac }) to changed
    }

    fun events(mac: String): List<AgentHistoryEvent> = pcs.firstOrNull { it.mac == mac }?.events.orEmpty()
}

/** Journal de l'agent d'un PC (démarrages, arrêts, veille, commandes). */
@Serializable
data class ArchiveJournalPc(val mac: String, val name: String = "", val events: List<AgentHistoryEvent> = emptyList())
