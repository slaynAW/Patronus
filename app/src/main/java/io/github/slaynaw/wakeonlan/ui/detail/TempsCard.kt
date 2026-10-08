package io.github.slaynaw.wakeonlan.ui.detail

import android.animation.ValueAnimator
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.core.status.TempLog
import io.github.slaynaw.wakeonlan.core.status.TempSample
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.formatCelsius
import io.github.slaynaw.wakeonlan.ui.common.rememberTraceClock
import io.github.slaynaw.wakeonlan.ui.common.temperatureColor
import io.github.slaynaw.wakeonlan.ui.devices.DeviceItem
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import kotlin.math.max

/** Durée de l'impulsion qui marque chaque nouveau relevé. */
private const val PULSE_MS = 700L

/**
 * Carte « Températures » de la fiche (PC avec agent) : processeur en bleu, carte graphique en violet,
 * valeurs en direct, tracé des 5 dernières minutes et synthèse. Mêmes règles que l'application Windows.
 */
@Composable
fun TempsCard(item: DeviceItem, now: Long) {
    val status = item.status
    val online = status.state == PowerState.ONLINE
    val temps = status.agent?.temperatures?.takeIf { online }
    val samples = item.temps
    val from = now - TempLog.WINDOW_MS
    val second = now / 1_000
    val cpu = remember(samples, second) { TempLog.stats(samples, from) { it.cpu } }
    val gpu = remember(samples, second) { TempLog.stats(samples, from) { it.gpu } }
    val visible = samples.any { it.time >= from }
    val none = "—"
    val deg = { v: Double? -> v?.let(::formatCelsius) ?: none }
    val empty = when {
        !online -> R.string.temps_offline
        status.agent != null && temps == null -> R.string.temps_none
        else -> R.string.temps_waiting
    }
    val description = if (visible) {
        stringResource(R.string.temps_description, deg(cpu?.average), deg(cpu?.max), deg(gpu?.average), deg(gpu?.max))
    } else {
        stringResource(empty)
    }

    WolCard {
        Column(Modifier.padding(start = 14.dp, end = 14.dp, top = 12.dp, bottom = 12.dp)) {
            Row(verticalAlignment = Alignment.Top) {
                Column(Modifier.weight(1f)) {
                    Text(
                        stringResource(R.string.temps_title).uppercase(),
                        style = MaterialTheme.typography.labelSmall.copy(letterSpacing = 0.9.sp, fontWeight = FontWeight.SemiBold),
                        color = WolPalette.Text3,
                    )
                    if (online && status.agent != null) {
                        Spacer(Modifier.height(7.dp))
                        LiveBadge()
                    }
                }
                CurrentTemp("CPU", WolPalette.Blue, temps?.cpu)
                Spacer(Modifier.width(18.dp))
                // Puce graphique intégrée sans sonde à part : température de la puce, celle du processeur.
                val integrated = temps?.gpuShared == true
                CurrentTemp(if (integrated) stringResource(R.string.temps_gpu_integrated) else "GPU", WolPalette.Gpu, temps?.gpu, dashed = integrated)
            }
            Spacer(Modifier.height(6.dp))
            Box(
                Modifier
                    .fillMaxWidth()
                    .height(128.dp)
                    .semantics { contentDescription = description },
            ) {
                TempTrace(samples, Modifier.fillMaxSize())
                if (!visible) {
                    Text(
                        stringResource(empty),
                        style = MaterialTheme.typography.bodySmall,
                        color = WolPalette.Text3,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.align(Alignment.Center).padding(horizontal = 24.dp),
                    )
                }
            }
            Row(Modifier.fillMaxWidth().padding(top = 2.dp)) {
                AxisLabel(stringResource(R.string.temps_axis_start))
                Spacer(Modifier.weight(1f))
                AxisLabel(stringResource(R.string.latency_axis_end))
            }
            Spacer(Modifier.height(10.dp))
            RowDivider()
            Spacer(Modifier.height(10.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Stat(stringResource(R.string.temps_avg_cpu), deg(cpu?.average))
                Stat(stringResource(R.string.temps_max_cpu), deg(cpu?.max))
                Stat(stringResource(R.string.temps_avg_gpu), deg(gpu?.average))
                Stat(stringResource(R.string.temps_max_gpu), deg(gpu?.max))
            }
        }
    }
}

/** Valeur en direct d'une courbe, avec son repère de couleur ([dashed] : courbe en pointillés). */
@Composable
private fun CurrentTemp(label: String, color: Color, value: Double?, dashed: Boolean = false) {
    Column(horizontalAlignment = Alignment.End) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (dashed) {
                Box(Modifier.size(width = 4.dp, height = 3.dp).background(color, RoundedCornerShape(1.dp)))
                Spacer(Modifier.width(2.dp))
                Box(Modifier.size(width = 4.dp, height = 3.dp).background(color, RoundedCornerShape(1.dp)))
            } else {
                Box(Modifier.size(width = 10.dp, height = 3.dp).background(color, RoundedCornerShape(2.dp)))
            }
            Spacer(Modifier.width(6.dp))
            Text(label, style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold), color = WolPalette.Text2)
        }
        Text(
            value?.let(::formatCelsius) ?: "—",
            style = MaterialTheme.typography.headlineSmall.copy(fontSize = 24.sp, lineHeight = 28.sp, fontWeight = FontWeight.Bold),
            color = if (value != null) temperatureColor(value) else WolPalette.Text3,
            maxLines = 1,
        )
    }
}

