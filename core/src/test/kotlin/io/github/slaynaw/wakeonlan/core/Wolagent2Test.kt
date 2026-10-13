package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent2.CertificateRejectedException
import io.github.slaynaw.wakeonlan.core.agent2.Commands
import io.github.slaynaw.wakeonlan.core.agent2.FrameException
import io.github.slaynaw.wakeonlan.core.agent2.Frames
import io.github.slaynaw.wakeonlan.core.agent2.InvalidLinkException
import io.github.slaynaw.wakeonlan.core.agent2.Migration
import io.github.slaynaw.wakeonlan.core.agent2.PairingCode
import io.github.slaynaw.wakeonlan.core.agent2.PairingLinkV2
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.agent2.Role
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.File
import java.util.Base64
import kotlin.random.Random

/** wolagent/2 : mêmes vecteurs que l'agent Go (protocol/v2-vectors.json, agent/protocol2). */
class Wolagent2Test {
    @Serializable
    private data class PinVector(val name: String, val certificate: String, val pin: String = "", val short: String = "", val error: String = "")

    @Serializable
    private data class Pins(val valid: List<PinVector>, val invalid: List<PinVector>)

    @Serializable
    private data class CodeEncode(val bytes: String, val code: String)

    @Serializable
    private data class CodeNormalize(val input: String, val code: String, val display: String)

    @Serializable
    private data class CodeInvalid(val name: String, val input: String)

    @Serializable
    private data class Codes(val encode: List<CodeEncode>, val normalize: List<CodeNormalize>, val invalid: List<CodeInvalid>)

    @Serializable
    private data class LinkFields(
        val name: String,
        val host: String,
        val port: Int,
        val mac: String,
        val agent: String,
        val code: String,
        val role: String,
        val expires: Long,
    )

    @Serializable
    private data class LinkVector(val name: String, val link: String, val canonical: Boolean, val expect: LinkFields)

    @Serializable
    private data class LinkInvalid(val name: String, val link: String, val field: String)

    @Serializable
    private data class LinkExpiry(val link: String, val now: Long, val expired: Boolean)

    @Serializable
    private data class Links(val valid: List<LinkVector>, val invalid: List<LinkInvalid>, val expiry: List<LinkExpiry>)

    @Serializable
    private data class MigrationVector(
        val name: String,
        val key: String,
        val agent: String,
        val device: String,
        val nc: String,
        val na: String,
        val agentProof: String,
        val deviceProof: String,
    )

    @Serializable
    private data class AuthorizeRow(
        val cmd: String,
        val minRole: String,
        val unregistered: String,
        val view: String,
        val wake: String,
        val control: String,
        val admin: String,
    )

    @Serializable
    private data class Roles(val order: List<String>, val authorize: List<AuthorizeRow>)

    @Serializable
    private data class FrameStream(val name: String, val stream: String, val limit: Int, val payloads: List<String>, val end: String)

    @Serializable
    private data class FrameVectors(val maxRequest: Int, val maxResponse: Int, val streams: List<FrameStream>)

    @Serializable
    private data class Vectors(
        val description: String,
        val protocol: String,
        val pins: Pins,
        val codes: Codes,
        val links: Links,
        val migration: List<MigrationVector>,
        val roles: Roles,
        val frames: FrameVectors,
    )

    // Lecture stricte : un champ inconnu signale un fichier que ce test ne vérifierait qu'en partie.
    private val v: Vectors = Json.decodeFromString(
        File(System.getProperty("wol.protocolDir") ?: "../protocol", "v2-vectors.json").readText(),
    )

    private fun b64(text: String): ByteArray = Base64.getUrlDecoder().decode(text)

    private fun hex(text: String): ByteArray = ByteArray(text.length / 2) { text.substring(it * 2, it * 2 + 2).toInt(16).toByte() }

    @Test
    fun `protocole`() {
        assertEquals(Wolagent2.ALPN, v.protocol)
    }

