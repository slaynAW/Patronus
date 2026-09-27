package io.github.slaynaw.wakeonlan.core.status

/**
 * Machine à états d'un terminal. Elle transforme une suite de sondes brutes en un état fiable :
 *
 * - une seule réponse suffit pour passer « en ligne » ;
 * - il faut [offlineThreshold] échecs consécutifs pour passer « hors ligne » depuis « en ligne »
 *   (hystérésis : un paquet perdu sur le Wi-Fi ne fait pas clignoter l'indicateur) ;
 * - après un réveil / une extinction / un redémarrage, l'état transitoire est conservé jusqu'à
 *   confirmation ou expiration du délai, avec un [StatusNotice] en cas d'échec.
 *
 * Classe pure et déterministe (horloge injectée) : entièrement testée unitairement.
 */
class StatusTracker(
    private val clock: () -> Long,
    var wakeTimeoutMs: Long = 180_000,
    var shutdownTimeoutMs: Long = 120_000,
    var restartTimeoutMs: Long = 300_000,
    private val offlineThreshold: Int = 2,
) {
    var status: DeviceStatus = DeviceStatus(since = clock())
        private set

    private var failures = 0
    private var sawOfflineDuringRestart = false

    fun onProbe(result: ProbeResult): DeviceStatus {
        val now = clock()
        val current = status
        val base = current.copy(
            unknownReason = null,
            agentError = result.agentError,
        )
        status = if (result.reachable) {
            failures = 0
            val seen = base.copy(
                lastSeen = now,
                latencyMs = result.latencyMs,
                method = result.method,
                agent = result.agent ?: current.agent,
            )
            when (current.state) {
                PowerState.SHUTTING_DOWN ->
                    if (elapsed(now) < shutdownTimeoutMs) seen
                    else seen.enter(PowerState.ONLINE, now).copy(notice = StatusNotice.SHUTDOWN_TIMEOUT)
                PowerState.RESTARTING ->
                    if (!sawOfflineDuringRestart && elapsed(now) < restartTimeoutMs) seen
                    else seen.enter(PowerState.ONLINE, now)
                PowerState.ONLINE -> seen
                else -> seen.enter(PowerState.ONLINE, now)
            }
        } else {
            failures++
            when (current.state) {
                PowerState.WAKING ->
                    if (elapsed(now) < wakeTimeoutMs) base
                    else base.enter(PowerState.OFFLINE, now).copy(notice = StatusNotice.WAKE_TIMEOUT)
                PowerState.RESTARTING -> {
                    sawOfflineDuringRestart = true
                    if (elapsed(now) < restartTimeoutMs) base else base.enter(PowerState.OFFLINE, now)
                }
                PowerState.ONLINE, PowerState.SHUTTING_DOWN ->
                    if (failures >= offlineThreshold) base.enter(PowerState.OFFLINE, now) else base
                PowerState.OFFLINE -> base
                PowerState.UNKNOWN -> base.enter(PowerState.OFFLINE, now)
            }
        }
        return status
    }

    /** La vérification est impossible (pas de réseau, pas d'adresse, pas d'autorisation). */
    fun onUnavailable(reason: UnknownReason): DeviceStatus {
        failures = 0
        val now = clock()
        status = status.enter(PowerState.UNKNOWN, now).copy(unknownReason = reason, latencyMs = null)
        return status
    }

    fun onWakeSent(): DeviceStatus = startAction(PowerState.WAKING)

    fun onShutdownSent(): DeviceStatus = startAction(PowerState.SHUTTING_DOWN)

    fun onRestartSent(): DeviceStatus {
        sawOfflineDuringRestart = false
        return startAction(PowerState.RESTARTING)
    }

    /** Efface la notification affichée (l'utilisateur l'a vue). */
    fun clearNotice(): DeviceStatus {
        status = status.copy(notice = null)
        return status
    }

    private fun startAction(state: PowerState): DeviceStatus {
        val now = clock()
        failures = 0
        status = status.copy(state = state, since = now, actionStartedAt = now, notice = null, unknownReason = null)
        return status
    }

    private fun elapsed(now: Long): Long = now - (status.actionStartedAt ?: now)

    private fun DeviceStatus.enter(state: PowerState, now: Long): DeviceStatus =
        if (this.state == state) this
        else copy(
            state = state,
            since = now,
            actionStartedAt = if (state.isTransitional) actionStartedAt else null,
            notice = null,
        )
}
