package io.github.slaynaw.wakeonlan.core

import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpServer
import io.github.slaynaw.wakeonlan.core.config.ConfigCodec
import io.github.slaynaw.wakeonlan.core.config.ExportCodec
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import io.github.slaynaw.wakeonlan.core.share.ExportedShareOwner
import io.github.slaynaw.wakeonlan.core.share.QrCode
import io.github.slaynaw.wakeonlan.core.share.ShareAccess
import io.github.slaynaw.wakeonlan.core.share.ShareContent
import io.github.slaynaw.wakeonlan.core.share.ShareCrypto
import io.github.slaynaw.wakeonlan.core.share.ShareException
import io.github.slaynaw.wakeonlan.core.share.ShareGitHub
import io.github.slaynaw.wakeonlan.core.share.ShareLinks
import io.github.slaynaw.wakeonlan.core.share.ShareOwner
import io.github.slaynaw.wakeonlan.core.share.SharePerson
import io.github.slaynaw.wakeonlan.core.share.ShareRight
import io.github.slaynaw.wakeonlan.core.share.ShareSyncResult
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File
import java.net.InetAddress
import java.net.InetSocketAddress
import java.util.Base64

/** Partage : vecteurs communs avec Windows (protocol/share-vectors.json, qr-vectors.json). */
class ShareTest {
    @Serializable
    data class Expected(
        val ownerKeyId: String, val deviceKeyId: String, val fileName: String, val verificationCode: String,
        val epk: String, val data: String, val ownerName: String, val recipientName: String,
        val deviceNames: List<String>, val agentKeys: List<String>,
    )

    @Serializable
    data class LinkVector<T>(val link: String, val fields: T)

    @Serializable
    data class InviteFields(val name: String, val owner: String, val user: String, val gist: String)

    @Serializable
    data class RequestFields(val name: String, val device: String, val owner: String)

    @Serializable
    data class Invalid(val name: String, val file: String, val error: String)

    @Serializable
    data class Vectors(
        val ownerPrivate: String, val ownerPublic: String, val devicePrivate: String, val devicePublic: String,
        val otherDevicePrivate: String, val otherDevicePublic: String,
        val ephemeralPrivate: String, val ephemeralPublic: String,
        val iv: String, val revision: Long, val issuedAt: String, val plaintext: String,
        val expected: Expected, val file: String,
        val invite: LinkVector<InviteFields>, val request: LinkVector<RequestFields>,
        val invalid: List<Invalid>, val invalidLinks: List<String>,
    )

    @Serializable
    data class QrVector(val text: String, val rows: List<String>)

    private val json = Json { ignoreUnknownKeys = true }
    private val prettyJson = Json(json) { prettyPrint = true }
    private val dir = System.getProperty("wol.protocolDir") ?: "../protocol"
    private val v: Vectors = json.decodeFromString(File(dir, "share-vectors.json").readText())
    private val owner = ShareCrypto.keyPair(v.ownerPrivate, v.ownerPublic)
    private val device = ShareCrypto.keyPair(v.devicePrivate, v.devicePublic)

    @Test
    fun `identifiants et code de verification`() {
        assertEquals(v.expected.ownerKeyId, ShareCrypto.keyId(v.ownerPublic))
        assertEquals(v.expected.deviceKeyId, ShareCrypto.keyId(v.devicePublic))
        assertEquals(v.expected.fileName, ShareCrypto.fileName(v.devicePublic))
        assertEquals(v.expected.verificationCode, ShareCrypto.verificationCode(v.ownerPublic, v.devicePublic))
    }

    @Test
    fun `chiffrement identique a Windows`() {
        val eph = ShareCrypto.keyPair(v.ephemeralPrivate, v.ephemeralPublic)
        val p = ShareCrypto.encrypt(v.ownerPublic, v.devicePublic, v.revision, v.issuedAt, v.plaintext.toByteArray(), eph, Base64.getDecoder().decode(v.iv))
        assertEquals(v.expected.epk, p.epk)
        assertEquals(v.expected.data, p.data)
    }

    @Test
    fun `fichier de Windows verifie et dechiffre`() {
        val opened = ShareCrypto.open(v.file, v.ownerPublic, device, 0)
        assertEquals(v.revision, opened.revision)
        assertEquals(v.expected.ownerName, opened.content.ownerName)
        assertEquals(v.expected.recipientName, opened.content.recipientName)
        assertEquals(v.expected.deviceNames, opened.content.devices.map { it.name })
        assertEquals(v.expected.agentKeys, opened.content.devices.map { it.agent?.key.orEmpty() })
    }

