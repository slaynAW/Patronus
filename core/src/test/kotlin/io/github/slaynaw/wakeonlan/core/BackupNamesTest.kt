package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.backup.BackupNames
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.time.LocalDate

/** Mêmes règles que l'application Windows (desktop/internal/backup). */
class BackupNamesTest {
    @Test
    fun `noms et versions`() {
        assertEquals("pc-bureau-d-helene-etage", BackupNames.slug("PC Bureau d'Hélène – Étage"))
        val id = BackupNames.deviceId("android", "Pixel 8 Pro")
        assertTrue(Regex("^android-pixel-8-pro-[0-9a-f]{4}$").matches(id), id)
        val long = BackupNames.deviceId("android", "Pixel ".repeat(20))
        assertTrue(long.length <= 35 && !long.contains("--"), long)

        val day = LocalDate.of(2026, 9, 30)
        val name = BackupNames.fileName("android-pixel-8-3fa2", day)
        assertEquals("patronus-android-pixel-8-3fa2-2026-09-30.json", name)
        val entry = BackupNames.parse(name)
        assertEquals("android-pixel-8-3fa2", entry?.device)
        assertEquals(day, entry?.date)
        for (bad in listOf("LISEZMOI.md", "patronus-x-2026-13-40.json", "patronus--2026-09-30.json", "acces-abc.json")) {
            assertNull(BackupNames.parse(bad), bad)
        }

        val names = (0 until 10).map { BackupNames.fileName("a-1111", day.minusDays(it.toLong())) } +
            BackupNames.fileName("b-2222", day.minusDays(30)) + "LISEZMOI.md"
        val old = BackupNames.outdated(names, "a-1111")
        assertEquals(3, old.size)
        assertEquals(BackupNames.fileName("a-1111", day.minusDays(7)), old[0])
        val sorted = BackupNames.sorted(names)
        assertEquals(11, sorted.size)
        assertEquals(day, sorted.first().date)
        assertEquals("b-2222", sorted.last().device)
        assertFalse(sorted.any { it.name == "LISEZMOI.md" })
    }
}
