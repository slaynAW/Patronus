package io.github.slaynaw.wakeonlan.ui.common

import android.animation.ValueAnimator
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.State
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.withFrameMillis
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.core.status.LatencyLog
import io.github.slaynaw.wakeonlan.core.status.LatencySample
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import kotlinx.coroutines.delay
import kotlin.math.max
import kotlin.math.min

/** Période tracée : la dernière minute. */
const val TRACE_WINDOW_MS = 60_000L

/** Durée de l'impulsion qui marque chaque nouvelle mesure. */
private const val PULSE_MS = 700L

/**
 * Heure courante pour les animations de tracé : une trentaine de mises à jour par seconde (une seule
 * si les animations sont désactivées dans les réglages du téléphone). Seul le dessin est relancé.
 */
@Composable
fun rememberTraceClock(): State<Long> {
    val time = remember { mutableLongStateOf(System.currentTimeMillis()) }
    val animate = remember { ValueAnimator.areAnimatorsEnabled() }
    LaunchedEffect(animate) {
        while (true) {
            if (animate) withFrameMillis { } else delay(1_000)
            val now = System.currentTimeMillis()
            if (now - time.longValue >= 30) time.longValue = now
        }
    }
    return time
}

/** Intervalle habituel entre deux mesures (médiane des derniers écarts), en ms. */
private fun expectedGap(samples: List<LatencySample>): Long {
    val gaps = samples.takeLast(7).zipWithNext { a, b -> b.time - a.time }.sorted()
    if (gaps.isEmpty()) return 1_000
    return gaps[gaps.size / 2].coerceIn(800, 6_000)
}

/** Haut de l'échelle pour les mesures de la dernière minute (ms), ou `null` sans mesure. */
fun traceScale(samples: List<LatencySample>, now: Long): Long? {
    val values = samples.filter { it.time >= now - TRACE_WINDOW_MS - 8_000 }.mapNotNull { it.latencyMs }
    return values.maxOrNull()?.let { LatencyLog.scaleMax(it) }
}

/**
 * Tracé de la latence façon électrocardiogramme : la dernière minute défile vers la gauche et suit
 * les mesures avec un léger retard (l'intervalle entre deux mesures), pour que la « plume », au bord
 * droit, glisse d'une mesure à l'autre. [detailed] : grille pour la fiche ; sinon mini-tracé de liste.
 * Mêmes règles de dessin que l'application Windows.
 */
@Composable
fun LatencyTrace(samples: List<LatencySample>, modifier: Modifier = Modifier, detailed: Boolean = false) {
    val clock = rememberTraceClock()
    val animate = remember { ValueAnimator.areAnimatorsEnabled() }
    val gap = remember(samples) { expectedGap(samples) }
    val delayMs by animateFloatAsState(gap + 200f, tween(700), label = "traceDelay")
    val scaleTarget = remember(samples) { (traceScale(samples, System.currentTimeMillis()) ?: LatencyLog.scaleMax(0)).toFloat() }
    val scale by animateFloatAsState(scaleTarget, tween(350), label = "traceScale")
    // Les mesures gardées couvrent plusieurs minutes : ce qui précède la dernière minute tombe à gauche
    // du cadre et ne doit pas déborder sur le reste de la ligne (nom du PC).
    Canvas(modifier.clipToBounds()) {
        drawTrace(samples, clock.value - delayMs.toLong(), scale, gap, detailed, animate)
    }
}

