package io.github.slaynaw.wakeonlan.ui.metrics

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.archive.MetricsPoint
import io.github.slaynaw.wakeonlan.archive.MetricsRange
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.dayKey
import io.github.slaynaw.wakeonlan.ui.common.dayText
import io.github.slaynaw.wakeonlan.ui.common.formatCelsius
import io.github.slaynaw.wakeonlan.ui.common.formatLongDuration
import io.github.slaynaw.wakeonlan.ui.common.formatPercent
import io.github.slaynaw.wakeonlan.ui.common.rememberNow
import io.github.slaynaw.wakeonlan.ui.history.HistoryRow
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import java.time.Instant
import java.time.YearMonth
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale
import kotlin.math.abs
import kotlin.math.ceil
import kotlin.math.floor

/** Couleurs des séries, comme l'application Windows. */
private val CpuColor = WolPalette.Blue
private val GpuColor = Color(0xFFC084FC)

/** Série d'un graphique : moyenne (trait plein) et maximum (trait fin). */
private data class Series(
    val label: String,
    val color: Color,
    val avg: (MetricsPoint) -> Double?,
    val max: (MetricsPoint) -> Double?,
)

/** Mesures d'un PC dans le temps : températures, utilisation et journal. */
@Composable
fun MetricsScreen(deviceId: String, onBack: () -> Unit) {
    val container = LocalContext.current.appContainer
    val vm: MetricsViewModel = viewModel(key = "metrics-$deviceId") { MetricsViewModel(container, deviceId) }
    val state by vm.state.collectAsStateWithLifecycle()
    MetricsContent(state, onBack = onBack, onRefresh = vm::load, onSelect = vm::select)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MetricsContent(state: MetricsUiState, onBack: () -> Unit, onRefresh: () -> Unit, onSelect: (MetricsPeriod) -> Unit) {
    val now = rememberNow(periodMs = 60_000)
    Scaffold(
        containerColor = WolPalette.Background,
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text(stringResource(R.string.metrics_title), style = MaterialTheme.typography.titleLarge)
                        if (state.deviceName.isNotEmpty()) {
                            Text(state.deviceName, style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                        }
                    }
                },
                navigationIcon = {
                    IconButton(onClick = onBack) { Icon(WolIcons.ArrowBack, contentDescription = stringResource(R.string.back)) }
                },
                actions = {
                    IconButton(onClick = onRefresh) {
                        Icon(WolIcons.Refresh, contentDescription = stringResource(R.string.history_refresh), tint = WolPalette.Text2)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = WolPalette.Background, scrolledContainerColor = WolPalette.Surface),
            )
        },
    ) { padding ->
        LazyColumn(
            contentPadding = PaddingValues(
                start = 16.dp,
                end = 16.dp,
                top = padding.calculateTopPadding() + 4.dp,
                bottom = padding.calculateBottomPadding() + 24.dp,
            ),
            verticalArrangement = Arrangement.spacedBy(12.dp),
            modifier = Modifier.fillMaxSize(),
        ) {
            item(key = "periods") { Periods(state, onSelect) }
            val range = state.range
            when {
                state.loading -> item(key = "loading") {
                    Row(Modifier.fillMaxWidth().padding(top = 32.dp), horizontalArrangement = Arrangement.Center, verticalAlignment = Alignment.CenterVertically) {
                        CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp, color = WolPalette.Blue)
                        Spacer(Modifier.width(10.dp))
                        Text(stringResource(R.string.metrics_loading), color = WolPalette.Text2, style = MaterialTheme.typography.bodyMedium)
                    }
                }
                state.error != null -> item(key = "error") {
                    Text(state.error.orEmpty(), color = WolPalette.DangerText, style = MaterialTheme.typography.bodyMedium)
                }
                range != null -> {
                    val notes = buildList<String?> {
                        if (range.minutes > 0) add(null)
                        addAll(range.notes)
                    }
                    if (notes.isNotEmpty()) item(key = "notes") {
                        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            notes.forEach { note ->
                                Text(
                                    note ?: stringResource(R.string.metrics_coverage, formatLongDuration(range.minutes * 60L)),
                                    style = MaterialTheme.typography.bodySmall,
                                    color = WolPalette.Text3,
                                )
                            }
                        }
                    }
                    if (range.points.isEmpty()) {
                        item(key = "empty") {
                            Text(
                                stringResource(R.string.metrics_empty),
                                color = WolPalette.Text2,
                                style = MaterialTheme.typography.bodyMedium,
                                modifier = Modifier.padding(top = 24.dp),
                            )
                        }
                    } else {
                        item(key = "temp") {
                            ChartCard(
                                stringResource(R.string.metrics_temp),
                                range,
                                listOf(
                                    Series("CPU", CpuColor, { it.ct }, { it.ctx }),
                                    Series("GPU", GpuColor, { it.gt }, { it.gtx }),
                                ),
                                percent = false,
                            )
                        }
                        item(key = "load") {
                            ChartCard(
                                stringResource(R.string.metrics_load),
                                range,
                                listOf(
                                    Series("CPU", CpuColor, { it.cl }, { it.clx }),
                                    Series("GPU", GpuColor, { it.gl }, { it.glx }),
                                ),
                                percent = true,
                            )
                        }
                    }
                    item(key = "journal-title") {
                        Text(
                            stringResource(R.string.metrics_journal).uppercase(),
                            style = MaterialTheme.typography.labelSmall.copy(letterSpacing = 0.9.sp),
                            color = WolPalette.Text3,
                            modifier = Modifier.padding(top = 8.dp, start = 4.dp),
                        )
                    }
                    if (state.events.isEmpty()) {
                        item(key = "journal-empty") {
                            Text(stringResource(R.string.metrics_journal_empty), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text3)
                        }
                    }
                    state.events.take(300).groupBy { dayKey(it.event.time) }.forEach { (day, events) ->
                        item(key = "day-$day") {
                            Text(
                                dayText(events.first().event.time, now).uppercase(),
                                style = MaterialTheme.typography.labelSmall.copy(letterSpacing = 0.9.sp),
                                color = WolPalette.Text3,
                                modifier = Modifier.padding(start = 4.dp),
                            )
                        }
                        item(key = "events-$day") {
                            WolCard {
                                events.forEachIndexed { index, item ->
                                    if (index > 0) RowDivider()
                                    HistoryRow(item, now, clockOnly = true, detailed = true, modifier = Modifier.padding(horizontal = 14.dp))
                                }
                            }
                        }
                    }
                    item(key = "sources") {
                        Text(stringResource(R.string.metrics_sources), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text3)
                    }
                }
            }
        }
    }
}

