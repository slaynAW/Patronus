package io.github.slaynaw.wakeonlan.core.hub

import io.github.slaynaw.wakeonlan.core.agent.AgentHistory
import io.github.slaynaw.wakeonlan.core.agent.AgentMetrics
import io.github.slaynaw.wakeonlan.core.agent2.DeviceIdentity
import io.github.slaynaw.wakeonlan.core.agent2.DeviceInfo
import io.github.slaynaw.wakeonlan.core.agent2.InviteInfo
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Client
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Session
import io.github.slaynaw.wakeonlan.core.net.NetworkBinder
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.jsonObject
import java.io.Closeable
import java.net.Socket

/** Requête au hub (§4) ; les champs vides ne sont pas envoyés. */
@Serializable
data class HubRequest(
    val cmd: String,
    val device: String? = null,
    val role: String? = null,
    val name: String? = null,
    val ttl: Long? = null,
    val invite: String? = null,
    val code: String? = null,
    val force: Boolean? = null,
    val pc: String? = null,
    val host: String? = null,
    val port: Int? = null,
    val mac: String? = null,
    val agent: String? = null,
    val probe: HubProbe? = null,
    val from: Long? = null,
    val day: String? = null,
    val retention: Int? = null,
    val since: Long? = null,
)

/** Sonde d'un appareil sans agent (§6.1) : « tcp » avec un port, « http » ou « https ». */
@Serializable
data class HubProbe(val kind: String, val port: Int = 0)

/** Invitation créée sur le hub : celle de wolagent/2, avec l'empreinte du hub. */
@Serializable
data class HubInvite(
    val id: String = "",
    val code: String = "",
    val role: String = "",
    val name: String = "",
    val expires: Long = 0,
    val device: String = "",
    val hub: String = "",
)

/** État du hub (hub.status). */
@Serializable
data class HubInfo(val name: String = "", val version: String = "", val pin: String = "", val started: Long = 0)

/** PC confié au hub, ou appareil sans agent (§6.1). */
@Serializable
data class HubPc(
    val id: String = "",
    val name: String = "",
    val host: String = "",
    val port: Int = 0,
    val mac: String = "",
    val agent: String = "",
    val role: String = "",
    val probe: HubProbe? = null,
    val added: Long = 0,
)

/**
 * État d'un PC vu par le hub (§6.2). [agent] est la dernière réponse de l'agent à « status », à lire comme une
 * réponse reçue directement d'un agent (mêmes validations).
 */
@Serializable
data class HubPcState(
    val id: String = "",
    val state: String = "",
    val since: Long = 0,
    val checked: Long = 0,
    val latency: Long = 0,
    val problem: String = "",
    val agent: JsonElement? = null,
) {
    companion object {
        const val UNKNOWN = "unknown"
        const val ONLINE = "online"
        const val OFFLINE = "offline"
        const val AGENT_DOWN = "agent_down"
    }
}

/** Évènement détecté par le hub (§6.5) : des codes, que l'application rédige elle-même. */
@Serializable
data class HubEvent(val t: Long = 0, val pc: String = "", val k: String = "", val x: String = "", val i: String = "")

/** Réglages du hub (§6.4). */
@Serializable
data class HubSettings(val retention: Int = 0)

/** Réponse du hub (§4). */
@Serializable
data class HubResponse(
    val id: Long = 0,
    val ok: Boolean = false,
    val code: String = "",
    val message: String = "",
    val role: String = "",
    val device: String = "",
    val devices: List<DeviceInfo> = emptyList(),
    val invite: HubInvite? = null,
    val invites: List<InviteInfo> = emptyList(),
    val hub: HubInfo? = null,
    val pcs: List<HubPc> = emptyList(),
    val pc: HubPc? = null,
    val states: List<HubPcState> = emptyList(),
    val history: AgentHistory? = null,
    val more: Boolean = false,
    val metrics: AgentMetrics? = null,
    val settings: HubSettings? = null,
    val events: List<HubEvent> = emptyList(),
)

/**
 * Client du hub : le transport de wolagent/2 (salut, TLS 1.3 avec l'empreinte du hub vérifiée avant tout envoi,
 * trames), avec le protocole patronus-hub/1. Bloquant : à appeler hors du fil principal.
 */
class HubClient(binder: NetworkBinder = NetworkBinder.Default, connectTimeoutMs: Int = 2_000, readTimeoutMs: Int = 10_000) {
    private val transport = Wolagent2Client(binder, connectTimeoutMs, readTimeoutMs)

    /**
     * Ouvre une session avec le hub [host]:[port], dont l'empreinte [hub] est épinglée.
     *
     * @throws io.github.slaynaw.wakeonlan.core.agent2.Wolagent2Exception ou une IOException de connexion.
     */
    fun connect(host: String, port: Int, identity: DeviceIdentity, hub: Pin, onSocket: (Socket) -> Unit = {}): HubSession =
        HubSession(transport.connect(host, port, identity, hub, onSocket, protocol = HubProtocol.ALPN))
}

/** Session avec le hub : une requête à la fois. */
class HubSession internal constructor(private val session: Wolagent2Session) : Closeable {
    /** Empreinte du hub, vérifiée pendant la poignée de main. */
    val hubPin: Pin get() = session.agentPin

    /** Envoie une requête et lit sa réponse. */
    fun request(request: HubRequest): HubResponse =
        session.call(json.encodeToJsonElement(HubRequest.serializer(), request).jsonObject, HubResponse.serializer())

    override fun close() = session.close()

    private companion object {
        val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
    }
}
