package io.github.slaynaw.wakeonlan

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.DeviceValidator
import io.github.slaynaw.wakeonlan.core.status.StatusMonitor
import io.github.slaynaw.wakeonlan.core.wol.BroadcastAddresses
import io.github.slaynaw.wakeonlan.core.wol.MagicPacket
import io.github.slaynaw.wakeonlan.core.wol.WakeOnLanSender
import io.github.slaynaw.wakeonlan.core.wol.WakeResult
import io.github.slaynaw.wakeonlan.network.LanNetworkMonitor
import java.net.Inet4Address
import java.net.InetAddress

/** Actions utilisateur sur un terminal : réveil, extinction / redémarrage / veille, test de l'agent. */
class DeviceActions(
    private val network: LanNetworkMonitor,
    private val sender: WakeOnLanSender,
    private val agentClient: AgentClient,
    private val monitor: StatusMonitor,
) {
    suspend fun wake(device: Device): WakeResult {
        val forced = device.broadcastAddress?.let { InetAddress.getByName(it) as? Inet4Address }
        val targets = BroadcastAddresses.targets(forced, network.state.value.addresses)
        val password = device.secureOnPassword?.let(DeviceValidator::secureOnBytes)
        val result = sender.send(MagicPacket.build(device.mac, password), targets, listOf(device.wolPort))
        if (result.success) monitor.onWakeSent(device.id)
        return result
    }

    suspend fun power(device: Device, action: PowerAction, force: Boolean = false): AgentResult<String> {
        val agent = device.agent?.takeIf { it.hasKey && device.hasHost }
            ?: return AgentResult.Failure(AgentError.NO_KEY)
        val result = agentClient.power(device.host, agent, action, force = force)
        if (result is AgentResult.Success) monitor.onPowerActionSent(device.id, action)
        return result
    }

    suspend fun testAgent(host: String, agent: AgentSettings): AgentResult<AgentStatus> =
        agentClient.status(host, agent)
}
