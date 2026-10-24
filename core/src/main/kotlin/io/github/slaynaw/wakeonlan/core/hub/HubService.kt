package io.github.slaynaw.wakeonlan.core.hub

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentHistoryEvent
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.MetricsRow
import io.github.slaynaw.wakeonlan.core.agent2.Commands
import io.github.slaynaw.wakeonlan.core.agent2.DeviceIdentity
import io.github.slaynaw.wakeonlan.core.agent2.DeviceInfo
import io.github.slaynaw.wakeonlan.core.agent2.InvalidLinkException
import io.github.slaynaw.wakeonlan.core.agent2.InviteInfo
import io.github.slaynaw.wakeonlan.core.agent2.PairingCode
import io.github.slaynaw.wakeonlan.core.agent2.PairingLinkV2
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.agent2.Role
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Exception
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.DeviceValidator
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import io.github.slaynaw.wakeonlan.core.net.NetworkBinder
import io.github.slaynaw.wakeonlan.core.net.useCancellable
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import java.io.Closeable
import java.io.IOException
import java.net.ConnectException
import java.net.Socket
import java.net.UnknownHostException

/**
 * Hub appairé, tel que l'application le garde (docs/HUB.md, §5.2). Comme l'identité de l'appareil, il n'est ni exporté
 * ni sauvegardé : l'appairage appartient à cette installation.
 */
@Serializable
data class PairedHub(
    val name: String,
    val host: String,
    val port: Int,
    /** Empreinte complète du hub. */
    val pin: String,
    /** Rôle de cet appareil sur le hub (view, wake ou admin). */
    val role: String,
    /** Date de l'appairage (secondes Unix). */
    val paired: Long,
    /** Heure (horloge du hub, secondes Unix) du dernier évènement vu ; 0 : aucun. */
    val eventsSeen: Long = 0,
) {
    val shortPin: String get() = Pin.parse(pin)?.short().orEmpty()

    /** Début des évènements à demander : après le dernier vu, ou depuis l'appairage. */
    val eventsSince: Long get() = if (eventsSeen > 0) eventsSeen + 1 else paired

    val canWake: Boolean get() = role == Role.WAKE.wire || role == Role.ADMIN.wire

    val isAdmin: Boolean get() = role == Role.ADMIN.wire
}

/** Lecture d'un lien collé dans « Appairer un hub ». */
sealed interface HubLinkResult {
    data class Valid(val link: PairingLinkV2) : HubLinkResult

    /** Lien d'appairage d'un PC (wolagent://pair) : à ajouter comme PC. */
    data object AgentLink : HubLinkResult

    data object Expired : HubLinkResult

    data object Invalid : HubLinkResult

    companion object {
        /** Lit [text] à l'heure [nowSeconds]. */
        fun read(text: String, nowSeconds: Long): HubLinkResult {
            val link = try {
                HubProtocol.parseLink(text)
            } catch (_: InvalidLinkException) {
                val agent = runCatching { PairingLinkV2.parse(text) }.isSuccess
                return if (agent) AgentLink else Invalid
            }
            return if (link.expired(nowSeconds)) Expired else Valid(link)
        }
    }
}

/** Appairage réussi : le hub, et [already] si cet appareil était déjà autorisé (le code n'a pas servi). */
data class HubPairing(val hub: PairedHub, val already: Boolean)

/** Relevé du hub : son état, ses PC et leur état, ses évènements non vus (codes connus seulement, T38). */
data class HubSnapshot(
    val info: HubInfo,
    val role: String,
    val pcs: List<HubPc>,
    val states: List<HubPcState>,
    val events: List<HubEvent>,
    /** Il reste des évènements après ceux-ci. */
    val moreEvents: Boolean,
    /** Évènements écartés car faits d'autre chose que de codes connus. */
    val dropped: Int,
) {
    /** PC du hub qui surveille l'agent d'empreinte [agentPin]. */
    fun pcOfAgent(agentPin: String): HubPc? = pcs.firstOrNull { it.agent.isNotEmpty() && it.agent == agentPin }

    fun stateOf(pcId: String): HubPcState? = states.firstOrNull { it.id == pcId }
}

/** Vue d'administration du hub. */
data class HubAdmin(val devices: List<DeviceInfo>, val invites: List<InviteInfo>, val retention: Int)

