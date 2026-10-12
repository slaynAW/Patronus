package io.github.slaynaw.wakeonlan.core.agent2

import java.util.Base64

/**
 * Protocole `wolagent/2` (docs/PROTOCOLE-V2.md) : fonctions pures du protocole, vérifiées contre les vecteurs
 * communs avec l'agent Go (protocol/v2-vectors.json) — même implémentation de référence que agent/protocol2.
 */
object Wolagent2 {
    const val ALPN = "wolagent/2"
    const val DEFAULT_PORT = 9770
    const val MAX_REQUEST_BYTES = 16 * 1024
    const val MAX_RESPONSE_BYTES = 4 * 1024 * 1024
    const val MAX_REQUEST_ID = Int.MAX_VALUE.toLong()
    const val MAX_HELLO_BYTES = 256
    const val TLS_HANDSHAKE_RECORD = 0x16
    const val MAX_PC_NAME_LENGTH = 64
    const val MAX_DEVICE_NAME_LENGTH = 40
    const val NONCE_BYTES = 32
    const val DEFAULT_INVITE_TTL = 600L
    const val MIN_INVITE_TTL = 60L
    const val MAX_INVITE_TTL = 7 * 24 * 3600L

    /**
     * Nettoie un nom à afficher : caractères de contrôle (U+0000 à U+001F, U+007F à U+009F) retirés, espaces
     * retirés au début et à la fin, au plus [limit] unités UTF-16 sans couper de caractère.
     */
    fun cleanName(name: String, limit: Int): String {
        val cleaned = name.filterNot { it.code <= 0x1F || it.code in 0x7F..0x9F }.trim(' ')
        val out = StringBuilder()
        var i = 0
        while (i < cleaned.length) {
            val codePoint = cleaned.codePointAt(i)
            val units = Character.charCount(codePoint)
            if (out.length + units > limit) break
            out.appendCodePoint(codePoint)
            i += units
        }
        return out.toString().trimEnd(' ')
    }
}

/** Codes de réponse (§4.4). */
object ResponseCode {
    const val OK = "ok"
    const val BAD_REQUEST = "bad_request"
    const val UNSUPPORTED = "unsupported"
    const val BUSY = "busy"
    const val FORBIDDEN = "forbidden"
    const val INSUFFICIENT_ROLE = "insufficient_role"
    const val ALREADY_PAIRED = "already_paired"
    const val LAST_ADMIN = "last_admin"
    const val UNKNOWN_DEVICE = "unknown_device"
    const val PAIRING_FAILED = "pairing_failed"
    const val MIGRATION_FAILED = "migration_failed"
    const val RATE_LIMITED = "rate_limited"
}

/** Écriture « B64 » de la spécification : Base64 URL sans remplissage, sous sa seule forme canonique. */
internal object B64 {
    private val encoder = Base64.getUrlEncoder().withoutPadding()
    private val decoder = Base64.getUrlDecoder()
    private val ALPHABET = Regex("[A-Za-z0-9_-]*")

    fun encode(bytes: ByteArray): String = encoder.encodeToString(bytes)

    /** Décode exactement [size] octets : ni remplissage, ni retour à la ligne, ni bits de fin non nuls. */
    fun decodeExact(text: String, size: Int): ByteArray? {
        if (text.length != (size * 8 + 5) / 6 || !ALPHABET.matches(text)) return null
        val bytes = runCatching { decoder.decode(text) }.getOrNull() ?: return null
        return bytes.takeIf { it.size == size && encode(it) == text }
    }
}

/** Rôle d'un appareil enregistré (§5) ; l'ordre des valeurs est celui des droits, cumulatifs. */
enum class Role(val wire: String) {
    VIEW("view"),
    WAKE("wake"),
    CONTROL("control"),
    ADMIN("admin"),
    ;

    companion object {
        fun parse(text: String): Role? = entries.firstOrNull { it.wire == text }
    }
}

/** Commandes et contrôle d'accès de l'agent (§5). */
object Commands {
    const val DEVICES = "devices"
    const val DEVICES_REVOKE = "devices.revoke"
    const val DEVICES_ROLE = "devices.role"
    const val DEVICES_RENAME = "devices.rename"
    const val INVITE = "invite"
    const val INVITES = "invites"
    const val INVITES_CANCEL = "invites.cancel"
    const val PAIR = "pair"
    const val MIGRATE = "migrate"

    /** Rôle minimal de chaque commande ; pair et migrate n'y figurent pas (identités inconnues seulement). */
    val MIN_ROLE: Map<String, Role> = linkedMapOf(
        "status" to Role.VIEW, "history" to Role.VIEW, "metrics" to Role.VIEW, "specs" to Role.VIEW,
        "wakes" to Role.WAKE,
        "shutdown" to Role.CONTROL, "reboot" to Role.CONTROL, "sleep" to Role.CONTROL,
        DEVICES to Role.ADMIN, DEVICES_REVOKE to Role.ADMIN, DEVICES_ROLE to Role.ADMIN, DEVICES_RENAME to Role.ADMIN,
        INVITE to Role.ADMIN, INVITES to Role.ADMIN, INVITES_CANCEL to Role.ADMIN,
    )

    /** Toutes les commandes connues, appairage compris, dans l'ordre de la spécification. */
    val ALL: List<String> = MIN_ROLE.keys.toList() + listOf(PAIR, MIGRATE)

    fun isEnrollment(cmd: String): Boolean = cmd == PAIR || cmd == MIGRATE

    /**
     * Vérifications 1 à 3 de l'agent (§5) pour une identité enregistrée avec [role], ou inconnue ([registered]
     * faux, [role] ignoré). Renvoie le code de réponse ; [ResponseCode.OK] : la commande passe ensuite par la
     * liste blanche du PC.
     */
    fun authorize(registered: Boolean, role: Role?, cmd: String): String {
        if (!registered) return if (isEnrollment(cmd)) ResponseCode.OK else ResponseCode.UNKNOWN_DEVICE
        if (isEnrollment(cmd)) return ResponseCode.ALREADY_PAIRED
        val minimum = MIN_ROLE[cmd] ?: return ResponseCode.UNSUPPORTED
        return if (role != null && role >= minimum) ResponseCode.OK else ResponseCode.INSUFFICIENT_ROLE
    }
}
