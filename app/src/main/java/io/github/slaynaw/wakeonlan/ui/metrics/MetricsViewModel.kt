package io.github.slaynaw.wakeonlan.ui.metrics

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.github.slaynaw.wakeonlan.AppContainer
import io.github.slaynaw.wakeonlan.archive.MetricsRange
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import io.github.slaynaw.wakeonlan.ui.history.HistoryItem
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.LocalDate
import java.time.ZoneId

/** Période affichée : les derniers jours, ou un mois archivé (« 2026-10 »). */
sealed interface MetricsPeriod {
    data class Last(val days: Int) : MetricsPeriod
    data class Month(val month: String) : MetricsPeriod
}

data class MetricsUiState(
    val deviceName: String = "",
    val period: MetricsPeriod = MetricsPeriod.Last(1),
    val months: List<String> = emptyList(),
    val loading: Boolean = true,
    val range: MetricsRange? = null,
    val events: List<HistoryItem> = emptyList(),
    val error: String? = null,
)

/** Mesures d'un PC dans le temps : lues sur son agent et dans les archives GitHub. */
class MetricsViewModel(private val container: AppContainer, private val deviceId: String) : ViewModel() {
    private val _state = MutableStateFlow(MetricsUiState())
    val state: StateFlow<MetricsUiState> = _state.asStateFlow()
    private var job: Job? = null

    init {
        viewModelScope.launch {
            val name = container.allDevices.first().devices.firstOrNull { it.id == deviceId }?.name.orEmpty()
            _state.update { it.copy(deviceName = name) }
        }
        viewModelScope.launch {
            val months = try {
                container.archives.archivedMonths()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                DiagnosticLog.w(AREA, "mois archivés illisibles", e)
                emptyList()
            }
            _state.update { it.copy(months = months) }
        }
        load()
    }

    fun select(period: MetricsPeriod) {
        _state.update { it.copy(period = period) }
        load()
    }

    fun load() {
        job?.cancel()
        val period = _state.value.period
        _state.update { it.copy(loading = true, error = null) }
        job = viewModelScope.launch {
            val now = System.currentTimeMillis()
            val (from, to) = when (period) {
                is MetricsPeriod.Last -> now - period.days * 86_400_000L to now
                is MetricsPeriod.Month -> {
                    val start = LocalDate.parse("${period.month}-01")
                    val zone = ZoneId.systemDefault()
                    start.atStartOfDay(zone).toInstant().toEpochMilli() to
                        minOf(start.plusMonths(1).atStartOfDay(zone).toInstant().toEpochMilli(), now)
                }
            }
            try {
                val range = async { container.archives.range(deviceId, from, to, POINTS) }
                val events = async { container.archives.journal(deviceId, from, to) }
                val name = _state.value.deviceName
                val items = events.await().map { HistoryItem(it, name) }
                _state.update { it.copy(loading = false, range = range.await(), events = items) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                DiagnosticLog.w(AREA, "mesures illisibles", e)
                _state.update { it.copy(loading = false, range = null, events = emptyList(), error = e.message ?: e.javaClass.simpleName) }
            }
        }
    }

    private companion object {
        const val AREA = "mesures"
        const val POINTS = 360
    }
}
