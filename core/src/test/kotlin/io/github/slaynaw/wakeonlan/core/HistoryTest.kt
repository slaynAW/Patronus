package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentHistory
import io.github.slaynaw.wakeonlan.core.agent.AgentHistoryEvent
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.config.ExportCodec
import io.github.slaynaw.wakeonlan.core.history.HistoryData
import io.github.slaynaw.wakeonlan.core.history.HistoryEvent
import io.github.slaynaw.wakeonlan.core.history.HistoryKind
import io.github.slaynaw.wakeonlan.core.history.HistoryRecorder
import io.github.slaynaw.wakeonlan.core.history.HistorySource
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.core.status.StatusNotice
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.encodeToJsonElement
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

class HistoryTest {
    @Serializable
    private data class AgentPart(val device: String, val fetchedAt: Long, val history: AgentHistory)

    @Serializable
    private data class Merged(val pc3: List<JsonArray>, val all: Int)

    @Serializable
    private data class Vectors(
        val now: Long,
        val events: List<HistoryEvent>,
        val agent: AgentPart,
        val expected: Map<String, List<JsonArray>>,
        val clients: Map<String, String>,
        val unreported: Map<String, List<Long>>,
        val import: HistoryData,
        val merged: Merged,
    )

    private val json = Json { ignoreUnknownKeys = true }

    private fun vectors(): Vectors {
        val dir = System.getProperty("wol.protocolDir") ?: "../protocol"
        return json.decodeFromString(File(dir, "history-vectors.json").readText())
    }

    /** [type, source, heure] : même forme que « expected » dans le fichier partagé. */
    private fun summary(events: List<HistoryEvent>) = events.map {
        JsonArray(listOf(json.encodeToJsonElement(it.kind), json.encodeToJsonElement(it.source), JsonPrimitive(it.time)))
    }

    @Test
    fun `scenario partage avec l'application Windows`() {
        val v = vectors()
        var data = HistoryData.EMPTY
        v.events.forEach { data = data.add(it, v.now) }
        data = data.replaceAgent(v.agent.device, v.agent.history, v.agent.fetchedAt, v.now)

        assertEquals(v.expected.getValue("pc1"), summary(data.view("pc1", v.now)))
        assertEquals(v.expected.getValue("all"), summary(data.view(null, v.now)))
        // Les demandes des autres appareils gardent leur origine (nom, à défaut adresse).
        for (e in data.view("pc1", v.now).filter { it.source == HistorySource.AGENT }) {
            val want = v.clients[json.encodeToJsonElement(e.kind).toString().trim('"')] ?: continue
            assertEquals(want, e.client, "appareil de ${e.kind}")
        }
        // Démarrages demandés ici, inconnus du journal de l'agent : à lui signaler.
        for ((device, want) in v.unreported) {
            val journal = if (device == v.agent.device) v.agent.history else AgentHistory()
            assertEquals(want, data.unreportedWakes(device, journal), "démarrages à signaler ($device)")
        }
        // Sauvegarde importée : fusionnée sans doublon, deux fois de suite sans effet.
        val merged = data.merge(v.import, v.now)
        assertEquals(v.merged.pc3, summary(merged.view("pc3", v.now)))
        assertEquals(v.merged.all, merged.view(null, v.now).size)
        assertEquals(v.merged.all, merged.merge(v.import, v.now).view(null, v.now).size)
        assertEquals(v.expected.getValue("all").size, data.view(null, v.now).size)
        // Nouvelle lecture du journal : l'ancienne version est remplacée, pas dupliquée.
        val again = data.replaceAgent(v.agent.device, v.agent.history, v.agent.fetchedAt, v.now)
        assertEquals(v.expected.getValue("all").size, again.view(null, v.now).size)
        // PC supprimé : son historique disparaît.
        val kept = data.keep(setOf("pc2"))
        assertEquals(1, kept.view(null, v.now).size)
        assertTrue(kept.coverage.isEmpty())
    }

    @Test
    fun `historique dans la sauvegarde complete`() {
        val now = 1_790_600_000_000
        val events = (0 until HistoryData.MAX_BACKUP_EVENTS + 10).map {
            HistoryEvent("a", now - it * 1000L, HistoryKind.WAKE_SENT, HistorySource.APP)
        }
        // Les plus récents seulement : le fichier reste sous la taille acceptée à l'import.
        val backup = HistoryData(version = HistoryData.VERSION, events = events).forBackup(now)
        assertEquals(HistoryData.MAX_BACKUP_EVENTS, backup.events.size)
        assertEquals(now, backup.events.maxOf { it.time })

        val extra = mapOf("history" to json.parseToJsonElement(HistoryData.encode(backup)))
        val text = ExportCodec.export(AppConfig(), "motdepasse".toCharArray(), "2026-09-28T16:00:00Z", extra = extra, iterations = ExportCodec.MIN_ITERATIONS)
        assertTrue(text.length < 1024 * 1024, "sauvegarde de ${text.length} octets")
        val restored = HistoryData.decode(ExportCodec.importWithExtra(text, "motdepasse".toCharArray()).second.getValue("history").toString())
        assertEquals(backup.events, restored?.events)
        // Export lisible : jamais d'historique.
        assertFalse(ExportCodec.export(AppConfig(), null, "2026-09-28T16:00:00Z", extra = extra).contains("history"))
    }