    @Test
    fun `empreintes des certificats`() {
        val certificatesByPin = mutableMapOf<String, MutableSet<String>>()
        for (case in v.pins.valid) {
            val pin = Pin.fromCertificate(b64(case.certificate))
            assertEquals(case.pin, pin.toString(), case.name)
            assertEquals(case.short, pin.short(), case.name)
            assertEquals(pin, Pin.parse(case.pin), case.name)
            certificatesByPin.getOrPut(case.pin) { mutableSetOf() } += case.certificate
        }
        assertTrue(certificatesByPin.values.any { it.size > 1 }, "deux certificats d'une même clé")
        for (case in v.pins.invalid) {
            val e = assertThrows<CertificateRejectedException>(case.name) { Pin.fromCertificate(b64(case.certificate)) }
            val expected = mapOf("certificate" to CertificateRejectedException.Reason.CERTIFICATE, "key_type" to CertificateRejectedException.Reason.KEY_TYPE)
            assertEquals(expected[case.error], e.reason, case.name)
        }
    }

    @Test
    fun `empreintes mal ecrites`() {
        val pin = v.pins.valid.first().pin
        for (bad in listOf("", pin.dropLast(1), pin + "A", "$pin=", pin.take(20) + "\n" + pin.drop(21), "+" + pin.drop(1))) {
            assertNull(Pin.parse(bad), bad)
        }
        // Bits de fin non nuls : une seule écriture admise par empreinte.
        val last = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_".indexOf(pin.last())
        assertNull(Pin.parse(pin.dropLast(1) + "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"[last or 1]))
    }

    @Test
    fun `empreintes courtes`() {
        fun pinOf(bytes: ByteArray) = Pin.parse(Base64.getUrlEncoder().withoutPadding().encodeToString(bytes))!!
        assertEquals("0000-0000-0000", pinOf(ByteArray(32)).short())
        assertEquals("ZZZZ-ZZZZ-ZZZZ", pinOf(ByteArray(32) { -1 }).short())
        val pattern = hex("0842108421084210").copyOf(32)
        assertEquals("1111-1111-1111", pinOf(pattern).short())
    }

    @Test
    fun `codes d'appairage`() {
        for (case in v.codes.encode) assertEquals(case.code, PairingCode.encode(hex(case.bytes)), case.bytes)
        for (case in v.codes.normalize) {
            assertEquals(case.code, PairingCode.normalize(case.input), case.input)
            assertEquals(case.display, PairingCode.format(case.code))
        }
        for (case in v.codes.invalid) assertNull(PairingCode.normalize(case.input), case.name)
        repeat(200) {
            val code = PairingCode.generate()
            for (typed in listOf(code, PairingCode.format(code), PairingCode.format(code).lowercase())) {
                assertEquals(code, PairingCode.normalize(typed), typed)
            }
        }
    }

    private fun fieldsOf(link: PairingLinkV2) = LinkFields(
        link.name, link.host, link.port, link.mac, link.agent.toString(), link.code, link.role?.wire.orEmpty(), link.expires,
    )

    @Test
    fun `liens d'appairage`() {
        for (case in v.links.valid) {
            val link = PairingLinkV2.parse(case.link)
            assertEquals(case.expect, fieldsOf(link), case.name)
            assertEquals(case.canonical, link.text() == case.link, case.name)
            assertEquals(link, PairingLinkV2.parse(link.text()), case.name)
            assertFalse(link.toString().contains(link.code), "code visible dans toString")
        }
        for (case in v.links.invalid) {
            val e = assertThrows<InvalidLinkException>(case.name) { PairingLinkV2.parse(case.link) }
            assertEquals(case.field, e.field, case.name)
        }
        for (case in v.links.expiry) assertEquals(case.expired, PairingLinkV2.parse(case.link).expired(case.now), "${case.link} à ${case.now}")
    }

