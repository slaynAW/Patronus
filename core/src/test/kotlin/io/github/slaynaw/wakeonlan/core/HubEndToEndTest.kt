package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent2.Commands
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Client
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Exception
import io.github.slaynaw.wakeonlan.core.hub.HubClient
import io.github.slaynaw.wakeonlan.core.hub.HubProbe
import io.github.slaynaw.wakeonlan.core.hub.HubProtocol
import io.github.slaynaw.wakeonlan.core.hub.HubRequest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable
import java.io.File
import java.io.IOException

/**
 * patronus-hub/1 de bout en bout contre le VRAI hub Go (lancé par la CI) : appairage avec le lien du premier
 * démarrage, état, invitation, rôles, révocation, réglages, appareils sans agent, et refus des protocoles mêlés.
 */
@EnabledIfEnvironmentVariable(named = "WOL_HUB_LOG", matches = ".+")
class HubEndToEndTest {
    private val port = System.getenv("WOL_HUB_PORT").toInt()
    private val agentPort = System.getenv("WOL_E2E_PORT")?.toIntOrNull()
    private val client = HubClient()

    /** Lien d'appairage écrit par le hub dans son journal au premier démarrage (jamais affiché par la CI). */
    private fun firstLink() = HubProtocol.parseLink(
        File(System.getenv("WOL_HUB_LOG")).readLines().first { "lien : " in it }.substringAfter("lien : ").trim(),
    )

    @Test
    fun `appairage, roles, revocation et reglages`() {
        val link = firstLink()
        assertEquals("127.0.0.1", link.host)
        assertEquals(port, link.port)
        val phone = TestIdentities.newDevice()
        client.connect("127.0.0.1", port, phone, link.agent).use { s ->
            val paired = s.request(HubRequest(cmd = Commands.PAIR, code = link.code, name = "Pixel CI"))
            assertTrue(paired.ok && paired.role == "admin" && paired.device == phone.pin.toString(), "$paired")
            val status = s.request(HubRequest(cmd = HubProtocol.STATUS))
            val hub = status.hub
            assertTrue(status.ok && hub != null && hub.name == "Hub CI" && hub.pin == link.agent.toString(), "$status")
            assertTrue(status.pcs.isEmpty() && status.states.isEmpty(), "$status")
            assertEquals(ResponseCode.ALREADY_PAIRED, s.request(HubRequest(cmd = Commands.PAIR, code = link.code)).code)
        }

        // Invitation au rôle view : la tablette consulte, sans administrer.
        val invite = client.connect("127.0.0.1", port, phone, link.agent).use { s ->
            s.request(HubRequest(cmd = Commands.INVITE, role = "view", name = "Tablette CI")).invite!!
        }
        assertEquals(link.agent.toString(), invite.hub)
        val tablet = TestIdentities.newDevice()
        client.connect("127.0.0.1", port, tablet, link.agent).use { s ->
            val paired = s.request(HubRequest(cmd = Commands.PAIR, code = invite.code))
            assertTrue(paired.ok && paired.role == "view", "$paired")
            assertTrue(s.request(HubRequest(cmd = HubProtocol.STATUS)).ok)
            assertEquals(ResponseCode.INSUFFICIENT_ROLE, s.request(HubRequest(cmd = Commands.DEVICES)).code)
            assertEquals(ResponseCode.INSUFFICIENT_ROLE, s.request(HubRequest(cmd = HubProtocol.WAKE, pc = "x")).code)
        }

        client.connect("127.0.0.1", port, phone, link.agent).use { s ->
            // Le rôle control n'existe pas sur le hub.
            assertEquals(ResponseCode.BAD_REQUEST, s.request(HubRequest(cmd = Commands.DEVICES_ROLE, device = tablet.pin.toString(), role = "control")).code)
            val devices = s.request(HubRequest(cmd = Commands.DEVICES)).devices
            assertEquals(setOf("Pixel CI", "Tablette CI"), devices.map { it.name }.toSet(), "$devices")
            assertTrue(s.request(HubRequest(cmd = Commands.DEVICES_REVOKE, device = tablet.pin.toString())).ok)
            assertEquals(ResponseCode.LAST_ADMIN, s.request(HubRequest(cmd = Commands.DEVICES_REVOKE, device = phone.pin.toString())).code)

            // Réglages : conservation de l'archive.
            assertEquals(365, s.request(HubRequest(cmd = HubProtocol.SETTINGS)).settings?.retention)
            assertEquals(ResponseCode.BAD_REQUEST, s.request(HubRequest(cmd = HubProtocol.SETTINGS, retention = 30)).code)
            assertEquals(730, s.request(HubRequest(cmd = HubProtocol.SETTINGS, retention = 730)).settings?.retention)

            // Appareil sans agent : ajouté, présent dans l'état, retiré.
            val added = s.request(HubRequest(cmd = HubProtocol.PCS_ADD, name = "Box CI", host = "192.168.1.1", probe = HubProbe("tcp", 80)))
            val box = added.pc ?: error("appareil non ajouté : $added")
            assertEquals("tcp", box.probe?.kind)
            val status = s.request(HubRequest(cmd = HubProtocol.STATUS))
            assertEquals(listOf(box.id), status.pcs.map { it.id })
            assertEquals(listOf(box.id), status.states.map { it.id })
            assertTrue(s.request(HubRequest(cmd = HubProtocol.EVENTS)).ok)
            assertTrue(s.request(HubRequest(cmd = HubProtocol.PCS_REMOVE, pc = box.id)).ok)
        }
        // La tablette retirée n'est plus reconnue.
        client.connect("127.0.0.1", port, tablet, link.agent).use { s ->
            assertEquals(ResponseCode.UNKNOWN_DEVICE, s.request(HubRequest(cmd = HubProtocol.STATUS)).code)
        }
    }

    @Test
    fun `protocoles meles refuses`() {
        val link = firstLink()
        val device = TestIdentities.newDevice()
        // Un client wolagent/2 ordinaire n'obtient pas de session avec le hub.
        assertThrows<IOException> { Wolagent2Client().connect("127.0.0.1", port, device, link.agent) }
        // Le client du hub refuse un agent dès son salut.
        if (agentPort != null) {
            val e = assertThrows<Wolagent2Exception> { client.connect("127.0.0.1", agentPort, device, link.agent) }
            assertEquals(Wolagent2Exception.Reason.NOT_AGENT, e.reason)
        }
        // Mauvaise empreinte : rien n'est envoyé.
        val other = TestIdentities.newDevice()
        val e = assertThrows<Wolagent2Exception> { client.connect("127.0.0.1", port, device, other.pin) }
        assertEquals(Wolagent2Exception.Reason.PIN_MISMATCH, e.reason)
    }
}
