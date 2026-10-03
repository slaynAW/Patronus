package io.github.slaynaw.wakeonlan.ui.common

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.AgentTemperatures
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.history.HistoryKind
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.core.status.UnknownReason
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import kotlinx.coroutines.delay
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale
import kotlin.math.roundToInt

@StringRes
fun AgentError.label(): Int = when (this) {
    AgentError.NO_KEY -> R.string.agent_error_no_key
    AgentError.UNKNOWN_HOST -> R.string.agent_error_unknown_host
    AgentError.UNREACHABLE -> R.string.agent_error_unreachable
    AgentError.REFUSED -> R.string.agent_error_refused
    AgentError.UNAUTHORIZED -> R.string.agent_error_unauthorized
    AgentError.RATE_LIMITED -> R.string.agent_error_rate_limited
    AgentError.PROTOCOL -> R.string.agent_error_protocol
    AgentError.REJECTED -> R.string.agent_error_rejected
}

@StringRes
fun UnknownReason.label(): Int = when (this) {
    UnknownReason.NO_HOST -> R.string.unknown_no_host
    UnknownReason.NO_NETWORK -> R.string.unknown_no_network
    UnknownReason.NO_PERMISSION -> R.string.unknown_no_permission
}

@StringRes
fun PowerAction.label(): Int = when (this) {
    PowerAction.SHUTDOWN -> R.string.action_shutdown
    PowerAction.REBOOT -> R.string.action_reboot
    PowerAction.SLEEP -> R.string.action_sleep
}

@StringRes
fun PowerAction.confirmTitle(): Int = when (this) {
    PowerAction.SHUTDOWN -> R.string.confirm_shutdown_title
    PowerAction.REBOOT -> R.string.confirm_reboot_title
    PowerAction.SLEEP -> R.string.confirm_sleep_title
}

@StringRes
fun PowerAction.sentMessage(): Int = when (this) {
    PowerAction.SHUTDOWN -> R.string.message_shutdown_sent
    PowerAction.REBOOT -> R.string.message_reboot_sent
    PowerAction.SLEEP -> R.string.message_sleep_sent
}

fun PowerAction.icon(): ImageVector = when (this) {
    PowerAction.SHUTDOWN -> WolIcons.Power
    PowerAction.REBOOT -> WolIcons.Restart
    PowerAction.SLEEP -> WolIcons.Moon
}

/** Nom de l'état (voyants, accessibilité). */
@StringRes
fun PowerState.label(): Int = when (this) {
    PowerState.ONLINE -> R.string.state_online
    PowerState.OFFLINE -> R.string.state_offline
    PowerState.UNKNOWN -> R.string.state_unknown
    PowerState.WAKING -> R.string.state_waking_short
    PowerState.SHUTTING_DOWN -> R.string.state_shutting_down_short
    PowerState.RESTARTING -> R.string.state_restarting_short
}

/** Système d'exploitation lisible, à partir de la valeur Go `runtime.GOOS`. */
fun AgentStatus.osLabel(): String = when (os) {
    "windows" -> "Windows"
    "linux" -> "Linux"
    "darwin" -> "macOS"
    else -> os
}

/** Architecture lisible, à partir de la valeur Go `runtime.GOARCH`. */
fun AgentStatus.archLabel(): String = when (arch) {
    "amd64" -> "x64"
    "386" -> "x86"
    "arm64" -> "ARM64"
    "arm" -> "ARM"
    else -> arch
}

/** « Windows · x64 ». */
fun AgentStatus.systemLabel(): String = listOf(osLabel(), archLabel()).filter { it.isNotBlank() }.joinToString(" · ")

/** Version de l'agent : « v1.2.0 », mais « dev » tel quel. */
fun AgentStatus.versionLabel(): String = if (version.firstOrNull()?.isDigit() == true) "v$version" else version

/** « 54 °C » : arrondi au degré, espace insécable. */
fun formatCelsius(celsius: Double): String = "${celsius.roundToInt()}\u00A0°C"

/** Couleur d'une température : [normal], chaude (orange), très chaude (rouge). */
fun temperatureColor(celsius: Double?, normal: Color = WolPalette.Text): Color = when {
    celsius == null -> normal
    celsius >= AgentTemperatures.HOT -> WolPalette.DangerText
    celsius >= AgentTemperatures.WARM -> WolPalette.Busy
    else -> normal
}

