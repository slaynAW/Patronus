package io.github.slaynaw.wakeonlan.core.hub

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent2.Commands
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.agent2.Role
import io.github.slaynaw.wakeonlan.core.model.Device
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.shareIn
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** Stockage du hub appairé, chiffré par l'application (Keystore sur Android) ; jamais exporté ni sauvegardé. */
interface HubStore {
    val hub: StateFlow<PairedHub?>

    /** Le hub appairé, une fois le stockage lu. */
    suspend fun current(): PairedHub?

    /** Applique une modification (null : oublie le hub) puis l'enregistre. */
    suspend fun update(transform: (PairedHub?) -> PairedHub?)
}

/** État du hub pour l'interface. */
data class HubState(
    val loaded: Boolean = false,
    val hub: PairedHub? = null,
    /** Dernier relevé réussi du hub appairé (null : pas encore). */
    val snapshot: HubSnapshot? = null,
    /** Heure de ce relevé (ms). */
    val checkedAt: Long = 0,
    /** Dernier relevé en échec (hub injoignable, appareil retiré…) ; null après un relevé réussi. */
    val failure: AgentResult.Failure? = null,
) {
    /** Évènements non vus (« Pendant votre absence »). */
    val unseen: List<HubEvent> get() = snapshot?.events.orEmpty()

    /** PC du hub qui surveille le PC [device] (par l'empreinte de son agent), avec son état. */
    fun pcOf(device: Device): Pair<HubPc, HubPcState?>? {
        val pin = device.agent?.pin?.takeIf { it.isNotEmpty() } ?: return null
        val pc = snapshot?.pcOfAgent(pin) ?: return null
        return pc to snapshot.stateOf(pc.id)
    }
}

/** Résultat de « Appairer un hub ». */
sealed interface PairOutcome {
    data class Paired(val hub: PairedHub, val already: Boolean) : PairOutcome

    /** Lien refusé avant tout échange : lien de PC, expiré ou invalide. */
    data class BadLink(val reason: HubLinkResult) : PairOutcome

    /** Un autre hub est déjà appairé : l'oublier d'abord. */
    data object OtherHub : PairOutcome

    data class Failed(val failure: AgentResult.Failure) : PairOutcome
}

/**
 * Le hub dans l'application (docs/HUB.md, §12) : hub appairé, relevé périodique tant que l'interface le regarde
 * ([watch]), évènements non vus, administration et PC confiés, au-dessus de [HubService]. Sans hub, rien ne change :
 * l'application parle directement aux agents.
 */
