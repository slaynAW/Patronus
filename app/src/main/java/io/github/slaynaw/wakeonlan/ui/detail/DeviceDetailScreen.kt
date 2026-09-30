package io.github.slaynaw.wakeonlan.ui.detail

import androidx.compose.foundation.background
import androidx.compose.foundation.border
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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.core.agent.AgentTemperatures
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.core.status.StatusNotice
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.common.DeviceDialogs
import io.github.slaynaw.wakeonlan.ui.common.KeyValueRow
import io.github.slaynaw.wakeonlan.ui.common.NoticeBox
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.SectionLabel
import io.github.slaynaw.wakeonlan.ui.common.StatusDot
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.formatCelsius
import io.github.slaynaw.wakeonlan.ui.common.formatLongDuration
import io.github.slaynaw.wakeonlan.ui.common.label
import io.github.slaynaw.wakeonlan.ui.common.rememberDeviceDialogState
import io.github.slaynaw.wakeonlan.ui.common.rememberNow
import io.github.slaynaw.wakeonlan.ui.common.statusColor
import io.github.slaynaw.wakeonlan.ui.common.statusText
import io.github.slaynaw.wakeonlan.ui.common.statusTextColor
import io.github.slaynaw.wakeonlan.ui.common.systemLabel
import io.github.slaynaw.wakeonlan.ui.common.temperatureColor
import io.github.slaynaw.wakeonlan.ui.common.versionLabel
import io.github.slaynaw.wakeonlan.ui.devices.DeviceItem
import io.github.slaynaw.wakeonlan.ui.devices.DeviceMenu
import io.github.slaynaw.wakeonlan.ui.devices.DevicesViewModel
import io.github.slaynaw.wakeonlan.ui.devices.ResArg
import io.github.slaynaw.wakeonlan.ui.history.HistoryRow
import io.github.slaynaw.wakeonlan.ui.history.HistoryUiState
import io.github.slaynaw.wakeonlan.ui.history.HistoryViewModel
import io.github.slaynaw.wakeonlan.ui.history.historyNote
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/** Nombre d'évènements de l'historique discret de la fiche. */
private const val RECENT_EVENTS = 6