/**
 * « CPU 54 °C · GPU 61 °C » ; vide si aucune température n'est connue. Une puce graphique intégrée
 * ([AgentTemperatures.gpuShared]) n'est pas répétée : sa température est celle du processeur.
 */
@Composable
fun AgentTemperatures.summary(): String = listOfNotNull(
    cpu?.let { stringResource(R.string.temperature_cpu_short, formatCelsius(it)) },
    gpu?.takeUnless { gpuShared && cpu != null }?.let { stringResource(R.string.temperature_gpu_short, formatCelsius(it)) },
).joinToString(" · ")

/** Température la plus élevée (couleur d'un résumé). */
fun AgentTemperatures.hottest(): Double? = listOfNotNull(cpu, gpu).maxOrNull()

/** Durée courte et lisible : « 12 s », « 5 min », « 3 h », « 2 j ». */
@Composable
fun formatDuration(millis: Long): String {
    val seconds = (millis / 1000).coerceAtLeast(0)
    return when {
        seconds < 60 -> stringResource(R.string.duration_seconds, seconds)
        seconds < 3600 -> stringResource(R.string.duration_minutes, seconds / 60)
        seconds < 86_400 -> stringResource(R.string.duration_hours, seconds / 3600)
        else -> stringResource(R.string.duration_days, seconds / 86_400)
    }
}

/** Durée détaillée : « 3 h 12 min », « 2 j 4 h ». */
@Composable
fun formatLongDuration(totalSeconds: Long): String {
    val s = totalSeconds.coerceAtLeast(0)
    val days = s / 86_400
    val hours = (s % 86_400) / 3600
    val minutes = (s % 3600) / 60
    return when {
        days > 0 && hours > 0 -> stringResource(R.string.duration_days_hours, days, hours)
        days > 0 -> stringResource(R.string.duration_days, days)
        hours > 0 && minutes > 0 -> stringResource(R.string.duration_hours_minutes, hours, minutes)
        hours > 0 -> stringResource(R.string.duration_hours, hours)
        minutes > 0 -> stringResource(R.string.duration_minutes, minutes)
        else -> stringResource(R.string.duration_seconds, s)
    }
}

/** Texte complet de l'état : « Allumé · 2 ms », « Éteint · vu il y a 2 h », « Démarrage en cours… 12 s ». */
@Composable
fun statusText(status: DeviceStatus, now: Long): String {
    val sinceAction = now - (status.actionStartedAt ?: now)
    return when (status.state) {
        PowerState.ONLINE -> {
            val online = stringResource(R.string.state_online)
            status.latencyMs?.let { stringResource(R.string.state_with_detail, online, stringResource(R.string.latency_ms, it)) }
                ?: online
        }
        PowerState.OFFLINE -> status.lastSeen
            ?.let { stringResource(R.string.state_offline_seen, formatDuration(now - it)) }
            ?: stringResource(R.string.state_offline)
        PowerState.WAKING -> stringResource(R.string.state_waking, formatDuration(sinceAction))
        PowerState.SHUTTING_DOWN -> stringResource(R.string.state_shutting_down, formatDuration(sinceAction))
        PowerState.RESTARTING -> stringResource(R.string.state_restarting, formatDuration(sinceAction))
        PowerState.UNKNOWN -> status.unknownReason
            ?.let { stringResource(R.string.state_with_detail, stringResource(R.string.state_unknown), stringResource(it.label())) }
            ?: stringResource(R.string.state_checking)
    }
}

/** Texte court de l'état (listes compactes) : « Allumé », « Éteint · il y a 2 h », « Démarrage… 12 s ». */
@Composable
fun shortStatusText(status: DeviceStatus, now: Long): String {
    val sinceAction = now - (status.actionStartedAt ?: now)
    return when (status.state) {
        PowerState.ONLINE -> stringResource(R.string.state_online)
        PowerState.OFFLINE -> status.lastSeen
            ?.let { stringResource(R.string.state_offline_ago, formatDuration(now - it)) }
            ?: stringResource(R.string.state_offline)
        PowerState.WAKING -> stringResource(R.string.state_waking_compact, formatDuration(sinceAction))
        PowerState.SHUTTING_DOWN -> stringResource(R.string.state_shutting_down_compact, formatDuration(sinceAction))
        PowerState.RESTARTING -> stringResource(R.string.state_restarting_compact, formatDuration(sinceAction))
        PowerState.UNKNOWN -> stringResource(if (status.unknownReason != null) R.string.state_unknown_short else R.string.state_checking)
    }
}