@Composable
private fun Periods(state: MetricsUiState, onSelect: (MetricsPeriod) -> Unit) {
    val monthFormat = remember { DateTimeFormatter.ofPattern("MMMM yyyy", Locale.FRENCH) }
    Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        listOf(1 to R.string.metrics_24h, 7 to R.string.metrics_7d, 30 to R.string.metrics_30d, 90 to R.string.metrics_90d).forEach { (days, label) ->
            PeriodChip(stringResource(label), state.period == MetricsPeriod.Last(days)) { onSelect(MetricsPeriod.Last(days)) }
        }
        state.months.forEach { month ->
            val label = runCatching { YearMonth.parse(month).format(monthFormat) }.getOrDefault(month)
            PeriodChip(label, state.period == MetricsPeriod.Month(month)) { onSelect(MetricsPeriod.Month(month)) }
        }
    }
}

@Composable
private fun PeriodChip(label: String, selected: Boolean, onClick: () -> Unit) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        label = { Text(label, maxLines = 1) },
        colors = FilterChipDefaults.filterChipColors(
            containerColor = WolPalette.Surface2,
            labelColor = WolPalette.Text2,
            selectedContainerColor = WolPalette.BlueSoft,
            selectedLabelColor = WolPalette.Blue,
        ),
    )
}

@Composable
private fun ChartCard(title: String, range: MetricsRange, all: List<Series>, percent: Boolean) {
    val series = all.filter { it.avg(range.summary) != null }
    if (series.isEmpty()) return
    val format: (Double) -> String = if (percent) ::formatPercent else ::formatCelsius
    var selected by remember(range) { mutableStateOf<MetricsPoint?>(null) }
    WolCard {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text(
                title.uppercase(),
                style = MaterialTheme.typography.labelSmall.copy(letterSpacing = 0.9.sp),
                color = WolPalette.Text3,
            )
            MetricsChart(range, series, percent, selected) { selected = it }
            val point = selected
            if (point != null) {
                // Valeurs à l'instant touché.
                Text(
                    pointTime(point.t, range) + series.joinToString("") { s ->
                        s.avg(point)?.let { "  ·  ${s.label} ${format(it)} (max ${s.max(point)?.let(format) ?: "—"})" }.orEmpty()
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = WolPalette.Text2,
                )
            }
            Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                series.forEach { s ->
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(Modifier.size(8.dp).clip(CircleShape).background(s.color))
                        Spacer(Modifier.width(6.dp))
                        Text(s.label, style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.SemiBold)
                        Spacer(Modifier.width(6.dp))
                        Text(
                            stringResource(R.string.metrics_stats, s.avg(range.summary)?.let(format) ?: "—", s.max(range.summary)?.let(format) ?: "—"),
                            style = MaterialTheme.typography.bodySmall,
                            color = WolPalette.Text2,
                        )
                    }
                }
            }
        }
    }
}

private val clockFormat = DateTimeFormatter.ofPattern("HH:mm", Locale.FRENCH)
private val dayFormat = DateTimeFormatter.ofPattern("d MMM", Locale.FRENCH)
private val fullFormat = DateTimeFormatter.ofPattern("EEE d MMM HH:mm", Locale.FRENCH)

