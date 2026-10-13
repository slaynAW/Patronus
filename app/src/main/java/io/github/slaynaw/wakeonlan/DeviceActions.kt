package io.github.slaynaw.wakeonlan

import io.github.slaynaw.wakeonlan.core.agent.AgentAdmin
import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentPairing
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.agent2.InviteInfo
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.history.HistoryKind
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.DeviceValidator
import io.github.slaynaw.wakeonlan.core.status.HostProber
import io.github.slaynaw.wakeonlan.core.status.StatusMonitor
import io.github.slaynaw.wakeonlan.core.wol.BroadcastAddresses
import io.github.slaynaw.wakeonlan.core.wol.MagicPacket
import io.github.slaynaw.wakeonlan.core.wol.WakeOnLanSender
import io.github.slaynaw.wakeonlan.core.wol.WakeResult
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import io.github.slaynaw.wakeonlan.network.LanNetworkMonitor
import java.net.Inet4Address
import java.net.InetAddress

/**
 * Actions utilisateur sur un terminal : réveil, extinction / redémarrage / veille, test de l'agent.
 * Chaque demande envoyée est notée dans l'historique (avant le changement d'état qu'elle provoque).
 */
class DeviceActions(
    private val network: LanNetworkMonitor,
    private val sender: WakeOnLanSender,
    private val agentClient: AgentClient,
    private val monitor: StatusMonitor,
    private val history: HistoryTracker,
    /** Sonde des PC : un agent qui accepte une action est recontacté sans attendre la fin d'un délai après refus. */
    private val prober: HostProber? = null,
) {
    suspend fun wake(device: Device): WakeResult {
        val forced = device.broadcastAddress?.let { InetAddress.getByName(it) as? Inet4Address }
        val targets = BroadcastAddresses.targets(forced, network.state.value.addresses)
        val password = device.secureOnPassword?.let(DeviceValidator::secureOnBytes)
        val result = sender.send(MagicPacket.build(device.mac, password), targets, listOf(device.wolPort))
        val summary = "« ${device.name} » (${device.mac}) : ${result.packetsSent} paquet(s) vers ${result.destinations.joinToString { "${it.hostString}:${it.port}" }}"
        if (result.success) {
            DiagnosticLog.i("action", "réveil envoyé à $summary" + if (result.errors.isEmpty()) "" else " ; erreurs : ${result.errors}")
        } else {
            DiagnosticLog.w("action", "réveil impossible pour $summary ; erreurs : ${result.errors}")
        }
        if (result.success) {
            history.record(device.id, HistoryKind.WAKE_SENT)
            monitor.onWakeSent(device.id)
        }
        return result
    }

    suspend fun power(device: Device, action: PowerAction, force: Boolean = false): AgentResult<String> {
        val agent = device.agent?.takeIf { it.ready && device.hasHost }
            ?: return AgentResult.Failure(AgentError.NO_KEY)
        val result = agentClient.power(device.host, agent, action, force = force)
        when (result) {
            is AgentResult.Success -> DiagnosticLog.i("action", "$action${if (force) " (forcé)" else ""} accepté par « ${device.name} » (${device.host}:${agent.port})")
            is AgentResult.Failure -> DiagnosticLog.w("action", "$action refusé ou impossible pour « ${device.name} » (${device.host}:${agent.port}) : ${result.error}${result.detail?.let { " — $it" }.orEmpty()}")
        }
        if (result is AgentResult.Success) {
            prober?.forget(device.id)
            history.record(
                device.id,
                when (action) {
                    PowerAction.SHUTDOWN -> HistoryKind.SHUTDOWN_SENT
                    PowerAction.REBOOT -> HistoryKind.REBOOT_SENT
                    PowerAction.SLEEP -> HistoryKind.SLEEP_SENT
                },
            )
            monitor.onPowerActionSent(device.id, action)
        }
        return result
    }

    /** Teste l'agent avec les réglages de la fiche ; [deviceId] : PC modifié (null pour un nouveau PC). */
    /**
     * Appaire ce téléphone avec l'agent d'un PC grâce au code d'un lien `v=2` : le code n'est envoyé qu'à l'agent qui
     * présente l'empreinte [AgentSettings.pin] du lien. Ni le code ni l'empreinte complète ne sont journalisés.
     */
    suspend fun pair(host: String, agent: AgentSettings, code: String, deviceId: String): AgentResult<AgentPairing> =
        agentClient.pair(host, agent, code).also { r ->
            when (r) {
                is AgentResult.Success -> {
                    prober?.forget(deviceId)
                    DiagnosticLog.i(
                        "action",
                        "appairage avec $host:${agent.port} : réussi (empreinte ${agent.shortPin}, rôle ${r.value.role}" +
                            if (r.value.already) ", déjà enregistré)" else ")",
                    )
                }
                is AgentResult.Failure ->
                    DiagnosticLog.w("action", "appairage avec $host:${agent.port} : ${r.error}${r.detail?.let { " — $it" }.orEmpty()}")
            }
        }

    // --- Administration des appareils autorisés par un PC (wolagent/2, rôle « admin », §6.2) ---
    // Journalisée sans code d'appairage ni empreinte complète.

    /** Appareils autorisés et invitations en attente du PC [device]. */
    suspend fun agentAdmin(device: Device): AgentResult<AgentAdmin> {
        val agent = device.agent ?: return AgentResult.Failure(AgentError.NO_KEY)
        return agentClient.admin(device.host, agent).also { logAdmin(device, "appareils autorisés lus", it) }
    }

    /** Invitation à usage unique pour un autre appareil ; la réponse est la seule à contenir le code. */
    suspend fun agentInvite(device: Device, role: String, ttlSeconds: Long, name: String): AgentResult<InviteInfo> {
        val agent = device.agent ?: return AgentResult.Failure(AgentError.NO_KEY)
        return agentClient.invite(device.host, agent, role, ttlSeconds, name)
            .also { logAdmin(device, "invitation créée (rôle $role, ${ttlSeconds / 60} min)", it) }
    }

    suspend fun agentAdminChange(device: Device, change: AdminChange): AgentResult<Unit> {
        val agent = device.agent ?: return AgentResult.Failure(AgentError.NO_KEY)
        val host = device.host
        val (result, what) = when (change) {
            is AdminChange.Revoke -> agentClient.revokeDevice(host, agent, change.device) to "appareil ${short(change.device)} retiré"
            is AdminChange.SetRole -> agentClient.setDeviceRole(host, agent, change.device, change.role) to
                "appareil ${short(change.device)} : rôle ${change.role}"
            is AdminChange.Rename -> agentClient.renameDevice(host, agent, change.device, change.name) to "appareil ${short(change.device)} renommé"
            is AdminChange.CancelInvite -> agentClient.cancelInvite(host, agent, change.id) to "invitation annulée"
        }
        logAdmin(device, what, result)
        return result
    }

    private fun short(pin: String) = Pin.parse(pin)?.short() ?: "?"

    private fun logAdmin(device: Device, what: String, result: AgentResult<*>) = when (result) {
        is AgentResult.Success -> DiagnosticLog.i("agent", "« ${device.name} » : $what")
        is AgentResult.Failure ->
            DiagnosticLog.w("agent", "« ${device.name} » : $what — échec ${result.error} ${result.code.orEmpty()}${result.detail?.let { " — $it" }.orEmpty()}")
    }

    suspend fun testAgent(host: String, agent: AgentSettings, deviceId: String? = null): AgentResult<AgentStatus> =
        agentClient.status(host, agent).also { r ->
            when (r) {
                is AgentResult.Success -> {
                    deviceId?.let { prober?.forget(it) }
                    DiagnosticLog.i("action", "test de l'agent $host:${agent.port} : réussi (version ${r.value.version}, ${r.value.os}, ${r.value.proto})")
                }
                is AgentResult.Failure -> DiagnosticLog.w("action", "test de l'agent $host:${agent.port} : ${r.error}${r.detail?.let { " — $it" }.orEmpty()}")
            }
        }
}

/** Modification demandée par un administrateur (docs/PROTOCOLE-V2.md, §6.2). [device] : empreinte complète de l'appareil. */
sealed interface AdminChange {
    data class Revoke(val device: String) : AdminChange

    data class SetRole(val device: String, val role: String) : AdminChange

    data class Rename(val device: String, val name: String) : AdminChange

    data class CancelInvite(val id: String) : AdminChange
}
