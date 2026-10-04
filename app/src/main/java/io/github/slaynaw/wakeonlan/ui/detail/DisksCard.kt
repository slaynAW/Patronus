package io.github.slaynaw.wakeonlan.ui.detail

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.agent.AgentDisks
import io.github.slaynaw.wakeonlan.core.agent.AgentDrive
import io.github.slaynaw.wakeonlan.core.agent.AgentVolume
import io.github.slaynaw.wakeonlan.core.agent.DiskLevel
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.SectionLabel
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.color
import io.github.slaynaw.wakeonlan.ui.common.eventTimeText
import io.github.slaynaw.wakeonlan.ui.common.formatBytes
import io.github.slaynaw.wakeonlan.ui.common.formatCelsius
import io.github.slaynaw.wakeonlan.ui.common.formatPercent
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import java.text.NumberFormat
import java.util.Locale

/** Disques du PC (agent 1.9.0) : espace de chaque lecteur, santé de chaque disque, erreurs d'accès. */
@Composable
internal fun DisksCard(disks: AgentDisks, now: Long) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        SectionLabel(stringResource(R.string.disks_title))
        WolCard {
            var first = true
            for (v in disks.volumes) {
                if (!first) RowDivider()
                first = false
                VolumeRow(v)
            }
            for (d in disks.drives) {
                if (!first) RowDivider()
                first = false
                DriveRow(d)
            }
            if (disks.errors > 0) {
                if (!first) RowDivider()
                Row(Modifier.padding(horizontal = 14.dp, vertical = 11.dp), verticalAlignment = Alignment.Top) {
                    Icon(WolIcons.Warning, contentDescription = null, tint = WolPalette.Busy, modifier = Modifier.size(16.dp).padding(top = 1.dp))
                    Spacer(Modifier.width(8.dp))
                    Column {
                        Text(
                            stringResource(R.string.disk_errors_log, count(disks.errors.toLong()), eventTimeText(disks.lastError * 1000, now)),
                            style = MaterialTheme.typography.bodySmall,
                            color = WolPalette.Busy,
                        )
                        Text(stringResource(R.string.disk_errors_help), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text3)
                    }
                }
            }
        }
    }
}

@Composable
private fun VolumeRow(v: AgentVolume) {
    val level = v.level
    Column(Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 11.dp), verticalArrangement = Arrangement.spacedBy(7.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                listOf(v.mount, v.label).filter { it.isNotBlank() }.joinToString(" "),
                style = MaterialTheme.typography.bodyMedium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            Spacer(Modifier.width(10.dp))
            Text(
                stringResource(R.string.disk_free, formatBytes(v.free), formatBytes(v.total)),
                style = MaterialTheme.typography.bodySmall,
                color = level.color(WolPalette.Text2),
                fontWeight = if (level == DiskLevel.NONE) null else FontWeight.SemiBold,
            )
        }
        Box(Modifier.fillMaxWidth().height(6.dp).clip(RoundedCornerShape(3.dp)).background(WolPalette.Soft)) {
            Box(
                Modifier.fillMaxWidth((v.usedPercent / 100).toFloat().coerceIn(0.01f, 1f)).fillMaxHeight()
                    .clip(RoundedCornerShape(3.dp)).background(level.color(WolPalette.Blue)),
            )
        }
    }
}

@Composable
private fun DriveRow(d: AgentDrive) {
    val level = d.level
    val health = when (d.health) {
        AgentDrive.HEALTHY -> R.string.disk_health_ok
        AgentDrive.WARNING -> R.string.disk_health_warning
        AgentDrive.UNHEALTHY -> R.string.disk_health_bad
        else -> when (level) {
            DiskLevel.BAD -> R.string.disk_health_bad
            DiskLevel.WARN -> R.string.disk_health_warning
            DiskLevel.NONE -> null
        }
    }
    Column(Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 11.dp), verticalArrangement = Arrangement.spacedBy(5.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(d.name, style = MaterialTheme.typography.bodyMedium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
            if (health != null) {
                Spacer(Modifier.width(10.dp))
                val chip = if (level == DiskLevel.NONE) WolPalette.On else level.color(WolPalette.On)
                Text(
                    stringResource(health),
                    style = MaterialTheme.typography.labelSmall,
                    color = chip,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.clip(RoundedCornerShape(50)).background(chip.copy(alpha = 0.15f)).padding(horizontal = 8.dp, vertical = 2.dp),
                )
            }
        }
        val facts = buildList {
            val kind = listOfNotNull(
                when (d.media) {
                    AgentDrive.SSD -> "SSD"
                    AgentDrive.HDD -> "HDD"
                    else -> null
                },
                d.bus.takeIf { it.isNotBlank() },
                d.size.takeIf { it > 0 }?.let { formatBytes(it) },
            ).joinToString(" · ")
            if (kind.isNotEmpty()) add(kind to WolPalette.Text2)
            d.temp?.let { t ->
                add(formatCelsius(t) to when {
                    t >= d.hotTemp + 10 -> WolPalette.DangerText
                    t >= d.hotTemp -> WolPalette.Busy
                    else -> WolPalette.Text2
                })
            }
            d.wear?.let { w ->
                add(stringResource(R.string.disk_wear, formatPercent(w.toDouble())) to when {
                    w >= 95 -> WolPalette.DangerText
                    w >= 80 -> WolPalette.Busy
                    else -> WolPalette.Text2
                })
            }
            d.hours?.let { add(stringResource(R.string.disk_hours, count(it)) to WolPalette.Text2) }
            if (d.uncorrected > 0) add(stringResource(R.string.disk_uncorrected, count(d.uncorrected)) to WolPalette.DangerText)
        }
        if (facts.isNotEmpty()) {
            Text(
                buildAnnotatedString {
                    facts.forEachIndexed { i, (text, color) ->
                        if (i > 0) withStyle(SpanStyle(color = WolPalette.Text3)) { append("  ·  ") }
                        withStyle(SpanStyle(color = color)) { append(text) }
                    }
                },
                style = MaterialTheme.typography.bodySmall,
            )
        }
    }
}

private fun count(n: Long): String = NumberFormat.getIntegerInstance(Locale.FRANCE).format(n)