/** Invitation créée sur le hub : lien patronus://hub (QR code) et code à transmettre. */
data class HubCreatedInvite(val link: String, val code: String, val role: String, val expires: Long) {
    override fun toString(): String = "HubCreatedInvite(role=$role, expires=$expires)"
}

/** PC confié au hub (§5.3) : résultat, avec l'étape refusée. */
sealed interface EntrustResult {
    data class Success(val pc: HubPc) : EntrustResult

    /** Le PC n'a pas créé l'invitation (éteint, ou cet appareil n'en est pas administrateur). */
    data class AgentFailed(val failure: AgentResult.Failure) : EntrustResult

    /** Le hub a refusé ou n'a pas pu s'appairer au PC ; l'invitation a été annulée. */
    data class HubFailed(val failure: AgentResult.Failure) : EntrustResult

    data object AlreadyEntrusted : EntrustResult
}

/** Retrait d'un PC du hub : [agentRevoked] null pour un appareil sans agent, sinon le retrait du hub sur le PC. */
data class HubRemoval(val pc: HubPc, val agentRevoked: Boolean?)

/** Champ refusé d'un appareil sans agent. */
enum class HubDeviceField { NAME, HOST, MAC, PROBE, PORT }

/**
 * Service du hub pour les applications : appairage, relevés, administration, PC confiés, appareils sans agent, réveil
 * et archive (docs/PROTOCOLE-HUB.md). Chaque appel ouvre une session (empreinte du hub vérifiée avant tout envoi) et
 * rend un [AgentResult] : un refus du hub est un échec REJECTED avec son code. Annulable à tout moment.
 */
