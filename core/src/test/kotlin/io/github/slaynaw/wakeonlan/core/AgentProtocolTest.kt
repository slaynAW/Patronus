package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentHistory
import io.github.slaynaw.wakeonlan.core.agent.AgentHistoryEvent
import io.github.slaynaw.wakeonlan.core.agent.AgentKey
import io.github.slaynaw.wakeonlan.core.agent.AgentProtocol
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.net.ServerSocket

class AgentProtocolTest {
    private val key = AgentKey.generate()
    private val keyBytes = AgentKey.decode(key)

    @Test
    fun `signatures conformes aux vecteurs partages`() {
        SharedVectors.value.vectors.forEach { v ->
            val k = AgentKey.decode(v.key)
            assertEquals(v.requestMac, AgentProtocol.requestMac(k, v.nonce, v.cnonce, v.requestBody), v.name)
            assertEquals(v.responseMac, AgentProtocol.responseMac(k, v.nonce, v.cnonce, v.responseBody), v.name)
        }
    }

    @Test
    fun `comparaison de signatures`() {
        val v = SharedVectors.value.vectors.first()
        assertTrue(AgentProtocol.macEquals(v.requestMac, v.requestMac))
        assertFalse(AgentProtocol.macEquals(v.requestMac, v.responseMac))
        assertFalse(AgentProtocol.macEquals(v.requestMac, "pas du base64 !"))
    }

    @Test
    fun `cle generee valide`() {
        assertEquals(43, key.length)
        assertEquals(32, keyBytes.size)
        assertEquals(null, AgentKey.decodeOrNull("trop-court"))
        assertEquals(key, AgentKey.encode(AgentKey.decode(" $key= ")))
    }

    @Test
    fun `journal de l'agent, meme volumineux`() = runBlocking {
        FakeAgentServer(keyBytes).use { server ->
            val settings = AgentSettings(port = server.port, key = key)
            // Agent trop ancien : la commande est refusée.
            val old = AgentClient().history("127.0.0.1", settings)
            assertEquals(AgentError.REJECTED, (old as AgentResult.Failure).error)
            // Journal plus grand qu'un message ordinaire.
            val events = List(1500) { AgentHistoryEvent(1_790_000_000L + it * 60, "cmd", "sleep", "192.168.100.100") }
            server.history = AgentHistory(from = 1_789_000_000, events = events)
            val result = AgentClient().history("127.0.0.1", settings)
            assertTrue(result is AgentResult.Success, "$result")
            assertEquals(server.history, (result as AgentResult.Success).value)
        }
    }

    @Test
    fun `statut et commande avec un agent valide`() = runBlocking {
        FakeAgentServer(keyBytes).use { server ->
            val settings = AgentSettings(port = server.port, key = key)
            val status = AgentClient().status("127.0.0.1", settings)
            assertTrue(status is AgentResult.Success, "$status")
            assertEquals("PC-TEST", (status as AgentResult.Success).value.hostname)
            assertEquals(3600, status.value.uptimeSeconds)

            val power = AgentClient().power("127.0.0.1", settings, PowerAction.SHUTDOWN)
            assertTrue(power is AgentResult.Success, "$power")
            assertEquals(listOf("status", "shutdown"), server.commands)
        }
    }

    @Test
    fun `mauvaise cle refusee par l agent`() = runBlocking {
        FakeAgentServer(keyBytes).use { server ->
            val result = AgentClient().status("127.0.0.1", AgentSettings(server.port, AgentKey.generate()))
            assertEquals(AgentError.UNAUTHORIZED, (result as AgentResult.Failure).error)
            assertTrue(result.hostAnswered)
            assertTrue(server.commands.isEmpty())
        }
    }

    @Test
    fun `reponse falsifiee detectee`() = runBlocking {
        FakeAgentServer(keyBytes, FakeAgentServer.Behavior.BAD_RESPONSE_MAC).use { server ->
            val result = AgentClient().power("127.0.0.1", AgentSettings(server.port, key), PowerAction.REBOOT)
            assertEquals(AgentError.PROTOCOL, (result as AgentResult.Failure).error)
        }
    }

    @Test
    fun `commande rejetee et protocole inconnu`() = runBlocking {
        FakeAgentServer(keyBytes, FakeAgentServer.Behavior.REJECT).use { server ->
            val result = AgentClient().power("127.0.0.1", AgentSettings(server.port, key), PowerAction.SLEEP)
            assertEquals(AgentError.REJECTED, (result as AgentResult.Failure).error)
        }
        FakeAgentServer(keyBytes, FakeAgentServer.Behavior.RATE_LIMITED).use { server ->
            val result = AgentClient().status("127.0.0.1", AgentSettings(server.port, key))
            assertEquals(AgentError.RATE_LIMITED, (result as AgentResult.Failure).error)
            assertTrue(result.hostAnswered)
        }
        FakeAgentServer(keyBytes, FakeAgentServer.Behavior.WRONG_PROTO).use { server ->
            val result = AgentClient().status("127.0.0.1", AgentSettings(server.port, key))
            assertEquals(AgentError.PROTOCOL, (result as AgentResult.Failure).error)
        }
    }

    @Test
    fun `port ferme et agent muet`() = runBlocking {
        val closedPort = ServerSocket(0).use { it.localPort }
        val refused = AgentClient().status("127.0.0.1", AgentSettings(closedPort, key))
        assertEquals(AgentError.REFUSED, (refused as AgentResult.Failure).error)
        assertTrue(refused.hostAnswered)

        FakeAgentServer(keyBytes, FakeAgentServer.Behavior.HANG).use { server ->
            val result = AgentClient(readTimeoutMs = 300).status("127.0.0.1", AgentSettings(server.port, key))
            assertEquals(AgentError.PROTOCOL, (result as AgentResult.Failure).error)
            assertTrue(result.hostAnswered)
        }

        val noKey = AgentClient().status("127.0.0.1", AgentSettings(closedPort, ""))
        assertEquals(AgentError.NO_KEY, (noKey as AgentResult.Failure).error)
    }
}
