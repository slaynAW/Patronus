package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentKey
import io.github.slaynaw.wakeonlan.core.agent.PairingInfo
import io.github.slaynaw.wakeonlan.core.agent.PairingLink
import io.github.slaynaw.wakeonlan.core.agent.ParsedPairing
import io.github.slaynaw.wakeonlan.core.agent2.PairingLinkV2
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.agent2.Role
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
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import java.io.File

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
    fun `empreinte de l agent (wolagent 2)`() {
        val pin = Pin.of("agent de test".toByteArray()).toString()
        val pinned = config.copy(devices = listOf(config.devices[0].copy(agent = AgentSettings(key = key, pin = pin))))
        val text = ConfigCodec.encode(pinned)
        assertTrue(text.contains(""""pin":"$pin""""), text)
        assertEquals(pinned, ConfigCodec.decode(text))
        val agent = pinned.devices[0].agent!!
        assertTrue(agent.ready && agent.hasPin)
        assertEquals(Pin.parse(pin)!!.short(), agent.shortPin)
        assertFalse(agent.toString().contains(key))
        assertTrue(agent.toString().contains("pin=${agent.shortPin}"))
        // Export sans secret : la clé part, l'empreinte (publique) reste.
        val plain = pinned.devices[0].withoutSecrets().agent!!
        assertTrue(!plain.hasKey && plain.pin == pin && plain.ready)
        // Sans empreinte, le champ n'est pas écrit : format inchangé pour les versions précédentes.
        assertFalse(ConfigCodec.encode(config).contains("\"pin\""))
        // Empreinte mal écrite : refusée comme une clé invalide.
        val bad = pinned.devices[0].copy(agent = AgentSettings(key = key, pin = pin.dropLast(1)))
        assertEquals(DeviceField.AGENT_KEY, DeviceValidator.validate(bad).first().field)
        assertFalse(AgentSettings(key = "").ready)
        // Fiche enregistrée : l'empreinte reste (clé vide ou inchangée), sauf si une nouvelle clé est saisie.
        assertEquals(pin, agent.pinKeptWith(""))
        assertEquals(pin, agent.pinKeptWith(key))
        assertEquals("", agent.pinKeptWith(AgentKey.generate()))
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
    fun `sauvegardes de reference lisibles (application Windows, autres outils)`() {
        // Produites indépendamment (Node.js / OpenSSL) et relues aussi par l'application Windows (Go) :
        // les sauvegardes sont interchangeables entre le téléphone et le PC.
        val dir = System.getProperty("wol.protocolDir") ?: "../protocol"
        val root = ConfigCodec.json.parseToJsonElement(File(dir, "export-vectors.json").readText()).jsonObject
        val password = root.getValue("password").jsonPrimitive.content
        val expected = ConfigCodec.fromJson(root.getValue("config").jsonObject)
        val imported = ExportCodec.import(root.getValue("encryptedExport").toString(), password.toCharArray())
        assertEquals(expected, imported)
        assertEquals(240, imported.settings.wakeTimeoutSeconds)
        assertEquals("192.168.1.255", imported.devices[1].broadcastAddress)
        val plain = ExportCodec.import(root.getValue("plainExport").toString(), null)
        assertEquals("", plain.devices[0].agent?.key)
        assertNull(plain.devices[1].secureOnPassword)
    }

    @Test
    fun `rechiffrement d une sauvegarde (changement de mot de passe)`() {
        val old = "ancien mot de passe".toCharArray()
        val text = ExportCodec.export(config, old, "2026-09-27T10:00:00Z", appVersion = "Patronus test")
        val wrong = assertThrows<ConfigException> { ExportCodec.reencrypt(text, "mauvais mot de passe".toCharArray(), "nouveau mot de passe".toCharArray()) }
        assertEquals(ConfigException.Reason.WRONG_PASSWORD, wrong.reason)
        assertThrows<IllegalArgumentException> { ExportCodec.reencrypt(text, old, "court".toCharArray()) }
        val next = ExportCodec.reencrypt(text, old, "nouveau mot de passe".toCharArray())
        assertEquals(config, ExportCodec.import(next, "nouveau mot de passe".toCharArray()))
        ExportCodec.checkPassword(next, "nouveau mot de passe".toCharArray())
        assertEquals(ConfigException.Reason.WRONG_PASSWORD, assertThrows<ConfigException> { ExportCodec.checkPassword(next, old) }.reason)
        val envelope = ExportCodec.inspect(next)
        assertEquals("2026-09-27T10:00:00Z", envelope.exportedAt)
        assertEquals("Patronus test", envelope.app)
        assertThrows<ConfigException> { ExportCodec.reencrypt(ExportCodec.export(config, null, "x"), old, "nouveau mot de passe".toCharArray()) }
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
    fun `lien d appairage v2 (wol-agent pair, agent 1_11)`() {
        val agent = TestIdentities.newDevice().pin
        val v2 = PairingLinkV2("PC Bureau", "192.168.1.20", 9771, "30:9C:23:01:02:03", agent, "7KQ29MXP", Role.ADMIN, 1_791_880_773)
        val parsed = PairingLink.parseAny(" " + v2.text() + "\n")
        assertEquals(ParsedPairing.V2(v2), parsed)
        assertFalse(parsed.toString().contains("7KQ29MXP"), "code visible dans le journal")
        // Lien v=1 : lu comme avant.
        val v1 = PairingInfo("PC", "192.168.1.42", 9770, null, key)
        assertEquals(ParsedPairing.V1(v1), PairingLink.parseAny(PairingLink.build(v1)))
        val link = v2.text()
        mapOf(
            link.replace("a=$agent", "a=abc") to "Empreinte de l'agent invalide dans le lien",
            link.replace("c=7KQ29MXP", "c=7KQ29MXU") to "Code d'appairage invalide dans le lien",
            link.replace("h=192.168.1.20", "h=") to "Adresse du PC invalide dans le lien",
            link.replace("r=admin", "r=owner") to "Rôle invalide dans le lien",
            "$link&c=7KQ29MXP" to "Lien d'appairage mal formé",
            link.replace("v=2", "v=3") to "Version de lien non prise en charge : 3",
            "https://example.com" to "Ce n'est pas un lien d'appairage wolagent://",
        ).forEach { (text, message) ->
            assertEquals(message, assertThrows<IllegalArgumentException> { PairingLink.parseAny(text) }.message, text)
        }
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