/** Échelle (°C) des relevés récents ; [measured] faux sans relevé. */
private data class TempScale(val lo: Double, val hi: Double, val measured: Boolean)

private fun tempScale(samples: List<TempSample>, now: Long): TempScale {
    val values = samples.filter { it.time >= now - TempLog.WINDOW_MS - 30_000 }.flatMap { listOfNotNull(it.cpu, it.gpu) }
    if (values.isEmpty()) return TempScale(20.0, 80.0, false)
    val (lo, hi) = TempLog.range(values.min(), values.max())
    return TempScale(lo, hi, true)
}

/** Intervalle habituel entre deux relevés (médiane des derniers écarts), en ms. */
private fun tempGap(samples: List<TempSample>): Long {
    val gaps = samples.takeLast(7).zipWithNext { a, b -> b.time - a.time }.sorted()
    if (gaps.isEmpty()) return 5_000
    return gaps[gaps.size / 2].coerceIn(1_000, 15_000)
}

/**
 * Tracé des températures : la fenêtre de 5 minutes défile vers la gauche et suit les relevés avec un
 * léger retard (l'intervalle entre deux relevés), pour que chaque courbe glisse d'un relevé au suivant.
 */
@Composable
private fun TempTrace(samples: List<TempSample>, modifier: Modifier = Modifier) {
    val clock = rememberTraceClock()
    val animate = remember { ValueAnimator.areAnimatorsEnabled() }
    val gap = remember(samples) { tempGap(samples) }
    val delayMs by animateFloatAsState(gap + 300f, tween(1_500), label = "tempDelay")
    val scale = remember(samples) { tempScale(samples, System.currentTimeMillis()) }
    val lo by animateFloatAsState(scale.lo.toFloat(), tween(400), label = "tempLo")
    val hi by animateFloatAsState(scale.hi.toFloat(), tween(400), label = "tempHi")
    val measurer = rememberTextMeasurer()
    val labelStyle = MaterialTheme.typography.labelSmall.copy(color = WolPalette.Text3)
    val hiLabel = remember(scale.hi, measurer, labelStyle) { measurer.measure(formatCelsius(scale.hi), labelStyle) }
    val loLabel = remember(scale.lo, measurer, labelStyle) { measurer.measure(formatCelsius(scale.lo), labelStyle) }
    Canvas(modifier.clipToBounds()) {
        val geometry = TempGeometry(
            top = 18.dp.toPx(),
            bottom = size.height - 6.dp.toPx(),
            right = size.width - 8.dp.toPx(),
            rt = clock.value - delayMs.toLong(),
            lo = lo,
            hi = max(lo + 1f, hi),
            gapBreak = max(30_000L, gap * 5 / 2),
        )
        drawGrid(geometry)
        if (scale.measured) drawScale(hiLabel, loLabel, geometry)
        drawSeries(samples, { it.cpu }, WolPalette.Blue, geometry, animate)
        drawSeries(samples, { it.gpu }, WolPalette.Gpu, geometry, animate, dashedWhen = { it.gpuShared })
    }
}

/** Repères du tracé : [rt] instant tracé au bord droit, échelle [lo]..[hi] (°C). */
private class TempGeometry(val top: Float, val bottom: Float, val right: Float, val rt: Long, val lo: Float, val hi: Float, val gapBreak: Long) {
    fun x(t: Long): Float = right - (rt - t).toFloat() / TempLog.WINDOW_MS * right

    fun y(v: Double): Float = bottom - (v.toFloat().coerceIn(lo, hi) - lo) / (hi - lo) * (bottom - top)
}

