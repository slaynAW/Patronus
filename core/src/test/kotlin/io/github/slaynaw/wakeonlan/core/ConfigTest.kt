package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentKey
import io.github.slaynaw.wakeonlan.core.agent.PairingInfo
import io.github.slaynaw.wakeonlan.core.agent.PairingLink
import io.github.slaynaw.wakeonlan.core.config.ConfigCodec
import io.github.slaynaw.wakeonlan.core.config.ConfigException
import io.github.slaynaw.wakeonlan.core.config.ExportCodec
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.AppSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.DeviceField
import io.github.slaynaw.wakeonlan.core.model.DeviceValidator
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class ConfigTest {
    private val key = AgentKey.generate()
    private val config = AppConfig(
        devices = listOf(
            Device(
                id = "pc1",
                name = "PC Bureau",
                mac = MacAddress.parse("AA:BB:CC:DD:EE:01"),
                host = "192.168.1.20",
                agent = AgentSettings(key = key),
            ),
            Device(id = "nas", name = "NAS", mac = MacAddress.parse("AA:BB:CC:DD:EE:02"), secureOnPassword = "11:22:33:44:55:66"),
        ),
        settings = AppSettings(pollIntervalSeconds = 5),
    )

    @Test
    fun `aller-retour JSON`() {
        assertEquals(config, ConfigCodec.decode(ConfigCodec.encode(config)))
    }

    @Test
    fun `les secrets n apparaissent jamais dans toString`() {
        assertFalse(config.toString().contains(key))
        assertFalse(config.toString().contains("11:22:33:44:55:66"))
    }

    @Test
    fun `configuration invalide refusee`() {
        val bad = ConfigCodec.encode(config).replace("192.168.1.20", "pas une ip !")
        val e = assertThrows<ConfigException> { ConfigCodec.decode(bad) }
        assertEquals(ConfigException.Reason.INVALID_DATA, e.reason)

        val duplicated = config.copy(devices = config.devices + config.devices.first())
        assertThrows<ConfigException> { ConfigCodec.sanitize(duplicated) }
    }

    @Test
    fun `version future refusee et reglages bornes`() {
        val future = """{"schemaVersion":99,"devices":[]}"""
        assertEquals(ConfigException.Reason.NEWER_VERSION, assertThrows<ConfigException> { ConfigCodec.decode(future) }.reason)
        val extreme = """{"schemaVersion":1,"devices":[],"settings":{"pollIntervalSeconds":0,"wakeTimeoutSeconds":999999}}"""
        val decoded = ConfigCodec.decode(extreme)
        assertEquals(1, decoded.settings.pollIntervalSeconds)
        assertEquals(900, decoded.settings.wakeTimeoutSeconds)
    }

    @Test
    fun `export en clair sans secrets`() {
        val text = ExportCodec.export(config, null, "2026-09-27T10:00:00Z")
        assertFalse(text.contains(key))
        assertFalse(text.contains("11:22:33:44:55:66"))
        assertFalse(ExportCodec.isEncrypted(text))
        val imported = ExportCodec.import(text, null)
        assertEquals("", imported.devices[0].agent?.key)
        assertNull(imported.devices[1].secureOnPassword)
        assertEquals(config.devices.map { it.mac }, imported.devices.map { it.mac })
    }

    @Test
    fun `export chiffre avec mot de passe`() {
        val password = "correct horse battery".toCharArray()
        val text = ExportCodec.export(config, password, "2026-09-27T10:00:00Z", iterations = ExportCodec.MIN_ITERATIONS)
        assertFalse(text.contains(key))
        assertFalse(text.contains("PC Bureau"))
        assertTrue(ExportCodec.isEncrypted(text))
        assertEquals(config, ExportCodec.import(text, password))

        val wrong = assertThrows<ConfigException> { ExportCodec.import(text, "mauvais mot de passe".toCharArray()) }
        assertEquals(ConfigException.Reason.WRONG_PASSWORD, wrong.reason)
        val missing = assertThrows<ConfigException> { ExportCodec.import(text, null) }
        assertEquals(ConfigException.Reason.PASSWORD_REQUIRED, missing.reason)

        // Toute altération du contenu chiffré est détectée.
        val data = Regex("\"data\": \"([^\"]+)\"").find(text)!!.groupValues[1]
        val tampered = text.replace(data, data.replaceRange(10, 11, if (data[10] == 'A') "B" else "A"))
        assertThrows<ConfigException> { ExportCodec.import(tampered, password) }
    }

    @Test
    fun `fichier etranger refuse`() {
        val e = assertThrows<ConfigException> { ExportCodec.import("""{"hello":"world"}""", null) }
        assertEquals(ConfigException.Reason.NOT_A_BACKUP, e.reason)
    }

    @Test
    fun `fusion et reordonnancement`() {
        val other = AppConfig(devices = listOf(config.devices[0].copy(name = "Renommé"), config.devices[1].copy(id = "new")))
        val merged = config.mergeDevicesFrom(other)
        assertEquals(listOf("pc1", "nas", "new"), merged.devices.map { it.id })
        assertEquals("Renommé", merged.devices[0].name)
        assertEquals(listOf("nas", "pc1"), config.move("pc1", 1).devices.map { it.id })
        assertEquals(config, config.move("pc1", -1))
    }

    @Test
    fun `lien d appairage`() {
        val info = PairingInfo("PC de Salon & Jeux", "192.168.1.42", 9770, MacAddress.parse("aa:bb:cc:dd:ee:ff"), key)
        val link = PairingLink.build(info)
        assertTrue(link.startsWith("wolagent://pair?v=1&"))
        assertEquals(info, PairingLink.parse(link))
        assertThrows<IllegalArgumentException> { PairingLink.parse("https://example.com") }
        assertThrows<IllegalArgumentException> { PairingLink.parse(link.replace(key, "abc")) }
        assertThrows<IllegalArgumentException> { PairingLink.parse(link.replace("v=1", "v=2")) }
        // Lien produit par l'agent Go (url.QueryEscape encode les espaces en « + »).
        val fromGo = "wolagent://pair?v=1&n=PC+Bureau&h=192.168.1.20&p=9770&m=AA%3ABB%3ACC%3ADD%3AEE%3AFF&k=$key"
        assertEquals("PC Bureau", PairingLink.parse(fromGo).name)
    }

    @Test
    fun `validation des champs`() {
        val ok = config.devices[0]
        assertTrue(DeviceValidator.validate(ok).isEmpty())
        fun fields(d: Device) = DeviceValidator.validate(d).map { it.field }.toSet()
        assertEquals(setOf(DeviceField.NAME), fields(ok.copy(name = " ")))
        assertEquals(setOf(DeviceField.HOST), fields(ok.copy(host = "256.1.1.1")))
        assertEquals(setOf(DeviceField.HOST), fields(ok.copy(host = "")))
        assertEquals(setOf(DeviceField.WOL_PORT), fields(ok.copy(wolPort = 70000)))
        assertEquals(setOf(DeviceField.AGENT_KEY), fields(ok.copy(agent = AgentSettings(key = "xyz"))))
        assertEquals(setOf(DeviceField.BROADCAST), fields(ok.copy(broadcastAddress = "192.168.1")))
        listOf("pc-bureau", "pc-bureau.local", "10.0.0.1", "fe80::1%wlan0", "2001:db8::1").forEach {
            assertTrue(DeviceValidator.isValidHost(it), it)
        }
        listOf("pc bureau", "-pc", "1.2.3", "http://pc", "a..b").forEach {
            assertFalse(DeviceValidator.isValidHost(it), it)
        }
    }
}
