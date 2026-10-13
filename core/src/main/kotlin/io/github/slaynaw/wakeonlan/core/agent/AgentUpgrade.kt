package io.github.slaynaw.wakeonlan.core.agent

import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus

/**
 * Passage d'un PC en wolagent/2 (docs/PROTOCOLE-V2.md, §9), sans rien demander à l'utilisateur, dès que son agent le
 * permet. Même règle que l'application Windows :
 *
 * 1. l'agent annonce wolagent/2 et son empreinte dans une réponse wolagent/1, signée par la clé : l'empreinte est donc
 *    authentique ;
 * 2. une session wolagent/2 où cette empreinte est exigée dit si l'agent connaît déjà cet appareil ; sinon, il
 *    l'enregistre (migration, prouvée par la clé de part et d'autre) ;
 * 3. l'empreinte est alors enregistrée et la clé effacée, en une seule écriture : l'application ne parle plus que
 *    wolagent/2 à ce PC.
 *
 * Une interruption à n'importe quelle étape laisse le PC en wolagent/1 ; l'essai suivant reprend (l'étape 2 reconnaît
 * un enregistrement déjà fait). Un PC partagé en v1 avec le droit « démarrer et éteindre » (personne sans partage v2,
 * §10) garde sa clé, que ce partage transmet, tant qu'il en a besoin. Les PC reçus d'une autre personne ne sont pas
 * concernés.
 */
object AgentUpgrade {
    /** Résultat d'un passage. */
    sealed interface Outcome {
        /** Rien à faire : l'agent n'annonce pas wolagent/2. */
        data object Unchanged : Outcome

        /** Réglages à enregistrer ; [role] : rôle donné par une migration, null si l'agent connaissait déjà cet appareil. */
        data class Upgraded(val settings: AgentSettings, val role: String?) : Outcome

        /** Échec : nouvel essai plus tard ([retryDelayMs]). */
        data class Failed(val error: AgentError, val detail: String?) : Outcome
    }

    /**
     * Faut-il avancer le passage de ce PC, d'après son dernier état ? [sharedWithControl] : PC partagé avec le droit
     * « démarrer et éteindre » (sa clé est gardée).
     */
    fun worthTrying(device: Device, status: DeviceStatus, sharedWithControl: Boolean): Boolean {
        val agent = device.agent ?: return false
        if (!agent.hasKey || !device.hasHost) return false
        val v2 = status.agent?.proto == Wolagent2.ALPN
        return when {
            // wolagent/1 : l'agent doit annoncer wolagent/2 et son empreinte.
            !agent.hasPin -> status.agent?.let { it.pin.isNotEmpty() && Wolagent2.ALPN in it.protocols } == true
            // Enregistré, clé gardée pour le partage : rien à faire.
            v2 && sharedWithControl -> false
            // PC éteint, injoignable ou autre empreinte : rien à tenter.
            !v2 && status.agentError != AgentError.UNREGISTERED -> false
            else -> true
        }
    }

    /** Mène le passage d'un PC ; [keepKey] : PC partagé avec le droit « démarrer et éteindre ». */
    suspend fun run(client: AgentClient, device: Device, keepKey: Boolean): Outcome {
        val before = device.agent ?: return Outcome.Unchanged
        if (!before.hasKey || !device.hasHost) return Outcome.Unchanged
        var next = before
        if (!next.hasPin) {
            // Réponse wolagent/1 signée par la clé : l'empreinte annoncée est celle du vrai agent.
            val announced = when (val r = client.status(device.host, next)) {
                is AgentResult.Success -> r.value
                is AgentResult.Failure -> return Outcome.Failed(r.error, r.detail)
            }
            if (Pin.parse(announced.pin) == null || Wolagent2.ALPN !in announced.protocols) return Outcome.Unchanged
            next = next.copy(pin = announced.pin)
        }
        var role: String? = null
        when (val r = client.status(device.host, next)) {
            is AgentResult.Success -> Unit
            is AgentResult.Failure -> {
                if (r.error != AgentError.UNREGISTERED) return Outcome.Failed(r.error, r.detail)
                when (val m = client.migrate(device.host, next)) {
                    is AgentResult.Success -> role = m.value
                    is AgentResult.Failure -> return Outcome.Failed(m.error, m.detail)
                }
            }
        }
        if (!keepKey) next = next.copy(key = "")
        return Outcome.Upgraded(next, role)
    }

    /** Délai avant un nouvel essai après [failures] échecs consécutifs : une minute, doublé à chaque fois, une heure au plus. */
    fun retryDelayMs(failures: Int): Long = minOf(3_600_000L, 60_000L shl (failures - 1).coerceIn(0, 6))
}