/** Fiche d'un PC : état, actions, informations et historique récent. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DeviceDetailScreen(
    deviceId: String,
    onBack: () -> Unit,
    onEdit: (String) -> Unit,
    onOpenHistory: (String) -> Unit,
) {
    val context = LocalContext.current
    val resources = LocalResources.current
    val container = context.appContainer
    val vm: DevicesViewModel = viewModel { DevicesViewModel(container) }
    val historyVm: HistoryViewModel = viewModel(key = "history-$deviceId") { HistoryViewModel(container, deviceId) }
    val state by vm.state.collectAsStateWithLifecycle()
    val history by historyVm.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val dialogs = rememberDeviceDialogState()
    val now = rememberNow()
    val item = state.items.firstOrNull { it.device.id == deviceId }
    val index = state.items.indexOfFirst { it.device.id == deviceId }
    val ownCount = state.items.count { it.editable }

    // PC affiché : sondé chaque seconde pour le tracé de latence en direct.
    DisposableEffect(container, deviceId) {
        container.statusMonitor.watch(deviceId)
        onDispose { container.statusMonitor.unwatch(deviceId) }
    }

    // PC supprimé (ici ou depuis sa fiche d'édition) : retour à la liste.
    LaunchedEffect(state.loaded, item == null) {
        if (state.loaded && item == null) onBack()
    }
    LaunchedEffect(vm) {
        vm.messages.collect { message ->
            val args = message.args.map { if (it is ResArg) resources.getString(it.id) else it }.toTypedArray()
            snackbar.showSnackbar(resources.getString(message.text, *args))
        }
    }

    val requestPower = { action: PowerAction ->
        item?.let { dialogs.requestPower(it.device, action, state.settings.confirmPowerActions, vm::power) }
    }

    DeviceDetailContent(
        item = item,
        isFirst = index == 0,
        isLast = index >= ownCount - 1,
        history = history,
        now = now,
        snackbar = snackbar,
        onBack = onBack,
        onWake = { item?.let { vm.wake(it.device) } },
        onPower = { requestPower(it) },
        onEdit = { onEdit(deviceId) },
        onOpenHistory = { onOpenHistory(deviceId) },
        onMove = { offset -> item?.let { vm.move(it.device, offset) } },
        onDelete = { item?.let { dialogs.pendingDelete = it.device } },
        onClearNotice = { item?.let { vm.clearNotice(it.device) } },
    )

    DeviceDialogs(dialogs, onPower = vm::power, onDelete = vm::delete, onConfigure = onEdit)
}

/** Contenu de la fiche (sans état propre : aperçus et captures d'écran). */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DeviceDetailContent(
    item: DeviceItem?,
    isFirst: Boolean,
    isLast: Boolean,
    history: HistoryUiState,
    now: Long,
    snackbar: SnackbarHostState,
    onBack: () -> Unit,
    onWake: () -> Unit,
    onPower: (PowerAction) -> Unit,
    onEdit: () -> Unit,
    onOpenHistory: () -> Unit,
    onMove: (Int) -> Unit,
    onDelete: () -> Unit,
    onClearNotice: () -> Unit,
) {
    Scaffold(
        containerColor = WolPalette.Background,
        topBar = {
            TopAppBar(
                title = {},
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(WolIcons.ArrowBack, contentDescription = stringResource(R.string.back))
                    }
                },
                actions = {
                    if (item != null) {
                        DeviceMenu(
                            canShutdown = item.device.canShutdown,
                            editable = item.editable,
                            isFirst = isFirst,
                            isLast = isLast,
                            onWake = onWake,
                            onPower = onPower,
                            onEdit = onEdit,
                            onHistory = onOpenHistory,
                            onMove = onMove,
                            onDelete = onDelete,
                        )
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = WolPalette.Background),
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        if (item == null) return@Scaffold
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Hero(item, now)
            item.status.notice?.let { notice ->
                NoticeBox(
                    text = stringResource(
                        when (notice) {
                            StatusNotice.WAKE_TIMEOUT -> R.string.notice_wake_timeout
                            StatusNotice.SHUTDOWN_TIMEOUT -> R.string.notice_shutdown_timeout
                        },
                    ),
                    onDismiss = onClearNotice,
                )
            }
            Actions(item = item, onWake = onWake, onPower = onPower, onEdit = onEdit.takeIf { item.editable })
            val sharedBy = item.sharedBy
            if (sharedBy != null) {
                Text(stringResource(R.string.hint_shared, sharedBy), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
            } else if (item.status.state == PowerState.ONLINE && !item.device.canShutdown) {
                Text(stringResource(R.string.hint_no_agent), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
            } else if (item.status.state == PowerState.OFFLINE && item.device.agent?.hasKey == true) {
                // Allumé mais sans réponse : le plus souvent un réseau « Public » sous Windows.
                Text(stringResource(R.string.hint_offline_agent), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
            }
            if (item.device.hasHost) LatencyCard(item, now)
            InfoCard(item)
            RecentHistory(history, now, onShowAll = onOpenHistory)
            Spacer(Modifier.height(24.dp))
        }
    }
}

/** Anneau lumineux, nom et état du PC. */
@Composable
private fun Hero(item: DeviceItem, now: Long) {
    val state = item.status.state
    val ring = statusColor(state)
    Column(Modifier.fillMaxWidth().padding(top = 4.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        Box(
            Modifier
                .size(104.dp)
                .drawBehind {
                    // Halo coloré autour de l'anneau.
                    drawCircle(ring.copy(alpha = 0.16f), radius = size.minDimension / 2)
                }
                .padding(4.dp)
                .border(3.dp, ring, CircleShape)
                .padding(6.dp)
                .background(WolPalette.Soft, CircleShape),
            contentAlignment = Alignment.Center,
        ) {
            Icon(WolIcons.Monitor, contentDescription = null, tint = WolPalette.Blue, modifier = Modifier.size(42.dp))
        }
        Spacer(Modifier.height(14.dp))
        Text(item.device.name, style = MaterialTheme.typography.headlineSmall, textAlign = TextAlign.Center)
        Spacer(Modifier.height(4.dp))
        Row(verticalAlignment = Alignment.CenterVertically) {
            StatusDot(state)
            Spacer(Modifier.width(8.dp))
            Text(statusText(item.status, now), style = MaterialTheme.typography.titleSmall, color = statusTextColor(state))
        }
    }
}

/** Actions selon l'état : éteindre / redémarrer / veille, ou démarrer. [onEdit] nul : PC reçu (non modifiable). */
@Composable
private fun Actions(item: DeviceItem, onWake: () -> Unit, onPower: (PowerAction) -> Unit, onEdit: (() -> Unit)?) {
    val state = item.status.state
    val edit = stringResource(R.string.action_edit)
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        when {
            state == PowerState.ONLINE && item.device.canShutdown -> {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    WolButton(stringResource(R.string.action_shutdown), { onPower(PowerAction.SHUTDOWN) }, Modifier.weight(1f), ButtonKind.DANGER, WolIcons.Power)
                    WolButton(stringResource(R.string.action_reboot), { onPower(PowerAction.REBOOT) }, Modifier.weight(1f), icon = WolIcons.Restart)
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    WolButton(stringResource(R.string.action_sleep_short), { onPower(PowerAction.SLEEP) }, Modifier.weight(1f), icon = WolIcons.Moon)
                    if (onEdit != null) WolButton(edit, onEdit, Modifier.weight(1f), icon = WolIcons.Edit)
                }
            }
            state == PowerState.ONLINE -> if (onEdit != null) WolButton(edit, onEdit, Modifier.fillMaxWidth(), icon = WolIcons.Edit)
            state == PowerState.WAKING -> Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                WolButton(stringResource(R.string.action_wake_again), onWake, Modifier.weight(1f), icon = WolIcons.Refresh)
                if (onEdit != null) WolButton(edit, onEdit, Modifier.weight(1f), icon = WolIcons.Edit)
            }
            state.isTransitional -> if (onEdit != null) WolButton(edit, onEdit, Modifier.fillMaxWidth(), icon = WolIcons.Edit)
            else -> {
                WolButton(stringResource(R.string.action_wake), onWake, Modifier.fillMaxWidth(), ButtonKind.PRIMARY, WolIcons.Power, height = 48.dp)
                if (onEdit != null) WolButton(edit, onEdit, Modifier.fillMaxWidth(), icon = WolIcons.Edit)
            }
        }
    }
}

@Composable
private fun InfoCard(item: DeviceItem) {
    val device = item.device
    val status = item.status
    val online = status.state == PowerState.ONLINE
    val agent = status.agent
    WolCard {
        KeyValueRow(
            stringResource(R.string.detail_ip),
            device.host.ifBlank { stringResource(R.string.detail_not_set) },
            mono = device.host.isNotBlank(),
            valueColor = if (device.host.isBlank()) WolPalette.Text2 else WolPalette.Text,
        )
        RowDivider()
        KeyValueRow(stringResource(R.string.detail_mac), device.mac.toString(), mono = true)
        if (online && agent != null) {
            if (agent.hostname.isNotBlank()) {
                RowDivider()
                KeyValueRow(stringResource(R.string.detail_hostname), agent.hostname)
            }
            RowDivider()
            KeyValueRow(stringResource(R.string.detail_system), agent.systemLabel())
            RowDivider()
            KeyValueRow(stringResource(R.string.detail_uptime), formatLongDuration(agent.uptimeSeconds))
            agent.temperatures?.let { temps ->
                val cpu = temps.cpu
                when {
                    cpu != null -> {
                        RowDivider()
                        KeyValueRow(stringResource(R.string.detail_temp_cpu), formatCelsius(cpu), valueColor = temperatureColor(cpu))
                    }
                    temps.cpuHint == AgentTemperatures.CPU_HINT_LHM -> {
                        RowDivider()
                        KeyValueRow(stringResource(R.string.detail_temp_cpu), stringResource(R.string.temperature_cpu_lhm), valueColor = WolPalette.Text2)
                    }
                }
                temps.gpu?.let { gpu ->
                    RowDivider()
                    KeyValueRow(stringResource(R.string.detail_temp_gpu), formatCelsius(gpu), valueColor = temperatureColor(gpu))
                }
            }
        }
        item.sharedBy?.let { owner ->
            RowDivider()
            KeyValueRow(stringResource(R.string.detail_shared), owner, valueColor = WolPalette.Blue, icon = WolIcons.Share)
        }
        val agentError = status.agentError
        if (device.agent != null || item.sharedBy == null) RowDivider()
        when {
            // PC reçu en « démarrer seulement » : l'agent n'est pas concerné.
            device.agent == null && item.sharedBy != null -> Unit
            device.agent == null -> KeyValueRow(stringResource(R.string.detail_agent), stringResource(R.string.agent_not_configured), valueColor = WolPalette.Text2)
            online && agentError != null -> KeyValueRow(
                stringResource(R.string.detail_agent),
                stringResource(agentError.label()),
                valueColor = WolPalette.DangerText,
                bold = true,
            )
            online && agent != null -> KeyValueRow(
                stringResource(R.string.detail_agent),
                stringResource(R.string.agent_authenticated, agent.versionLabel()),
                valueColor = WolPalette.On,
                icon = WolIcons.Shield,
                bold = true,
            )
            else -> KeyValueRow(stringResource(R.string.detail_agent), stringResource(R.string.agent_configured), valueColor = WolPalette.Text2)
        }
    }
}

/** Historique discret : les derniers évènements du PC. */
@Composable
private fun RecentHistory(state: HistoryUiState, now: Long, onShowAll: () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        SectionLabel(
            stringResource(R.string.history_title),
            action = if (state.events.isNotEmpty()) stringResource(R.string.history_show_all) else null,
            onAction = onShowAll,
        )
        if (state.loaded && state.events.isEmpty()) {
            Text(stringResource(R.string.history_empty), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text3)
        }
        state.events.take(RECENT_EVENTS).forEachIndexed { index, event ->
            if (index > 0) RowDivider()
            HistoryRow(event, now)
        }
        historyNote(state)?.let { note ->
            Text(
                stringResource(note),
                style = MaterialTheme.typography.bodySmall,
                color = WolPalette.Text3,
                modifier = Modifier.padding(top = 6.dp),
            )
        }
    }
}
