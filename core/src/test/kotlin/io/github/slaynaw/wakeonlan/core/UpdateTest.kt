package io.github.slaynaw.wakeonlan.core

import com.sun.net.httpserver.HttpServer
import io.github.slaynaw.wakeonlan.core.update.UpdateClient
import io.github.slaynaw.wakeonlan.core.update.UpdateException
import io.github.slaynaw.wakeonlan.core.update.UpdateFile
import io.github.slaynaw.wakeonlan.core.update.Updates
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.io.File
import java.net.InetAddress
import java.net.InetSocketAddress
import java.security.MessageDigest

private val json = Json { ignoreUnknownKeys = true }

/** Mises à jour : mêmes vecteurs que l'application Windows (protocol/update-vectors.json). */
class UpdateTest {
    @Serializable
    private data class Expected(
        val version: String,
        val code: Long,
        val date: String,
        val platforms: List<String>,
        val android: UpdateFile,
        val url: String,
    )

    @Serializable
    private data class Invalid(val reason: String, val manifest: String)

    @Serializable
    private data class Vectors(
        val publicKey: String,
        val manifest: String,
        val signature: String,
        val tamperedManifest: String,
        val expected: Expected,
        val invalid: List<Invalid>,
    )

    private val v: Vectors = json.decodeFromString(
        File(System.getProperty("wol.protocolDir") ?: "../protocol", "update-vectors.json").readText(),
    )
    private val key = Updates.publicKey(v.publicKey)

    @TempDir
    lateinit var dir: File

    @Test
    fun `vecteurs partages`() {
        assertTrue(Updates.verify(key, v.manifest.toByteArray(), v.signature))
        assertFalse(Updates.verify(key, v.tamperedManifest.toByteArray(), v.signature), "manifeste modifié")
        assertFalse(Updates.verify(key, v.manifest.toByteArray(), "pas du base64"), "signature illisible")

        val m = Updates.parse(v.manifest.toByteArray())
        assertEquals(v.expected.version, m.version)
        assertEquals(v.expected.code, m.code)
        assertEquals(v.expected.date, m.date)
        assertEquals(v.expected.platforms, m.files.map { it.platform })
        assertEquals(v.expected.android, m.file("android"))
        assertTrue("Latence en direct" in m.notes)
        assertEquals(v.expected.url, Updates.fileUrl(Updates.DEFAULT_BASE, m, m.file("android")!!))

        for (case in v.invalid) {
            val e = assertThrows(UpdateException::class.java, { Updates.parse(case.manifest.toByteArray()) }, case.reason)
            assertEquals(UpdateException.Reason.INVALID, e.reason, case.reason)
        }
    }

    /** Serveur local qui imite GitHub Releases. */
    private fun serve(manifest: String, payload: ByteArray, block: (String) -> Unit) {
        val server = HttpServer.create(InetSocketAddress(InetAddress.getLoopbackAddress(), 0), 0)
        fun route(path: String, body: ByteArray) = server.createContext(path) { exchange ->
            if (exchange.requestURI.path != path) {
                exchange.sendResponseHeaders(404, -1)
            } else {
                exchange.sendResponseHeaders(200, body.size.toLong())
                exchange.responseBody.use { it.write(body) }
            }
            exchange.close()
        }
        route("/releases/latest/download/update.json", manifest.toByteArray())
        route("/releases/latest/download/update.json.sig", (v.signature + "\n").toByteArray())
        route("/releases/download/v1.3.0/WakeOnLan-1.3.0.apk", payload)
        server.start()
        try {
            block("http://127.0.0.1:${server.address.port}/releases")
        } finally {
            server.stop(0)
        }
    }

    @Test
    fun `recherche et telechargement verifies`() = serve(v.manifest, "contenu".toByteArray()) { base ->
        runBlocking {
            val client = UpdateClient(key, base)
            val m = client.latest()
            assertEquals("1.3.0", m.version)

            val sha = MessageDigest.getInstance("SHA-256").digest("contenu".toByteArray()).joinToString("") { "%02x".format(it) }
            val file = m.file("android")!!.copy(size = 7, sha256 = sha)
            var received = 0L
            val dest = File(dir, "update.apk")
            client.download(m, file, dest) { done, _ -> received = done }
            assertEquals("contenu", dest.readText())
            assertEquals(7, received)

            val corrupt = File(dir, "autre.apk")
            val e = assertThrows(UpdateException::class.java) {
                runBlocking { client.download(m, file.copy(sha256 = "0".repeat(64)), corrupt) }
            }
            assertEquals(UpdateException.Reason.CORRUPT, e.reason)
            assertFalse(corrupt.exists(), "fichier corrompu écrit")
            assertEquals(listOf("update.apk"), dir.list()!!.toList(), "fichiers temporaires restants")
        }
    }

    @Test
    fun `manifeste modifie ou absent`() {
        serve(v.tamperedManifest, ByteArray(0)) { base ->
            val e = assertThrows(UpdateException::class.java) { runBlocking { UpdateClient(key, base).latest() } }
            assertEquals(UpdateException.Reason.SIGNATURE, e.reason)
        }
        serve(v.manifest, ByteArray(0)) { base ->
            val e = assertThrows(UpdateException::class.java) { runBlocking { UpdateClient(key, "$base/absent").latest() } }
            assertEquals(UpdateException.Reason.NOT_PUBLISHED, e.reason)
        }
    }
}
