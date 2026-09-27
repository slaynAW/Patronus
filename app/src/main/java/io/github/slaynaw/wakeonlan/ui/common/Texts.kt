package io.github.slaynaw.wakeonlan.ui.common

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.res.stringResource
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.status.UnknownReason
import kotlinx.coroutines.delay

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

/** Système d'exploitation lisible, à partir de la valeur Go `runtime.GOOS`. */
fun AgentStatus.osLabel(): String = when (os) {
    "windows" -> "Windows"
    "linux" -> "Linux"
    "darwin" -> "macOS"
    else -> os
}

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
