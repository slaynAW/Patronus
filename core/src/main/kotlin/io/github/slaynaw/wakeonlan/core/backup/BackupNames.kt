package io.github.slaynaw.wakeonlan.core.backup

import java.security.SecureRandom
import java.text.Normalizer
import java.time.LocalDate
import java.time.format.DateTimeParseException

/**
 * Sauvegardes automatiques (docs/SAUVEGARDE.md), mêmes règles que l'application Windows : un fichier
 * par appareil et par jour, « patronus-<appareil>-AAAA-MM-JJ.json », [KEEP] versions conservées par
 * appareil, dans le Gist secret [GIST_DESCRIPTION] du compte GitHub et/ou dans un dossier.
 */
object BackupNames {
    const val GIST_DESCRIPTION = "Patronus – sauvegardes chiffrées"
    const val KEEP = 7
    const val GIST_NOTE = "# Patronus – sauvegardes chiffrées\n\nSauvegardes automatiques de l'application Patronus " +
        "(https://github.com/slaynAW/Patronus), chiffrées par un mot de passe : illisibles sans lui.\n"

    private val NAME = Regex("""^patronus-([a-z0-9-]{1,40})-(\d{4}-\d{2}-\d{2})\.json$""")
    private val ANONYMOUS = Regex("""^[a-z0-9]{1,20}-[0-9a-f]{8}$""")
    private val MARKS = Regex("""\p{Mn}+""")

    /** Sauvegarde repérée par son nom. */
    data class Entry(val name: String, val device: String, val date: LocalDate, val size: Int = 0)

    /** Nom simplifié : minuscules sans accents, chiffres et tirets. */
    fun slug(name: String): String {
        val plain = MARKS.replace(Normalizer.normalize(name.lowercase(), Normalizer.Form.NFD), "")
            .replace("œ", "oe").replace("æ", "ae").replace("ß", "ss")
        val out = StringBuilder()
        var dash = false
        for (c in plain) {
            when {
                c in 'a'..'z' || c in '0'..'9' -> {
                    out.append(c)
                    dash = false
                }
                !dash && out.isNotEmpty() -> {
                    out.append('-')
                    dash = true
                }
            }
        }
        return out.toString().trim('-')
    }

    /**
     * Identifiant de l'appareil dans les noms de fichiers : son type et 8 caractères aléatoires
     * (« android-3fa29c1e »). Les noms des fichiers sont visibles en clair : le nom de l'appareil n'y
     * figure pas.
     */
    fun deviceId(kind: String, random: SecureRandom = SecureRandom()): String {
        var k = slug(kind).replace("-", "")
        if (k.isEmpty() || k.length > 20) k = "appareil"
        val suffix = ByteArray(4).also(random::nextBytes).joinToString("") { "%02x".format(it) }
        return "$k-$suffix"
    }

    /** Identifiant sans nom d'appareil ([deviceId]) ; ceux d'avant la 1.9.0 contiennent le nom. */
    fun isAnonymous(device: String): Boolean = ANONYMOUS.matches(device)

    /** Nom de la sauvegarde [name] de l'appareil [from] sous l'identifiant [to] (null : autre fichier). */
    fun renamed(name: String, from: String, to: String): String? {
        val e = parse(name) ?: return null
        return if (e.device == from) "patronus-$to-${e.date}.json" else null
    }

    fun fileName(device: String, day: LocalDate): String = "patronus-$device-$day.json"

    fun parse(name: String, size: Int = 0): Entry? {
        val m = NAME.matchEntire(name) ?: return null
        val date = try {
            LocalDate.parse(m.groupValues[2])
        } catch (e: DateTimeParseException) {
            return null
        }
        return Entry(name, m.groupValues[1], date, size)
    }

    /** Sauvegardes parmi [names], de la plus récente à la plus ancienne. */
    fun sorted(names: Collection<String>): List<Entry> =
        names.mapNotNull { parse(it) }.sortedWith(compareByDescending<Entry> { it.date }.thenBy { it.device })

    /** Sauvegardes de l'appareil au-delà des [keep] plus récentes (à supprimer). */
    fun outdated(names: Collection<String>, device: String, keep: Int = KEEP): List<String> =
        sorted(names).filter { it.device == device }.drop(keep).map { it.name }
}