    @Test
    fun `a heure egale le dernier enregistre passe devant`() {
        val now = 1_790_600_000_000
        val data = HistoryData.EMPTY
            .add(HistoryEvent("a", now, HistoryKind.SHUTDOWN_SENT, HistorySource.APP), now)
            .add(HistoryEvent("a", now, HistoryKind.OFF, HistorySource.APP, approx = true), now)
        assertEquals(listOf(HistoryKind.OFF, HistoryKind.SHUTDOWN_SENT), data.view("a", now).map { it.kind })
    }

    @Test
    fun `conservation de 30 jours et capacite bornee`() {
        val now = 1_790_600_000_000
        val old = HistoryData.EMPTY.add(HistoryEvent("a", now - HistoryData.RETENTION_MS - 1, HistoryKind.ON, HistorySource.APP), now)
        assertTrue(old.events.isEmpty())

        val full = HistoryData(
            version = HistoryData.VERSION,
            events = List(HistoryData.MAX_EVENTS + 5) { HistoryEvent("a", now - it - 1, HistoryKind.ON, HistorySource.APP) },
        ).add(HistoryEvent("a", now, HistoryKind.OFF, HistorySource.APP), now)
        assertEquals(HistoryData.MAX_EVENTS, full.events.size)
        assertEquals(HistoryKind.OFF, full.view("a", now).first().kind)
    }

    @Test
    fun `evenements inconnus du journal ignores`() {
        assertNull(HistoryData.fromAgent("a", AgentHistoryEvent(1, "cmd", "unknown")))
        assertNull(HistoryData.fromAgent("a", AgentHistoryEvent(1, "future")))
        assertEquals(null, HistoryData.fromAgent("a", AgentHistoryEvent(1, "boot", c = "10.0.0.1"))?.client)
    }

    @Test
    fun `transitions constatees par la surveillance`() {
        val now = 1_790_600_000_000
        val previous = mapOf(
            "booted" to DeviceStatus(PowerState.OFFLINE, since = now - 3_600_000),
            "resumed" to DeviceStatus(PowerState.OFFLINE, since = now - 600_000),
            "noagent" to DeviceStatus(PowerState.WAKING, since = now - 60_000),
            "off" to DeviceStatus(PowerState.ONLINE, lastSeen = now - 4_000),
            "timeout" to DeviceStatus(PowerState.WAKING),
            "unknown" to DeviceStatus(PowerState.UNKNOWN),
            "same" to DeviceStatus(PowerState.ONLINE),
            "shutdown" to DeviceStatus(PowerState.SHUTTING_DOWN, lastSeen = now - 9_000),
            "request" to DeviceStatus(PowerState.SHUTTING_DOWN, since = now - 2_000, lastSeen = now - 5_000),
        )
        val current = mapOf(
            "booted" to DeviceStatus(PowerState.ONLINE, agent = AgentStatus(uptimeSeconds = 120)),
            "resumed" to DeviceStatus(PowerState.ONLINE, agent = AgentStatus(uptimeSeconds = 86_400)),
            "noagent" to DeviceStatus(PowerState.ONLINE),
            "off" to DeviceStatus(PowerState.OFFLINE),
            "timeout" to DeviceStatus(PowerState.OFFLINE, notice = StatusNotice.WAKE_TIMEOUT),
            "unknown" to DeviceStatus(PowerState.ONLINE),
            "same" to DeviceStatus(PowerState.ONLINE),
            "shutdown" to DeviceStatus(PowerState.OFFLINE),
            "request" to DeviceStatus(PowerState.OFFLINE),
            "new" to DeviceStatus(PowerState.ONLINE),
        )
        val app = HistorySource.APP
        val expected = mapOf(
            "booted" to HistoryEvent("booted", now - 120_000, HistoryKind.ON, app),
            "resumed" to HistoryEvent("resumed", now, HistoryKind.ON, app, approx = true),
            "noagent" to HistoryEvent("noagent", now, HistoryKind.ON, app, approx = true),
            "off" to HistoryEvent("off", now - 4_000, HistoryKind.OFF, app, approx = true),
            "timeout" to HistoryEvent("timeout", now, HistoryKind.WAKE_TIMEOUT, app),
            "shutdown" to HistoryEvent("shutdown", now - 9_000, HistoryKind.OFF, app, approx = true),
            "request" to HistoryEvent("request", now - 2_000, HistoryKind.OFF, app, approx = true),
        )
        assertEquals(expected, HistoryRecorder.transitions(previous, current, now).associateBy { it.device })
    }

    @Test
    fun `enregistrement et relecture`() {
        val now = 1_790_600_000_000
        val data = HistoryData.EMPTY
            .add(HistoryEvent("a", now, HistoryKind.WAKE_SENT, HistorySource.APP), now)
            .replaceAgent("a", AgentHistory(now / 1000, listOf(AgentHistoryEvent(now / 1000, "cmd", "sleep", "10.0.0.7"))), now, now)
        val text = HistoryData.encode(data)
        assertEquals(data, HistoryData.decode(text))
        assertFalse(text.contains("\"x\""), "heure exacte : champ omis")
        assertNull(HistoryData.decode("abîmé"))
    }
}
