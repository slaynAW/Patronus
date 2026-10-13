package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentKey
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.AgentUpgrade
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.core.status.PowerState
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Passage automatique en wolagent/2 : décision et cas sans agent wolagent/2 (le reste : Wolagent2EndToEndTest). */
class AgentUpgradeTest {
    private val key = AgentKey.generate()
    private val pin = Pin.of("agent".toByteArray()).toString()

    private fun device(agent: AgentSettings?, host: String = "127.0.0.1") =
        Device(id = "pc", name = "PC", mac = MacAddress.parse("AA:BB:CC:DD:EE:01"), host = host, agent = agent)

    private val announced = DeviceStatus(
        PowerState.ONLINE,
        agent = AgentStatus(proto = "wolagent/1", protocols = listOf("wolagent/1", "wolagent/2"), pin = pin),
    )

    @Test
    fun `quand tenter le passage`() {
        val v1 = device(AgentSettings(key = key))
        assertTrue(AgentUpgrade.worthTrying(v1, announced, sharedWithControl = false))
        assertTrue(AgentUpgrade.worthTrying(v1, announced, sharedWithControl = true), "un PC partagé passe aussi (clé gardée)")
        assertFalse(AgentUpgrade.worthTrying(v1, DeviceStatus(PowerState.ONLINE, agent = AgentStatus(proto = "wolagent/1")), false), "agent sans wolagent/2")
        assertFalse(AgentUpgrade.worthTrying(v1, announced.copy(agent = announced.agent!!.copy(pin = "")), false), "empreinte non annoncée")
        assertFalse(AgentUpgrade.worthTrying(device(null), announced, false))
        assertFalse(AgentUpgrade.worthTrying(device(AgentSettings(key = key), host = ""), announced, false))

        // Empreinte connue, clé encore là : terminer (effacer la clé) ou enregistrer cet appareil.
        val both = device(AgentSettings(key = key, pin = pin))
        val v2ok = DeviceStatus(PowerState.ONLINE, agent = AgentStatus(proto = "wolagent/2"))
        assertTrue(AgentUpgrade.worthTrying(both, v2ok, sharedWithControl = false))
        assertFalse(AgentUpgrade.worthTrying(both, v2ok, sharedWithControl = true), "clé gardée pour le partage")
        assertTrue(AgentUpgrade.worthTrying(both, DeviceStatus(PowerState.ONLINE, agentError = AgentError.UNREGISTERED), true))
        assertFalse(AgentUpgrade.worthTrying(both, DeviceStatus(PowerState.OFFLINE, agentError = AgentError.UNREACHABLE), false))
        assertFalse(AgentUpgrade.worthTrying(both, DeviceStatus(PowerState.ONLINE, agentError = AgentError.PIN_MISMATCH), false))

        // Plus de clé (passé en wolagent/2, ou retiré par le PC) : jamais de réenregistrement automatique.
        val pinned = device(AgentSettings(key = "", pin = pin))
        assertFalse(AgentUpgrade.worthTrying(pinned, DeviceStatus(PowerState.ONLINE, agentError = AgentError.UNREGISTERED), false))
    }

    @Test
    fun `agent wolagent 1 seul ou cle refusee`() = runBlocking {
        FakeAgentServer(AgentKey.decode(key)).use { server ->
            val client = AgentClient()
            val v1 = device(AgentSettings(server.port, key))
            assertEquals(AgentUpgrade.Outcome.Unchanged, AgentUpgrade.run(client, v1, keepKey = false))
            val wrong = device(AgentSettings(server.port, AgentKey.generate()))
            val failed = AgentUpgrade.run(client, wrong, keepKey = false)
            assertEquals(AgentError.UNAUTHORIZED, (failed as AgentUpgrade.Outcome.Failed).error)
        }
    }

    @Test
    fun `delai entre deux essais`() {
        assertEquals(60_000L, AgentUpgrade.retryDelayMs(1))
        assertEquals(120_000L, AgentUpgrade.retryDelayMs(2))
        assertEquals(3_600_000L, AgentUpgrade.retryDelayMs(7))
        assertEquals(3_600_000L, AgentUpgrade.retryDelayMs(40))
    }
}
