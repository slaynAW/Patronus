package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.model.MacAddress
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.ValueSource

class MacAddressTest {
    @ParameterizedTest
    @ValueSource(strings = ["01:23:45:67:89:ab", "01-23-45-67-89-AB", "0123.4567.89ab", "0123456789AB", "  01:23:45:67:89:AB "])
    fun `accepte les formats courants`(input: String) {
        assertEquals("01:23:45:67:89:AB", MacAddress.parse(input).toString())
    }

    @ParameterizedTest
    @ValueSource(strings = ["", "01:23:45:67:89", "01:23:45:67:89:AB:CD", "01:23-45:67:89:AB", "GG:23:45:67:89:AB", "00:00:00:00:00:00", "FF:FF:FF:FF:FF:FF", "0123456789A"])
    fun `refuse les formats invalides`(input: String) {
        assertThrows<IllegalArgumentException> { MacAddress.parse(input) }
        assertNull(MacAddress.parseOrNull(input))
    }

    @Test
    fun `egalite et copie defensive`() {
        val a = MacAddress.parse("aa:bb:cc:dd:ee:ff")
        val b = MacAddress.parse("AABBCCDDEEFF")
        assertEquals(a, b)
        assertEquals(a.hashCode(), b.hashCode())
        a.toByteArray()[0] = 0
        assertEquals("AA:BB:CC:DD:EE:FF", a.toString())
    }
}