    @Test
    fun `fichiers refuses`() {
        val other = ShareCrypto.keyPair(v.otherDevicePrivate, v.otherDevicePublic)
        for (case in v.invalid) {
            val key = if (case.name == "autre appareil") other else device
            val e = assertThrows(ShareException::class.java, { ShareCrypto.open(case.file, v.ownerPublic, key, v.revision) }, case.name)
            val expected = when (case.error) {
                "signature" -> ShareException.Reason.SIGNATURE
                "recipient" -> ShareException.Reason.RECIPIENT
                "older" -> ShareException.Reason.OLDER
                else -> ShareException.Reason.INVALID
            }
            assertEquals(expected, e.reason, case.name)
        }
    }

    @Test
    fun `fichier Android relu, contenu illisible sans la cle`() {
        val content = ShareContent("Hugo", "Léa", listOf(device("a", "PC streaming", "192.168.1.20")))
        val text = ShareCrypto.seal(owner, device.publicEncoded, 7, "2026-09-28T15:00:00Z", content)
        assertFalse(text.contains("streaming") || text.contains("192.168"))
        assertEquals("PC streaming", ShareCrypto.open(text, owner.publicEncoded, device, 7).content.devices.single().name)
        val intruder = ShareCrypto.newKeyPair()
        val forged = ShareCrypto.seal(intruder, device.publicEncoded, 9, "", content)
        assertEquals(ShareException.Reason.SIGNATURE, assertThrows(ShareException::class.java) { ShareCrypto.open(forged, owner.publicEncoded, device, 0) }.reason)
        assertEquals(ShareException.Reason.OLDER, assertThrows(ShareException::class.java) { ShareCrypto.open(text, owner.publicEncoded, device, 8) }.reason)
    }

    @Test
    fun `liens de partage`() {
        val inv = ShareLinks.parseInvite(v.invite.link)
        assertEquals(v.invite.fields, InviteFields(inv.name, inv.owner, inv.user, inv.gist))
        val req = ShareLinks.parseRequest(v.request.link)
        assertEquals(v.request.fields, RequestFields(req.name, req.device, req.owner))
        // Aller-retour Android, relu identique.
        assertEquals(inv, ShareLinks.parseInvite(inv.link))
        assertEquals(req, ShareLinks.parseRequest("  " + req.link + "\n"))
        assertEquals("invite", ShareLinks.kind(inv.link))
        assertEquals("request", ShareLinks.kind(req.link))
        assertNull(ShareLinks.kind("wolagent://pair?x=1"))
        for (link in v.invalidLinks) {
            assertThrows(ShareException::class.java, { ShareLinks.parseInvite(link) }, link)
            assertThrows(ShareException::class.java, { ShareLinks.parseRequest(link) }, link)
        }
        assertFalse(ShareCrypto.isValidName(" Hugo"))
        assertTrue(ShareCrypto.isValidName("é".repeat(40)))
        assertFalse(ShareCrypto.isValidName("a".repeat(41)))
    }

    @Test
    fun `QR code identique a Windows`() {
        val vectors: List<QrVector> = json.decodeFromString(File(dir, "qr-vectors.json").readText())
        for (q in vectors) {
            val code = QrCode.encode(q.text)
            val rows = (0 until code.size).map { y -> (0 until code.size).joinToString("") { x -> if (code.isDark(x, y)) "#" else "." } }
            assertEquals(q.rows.joinToString("\n"), rows.joinToString("\n"), q.text)
        }
    }