class HubService(
    binder: NetworkBinder = NetworkBinder.Default,
    private val deviceName: () -> String? = { null },
    private val identity: () -> DeviceIdentity? = { null },
    connectTimeoutMs: Int = 2_000,
    readTimeoutMs: Int = 20_000,
) {
    private val client = HubClient(binder, connectTimeoutMs, readTimeoutMs)

    /** Appaire cet appareil au hub du lien [link] (§5.2), puis lit son nom ; [now] en secondes. */
    suspend fun pair(link: PairingLinkV2, now: Long): AgentResult<HubPairing> {
        val target = PairedHub(name = link.name, host = link.host, port = link.port, pin = link.agent.toString(), role = "", paired = now)
        return session(target) { s ->
            val r = s.request(HubRequest(cmd = Commands.PAIR, code = link.code, name = name()))
            val already = !r.ok && r.code == ResponseCode.ALREADY_PAIRED
            when {
                r.code.isEmpty() -> return@session protocolError("réponse sans code")
                !r.ok && r.code == ResponseCode.PAIRING_FAILED -> return@session AgentResult.Failure(AgentError.PAIRING_FAILED, r.message)
                !r.ok && !already -> return@session refusal(r)
            }
            val status = s.request(HubRequest(cmd = HubProtocol.STATUS))
            val name = status.hub?.name?.trim().orEmpty().ifEmpty { link.name }.ifEmpty { "Hub" }
            val role = status.role.ifEmpty { r.role }
            AgentResult.Success(HubPairing(target.copy(name = name, role = role), already))
        }
    }

    /** Relevé du hub : hub.status puis les évènements non vus (hub.events), sur une session. */
    suspend fun read(hub: PairedHub): AgentResult<HubSnapshot> = session(hub) { s ->
        val status = when (val r = call(s, HubRequest(cmd = HubProtocol.STATUS))) {
            is AgentResult.Success -> r.value
            is AgentResult.Failure -> return@session r
        }
        val info = status.hub ?: return@session protocolError("état du hub absent")
        val since = hub.eventsSince
        val events = when (val r = call(s, HubRequest(cmd = HubProtocol.EVENTS, since = since))) {
            is AgentResult.Success -> r.value
            is AgentResult.Failure -> return@session r
        }
        val kept = events.events.filter { it.t >= since && HubProtocol.validEvent(it) }
        AgentResult.Success(
            HubSnapshot(info, status.role, status.pcs, status.states, kept, events.more, events.events.size - kept.size),
        )
    }

    /** Appareils autorisés, invitations en attente et durée de conservation (rôle admin), sur une session. */
    suspend fun admin(hub: PairedHub): AgentResult<HubAdmin> = session(hub) { s ->
        val devices = when (val r = call(s, HubRequest(cmd = Commands.DEVICES))) {
            is AgentResult.Success -> r.value.devices
            is AgentResult.Failure -> return@session r
        }
        val invites = when (val r = call(s, HubRequest(cmd = Commands.INVITES))) {
            is AgentResult.Success -> r.value.invites
            is AgentResult.Failure -> return@session r
        }
        val retention = when (val r = call(s, HubRequest(cmd = HubProtocol.SETTINGS))) {
            is AgentResult.Success -> r.value.settings?.retention ?: 0
            is AgentResult.Failure -> return@session r
        }
        AgentResult.Success(HubAdmin(devices, invites, retention))
    }

    /** Envoie une requête au hub (retrait, rôle, nom d'un appareil, annulation d'invitation, réglages, réveil…). */
    suspend fun request(hub: PairedHub, request: HubRequest): AgentResult<HubResponse> = session(hub) { s -> call(s, request) }

    /** Crée sur le hub une invitation pour un autre appareil, avec le lien patronus://hub et son code. */
    suspend fun invite(hub: PairedHub, role: Role, ttlSeconds: Long, name: String): AgentResult<HubCreatedInvite> {
        if (role !in HubProtocol.ROLES) return AgentResult.Failure(AgentError.REJECTED, code = ResponseCode.BAD_REQUEST)
        val pin = Pin.parse(hub.pin) ?: return protocolError("empreinte enregistrée invalide")
        return session(hub) { s ->
            val request = HubRequest(cmd = Commands.INVITE, role = role.wire, ttl = ttlSeconds, name = name.trim().ifEmpty { null })
            val invite = when (val r = call(s, request)) {
                is AgentResult.Success -> r.value.invite
                is AgentResult.Failure -> return@session r
            } ?: return@session protocolError("invitation absente de la réponse")
            val code = PairingCode.normalize(invite.code)
            if (code == null || invite.hub != pin.toString()) return@session protocolError("invitation d'un autre hub")
            val link = PairingLinkV2(name = hub.name, host = hub.host, port = hub.port, mac = "", agent = pin, code = code, role = role,
                expires = invite.expires).text(HubProtocol.LINK)
            AgentResult.Success(HubCreatedInvite(link, PairingCode.format(code), role.wire, invite.expires))
        }
    }

    /**
     * Confie le PC [device] au hub (§5.3) : l'agent crée une invitation réservée à l'empreinte du hub, au rôle « wake »
     * ([agents], cet appareil administrateur du PC), puis le hub s'en sert pour s'appairer (hub.pcs.add). Si le hub
     * refuse, l'invitation est annulée sur le PC.
     */
    suspend fun entrust(hub: PairedHub, agents: AgentClient, device: Device, snapshot: HubSnapshot?): EntrustResult {
        val agent = device.agent
        if (agent == null || !agent.hasPin || !device.hasHost) {
            return EntrustResult.AgentFailed(AgentResult.Failure(AgentError.NO_KEY))
        }
        if (snapshot?.pcOfAgent(agent.pin) != null) return EntrustResult.AlreadyEntrusted
        val invite = when (val r = agents.invite(device.host, agent, Role.WAKE.wire, ENTRUST_INVITE_TTL, device = hub.pin)) {
            is AgentResult.Success -> r.value
            is AgentResult.Failure -> return EntrustResult.AgentFailed(r)
        }
        val request = HubRequest(cmd = HubProtocol.PCS_ADD, name = device.name, host = device.host, port = agent.port, mac = wakeMac(device.mac),
            agent = agent.pin, code = invite.code)
        return when (val r = request(hub, request)) {
            is AgentResult.Success -> r.value.pc?.let { EntrustResult.Success(it) }
                ?: EntrustResult.HubFailed(AgentResult.Failure(AgentError.PROTOCOL, "PC absent de la réponse"))
            is AgentResult.Failure -> {
                agents.cancelInvite(device.host, agent, invite.id)
                EntrustResult.HubFailed(r)
            }
        }
    }

    /**
     * Retire un PC ou un appareil du hub. Pour un PC avec agent, [local] est ce PC sur cet appareil (s'il le connaît) :
     * le hub est aussi retiré de ses appareils autorisés, ce qu'il ne peut pas faire lui-même.
     */
    suspend fun remove(hub: PairedHub, agents: AgentClient, pcId: String, local: Device?): AgentResult<HubRemoval> {
        val pc = when (val r = request(hub, HubRequest(cmd = HubProtocol.PCS_REMOVE, pc = pcId))) {
            is AgentResult.Success -> r.value.pc ?: return protocolError("PC absent de la réponse")
            is AgentResult.Failure -> return r
        }
        if (pc.agent.isEmpty()) return AgentResult.Success(HubRemoval(pc, null))
        val agent = local?.agent?.takeIf { it.pin == pc.agent && local.hasHost }
            ?: return AgentResult.Success(HubRemoval(pc, false))
        val revoked = when (val r = agents.revokeDevice(local.host, agent, hub.pin)) {
            is AgentResult.Success -> true
            // Appareil inconnu de l'agent : le hub en était déjà retiré.
            is AgentResult.Failure -> r.code == ResponseCode.BAD_REQUEST
        }
        return AgentResult.Success(HubRemoval(pc, revoked))
    }

    /** Ajoute au hub un appareil sans agent (§5.4) ; [validateDevice] d'abord. */
    suspend fun addDevice(hub: PairedHub, name: String, host: String, mac: String, probe: HubProbe): AgentResult<HubPc> {
        val request = HubRequest(cmd = HubProtocol.PCS_ADD, name = name.trim(), host = host.trim(),
            mac = MacAddress.parseOrNull(mac)?.toString(), probe = probe)
        return when (val r = request(hub, request)) {
            is AgentResult.Success -> r.value.pc?.let { AgentResult.Success(it) } ?: protocolError("appareil absent de la réponse")
            is AgentResult.Failure -> r
        }
    }

    /** Journal du PC [pcId] gardé par le hub entre [from] et [to] (secondes), page par page. */
    suspend fun journal(hub: PairedHub, pcId: String, from: Long, to: Long): AgentResult<List<AgentHistoryEvent>> = session(hub) { s ->
        val out = mutableListOf<AgentHistoryEvent>()
        var next = from
        repeat(JOURNAL_PAGES) {
            val r = when (val c = call(s, HubRequest(cmd = HubProtocol.HISTORY, pc = pcId, from = next))) {
                is AgentResult.Success -> c.value
                is AgentResult.Failure -> return@session c
            }
            val events = r.history?.events.orEmpty()
            for (e in events) {
                if (e.t >= to) return@session AgentResult.Success(out)
                if (e.t >= from) out += e
            }
            if (!r.more || events.isEmpty()) return@session AgentResult.Success(out)
            next = events.last().t + 1
        }
        AgentResult.Success(out)
    }

    /** Jours de mesures (« 2026-10-03 ») gardés par le hub pour le PC [pcId]. */
    suspend fun metricsDays(hub: PairedHub, pcId: String): AgentResult<List<String>> =
        when (val r = request(hub, HubRequest(cmd = HubProtocol.METRICS, pc = pcId))) {
            is AgentResult.Success -> AgentResult.Success(r.value.metrics?.days.orEmpty())
            is AgentResult.Failure -> r
        }

    /** Mesures des jours [wanted] que le hub garde pour le PC [pcId], sur une seule session. */
    suspend fun metrics(hub: PairedHub, pcId: String, wanted: List<String>): AgentResult<Map<String, List<MetricsRow>>> {
        if (wanted.isEmpty()) return AgentResult.Success(emptyMap())
        return session(hub) { s ->
            val kept = when (val r = call(s, HubRequest(cmd = HubProtocol.METRICS, pc = pcId))) {
                is AgentResult.Success -> r.value.metrics?.days.orEmpty().toSet()
                is AgentResult.Failure -> return@session r
            }
            val out = linkedMapOf<String, List<MetricsRow>>()
            for (day in wanted) {
                if (day !in kept) continue
                when (val r = call(s, HubRequest(cmd = HubProtocol.METRICS, pc = pcId, day = day))) {
                    is AgentResult.Success -> out[day] = r.value.metrics?.rows.orEmpty()
                    is AgentResult.Failure -> return@session r
                }
            }
            AgentResult.Success(out)
        }
    }

    private fun name(): String? = deviceName()?.trim()?.take(MAX_NAME_LENGTH)?.takeIf { it.isNotEmpty() }

    /** Une requête ; un refus du hub devient un échec (UNREGISTERED pour un appareil retiré, REJECTED sinon). */
    private fun call(session: HubSession, request: HubRequest): AgentResult<HubResponse> {
        val r = session.request(request)
        return when {
            r.code.isEmpty() -> protocolError("réponse sans code")
            !r.ok && r.code == ResponseCode.UNKNOWN_DEVICE -> AgentResult.Failure(AgentError.UNREGISTERED, r.message)
            !r.ok -> refusal(r)
            else -> AgentResult.Success(r)
        }
    }

    private fun refusal(r: HubResponse): AgentResult.Failure = AgentResult.Failure(AgentError.REJECTED, r.message, r.code)

    /** Ouvre une session avec [hub] (empreinte exigée), y mène [run], puis la ferme. */
    private suspend fun <T> session(hub: PairedHub, run: (HubSession) -> AgentResult<T>): AgentResult<T> {
        val pin = Pin.parse(hub.pin) ?: return protocolError("empreinte enregistrée invalide")
        val id = withContext(Dispatchers.IO) { runCatching { identity() }.getOrNull() }
            ?: return AgentResult.Failure(AgentError.NO_IDENTITY)
        val call = Call()
        return try {
            call.useCancellable { c -> client.connect(hub.host, hub.port, id, pin, onSocket = c::attach).use(run) }
        } catch (e: Wolagent2Exception) {
            when (e.reason) {
                Wolagent2Exception.Reason.PIN_MISMATCH -> AgentResult.Failure(AgentError.PIN_MISMATCH, e.message)
                Wolagent2Exception.Reason.RATE_LIMITED -> AgentResult.Failure(AgentError.RATE_LIMITED, e.message)
                Wolagent2Exception.Reason.NOT_AGENT -> AgentResult.Failure(AgentError.OTHER_SERVICE, e.message)
                else -> AgentResult.Failure(AgentError.PROTOCOL, e.message)
            }
        } catch (e: UnknownHostException) {
            AgentResult.Failure(AgentError.UNKNOWN_HOST, e.message)
        } catch (e: ConnectException) {
            val refused = e.message?.contains("refused", ignoreCase = true) == true
            AgentResult.Failure(if (refused) AgentError.REFUSED else AgentError.UNREACHABLE, e.message)
        } catch (e: IOException) {
            AgentResult.Failure(if (call.connected) AgentError.PROTOCOL else AgentError.UNREACHABLE, e.message)
        }
    }

    /** Connexion d'une session en cours, fermée si l'appel est annulé. */
    private class Call : Closeable {
        @Volatile private var socket: Socket? = null

        @Volatile private var closed = false

        val connected: Boolean get() = socket?.isConnected == true

        fun attach(s: Socket) {
            socket = s
            if (closed) runCatching { s.close() }
        }

        override fun close() {
            closed = true
            socket?.let { runCatching { it.close() } }
        }
    }

    companion object {
        /** Validité de l'invitation de l'agent transmise au hub (elle sert aussitôt). */
        const val ENTRUST_INVITE_TTL = 600L

        /** Durée de conservation de l'archive du hub, en jours (§6.4). */
        const val MIN_RETENTION = 90
        const val MAX_RETENTION = 1095

        private const val JOURNAL_PAGES = 20
        private const val MAX_NAME_LENGTH = 64

        /** Adresse MAC du PC pour le réveil par le hub, sauf valeur qu'il refuse. */
        fun wakeMac(mac: MacAddress): String? = mac.toString().takeIf { it != "00:00:00:00:00:00" && it != "FF:FF:FF:FF:FF:FF" }

        /** Vérifie un appareil sans agent avant de l'envoyer au hub : null si tout va bien, sinon le champ refusé. */
        fun validateDevice(name: String, host: String, mac: String, probe: HubProbe): HubDeviceField? = when {
            name.isBlank() || name.trim().length > DeviceValidator.MAX_NAME_LENGTH -> HubDeviceField.NAME
            !DeviceValidator.isValidHost(host.trim()) -> HubDeviceField.HOST
            mac.isNotBlank() && MacAddress.parseOrNull(mac)?.let { wakeMac(it) } == null -> HubDeviceField.MAC
            probe.kind !in PROBES -> HubDeviceField.PROBE
            probe.kind == PROBE_TCP && !DeviceValidator.isValidPort(probe.port) -> HubDeviceField.PORT
            probe.kind != PROBE_TCP && probe.port != 0 && !DeviceValidator.isValidPort(probe.port) -> HubDeviceField.PORT
            else -> null
        }

        const val PROBE_TCP = "tcp"
        const val PROBE_HTTP = "http"
        const val PROBE_HTTPS = "https"
        private val PROBES = setOf(PROBE_TCP, PROBE_HTTP, PROBE_HTTPS)

        private fun protocolError(detail: String) = AgentResult.Failure(AgentError.PROTOCOL, detail)
    }
}
