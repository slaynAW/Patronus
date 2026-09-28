package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentKey
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable

/**
 * Test de bout en bout contre le VRAI agent Go (lancé en mode simulation par la CI) :
 * garantit que l'application et l'agent parlent exactement le même protocole.
 */
@EnabledIfEnvironmentVariable(named = "WOL_E2E_PORT", matches = "\\d+")
class AgentEndToEndTest {
    private val port = System.getenv("WOL_E2E_PORT").toInt()
    private val settings = AgentSettings(port = port, key = System.getenv("WOL_E2E_KEY"))

    @Test
    fun `statut puis extinction simulee`() = runBlocking {
        val status = AgentClient().status("127.0.0.1", settings)
        assertTrue(status is AgentResult.Success, "$status")
        assertTrue((status as AgentResult.Success).value.hostname.isNotEmpty())

        val power = AgentClient().power("127.0.0.1", settings, PowerAction.SHUTDOWN, delaySeconds = 0)
        assertTrue(power is AgentResult.Success, "$power")

        // Journal de l'agent : son démarrage et la commande reçue y figurent.
        val history = AgentClient().history("127.0.0.1", settings)
        assertTrue(history is AgentResult.Success, "$history")
        val events = (history as AgentResult.Success).value.events
        assertTrue(events.any { it.k == "boot" }, "$events")
        assertTrue(events.any { it.k == "cmd" && it.a == "shutdown" }, "$events")
    }

    @Test
    fun `journal commun demarrage signale et nom de l'appareil`() = runBlocking {
        val client = AgentClient(deviceName = { "Pixel CI" })
        val wake = System.currentTimeMillis() / 1000 - 30
        val reported = client.reportWakes("127.0.0.1", settings, listOf(wake))
        assertTrue(reported is AgentResult.Success, "$reported")
        val events = (reported as AgentResult.Success).value.events
        assertTrue(events.any { it.k == "wake" && it.t == wake && it.b == "Pixel CI" && it.c == "127.0.0.1" }, "$events")

        val power = client.power("127.0.0.1", settings, PowerAction.SLEEP, delaySeconds = 0)
        assertTrue(power is AgentResult.Success, "$power")
        val history = client.history("127.0.0.1", settings) as AgentResult.Success
        assertTrue(history.value.events.any { it.k == "cmd" && it.a == "sleep" && it.b == "Pixel CI" }, "${history.value.events}")
    }

    @Test
    fun `mauvaise cle refusee`() = runBlocking {
        val result = AgentClient().status("127.0.0.1", settings.copy(key = AgentKey.generate()))
        assertEquals(AgentError.UNAUTHORIZED, (result as AgentResult.Failure).error)
    }
}
