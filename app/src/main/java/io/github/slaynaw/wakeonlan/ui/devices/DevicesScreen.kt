package io.github.slaynaw.wakeonlan.ui.devices

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.core.app.ActivityCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.network.LanTransport
import io.github.slaynaw.wakeonlan.network.LocalNetworkAccess
import io.github.slaynaw.wakeonlan.ui.common.confirmTitle
import io.github.slaynaw.wakeonlan.ui.common.label

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DevicesScreen(
    onAddDevice: () -> Unit,
    onEditDevice: (String) -> Unit,
    onOpenSettings: () -> Unit,
) {
    val context = LocalContext.current
    val resources = LocalResources.current
    val container = context.appContainer
    val vm: DevicesViewModel = viewModel { DevicesViewModel(container) }
    val state by vm.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }

    var pendingPower by remember { mutableStateOf<Pair<Device, PowerAction>?>(null) }
    var pendingDelete by remember { mutableStateOf<Device?>(null) }
    var agentHelpFor by remember { mutableStateOf<Device?>(null) }
    var permissionDenied by rememberSaveable { mutableStateOf(false) }
    var permissionAsked by rememberSaveable { mutableStateOf(false) }

    val permissionLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        vm.onPermissionResult()
        // Après deux refus, Android n'affiche plus la demande : il faut passer par les paramètres.
        val activity = context.findActivity()
        permissionDenied = !granted && activity != null &&
            !ActivityCompat.shouldShowRequestPermissionRationale(activity, LocalNetworkAccess.PERMISSION)
    }
    fun requestPermission() {
        if (permissionDenied) {
            // Refus définitif : seul l'écran des paramètres de l'application permet de l'accorder.
            context.startActivity(
                Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", context.packageName, null)),
            )
        } else {
            permissionLauncher.launch(LocalNetworkAccess.PERMISSION)
        }
    }

    // Android 17+ : demande l'accès au réseau local dès l'ouverture, une seule fois.
    LaunchedEffect(Unit) {
        if (!permissionAsked && !LocalNetworkAccess.isGranted(context)) {
            permissionAsked = true
            permissionLauncher.launch(LocalNetworkAccess.PERMISSION)
        }
    }

    LaunchedEffect(vm) {
        vm.messages.collect { message ->
            val args = message.args.map { if (it is ResArg) resources.getString(it.id) else it }.toTypedArray()
            snackbar.showSnackbar(resources.getString(message.text, *args))
        }
    }

    fun requestPower(device: Device, action: PowerAction) {
        when {
            !device.canShutdown -> agentHelpFor = device
            state.settings.confirmPowerActions -> pendingPower = device to action
            else -> vm.power(device, action, force = false)
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.app_name)) },
                actions = {
                    IconButton(onClick = { vm.refresh() }) {
                        Icon(Icons.Default.Refresh, contentDescription = stringResource(R.string.action_refresh))
                    }
                    IconButton(onClick = onOpenSettings) {
                        Icon(Icons.Default.Settings, contentDescription = stringResource(R.string.settings_title))
                    }
                },
            )
        },
        floatingActionButton = {
            ExtendedFloatingActionButton(
                onClick = onAddDevice,
                icon = { Icon(Icons.Default.Add, contentDescription = null) },
                text = { Text(stringResource(R.string.action_add_device)) },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        LazyColumn(
            contentPadding = PaddingValues(
                start = 16.dp,
                end = 16.dp,
                top = padding.calculateTopPadding() + 8.dp,
                bottom = padding.calculateBottomPadding() + 96.dp,
            ),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item(key = "network") {
                NetworkBanner(state, onRequestPermission = ::requestPermission)
            }
            if (state.loaded && state.items.isEmpty()) {
                item(key = "empty") { EmptyState(onAddDevice) }
            }
            itemsIndexed(state.items, key = { _, item -> item.device.id }) { index, item ->
                DeviceCard(
                    item = item,
                    isFirst = index == 0,
                    isLast = index == state.items.lastIndex,
                    onClick = { onEditDevice(item.device.id) },
                    onWake = { vm.wake(item.device) },
                    onPower = { action -> requestPower(item.device, action) },
                    onMove = { offset -> vm.move(item.device, offset) },
                    onDelete = { pendingDelete = item.device },
                    onClearNotice = { vm.clearNotice(item.device) },
                    modifier = Modifier.animateItem(),
                )
            }
        }
    }

    pendingPower?.let { (device, action) ->
        PowerConfirmDialog(
            device = device,
            action = action,
            onConfirm = { force ->
                pendingPower = null
                vm.power(device, action, force)
            },
            onDismiss = { pendingPower = null },
        )
    }

    pendingDelete?.let { device ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text(stringResource(R.string.confirm_delete_title, device.name)) },
            text = { Text(stringResource(R.string.confirm_delete_text)) },
            confirmButton = {
                TextButton(onClick = {
                    pendingDelete = null
                    vm.delete(device)
                }) { Text(stringResource(R.string.action_delete)) }
            },
            dismissButton = { TextButton(onClick = { pendingDelete = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }

    agentHelpFor?.let { device ->
        AlertDialog(
            onDismissRequest = { agentHelpFor = null },
            title = { Text(stringResource(R.string.agent_help_title)) },
            text = { Text(stringResource(R.string.agent_help_text, device.name)) },
            confirmButton = {
                TextButton(onClick = {
                    agentHelpFor = null
                    onEditDevice(device.id)
                }) { Text(stringResource(R.string.action_configure)) }
            },
            dismissButton = { TextButton(onClick = { agentHelpFor = null }) { Text(stringResource(R.string.cancel)) } },
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
        icon = { Icon(painterResource(R.drawable.ic_power), contentDescription = null) },
        title = { Text(stringResource(action.confirmTitle(), device.name)) },
        text = {
            Column {
                Text(stringResource(R.string.confirm_power_text))
                if (action != PowerAction.SLEEP) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Checkbox(checked = force, onCheckedChange = { force = it })
                        Text(stringResource(R.string.confirm_power_force))
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = { onConfirm(force) }) { Text(stringResource(action.label())) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun NetworkBanner(state: DevicesUiState, onRequestPermission: () -> Unit) {
    when {
        !state.localNetworkGranted -> WarningCard(
            title = stringResource(R.string.banner_permission_title),
            text = stringResource(R.string.banner_permission_text),
            action = stringResource(R.string.banner_permission_action),
            onAction = onRequestPermission,
        )
        !state.lan.connected -> WarningCard(
            title = stringResource(R.string.banner_no_lan_title),
            text = stringResource(
                if (state.lan.vpnActive) R.string.banner_no_lan_vpn_text else R.string.banner_no_lan_text,
            ),
        )
        else -> {
            val transport = stringResource(
                if (state.lan.transport == LanTransport.ETHERNET) R.string.network_ethernet else R.string.network_wifi,
            )
            Text(
                listOfNotNull(transport, state.lan.primaryAddress).joinToString(" · "),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 4.dp),
            )
        }
    }
}

@Composable
private fun WarningCard(title: String, text: String, action: String? = null, onAction: () -> Unit = {}) {
    Card(
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.errorContainer,
            contentColor = MaterialTheme.colorScheme.onErrorContainer,
        ),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(Modifier.padding(16.dp)) {
            Icon(Icons.Default.Warning, contentDescription = null)
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text(title, style = MaterialTheme.typography.titleSmall)
                Text(text, style = MaterialTheme.typography.bodyMedium)
                if (action != null) {
                    TextButton(onClick = onAction) { Text(action) }
                }
            }
        }
    }
}

@Composable
private fun EmptyState(onAddDevice: () -> Unit) {
    Column(
        Modifier.fillMaxWidth().padding(top = 48.dp, start = 24.dp, end = 24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(
            painterResource(R.drawable.ic_power),
            contentDescription = null,
            modifier = Modifier.size(72.dp),
            tint = MaterialTheme.colorScheme.primary,
        )
        Spacer(Modifier.height(16.dp))
        Text(stringResource(R.string.empty_title), style = MaterialTheme.typography.titleLarge)
        Spacer(Modifier.height(8.dp))
        Text(
            stringResource(R.string.empty_text),
            style = MaterialTheme.typography.bodyMedium,
            textAlign = TextAlign.Center,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(24.dp))
        Button(onClick = onAddDevice) { Text(stringResource(R.string.action_add_device)) }
    }
}

private tailrec fun Context.findActivity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.findActivity()
    else -> null
}
