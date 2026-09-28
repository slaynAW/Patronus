package io.github.slaynaw.wakeonlan.ui.detail

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.status.LatencyLog
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.core.status.ProbeMethod
import io.github.slaynaw.wakeonlan.ui.common.LatencyTrace
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.TRACE_WINDOW_MS
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.traceScale
import io.github.slaynaw.wakeonlan.ui.devices.DeviceItem
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/**
 * Carte « Latence » de la fiche : valeur en direct, tracé de la dernière minute façon
 * électrocardiogramme et synthèse (min, moyenne, max, sondes sans réponse).
 */
@Composable
fun LatencyCard(item: DeviceItem, now: Long) {
    val status = item.status
    val checking = status.state != PowerState.UNKNOWN
    val current = status.latencyMs.takeIf { status.state == PowerState.ONLINE }
    val samples = item.latency
    val stats = remember(samples, now) { LatencyLog.stats(samples, now - TRACE_WINDOW_MS) }
    val scale = remember(samples, now) { traceScale(samples, now) }
    val none = "—"
    val min = stats?.takeIf { it.lost < it.count }?.min?.let { msText(it) } ?: none
    val average = stats?.takeIf { it.lost < it.count }?.average?.let { msText(it) } ?: none
    val max = stats?.takeIf { it.lost < it.count }?.max?.let { msText(it) } ?: none
    val description = if (stats != null) {
        stringResource(R.string.latency_description, average, max, stats.lost, stats.count)
    } else {
        stringResource(R.string.latency_waiting)
    }

    WolCard {
        Column(Modifier.padding(start = 14.dp, end = 14.dp, top = 12.dp, bottom = 12.dp)) {
            Row(verticalAlignment = Alignment.Top) {
                Column(Modifier.weight(1f)) {
                    Text(
                        stringResource(R.string.latency_title).uppercase(),
                        style = MaterialTheme.typography.labelSmall.copy(letterSpacing = 0.9.sp, fontWeight = FontWeight.SemiBold),
                        color = WolPalette.Text3,
                    )
                    if (checking) {
                        Spacer(Modifier.height(7.dp))
                        LiveBadge()
                    }
                }
                Column(horizontalAlignment = Alignment.End) {
                    Text(
                        current?.let { msText(it) } ?: none,
                        style = MaterialTheme.typography.headlineSmall.copy(fontSize = 26.sp, lineHeight = 30.sp),
                        color = if (current != null) WolPalette.Text else WolPalette.Text3,
                    )
                    Text(
                        when {
                            current != null -> status.method?.let { stringResource(it.viaLabel()) }.orEmpty()
                            checking -> stringResource(R.string.latency_no_answer)
                            else -> stringResource(R.string.latency_not_measured)
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = WolPalette.Text2,
                    )
                }
            }
            Spacer(Modifier.height(6.dp))
            Text(
                scale?.let { msText(it) }.orEmpty(),
                style = MaterialTheme.typography.labelSmall,
                color = WolPalette.Text3,
            )
            LatencyTrace(
                samples,
                Modifier
                    .fillMaxWidth()
                    .height(112.dp)
                    .semantics { contentDescription = description },
                detailed = true,
            )
            Row(Modifier.fillMaxWidth().padding(top = 2.dp)) {
                AxisLabel(stringResource(R.string.latency_axis_start))
                Spacer(Modifier.weight(1f))
                AxisLabel(stringResource(R.string.latency_axis_end))
            }
            Spacer(Modifier.height(10.dp))
            RowDivider()
            Spacer(Modifier.height(10.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Stat(stringResource(R.string.latency_min), min)
                Stat(stringResource(R.string.latency_avg), average)
                Stat(stringResource(R.string.latency_max), max)
                Stat(
                    stringResource(R.string.latency_lost),
                    stats?.let { stringResource(R.string.latency_lost_value, it.lost, it.count) } ?: none,
                )
            }
        }
    }
}

@Composable
private fun msText(value: Long): String = stringResource(R.string.latency_ms, value)

private fun ProbeMethod.viaLabel(): Int = when (this) {
    ProbeMethod.AGENT -> R.string.latency_via_agent
    ProbeMethod.TCP -> R.string.latency_via_tcp
    ProbeMethod.PING -> R.string.latency_via_ping
}

/** « En direct » : point vert qui bat au rythme des mesures (une par seconde). */
@Composable
private fun LiveBadge() {
    val beat by rememberInfiniteTransition(label = "live").animateFloat(
        initialValue = 0f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(1_000, easing = LinearEasing)),
        label = "liveBeat",
    )
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(
            Modifier
                .size(7.dp)
                .drawBehind {
                    drawCircle(WolPalette.On.copy(alpha = 0.5f * (1 - beat)), radius = size.minDimension / 2 + 7.dp.toPx() * beat)
                }
                .background(WolPalette.On, CircleShape),
        )
        Spacer(Modifier.width(7.dp))
        Text(
            stringResource(R.string.latency_live),
            style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold),
            color = WolPalette.Text2,
        )
    }
}

@Composable
private fun AxisLabel(text: String) {
    Text(text, style = MaterialTheme.typography.labelSmall, color = WolPalette.Text3)
}

@Composable
private fun RowScope.Stat(label: String, value: String) {
    Column(Modifier.weight(1f)) {
        Text(label, style = MaterialTheme.typography.labelSmall, color = WolPalette.Text3, maxLines = 1)
        Text(
            value,
            style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold),
            color = WolPalette.Text,
            maxLines = 1,
        )
    }
}