/** Grille : bas, milieu et haut de l'échelle, un trait par minute. */
private fun DrawScope.drawGrid(g: TempGeometry) {
    val hair = 1.dp.toPx()
    for (f in floatArrayOf(0f, 0.5f, 1f)) {
        val y = g.bottom - f * (g.bottom - g.top)
        drawLine(WolPalette.Line, Offset(0f, y), Offset(size.width, y), hair)
    }
    var t = Math.floorDiv(g.rt - TempLog.WINDOW_MS, 60_000L) * 60_000L + 60_000L
    while (t <= g.rt) {
        val x = g.x(t)
        drawLine(WolPalette.Line, Offset(x, g.top), Offset(x, g.bottom), hair)
        t += 60_000L
    }
}

private fun DrawScope.drawScale(hiLabel: TextLayoutResult, loLabel: TextLayoutResult, g: TempGeometry) {
    drawText(hiLabel, topLeft = Offset(0f, g.top - hiLabel.size.height - 2.dp.toPx()))
    drawText(loLabel, topLeft = Offset(2.dp.toPx(), g.bottom - loLabel.size.height - 2.dp.toPx()))
}

/** Partie continue d'une courbe ; [dashed] : puce graphique intégrée, tracée en pointillés. */
private class Segment(val points: MutableList<Offset>, val dashed: Boolean)

/**
 * Une courbe : seuls les relevés de la série comptent (un relevé manquant est sauté), une longue
 * interruption la coupe. Puce graphique intégrée (même température que le processeur) : en
 * pointillés, pour laisser voir la courbe du processeur dessous.
 */
private fun DrawScope.drawSeries(
    samples: List<TempSample>,
    value: (TempSample) -> Double?,
    color: Color,
    g: TempGeometry,
    animate: Boolean,
    dashedWhen: (TempSample) -> Boolean = { false },
) {
    val segments = mutableListOf<Segment>()
    var segment: Segment? = null
    var previous: TempSample? = null
    var head: Offset? = null
    for (s in samples) {
        val v = value(s) ?: continue
        val p = previous
        if (s.time > g.rt) {
            val open = segment
            val pv = p?.let(value)
            if (open != null && p != null && pv != null && s.time - p.time <= g.gapBreak) {
                val at = pv + (v - pv) * (g.rt - p.time).toDouble() / (s.time - p.time)
                val pen = Offset(g.right, g.y(at))
                open.points.add(pen)
                head = pen
            }
            break
        }
        val point = Offset(g.x(s.time), g.y(v))
        val dashed = dashedWhen(s)
        val open = segment
        segment = when {
            open == null || (p != null && s.time - p.time > g.gapBreak) -> Segment(mutableListOf(point), dashed).also { segments += it }
            // Puce intégrée ↔ carte dédiée : la courbe continue, en pointillés ou non.
            open.dashed != dashed -> Segment(mutableListOf(open.points.last(), point), dashed).also { segments += it }
            else -> open.also { it.points.add(point) }
        }
        previous = s
    }
    val last = previous
    if (head == null && last != null && g.rt - last.time <= g.gapBreak) head = segment?.points?.lastOrNull()

    val lineWidth = 2.dp.toPx()
    val dash = PathEffect.dashPathEffect(floatArrayOf(6.dp.toPx(), 5.dp.toPx()))
    for (seg in segments) {
        val points = seg.points
        if (points.last().x < -4f) continue
        val line = Path().apply {
            moveTo(points.first().x, points.first().y)
            points.drop(1).forEach { lineTo(it.x, it.y) }
            if (points.size == 1) lineTo(points.first().x + 1f, points.first().y)
        }
        if (seg.dashed) {
            drawPath(line, color, style = Stroke(lineWidth, cap = StrokeCap.Butt, join = StrokeJoin.Round, pathEffect = dash))
            continue
        }
        val area = Path().apply {
            moveTo(points.first().x, g.bottom)
            points.forEach { lineTo(it.x, it.y) }
            lineTo(points.last().x, g.bottom)
            close()
        }
        drawPath(area, Brush.verticalGradient(listOf(color.copy(alpha = 0.12f), color.copy(alpha = 0f)), g.top, g.bottom))
        drawPath(line, color.copy(alpha = 0.18f), style = Stroke(lineWidth * 3, cap = StrokeCap.Round, join = StrokeJoin.Round))
        drawPath(line, color, style = Stroke(lineWidth, cap = StrokeCap.Round, join = StrokeJoin.Round))
    }

    // Plume : point au bout de la courbe, avec une impulsion à chaque nouveau relevé.
    head?.let { h ->
        val radius = 4.dp.toPx()
        val age = last?.let { g.rt - it.time } ?: Long.MAX_VALUE
        if (animate && age in 0 until PULSE_MS) {
            val k = age.toFloat() / PULSE_MS
            drawCircle(color.copy(alpha = 0.45f * (1 - k)), radius + 10.dp.toPx() * k, h)
        }
        drawCircle(WolPalette.Surface, radius + 2.dp.toPx(), h)
        drawCircle(color, radius, h)
    }
}
