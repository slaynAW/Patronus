package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent2.InvalidLinkException
import io.github.slaynaw.wakeonlan.core.agent2.PairingLinkV2
import io.github.slaynaw.wakeonlan.core.agent2.Role
import io.github.slaynaw.wakeonlan.core.hub.HubProtocol
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import java.io.File

/** Vecteurs de patronus-hub/1, produits par l'implémentation de référence en Go (agent/hubprotocol). */
class HubTest {
    @Serializable
    private data class LinkVector(
        val name: String,
        val link: String,
        val error: String = "",
        val hubName: String = "",
        val host: String = "",
        val port: Int = 0,
        val hub: String = "",
        val code: String = "",
        val role: String = "",
        val expires: Long = 0,
    )

    @Serializable
    private data class Links(val valid: List<LinkVector>, val invalid: List<LinkVector>)

    @Serializable
    private data class Auth(val registered: Boolean, val role: String = "", val cmd: String, val code: String)

    @Serializable
    private data class CommandRole(val cmd: String, val minRole: String = "")

    @Serializable
    private data class Vectors(
        val protocol: String,
        val port: Int,
        val hello: String,
        val links: Links,
        val roles: List<String>,
        val authorize: List<Auth>,
        val commands: List<CommandRole>,
    )

    private val vectors: Vectors by lazy {
        val dir = System.getProperty("wol.protocolDir") ?: "../protocol"
        json.decodeFromString(File(dir, "hub-vectors.json").readText())
    }

    private companion object {
        val json = Json { ignoreUnknownKeys = true }
    }

    @Test
    fun `protocole, port et salut`() {
        assertEquals(HubProtocol.ALPN, vectors.protocol)
        assertEquals(HubProtocol.DEFAULT_PORT, vectors.port)
        assertEquals("""{"proto":"${HubProtocol.ALPN}"}""", vectors.hello)
    }

    @Test
    fun `liens d appairage du hub`() {
        for (v in vectors.links.valid) {
            val link = HubProtocol.parseLink(v.link)
            assertEquals(v.hubName, link.name, v.name)
            assertEquals(v.host, link.host, v.name)
            assertEquals(v.port, link.port, v.name)
            assertEquals(v.hub, link.agent.toString(), v.name)
            assertEquals(v.code, link.code, v.name)
            assertEquals(v.role, link.role?.wire.orEmpty(), v.name)
            assertEquals(v.expires, link.expires, v.name)
            assertEquals(link, HubProtocol.parseLink(link.text(HubProtocol.LINK)), v.name)
        }
        for (v in vectors.links.invalid) {
            val e = assertThrows<InvalidLinkException>(v.name) { HubProtocol.parseLink(v.link) }
            assertEquals(v.error, e.field, v.name)
        }
        // Un lien du hub n'est jamais lu comme celui d'un agent.
        val hub = vectors.links.valid.first().link
        assertThrows<InvalidLinkException> { PairingLinkV2.parse(hub) }
    }

    @Test
    fun `roles et autorisation`() {
        assertEquals(vectors.roles, HubProtocol.ROLES.map { it.wire })
        assertEquals(vectors.commands.map { it.cmd }, HubProtocol.ALL)
        for (c in vectors.commands) {
            assertEquals(c.minRole, HubProtocol.MIN_ROLE[c.cmd]?.wire.orEmpty(), c.cmd)
        }
        for (a in vectors.authorize) {
            val role = Role.parse(a.role)
            assertEquals(a.code, HubProtocol.authorize(a.registered, role, a.cmd), "${a.registered} ${a.role} ${a.cmd}")
        }
    }
}
