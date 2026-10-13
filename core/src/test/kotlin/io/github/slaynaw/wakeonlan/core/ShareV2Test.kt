package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.config.ConfigCodec
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.share.AgentInvite
import io.github.slaynaw.wakeonlan.core.share.AgentPlan
import io.github.slaynaw.wakeonlan.core.share.PlanResult
import io.github.slaynaw.wakeonlan.core.share.AgentView
import io.github.slaynaw.wakeonlan.core.share.ExportedShareOwner
import io.github.slaynaw.wakeonlan.core.share.ShareContent
import io.github.slaynaw.wakeonlan.core.share.ShareCrypto
import io.github.slaynaw.wakeonlan.core.share.ShareException
import io.github.slaynaw.wakeonlan.core.share.ShareLinks
import io.github.slaynaw.wakeonlan.core.share.ShareOwner
import io.github.slaynaw.wakeonlan.core.share.SharePairing
import io.github.slaynaw.wakeonlan.core.share.SharePerson
import io.github.slaynaw.wakeonlan.core.share.ShareRequest
import io.github.slaynaw.wakeonlan.core.share.ShareRevocation
import io.github.slaynaw.wakeonlan.core.share.ShareRight
import io.github.slaynaw.wakeonlan.core.share.ShareAgents
import io.github.slaynaw.wakeonlan.core.share.agentWork
import io.github.slaynaw.wakeonlan.core.share.applyAgent
import io.github.slaynaw.wakeonlan.core.share.needsKey
import io.github.slaynaw.wakeonlan.core.share.planAgent
import kotlinx.serialization.Serializable
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

/** Partage v2 : vecteurs communs avec Windows (protocol/share-v2-vectors.json, docs/PARTAGE.md). */
class ShareV2Test {
    @Serializable
    data class Case(val name: String, val person: SharePerson, val content: ShareContent)

    @Serializable
    data class Step(
        val op: String,
        val person: SharePerson? = null,
        val device: String = "",
        val pairings: List<SharePairing>,
        val revocations: List<ShareRevocation>,
    )

    @Serializable
    data class Revocations(val people: List<SharePerson>, val pairings: List<SharePairing>, val steps: List<Step>)

    @Serializable
    data class RequestFields(val name: String, val device: String, val owner: String, val identity: String = "")

    @Serializable
    data class RequestVector(val link: String, val fields: RequestFields)

    @Serializable
    data class ContentVector(val name: String, val plaintext: String, val identity: String = "", val invites: List<AgentInvite> = emptyList())

    @Serializable
    data class Plan(
        val name: String, val device: String, val now: Long,
        val people: List<SharePerson>, val pairings: List<SharePairing>, val revocations: List<ShareRevocation>,
        val view: AgentView, val work: Boolean, val poll: Boolean, val plan: AgentPlan, val result: PlanResult,
        val afterPairings: List<SharePairing>, val afterRevocations: List<ShareRevocation>,
        val afterWork: Boolean, val afterPoll: Boolean,
    )

    @Serializable
    data class Vectors(
        val ownerPrivate: String, val ownerPublic: String, val devicePrivate: String, val devicePublic: String,
        val now: Long, val ownerName: String, val devices: List<Device>, val pairings: List<SharePairing>,
        val cases: List<Case>, val revocations: Revocations,
        val request: RequestVector, val invalidRequests: List<String>,
        val contents: List<ContentVector>, val invalidContents: List<ContentVector>,
        val plans: List<Plan>,
    )

    private val dir = System.getProperty("wol.protocolDir") ?: "../protocol"
    private val v: Vectors = ConfigCodec.json.decodeFromString(Vectors.serializer(), File(dir, "share-v2-vectors.json").readText())
    private val owner = ShareCrypto.keyPair(v.ownerPrivate, v.ownerPublic)
    private val device = ShareCrypto.keyPair(v.devicePrivate, v.devicePublic)