private fun isShort(range: MetricsRange) = range.to - range.from <= 36 * 3_600_000L

private fun axisTime(ms: Long, range: MetricsRange): String =
    Instant.ofEpochMilli(ms).atZone(ZoneId.systemDefault()).format(if (isShort(range)) clockFormat else dayFormat)

private fun pointTime(ms: Long, range: MetricsRange): String =
    Instant.ofEpochMilli(ms).atZone(ZoneId.systemDefault()).format(if (isShort(range)) clockFormat else fullFormat)

/**
 * Graphique : une courbe par série (trait plein : moyenne, trait fin : maximum), coupée quand le PC
 * n'a rien enregistré. Toucher ou glisser montre les valeurs d'un instant.
 */
@Composable
private fun MetricsChart(range: MetricsRange, series: List<Series>, percent: Boolean, selected: MetricsPoint?, onSelect: (MetricsPoint?) -> Unit) {
    val points = range.points
    val values = points.flatMap { p -> series.flatMap { listOfNotNull(it.avg(p), it.max(p)) } }
    val (lo, hi) = if (percent || values.isEmpty()) {
        0.0 to 100.0
    } else {
        // Graduations rondes : 4 intervalles de 5, 10, 15 ou 20 °C.
        val low = maxOf(0.0, floor((values.min() - 5) / 10) * 10)
        val span = maxOf(20.0, values.max() + 5 - low)
        low to low + ceil(span / 20) * 20
    }
    val measurer = rememberTextMeasurer()
    val axisStyle = TextStyle(color = WolPalette.Text3, fontSize = 10.sp)
    val gap = range.step * 1000.0 * 2.5
    fun nearest(x: Float, width: Float, left: Float): MetricsPoint? {
        val t = range.from + ((x - left) / (width - left)).toDouble() * (range.to - range.from)
        val best = points.minByOrNull { abs(it.t - t) } ?: return null
        return best.takeIf { abs(best.t - t) <= gap }
    }
    Canvas(
        Modifier
            .fillMaxWidth()
            .height(170.dp)
            .pointerInput(range) { detectTapGestures { onSelect(nearest(it.x, size.width.toFloat(), 34.dp.toPx())) } }
            .pointerInput(range) {
                detectHorizontalDragGestures { change, _ -> onSelect(nearest(change.position.x, size.width.toFloat(), 34.dp.toPx())) }
            },
    ) {
        val left = 34.dp.toPx()
        val right = 4.dp.toPx()
        val top = 6.dp.toPx()
        val bottom = 18.dp.toPx()
        val w = size.width - left - right
        val h = size.height - top - bottom
        fun x(t: Long) = left + ((t - range.from).toFloat() / (range.to - range.from)) * w
        fun y(v: Double) = top + (1 - ((v.coerceIn(lo, hi) - lo) / (hi - lo)).toFloat()) * h
        for (i in 0..4) {
            val v = lo + (hi - lo) * i / 4
            drawLine(WolPalette.Line, Offset(left, y(v)), Offset(left + w, y(v)), strokeWidth = 1f)
            val label = if (percent) "${v.toInt()} %" else "${v.toInt()}°"
            val layout = measurer.measure(label, axisStyle)
            drawText(layout, topLeft = Offset(left - layout.size.width - 4.dp.toPx(), y(v) - layout.size.height / 2f))
        }
        for (i in 0..4) {
            val t = range.from + (range.to - range.from) * i / 4
            val layout = measurer.measure(axisTime(t, range), axisStyle)
            val tx = when (i) {
                0 -> left
                4 -> left + w - layout.size.width
                else -> x(t) - layout.size.width / 2f
            }
            drawText(layout, topLeft = Offset(tx, size.height - layout.size.height))
        }
        for (s in series) {
            for ((value, thin) in listOf(s.max to true, s.avg to false)) {
                val path = Path()
                var prev: Long? = null
                for (p in points) {
                    val v = value(p)
                    if (v == null) {
                        prev = null
                        continue
                    }
                    val px = x(p.t)
                    val py = y(v)
                    if (prev == null || (p.t - prev).toDouble() > gap) path.moveTo(px, py) else path.lineTo(px, py)
                    prev = p.t
                }
                drawPath(
                    path,
                    color = if (thin) s.color.copy(alpha = 0.4f) else s.color,
                    style = Stroke(width = if (thin) 1.dp.toPx() else 1.8.dp.toPx(), cap = StrokeCap.Round, join = StrokeJoin.Round),
                )
            }
        }
        selected?.let { p ->
            drawLine(
                WolPalette.Text3,
                Offset(x(p.t), top),
                Offset(x(p.t), top + h),
                strokeWidth = 1.dp.toPx(),
                pathEffect = PathEffect.dashPathEffect(floatArrayOf(6f, 6f)),
            )
        }
    }
}
