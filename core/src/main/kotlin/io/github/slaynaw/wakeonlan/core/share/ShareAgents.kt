package io.github.slaynaw.wakeonlan.core.share

import io.github.slaynaw.wakeonlan.core.agent2.DeviceInfo
import io.github.slaynaw.wakeonlan.core.agent2.InviteInfo
import io.github.slaynaw.wakeonlan.core.agent2.PairingCode
import io.github.slaynaw.wakeonlan.core.agent2.Role
import kotlinx.serialization.Serializable

/**
 * Partage v2, côté propriétaire : ce que l'application, administratrice d'un PC, y fait pour les personnes autorisées
 * (docs/PARTAGE.md, « Partage v2 ») : invitations réservées à leur appareil, alignement des rôles, retraits. Les
 * décisions sont pures ([planAgent]) ; l'application les exécute sur une session d'administration, puis enregistre le
 * résultat ([applyAgent]). Mêmes règles et mêmes vecteurs que Windows (protocol/share-v2-vectors.json).
 *
 * L'application ne touche jamais à son propre enregistrement ni à celui d'un administrateur : ils ne relèvent pas du
 * partage.
 */
object ShareAgents {
    /** Validité des invitations du partage : 7 jours, le maximum de l'agent. */
    const val INVITE_TTL_SECONDS = 604_800L

    /** Une invitation qui expire dans moins de ce délai (secondes) est remplacée. */
    const val INVITE_RENEWAL_SECONDS = 86_400L

    /**
     * Droit effectivement accordé sur un PC : « démarrer et consulter » demande le partage v2 (demande avec empreinte,
     * agent joint par la sienne) ; « démarrer et éteindre », le partage v2 ou la clé de l'agent (partage v1). À défaut :
     * « démarrer ».
     */
    fun allowedRight(right: ShareRight, identity: Boolean, pinned: Boolean, hasKey: Boolean): ShareRight {
        val v2 = identity && pinned
        return when {
            right == ShareRight.VIEW && v2 -> right
            right == ShareRight.FULL && (v2 || hasKey) -> right
            else -> ShareRight.WAKE
        }
    }
}

/** Ce PC se partage avec cette personne en v2 (enregistrement par l'agent). */
fun SharePerson.isV2(pinned: Boolean): Boolean = identity.isNotEmpty() && pinned

/** État d'un agent vu par un administrateur. */
@Serializable
data class AgentView(val devices: List<DeviceInfo> = emptyList(), val invites: List<InviteInfo> = emptyList())

/** Empreinte associée à un rôle : changement à demander à l'agent, ou enregistrement constaté. */
@Serializable
data class RoleChange(val identity: String, val role: String)

/** Invitation à créer, réservée à une empreinte. */
@Serializable
data class InviteRequest(val identity: String, val role: String, val name: String)

/** Ce qu'il faut faire sur un PC, dans l'ordre : annulations d'invitations, retraits, changements de rôle, invitations. */
@Serializable
data class AgentPlan(
    val cancel: List<String> = emptyList(),
    val revoke: List<String> = emptyList(),
    val roles: List<RoleChange> = emptyList(),
    val invite: List<InviteRequest> = emptyList(),
    /** Empreintes enregistrées par le PC, avec leur rôle une fois le plan exécuté. */
    val registered: List<RoleChange> = emptyList(),
    /** Retraits soldés une fois les annulations et les retraits faits. */
    val done: List<ShareRevocation> = emptyList(),
    /** Empreintes que le partage ne touche pas (cet appareil, un administrateur). */
    val skipped: List<String> = emptyList(),
) {
    /** Rien à demander à l'agent (l'état peut tout de même changer). */
    val isEmpty: Boolean get() = cancel.isEmpty() && revoke.isEmpty() && roles.isEmpty() && invite.isEmpty()
}

/** Résultat de l'exécution d'un plan : [changed] si annulations, retraits et rôles sont tous faits ; invitations créées. */
@Serializable
data class PlanResult(val changed: Boolean = false, val created: Map<String, InviteInfo> = emptyMap()) {
    override fun toString(): String = "PlanResult(changed=$changed, created=${created.size})"
}

/** Enregistrement voulu d'une empreinte sur un PC : le rôle le plus fort parmi les personnes qui la portent. */
private class Want(val identity: String, var role: Role, val people: MutableList<SharePerson>)

private fun ShareOwner.wanted(device: String): List<Want> {
    val out = ArrayList<Want>()
    for (p in people) {
        val role = p.rights[device]?.agentRole ?: continue
        if (p.identity.isEmpty()) continue
        val w = out.firstOrNull { it.identity == p.identity } ?: Want(p.identity, role, ArrayList()).also { out += it }
        if (role > w.role) w.role = role
        w.people += p
    }
    return out
}

private fun ShareOwner.wantsAgent(device: String, identity: String): Boolean =
    people.any { it.identity == identity && it.rights[device]?.agentRole != null }

private fun ShareOwner.pairingOf(person: String, device: String): SharePairing? =
    pairings.firstOrNull { it.person == person && it.device == device }

/**
 * La clé wolagent/1 du PC est transmise à quelqu'un (partage v1 avec le droit « démarrer et éteindre ») : l'application
 * doit alors la garder.
 */
fun ShareOwner.needsKey(device: String, pinned: Boolean): Boolean =
    people.any { it.rights[device] == ShareRight.FULL && !it.isV2(pinned) }

/**
 * Il y a quelque chose à faire sur un PC (joint en wolagent/2) : un retrait, une invitation à créer ou à renouveler
 * (premier élément). Second élément : des invitations attendent d'être utilisées ; l'état de l'agent mérite d'être relu
 * de temps en temps pour le constater.
 */
