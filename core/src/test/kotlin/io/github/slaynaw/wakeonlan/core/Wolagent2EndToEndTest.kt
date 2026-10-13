package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentKey
import io.github.slaynaw.wakeonlan.core.agent2.Commands
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.agent2.Request2
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Client
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Exception
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable
import java.io.File
import java.io.IOException

/**
 * wolagent/2 de bout en bout contre le VRAI agent Go (lancé en simulation par la CI, comme pour AgentEndToEndTest) :
 * migration depuis wolagent/1, commandes, administration par wol-agent sur le PC, appairage par invitation,
 * rôles et révocation.
 */
@EnabledIfEnvironmentVariable(named = "WOL_E2E_IDENTITY", matches = ".+")
class Wolagent2EndToEndTest {
    private val port = System.getenv("WOL_E2E_PORT").toInt()
    private val key = AgentKey.decode(System.getenv("WOL_E2E_KEY"))
    private val local = TestIdentities.agent(File(System.getenv("WOL_E2E_IDENTITY")))
    private val client = Wolagent2Client()

    @Test
    fun `migration, appairage, roles et revocation`() {
        // Migration : un appareil appairé en wolagent/1 passe en wolagent/2 avec la clé du PC.
        val phone = TestIdentities.newDevice()
        val agentPin: Pin
        client.connect("127.0.0.1", port, phone, agent = null).use { session ->
            assertThrows<Wolagent2Exception> { session.request(Request2(cmd = "status")) }
            val migrated = session.migrate(key, "Pixel CI v2")
            assertTrue(migrated.ok, "$migrated")
            assertEquals("control", migrated.role)
            assertEquals(phone.pin.toString(), migrated.device)
            agentPin = session.agentPin
            assertEquals(local.pin, agentPin, "empreinte de l'agent prouvée par la migration")
            val status = session.request(Request2(cmd = "status"))
            assertTrue(status.ok && status.hostname.isNotEmpty() && "wolagent/2" in status.protocols, "$status")
            assertTrue(session.request(Request2(cmd = "sleep", delay = 0)).ok)
            val history = session.request(Request2(cmd = "history")).history!!.events
            assertTrue(history.any { it.k == "cmd" && it.a == "sleep" && it.b == "Pixel CI v2" && it.i == phone.pin.short() }, "$history")
        }

        // wol-agent sur le PC (identité de l'agent, adresse de bouclage) invite un deuxième appareil.
        val invite = client.connect("127.0.0.1", port, local, agentPin).use { admin ->
            admin.request(Request2(cmd = Commands.INVITE, role = "view", name = "Tablette CI")).invite!!
        }
        val tablet = TestIdentities.newDevice()
        client.connect("127.0.0.1", port, tablet, agentPin).use { session ->
            val paired = session.request(Request2(cmd = Commands.PAIR, code = invite.code, name = "ignoré"))
            assertTrue(paired.ok && paired.role == "view", "$paired")
            assertEquals(ResponseCode.INSUFFICIENT_ROLE, session.request(Request2(cmd = "shutdown")).code)
            assertTrue(session.request(Request2(cmd = "status")).ok)
        }

        // Révocation : la tablette n'est plus reconnue.
        client.connect("127.0.0.1", port, local, agentPin).use { admin ->
            val list = admin.request(Request2(cmd = Commands.DEVICES)).devices!!
            assertTrue(list.any { it.name == "Tablette CI" && it.role == "view" && it.via == "pair" }, "$list")
            assertTrue(list.any { it.name == "Pixel CI v2" && it.role == "control" && it.via == "migrate" }, "$list")
            assertTrue(admin.request(Request2(cmd = Commands.DEVICES_REVOKE, device = tablet.pin.toString())).ok)
        }
        client.connect("127.0.0.1", port, tablet, agentPin).use { session ->
            assertEquals(ResponseCode.UNKNOWN_DEVICE, session.request(Request2(cmd = "status")).code)
        }
    }

    @Test
    fun `empreinte differente refusee avant tout envoi`() {
        val wrong = Pin.of("autre agent".toByteArray())
        val e = assertThrows<Wolagent2Exception> { client.connect("127.0.0.1", port, TestIdentities.newDevice(), wrong) }
        assertEquals(Wolagent2Exception.Reason.PIN_MISMATCH, e.reason)
    }

    @Test
    fun `migration avec une mauvaise cle`() {
        client.connect("127.0.0.1", port, TestIdentities.newDevice(), agent = null).use { session ->
            val e = assertThrows<Wolagent2Exception> { session.migrate(AgentKey.decode(AgentKey.generate()), "x") }
            assertEquals(Wolagent2Exception.Reason.MIGRATION_PROOF, e.reason)
            assertThrows<IOException> { session.request(Request2(cmd = Commands.MIGRATE)) }
        }
    }
}