    @Test
    fun `publication puis reception`() {
        val ownerKey = ShareCrypto.newKeyPair()
        val guest = ShareCrypto.newKeyPair()
        val devices = listOf(
            device("a", "PC streaming", "192.168.1.20", agent = true),
            device("b", "Bureau", "192.168.1.21", agent = true),
        )
        var o = ShareOwner(key = ownerKey.privateEncoded, publicKey = ownerKey.publicEncoded, name = "Hugo")
            .grant(SharePerson("Léa", guest.publicEncoded, 1, mapOf("a" to ShareRight.WAKE)))
        val pub = o.prepare(devices, 1_790_600_000_000)
        assertEquals(1, pub.files.size)
        o = o.commit(pub)
        assertTrue(o.prepare(devices, 1_790_600_060_000).isEmpty, "rien à republier")

        var access = ShareAccess(owner = ownerKey.publicEncoded, ownerName = "Hugo", user = "slaynAW", gist = "0123456789abcdef0123456789abcdef", myName = "Léa")
        val files = HashMap(pub.files.mapValues { it.value!! })
        var (next, result) = access.apply(files, guest, 1)
        assertEquals(ShareSyncResult.GRANTED, result)
        access = next
        assertEquals(listOf("PC streaming"), access.devices.map { it.name })
        assertNull(access.devices.single().agent, "pas de clé d'agent pour « démarrer »")

        // Droits étendus : la clé de l'agent est transmise.
        o = o.grant(SharePerson("Léa", guest.publicEncoded, 2, mapOf("a" to ShareRight.FULL, "b" to ShareRight.WAKE)))
        val pub2 = o.prepare(devices, 1_790_600_120_000)
        o = o.commit(pub2)
        files[ShareCrypto.fileName(guest.publicEncoded)] = pub2.files.values.single()!!
        access.apply(files, guest, 2).let { (a, r) ->
            assertEquals(ShareSyncResult.UPDATED, r)
            assertNotNull(a.devices.first().agent)
            assertEquals(2, a.devices.size)
            access = a
        }
        // Ancienne version refusée.
        assertThrows(ShareException::class.java) { access.apply(pub.files.mapValues { it.value!! }, guest, 3) }

        // Retrait.
        o = o.withdraw(guest.publicEncoded)
        val pub3 = o.prepare(devices, 1_790_600_180_000)
        assertTrue(pub3.files.containsKey(ShareCrypto.fileName(guest.publicEncoded)) && pub3.files.values.single() == null)
        o = o.commit(pub3)
        assertTrue(o.withdrawn.isEmpty())
        val (removed, r) = access.apply(emptyMap(), guest, 4)
        assertEquals(ShareSyncResult.WITHDRAWN, r)
        assertTrue(removed.removed && !removed.active && removed.devices.isEmpty())
        assertEquals(ShareAccess.sharedId("x", "a"), ShareAccess.sharedId("x", "a"))
        assertTrue(ShareAccess.sharedId("x", "a").startsWith("s-"))
    }

    @Test
    fun `client GitHub`() {
        val gists = HashMap<String, HashMap<String, String>>()
        var seq = 0
        var polls = 0
        var lastAuth: String? = "?"
        val server = HttpServer.create(InetSocketAddress(InetAddress.getLoopbackAddress(), 0), 0)
        fun HttpExchange.reply(code: Int, body: String = "", etag: String? = null) {
            etag?.let { responseHeaders.add("ETag", it) }
            val bytes = body.toByteArray()
            sendResponseHeaders(code, if (bytes.isEmpty()) -1 else bytes.size.toLong())
            if (bytes.isNotEmpty()) responseBody.use { it.write(bytes) }
            close()
        }
        server.createContext("/login/device/code") { it.reply(200, """{"device_code":"d","user_code":"ABCD-1234","verification_uri":"https://github.com/login/device","expires_in":600,"interval":1}""") }
        server.createContext("/login/oauth/access_token") {
            polls++
            it.reply(200, if (polls < 2) """{"error":"authorization_pending"}""" else """{"access_token":"tok","scope":"gist"}""")
        }
        server.createContext("/user") { it.reply(if (it.requestHeaders.getFirst("Authorization") == "Bearer tok") 200 else 401, """{"login":"slaynAW"}""") }
        server.createContext("/gists") { ex ->
            val id = ex.requestURI.path.removePrefix("/gists").trim('/')
            val body = ex.requestBody.readBytes().decodeToString()
            when (ex.requestMethod) {
                "POST" -> {
                    val files = Json.parseToJsonElement(body).jsonObject["files"]!!.jsonObject
                    gists["0123456789abcdef0123456789abcdef"] = HashMap(files.mapValues { it.value.jsonObject["content"]!!.jsonPrimitive.content })
                    seq++
                    ex.reply(201, """{"id":"0123456789abcdef0123456789abcdef"}""")
                }
                "PATCH" -> {
                    val g = gists[id] ?: return@createContext ex.reply(404)
                    Json.parseToJsonElement(body).jsonObject["files"]!!.jsonObject.forEach { (name, f) ->
                        if (f is JsonNull) g.remove(name) else g[name] = (f as JsonObject)["content"]!!.jsonPrimitive.content
                    }
                    seq++
                    ex.reply(200, "{}")
                }
                "DELETE" -> {
                    gists.remove(id)
                    ex.reply(204)
                }
                else -> {
                    lastAuth = ex.requestHeaders.getFirst("Authorization")
                    val g = gists[id] ?: return@createContext ex.reply(404)
                    val etag = "\"v$seq\""
                    if (ex.requestHeaders.getFirst("If-None-Match") == etag) return@createContext ex.reply(304)
                    val files = buildJsonObject { g.forEach { (n, c) -> put(n, buildJsonObject { put("content", c) }) } }
                    ex.reply(200, buildJsonObject { put("files", files) }.toString(), etag)
                }
            }
        }
        server.start()
        try {
            val base = "http://127.0.0.1:${server.address.port}"
            val gh = ShareGitHub("client", api = base, web = base, pollUnitMillis = 1)
            runBlocking {
                val code = gh.startLogin()
                assertEquals("ABCD-1234", code.userCode)
                val token = gh.waitLogin(code)
                assertEquals("tok", token)
                assertEquals("slaynAW", gh.user(token))
                assertEquals(ShareException.Reason.UNAUTHORIZED, runCatching { gh.user("mauvais") }.exceptionOrNull().let { (it as ShareException).reason })
                val id = gh.createGist(token, "Wake On LAN", mapOf("LISEZMOI.md" to "chiffré"))
                gh.updateGist(token, id, mapOf("acces-1.json" to "{}"))
                val snap = gh.fetchGist(id, "")
                assertEquals(mapOf("acces-1.json" to "{}"), snap.files)
                assertNull(lastAuth, "lecture sans jeton")
                assertTrue(gh.fetchGist(id, snap.etag).notModified)
                gh.updateGist(token, id, mapOf("acces-1.json" to null))
                assertTrue(gh.fetchGist(id, snap.etag).files.isEmpty())
                gh.deleteGist(token, id)
                assertEquals(ShareException.Reason.NOT_FOUND, (runCatching { gh.fetchGist(id, "") }.exceptionOrNull() as ShareException).reason)
            }
        } finally {
            server.stop(0)
        }
    }

