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
        // Identifiant anonyme : type de l'appareil et 8 caractères aléatoires, jamais son nom.
        val id = BackupNames.deviceId("android")
        assertTrue(Regex("^android-[0-9a-f]{8}$").matches(id) && BackupNames.isAnonymous(id), id)
        assertTrue(id != BackupNames.deviceId("android"))
        for (legacy in listOf("android-pixel-8-3fa2", "windows-bureau-3fa2", "android-3fa2", "windows-a1b2c3d4-3fa2")) {
            assertFalse(BackupNames.isAnonymous(legacy), legacy)
        }
        assertEquals(
            "patronus-android-0a1b2c3d-2026-09-30.json",
            BackupNames.renamed("patronus-android-pixel-8-3fa2-2026-09-30.json", "android-pixel-8-3fa2", "android-0a1b2c3d"),
        )
        assertNull(BackupNames.renamed("patronus-windows-bureau-3fa2-2026-09-30.json", "android-pixel-8-3fa2", "android-0a1b2c3d"))

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
