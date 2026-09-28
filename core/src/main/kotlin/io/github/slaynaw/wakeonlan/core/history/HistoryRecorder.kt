package io.github.slaynaw.wakeonlan.core.history

import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.core.status.StatusNotice

/** Déduit les évènements de l'historique des changements d'état constatés par la surveillance. */
object HistoryRecorder {
    /**
     * Évènements survenus entre deux relevés ([previous] → [current]) :
     * - éteint / en démarrage / en redémarrage → allumé : « allumé ». Si l'agent répond, l'heure réelle
     *   du démarrage est calculée depuis son uptime ; sinon l'heure est celle du constat (approximative) ;
     * - allumé / en arrêt / en redémarrage → éteint : « éteint », daté de la dernière réponse du PC
     *   (et jamais avant la demande d'extinction ou de redémarrage) ;
     * - démarrage demandé sans réponse dans le délai : « pas de réponse ».
     *
     * Un état précédent inconnu (application qui démarre, réseau absent) ne produit rien : on ne sait
     * pas ce qui s'est passé pendant ce temps (le journal de l'agent le dira).
     */
    fun transitions(previous: Map<String, DeviceStatus>, current: Map<String, DeviceStatus>, now: Long): List<HistoryEvent> {
        val out = mutableListOf<HistoryEvent>()
        for ((id, c) in current) {
            val p = previous[id] ?: continue
            if (p.state == c.state) continue
            when {
                c.state == PowerState.ONLINE && p.state in RETURNING -> {
                    var event = HistoryEvent(id, now, HistoryKind.ON, HistorySource.APP, approx = true)
                    val uptime = c.agent?.uptimeSeconds ?: 0
                    if (uptime > 0) {
                        val boot = now - uptime * 1000
                        // Démarrage postérieur à la dernière extinction constatée : c'est bien ce démarrage-ci
                        // (sinon, sortie de veille : l'uptime date d'avant).
                        if (boot >= p.since - 60_000 && boot <= now) event = event.copy(time = boot, approx = false)
                    }
                    out += event
                }
                c.state == PowerState.OFFLINE && p.state in LEAVING -> {
                    var at = p.lastSeen ?: now
                    // Après une demande d'extinction, l'arrêt ne peut pas la précéder.
                    if (p.state != PowerState.ONLINE && at < p.since) at = p.since
                    out += HistoryEvent(id, at, HistoryKind.OFF, HistorySource.APP, approx = true)
                }
                c.state == PowerState.OFFLINE && p.state == PowerState.WAKING && c.notice == StatusNotice.WAKE_TIMEOUT ->
                    out += HistoryEvent(id, now, HistoryKind.WAKE_TIMEOUT, HistorySource.APP)
            }
        }
        return out
    }

    private val RETURNING = setOf(PowerState.OFFLINE, PowerState.WAKING, PowerState.RESTARTING)
    private val LEAVING = setOf(PowerState.ONLINE, PowerState.SHUTTING_DOWN, PowerState.RESTARTING)
}
