package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentDisks
import io.github.slaynaw.wakeonlan.core.agent.AgentDrive
import io.github.slaynaw.wakeonlan.core.agent.AgentHistoryEvent
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.AgentVolume
import io.github.slaynaw.wakeonlan.core.agent.DiskAlert
import io.github.slaynaw.wakeonlan.core.agent.DiskLevel
import io.github.slaynaw.wakeonlan.core.archive.ArchiveJournal
import io.github.slaynaw.wakeonlan.core.history.HistoryData
import io.github.slaynaw.wakeonlan.core.history.HistoryKind
import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Disques et causes des arrêts anormaux (agent 1.9.0) : mêmes règles que l'application Windows. */
class DisksTest {
    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }

    @Test
    fun `lecture de la reponse de l agent`() {
        val status = json.decodeFromString(
            AgentStatus.serializer(),
            """{"hostname":"PC","disks":{"volumes":[{"mount":"C:","label":"Windows","fs":"NTFS","total":1000,"free":40}],
               "drives":[{"name":"Samsung SSD 980","media":"ssd","bus":"NVMe","size":1000,"health":"ok","temp":41,"wear":3,"hours":1234,"readErrors":0}],
               "errors":2,"lastError":1791000000,"futur":true}}""",
        )
        val disks = status.disks!!
        assertEquals(96.0, disks.volumes[0].usedPercent, 0.001)
        assertEquals(DiskLevel.BAD, disks.volumes[0].level)
        assertEquals(DiskLevel.NONE, disks.drives[0].level)
        assertEquals(3, disks.drives[0].wear)
        assertEquals(2, disks.errors)
        assertNull(json.decodeFromString(AgentStatus.serializer(), """{"hostname":"PC"}""").disks)
    }

    @Test
    fun `gravite et alerte`() {
        val ok = AgentDrive("SSD", media = AgentDrive.SSD, health = AgentDrive.HEALTHY, temp = 41.0, wear = 3)
        assertEquals(DiskLevel.NONE, ok.level)
        assertEquals(DiskLevel.WARN, ok.copy(wear = 85).level)
        assertEquals(DiskLevel.BAD, ok.copy(wear = 97).level)
        assertEquals(DiskLevel.WARN, ok.copy(temp = 72.0).level)
        assertEquals(DiskLevel.WARN, AgentDrive("HDD", media = AgentDrive.HDD, temp = 56.0).level)
        assertEquals(DiskLevel.WARN, ok.copy(readErrors = 3).level)
        assertEquals(DiskLevel.BAD, ok.copy(health = AgentDrive.UNHEALTHY).level)
        assertEquals(DiskLevel.WARN, AgentVolume("D:", total = 100, free = 9).level)
        assertEquals(DiskLevel.NONE, AgentVolume("D:", total = 100, free = 11).level)

        assertNull(AgentDisks(volumes = listOf(AgentVolume("C:", total = 100, free = 50)), drives = listOf(ok)).alert())
        val full = AgentDisks(volumes = listOf(AgentVolume("C:", total = 100, free = 8), AgentVolume("D:", total = 100, free = 3)), drives = listOf(ok))
        assertEquals(DiskAlert(DiskAlert.Kind.FULL, DiskLevel.BAD, full.volumes[1]), full.alert())
        assertEquals(DiskAlert.Kind.DRIVE_BAD, full.copy(drives = listOf(ok.copy(health = AgentDrive.UNHEALTHY))).alert()?.kind)
        assertEquals(DiskAlert.Kind.DRIVE_WATCH, AgentDisks(drives = listOf(ok.copy(health = AgentDrive.WARNING))).alert()?.kind)
        assertEquals(DiskAlert.Kind.ERRORS, AgentDisks(errors = 4).alert()?.kind)
        assertTrue(AgentDisks().isEmpty)
    }

    @Test
    fun `cause des arrets anormaux`() {
        val lost = AgentHistoryEvent(t = 1790985600, k = "lost", r = "power")
        val precise = lost.copy(r = "bsod", d = "0x7E SYSTEM_THREAD_EXCEPTION_NOT_HANDLED")
        val events = mutableListOf<AgentHistoryEvent>()
        assertTrue(AgentHistoryEvent.merge(events, lost))
        assertTrue(AgentHistoryEvent.merge(events, precise))
        assertFalse(AgentHistoryEvent.merge(events, lost.copy(r = null)))
        assertEquals(listOf(precise), events)
        assertEquals(listOf(precise), AgentHistoryEvent.merged(listOf(lost, precise, lost)))

        // Archives : remplacé, jamais doublé.
        var journal = ArchiveJournal("2026-10")
        journal = journal.addEvents("AA", "PC", listOf(lost)).first
        val (next, changed) = journal.addEvents("AA", "PC", listOf(precise))
        assertTrue(changed)
        assertEquals(listOf(precise), next.events("AA"))

        val e = HistoryData.fromAgent("pc", precise)!!
        assertEquals(HistoryKind.LOST, e.kind)
        assertEquals("bsod", e.cause)
        assertEquals("0x7E SYSTEM_THREAD_EXCEPTION_NOT_HANDLED", e.detail)
        assertNull(HistoryData.fromAgent("pc", lost.copy(r = "inconnue"))!!.cause)
        assertNull(HistoryData.fromAgent("pc", precise.copy(d = "x".repeat(500)))!!.detail)
        assertNull(HistoryData.fromAgent("pc", AgentHistoryEvent(t = 1, k = "boot", r = "bsod"))!!.cause)
    }
}
