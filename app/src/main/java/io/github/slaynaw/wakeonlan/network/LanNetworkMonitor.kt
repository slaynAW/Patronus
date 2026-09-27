package io.github.slaynaw.wakeonlan.network

import android.content.Context
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import io.github.slaynaw.wakeonlan.core.net.NetworkBinder
import io.github.slaynaw.wakeonlan.core.wol.InterfaceAddress4
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.net.DatagramSocket
import java.net.Inet4Address
import java.net.InetAddress
import java.net.Socket
import java.util.concurrent.ConcurrentHashMap

enum class LanTransport { WIFI, ETHERNET }

/** État du réseau local vu par le téléphone. */
data class LanState(
    val network: Network? = null,
    val transport: LanTransport? = null,
    val addresses: List<InterfaceAddress4> = emptyList(),
    /** Un VPN (ex. WireGuard, Tailscale) est actif : utile plus tard pour l'accès à distance. */
    val vpnActive: Boolean = false,
) {
    val connected: Boolean get() = network != null
    val primaryAddress: String? get() = addresses.firstOrNull()?.let { "${it.address.hostAddress}/${it.prefixLength}" }
}

/**
 * Suit en temps réel le réseau Wi-Fi / Ethernet du téléphone, même s'il n'a pas d'accès Internet,
 * et attache les sockets de l'application à ce réseau (voir [NetworkBinder]).
 */
class LanNetworkMonitor(context: Context) : NetworkBinder {

    private val connectivity = context.getSystemService(ConnectivityManager::class.java)
    private val networks = ConcurrentHashMap<Network, Pair<NetworkCapabilities?, LinkProperties?>>()
    private val _state = MutableStateFlow(LanState())
    @Volatile private var vpnActive = false

    val state: StateFlow<LanState> = _state.asStateFlow()

    init {
        val request = NetworkRequest.Builder()
            .addTransportType(NetworkCapabilities.TRANSPORT_WIFI)
            .addTransportType(NetworkCapabilities.TRANSPORT_ETHERNET)
            // Un réseau local sans accès Internet reste parfaitement utilisable pour le Wake-on-LAN.
            .removeCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .build()
        connectivity.registerNetworkCallback(request, object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) = update(network, null, null)
            override fun onCapabilitiesChanged(network: Network, networkCapabilities: NetworkCapabilities) =
                update(network, networkCapabilities, null)

            override fun onLinkPropertiesChanged(network: Network, linkProperties: LinkProperties) =
                update(network, null, linkProperties)

            override fun onLost(network: Network) {
                networks.remove(network)
                publish()
            }
        })
        connectivity.registerDefaultNetworkCallback(object : ConnectivityManager.NetworkCallback() {
            override fun onCapabilitiesChanged(network: Network, networkCapabilities: NetworkCapabilities) {
                vpnActive = networkCapabilities.hasTransport(NetworkCapabilities.TRANSPORT_VPN)
                publish()
            }

            override fun onLost(network: Network) {
                vpnActive = false
                publish()
            }
        })
    }

    private fun update(network: Network, caps: NetworkCapabilities?, props: LinkProperties?) {
        val previous = networks[network]
        networks[network] = Pair(
            caps ?: previous?.first ?: connectivity.getNetworkCapabilities(network),
            props ?: previous?.second ?: connectivity.getLinkProperties(network),
        )
        publish()
    }

    private fun publish() {
        // Ethernet d'abord (plus fiable), puis Wi-Fi.
        val best = networks.entries
            .mapNotNull { (network, info) -> transportOf(info.first)?.let { Triple(network, it, info.second) } }
            .maxByOrNull { it.second.ordinal }
        _state.value = if (best == null) {
            LanState(vpnActive = vpnActive)
        } else {
            val addresses = best.third?.linkAddresses.orEmpty().mapNotNull { la ->
                (la.address as? Inet4Address)?.let { InterfaceAddress4(it, la.prefixLength) }
            }
            LanState(best.first, best.second, addresses, vpnActive)
        }
    }

    private fun transportOf(caps: NetworkCapabilities?): LanTransport? = when {
        caps == null -> null
        caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> LanTransport.ETHERNET
        caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> LanTransport.WIFI
        else -> null
    }

    // --- NetworkBinder : si l'attachement échoue (VPN verrouillé...), le système choisit la route. ---

    override fun bind(socket: Socket) {
        _state.value.network?.let { runCatching { it.bindSocket(socket) } }
    }

    override fun bind(socket: DatagramSocket) {
        _state.value.network?.let { runCatching { it.bindSocket(socket) } }
    }

    override fun resolve(host: String): InetAddress =
        _state.value.network?.let { runCatching { it.getByName(host) }.getOrNull() } ?: InetAddress.getByName(host)
}
