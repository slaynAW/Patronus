package io.github.slaynaw.wakeonlan.core.status

import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus

/** État d'alimentation affiché pour un terminal. */
enum class PowerState {
    /** Pas encore vérifié, ou vérification impossible (voir [UnknownReason]). */
    UNKNOWN,
    ONLINE,
    OFFLINE,

    /** Paquet magique envoyé, en attente de la première réponse. */
    WAKING,

    /** Extinction / mise en veille demandée, en attente de la disparition. */
    SHUTTING_DOWN,

    /** Redémarrage demandé : attente de la disparition puis du retour. */
    RESTARTING,
    ;

    val isTransitional: Boolean get() = this == WAKING || this == SHUTTING_DOWN || this == RESTARTING
}

/** Pourquoi l'état ne peut pas être déterminé : on préfère « inconnu » à un faux « éteint ». */
enum class UnknownReason {
    /** Aucune adresse IP / nom d'hôte renseigné pour ce terminal. */
    NO_HOST,

    /** Le téléphone n'est pas connecté au réseau local (Wi-Fi / Ethernet). */
    NO_NETWORK,

    /** Android 17+ : l'autorisation « appareils à proximité / réseau local » est refusée. */
    NO_PERMISSION,
}

/** Évènement notable à signaler à l'utilisateur. */
enum class StatusNotice {
    /** Le PC n'a pas répondu dans le délai après l'envoi du paquet magique. */
    WAKE_TIMEOUT,

    /** Le PC répond toujours longtemps après la demande d'extinction. */
    SHUTDOWN_TIMEOUT,
}

data class DeviceStatus(
    val state: PowerState = PowerState.UNKNOWN,
    /** Horodatage (ms) d'entrée dans l'état courant. */
    val since: Long = 0,
    /** Dernière fois où la machine a répondu (ms), ou `null`. */
    val lastSeen: Long? = null,
    val latencyMs: Long? = null,
    val method: ProbeMethod? = null,
    val agent: AgentStatus? = null,
    val agentError: AgentError? = null,
    val unknownReason: UnknownReason? = null,
    /** Début de l'action en cours (réveil / extinction / redémarrage), en ms. */
    val actionStartedAt: Long? = null,
    val notice: StatusNotice? = null,
)