    @Test
    fun `liens relus a l'identique`() {
        val random = Random(42)
        val agent = Pin.parse(v.pins.valid.first().pin)!!
        val names = listOf("PC", "PC de Léa", "a+b & c=d %41 #1", "😀".repeat(32), "é".repeat(64), "Bureau ~*'()")
        val hosts = listOf("192.168.1.20", "pc-bureau.local", "::1", "fe80::1%eth0", "pc.local.")
        repeat(300) {
            val link = PairingLinkV2(
                name = names.random(random),
                host = hosts.random(random),
                port = random.nextInt(1, 65536),
                mac = if (random.nextBoolean()) "" else "0A:1B:2C:3D:4E:5F",
                agent = agent,
                code = PairingCode.encode(random.nextBytes(5)),
                role = Role.entries.plus(listOf(null)).random(random),
                expires = if (random.nextBoolean()) 0 else random.nextLong(1, PairingLinkV2.MAX_EXPIRES + 1),
            )
            assertEquals(link, PairingLinkV2.parse(link.text()), link.text())
        }
    }

    @Test
    fun `preuves de migration`() {
        for (case in v.migration) {
            val key = b64(case.key)
            val agent = Pin.parse(case.agent)!!
            val device = Pin.parse(case.device)!!
            val nc = Migration.decode(case.nc)!!
            val na = Migration.decode(case.na)!!
            val pa = Migration.agentProof(key, agent, device, nc, na)
            val pd = Migration.deviceProof(key, agent, device, nc, na)
            assertEquals(case.agentProof, Migration.encode(pa), case.name)
            assertEquals(case.deviceProof, Migration.encode(pd), case.name)
            assertTrue(Migration.proofsEqual(pa, Migration.decode(case.agentProof)!!))
            assertFalse(Migration.proofsEqual(pa, pd), case.name)
            assertNotEquals(Migration.encode(Migration.agentProof(key, device, agent, nc, na)), case.agentProof, "empreintes inversées")
        }
    }

    @Test
    fun `roles et droits`() {
        assertEquals(Role.entries.map { it.wire }, v.roles.order)
        assertTrue(v.roles.authorize.map { it.cmd }.containsAll(Commands.ALL))
        for (row in v.roles.authorize) {
            val kind = when {
                Commands.isEnrollment(row.cmd) -> "pairing"
                else -> Commands.MIN_ROLE[row.cmd]?.wire ?: "unknown"
            }
            assertEquals(row.minRole, kind, row.cmd)
            assertEquals(row.unregistered, Commands.authorize(false, null, row.cmd), row.cmd)
            assertEquals(listOf(row.view, row.wake, row.control, row.admin), Role.entries.map { Commands.authorize(true, it, row.cmd) }, row.cmd)
        }
    }

    @Test
    fun `trames`() {
        assertEquals(Wolagent2.MAX_REQUEST_BYTES, v.frames.maxRequest)
        assertEquals(Wolagent2.MAX_RESPONSE_BYTES, v.frames.maxResponse)
        for (case in v.frames.streams) {
            val bytes = hex(case.stream)
            val input = ByteArrayInputStream(bytes)
            val payloads = mutableListOf<String>()
            val end = try {
                while (true) {
                    val payload = Frames.read(input, case.limit) ?: break
                    payloads += payload.decodeToString()
                }
                "eof"
            } catch (e: FrameException) {
                e.kind.name.lowercase()
            }
            assertEquals(case.payloads, payloads, case.name)
            assertEquals(case.end, end, case.name)
            if (case.end == "eof") {
                val out = ByteArrayOutputStream()
                case.payloads.forEach { Frames.write(out, it.encodeToByteArray(), case.limit) }
                assertArrayEquals(bytes, out.toByteArray(), case.name)
            }
        }
        assertThrows<FrameException> { Frames.write(ByteArrayOutputStream(), ByteArray(0), 10) }
        assertThrows<FrameException> { Frames.write(ByteArrayOutputStream(), ByteArray(11), 10) }
    }

    @Test
    fun `noms nettoyes`() {
        assertEquals("Pixel 8", Wolagent2.cleanName("  Pixel 8  ", 40))
        assertEquals("abcd", Wolagent2.cleanName("a\u0000b\u007fc\u009fd\r\n", 40))
        assertEquals("abc", Wolagent2.cleanName("abc def", 4))
        assertEquals("ab", Wolagent2.cleanName("ab😀", 3))
    }
}
