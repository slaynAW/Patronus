package io.github.slaynaw.wakeonlan.core.agent2

import io.github.slaynaw.wakeonlan.core.model.DeviceValidator
import java.io.ByteArrayOutputStream
import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException
import java.nio.charset.CodingErrorAction

/**
 * Lien d'appairage invalide. [field] désigne la cause : « format » (pas un lien d'appairage, trop long, ou
 * paramètres mal écrits), « duplicate » (paramètre en double) ou le paramètre en cause (« v », « h », « a »,
 * « c », « p », « m », « r », « e »).
 */
class InvalidLinkException(val field: String) : IllegalArgumentException("lien d'appairage invalide ($field)")

/**
 * Famille de liens d'appairage aux mêmes paramètres (§7.1) : celui d'un agent ([PairingLinkV2.AGENT]) et celui du
 * hub (docs/PROTOCOLE-HUB.md, §7).
 */
data class LinkFormat(
    /** Début du lien, en minuscules (la casse est indifférente à la lecture). */
    val prefix: String,
    /** Valeur exigée de « v ». */
    val version: String,
    /** Port quand « p » est absent. */
    val defaultPort: Int,
)

/**
 * Lien d'appairage `v=2` (§7.1). Il ne contient aucun secret durable : l'empreinte de l'agent est publique, et le
 * code ne sert qu'une fois, pendant quelques minutes.
 */
