package io.github.slaynaw.wakeonlan.core.wol

import io.github.slaynaw.wakeonlan.core.model.MacAddress

/**
 * Construction du « paquet magique » Wake-on-LAN :
 * 6 octets `0xFF`, puis 16 répétitions de l'adresse MAC, puis (optionnel) 6 octets SecureOn.
 */
object MagicPacket {
    const val HEADER_SIZE = 6
    const val REPETITIONS = 16
    const val SIZE = HEADER_SIZE + REPETITIONS * MacAddress.LENGTH
    const val SIZE_WITH_PASSWORD = SIZE + 6

    fun build(mac: MacAddress, secureOnPassword: ByteArray? = null): ByteArray {
        require(secureOnPassword == null || secureOnPassword.size == 6) { "Le mot de passe SecureOn fait 6 octets" }
        val macBytes = mac.toByteArray()
        val packet = ByteArray(if (secureOnPassword == null) SIZE else SIZE_WITH_PASSWORD)
        packet.fill(0xFF.toByte(), 0, HEADER_SIZE)
        repeat(REPETITIONS) { i -> macBytes.copyInto(packet, HEADER_SIZE + i * MacAddress.LENGTH) }
        secureOnPassword?.copyInto(packet, SIZE)
        return packet
    }
}
