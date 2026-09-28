package io.github.slaynaw.wakeonlan.ui.history

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.slaynaw.wakeonlan.AgentJournalState
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.core.history.HistoryKind
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.clockText
import io.github.slaynaw.wakeonlan.ui.common.dayKey
import io.github.slaynaw.wakeonlan.ui.common.dayText
import io.github.slaynaw.wakeonlan.ui.common.eventTimeText
import io.github.slaynaw.wakeonlan.ui.common.icon
import io.github.slaynaw.wakeonlan.ui.common.label
import io.github.slaynaw.wakeonlan.ui.common.rememberNow
import io.github.slaynaw.wakeonlan.ui.common.tint
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/** Historique complet des 30 derniers jours, groupé par jour, pour un PC ou pour tous. */
@Composable
fun HistoryScreen(deviceId: String?, onBack: () -> Unit) {
    val container = LocalContext.current.appContainer
    val vm: HistoryViewModel = viewModel(key = "history-screen-${deviceId.orEmpty()}") { HistoryViewModel(container, deviceId) }
    val state by vm.state.collectAsStateWithLifecycle()
    val now = rememberNow(periodMs = 60_000)
    HistoryContent(state, now, onBack = onBack, onRefresh = vm::refresh, onSelect = vm::select)
}

/** Contenu de l'historique (sans état propre : aperçus et captures d'écran). */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HistoryContent(
    state: HistoryUiState,
    now: Long,
    onBack: () -> Unit,
    onRefresh: () -> Unit,
    onSelect: (String?) -> Unit,
) {
    val days = state.events.groupBy { dayKey(it.event.time) }

    Scaffold(
        containerColor = WolPalette.Background,
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text(stringResource(R.string.history_title), style = MaterialTheme.typography.titleLarge)
                        Text(stringResource(R.string.history_subtitle), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                    }
                },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(WolIcons.ArrowBack, contentDescription = stringResource(R.string.back))
                    }
                },
                actions = {
                    IconButton(onClick = onRefresh) {
                        Icon(WolIcons.Refresh, contentDescription = stringResource(R.string.history_refresh), tint = WolPalette.Text2)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = WolPalette.Background,
                    scrolledContainerColor = WolPalette.Surface,
                ),
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
            verticalArrangement = Arrangement.spacedBy(10.dp),
            modifier = Modifier.fillMaxSize(),
        ) {
            if (state.devices.size > 1) {
                item(key = "filters") {
                    Row(
                        Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        FilterOption(stringResource(R.string.history_all_devices), state.filter == null) { onSelect(null) }
                        state.devices.forEach { (id, name) ->
                            FilterOption(name, state.filter == id) { onSelect(id) }
                        }
                    }
                }
            }
            historyNote(state)?.let { note ->
                item(key = "note") { Text(stringResource(note), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text3) }
            }
            if (state.loaded && state.events.isEmpty()) {
                item(key = "empty") {
                    Column(Modifier.fillMaxWidth().padding(top = 48.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                        Box(
                            Modifier.size(56.dp).clip(RoundedCornerShape(16.dp)).background(WolPalette.Surface2),
                            contentAlignment = Alignment.Center,
                        ) {
                            Icon(WolIcons.History, contentDescription = null, tint = WolPalette.Text2, modifier = Modifier.size(28.dp))
                        }
                        Spacer(Modifier.size(14.dp))
                        Text(stringResource(R.string.history_empty), style = MaterialTheme.typography.bodyMedium, color = WolPalette.Text2)
                    }
                }
            }
            days.forEach { (day, events) ->
                item(key = "day-$day") {
                    Text(
                        dayText(events.first().event.time, now).uppercase(),
                        style = MaterialTheme.typography.labelSmall.copy(letterSpacing = 0.9.sp),
                        color = WolPalette.Text3,
                        modifier = Modifier.padding(top = 8.dp, start = 4.dp),
                    )
                }
                item(key = "events-$day") {
                    WolCard {
                        events.forEachIndexed { index, item ->
                            if (index > 0) RowDivider()
                            HistoryRow(item, now, showDevice = state.filter == null, clockOnly = true, detailed = true, modifier = Modifier.padding(horizontal = 14.dp))
                        }
                    }
                }
            }
            if (state.events.isNotEmpty()) {
                item(key = "sources") {
                    Text(
                        stringResource(R.string.history_sources),
                        style = MaterialTheme.typography.bodySmall,
                        color = WolPalette.Text3,
                        modifier = Modifier.padding(top = 6.dp, start = 4.dp, end = 4.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun FilterOption(label: String, selected: Boolean, onClick: () -> Unit) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        label = { Text(label, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        colors = FilterChipDefaults.filterChipColors(
            containerColor = WolPalette.Surface2,
            labelColor = WolPalette.Text2,
            selectedContainerColor = WolPalette.BlueSoft,
            selectedLabelColor = WolPalette.Blue,
        ),
    )
}

/** Remarque sur la complétude de l'historique du PC affiché (sans agent, agent ancien). */
fun historyNote(state: HistoryUiState): Int? = when {
    state.filter == null -> null
    !state.hasAgent -> R.string.history_note_no_agent
    state.agentState == AgentJournalState.OUTDATED -> R.string.history_note_outdated
    else -> null
}

/**
 * Ligne d'historique : icône colorée, libellé (« · par 192.168.1.50 » pour une commande venue d'un
 * autre appareil) et heure (« ≈ » : constatée par le téléphone, à quelques secondes près).
 */
@Composable
fun HistoryRow(
    item: HistoryItem,
    now: Long,
    modifier: Modifier = Modifier,
    showDevice: Boolean = false,
    clockOnly: Boolean = false,
    detailed: Boolean = false,
) {
    val e = item.event
    val time = if (clockOnly) clockText(e.time) else eventTimeText(e.time, now)
    val label = stringResource(e.kind.label())
    val by = e.client?.let { stringResource(R.string.history_by, it) }
    Row(modifier.fillMaxWidth().padding(vertical = 9.dp), verticalAlignment = Alignment.CenterVertically) {
        Box(
            Modifier.size(26.dp).clip(RoundedCornerShape(7.dp)).background(WolPalette.Soft),
            contentAlignment = Alignment.Center,
        ) {
            Icon(e.kind.icon(), contentDescription = null, tint = e.kind.tint(), modifier = Modifier.size(14.dp))
        }
        Spacer(Modifier.width(11.dp))
        Column(Modifier.weight(1f)) {
            Text(
                buildAnnotatedString {
                    append(label)
                    if (by != null) withStyle(SpanStyle(color = WolPalette.Text3, fontSize = 12.sp)) { append(" · $by") }
                },
                style = MaterialTheme.typography.bodyMedium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (showDevice) {
                Text(item.deviceName, style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            if (detailed && e.kind == HistoryKind.LOST) {
                Text(stringResource(R.string.history_lost_help), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text3)
            }
        }
        Spacer(Modifier.width(10.dp))
        Text(
            if (e.approx) stringResource(R.string.history_approx, time) else time,
            style = MaterialTheme.typography.bodySmall,
            color = WolPalette.Text3,
        )
    }
}
