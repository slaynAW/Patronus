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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.agent.AgentMeasure
import io.github.slaynaw.wakeonlan.core.agent.MeasureLevel
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.SectionLabel
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import java.text.NumberFormat
import java.util.Locale

/**
 * Ressources du PC : mesures décrites par l'agent lui-même (mémoire, batterie… ; agent 1.13.0), déjà validées. Une
 * mesure que l'application ne connaît pas s'affiche de la même façon.
 */
@Composable
internal fun MeasuresCard(measures: List<AgentMeasure>) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        SectionLabel(stringResource(R.string.measures_title))
        WolCard {
            measures.forEachIndexed { i, m ->
                if (i > 0) RowDivider()
                MeasureRow(m)
            }
        }
    }
}

@Composable
private fun MeasureRow(m: AgentMeasure) {
    val level = m.level
    Column(Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 11.dp), verticalArrangement = Arrangement.spacedBy(7.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(m.label, style = MaterialTheme.typography.bodyMedium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
            Spacer(Modifier.width(10.dp))
            Text(
                listOf(measureValue(m), m.text).filter { it.isNotEmpty() }.joinToString(" · "),
                style = MaterialTheme.typography.bodySmall,
                color = level.color(WolPalette.Text2),
                fontWeight = if (level == MeasureLevel.NONE) null else FontWeight.SemiBold,
            )
        }
        val min = m.min
        val max = m.max
        if (min != null && max != null) {
            Box(Modifier.fillMaxWidth().height(6.dp).clip(RoundedCornerShape(3.dp)).background(WolPalette.Soft)) {
                Box(
                    Modifier.fillMaxWidth(((m.value - min) / (max - min)).toFloat().coerceIn(0.01f, 1f)).fillMaxHeight()
                        .clip(RoundedCornerShape(3.dp)).background(level.color(WolPalette.Blue)),
                )
            }
        }
    }
}

/** Valeur d'une mesure : entière en %, sinon au dixième ; espace insécable avant l'unité. */
internal fun measureValue(m: AgentMeasure): String {
    val format = NumberFormat.getNumberInstance(Locale.FRANCE).apply { maximumFractionDigits = if (m.unit == "%") 0 else 1 }
    val text = format.format(m.value)
    return if (m.unit.isEmpty()) text else "$text ${m.unit}"
}

private fun MeasureLevel.color(normal: Color): Color = when (this) {
    MeasureLevel.CRIT -> WolPalette.DangerText
    MeasureLevel.WARN -> WolPalette.Busy
    MeasureLevel.NONE -> normal
}
