package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.diagnostics.DiagnosticCodec
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test
import java.io.File

private val vectorJson = Json { ignoreUnknownKeys = true }

class DiagnosticTest {
    @Serializable
    private data class Vector(val password: String, val report: String, val file: String)

    private val vector: Vector by lazy {
        val dir = System.getProperty("wol.protocolDir") ?: "../protocol"
        vectorJson.decodeFromString(Vector.serializer(), File(dir, "diagnostic-vectors.json").readText())
    }

    @Test
    fun `rapport de reference produit independamment`() {
        val (env, text) = DiagnosticCodec.open(vector.file, vector.password.toCharArray())
        assertEquals(vector.report, text)
        assertEquals("Patronus Android 9.9.9", env.app)
        assertThrows(IllegalArgumentException::class.java) { DiagnosticCodec.open(vector.file, (vector.password + "!").toCharArray()) }
    }

    @Test
    fun `aller-retour et refus`() {
        val report = "ligne 1\nétat : ✓\n"
        val sealed = DiagnosticCodec.seal(report, "motdepasse".toCharArray(), "Patronus Android 1.5.4", "2026-09-29T17:00:00Z", iterations = DiagnosticCodec.MIN_ITERATIONS)
        assertFalse(sealed.contains("ligne 1"))
        val (env, text) = DiagnosticCodec.open(sealed, "motdepasse".toCharArray())
        assertEquals(report, text)
        assertEquals("2026-09-29T17:00:00Z", env.createdAt)
        assertThrows(IllegalArgumentException::class.java) { DiagnosticCodec.seal(report, "court".toCharArray(), "x", "y") }
        assertThrows(IllegalArgumentException::class.java) { DiagnosticCodec.open("""{"format":"wakeonlan-config"}""", "motdepasse".toCharArray()) }
        val weak = sealed.replace("\"iterations\": 100000", "\"iterations\": 1000")
        assertThrows(IllegalArgumentException::class.java) { DiagnosticCodec.open(weak, "motdepasse".toCharArray()) }
    }
}
