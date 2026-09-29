package io.github.slaynaw.wakeonlan

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.history.HistoryKind
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.DeviceValidator
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
        val agent = device.agent?.takeIf { it.hasKey && device.hasHost }
            ?: return AgentResult.Failure(AgentError.NO_KEY)
        val result = agentClient.power(device.host, agent, action, force = force)
        when (result) {
            is AgentResult.Success -> DiagnosticLog.i("action", "$action${if (force) " (forcé)" else ""} accepté par « ${device.name} » (${device.host}:${agent.port})")
            is AgentResult.Failure -> DiagnosticLog.w("action", "$action refusé ou impossible pour « ${device.name} » (${device.host}:${agent.port}) : ${result.error}${result.detail?.let { " — $it" }.orEmpty()}")
        }
        if (result is AgentResult.Success) {
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

    suspend fun testAgent(host: String, agent: AgentSettings): AgentResult<AgentStatus> =
        agentClient.status(host, agent).also { r ->
            when (r) {
                is AgentResult.Success -> DiagnosticLog.i("action", "test de l'agent $host:${agent.port} : réussi (version ${r.value.version}, ${r.value.os})")
                is AgentResult.Failure -> DiagnosticLog.w("action", "test de l'agent $host:${agent.port} : ${r.error}${r.detail?.let { " — $it" }.orEmpty()}")
            }
        }
}
