package io.github.slaynaw.wakeonlan.ui.devices

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
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.core.status.StatusNotice
import io.github.slaynaw.wakeonlan.ui.common.BusySpinner
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.common.DeviceGlyph
import io.github.slaynaw.wakeonlan.ui.common.LatencyTrace
import io.github.slaynaw.wakeonlan.ui.common.NoticeBox
import io.github.slaynaw.wakeonlan.ui.common.RoundIconButton
import io.github.slaynaw.wakeonlan.ui.common.StatusDot
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.formatLongDuration
import io.github.slaynaw.wakeonlan.ui.common.icon
import io.github.slaynaw.wakeonlan.ui.common.label
import io.github.slaynaw.wakeonlan.ui.common.rememberNow
import io.github.slaynaw.wakeonlan.ui.common.statusText
import io.github.slaynaw.wakeonlan.ui.common.statusTextColor
import io.github.slaynaw.wakeonlan.ui.common.systemLabel
import io.github.slaynaw.wakeonlan.ui.overview.EmptyState
import io.github.slaynaw.wakeonlan.ui.overview.NetworkStatus
import io.github.slaynaw.wakeonlan.ui.overview.ScreenHeader
import io.github.slaynaw.wakeonlan.ui.theme.MonoStyle
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/** Onglet « Appareils » : fiche résumée de chaque PC, avec ses actions. */
@Composable
fun DevicesTab(
    state: DevicesUiState,
    contentPadding: PaddingValues,
    onOpenDevice: (String) -> Unit,
    onAddDevice: () -> Unit,
    onEditDevice: (String) -> Unit,
    onOpenHistory: (String) -> Unit,
    onWake: (Device) -> Unit,
    onPower: (Device, PowerAction) -> Unit,
    onMove: (Device, Int) -> Unit,
    onDelete: (Device) -> Unit,
    onClearNotice: (Device) -> Unit,
    onRequestPermission: () -> Unit,
) {
    val now = rememberNow()
    LazyColumn(
        contentPadding = PaddingValues(
            start = 16.dp,
            end = 16.dp,
            top = contentPadding.calculateTopPadding() + 12.dp,
            bottom = contentPadding.calculateBottomPadding() + 96.dp,
        ),
        verticalArrangement = Arrangement.spacedBy(12.dp),
        modifier = Modifier.fillMaxSize(),
    ) {
        item(key = "header") {
            ScreenHeader(
                title = stringResource(R.string.tab_devices),
                subtitle = if (state.items.isEmpty()) null else stringResource(
                    R.string.devices_summary,
                    state.items.size,
                    state.settings.pollIntervalSeconds,
                ),
            ) {
                RoundIconButton(WolIcons.Plus, stringResource(R.string.action_add_device), onAddDevice)
            }
        }
        item(key = "network") { NetworkStatus(state, onRequestPermission) }
        if (state.loaded && state.items.isEmpty()) {
            item(key = "empty") { EmptyState(onAddDevice) }
        }
        // Les PC reçus d'autres personnes suivent ceux du téléphone et ne se déplacent pas.
        val ownCount = state.items.count { it.editable }
        itemsIndexed(state.items, key = { _, item -> item.device.id }) { index, item ->
            DeviceCard(
                item = item,
                now = now,
                isFirst = index == 0,
                isLast = index >= ownCount - 1,
                onOpen = { onOpenDevice(item.device.id) },
                onWake = { onWake(item.device) },
                onPower = { action -> onPower(item.device, action) },
                onEdit = { onEditDevice(item.device.id) },
                onHistory = { onOpenHistory(item.device.id) },
                onMove = { offset -> onMove(item.device, offset) },
                onDelete = { onDelete(item.device) },
                onClearNotice = { onClearNotice(item.device) },
                modifier = Modifier.animateItem(),
            )
        }
    }
}

