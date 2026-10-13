package io.github.slaynaw.wakeonlan.core.agent

import io.github.slaynaw.wakeonlan.core.agent2.InvalidLinkException
import io.github.slaynaw.wakeonlan.core.agent2.PairingLinkV2
import io.github.slaynaw.wakeonlan.core.model.DEFAULT_AGENT_PORT
import io.github.slaynaw.wakeonlan.core.model.DeviceValidator
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import java.net.URLDecoder
import java.net.URLEncoder

/** Informations d'appairage affichées par l'agent (QR code ou lien). */
data class PairingInfo(
    val name: String,
    val host: String,
    val port: Int,
    val mac: MacAddress?,
    val key: String,
) {
    override fun toString(): String = "PairingInfo(name=$name, host=$host, port=$port, mac=$mac, key=***)"
}

/** Lien d'appairage lu : version 1 (clé de l'agent) ou 2 (empreinte de l'agent et code à usage unique). */
sealed interface ParsedPairing {
    data class V1(val info: PairingInfo) : ParsedPairing

    data class V2(val link: PairingLinkV2) : ParsedPairing
}

/**
 * Lien d'appairage : `wolagent://pair?v=1&n=<nom>&h=<ip>&p=<port>&m=<mac>&k=<clé>`, ou `v=2` (docs/PROTOCOLE-V2.md,
 * §7.1 : [PairingLinkV2]). Il est généré par `wol-agent pair` sur le PC et scanné / collé dans l'application.
 */
object PairingLink {
    const val PREFIX = "wolagent://pair?"
    const val VERSION = "1"

    fun build(info: PairingInfo): String = PREFIX + listOfNotNull(
        "v" to VERSION,
        "n" to info.name,
        "h" to info.host,
        "p" to info.port.toString(),
        info.mac?.let { "m" to it.toString() },
        "k" to info.key,
    ).joinToString("&") { (k, v) -> "$k=${URLEncoder.encode(v, "UTF-8")}" }

    /** Message d'un lien `v=2` invalide, selon le paramètre en cause. */
    private val V2_ERRORS = mapOf(
        "duplicate" to "Lien d'appairage mal formé",
        "h" to "Adresse du PC invalide dans le lien",
        "a" to "Empreinte de l'agent invalide dans le lien",
        "c" to "Code d'appairage invalide dans le lien",
        "p" to "Port invalide dans le lien",
        "m" to "Adresse MAC invalide dans le lien",
        "r" to "Rôle invalide dans le lien",
        "e" to "Date d'expiration invalide dans le lien",
    )

    /**
     * Lit un lien de l'une ou l'autre version : `v=2` selon les règles de la spécification (paramètre en double refusé),
     * les autres comme avant.
     *
     * @throws IllegalArgumentException avec un message lisible si le lien est invalide.
     */
    fun parseAny(text: String): ParsedPairing {
        try {
            return ParsedPairing.V2(PairingLinkV2.parse(text))
        } catch (e: InvalidLinkException) {
            V2_ERRORS[e.field]?.let { throw IllegalArgumentException(it) }
        }
        return ParsedPairing.V1(parse(text))
    }

    /** Lit un lien `v=1`. @throws IllegalArgumentException avec un message lisible si le lien est invalide. */
    fun parse(text: String): PairingInfo {
        val link = text.trim()
        require(link.startsWith(PREFIX, ignoreCase = true)) { "Ce n'est pas un lien d'appairage wolagent://" }
        val params = link.substring(PREFIX.length).split('&')
            .filter { it.isNotEmpty() }
            .associate { part ->
                val eq = part.indexOf('=')
                require(eq > 0) { "Lien d'appairage mal formé" }
                part.substring(0, eq) to URLDecoder.decode(part.substring(eq + 1), "UTF-8")
            }
        require(params["v"] == VERSION) { "Version de lien non prise en charge : ${params["v"]}" }

        val host = params["h"].orEmpty()
        require(DeviceValidator.isValidHost(host)) { "Adresse du PC invalide dans le lien" }
        val port = params["p"]?.toIntOrNull() ?: DEFAULT_AGENT_PORT
        require(DeviceValidator.isValidPort(port)) { "Port invalide dans le lien" }
        val key = params["k"].orEmpty()
        require(AgentKey.decodeOrNull(key) != null) { "Clé invalide dans le lien" }
        val mac = params["m"]?.takeIf { it.isNotEmpty() }?.let {
            MacAddress.parseOrNull(it) ?: throw IllegalArgumentException("Adresse MAC invalide dans le lien")
        }
        val name = params["n"].orEmpty().trim().take(DeviceValidator.MAX_NAME_LENGTH).filterNot { it.isISOControl() }
        return PairingInfo(name = name.ifEmpty { host }, host = host, port = port, mac = mac, key = key)
    }
}