    @Test
    fun `contenu identique a Windows pour chaque regle`() {
        val o = ShareOwner(key = "", publicKey = "", name = v.ownerName, pairings = v.pairings)
        for (c in v.cases) assertEquals(c.content, o.content(c.person, v.devices, v.now * 1000), c.name)
    }

    @Test
    fun `cle de l agent jamais transmise a un appareil enregistrable`() {
        val o = ShareOwner(key = "", publicKey = "", name = v.ownerName, pairings = v.pairings)
        val key = v.devices.single { it.id == "c" }.agent!!.key
        for (c in v.cases.filter { it.person.identity.isNotEmpty() }) {
            val text = ConfigCodec.json.encodeToString(ShareContent.serializer(), o.content(c.person, v.devices, v.now * 1000))
            assertFalse(text.contains(key), c.name)
        }
        // Partage v1 : contenu inchangé, sans les champs du partage v2.
        val v1 = ConfigCodec.json.encodeToString(ShareContent.serializer(), o.content(v.cases.first().person, v.devices, v.now * 1000))
        assertFalse(v1.contains("identity") || v1.contains("invites"), v1)
    }

    @Test
    fun `file des retraits identique a Windows`() {
        var o = ShareOwner(key = "", publicKey = "", name = "Hugo", people = v.revocations.people, pairings = v.revocations.pairings)
        for ((n, step) in v.revocations.steps.withIndex()) {
            o = when (step.op) {
                "grant" -> o.grant(step.person!!)
                "withdraw" -> o.withdraw(step.device)
                else -> error("étape ${step.op}")
            }
            assertEquals(step.pairings, o.pairings, "étape ${n + 1} : appairages")
            assertEquals(step.revocations, o.revocations, "étape ${n + 1} : retraits")
        }
    }

    @Test
    fun `plans sur un PC identiques a Windows`() {
        for ((n, p) in v.plans.withIndex()) {
            var o = ShareOwner(key = "", publicKey = "", name = "Hugo", people = p.people, pairings = p.pairings, revocations = p.revocations)
            assertEquals(p.work to p.poll, o.agentWork(p.device, p.now), "${p.name} : travail")
            assertEquals(p.plan, o.planAgent(p.device, p.view, p.now), "${p.name} : plan")
            // Troisième scénario : Paul est retiré pendant l'échange.
            if (n == 2) o = o.withdraw(p.people[1].device)
            o = o.applyAgent(p.device, p.plan, p.result).first
            assertEquals(p.afterPairings, o.pairings, "${p.name} : appairages")
            assertEquals(p.afterRevocations, o.revocations, "${p.name} : retraits")
            assertEquals(p.afterWork to p.afterPoll, o.agentWork(p.device, p.now), "${p.name} : travail après")
        }
        // Échec avant les invitations : rien ne change.
        val p = v.plans.first()
        val o = ShareOwner(key = "", publicKey = "", name = "Hugo", people = p.people, pairings = p.pairings, revocations = p.revocations)
        val (same, changed) = o.applyAgent(p.device, p.plan, PlanResult())
        assertFalse(changed)
        assertEquals(o, same)
    }

    @Test
    fun `droits possibles et cle gardee`() {
        val cases = listOf(
            listOf(ShareRight.WAKE, true, true, true, ShareRight.WAKE),
            listOf(ShareRight.VIEW, true, true, false, ShareRight.VIEW),
            listOf(ShareRight.VIEW, false, true, true, ShareRight.WAKE),
            listOf(ShareRight.VIEW, true, false, true, ShareRight.WAKE),
            listOf(ShareRight.FULL, true, true, false, ShareRight.FULL),
            listOf(ShareRight.FULL, false, true, true, ShareRight.FULL),
            listOf(ShareRight.FULL, true, false, true, ShareRight.FULL),
            listOf(ShareRight.FULL, false, true, false, ShareRight.WAKE),
            listOf(ShareRight.FULL, true, false, false, ShareRight.WAKE),
        )
        for (c in cases) {
            assertEquals(c[4], ShareAgents.allowedRight(c[0] as ShareRight, c[1] as Boolean, c[2] as Boolean, c[3] as Boolean), c.toString())
        }
        val o = ShareOwner(
            key = "", publicKey = "", name = "Hugo",
            people = listOf(
                SharePerson("Léa", "a", rights = mapOf("c" to ShareRight.FULL, "d" to ShareRight.VIEW), identity = v.request.fields.identity),
                SharePerson("Paul", "b", rights = mapOf("d" to ShareRight.FULL, "e" to ShareRight.WAKE)),
            ),
        )
        assertFalse(o.needsKey("c", true))
        assertTrue(o.needsKey("c", false))
        assertTrue(o.needsKey("d", true))
        assertFalse(o.needsKey("e", true))
        assertFalse(o.needsKey("z", true))
    }