private fun DrawScope.drawTrace(
    samples: List<LatencySample>,
    rt: Long,
    scale: Float,
    gap: Long,
    detailed: Boolean,
    animate: Boolean,
) {
    val top = (if (detailed) 4.dp else 3.dp).toPx()
    val bottom = size.height - (if (detailed) 4.dp else 3.dp).toPx()
    val right = size.width - (if (detailed) 8.dp else 4.dp).toPx()
    val gapBreak = max(4_000L, gap * 5 / 2)
    fun xOf(t: Long) = right - (rt - t).toFloat() / TRACE_WINDOW_MS * right
    fun yOf(v: Float) = bottom - min(v, scale) / scale * (bottom - top)

    if (detailed) {
        // Grille « papier d'électrocardiogramme » : repères horizontaux et un trait toutes les 10 s.
        val hair = 1.dp.toPx()
        for (f in floatArrayOf(0f, 0.5f, 1f)) {
            val y = bottom - f * (bottom - top)
            drawLine(WolPalette.Line, Offset(0f, y), Offset(size.width, y), hair)
        }
        var t = Math.floorDiv(rt - TRACE_WINDOW_MS, 10_000L) * 10_000L + 10_000L
        while (t <= rt) {
            val x = xOf(t)
            drawLine(WolPalette.Line, Offset(x, top), Offset(x, bottom), hair)
            t += 10_000L
        }
    }

    // Segments continus : une sonde sans réponse ou une longue interruption coupe le tracé.
    val segments = mutableListOf<MutableList<Offset>>()
    val lost = mutableListOf<Float>()
    var segment: MutableList<Offset>? = null
    var previous: LatencySample? = null
    var head: Offset? = null
    for (s in samples) {
        val value = s.latencyMs
        val p = previous
        if (s.time > rt) {
            val open = segment
            val pv = p?.latencyMs
            if (open != null && p != null && pv != null && value != null && s.time - p.time <= gapBreak) {
                val v = pv + (value - pv).toFloat() * (rt - p.time) / (s.time - p.time)
                val pen = Offset(right, yOf(v))
                open.add(pen)
                head = pen
            }
            break
        }
        if (value == null) {
            lost += xOf(s.time)
            segment = null
        } else {
            val current = segment?.takeIf { p == null || s.time - p.time <= gapBreak }
                ?: mutableListOf<Offset>().also { segments += it }
            current.add(Offset(xOf(s.time), yOf(value.toFloat())))
            segment = current
        }
        previous = s
    }
    if (head == null && previous?.latencyMs != null) head = segment?.lastOrNull()

    val lineWidth = (if (detailed) 2.dp else 1.5.dp).toPx()
    for (points in segments) {
        if (points.last().x < -4f) continue
        // Voile sous la courbe, lueur de moniteur, puis la courbe elle-même.
        val area = Path().apply {
            moveTo(points.first().x, bottom)
            points.forEach { lineTo(it.x, it.y) }
            lineTo(points.last().x, bottom)
            close()
        }
        drawPath(area, Brush.verticalGradient(listOf(WolPalette.On.copy(alpha = if (detailed) 0.16f else 0.12f), WolPalette.On.copy(alpha = 0f)), top, bottom))
        val line = Path().apply {
            moveTo(points.first().x, points.first().y)
            points.drop(1).forEach { lineTo(it.x, it.y) }
            if (points.size == 1) lineTo(points.first().x - 1f, points.first().y)
        }
        drawPath(line, WolPalette.On.copy(alpha = 0.18f), style = Stroke(lineWidth * 3, cap = StrokeCap.Round, join = StrokeJoin.Round))
        drawPath(line, WolPalette.On, style = Stroke(lineWidth, cap = StrokeCap.Round, join = StrokeJoin.Round))
    }

    // Sondes restées sans réponse : petits traits rouges sur la ligne de base.
    val tick = (if (detailed) 7.dp else 4.dp).toPx()
    val tickWidth = 2.dp.toPx()
    for (x in lost) {
        if (x > -2f) drawRect(WolPalette.Off, Offset(x - tickWidth / 2, bottom - tick), Size(tickWidth, tick))
    }

    // Plume : point au bout du tracé, avec une impulsion à chaque nouvelle mesure.
    head?.let { h ->
        val radius = (if (detailed) 4.dp else 2.5.dp).toPx()
        val age = previous?.let { rt - it.time } ?: Long.MAX_VALUE
        if (animate && age in 0 until PULSE_MS) {
            val k = age.toFloat() / PULSE_MS
            drawCircle(WolPalette.On.copy(alpha = 0.45f * (1 - k)), radius + (if (detailed) 10.dp else 5.dp).toPx() * k, h)
        }
        if (detailed) drawCircle(WolPalette.Surface, radius + 2.dp.toPx(), h)
        drawCircle(WolPalette.On, radius, h)
    }
}
