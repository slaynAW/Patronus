package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentHistoryEvent
import io.github.slaynaw.wakeonlan.core.agent.MetricsRow
import io.github.slaynaw.wakeonlan.core.archive.ArchiveCodec
import io.github.slaynaw.wakeonlan.core.archive.ArchiveDay
import io.github.slaynaw.wakeonlan.core.archive.ArchiveDayPc
import io.github.slaynaw.wakeonlan.core.archive.ArchiveException
import io.github.slaynaw.wakeonlan.core.archive.ArchiveJournal
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import java.io.File

/** Archives chiffrées : mêmes vecteurs que l'application Windows (protocol/archive-vectors.json). */
class ArchiveTest {
    private val root: JsonObject by lazy {
        val dir = System.getProperty("wol.protocolDir") ?: "../protocol"
        Json.parseToJsonElement(File(dir, "archive-vectors.json").readText()).jsonObject
    }

    private fun text(key: String) = root.getValue(key).jsonPrimitive.content

    @Test
    fun `archives de reference lisibles (application Windows, autres outils)`() {
        val manifest = ArchiveCodec.parseManifest(root.getValue("manifest").toString())
        val wrong = assertThrows<ArchiveException> { ArchiveCodec.unlock(manifest, text("wrongPassword").toCharArray()) }
        assertTrue(wrong.wrongPassword)
        val key = ArchiveCodec.unlock(manifest, text("password").toCharArray())
        val files = root.getValue("files").jsonObject
        val day = key.open(ArchiveCodec.dayFile("2026-10-03"), files.getValue("mesures-2026-10-03.txt").jsonPrimitive.content, ArchiveDay.serializer())
        assertEquals(Json.decodeFromJsonElement(ArchiveDay.serializer(), root.getValue("day")), day)
        assertEquals(55.1, day.rows("AA:BB:CC:DD:EE:01").first().ctx)
        assertNull(day.rows("AA:BB:CC:DD:EE:01")[1].gt)
        val journal = key.open(ArchiveCodec.journalFile("2026-10"), files.getValue("journal-2026-10.txt").jsonPrimitive.content, ArchiveJournal.serializer())
        assertEquals(Json.decodeFromJsonElement(ArchiveJournal.serializer(), root.getValue("journal")), journal)
        assertEquals("Pixel 8", journal.events("AA:BB:CC:DD:EE:01")[1].b)
        // Fichier présenté sous un autre nom (un autre jour) : refusé.
        assertThrows<ArchiveException> {
            key.open(ArchiveCodec.dayFile("2026-10-04"), files.getValue("mesures-2026-10-03.txt").jsonPrimitive.content, ArchiveDay.serializer())
        }
    }

    @Test
    fun `rechiffrement des archives de reference (changement de mot de passe)`() {
        val files = root.getValue("files").jsonObject.mapValues { it.value.jsonPrimitive.content } +
            (ArchiveCodec.MANIFEST_FILE to root.getValue("manifest").toString()) + ("mesures-2026-10-04.txt" to "abîmé")
        val wrong = assertThrows<ArchiveException> {
            ArchiveCodec.reencrypt(files, text("wrongPassword").toCharArray(), "nouveau mot de passe".toCharArray())
        }
        assertTrue(wrong.wrongPassword)
        val (out, unreadable) = ArchiveCodec.reencrypt(files, text("password").toCharArray(), "nouveau mot de passe".toCharArray())
        assertEquals(listOf("mesures-2026-10-04.txt"), unreadable)
        assertEquals("abîmé", out["mesures-2026-10-04.txt"])
        val manifest = ArchiveCodec.parseManifest(out.getValue(ArchiveCodec.MANIFEST_FILE))
        assertEquals("2026-10", manifest.month)
        assertTrue(assertThrows<ArchiveException> { ArchiveCodec.unlock(manifest, text("password").toCharArray()) }.wrongPassword)
        val key = ArchiveCodec.unlock(manifest, "nouveau mot de passe".toCharArray())
        val day = key.open(ArchiveCodec.dayFile("2026-10-03"), out.getValue("mesures-2026-10-03.txt"), ArchiveDay.serializer())
        assertEquals(Json.decodeFromJsonElement(ArchiveDay.serializer(), root.getValue("day")), day)
        val journal = key.open(ArchiveCodec.journalFile("2026-10"), out.getValue("journal-2026-10.txt"), ArchiveJournal.serializer())
        assertEquals(Json.decodeFromJsonElement(ArchiveJournal.serializer(), root.getValue("journal")), journal)
    }