    @Test
    fun `demande d acces avec empreinte`() {
        val r = ShareLinks.parseRequest(v.request.link)
        assertEquals(v.request.fields, RequestFields(r.name, r.device, r.owner, r.identity))
        assertEquals(r, ShareLinks.parseRequest(r.link))
        assertTrue(r.link.contains("&id="))
        // Sans empreinte : la demande des versions précédentes, inchangée.
        val v1 = r.copy(identity = "")
        assertFalse(v1.link.contains("id="))
        assertEquals(v1, ShareLinks.parseRequest(v1.link))
        for (link in v.invalidRequests) assertThrows(ShareException::class.java, { ShareLinks.parseRequest(link) }, link)
        assertFalse(ShareRequest(r.name, r.device, r.owner, "AAAA").isValid)
    }

    private fun seal(plain: String): String =
        ShareCrypto.sealRaw(owner, device.publicEncoded, 1, "2026-10-13T12:00:00Z", plain.toByteArray(), ShareCrypto.newKeyPair(), ByteArray(12) { it.toByte() })

    @Test
    fun `contenus v2 relus comme Windows`() {
        for (c in v.contents) {
            val opened = ShareCrypto.open(seal(c.plaintext), v.ownerPublic, device, 0).content
            assertEquals(c.identity, opened.identity, c.name)
            assertEquals(c.invites, opened.invites, c.name)
            assertEquals(4, opened.devices.size, c.name)
        }
        for (c in v.invalidContents) {
            val e = assertThrows(ShareException::class.java, { ShareCrypto.open(seal(c.plaintext), v.ownerPublic, device, 0) }, c.name)
            assertEquals(ShareException.Reason.INVALID, e.reason, c.name)
        }
    }

    @Test
    fun `empreintes et retraits dans la sauvegarde complete`() {
        val key = ShareCrypto.newKeyPair()
        val person = v.cases[1].person
        val revocation = ShareRevocation("c", v.pairings.last().identity)
        val o = ShareOwner(
            key = key.privateEncoded, publicKey = key.publicEncoded, name = "Hugo", people = listOf(person),
            pairings = v.pairings, revocations = listOf(revocation),
        )
        val exported = ExportedShareOwner.of(o)
        val text = ConfigCodec.json.encodeToString(ExportedShareOwner.serializer(), exported)
        assertFalse(text.contains("7KQ29MXP") || text.contains("pairings"), "appairages exportés")
        val restored = ConfigCodec.json.decodeFromString(ExportedShareOwner.serializer(), text).toOwner()
        assertEquals(person.identity, restored.people.single().identity)
        assertEquals(listOf(revocation), restored.revocations)
        assertTrue(restored.pairings.isEmpty())
        for (bad in listOf(
            exported.copy(people = listOf(person.copy(identity = "AAAA"))),
            exported.copy(revocations = listOf(ShareRevocation("", revocation.identity))),
            exported.copy(revocations = listOf(ShareRevocation("c", "AAAA"))),
        )) {
            assertThrows(ShareException::class.java) { bad.toOwner() }
        }
        assertEquals(ShareRight.WAKE.agentRole, null)
        assertEquals("wake", ShareRight.VIEW.agentRole?.wire)
        assertEquals("control", ShareRight.FULL.agentRole?.wire)
    }
}
