package io.github.slaynaw.wakeonlan.ui.devices

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.core.status.StatusNotice
import io.github.slaynaw.wakeonlan.ui.common.formatDuration
import io.github.slaynaw.wakeonlan.ui.common.label
import io.github.slaynaw.wakeonlan.ui.common.osLabel
import io.github.slaynaw.wakeonlan.ui.common.rememberNow

@Composable
fun DeviceCard(
    item: DeviceItem,
    isFirst: Boolean,
    isLast: Boolean,
    onClick: () -> Unit,
    onWake: () -> Unit,
    onPower: (PowerAction) -> Unit,
    onMove: (Int) -> Unit,
    onDelete: () -> Unit,
    onClearNotice: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val device = item.device
    val status = item.status
    val now = rememberNow()

    ElevatedCard(onClick = onClick, modifier = modifier.fillMaxWidth()) {
        Column(Modifier.padding(start = 16.dp, top = 12.dp, end = 4.dp, bottom = 12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusIndicator(status.state)
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f)) {
                    Text(
                        device.name,
                        style = MaterialTheme.typography.titleMedium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    Text(
                        statusText(status, now),
                        style = MaterialTheme.typography.bodyMedium,
                        color = statusColor(status.state),
                    )
                }
                DeviceMenu(
                    canShutdown = device.canShutdown,
                    isFirst = isFirst,
                    isLast = isLast,
                    onWake = onWake,
                    onPower = onPower,
                    onEdit = onClick,
                    onMove = onMove,
                    onDelete = onDelete,
                )
            }

            Column(Modifier.padding(start = 26.dp, end = 12.dp, top = 4.dp)) {
                val address = listOf(device.host, device.mac.toString()).filter { it.isNotBlank() }.joinToString(" · ")
                Text(address, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)

                val agent = status.agent
                if (status.state == PowerState.ONLINE && agent != null && status.agentError == null) {
                    Text(
                        stringResource(
                            R.string.agent_info,
                            agent.hostname,
                            agent.osLabel(),
                            formatDuration(agent.uptimeSeconds * 1000),
                        ),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                val agentError = status.agentError
                if (device.agent != null && agentError != null && status.state == PowerState.ONLINE) {
                    Text(
                        stringResource(R.string.agent_problem, stringResource(agentError.label())),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
                status.notice?.let { notice ->
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            stringResource(
                                when (notice) {
                                    StatusNotice.WAKE_TIMEOUT -> R.string.notice_wake_timeout
                                    StatusNotice.SHUTDOWN_TIMEOUT -> R.string.notice_shutdown_timeout
                                },
                            ),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.error,
                            modifier = Modifier.weight(1f),
                        )
                        TextButton(onClick = onClearNotice) { Text(stringResource(R.string.ok)) }
                    }
                }
            }

            Row(
                Modifier.fillMaxWidth().padding(top = 8.dp, end = 12.dp),
                horizontalArrangement = Arrangement.End,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                PrimaryAction(status, device.canShutdown, onWake, onPower)
            }
        }
    }
}

@Composable
private fun PrimaryAction(
    status: DeviceStatus,
    canShutdown: Boolean,
    onWake: () -> Unit,
    onPower: (PowerAction) -> Unit,
) {
    val powerIcon = @Composable {
        Icon(painterResource(R.drawable.ic_power), contentDescription = null, modifier = Modifier.size(18.dp))
        Spacer(Modifier.width(8.dp))
    }
    when (status.state) {
        PowerState.ONLINE -> if (canShutdown) {
            FilledTonalButton(onClick = { onPower(PowerAction.SHUTDOWN) }) {
                powerIcon()
                Text(stringResource(R.string.action_shutdown))
            }
        }
        PowerState.OFFLINE, PowerState.UNKNOWN -> Button(onClick = onWake) {
            powerIcon()
            Text(stringResource(R.string.action_wake))
        }
        PowerState.WAKING -> {
            CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
            Spacer(Modifier.width(12.dp))
            OutlinedButton(onClick = onWake) {
                Icon(Icons.Default.Refresh, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text(stringResource(R.string.action_wake_again))
            }
        }
        PowerState.SHUTTING_DOWN, PowerState.RESTARTING ->
            CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
    }
}

@Composable
private fun DeviceMenu(
    canShutdown: Boolean,
    isFirst: Boolean,
    isLast: Boolean,
    onWake: () -> Unit,
    onPower: (PowerAction) -> Unit,
    onEdit: () -> Unit,
    onMove: (Int) -> Unit,
    onDelete: () -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick = { expanded = true }) {
            Icon(Icons.Default.MoreVert, contentDescription = stringResource(R.string.action_more))
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            fun close(action: () -> Unit): () -> Unit = {
                expanded = false
                action()
            }
            DropdownMenuItem(
                text = { Text(stringResource(R.string.action_wake_menu)) },
                leadingIcon = { Icon(painterResource(R.drawable.ic_power), null, Modifier.size(20.dp)) },
                onClick = close(onWake),
            )
            if (canShutdown) {
                PowerAction.entries.forEach { action ->
                    DropdownMenuItem(
                        text = { Text(stringResource(action.label())) },
                        onClick = close { onPower(action) },
                    )
                }
            }
            HorizontalDivider()
            DropdownMenuItem(
                text = { Text(stringResource(R.string.action_edit)) },
                leadingIcon = { Icon(Icons.Default.Edit, null) },
                onClick = close(onEdit),
            )
            if (!isFirst) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_move_up)) },
                    leadingIcon = { Icon(Icons.Default.KeyboardArrowUp, null) },
                    onClick = close { onMove(-1) },
                )
            }
            if (!isLast) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.action_move_down)) },
                    leadingIcon = { Icon(Icons.Default.KeyboardArrowDown, null) },
                    onClick = close { onMove(1) },
                )
            }
            DropdownMenuItem(
                text = { Text(stringResource(R.string.action_delete)) },
                leadingIcon = { Icon(Icons.Default.Delete, null) },
                onClick = close(onDelete),
            )
        }
    }
}

@Composable
private fun statusText(status: DeviceStatus, now: Long): String {
    val sinceAction = now - (status.actionStartedAt ?: now)
    return when (status.state) {
        PowerState.ONLINE -> {
            val online = stringResource(R.string.state_online)
            status.latencyMs?.let { stringResource(R.string.state_with_detail, online, stringResource(R.string.latency_ms, it)) }
                ?: online
        }
        PowerState.OFFLINE -> status.lastSeen
            ?.let { stringResource(R.string.state_offline_seen, formatDuration(now - it)) }
            ?: stringResource(R.string.state_offline)
        PowerState.WAKING -> stringResource(R.string.state_waking, formatDuration(sinceAction))
        PowerState.SHUTTING_DOWN -> stringResource(R.string.state_shutting_down, formatDuration(sinceAction))
        PowerState.RESTARTING -> stringResource(R.string.state_restarting, formatDuration(sinceAction))
        PowerState.UNKNOWN -> status.unknownReason
            ?.let { stringResource(R.string.state_with_detail, stringResource(R.string.state_unknown), stringResource(it.label())) }
            ?: stringResource(R.string.state_checking)
    }
}