@Composable
private fun DeviceCard(
    item: DeviceItem,
    now: Long,
    isFirst: Boolean,
    isLast: Boolean,
    onOpen: () -> Unit,
    onWake: () -> Unit,
    onPower: (PowerAction) -> Unit,
    onEdit: () -> Unit,
    onHistory: () -> Unit,
    onMove: (Int) -> Unit,
    onDelete: () -> Unit,
    onClearNotice: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val device = item.device
    val status = item.status
    WolCard(modifier, onClick = onOpen) {
        Row(Modifier.padding(start = 14.dp, top = 14.dp, end = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            DeviceGlyph(status.state)
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text(device.name, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Row(verticalAlignment = Alignment.CenterVertically) {
                    StatusDot(status.state, size = 7.dp)
                    Spacer(Modifier.width(7.dp))
                    Text(
                        statusText(status, now),
                        style = MaterialTheme.typography.bodySmall,
                        color = statusTextColor(status.state),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            if (status.state == PowerState.ONLINE && item.latency.isNotEmpty()) {
                Spacer(Modifier.width(8.dp))
                // Tracé du milieu de la ligne jusqu'au menu ; le nom occupe l'autre moitié.
                LatencyTrace(item.latency, Modifier.weight(1f).height(24.dp))
            }
            DeviceMenu(
                canShutdown = device.canShutdown,
                editable = item.editable,
                isFirst = isFirst,
                isLast = isLast,
                onWake = onWake,
                onPower = onPower,
                onEdit = onEdit,
                onHistory = onHistory,
                onMove = onMove,
                onDelete = onDelete,
            )
        }

        Column(Modifier.padding(start = 14.dp, end = 14.dp, top = 10.dp, bottom = 14.dp)) {
            val address = listOf(device.host, device.mac.toString()).filter { it.isNotBlank() }.joinToString("  ·  ")
            Text(address, style = MonoStyle, color = WolPalette.Text2, maxLines = 1, overflow = TextOverflow.Ellipsis)
            item.sharedBy?.let { owner ->
                Row(Modifier.padding(top = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                    Icon(WolIcons.Share, contentDescription = null, tint = WolPalette.Blue, modifier = Modifier.size(14.dp))
                    Spacer(Modifier.width(6.dp))
                    Text(stringResource(R.string.share_by, owner), style = MaterialTheme.typography.bodySmall, color = WolPalette.Blue)
                }
            }

            val agent = status.agent
            if (status.state == PowerState.ONLINE && agent != null && status.agentError == null) {
                Text(
                    stringResource(R.string.agent_info, agent.hostname, agent.systemLabel(), formatLongDuration(agent.uptimeSeconds)),
                    style = MaterialTheme.typography.bodySmall,
                    color = WolPalette.Text2,
                    modifier = Modifier.padding(top = 2.dp),
                )
            }
            val agentError = status.agentError
            if (device.agent != null && agentError != null && status.state == PowerState.ONLINE) {
                Text(
                    stringResource(R.string.agent_problem, stringResource(agentError.label())),
                    style = MaterialTheme.typography.bodySmall,
                    color = WolPalette.DangerText,
                    modifier = Modifier.padding(top = 2.dp),
                )
            }
            status.notice?.let { notice ->
                NoticeBox(
                    text = stringResource(
                        when (notice) {
                            StatusNotice.WAKE_TIMEOUT -> R.string.notice_wake_timeout
                            StatusNotice.SHUTDOWN_TIMEOUT -> R.string.notice_shutdown_timeout
                        },
                    ),
                    onDismiss = onClearNotice,
                    modifier = Modifier.padding(top = 10.dp),
                )
            }
            Row(
                Modifier.fillMaxWidth().padding(top = 12.dp),
                horizontalArrangement = Arrangement.End,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                when (status.state) {
                    PowerState.ONLINE -> if (device.canShutdown) {
                        WolButton(
                            stringResource(R.string.action_shutdown),
                            onClick = { onPower(PowerAction.SHUTDOWN) },
                            kind = ButtonKind.DANGER,
                            icon = WolIcons.Power,
                            height = 40.dp,
                        )
                    }
                    PowerState.OFFLINE, PowerState.UNKNOWN -> WolButton(
                        stringResource(R.string.action_wake),
                        onClick = onWake,
                        kind = ButtonKind.PRIMARY,
                        icon = WolIcons.Power,
                        height = 40.dp,
                    )
                    PowerState.WAKING -> {
                        BusySpinner()
                        Spacer(Modifier.width(12.dp))
                        WolButton(stringResource(R.string.action_wake_again), onClick = onWake, icon = WolIcons.Refresh, height = 40.dp)
                    }
                    PowerState.SHUTTING_DOWN, PowerState.RESTARTING -> Box(Modifier.size(40.dp), contentAlignment = Alignment.Center) {
                        BusySpinner()
                    }
                }
            }
        }
    }
}

/** Menu « ⋯ » d'un PC : toutes les actions, l'édition, l'historique et l'ordre de la liste. */
@Composable
fun DeviceMenu(
    canShutdown: Boolean,
    editable: Boolean,
    isFirst: Boolean,
    isLast: Boolean,
    onWake: () -> Unit,
    onPower: (PowerAction) -> Unit,
    onEdit: () -> Unit,
    onHistory: () -> Unit,
    onMove: (Int) -> Unit,
    onDelete: () -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick = { expanded = true }) {
            Icon(WolIcons.More, contentDescription = stringResource(R.string.action_more), tint = WolPalette.Text2)
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            fun close(action: () -> Unit): () -> Unit = {
                expanded = false
                action()
            }
            MenuItem(stringResource(R.string.action_wake_menu), WolIcons.Power, close(onWake))
            if (canShutdown) {
                PowerAction.entries.forEach { action ->
                    MenuItem(stringResource(action.label()), action.icon(), close { onPower(action) })
                }
            }
            HorizontalDivider(color = WolPalette.Line)
            // Un PC reçu d'une autre personne ne se modifie pas, ne se déplace pas, ne se supprime pas.
            if (editable) MenuItem(stringResource(R.string.action_edit), WolIcons.Edit, close(onEdit))
            MenuItem(stringResource(R.string.history_title), WolIcons.History, close(onHistory))
            if (editable) {
                if (!isFirst) MenuItem(stringResource(R.string.action_move_up), WolIcons.ArrowUp, close { onMove(-1) })
                if (!isLast) MenuItem(stringResource(R.string.action_move_down), WolIcons.ArrowDown, close { onMove(1) })
                HorizontalDivider(color = WolPalette.Line)
                MenuItem(stringResource(R.string.action_delete), WolIcons.Delete, close(onDelete), danger = true)
            }
        }
    }
}

@Composable
private fun MenuItem(text: String, icon: ImageVector, onClick: () -> Unit, danger: Boolean = false) {
    val color = if (danger) WolPalette.DangerText else WolPalette.Text
    DropdownMenuItem(
        text = { Text(text, color = color) },
        leadingIcon = { Icon(icon, contentDescription = null, tint = if (danger) color else WolPalette.Text2, modifier = Modifier.size(19.dp)) },
        onClick = onClick,
    )
}
