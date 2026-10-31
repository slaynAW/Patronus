package io.github.slaynaw.wakeonlan.ui.hub

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2
import io.github.slaynaw.wakeonlan.core.hub.EntrustResult
import io.github.slaynaw.wakeonlan.core.hub.HubPcState
import io.github.slaynaw.wakeonlan.core.hub.HubState
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.label
import io.github.slaynaw.wakeonlan.ui.devices.DeviceItem
import io.github.slaynaw.wakeonlan.ui.edit.roleLabel
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/**
 * Ligne « Hub » de la fiche d'un PC : PC confié au hub [hubName] (rôle du hub sur le PC, état vu par le hub), ou
 * proposition de le lui confier ([entrusted] faux).
 */
data class DeviceHubRow(val hubName: String, val entrusted: Boolean, val role: String = "", val state: HubPcState? = null)

/**
 * Ligne « Hub » d'un PC de ce téléphone, avec agent wolagent/2, quand un hub est appairé : son état s'il est confié au
 * hub ; sinon « Confier au hub » quand ce téléphone administre à la fois le PC (allumé) et le hub. null : rien à montrer
 * (hub pas encore relevé, PC partagé par quelqu'un d'autre…).
 */
fun deviceHubRow(item: DeviceItem, hub: HubState): DeviceHubRow? {
    val paired = hub.hub ?: return null
    if (!item.editable || item.device.agent?.pin.isNullOrEmpty() || hub.snapshot == null) return null
    hub.pcOf(item.device)?.let { (pc, state) -> return DeviceHubRow(paired.name, entrusted = true, role = pc.role, state = state) }
    val agent = item.status.agent
    val adminOfPc = item.status.state == PowerState.ONLINE && agent?.proto == Wolagent2.ALPN && agent.role == "admin"
    return if (adminOfPc && paired.isAdmin) DeviceHubRow(paired.name, entrusted = false) else null
}

/** Carte « Hub » de la fiche : ouvre l'écran du hub (PC confié) ou propose de lui confier le PC. */
@Composable
fun DeviceHubCard(row: DeviceHubRow, now: Long, onClick: () -> Unit) {
    WolCard(onClick = onClick) {
        Row(Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(WolIcons.Hub, contentDescription = null, tint = WolPalette.Blue, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                if (row.entrusted) {
                    Text(stringResource(R.string.detail_hub_entrusted, row.hubName, roleLabel(row.role)), style = MaterialTheme.typography.bodyLarge)
                    val (text, color) = hubPcStateText(row.state, now)
                    Text(text, style = MaterialTheme.typography.bodySmall, color = color)
                } else {
                    Text(stringResource(R.string.detail_hub_entrust, row.hubName), style = MaterialTheme.typography.bodyLarge, color = WolPalette.Blue)
                    Text(stringResource(R.string.detail_hub_entrust_help), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                }
            }
            Spacer(Modifier.width(8.dp))
            Icon(WolIcons.Chevron, contentDescription = null, tint = WolPalette.Text3, modifier = Modifier.size(16.dp))
        }
    }
}

/** Confirmation avant de confier le PC [deviceName] au hub [hubName] ; [busy] : le hub s'appaire au PC. */
@Composable
fun EntrustDialog(deviceName: String, hubName: String, busy: Boolean, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        icon = { Icon(WolIcons.Hub, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(R.string.hub_entrust_title, deviceName)) },
        text = { Text(stringResource(R.string.hub_entrust_text, hubName)) },
        confirmButton = {
            TextButton(onClick = onConfirm, enabled = !busy) {
                Text(stringResource(if (busy) R.string.hub_entrusting else R.string.hub_entrust))
            }
        },
        dismissButton = { TextButton(onClick = onDismiss, enabled = !busy) { Text(stringResource(R.string.cancel)) } },
    )
}

/** Échec de « Confier au hub », avec l'étape refusée. */
@Composable
fun EntrustFailureDialog(result: EntrustResult, onDismiss: () -> Unit) {
    val text = when (result) {
        EntrustResult.AlreadyEntrusted -> stringResource(R.string.hub_already_entrusted)
        is EntrustResult.AgentFailed -> stringResource(
            R.string.hub_entrust_agent,
            if (result.failure.code == ResponseCode.INSUFFICIENT_ROLE) {
                stringResource(R.string.hub_entrust_not_admin)
            } else {
                stringResource(result.failure.error.label())
            },
        )
        is EntrustResult.HubFailed -> stringResource(R.string.hub_entrust_hub, hubFailureText(result.failure, capitalize = false))
        is EntrustResult.Success -> return
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.hub_entrust_failed)) },
        text = { Text(text) },
        confirmButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.ok)) } },
    )
}
