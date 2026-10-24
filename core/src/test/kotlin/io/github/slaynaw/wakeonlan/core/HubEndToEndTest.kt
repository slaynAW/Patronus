package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent2.Commands
import io.github.slaynaw.wakeonlan.core.agent2.Role
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Client
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Exception
import io.github.slaynaw.wakeonlan.core.hub.HubClient
import io.github.slaynaw.wakeonlan.core.hub.HubLinkResult
import io.github.slaynaw.wakeonlan.core.hub.HubProbe
import io.github.slaynaw.wakeonlan.core.hub.HubProtocol
import io.github.slaynaw.wakeonlan.core.hub.HubRequest
import io.github.slaynaw.wakeonlan.core.hub.HubService
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
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

    /** Le service du hub des applications, avec le lien d'un second administrateur (patronus-hub pair, CI). */
    @Test
    fun `service du hub`() = runBlocking {
        val text = File(System.getenv("WOL_HUB_PAIR")).readLines().first { "lien : " in it }.substringAfter("lien : ").trim()
        val now = System.currentTimeMillis() / 1000
        val link = (HubLinkResult.read(text, now) as HubLinkResult.Valid).link
        val phone = TestIdentities.newDevice()
        val service = HubService(deviceName = { "Pixel 9 CI" }, identity = { phone })
        val pairing = service.pair(link, now).value()
        val hub = pairing.hub
        assertTrue(!pairing.already && hub.name == "Hub CI" && hub.isAdmin && hub.pin == link.agent.toString(), "$pairing")
        // Le lien a servi : un nouvel appairage de la même identité retrouve son rôle sans le code.
        assertTrue(service.pair(link, now).value().already)

        val admin = service.admin(hub).value()
        assertTrue(admin.devices.any { it.name == "Pixel 9 CI" && it.self } && admin.retention in 90..1095, "$admin")

        // Invitation au rôle view, lien du hub ; pas de rôle control sur le hub.
        val invite = service.invite(hub, Role.VIEW, 600, "Tablette").value()
        val invited = (HubLinkResult.read(invite.link, now) as HubLinkResult.Valid).link
        assertTrue(invited.role == Role.VIEW && invited.agent == link.agent && invite.code.length == 9, invite.link)
        assertEquals(AgentError.REJECTED, (service.invite(hub, Role.CONTROL, 600, "") as AgentResult.Failure).error)
        val tabletId = TestIdentities.newDevice()
        val tablet = HubService(deviceName = { "Tablette CI" }, identity = { tabletId })
        val viewer = tablet.pair(invited, now).value().hub
        assertEquals("view", viewer.role)
        assertTrue(tablet.read(viewer) is AgentResult.Success)
        assertEquals(ResponseCode.INSUFFICIENT_ROLE, (tablet.admin(viewer) as AgentResult.Failure).code)

        // Appareil sans agent avec MAC : réveil par le hub, vu dans les évènements avec l'empreinte de ce téléphone.
        val box = service.addDevice(hub, "Box service", "192.168.1.2", "aa:bb:cc:dd:ee:02", HubProbe("http")).value()
        assertTrue(service.request(hub, HubRequest(cmd = HubProtocol.WAKE, pc = box.id)) is AgentResult.Success)
        val snapshot = service.read(hub).value()
        assertTrue(snapshot.pcs.any { it.id == box.id } && snapshot.stateOf(box.id) != null, "$snapshot")
        val wake = snapshot.events.firstOrNull { it.k == "wake" && it.pc == box.id }
        assertTrue(wake != null && wake.i == phone.pin.short() && snapshot.dropped == 0, "${snapshot.events}")
        // Vus jusqu'à ce réveil : il ne revient plus.
        val seen = hub.copy(eventsSeen = wake!!.t)
        assertTrue(service.read(seen).value().events.none { it.k == "wake" && it.pc == box.id && it.t <= wake.t })

        assertEquals(emptyList<String>(), service.metricsDays(hub, box.id).value())
        assertEquals(emptyMap<String, Any>(), service.metrics(hub, box.id, listOf("2026-01-01")).value())
        assertEquals(emptyList<Any>(), service.journal(hub, box.id, 0, now + 60).value())
        val removal = service.remove(hub, AgentClient(), box.id, null).value()
        assertTrue(removal.pc.id == box.id)
        assertNull(removal.agentRevoked)
        assertEquals(ResponseCode.BAD_REQUEST, (service.remove(hub, AgentClient(), box.id, null) as AgentResult.Failure).code)

        // Le port d'un agent n'est pas un hub : refusé avant l'envoi du code.
        if (agentPort != null) {
            val wrong = service.pair(link.copy(port = agentPort), now) as AgentResult.Failure
            assertEquals(AgentError.OTHER_SERVICE, wrong.error)
        }
    }

    private fun <T> AgentResult<T>.value(): T = when (this) {
        is AgentResult.Success -> value
        is AgentResult.Failure -> error("échec : $error $code $detail")
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
