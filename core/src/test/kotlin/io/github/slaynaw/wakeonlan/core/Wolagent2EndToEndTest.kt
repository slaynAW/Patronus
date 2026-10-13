package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentKey
import io.github.slaynaw.wakeonlan.core.agent.AgentPairing
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.AgentUpgrade
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.agent2.Commands
import io.github.slaynaw.wakeonlan.core.agent2.PairingCode
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.agent2.Request2
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Client
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Exception
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import io.github.slaynaw.wakeonlan.core.share.RoleChange
import io.github.slaynaw.wakeonlan.core.share.ShareAgents
import io.github.slaynaw.wakeonlan.core.share.ShareOwner
import io.github.slaynaw.wakeonlan.core.share.SharePerson
import io.github.slaynaw.wakeonlan.core.share.ShareRight
import io.github.slaynaw.wakeonlan.core.share.agentWork
import io.github.slaynaw.wakeonlan.core.share.applyAgent
import io.github.slaynaw.wakeonlan.core.share.planAgent
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

    /**
     * Ce que fait l'application : l'empreinte est lue dans une réponse wolagent/1 signée par la clé, l'appareil
     * migre sur une session où elle est exigée, puis ne parle plus que wolagent/2 (sans la clé).
     */
    @Test
    fun `client unifie, empreinte annoncee, migration puis wolagent2 seul`() = runBlocking {
        val phone = TestIdentities.newDevice()
        val agentClient = AgentClient(deviceName = { "Pixel unifié" }, identity = { phone })
        val v1 = AgentSettings(port, System.getenv("WOL_E2E_KEY"))
        val announced = (agentClient.status("127.0.0.1", v1) as AgentResult.Success).value
        assertEquals("wolagent/1", announced.proto)
        assertTrue(announced.pin.isNotEmpty() && "wolagent/2" in announced.protocols, "$announced")
        assertEquals(local.pin.toString(), announced.pin, "empreinte annoncée = celle de l'agent")

        val pinned = v1.copy(pin = announced.pin)
        assertEquals(AgentError.UNREGISTERED, (agentClient.status("127.0.0.1", pinned) as AgentResult.Failure).error)
        assertEquals("control", (agentClient.migrate("127.0.0.1", pinned) as AgentResult.Success).value)

        val v2 = pinned.copy(key = "")
        val status = (agentClient.status("127.0.0.1", v2) as AgentResult.Success).value
        assertEquals("wolagent/2", status.proto)
        assertEquals("control", status.role)
        assertEquals(announced.hostname, status.hostname)
        assertTrue(agentClient.power("127.0.0.1", v2, PowerAction.SLEEP) is AgentResult.Success)
        val history = (agentClient.history("127.0.0.1", v2) as AgentResult.Success).value.events
        assertTrue(history.any { it.k == "cmd" && it.a == "sleep" && it.b == "Pixel unifié" && it.i == phone.pin.short() }, "$history")

        val elsewhere = v2.copy(pin = Pin.of("autre agent".toByteArray()).toString())
        assertEquals(AgentError.PIN_MISMATCH, (agentClient.status("127.0.0.1", elsewhere) as AgentResult.Failure).error)
        val noIdentity = AgentClient()
        assertEquals(AgentError.NO_IDENTITY, (noIdentity.status("127.0.0.1", v2) as AgentResult.Failure).error)
    }

    /** Passage automatique (celui de l'application), puis reprise d'une configuration restaurée sur la même installation. */
    @Test
    fun `passage automatique en wolagent2`() = runBlocking {
        val phone = TestIdentities.newDevice()
        val agentClient = AgentClient(deviceName = { "Pixel passage" }, identity = { phone })
        val pc = Device(
            id = "pc", name = "PC CI", mac = MacAddress.parse("AA:BB:CC:DD:EE:09"), host = "127.0.0.1",
            agent = AgentSettings(port, System.getenv("WOL_E2E_KEY")),
        )
        val first = AgentUpgrade.run(agentClient, pc, keepKey = false) as AgentUpgrade.Outcome.Upgraded
        assertEquals("control", first.role)
        assertEquals(local.pin.toString(), first.settings.pin)
        assertEquals("", first.settings.key, "clé effacée")
        assertTrue(agentClient.status("127.0.0.1", first.settings) is AgentResult.Success)
        // Déjà enregistré (configuration restaurée, avec la clé et sans empreinte) : aucune nouvelle migration.
        val again = AgentUpgrade.run(agentClient, pc, keepKey = true) as AgentUpgrade.Outcome.Upgraded
        assertNull(again.role)
        assertEquals(first.settings.pin, again.settings.pin)
        assertEquals(pc.agent!!.key, again.settings.key, "clé gardée pour le partage")
    }

    @Test
    fun `appairage avec un code par le client unifie`() = runBlocking {
        // wol-agent pair sur le PC : invitation (rôle wake) créée avec l'identité de l'agent.
        val agentPin = local.pin
        val invite = client.connect("127.0.0.1", port, local, agentPin).use { admin ->
            admin.request(Request2(cmd = Commands.INVITE, role = "wake", ttl = 120)).invite!!
        }
        val phone = TestIdentities.newDevice()
        val app = AgentClient(deviceName = { "Pixel appairé" }, identity = { phone })
        val settings = AgentSettings(port = port, key = "", pin = agentPin.toString())
        // Code tapé en minuscules avec un tiret : normalisé.
        val paired = app.pair("127.0.0.1", settings, PairingCode.format(invite.code).lowercase())
        assertEquals(AgentResult.Success(AgentPairing("wake")), paired)
        val status = app.status("127.0.0.1", settings)
        assertTrue(status is AgentResult.Success && status.value.proto == "wolagent/2" && status.value.role == "wake", "$status")
        val shutdown = app.power("127.0.0.1", settings, PowerAction.SHUTDOWN)
        assertTrue(shutdown is AgentResult.Failure && shutdown.code == ResponseCode.INSUFFICIENT_ROLE, "$shutdown")
        // Déjà enregistré : le code n'est pas consommé une seconde fois.
        assertEquals(AgentResult.Success(AgentPairing("wake", already = true)), app.pair("127.0.0.1", settings, invite.code))
        // Code mal écrit : refusé sans connexion.
        val bad = app.pair("127.0.0.1", settings, "7KQ2-9MX")
        assertTrue(bad is AgentResult.Failure && bad.error == AgentError.PAIRING_FAILED, "$bad")
        client.connect("127.0.0.1", port, local, agentPin).use { admin ->
            assertTrue(admin.request(Request2(cmd = Commands.DEVICES_REVOKE, device = phone.pin.toString())).ok)
        }
    }

    @Test
    fun `administration des appareils par le client unifie`() = runBlocking {
        val agentPin = local.pin
        fun invite(role: String) = client.connect("127.0.0.1", port, local, agentPin).use { admin ->
            admin.request(Request2(cmd = Commands.INVITE, role = role, ttl = 120)).invite!!.code
        }
        val phone = TestIdentities.newDevice()
        val app = AgentClient(deviceName = { "Pixel admin" }, identity = { phone })
        val settings = AgentSettings(port = port, key = "", pin = agentPin.toString())
        assertTrue(app.pair("127.0.0.1", settings, invite("admin")) is AgentResult.Success)
        val tablet = TestIdentities.newDevice()
        assertTrue(AgentClient(identity = { tablet }).pair("127.0.0.1", settings, invite("view")) is AgentResult.Success)

        val state = app.admin("127.0.0.1", settings)
        assertTrue(state is AgentResult.Success && state.value.devices.any { it.self && it.device == phone.pin.toString() }, "$state")
        val created = app.invite("127.0.0.1", settings, "control", 600, "Invité CI")
        assertTrue(created is AgentResult.Success && created.value.agent == agentPin.toString() && created.value.code.length == 8, "$created")
        val listed = (app.admin("127.0.0.1", settings) as AgentResult.Success).value.invites
        val pending = listed.first { it.name == "Invité CI" }
        assertEquals("", pending.code, "code absent de la liste")
        assertEquals(AgentResult.Success(Unit), app.cancelInvite("127.0.0.1", settings, pending.id))
        assertEquals(AgentResult.Success(Unit), app.setDeviceRole("127.0.0.1", settings, tablet.pin.toString(), "wake"))
        assertEquals(AgentResult.Success(Unit), app.renameDevice("127.0.0.1", settings, tablet.pin.toString(), "Tablette CI"))
        assertEquals(AgentResult.Success(Unit), app.revokeDevice("127.0.0.1", settings, tablet.pin.toString()))
        // Sans empreinte (PC encore en wolagent/1) : rien n'est envoyé.
        val v1 = app.admin("127.0.0.1", AgentSettings(port = port, key = "x"))
        assertTrue(v1 is AgentResult.Failure && v1.error == AgentError.NO_KEY, "$v1")
        // Nettoyage par le PC (le dernier administrateur ne peut être retiré que sur le PC).
        client.connect("127.0.0.1", port, local, agentPin).use { admin ->
            assertTrue(admin.request(Request2(cmd = Commands.DEVICES_REVOKE, device = phone.pin.toString(), force = true)).ok)
        }
    }

    @Test
    fun `partage v2 sur le vrai agent`() = runBlocking {
        val agentPin = local.pin
        val phone = TestIdentities.newDevice()
        val app = AgentClient(deviceName = { "Pixel propriétaire" }, identity = { phone })
        val settings = AgentSettings(port = port, key = "", pin = agentPin.toString())
        val adminCode = client.connect("127.0.0.1", port, local, agentPin).use { admin ->
            admin.request(Request2(cmd = Commands.INVITE, role = "admin", ttl = 120)).invite!!.code
        }
        assertTrue(app.pair("127.0.0.1", settings, adminCode) is AgentResult.Success)

        // Léa demande l'accès avec l'empreinte de son appareil ; « démarrer et éteindre » sur ce PC.
        val lea = TestIdentities.newDevice()
        val now = System.currentTimeMillis() / 1000
        var owner = ShareOwner(
            key = "", publicKey = "", name = "Hugo",
            people = listOf(SharePerson("Léa", "lea", rights = mapOf("pc" to ShareRight.FULL), identity = lea.pin.toString())),
        )
        val first = app.shareSync("127.0.0.1", settings) { view -> owner.planAgent("pc", view, now) }
        assertNull(first.failure, "${first.failure}")
        val created = first.result.created.getValue(lea.pin.toString())
        assertEquals("control", created.role)
        assertTrue(created.expires >= now + ShareAgents.INVITE_TTL_SECONDS - 60, "$created")
        owner = owner.applyAgent("pc", first.plan!!, first.result).first
        val listed = (app.admin("127.0.0.1", settings) as AgentResult.Success).value.invites.single { it.id == created.id }
        assertEquals(lea.pin.toString(), listed.device, "invitation réservée à l'appareil de Léa")
        assertEquals(created.code, owner.pairings.single().code)

        // Rien de plus à faire tant que l'invitation attend.
        assertEquals(false to true, owner.agentWork("pc", now))
        val idle = app.shareSync("127.0.0.1", settings) { view -> owner.planAgent("pc", view, now) }
        assertTrue(idle.plan!!.isEmpty && idle.result.created.isEmpty(), "${idle.plan}")

        // Léa utilise l'invitation : enregistrée avec le rôle « control », constaté à la session suivante.
        val leaApp = AgentClient(deviceName = { "Pixel de Léa" }, identity = { lea })
        assertEquals(AgentResult.Success(AgentPairing("control")), leaApp.pair("127.0.0.1", settings, created.code))
        val second = app.shareSync("127.0.0.1", settings) { view -> owner.planAgent("pc", view, now) }
        val secondPlan = second.plan!!
        assertEquals(listOf(RoleChange(lea.pin.toString(), "control")), secondPlan.registered)
        owner = owner.applyAgent("pc", secondPlan, second.result).first
        assertTrue(owner.pairings.single().registered && owner.pairings.single().code.isEmpty())

        // Accès retiré : son appareil est retiré du PC.
        owner = owner.withdraw("lea")
        val third = app.shareSync("127.0.0.1", settings) { view -> owner.planAgent("pc", view, now) }
        val thirdPlan = third.plan!!
        assertEquals(listOf(lea.pin.toString()), thirdPlan.revoke)
        owner = owner.applyAgent("pc", thirdPlan, third.result).first
        assertTrue(owner.revocations.isEmpty() && owner.pairings.isEmpty())
        val gone = leaApp.status("127.0.0.1", settings)
        assertTrue(gone is AgentResult.Failure && gone.error == AgentError.UNREGISTERED, "$gone")

        client.connect("127.0.0.1", port, local, agentPin).use { admin ->
            assertTrue(admin.request(Request2(cmd = Commands.DEVICES_REVOKE, device = phone.pin.toString(), force = true)).ok)
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