    @Test
    fun `cle de partage dans la sauvegarde complete`() {
        val key = ShareCrypto.newKeyPair()
        val guest = ShareCrypto.newKeyPair()
        val owner = ShareOwner(key = key.privateEncoded, publicKey = key.publicEncoded, name = "Hugo", token = "gho_secret", user = "slaynAW", gist = "0123456789abcdef0123456789abcdef")
            .grant(SharePerson("Léa", guest.publicEncoded, 1, mapOf("a" to ShareRight.WAKE)))
        val config = AppConfig(devices = listOf(device("a", "PC streaming", "192.168.1.20")))
        val extra = mapOf("sharing" to ConfigCodec.json.encodeToJsonElement(ExportedShareOwner.serializer(), ExportedShareOwner.of(owner)))
        val text = ExportCodec.export(config, "motdepasse".toCharArray(), "2026-09-28T16:00:00Z", extra = extra, iterations = ExportCodec.MIN_ITERATIONS)
        assertFalse(text.contains("Hugo") || text.contains("gho_secret"))
        val (back, found) = ExportCodec.importWithExtra(text, "motdepasse".toCharArray())
        assertEquals(config.devices, back.devices)
        val restored = ConfigCodec.json.decodeFromJsonElement(ExportedShareOwner.serializer(), found.getValue("sharing")).toOwner()
        assertEquals("", restored.token, "jeton jamais exporté")
        assertEquals(owner.people, restored.people)
        assertEquals(owner.publicKey, restored.publicKey)
        // Export lisible : jamais de clé de partage.
        assertFalse(ExportCodec.export(config, null, "2026-09-28T16:00:00Z", extra = extra).contains("sharing"))
    }

    /** Fichier produit par Android, relu par les tests Windows (WOL_WRITE_SHARE_VECTORS=1 pour le régénérer). */
    @Test
    fun `fichier Android pour Windows`() {
        val out = File(dir, "share-android.json")
        if (System.getenv("WOL_WRITE_SHARE_VECTORS") == "1") {
            val content = ShareContent(
                "Hugo", "Léa",
                listOf(
                    device("a", "PC streaming", "192.168.1.20").copy(broadcastAddress = "192.168.1.255", wolPort = 7, secureOnPassword = "01:23:45:67:89:AB"),
                    device("bb", "Bureau", "bureau.local", agent = true).copy(probePorts = emptyList()),
                ),
            )
            val file = ShareCrypto.seal(owner, device.publicEncoded, v.revision + 1, "2026-09-28T16:00:00Z", content)
            val text = prettyJson.encodeToString(AndroidFile.serializer(), AndroidFile(file, v.revision + 1))
            out.writeText(text + "\n")
        }
        val android = json.decodeFromString(AndroidFile.serializer(), out.readText())
        assertEquals(android.revision, ShareCrypto.open(android.file, v.ownerPublic, device, 0).revision)
    }

    @Serializable
    data class AndroidFile(val file: String, val revision: Long)

    private fun device(id: String, name: String, host: String, agent: Boolean = false) = Device(
        id = id, name = name, mac = MacAddress.parse("AA:BB:CC:DD:EE:0${id.length}"), host = host,
        agent = if (agent) AgentSettings(key = "kY5jjMI6cQpU1gqHSiok1wAPlr6OUVgey66jYkJw5DI") else null,
    )
}