class HubManager(
    scope: CoroutineScope,
    private val store: HubStore,
    private val service: HubService,
    private val agents: AgentClient,
    /** Empreinte complète de cet appareil (null : identité indisponible). */
    private val selfPin: () -> String?,
    private val now: () -> Long = System::currentTimeMillis,
    private val log: (String) -> Unit = {},
) {
    private data class Read(val pin: String = "", val snapshot: HubSnapshot? = null, val at: Long = 0, val failure: AgentResult.Failure? = null)

    private val reads = MutableStateFlow(Read())
    private val loaded = MutableStateFlow(false)
    private val refreshing = Mutex()

    val state: StateFlow<HubState> = combine(store.hub, reads, loaded) { hub, read, isLoaded ->
        val mine = hub != null && read.pin == hub.pin
        HubState(
            loaded = isLoaded,
            hub = hub,
            snapshot = read.snapshot.takeIf { mine },
            checkedAt = if (mine) read.at else 0,
            failure = read.failure.takeIf { mine },
        )
    }.stateIn(scope, SharingStarted.Eagerly, HubState())

    /** Relève le hub toutes les [REFRESH_MS] tant que ce flux est collecté (interface affichée). */
    val watch: SharedFlow<Unit> = flow {
        while (true) {
            refresh()
            emit(Unit)
            delay(REFRESH_MS)
        }
    }.shareIn(scope, SharingStarted.WhileSubscribed(5_000))

    /** Relit le hub appairé : son état, ses PC et les évènements non vus. */
    suspend fun refresh() {
        val hub = store.current()
        loaded.value = true
        if (hub == null) return
        refreshing.withLock {
            when (val r = service.read(hub)) {
                is AgentResult.Success -> {
                    val snapshot = r.value
                    if (snapshot.dropped > 0) log("hub « ${hub.name} » : ${snapshot.dropped} évènement(s) inattendu(s) écarté(s)")
                    if (reads.value.failure != null && reads.value.pin == hub.pin) log("hub « ${hub.name} » de nouveau joint")
                    reads.value = Read(hub.pin, snapshot, now(), null)
                    store.update { current ->
                        current?.takeIf { it.pin == hub.pin }?.copy(
                            role = snapshot.role.ifEmpty { current.role },
                            name = snapshot.info.name.trim().ifEmpty { current.name },
                        ) ?: current
                    }
                }
                is AgentResult.Failure -> {
                    val previous = reads.value.takeIf { it.pin == hub.pin }
                    val before = previous?.failure
                    if (before == null || before.error != r.error || before.code != r.code) {
                        log("relevé du hub « ${hub.name} » : ${r.error} ${r.code.orEmpty()}")
                    }
                    reads.value = Read(hub.pin, previous?.snapshot, previous?.at ?: 0, r)
                }
            }
        }
    }

    /** Appaire ce téléphone au hub d'un lien collé ou scanné (§5.2). */
    suspend fun pair(text: String): PairOutcome {
        val link = when (val r = HubLinkResult.read(text.trim(), now() / 1000)) {
            is HubLinkResult.Valid -> r.link
            else -> return PairOutcome.BadLink(r)
        }
        val current = store.current()
        if (current != null && current.pin != link.agent.toString()) return PairOutcome.OtherHub
        return when (val r = service.pair(link, now() / 1000)) {
            is AgentResult.Success -> {
                val paired = r.value.hub.copy(eventsSeen = current?.eventsSeen ?: 0)
                store.update { paired }
                log("hub « ${paired.name} » appairé (${paired.shortPin}, rôle ${paired.role}${if (r.value.already) ", déjà connu" else ""})")
                refresh()
                PairOutcome.Paired(paired, r.value.already)
            }
            is AgentResult.Failure -> {
                log("appairage au hub ${link.agent.short()} : ${r.error} ${r.code.orEmpty()}")
                PairOutcome.Failed(r)
            }
        }
    }

    /**
     * Oublie le hub sur ce téléphone, après s'en être retiré quand c'est possible (administrateur, mais pas le dernier,
     * hub joignable) : true si le hub l'a retiré, false s'il garde ce téléphone dans ses appareils autorisés.
     */
    suspend fun forget(): Boolean {
        val hub = store.current() ?: return false
        var revoked = false
        val pin = selfPin()
        if (pin != null && hub.isAdmin) {
            revoked = when (val r = service.request(hub, HubRequest(cmd = Commands.DEVICES_REVOKE, device = pin))) {
                is AgentResult.Success -> true
                is AgentResult.Failure -> r.error == AgentError.UNREGISTERED
            }
        }
        store.update { null }
        reads.value = Read()
        log("hub « ${hub.name} » oublié${if (revoked) " (retiré du hub)" else ""}")
        return revoked
    }

    /** Appareils autorisés, invitations et conservation (administrateur du hub). */
    suspend fun admin(): AgentResult<HubAdmin> = withHub { service.admin(it) }

    /**
     * Modification d'un appareil autorisé ou d'une invitation, ou des réglages. Un changement qui concerne ce téléphone
     * (son rôle, son retrait) est reporté sur le hub appairé.
     */
    suspend fun change(request: HubRequest): AgentResult<HubResponse> = withHub { hub ->
        val r = service.request(hub, request)
        if (r is AgentResult.Success && request.device != null && request.device == selfPin()) {
            when (request.cmd) {
                Commands.DEVICES_ROLE -> store.update { it?.copy(role = request.role ?: it.role) }
                Commands.DEVICES_REVOKE -> {
                    store.update { null }
                    reads.value = Read()
                }
            }
        }
        r
    }

    suspend fun invite(role: Role, ttlSeconds: Long, name: String): AgentResult<HubCreatedInvite> =
        withHub { service.invite(it, role, ttlSeconds, name) }

    /** Durée de conservation de l'archive du hub, en jours. */
    suspend fun retention(days: Int): AgentResult<Int> {
        if (days !in HubService.MIN_RETENTION..HubService.MAX_RETENTION) {
            return AgentResult.Failure(AgentError.REJECTED, code = ResponseCode.BAD_REQUEST)
        }
        return when (val r = change(HubRequest(cmd = HubProtocol.SETTINGS, retention = days))) {
            is AgentResult.Success -> r.value.settings?.let { AgentResult.Success(it.retention) }
                ?: AgentResult.Failure(AgentError.PROTOCOL, "réglages absents de la réponse")
            is AgentResult.Failure -> r
        }
    }

    /** Confie le PC [device] au hub (§5.3). */
    suspend fun entrust(device: Device): EntrustResult {
        val hub = store.current() ?: return EntrustResult.HubFailed(noHub())
        val r = service.entrust(hub, agents, device, snapshotOf(hub))
        when (r) {
            is EntrustResult.Success -> log("« ${device.name} » confié au hub « ${hub.name} » (rôle du hub ${r.pc.role})")
            is EntrustResult.AgentFailed -> log("invitation du hub par « ${device.name} » : ${r.failure.error} ${r.failure.code.orEmpty()}")
            is EntrustResult.HubFailed -> log("« ${device.name} » confié au hub : ${r.failure.error} ${r.failure.code.orEmpty()}")
            EntrustResult.AlreadyEntrusted -> {}
        }
        if (r is EntrustResult.Success) refresh()
        return r
    }

    /** Retire un PC ou un appareil du hub ; [devices] : les PC de ce téléphone, pour retirer le hub de l'agent. */
    suspend fun remove(pcId: String, devices: List<Device>): AgentResult<HubRemoval> = withHub { hub ->
        val agentPin = snapshotOf(hub)?.pcs?.firstOrNull { it.id == pcId }?.agent.orEmpty()
        val local = devices.firstOrNull { agentPin.isNotEmpty() && it.agent?.pin == agentPin }
        val r = service.remove(hub, agents, pcId, local)
        if (r is AgentResult.Success) {
            log("« ${r.value.pc.name} » retiré du hub « ${hub.name} »")
            refresh()
        }
        r
    }

    /** Ajoute un appareil sans agent (§5.4), vérifié d'abord ([HubService.validateDevice]). */
    suspend fun addDevice(name: String, host: String, mac: String, probe: HubProbe): AgentResult<HubPc> = withHub { hub ->
        val r = service.addDevice(hub, name, host, mac, probe)
        if (r is AgentResult.Success) {
            log("appareil « ${r.value.name} » ajouté au hub « ${hub.name} » (sonde ${probe.kind})")
            refresh()
        }
        r
    }

    /** Marque comme vus les évènements jusqu'à [until] (horloge du hub, secondes Unix). */
    suspend fun markSeen(until: Long) {
        val hub = store.current() ?: return
        if (until <= hub.eventsSeen) return
        store.update { it?.takeIf { h -> h.pin == hub.pin }?.copy(eventsSeen = until) ?: it }
        reads.update { r -> if (r.pin == hub.pin && r.snapshot != null) r.copy(snapshot = r.snapshot.copy(events = r.snapshot.events.filter { it.t > until })) else r }
        if (snapshotOf(hub)?.moreEvents == true) refresh()
    }

    /**
     * Demande au hub de démarrer le PC [device] (paquet magique envoyé depuis le réseau du NAS, §6.6), s'il lui est
     * confié avec sa MAC et que ce téléphone a le rôle « wake » sur le hub. true si le hub l'a envoyé.
     */
    suspend fun relayWake(device: Device): Boolean {
        val hub = store.current() ?: return false
        if (!hub.canWake) return false
        val pin = device.agent?.pin?.takeIf { it.isNotEmpty() } ?: return false
        val pc = snapshotOf(hub)?.pcOfAgent(pin) ?: return false
        if (pc.mac.isEmpty()) return false
        return when (val r = service.request(hub, HubRequest(cmd = HubProtocol.WAKE, pc = pc.id))) {
            is AgentResult.Success -> {
                log("réveil de « ${device.name} » envoyé par le hub « ${hub.name} »")
                true
            }
            is AgentResult.Failure -> {
                log("réveil de « ${device.name} » par le hub : ${r.error} ${r.code.orEmpty()}")
                false
            }
        }
    }

    /** Dernier relevé du hub [hub] (à jour, sans attendre [state]). */
    private fun snapshotOf(hub: PairedHub): HubSnapshot? = reads.value.takeIf { it.pin == hub.pin }?.snapshot

    private suspend fun <T> withHub(run: suspend (PairedHub) -> AgentResult<T>): AgentResult<T> {
        val hub = store.current() ?: return noHub()
        return run(hub)
    }

    private fun noHub() = AgentResult.Failure(AgentError.NO_KEY, "aucun hub appairé", NO_HUB)

    companion object {
        /** Intervalle entre deux relevés du hub, interface affichée. */
        const val REFRESH_MS = 30_000L

        /** Code d'échec : aucun hub appairé. */
        const val NO_HUB = "no_hub"
    }
}
