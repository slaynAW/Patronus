package io.github.slaynaw.wakeonlan.ui.main

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.core.app.ActivityCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.core.agent.PowerAction
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.diagnostics.CrashReporter
import io.github.slaynaw.wakeonlan.network.LocalNetworkAccess
import io.github.slaynaw.wakeonlan.ui.common.DeviceDialogs
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.rememberDeviceDialogState
import io.github.slaynaw.wakeonlan.ui.devices.DevicesTab
import io.github.slaynaw.wakeonlan.ui.devices.DevicesViewModel
import io.github.slaynaw.wakeonlan.ui.devices.ResArg
import io.github.slaynaw.wakeonlan.ui.overview.OverviewTab
import io.github.slaynaw.wakeonlan.ui.settings.SettingsTab
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/** Onglets de la barre de navigation. */
enum class MainTab(@StringRes val label: Int, val icon: ImageVector) {
    OVERVIEW(R.string.tab_overview, WolIcons.Topology),
    DEVICES(R.string.tab_devices, WolIcons.Rows),
    SETTINGS(R.string.tab_settings, WolIcons.Settings),
}

/** Écran principal : vue d'ensemble, appareils et réglages, avec la barre de navigation. */
@Composable
fun MainScreen(
    onOpenDevice: (String) -> Unit,
    onAddDevice: () -> Unit,
    onEditDevice: (String) -> Unit,
    onOpenHistory: (String?) -> Unit,
) {
    val context = LocalContext.current
    val resources = LocalResources.current
    val container = context.appContainer
    val vm: DevicesViewModel = viewModel { DevicesViewModel(container) }
    val state by vm.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val dialogs = rememberDeviceDialogState()
    var tab by rememberSaveable { mutableStateOf(MainTab.OVERVIEW) }
    var crashReport by remember { mutableStateOf(CrashReporter.pending(context)) }
    var permissionDenied by rememberSaveable { mutableStateOf(false) }
    var permissionAsked by rememberSaveable { mutableStateOf(false) }

    val permissionLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        vm.onPermissionResult()
        // Après deux refus, Android n'affiche plus la demande : il faut passer par les paramètres.
        val activity = context.findActivity()
        permissionDenied = !granted && activity != null &&
            !ActivityCompat.shouldShowRequestPermissionRationale(activity, LocalNetworkAccess.PERMISSION)
    }
    val requestPermission = {
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

    // Retour depuis un autre onglet : d'abord la vue d'ensemble.
    BackHandler(enabled = tab != MainTab.OVERVIEW) { tab = MainTab.OVERVIEW }

    val requestPower = { device: Device, action: PowerAction ->
        dialogs.requestPower(device, action, state.settings.confirmPowerActions, vm::power)
    }

    MainScaffold(
        tab = tab,
        onSelectTab = { tab = it },
        snackbar = snackbar,
        showAddButton = tab == MainTab.DEVICES && state.items.isNotEmpty(),
        onAddDevice = onAddDevice,
    ) { padding ->
        when (tab) {
            MainTab.OVERVIEW -> OverviewTab(
                state = state,
                contentPadding = padding,
                onOpenDevice = onOpenDevice,
                onAddDevice = onAddDevice,
                onWake = vm::wake,
                onShutdown = { requestPower(it, PowerAction.SHUTDOWN) },
                onRefresh = vm::refresh,
                onRequestPermission = requestPermission,
            )
            MainTab.DEVICES -> DevicesTab(
                state = state,
                contentPadding = padding,
                onOpenDevice = onOpenDevice,
                onAddDevice = onAddDevice,
                onEditDevice = onEditDevice,
                onOpenHistory = { onOpenHistory(it) },
                onWake = vm::wake,
                onPower = requestPower,
                onMove = vm::move,
                onDelete = { dialogs.pendingDelete = it },
                onClearNotice = vm::clearNotice,
                onRequestPermission = requestPermission,
            )
            MainTab.SETTINGS -> SettingsTab(
                contentPadding = padding,
                snackbar = snackbar,
                onOpenHistory = { onOpenHistory(null) },
            )
        }
    }

    DeviceDialogs(dialogs, onPower = vm::power, onDelete = vm::delete, onConfigure = onEditDevice)

    crashReport?.let { report ->
        AlertDialog(
            onDismissRequest = {
                CrashReporter.clear(context)
                crashReport = null
            },
            icon = { Icon(WolIcons.Warning, contentDescription = null, tint = WolPalette.DangerText) },
            title = { Text(stringResource(R.string.crash_title)) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(stringResource(R.string.crash_text))
                    Text(
                        report,
                        style = MaterialTheme.typography.bodySmall,
                        fontFamily = FontFamily.Monospace,
                        modifier = Modifier
                            .heightIn(max = 220.dp)
                            .verticalScroll(rememberScrollState()),
                    )
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    val send = Intent(Intent.ACTION_SEND)
                        .setType("text/plain")
                        .putExtra(Intent.EXTRA_SUBJECT, resources.getString(R.string.crash_subject))
                        .putExtra(Intent.EXTRA_TEXT, report)
                    context.startActivity(Intent.createChooser(send, null))
                    CrashReporter.clear(context)
                    crashReport = null
                }) { Text(stringResource(R.string.crash_share)) }
            },
            dismissButton = {
                TextButton(onClick = {
                    CrashReporter.clear(context)
                    crashReport = null
                }) { Text(stringResource(R.string.crash_ignore)) }
            },
        )
    }
}

/** Cadre de l'écran principal : barre de navigation en bas, messages, bouton d'ajout (onglet Appareils). */
@Composable
fun MainScaffold(
    tab: MainTab,
    onSelectTab: (MainTab) -> Unit,
    snackbar: SnackbarHostState,
    showAddButton: Boolean,
    onAddDevice: () -> Unit,
    content: @Composable (PaddingValues) -> Unit,
) {
    Scaffold(
        containerColor = WolPalette.Background,
        bottomBar = { WolNavigationBar(tab, onSelect = onSelectTab) },
        snackbarHost = { SnackbarHost(snackbar) },
        floatingActionButton = {
            if (showAddButton) {
                ExtendedFloatingActionButton(
                    onClick = onAddDevice,
                    containerColor = WolPalette.Blue,
                    contentColor = Color.White,
                    icon = { Icon(WolIcons.Plus, contentDescription = null) },
                    text = { Text(stringResource(R.string.action_add_device)) },
                )
            }
        },
        content = content,
    )
}

@Composable
private fun WolNavigationBar(current: MainTab, onSelect: (MainTab) -> Unit) {
    Column {
        HorizontalDivider(thickness = 1.dp, color = WolPalette.Line)
        NavigationBar(containerColor = WolPalette.Surface, tonalElevation = 0.dp) {
            MainTab.entries.forEach { tab ->
                NavigationBarItem(
                    selected = tab == current,
                    onClick = { onSelect(tab) },
                    icon = { Icon(tab.icon, contentDescription = null) },
                    label = { Text(stringResource(tab.label), style = MaterialTheme.typography.labelMedium) },
                    colors = NavigationBarItemDefaults.colors(
                        selectedIconColor = WolPalette.Blue,
                        selectedTextColor = WolPalette.Blue,
                        indicatorColor = WolPalette.BlueSoft,
                        unselectedIconColor = WolPalette.Text2,
                        unselectedTextColor = WolPalette.Text2,
                    ),
                )
            }
        }
    }
}

private tailrec fun Context.findActivity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.findActivity()
    else -> null
}
