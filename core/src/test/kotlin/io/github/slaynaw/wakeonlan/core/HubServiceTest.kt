package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.hub.HubDeviceField
import io.github.slaynaw.wakeonlan.core.hub.HubLinkResult
import io.github.slaynaw.wakeonlan.core.hub.HubProbe
import io.github.slaynaw.wakeonlan.core.hub.HubService
import io.github.slaynaw.wakeonlan.core.hub.PairedHub
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Règles du service du hub qui ne demandent pas de hub (docs/HUB.md, §5 et §12). */
class HubServiceTest {
    private val pin = TestIdentities.newDevice().pin

    @Test
    fun `liens colles dans Appairer un hub`() {
        val hub = "patronus://hub?v=1&n=Hub+NAS&h=192.168.1.20&a=$pin&c=7KQ29MXP&r=admin&e=2000"
        assertTrue(HubLinkResult.read(hub, 1999) is HubLinkResult.Valid)
        assertEquals(HubLinkResult.Expired, HubLinkResult.read(hub, 2000))
        assertEquals(HubLinkResult.AgentLink, HubLinkResult.read("wolagent://pair?v=2&h=192.168.1.20&a=$pin&c=7KQ29MXP", 0))
        assertEquals(HubLinkResult.Invalid, HubLinkResult.read("https://exemple.fr", 0))
        assertEquals(HubLinkResult.Invalid, HubLinkResult.read("patronus://hub?v=1&h=192.168.1.20&a=$pin&c=7KQ29MXP&r=control", 0))
    }

    @Test
    fun `hub appaire`() {
        val hub = PairedHub(name = "Hub NAS", host = "192.168.1.20", port = 9771, pin = pin.toString(), role = "view", paired = 100)
        assertEquals(100, hub.eventsSince)
        assertEquals(501, hub.copy(eventsSeen = 500).eventsSince)
        assertEquals(Pin.parse(pin.toString())!!.short(), hub.shortPin)
        assertFalse(hub.canWake || hub.isAdmin)
        assertTrue(hub.copy(role = "wake").canWake)
        assertTrue(hub.copy(role = "admin").let { it.canWake && it.isAdmin })
    }

    @Test
    fun `appareil sans agent verifie avant le hub`() {
        val http = HubProbe("http")
        assertNull(HubService.validateDevice("Box", "192.168.1.1", "", http))
        assertNull(HubService.validateDevice("Imprimante", "imprimante.local", "aa-bb-cc-dd-ee-ff", HubProbe("tcp", 9100)))
        assertNull(HubService.validateDevice("Caméra", "fd00::5", "", HubProbe("https", 8443)))
        assertEquals(HubDeviceField.NAME, HubService.validateDevice("  ", "192.168.1.1", "", http))
        assertEquals(HubDeviceField.NAME, HubService.validateDevice("x".repeat(65), "192.168.1.1", "", http))
        assertEquals(HubDeviceField.HOST, HubService.validateDevice("Box", "pas une adresse", "", http))
        assertEquals(HubDeviceField.MAC, HubService.validateDevice("Box", "192.168.1.1", "zz", http))
        assertEquals(HubDeviceField.MAC, HubService.validateDevice("Box", "192.168.1.1", "ff:ff:ff:ff:ff:ff", http))
        assertEquals(HubDeviceField.PROBE, HubService.validateDevice("Box", "192.168.1.1", "", HubProbe("ping")))
        assertEquals(HubDeviceField.PORT, HubService.validateDevice("Box", "192.168.1.1", "", HubProbe("tcp")))
        assertEquals(HubDeviceField.PORT, HubService.validateDevice("Box", "192.168.1.1", "", HubProbe("http", 70000)))
        assertEquals("AA:BB:CC:DD:EE:01", HubService.wakeMac(MacAddress.parse("aa:bb:cc:dd:ee:01")))
    }
}
