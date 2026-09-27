package io.github.slaynaw.wakeonlan.core.net

import java.net.DatagramSocket
import java.net.InetAddress
import java.net.Socket

/**
 * Permet d'attacher les sockets (et la résolution DNS) à un réseau précis.
 *
 * Sur Android, si le Wi-Fi n'a pas d'accès Internet, le système peut router le trafic par défaut
 * vers les données mobiles : les paquets destinés au réseau local partiraient alors dans le vide.
 * L'implémentation Android attache donc chaque socket au réseau Wi-Fi / Ethernet.
 */
interface NetworkBinder {
    fun bind(socket: Socket)
    fun bind(socket: DatagramSocket)
    fun resolve(host: String): InetAddress

    /** Aucun attachement : le système choisit le réseau (JVM, tests). */
    object Default : NetworkBinder {
        override fun bind(socket: Socket) = Unit
        override fun bind(socket: DatagramSocket) = Unit
        override fun resolve(host: String): InetAddress = InetAddress.getByName(host)
    }
}
