package io.github.slaynaw.wakeonlan.core.wol

import java.net.Inet4Address
import java.net.InetAddress

/** Adresse IPv4 d'une interface réseau du téléphone avec la longueur de son préfixe (ex. /24). */
data class InterfaceAddress4(val address: Inet4Address, val prefixLength: Int) {
    init {
        require(prefixLength in 0..32) { "Préfixe IPv4 invalide : $prefixLength" }
    }
}

object BroadcastAddresses {
    val LIMITED: Inet4Address = InetAddress.getByName("255.255.255.255") as Inet4Address

    /**
     * Adresse de diffusion dirigée d'un sous-réseau (ex. 192.168.1.37/24 → 192.168.1.255).
     * Retourne `null` pour les préfixes /31 et /32 qui n'ont pas de broadcast.
     */
    fun directed(iface: InterfaceAddress4): Inet4Address? {
        if (iface.prefixLength >= 31) return null
        val ip = iface.address.address.fold(0) { acc, b -> (acc shl 8) or (b.toInt() and 0xFF) }
        val mask = if (iface.prefixLength == 0) 0 else -1 shl (32 - iface.prefixLength)
        val broadcast = ip or mask.inv()
        val bytes = ByteArray(4) { i -> (broadcast ushr (24 - 8 * i)).toByte() }
        return InetAddress.getByAddress(bytes) as Inet4Address
    }

    /**
     * Toutes les destinations à utiliser pour un réveil, sans doublon, par ordre de priorité :
     * 1. l'adresse de diffusion forcée par l'utilisateur (si définie) ;
     * 2. les diffusions dirigées des sous-réseaux du téléphone ;
     * 3. la diffusion limitée 255.255.255.255.
     */
    fun targets(override: Inet4Address?, interfaces: List<InterfaceAddress4>): List<Inet4Address> =
        buildList {
            override?.let(::add)
            interfaces.mapNotNullTo(this, ::directed)
            add(LIMITED)
        }.distinct()
}
