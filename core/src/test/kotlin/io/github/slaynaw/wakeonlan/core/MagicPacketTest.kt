package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.model.DeviceValidator
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import io.github.slaynaw.wakeonlan.core.wol.BroadcastAddresses
import io.github.slaynaw.wakeonlan.core.wol.InterfaceAddress4
import io.github.slaynaw.wakeonlan.core.wol.MagicPacket
import io.github.slaynaw.wakeonlan.core.wol.WakeOnLanSender
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.Inet4Address
import java.net.InetAddress
import java.net.InetSocketAddress

class MagicPacketTest {
    @Test
    fun `paquets conformes aux vecteurs partages`() {
        SharedVectors.value.magicPackets.forEach { v ->
            val password = v.secureOn?.let(DeviceValidator::secureOnBytes)
            val packet = MagicPacket.build(MacAddress.parse(v.mac), password)
            assertEquals(v.packetHex, packet.toHex(), v.mac)
        }
    }

    @Test
    fun `taille et structure`() {
        val packet = MagicPacket.build(MacAddress.parse("01:02:03:04:05:06"))
        assertEquals(102, packet.size)
        assertTrue(packet.take(6).all { it == 0xFF.toByte() })
        assertArrayEquals(byteArrayOf(1, 2, 3, 4, 5, 6), packet.copyOfRange(96, 102))
    }

    @Test
    fun `adresses de diffusion dirigees`() {
        fun iface(ip: String, prefix: Int) = InterfaceAddress4(InetAddress.getByName(ip) as Inet4Address, prefix)
        assertEquals("192.168.1.255", BroadcastAddresses.directed(iface("192.168.1.37", 24))?.hostAddress)
        assertEquals("10.0.15.255", BroadcastAddresses.directed(iface("10.0.12.4", 22))?.hostAddress)
        assertEquals("172.31.255.255", BroadcastAddresses.directed(iface("172.16.0.1", 12))?.hostAddress)
        assertNull(BroadcastAddresses.directed(iface("192.168.1.1", 31)))

        val targets = BroadcastAddresses.targets(
            InetAddress.getByName("192.168.1.255") as Inet4Address,
            listOf(iface("192.168.1.37", 24)),
        ).map { it.hostAddress }
        assertEquals(listOf("192.168.1.255", "255.255.255.255"), targets)
    }

    @Test
    fun `envoi reel sur la boucle locale`() = runTest {
        DatagramSocket(InetSocketAddress(InetAddress.getLoopbackAddress(), 0)).use { receiver ->
            receiver.soTimeout = 2_000
            val packet = MagicPacket.build(MacAddress.parse("AA:BB:CC:DD:EE:01"))
            val result = WakeOnLanSender().send(
                packet,
                listOf(InetAddress.getLoopbackAddress()),
                listOf(receiver.localPort),
                repeat = 2,
                intervalMs = 10,
            )
            assertEquals(2, result.packetsSent)
            repeat(2) {
                val buffer = DatagramPacket(ByteArray(200), 200)
                receiver.receive(buffer)
                assertArrayEquals(packet, buffer.data.copyOf(buffer.length))
            }
        }
    }
}
