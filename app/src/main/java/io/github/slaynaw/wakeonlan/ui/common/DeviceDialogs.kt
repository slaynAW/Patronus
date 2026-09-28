package io.github.slaynaw.wakeonlan.ui.common

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.size
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/** Dialogues communs à la liste des PC et à la fiche d'un PC (confirmation, suppression, aide). */
@Stable
class DeviceDialogState {
    var pendingPower by mutableStateOf<Pair<Device, PowerAction>?>(null)
    var pendingDelete by mutableStateOf<Device?>(null)
    var agentHelpFor by mutableStateOf<Device?>(null)

    /** Extinction / redémarrage / veille : aide si le PC n'a pas d'agent, confirmation si demandée. */
    fun requestPower(device: Device, action: PowerAction, confirm: Boolean, send: (Device, PowerAction, Boolean) -> Unit) {
        when {
            !device.canShutdown -> agentHelpFor = device
            confirm -> pendingPower = device to action
            else -> send(device, action, false)
        }
    }
}

@Composable
fun rememberDeviceDialogState(): DeviceDialogState = remember { DeviceDialogState() }

@Composable
fun DeviceDialogs(
    state: DeviceDialogState,
    onPower: (Device, PowerAction, Boolean) -> Unit,
    onDelete: (Device) -> Unit,
    onConfigure: (String) -> Unit,
) {
    state.pendingPower?.let { (device, action) ->
        PowerConfirmDialog(
            device = device,
            action = action,
            onConfirm = { force ->
                state.pendingPower = null
                onPower(device, action, force)
            },
            onDismiss = { state.pendingPower = null },
        )
    }

    state.pendingDelete?.let { device ->
        AlertDialog(
            onDismissRequest = { state.pendingDelete = null },
            icon = { Icon(WolIcons.Delete, contentDescription = null, tint = WolPalette.DangerText) },
            title = { Text(stringResource(R.string.confirm_delete_title, device.name)) },
            text = { Text(stringResource(R.string.confirm_delete_text)) },
            confirmButton = {
                WolButton(
                    text = stringResource(R.string.action_delete),
                    kind = ButtonKind.DANGER,
                    onClick = {
                        state.pendingDelete = null
                        onDelete(device)
                    },
                )
            },
            dismissButton = { TextButton(onClick = { state.pendingDelete = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }

    state.agentHelpFor?.let { device ->
        AlertDialog(
            onDismissRequest = { state.agentHelpFor = null },
            icon = { Icon(WolIcons.Shield, contentDescription = null, tint = WolPalette.Blue) },
            title = { Text(stringResource(R.string.agent_help_title)) },
            text = { Text(stringResource(R.string.agent_help_text, device.name)) },
            confirmButton = {
                WolButton(
                    text = stringResource(R.string.action_configure),
                    kind = ButtonKind.PRIMARY,
                    onClick = {
                        state.agentHelpFor = null
                        onConfigure(device.id)
                    },
                )
            },
            dismissButton = { TextButton(onClick = { state.agentHelpFor = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
private fun PowerConfirmDialog(
    device: Device,
    action: PowerAction,
    onConfirm: (force: Boolean) -> Unit,
    onDismiss: () -> Unit,
) {
    var force by remember { mutableStateOf(false) }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(action.icon(), contentDescription = null, tint = WolPalette.Blue, modifier = Modifier.size(26.dp)) },
        title = { Text(stringResource(action.confirmTitle(), device.name)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(stringResource(R.string.confirm_power_text))
                if (action != PowerAction.SLEEP) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Checkbox(checked = force, onCheckedChange = { force = it })
                        Text(stringResource(R.string.confirm_power_force), color = WolPalette.Text)
                    }
                }
            }
        },
        confirmButton = {
            WolButton(
                text = stringResource(action.label()),
                kind = if (action == PowerAction.SHUTDOWN) ButtonKind.DANGER else ButtonKind.PRIMARY,
                onClick = { onConfirm(force) },
            )
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}
