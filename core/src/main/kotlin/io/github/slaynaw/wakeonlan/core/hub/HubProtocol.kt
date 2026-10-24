package io.github.slaynaw.wakeonlan.core.hub

import io.github.slaynaw.wakeonlan.core.agent2.Commands
import io.github.slaynaw.wakeonlan.core.agent2.InvalidLinkException
import io.github.slaynaw.wakeonlan.core.agent2.LinkFormat
import io.github.slaynaw.wakeonlan.core.agent2.PairingLinkV2
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.agent2.Role

/**
 * Protocole « patronus-hub/1 » entre les applications et le hub sur le NAS (docs/PROTOCOLE-HUB.md) : même
 * transport que wolagent/2, avec son propre protocole applicatif, son port, ses rôles et ses commandes. Mêmes
 * règles que l'implémentation de référence en Go (agent/hubprotocol), vérifiées par protocol/hub-vectors.json.
 */
object HubProtocol {
    const val ALPN = "patronus-hub/1"
    const val DEFAULT_PORT = 9771
    const val LINK_PREFIX = "patronus://hub?"

    /** Lien d'appairage du hub : paramètres du lien v=2 d'un agent, « a » portant l'empreinte du hub (§7). */
    val LINK = LinkFormat(LINK_PREFIX, "1", DEFAULT_PORT)

    /** Rôles d'un appareil sur le hub (§5) : pas de « control », le hub ne relaie aucune commande. */
    val ROLES: List<Role> = listOf(Role.VIEW, Role.WAKE, Role.ADMIN)

    const val STATUS = "hub.status"
    const val EVENTS = "hub.events"
    const val HISTORY = "hub.history"
    const val METRICS = "hub.metrics"
    const val WAKE = "hub.wake"
    const val PCS = "hub.pcs"
    const val PCS_ADD = "hub.pcs.add"
    const val PCS_UPDATE = "hub.pcs.update"
    const val PCS_REMOVE = "hub.pcs.remove"
    const val SCHEDULES = "hub.schedules"
    const val SCHEDULES_SET = "hub.schedules.set"
    const val SETTINGS = "hub.settings"

    /** Rôle minimal de chaque commande, dans l'ordre de la spécification (§6) ; pair n'y figure pas. */
    val MIN_ROLE: Map<String, Role> = linkedMapOf(
        STATUS to Role.VIEW, EVENTS to Role.VIEW, HISTORY to Role.VIEW, METRICS to Role.VIEW,
        WAKE to Role.WAKE,
        PCS to Role.ADMIN, PCS_ADD to Role.ADMIN, PCS_UPDATE to Role.ADMIN, PCS_REMOVE to Role.ADMIN,
        SCHEDULES to Role.ADMIN, SCHEDULES_SET to Role.ADMIN, SETTINGS to Role.ADMIN,
        Commands.DEVICES to Role.ADMIN, Commands.DEVICES_REVOKE to Role.ADMIN, Commands.DEVICES_ROLE to Role.ADMIN,
        Commands.DEVICES_RENAME to Role.ADMIN, Commands.INVITE to Role.ADMIN, Commands.INVITES to Role.ADMIN,
        Commands.INVITES_CANCEL to Role.ADMIN,
    )

    /** Toutes les commandes, appairage compris, dans l'ordre de la spécification. */
    val ALL: List<String> = MIN_ROLE.keys.toList() + Commands.PAIR

    /**
     * Vérifications du hub (§5), dans l'ordre : identité enregistrée (sinon seule pair passe), commande connue, rôle
     * suffisant. Un rôle inconnu du hub (control) ne donne aucun droit.
     */
    fun authorize(registered: Boolean, role: Role?, cmd: String): String {
        if (!registered) return if (cmd == Commands.PAIR) ResponseCode.OK else ResponseCode.UNKNOWN_DEVICE
        if (cmd == Commands.PAIR) return ResponseCode.ALREADY_PAIRED
        val minimum = MIN_ROLE[cmd] ?: return ResponseCode.UNSUPPORTED
        return if (role != null && role in ROLES && role >= minimum) ResponseCode.OK else ResponseCode.INSUFFICIENT_ROLE
    }

    /** Précisions des évènements (§6.5) admises pour chaque type ; liste vide : aucune. */
    private val EVENT_DETAILS: Map<String, List<String>> = mapOf(
        "online" to emptyList(), "offline" to emptyList(), "agent_down" to emptyList(),
        "measure" to emptyList(), "wake" to emptyList(),
        "problem" to listOf("revoked", "pin_mismatch", "rate_limited"),
        "lost" to listOf("", "bsod", "button", "power", "hardware"),
        "disk" to listOf("health", "full"),
    )
    private val MEASURE_ID = Regex("[a-z0-9][a-z0-9._-]{0,23}")
    private val SHORT_PIN = Regex("[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}")

    /** Réveil demandé sur le NAS (HubEvent.i). */
    const val WAKE_LOCAL = "local"

    /**
     * Un évènement n'est fait que de codes connus (§6.5). L'application écarte les autres sans rien en afficher : un
     * hub compromis pourrait y glisser du texte ou un lien (T38).
     */
    fun validEvent(e: HubEvent): Boolean {
        val allowed = EVENT_DETAILS[e.k] ?: return false
        return when {
            e.t <= 0 || e.pc.isEmpty() -> false
            e.k == "measure" -> MEASURE_ID.matches(e.x) && e.i.isEmpty()
            e.k == "wake" -> e.x.isEmpty() && (e.i == WAKE_LOCAL || SHORT_PIN.matches(e.i))
            allowed.isEmpty() -> e.x.isEmpty() && e.i.isEmpty()
            else -> e.x in allowed && e.i.isEmpty()
        }
    }

    /**
     * Lit un lien d'appairage du hub ; un rôle annoncé doit être un rôle du hub.
     *
     * @throws InvalidLinkException
     */
    fun parseLink(text: String): PairingLinkV2 {
        val link = PairingLinkV2.parse(text, LINK)
        if (link.role != null && link.role !in ROLES) throw InvalidLinkException("r")
        return link
    }
}