data class PairingLinkV2(
    /** Jamais vide après [parse] : le début de l'hôte à défaut de nom. */
    val name: String,
    val host: String,
    val port: Int,
    /** `AA:BB:CC:DD:EE:FF`, ou vide. */
    val mac: String,
    val agent: Pin,
    /** Normalisé (8 caractères). */
    val code: String,
    /** Rôle annoncé, purement indicatif ; null s'il est absent. */
    val role: Role?,
    /** Expiration annoncée (secondes Unix), pour l'affichage ; 0 si elle est absente. */
    val expires: Long,
) {
    /** L'expiration annoncée est-elle atteinte à [now] (secondes Unix) ? L'agent applique de toute façon la sienne. */
    fun expired(now: Long): Boolean = expires != 0L && now >= expires

    /** Le lien, paramètres dans l'ordre de la spécification, sans ceux qui sont vides. */
    fun text(format: LinkFormat = AGENT): String = format.prefix + buildList {
        add("v=" + format.version)
        if (name.isNotEmpty()) add("n=" + escape(name))
        add("h=" + escape(host))
        if (port != 0) add("p=$port")
        if (mac.isNotEmpty()) add("m=" + escape(mac))
        add("a=$agent")
        add("c=" + escape(code))
        role?.let { add("r=" + it.wire) }
        if (expires != 0L) add("e=$expires")
    }.joinToString("&")

    /** Sans le code, qui ne doit pas finir dans un journal. */
    override fun toString(): String =
        "PairingLinkV2(name=$name, host=$host, port=$port, mac=$mac, agent=$agent, code=***, role=$role, expires=$expires)"

    companion object {
        const val PREFIX = "wolagent://pair?"

        /** Lien d'appairage v=2 d'un agent. */
        val AGENT = LinkFormat(PREFIX, "2", Wolagent2.DEFAULT_PORT)
        const val MAX_LENGTH = 2048
        const val MAX_EXPIRES = 253_402_300_799L
        private val DIGITS_5 = Regex("[0-9]{1,5}")
        private val DIGITS_12 = Regex("[0-9]{1,12}")
        private val MAC = Regex("[0-9A-Fa-f]{2}([:-])[0-9A-Fa-f]{2}(\\1[0-9A-Fa-f]{2}){4}")
        private const val HEX = "0123456789ABCDEF"

        /**
         * Lit un lien `v=2` ; les paramètres sont vérifiés dans l'ordre du §7.1. Une autre version que 2 donne la
         * cause « v » : l'application lit alors le lien comme avant.
         *
         * @throws InvalidLinkException
         */
        fun parse(text: String, format: LinkFormat = AGENT): PairingLinkV2 {
            fun fail(field: String): Nothing = throw InvalidLinkException(field)
            val link = text.trim(' ', '\t', '\r', '\n')
            if (link.toByteArray(Charsets.UTF_8).size > MAX_LENGTH || !hasPrefix(link, format.prefix)) fail("format")
            val params = LinkedHashMap<String, String>()
            for (part in link.substring(format.prefix.length).split('&')) {
                if (part.isEmpty()) continue
                val eq = part.indexOf('=')
                if (eq <= 0) fail("format")
                val name = part.substring(0, eq)
                val value = formDecode(part.substring(eq + 1)) ?: fail("format")
                if (name in params) fail("duplicate")
                params[name] = value
            }
            if (params["v"] != format.version) fail("v")
            val host = params["h"].orEmpty()
            if (!DeviceValidator.isValidHost(host)) fail("h")
            val agent = Pin.parse(params["a"].orEmpty()) ?: fail("a")
            val code = PairingCode.normalize(params["c"].orEmpty()) ?: fail("c")
            val port = params["p"]?.let { p ->
                p.takeIf { DIGITS_5.matches(it) }?.toInt()?.takeIf { it in 1..65535 } ?: fail("p")
            } ?: format.defaultPort
            val mac = params["m"]?.let { m ->
                val canonical = m.uppercase().replace('-', ':')
                if (!MAC.matches(m) || canonical == "00:00:00:00:00:00" || canonical == "FF:FF:FF:FF:FF:FF") fail("m")
                canonical
            }.orEmpty()
            val role = params["r"]?.let { Role.parse(it) ?: fail("r") }
            val expires = params["e"]?.let { e ->
                e.takeIf { DIGITS_12.matches(it) }?.toLong()?.takeIf { it in 1..MAX_EXPIRES } ?: fail("e")
            } ?: 0L
            val name = Wolagent2.cleanName(params["n"].orEmpty(), Wolagent2.MAX_PC_NAME_LENGTH)
                .ifEmpty { Wolagent2.cleanName(host, Wolagent2.MAX_PC_NAME_LENGTH) }
            return PairingLinkV2(name, host, port, mac, agent, code, role, expires)
        }

        /** Préfixe sans tenir compte de la casse des lettres ASCII, et d'elles seules. */
        private fun hasPrefix(link: String, prefix: String): Boolean =
            link.length >= prefix.length && prefix.indices.all { i ->
                val c = link[i]
                (if (c in 'A'..'Z') c + ('a' - 'A') else c) == prefix[i]
            }

        /** Décodage de formulaire strict : `%` suivi de deux chiffres hexadécimaux, `+` pour une espace, UTF-8 valide. */
        private fun formDecode(value: String): String? {
            val raw = value.toByteArray(Charsets.UTF_8)
            val out = ByteArrayOutputStream(raw.size)
            var i = 0
            while (i < raw.size) {
                val b = raw[i].toInt() and 0xFF
                when (b) {
                    '%'.code -> {
                        if (i + 2 >= raw.size) return null
                        val hi = hexDigit(raw[i + 1])
                        val lo = hexDigit(raw[i + 2])
                        if (hi < 0 || lo < 0) return null
                        out.write(hi * 16 + lo)
                        i += 3
                    }
                    '+'.code -> {
                        out.write(' '.code)
                        i++
                    }
                    else -> {
                        out.write(b)
                        i++
                    }
                }
            }
            return try {
                Charsets.UTF_8.newDecoder()
                    .onMalformedInput(CodingErrorAction.REPORT)
                    .onUnmappableCharacter(CodingErrorAction.REPORT)
                    .decode(ByteBuffer.wrap(out.toByteArray()))
                    .toString()
            } catch (_: CharacterCodingException) {
                null
            }
        }

        private fun hexDigit(b: Byte): Int = when (val c = b.toInt()) {
            in '0'.code..'9'.code -> c - '0'.code
            in 'a'.code..'f'.code -> c - 'a'.code + 10
            in 'A'.code..'F'.code -> c - 'A'.code + 10
            else -> -1
        }

        /** Encode une valeur comme url.QueryEscape de Go, « : » gardé tel quel pour un QR code plus petit. */
        private fun escape(value: String): String = buildString {
            for (byte in value.toByteArray(Charsets.UTF_8)) {
                val c = byte.toInt() and 0xFF
                when {
                    c in 'A'.code..'Z'.code || c in 'a'.code..'z'.code || c in '0'.code..'9'.code ||
                        c == '-'.code || c == '_'.code || c == '.'.code || c == '~'.code || c == ':'.code -> append(c.toChar())
                    c == ' '.code -> append('+')
                    else -> append('%').append(HEX[c shr 4]).append(HEX[c and 15])
                }
            }
        }
    }
}
