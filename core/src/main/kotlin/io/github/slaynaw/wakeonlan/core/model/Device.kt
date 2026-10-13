package io.github.slaynaw.wakeonlan.core.model

import io.github.slaynaw.wakeonlan.core.agent2.Pin
import kotlinx.serialization.EncodeDefault
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.Serializable

/** Port UDP standard du Wake-on-LAN (« discard »). */
const val DEFAULT_WOL_PORT = 9

/** Port TCP par défaut de l'agent d'extinction installé sur les PC. */
const val DEFAULT_AGENT_PORT = 9770

/**
 * Ports TCP testés pour déterminer si un PC est allumé quand aucun agent n'est installé :
 * RDP, SMB, SSH et NetBIOS. Une connexion acceptée OU refusée prouve que la machine répond.
 */
val DEFAULT_PROBE_PORTS: List<Int> = listOf(3389, 445, 22, 139)

/**
 * Un terminal (PC, serveur, NAS...) à réveiller et surveiller.
 *
 * @property host adresse IP ou nom d'hôte utilisé pour le suivi d'état et l'agent. Vide = pas de suivi.
 * @property broadcastAddress adresse de diffusion forcée ; `null` = calculée automatiquement depuis le Wi-Fi.
 * @property secureOnPassword mot de passe « SecureOn » (6 octets hexadécimaux), exigé par de rares cartes réseau.
 * @property agent paramètres de l'agent d'extinction ; `null` si aucun agent n'est installé sur ce PC.
 */
@Serializable
data class Device(
    val id: String,
    val name: String,
    val mac: MacAddress,
    val host: String = "",
    val broadcastAddress: String? = null,
    val wolPort: Int = DEFAULT_WOL_PORT,
    val secureOnPassword: String? = null,
    val probePorts: List<Int> = DEFAULT_PROBE_PORTS,
    val agent: AgentSettings? = null,
) {
    val hasHost: Boolean get() = host.isNotBlank()
    val canShutdown: Boolean get() = hasHost && agent != null

    /** Copie sans aucun secret (clé d'agent, mot de passe SecureOn), pour l'export en clair. L'empreinte de l'agent, publique, reste. */
    fun withoutSecrets(): Device = copy(secureOnPassword = null, agent = agent?.copy(key = ""))

    override fun toString(): String =
        "Device(id=$id, name=$name, mac=$mac, host=$host, broadcast=$broadcastAddress, wolPort=$wolPort, " +
            "secureOn=${if (secureOnPassword == null) "none" else "***"}, probePorts=$probePorts, agent=$agent)"
}

/**
 * Paramètres de connexion à l'agent.
 * @property key clé secrète partagée de wolagent/1 (32 octets encodés en Base64 URL-safe). Vide = à renseigner, ou
 * effacée une fois le PC passé en wolagent/2.
 * @property pin empreinte wolagent/2 de l'agent (B64), lue dans une réponse signée par la clé ou dans un lien
 * d'appairage. Une fois connue, l'application ne parle plus que wolagent/2 à ce PC (docs/PROTOCOLE-V2.md, §9). Ce
 * n'est pas un secret. Absente du JSON quand elle est vide (format inchangé pour les versions précédentes).
 */
@OptIn(ExperimentalSerializationApi::class)
@Serializable
data class AgentSettings(
    val port: Int = DEFAULT_AGENT_PORT,
    val key: String,
    @EncodeDefault(EncodeDefault.Mode.NEVER) val pin: String = "",
) {
    val hasKey: Boolean get() = key.isNotEmpty()

    /** L'agent est joint en wolagent/2. */
    val hasPin: Boolean get() = pin.isNotEmpty()

    /** L'agent peut être joint : par sa clé (wolagent/1) ou son empreinte (wolagent/2). */
    val ready: Boolean get() = hasKey || hasPin

    /** Empreinte courte de l'agent (« 7KQ2-9MXP-4D1Z »), ou null. */
    val shortPin: String? get() = Pin.parse(pin)?.short()

    /**
     * Empreinte à garder quand la fiche du PC est enregistrée avec la clé [newKey] : l'empreinte connue, sauf si une
     * nouvelle clé est saisie (agent réinstallé, nouveau lien d'appairage) ; le passage en wolagent/2 reprend alors
     * depuis cette clé, qui authentifiera l'empreinte de l'agent.
     */
    fun pinKeptWith(newKey: String): String = if (newKey.isNotEmpty() && newKey != key) "" else pin

    // Ne jamais afficher la clé dans les journaux.
    override fun toString(): String =
        "AgentSettings(port=$port, key=${if (hasKey) "***" else "<vide>"}${shortPin?.let { ", pin=$it" } ?: ""})"
}
