package io.github.slaynaw.wakeonlan.core.wol

import io.github.slaynaw.wakeonlan.core.net.NetworkBinder
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import java.io.IOException
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetAddress
import java.net.InetSocketAddress

/** Résultat d'un envoi : combien de paquets sont partis, vers quelles destinations, et les erreurs. */
data class WakeResult(
    val packetsSent: Int,
    val destinations: List<InetSocketAddress>,
    val errors: List<String>,
) {
    val success: Boolean get() = packetsSent > 0
}

/**
 * Envoie le paquet magique en UDP.
 *
 * Pour la fiabilité, chaque destination reçoit le paquet [DEFAULT_REPEAT] fois : l'UDP ne garantit
 * pas la livraison et certaines cartes réseau ratent le premier paquet.
 */
class WakeOnLanSender(
    private val binder: NetworkBinder = NetworkBinder.Default,
    private val socketFactory: () -> DatagramSocket = { DatagramSocket(null) },
) {
    suspend fun send(
        packet: ByteArray,
        addresses: List<InetAddress>,
        ports: Collection<Int>,
        repeat: Int = DEFAULT_REPEAT,
        intervalMs: Long = DEFAULT_INTERVAL_MS,
    ): WakeResult = withContext(Dispatchers.IO) {
        require(addresses.isNotEmpty()) { "Aucune destination" }
        require(repeat >= 1) { "repeat doit être >= 1" }
        val destinations = addresses.distinct().flatMap { a -> ports.distinct().map { InetSocketAddress(a, it) } }
        val errors = linkedSetOf<String>()
        var sent = 0

        socketFactory().use { socket ->
            socket.reuseAddress = true
            socket.broadcast = true
            socket.bind(InetSocketAddress(0))
            binder.bind(socket)
            for (round in 0 until repeat) {
                if (round > 0) delay(intervalMs)
                for (destination in destinations) {
                    try {
                        socket.send(DatagramPacket(packet, packet.size, destination))
                        sent++
                    } catch (e: IOException) {
                        // Une destination peut échouer (ex. broadcast refusé) sans bloquer les autres.
                        errors += "${destination.address.hostAddress}:${destination.port} : ${e.message}"
                    }
                }
            }
        }
        WakeResult(sent, destinations, errors.toList())
    }

    companion object {
        const val DEFAULT_REPEAT = 3
        const val DEFAULT_INTERVAL_MS = 120L
    }
}
