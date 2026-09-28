package io.github.slaynaw.wakeonlan.core.share

import java.net.URLDecoder
import java.net.URLEncoder

/** Invitation affichée par la personne qui partage (aucun secret). */
data class ShareInvite(
    /** Nom de la personne qui partage. */
    val name: String,
    /** Sa clé publique de signature. */
    val owner: String,
    /** Compte GitHub et Gist contenant les fichiers d'accès. */
    val user: String,
    val gist: String,
) {
    val link: String get() = ShareLinks.build("invite", "name" to name, "owner" to owner, "user" to user, "gist" to gist)

    val isValid: Boolean
        get() = ShareCrypto.isValidName(name) && ShareCrypto.isValidPublicKey(owner) &&
            ShareLinks.LOGIN.matches(user) && ShareLinks.GIST.matches(gist)
}

/** Demande d'accès renvoyée par la personne invitée (aucun secret). */
data class ShareRequest(
    val name: String,
    /** Clé publique de réception de son appareil. */
    val device: String,
    /** Identifiant (KeyID) de la clé de la personne invitante : une demande ne sert qu'à elle. */
    val owner: String,
) {
    val link: String get() = ShareLinks.build("request", "name" to name, "device" to device, "owner" to owner)

    val isValid: Boolean
        get() = ShareCrypto.isValidName(name) && ShareCrypto.isValidPublicKey(device) && ShareLinks.KEY_ID.matches(owner)
}

/**
 * Liens échangés entre les deux personnes, par QR code ou message (même format que Windows) :
 * `wolshare://invite?v=1&name=…&owner=…&user=…&gist=…` et `wolshare://request?v=1&name=…&device=…&owner=…`.
 */
object ShareLinks {
    const val SCHEME = "wolshare"

    internal val LOGIN = Regex("^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$")
    internal val GIST = Regex("^[0-9a-f]{20,40}$")
    internal val KEY_ID = Regex("^[0-9a-f]{32}$")
    private val LINK = Regex("^([A-Za-z][A-Za-z0-9+.-]*)://([A-Za-z]+)\\?(.*)$", RegexOption.DOT_MATCHES_ALL)

    /** Type d'un lien de partage (« invite », « request ») ou null. */
    fun kind(text: String): String? {
        val m = LINK.matchEntire(text.trim()) ?: return null
        if (!m.groupValues[1].equals(SCHEME, ignoreCase = true)) return null
        return m.groupValues[2].lowercase().takeIf { it == "invite" || it == "request" }
    }

    fun parseInvite(text: String): ShareInvite {
        val q = parse(text, "invite")
        return ShareInvite(q["name"].orEmpty(), q["owner"].orEmpty(), q["user"].orEmpty(), q["gist"].orEmpty())
            .takeIf { it.isValid } ?: invalid()
    }

    fun parseRequest(text: String): ShareRequest {
        val q = parse(text, "request")
        return ShareRequest(q["name"].orEmpty(), q["device"].orEmpty(), q["owner"].orEmpty())
            .takeIf { it.isValid } ?: invalid()
    }

    internal fun build(kind: String, vararg params: Pair<String, String>): String =
        "$SCHEME://$kind?v=1&" + params.joinToString("&") { (k, v) -> k + "=" + URLEncoder.encode(v, "UTF-8") }

    private fun parse(text: String, kind: String): Map<String, String> {
        val trimmed = text.trim()
        if (trimmed.length > 2048 || kind(trimmed) != kind) invalid()
        val query = LINK.matchEntire(trimmed)!!.groupValues[3]
        val out = HashMap<String, String>()
        for (part in query.split('&')) {
            if (part.isEmpty()) continue
            val eq = part.indexOf('=')
            val key = if (eq < 0) part else part.substring(0, eq)
            val value = if (eq < 0) "" else part.substring(eq + 1)
            val (k, decoded) = try {
                URLDecoder.decode(key, "UTF-8") to URLDecoder.decode(value, "UTF-8")
            } catch (e: IllegalArgumentException) {
                invalid()
            }
            if (out.put(k, decoded) != null) invalid()
        }
        if (out["v"] != "1") {
            throw ShareException(ShareException.Reason.INVALID, "lien de partage d'une version plus récente : mettez l'application à jour")
        }
        return out
    }

    private fun invalid(): Nothing = throw ShareException(ShareException.Reason.INVALID, "lien de partage invalide")
}