    @Test
    fun `aller-retour et manifeste`() {
        val (manifest, key) = ArchiveCodec.newManifest("2026-11", "mot de passe très sûr".toCharArray())
        val parsed = ArchiveCodec.parseManifest(ArchiveCodec.manifestJson(manifest))
        assertEquals(manifest, parsed)
        val again = ArchiveCodec.unlock(parsed, "mot de passe très sûr".toCharArray())
        val day = ArchiveDay("2026-11-02", listOf(ArchiveDayPc("AA:BB:CC:DD:EE:01", "PC", listOf(MetricsRow(t = 1793577600, n = 6, ct = 50.5)))))
        val text = key.seal(ArchiveCodec.dayFile(day.day), ArchiveDay.serializer(), day)
        assertEquals(day, again.open(ArchiveCodec.dayFile(day.day), text, ArchiveDay.serializer()))
        assertThrows<ArchiveException> { ArchiveCodec.parseManifest("""{"format":"autre","version":1,"month":"2026-10","kdf":"PBKDF2WithHmacSHA256","iterations":600000,"salt":"","check":""}""") }
        assertThrows<ArchiveException> { ArchiveCodec.parseManifest("""{"format":"patronus-archive","version":1,"month":"2026-10","kdf":"PBKDF2WithHmacSHA256","iterations":10,"salt":"","check":""}""") }
    }

    @Test
    fun `fusion des mesures et du journal`() {
        val start = ArchiveCodec.dayStart("2026-10-03")!!
        val rows = listOf(
            MetricsRow(t = start + 60, n = 6, ct = 50.0),
            MetricsRow(t = start, n = 6, ct = 49.0),
            MetricsRow(t = start - 60, n = 6, ct = 40.0),
            MetricsRow(t = start + 86_400, n = 6, ct = 40.0),
        )
        val (day, changed) = ArchiveDay("2026-10-03").addRows("AA:BB:CC:DD:EE:01", "PC Bureau", rows)
        assertTrue(changed)
        assertEquals(listOf(start, start + 60), day.rows("AA:BB:CC:DD:EE:01").map { it.t })
        assertFalse(day.addRows("AA:BB:CC:DD:EE:01", "PC Bureau", rows.take(2)).second)
        val (completed, again) = day.addRows("AA:BB:CC:DD:EE:01", "PC Bureau", listOf(MetricsRow(t = start + 60, n = 6, ct = 50.0, gt = 45.0)))
        assertTrue(again)
        assertEquals(45.0, completed.rows("AA:BB:CC:DD:EE:01")[1].gt)

        val events = listOf(AgentHistoryEvent(1791025260, "shutdown"), AgentHistoryEvent(1790982000, "boot"), AgentHistoryEvent(1790640000, "boot"))
        val (journal, added) = ArchiveJournal("2026-10").addEvents("AA:BB:CC:DD:EE:01", "PC Bureau", events)
        assertTrue(added)
        assertEquals(listOf("boot", "shutdown"), journal.events("AA:BB:CC:DD:EE:01").map { it.k })
        assertFalse(journal.addEvents("AA:BB:CC:DD:EE:01", "PC Bureau", events).second)
    }

    @Test
    fun noms() {
        assertEquals("2026-10", ArchiveCodec.monthOf(ArchiveCodec.description("2026-10")))
        assertNull(ArchiveCodec.monthOf("Patronus – sauvegardes chiffrées"))
        assertEquals("2026-10-03", ArchiveCodec.dayOf(ArchiveCodec.dayFile("2026-10-03")))
        assertNull(ArchiveCodec.dayOf(ArchiveCodec.journalFile("2026-10")))
        assertEquals("2026-10-03", ArchiveCodec.dayOfTime(1790985600))
    }
}
