package io.github.slaynaw.wakeonlan.ui.devices

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.ui.theme.LocalStatusColors

@Composable
fun statusColor(state: PowerState): Color {
    val colors = LocalStatusColors.current
    return when (state) {
        PowerState.ONLINE -> colors.online
        PowerState.OFFLINE -> colors.offline
        PowerState.UNKNOWN -> colors.unknown
        PowerState.WAKING, PowerState.SHUTTING_DOWN, PowerState.RESTARTING -> colors.transition
    }
}

/** Voyant d'état : vert = allumé, rouge = éteint, orange clignotant = en transition, gris = inconnu. */
@Composable
fun StatusIndicator(state: PowerState, modifier: Modifier = Modifier, size: Dp = 14.dp) {
    val color by animateColorAsState(statusColor(state), label = "statusColor")
    val transition = rememberInfiniteTransition(label = "statusPulse")
    val pulse by transition.animateFloat(
        initialValue = 0.3f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(durationMillis = 700), RepeatMode.Reverse),
        label = "statusAlpha",
    )
    val description = stringResource(
        when (state) {
            PowerState.ONLINE -> R.string.state_online
            PowerState.OFFLINE -> R.string.state_offline
            PowerState.UNKNOWN -> R.string.state_unknown
            PowerState.WAKING -> R.string.state_waking_short
            PowerState.SHUTTING_DOWN -> R.string.state_shutting_down_short
            PowerState.RESTARTING -> R.string.state_restarting_short
        },
    )
    Box(
        modifier
            .size(size)
            .graphicsLayer { alpha = if (state.isTransitional) pulse else 1f }
            .background(color, CircleShape)
            .semantics { contentDescription = description },
    )
}