fun ShareOwner.agentWork(device: String, nowSeconds: Long): Pair<Boolean, Boolean> {
    if (revocations.any { it.device == device }) return true to false
    var poll = false
    for (w in wanted(device)) {
        for (p in w.people) {
            val x = pairingOf(p.device, device) ?: return true to false
            when {
                x.identity != w.identity -> return true to false
                // Un administrateur garde son rôle : le partage ne le change pas.
                x.registered -> if (x.role != w.role.wire && x.role != Role.ADMIN.wire) return true to false
                x.code.isEmpty() || x.role != w.role.wire || x.expires < nowSeconds + ShareAgents.INVITE_RENEWAL_SECONDS ->
                    return true to false
                else -> poll = true
            }
        }
    }
    return false to poll
}

/**
 * Décide des opérations sur un PC d'après l'état de son agent ([view]) :
 * - retraits en file : invitations réservées à l'empreinte annulées, enregistrement supprimé ;
 * - empreinte voulue et enregistrée : rôle aligné, invitations devenues inutiles annulées ;
 * - empreinte voulue, pas enregistrée : l'invitation connue est gardée si elle est encore en attente, au bon rôle et
 *   valable assez longtemps ; sinon les invitations réservées à cette empreinte sont annulées et une nouvelle est créée.
 */
fun ShareOwner.planAgent(device: String, view: AgentView, nowSeconds: Long): AgentPlan {
    val registered = view.devices.associateBy { it.device }
    fun protected(identity: String): Boolean = registered[identity]?.let { it.self || it.role == Role.ADMIN.wire } == true
    val cancel = ArrayList<String>()
    val revoke = ArrayList<String>()
    val roles = ArrayList<RoleChange>()
    val invite = ArrayList<InviteRequest>()
    val found = ArrayList<RoleChange>()
    val done = ArrayList<ShareRevocation>()
    val skipped = ArrayList<String>()
    fun cancelFor(identity: String) {
        for (inv in view.invites) if (inv.device == identity && inv.id !in cancel) cancel += inv.id
    }
    for (r in revocations) {
        if (r.device != device || wantsAgent(device, r.identity)) continue
        cancelFor(r.identity)
        when {
            protected(r.identity) -> skipped += r.identity
            r.identity in registered -> revoke += r.identity
        }
        done += r
    }
    for (w in wanted(device)) {
        val d = registered[w.identity]
        if (d != null) {
            cancelFor(w.identity)
            var role = d.role
            if (protected(w.identity)) {
                skipped += w.identity
            } else if (role != w.role.wire) {
                roles += RoleChange(w.identity, w.role.wire)
                role = w.role.wire
            }
            found += RoleChange(w.identity, role)
            continue
        }
        if (inviteValid(device, w, view, nowSeconds)) continue
        cancelFor(w.identity)
        invite += InviteRequest(w.identity, w.role.wire, w.people.first().name)
    }
    return AgentPlan(cancel, revoke, roles, invite, found, done, skipped)
}

/**
 * L'invitation connue pour une empreinte peut servir encore : en attente sur l'agent, réservée à cette empreinte, au
 * rôle voulu, valable au moins [ShareAgents.INVITE_RENEWAL_SECONDS], et connue de chaque personne qui porte l'empreinte.
 */
private fun ShareOwner.inviteValid(device: String, w: Want, view: AgentView, nowSeconds: Long): Boolean {
    var id: String? = null
    for (p in w.people) {
        val x = pairingOf(p.device, device) ?: return false
        if (x.identity != w.identity || x.registered || x.code.isEmpty() || x.role != w.role.wire ||
            x.expires < nowSeconds + ShareAgents.INVITE_RENEWAL_SECONDS || (id != null && x.invite != id)
        ) {
            return false
        }
        id = x.invite
    }
    return view.invites.any { it.id == id && it.device == w.identity }
}

/**
 * Enregistre le résultat d'un plan exécuté sur un PC. L'état a pu changer pendant les échanges : seules les personnes qui
 * veulent encore ces enregistrements sont mises à jour, et une invitation créée pour une empreinte qui n'est plus voulue
 * part dans la file des retraits. Renvoie le nouvel état, et s'il a changé.
 */
fun ShareOwner.applyAgent(device: String, plan: AgentPlan, result: PlanResult): Pair<ShareOwner, Boolean> {
    var o = this
    if (result.changed) {
        o = o.copy(revocations = o.revocations.filterNot { it in plan.done })
        for (r in plan.registered) o = o.setPairings(device, r.identity, SharePairing("", "", "", r.role, registered = true))
    }
    for (req in plan.invite) {
        val inv = result.created[req.identity] ?: continue
        val code = PairingCode.normalize(inv.code) ?: continue
        if (!o.wantsAgent(device, req.identity)) {
            val r = ShareRevocation(device, req.identity)
            if (r !in o.revocations) o = o.copy(revocations = o.revocations + r)
            continue
        }
        o = o.setPairings(device, req.identity, SharePairing("", "", "", inv.role, invite = inv.id, code = code, expires = inv.expires))
    }
    return o to (o.revocations != revocations || o.pairings != pairings)
}

/** Écrit l'appairage d'une empreinte sur un PC pour chaque personne qui la porte et veut joindre ce PC par l'agent. */
private fun ShareOwner.setPairings(device: String, identity: String, x: SharePairing): ShareOwner {
    var list = pairings
    for (p in people) {
        if (p.identity != identity || p.rights[device]?.agentRole == null) continue
        val next = x.copy(person = p.device, device = device, identity = identity)
        val i = list.indexOfFirst { it.person == p.device && it.device == device }
        list = if (i >= 0) list.toMutableList().also { it[i] = next } else list + next
    }
    return copy(pairings = list)
}