// ---------------------------------------------------------------------------------------------
// Historique
// ---------------------------------------------------------------------------------------------

@StringRes
fun HistoryKind.label(): Int = when (this) {
    HistoryKind.ON -> R.string.kind_on
    HistoryKind.OFF -> R.string.kind_off
    HistoryKind.LOST -> R.string.kind_lost
    HistoryKind.SLEEP -> R.string.kind_sleep
    HistoryKind.RESUME -> R.string.kind_resume
    HistoryKind.WAKE_SENT -> R.string.kind_wake
    HistoryKind.SHUTDOWN_SENT -> R.string.kind_shutdown_req
    HistoryKind.REBOOT_SENT -> R.string.kind_reboot_req
    HistoryKind.SLEEP_SENT -> R.string.kind_sleep_req
    HistoryKind.WAKE_TIMEOUT -> R.string.kind_wake_timeout
}

fun HistoryKind.icon(): ImageVector = when (this) {
    HistoryKind.ON, HistoryKind.OFF, HistoryKind.SHUTDOWN_SENT -> WolIcons.Power
    HistoryKind.LOST -> WolIcons.Bolt
    HistoryKind.SLEEP, HistoryKind.SLEEP_SENT -> WolIcons.Moon
    HistoryKind.RESUME -> WolIcons.Sun
    HistoryKind.WAKE_SENT -> WolIcons.Send
    HistoryKind.REBOOT_SENT -> WolIcons.Restart
    HistoryKind.WAKE_TIMEOUT -> WolIcons.Clock
}

/** Couleur discrète de l'icône : vert (allumé), rouge (éteint), bleu (démarrage demandé)… */
fun HistoryKind.tint(): Color = when (this) {
    HistoryKind.ON, HistoryKind.RESUME -> WolPalette.On
    HistoryKind.OFF, HistoryKind.LOST -> WolPalette.Off
    HistoryKind.WAKE_TIMEOUT -> WolPalette.Busy
    HistoryKind.WAKE_SENT -> WolPalette.Blue
    else -> WolPalette.Text2
}

private val clockFormat = DateTimeFormatter.ofPattern("HH:mm", Locale.FRANCE)
private val shortDateFormat = DateTimeFormatter.ofPattern("d MMM", Locale.FRANCE)
private val longDateFormat = DateTimeFormatter.ofPattern("EEEE d MMMM", Locale.FRANCE)

private fun localDate(ms: Long): LocalDate = Instant.ofEpochMilli(ms).atZone(ZoneId.systemDefault()).toLocalDate()

/** Heure seule : « 14:32 ». */
fun clockText(ms: Long): String = Instant.ofEpochMilli(ms).atZone(ZoneId.systemDefault()).format(clockFormat)

/** Heure d'un évènement, relative au jour : « 14:32 », « hier 09:10 », « 22 sept. 18:04 ». */
@Composable
fun eventTimeText(ms: Long, now: Long): String {
    val day = localDate(ms)
    val today = localDate(now)
    return when {
        !day.isBefore(today) -> clockText(ms)
        day == today.minusDays(1) -> stringResource(R.string.history_yesterday_at, clockText(ms))
        else -> "${shortDateFormat.format(day)} ${clockText(ms)}"
    }
}

/** Jour d'un groupe d'évènements : « Aujourd'hui », « Hier », « lundi 22 septembre ». */
@Composable
fun dayText(ms: Long, now: Long): String {
    val day = localDate(ms)
    val today = localDate(now)
    return when {
        !day.isBefore(today) -> stringResource(R.string.history_today)
        day == today.minusDays(1) -> stringResource(R.string.history_yesterday)
        else -> longDateFormat.format(day)
    }
}

/** Jour (pour grouper les évènements). */
fun dayKey(ms: Long): Long = localDate(ms).toEpochDay()

/** Horloge qui se met à jour toutes les [periodMs] : pour les « il y a 12 s » et les chronomètres. */
@Composable
fun rememberNow(periodMs: Long = 1_000): Long {
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(periodMs) {
        while (true) {
            now = System.currentTimeMillis()
            delay(periodMs)
        }
    }
    return now
}
