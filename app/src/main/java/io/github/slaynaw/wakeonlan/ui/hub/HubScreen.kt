package io.github.slaynaw.wakeonlan.ui.hub

import android.content.ClipData
import android.content.ClipboardManager
import androidx.compose.foundation.clickable
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
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.slaynaw.wakeonlan.AdminChange
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent2.DeviceInfo
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.core.hub.HubDeviceField
import io.github.slaynaw.wakeonlan.core.hub.HubLinkResult
import io.github.slaynaw.wakeonlan.core.hub.HubManager
import io.github.slaynaw.wakeonlan.core.hub.HubPc
import io.github.slaynaw.wakeonlan.core.hub.HubPcState
import io.github.slaynaw.wakeonlan.core.hub.HubProbe
import io.github.slaynaw.wakeonlan.core.hub.HubService
import io.github.slaynaw.wakeonlan.core.hub.HubState
import io.github.slaynaw.wakeonlan.security.rememberSensitiveGate
import io.github.slaynaw.wakeonlan.ui.admin.AdminTexts
import io.github.slaynaw.wakeonlan.ui.admin.CreatedInviteDialog
import io.github.slaynaw.wakeonlan.ui.admin.DeviceDialog
import io.github.slaynaw.wakeonlan.ui.admin.DeviceRow
import io.github.slaynaw.wakeonlan.ui.admin.InviteDialog
import io.github.slaynaw.wakeonlan.ui.admin.InviteRow
import io.github.slaynaw.wakeonlan.ui.admin.RoleChoice
import io.github.slaynaw.wakeonlan.ui.common.BusySpinner
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.common.KeyValueRow
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.ScanFailure
import io.github.slaynaw.wakeonlan.ui.common.SectionLabel
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.formatDuration
import io.github.slaynaw.wakeonlan.ui.common.label
import io.github.slaynaw.wakeonlan.ui.common.rememberNow
import io.github.slaynaw.wakeonlan.ui.common.scanQrCode
import io.github.slaynaw.wakeonlan.ui.edit.roleLabel
import io.github.slaynaw.wakeonlan.ui.theme.MonoStyle
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import kotlinx.coroutines.launch

/** Textes des dialogues d'appareils autorisés, côté hub. */
private val HUB_TEXTS = AdminTexts(
    revoke = R.string.hub_revoke_text,
    revokeSelf = R.string.hub_revoke_self_text,
    demoteSelf = R.string.hub_demote_self_text,
    invite = R.string.hub_invite_text,
    inviteReady = R.string.hub_invite_ready,
)

/** Écran « Hub sur le NAS » (docs/HUB.md, §12). */
@Composable
fun HubScreen(onBack: () -> Unit) {
    val context = LocalContext.current
    val resources = LocalResources.current
    val container = context.appContainer
    val vm: HubViewModel = viewModel { HubViewModel(container) }
    val hub by vm.hub.collectAsStateWithLifecycle()
    val state by vm.state.collectAsStateWithLifecycle()
    val now = rememberNow(periodMs = 30_000)
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    // Administration du hub : empreinte, visage ou code si le verrouillage le demande.
    val gate = rememberSensitiveGate()
    val hubReason = stringResource(R.string.lock_reason_hub)
    val inviteReason = stringResource(R.string.lock_reason_invite)
    var scanFailure by remember { mutableStateOf<ScanFailure?>(null) }

    // Résultats brefs : un message en bas de l'écran (hors de cet effet, relancé dès le résultat effacé) ; les
    // autres s'affichent dans un dialogue.
    val brief = state.notice?.let { briefText(it) }
    LaunchedEffect(state.notice) {
        if (brief != null) {
            vm.dismissNotice()
            scope.launch { snackbar.showSnackbar(brief) }
        }
    }

    HubContent(
        hub = hub,
        state = state,
        now = now,
        snackbar = snackbar,
        onBack = onBack,
        onRefresh = vm::refresh,
        onScan = {
            scanQrCode(context, onResult = vm::pair, onError = { scanFailure = it })
        },
        onPaste = vm::pair,
        onForget = { gate.run(hubReason) { vm.forget() } },
        onChange = { change -> gate.run(hubReason) { vm.change(change) } },
        onInvite = { role, ttl, name -> gate.run(inviteReason) { vm.invite(role, ttl, name) } },
        onRetention = { days -> gate.run(hubReason) { vm.retention(days) } },
        onRemove = { pcId -> gate.run(hubReason) { vm.remove(pcId) } },
        onAddDevice = { name, host, mac, probe -> gate.run(hubReason) { vm.addDevice(name, host, mac, probe) } },
        onDismissNotice = vm::dismissNotice,
        onDismissCreated = vm::dismissCreated,
        onCopy = { text ->
            context.getSystemService(ClipboardManager::class.java)?.setPrimaryClip(ClipData.newPlainText("Patronus", text))
            scope.launch { snackbar.showSnackbar(resources.getString(R.string.share_copied)) }
        },
    )

    scanFailure?.let { failure ->
        AlertDialog(
            onDismissRequest = { scanFailure = null },
            title = { Text(stringResource(if (failure is ScanFailure.ModuleDownloading) R.string.scan_module_title else R.string.scan_error_title)) },
            text = {
                Text(
                    when (failure) {
                        is ScanFailure.ModuleDownloading -> stringResource(R.string.scan_module_text)
                        is ScanFailure.Error -> stringResource(R.string.scan_error_text, failure.message)
                    },
                )
            },
            confirmButton = { TextButton(onClick = { scanFailure = null }) { Text(stringResource(R.string.ok)) } },
        )
    }
}

/** Contenu de l'écran (sans état propre : captures d'écran). */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HubContent(
    hub: HubState,
    state: HubUiState,
    now: Long,
    snackbar: SnackbarHostState,
    onBack: () -> Unit,
    onRefresh: () -> Unit,
    onScan: () -> Unit,
    onPaste: (String) -> Unit,
    onForget: () -> Unit,
    onChange: (AdminChange) -> Unit,
    onInvite: (role: String, ttlSeconds: Long, name: String) -> Unit,
    onRetention: (Int) -> Unit,
    onRemove: (pcId: String) -> Unit,
    onAddDevice: (name: String, host: String, mac: String, probe: HubProbe) -> Unit,
    onDismissNotice: () -> Unit,
    onDismissCreated: () -> Unit,
    onCopy: (String) -> Unit,
) {
    var showPaste by rememberSaveable { mutableStateOf(false) }
    var showForget by rememberSaveable { mutableStateOf(false) }
    var showInvite by rememberSaveable { mutableStateOf(false) }
    var showRetention by rememberSaveable { mutableStateOf(false) }
    var showAddDevice by rememberSaveable { mutableStateOf(false) }
    var selected by remember { mutableStateOf<DeviceInfo?>(null) }
    var removing by remember { mutableStateOf<HubPc?>(null) }
    val paired = hub.hub
    val admin = paired?.isAdmin == true
    Scaffold(
        containerColor = WolPalette.Background,
        snackbarHost = { SnackbarHost(snackbar) },
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text(stringResource(R.string.hub_title), style = MaterialTheme.typography.titleLarge)
                        if (paired != null) Text(paired.name, style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                    }
                },
                navigationIcon = {
                    IconButton(onClick = onBack) { Icon(WolIcons.ArrowBack, contentDescription = stringResource(R.string.back)) }
                },
                actions = {
                    if (state.busy) {
                        BusySpinner(Modifier.padding(end = 12.dp))
                    } else if (paired != null) {
                        IconButton(onClick = onRefresh) {
                            Icon(WolIcons.Refresh, contentDescription = stringResource(R.string.admin_retry), tint = WolPalette.Text2)
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = WolPalette.Background),
            )
        },
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            when {
                !hub.loaded -> Box(Modifier.fillMaxWidth().padding(24.dp), contentAlignment = Alignment.Center) { BusySpinner() }
                paired == null -> PairCard(busy = state.busy, onScan = onScan, onPaste = { showPaste = true })
                else -> {
                    StateCard(hub, now)
                    SectionLabel(stringResource(R.string.hub_pcs), Modifier.padding(top = 6.dp))
                    val pcs = hub.snapshot?.pcs.orEmpty()
                    WolCard {
                        if (pcs.isEmpty()) {
                            Text(
                                stringResource(R.string.hub_pcs_none),
                                style = MaterialTheme.typography.bodySmall,
                                color = WolPalette.Text2,
                                modifier = Modifier.padding(14.dp),
                            )
                        }
                        pcs.forEachIndexed { index, pc ->
                            if (index > 0) RowDivider()
                            PcRow(pc, hub.snapshot?.stateOf(pc.id), now, admin, onRemove = { removing = pc })
                        }
                    }
                    if (admin) {
                        WolButton(
                            stringResource(R.string.hub_add_device),
                            onClick = { showAddDevice = true },
                            modifier = Modifier.fillMaxWidth(),
                            icon = WolIcons.Plus,
                            enabled = !state.busy,
                        )
                        AdminSections(state, onSelect = { selected = it }, onChange = onChange, onRetry = onRefresh, onRetention = { showRetention = true })
                        WolButton(
                            stringResource(R.string.admin_invite),
                            onClick = { showInvite = true },
                            modifier = Modifier.fillMaxWidth(),
                            kind = ButtonKind.PRIMARY,
                            icon = WolIcons.Plus,
                            enabled = !state.busy && state.admin != null,
                            height = 48.dp,
                        )
                    }
                    WolButton(
                        stringResource(R.string.hub_forget),
                        onClick = { showForget = true },
                        modifier = Modifier.fillMaxWidth().padding(top = 6.dp),
                        kind = ButtonKind.DANGER,
                        icon = WolIcons.Delete,
                        enabled = !state.busy,
                    )
                }
            }
            Spacer(Modifier.height(24.dp))
        }
    }

    if (showPaste) {
        PasteDialog(
            onConfirm = { text ->
                showPaste = false
                onPaste(text)
            },
            onDismiss = { showPaste = false },
        )
    }
    if (showForget && paired != null) {
        AlertDialog(
            onDismissRequest = { showForget = false },
            title = { Text(stringResource(R.string.hub_forget_title, paired.name)) },
            text = { Text(stringResource(R.string.hub_forget_text)) },
            confirmButton = {
                TextButton(onClick = { showForget = false; onForget() }) { Text(stringResource(R.string.hub_forget), color = WolPalette.DangerText) }
            },
            dismissButton = { TextButton(onClick = { showForget = false }) { Text(stringResource(R.string.cancel)) } },
        )
    }
    selected?.let { device ->
        DeviceDialog(device, onChange = onChange, onDismiss = { selected = null }, roles = HubViewModel.ROLES, texts = HUB_TEXTS)
    }
    if (showInvite) {
        InviteDialog(
            onCreate = { role, ttl, name ->
                showInvite = false
                onInvite(role, ttl, name)
            },
            onDismiss = { showInvite = false },
            roles = HubViewModel.ROLES,
            defaultRole = "view",
            texts = HUB_TEXTS,
        )
    }
    state.created?.let { created ->
        CreatedInviteDialog(created.link, created.code, created.expires, HUB_TEXTS, onCopy = onCopy, onDismiss = onDismissCreated)
    }
    if (showRetention) {
        RetentionDialog(
            current = state.admin?.retention ?: 0,
            onChoose = { days ->
                showRetention = false
                if (days != state.admin?.retention) onRetention(days)
            },
            onDismiss = { showRetention = false },
        )
    }
    removing?.let { pc ->
        AlertDialog(
            onDismissRequest = { removing = null },
            title = { Text(stringResource(R.string.hub_pc_remove_title, pc.name)) },
            text = { Text(stringResource(if (pc.agent.isNotEmpty()) R.string.hub_pc_remove_text else R.string.hub_device_remove_text)) },
            confirmButton = {
                TextButton(onClick = { removing = null; onRemove(pc.id) }) { Text(stringResource(R.string.hub_pc_remove), color = WolPalette.DangerText) }
            },
            dismissButton = { TextButton(onClick = { removing = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }
    if (showAddDevice) {
        AddDeviceDialog(
            onAdd = { name, host, mac, probe ->
                showAddDevice = false
                onAddDevice(name, host, mac, probe)
            },
            onDismiss = { showAddDevice = false },
        )
    }
    state.notice?.let { notice ->
        noticeDialog(notice)?.let { (title, text) ->
            AlertDialog(
                onDismissRequest = onDismissNotice,
                title = { Text(title) },
                text = { Text(text) },
                confirmButton = { TextButton(onClick = onDismissNotice) { Text(stringResource(R.string.ok)) } },
            )
        }
    }
}

/** Sans hub : à quoi il sert, et comment l'appairer. */
@Composable
private fun PairCard(busy: Boolean, onScan: () -> Unit, onPaste: () -> Unit) {
    WolCard {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(WolIcons.Hub, contentDescription = null, tint = WolPalette.Blue, modifier = Modifier.size(22.dp))
                Spacer(Modifier.width(12.dp))
                Text(stringResource(R.string.hub_pair), style = MaterialTheme.typography.titleMedium)
            }
            Text(stringResource(R.string.hub_none_help), style = MaterialTheme.typography.bodyMedium)
            Text(stringResource(R.string.hub_pair_text), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
            WolButton(
                stringResource(R.string.hub_pair_scan),
                onClick = onScan,
                modifier = Modifier.fillMaxWidth(),
                kind = ButtonKind.PRIMARY,
                icon = WolIcons.Scan,
                enabled = !busy,
                height = 48.dp,
            )
            WolButton(
                stringResource(R.string.hub_pair_paste),
                onClick = onPaste,
                modifier = Modifier.fillMaxWidth(),
                icon = WolIcons.Paste,
                enabled = !busy,
            )
        }
    }
}

/** État du hub : joignable ou non, version, démarrage, adresse, empreinte et rôle de ce téléphone. */
@Composable
private fun StateCard(hub: HubState, now: Long) {
    val paired = hub.hub ?: return
    val snapshot = hub.snapshot
    val failure = hub.failure
    SectionLabel(stringResource(R.string.hub_state))
    WolCard {
        val (state, color) = when {
            failure != null -> stringResource(R.string.hub_state_failed) to WolPalette.DangerText
            snapshot != null -> stringResource(R.string.hub_state_ok) to WolPalette.On
            else -> stringResource(R.string.hub_state_loading) to WolPalette.Text2
        }
        KeyValueRow(stringResource(R.string.hub_state), state, valueColor = color)
        if (failure != null) {
            Text(
                hubFailureText(failure),
                style = MaterialTheme.typography.bodySmall,
                color = WolPalette.DangerText,
                modifier = Modifier.padding(start = 14.dp, end = 14.dp, bottom = 11.dp),
            )
        }
        if (snapshot != null) {
            if (snapshot.info.version.isNotEmpty()) {
                RowDivider()
                val version = snapshot.info.version
                KeyValueRow(stringResource(R.string.hub_version), if (version.firstOrNull()?.isDigit() == true) "v$version" else version)
            }
            if (snapshot.info.started > 0) {
                RowDivider()
                KeyValueRow(
                    stringResource(R.string.hub_started),
                    stringResource(R.string.hub_started_ago, formatDuration(now - snapshot.info.started * 1000)),
                )
            }
        }
        RowDivider()
        KeyValueRow(stringResource(R.string.hub_address), "${paired.host}:${paired.port}", mono = true)
        RowDivider()
        KeyValueRow(stringResource(R.string.hub_pin), paired.shortPin, mono = true)
        RowDivider()
        KeyValueRow(stringResource(R.string.hub_role), roleLabel(paired.role))
    }
}

/** PC ou appareil confié au hub, avec l'état relevé par le hub. */
@Composable
private fun PcRow(pc: HubPc, state: HubPcState?, now: Long, admin: Boolean, onRemove: () -> Unit) {
    val probe = pc.probe
    val what = when {
        pc.agent.isNotEmpty() -> stringResource(R.string.hub_pc_agent, Pin.parse(pc.agent)?.short().orEmpty(), roleLabel(pc.role))
        probe != null && probe.kind == HubService.PROBE_TCP -> stringResource(R.string.hub_pc_probe_tcp, probe.port)
        probe != null && probe.kind == HubService.PROBE_HTTPS -> stringResource(R.string.hub_pc_probe_https)
        else -> stringResource(R.string.hub_pc_probe_http)
    }
    val (stateText, stateColor) = hubPcStateText(state, now)
    Row(
        Modifier.fillMaxWidth().padding(start = 14.dp, end = 4.dp, top = 10.dp, bottom = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(if (pc.agent.isNotEmpty()) WolIcons.Monitor else WolIcons.Hub, contentDescription = null, tint = WolPalette.Text2, modifier = Modifier.size(18.dp))
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(pc.name, style = MaterialTheme.typography.bodyLarge)
            Text(pc.host, style = MonoStyle.copy(fontSize = 12.sp), color = WolPalette.Text2)
            Text(what, style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
            Text(stateText, style = MaterialTheme.typography.bodySmall, color = stateColor)
        }
        if (admin) {
            IconButton(onClick = onRemove) {
                Icon(WolIcons.Delete, contentDescription = stringResource(R.string.hub_pc_remove), tint = WolPalette.Text2)
            }
        }
    }
}

/** Administration du hub : conservation de l'historique, appareils autorisés et invitations. */
@Composable
private fun AdminSections(
    state: HubUiState,
    onSelect: (DeviceInfo) -> Unit,
    onChange: (AdminChange) -> Unit,
    onRetry: () -> Unit,
    onRetention: () -> Unit,
) {
    state.adminFailure?.let { failure ->
        WolCard {
            Text(
                hubFailureText(failure),
                style = MaterialTheme.typography.bodyMedium,
                color = WolPalette.DangerText,
                modifier = Modifier.padding(14.dp),
            )
            TextButton(onClick = onRetry, modifier = Modifier.padding(start = 6.dp, bottom = 6.dp)) {
                Text(stringResource(R.string.admin_retry), color = WolPalette.Blue)
            }
        }
    }
    val admin = state.admin ?: return
    SectionLabel(stringResource(R.string.hub_retention), Modifier.padding(top = 6.dp))
    WolCard {
        KeyValueRow(
            stringResource(R.string.hub_retention_label),
            retentionLabel(admin.retention),
            modifier = Modifier.clickable(enabled = !state.busy, onClick = onRetention),
            valueColor = WolPalette.Blue,
        )
    }
    Text(stringResource(R.string.hub_retention_help), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
    if (admin.devices.isNotEmpty()) {
        SectionLabel(stringResource(R.string.admin_devices), Modifier.padding(top = 6.dp))
        WolCard {
            admin.devices.forEachIndexed { index, device ->
                if (index > 0) RowDivider()
                DeviceRow(device, onClick = { onSelect(device) })
            }
        }
    }
    if (admin.invites.isNotEmpty()) {
        SectionLabel(stringResource(R.string.admin_invites), Modifier.padding(top = 6.dp))
        WolCard {
            admin.invites.forEachIndexed { index, invite ->
                if (index > 0) RowDivider()
                InviteRow(invite, onCancel = { onChange(AdminChange.CancelInvite(invite.id)) })
            }
        }
    }
    Text(stringResource(R.string.hub_devices_help), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
}

@Composable
private fun retentionLabel(days: Int): String = when (days) {
    90 -> stringResource(R.string.hub_retention_90)
    182 -> stringResource(R.string.hub_retention_182)
    365 -> stringResource(R.string.hub_retention_365)
    730 -> stringResource(R.string.hub_retention_730)
    1095 -> stringResource(R.string.hub_retention_1095)
    else -> stringResource(R.string.hub_retention_days, days)
}

@Composable
private fun RetentionDialog(current: Int, onChoose: (Int) -> Unit, onDismiss: () -> Unit) {
    val options = if (current in HubViewModel.RETENTIONS || current <= 0) HubViewModel.RETENTIONS else listOf(current) + HubViewModel.RETENTIONS
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.hub_retention)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(stringResource(R.string.hub_retention_help), style = MaterialTheme.typography.bodyMedium)
                options.forEach { days -> RoleChoice(days == current, retentionLabel(days)) { onChoose(days) } }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun PasteDialog(onConfirm: (String) -> Unit, onDismiss: () -> Unit) {
    var value by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(WolIcons.Paste, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(R.string.hub_pair)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(stringResource(R.string.hub_pair_paste_text))
                OutlinedTextField(
                    value = value,
                    onValueChange = { value = it },
                    placeholder = { Text(stringResource(R.string.hub_pair_placeholder)) },
                    minLines = 2,
                    maxLines = 4,
                )
            }
        },
        confirmButton = { TextButton(onClick = { onConfirm(value.trim()) }, enabled = value.isNotBlank()) { Text(stringResource(R.string.ok)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

/** Appareil sans agent (§5.4) : nom, adresse, vérification et MAC facultative, vérifiés avant l'envoi. */
@Composable
private fun AddDeviceDialog(onAdd: (name: String, host: String, mac: String, probe: HubProbe) -> Unit, onDismiss: () -> Unit) {
    var name by rememberSaveable { mutableStateOf("") }
    var host by rememberSaveable { mutableStateOf("") }
    var kind by rememberSaveable { mutableStateOf(HubService.PROBE_HTTP) }
    var port by rememberSaveable { mutableStateOf("80") }
    var mac by rememberSaveable { mutableStateOf("") }
    var invalid by remember { mutableStateOf<HubDeviceField?>(null) }
    val probes = listOf(
        HubService.PROBE_HTTP to stringResource(R.string.hub_pc_probe_http),
        HubService.PROBE_HTTPS to stringResource(R.string.hub_pc_probe_https),
        HubService.PROBE_TCP to stringResource(R.string.hub_add_probe_tcp),
    )
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.hub_add_device)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(stringResource(R.string.hub_add_device_text), style = MaterialTheme.typography.bodyMedium)
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it.take(64) },
                    label = { Text(stringResource(R.string.field_name)) },
                    singleLine = true,
                    isError = invalid == HubDeviceField.NAME,
                    keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
                )
                OutlinedTextField(
                    value = host,
                    onValueChange = { host = it.trim() },
                    label = { Text(stringResource(R.string.hub_add_host)) },
                    singleLine = true,
                    isError = invalid == HubDeviceField.HOST,
                    textStyle = MonoStyle,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                )
                Text(stringResource(R.string.hub_add_probe), style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 6.dp))
                probes.forEach { (value, label) -> RoleChoice(value == kind, label) { kind = value } }
                if (kind == HubService.PROBE_TCP) {
                    OutlinedTextField(
                        value = port,
                        onValueChange = { port = it.filter(Char::isDigit).take(5) },
                        label = { Text(stringResource(R.string.hub_add_port)) },
                        singleLine = true,
                        isError = invalid == HubDeviceField.PORT,
                        textStyle = MonoStyle,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    )
                }
                OutlinedTextField(
                    value = mac,
                    onValueChange = { mac = it.trim() },
                    label = { Text(stringResource(R.string.hub_add_mac)) },
                    singleLine = true,
                    isError = invalid == HubDeviceField.MAC,
                    textStyle = MonoStyle,
                    keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Characters, keyboardType = KeyboardType.Ascii),
                )
                invalid?.let { field ->
                    Text(invalidText(field), style = MaterialTheme.typography.bodySmall, color = WolPalette.DangerText)
                }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                val probe = HubProbe(kind, if (kind == HubService.PROBE_TCP) port.toIntOrNull() ?: 0 else 0)
                invalid = HubService.validateDevice(name, host, mac, probe)
                if (invalid == null) onAdd(name.trim(), host.trim(), mac.trim(), probe)
            }) { Text(stringResource(R.string.hub_add_device)) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun invalidText(field: HubDeviceField): String = stringResource(
    when (field) {
        HubDeviceField.NAME -> R.string.hub_add_invalid_name
        HubDeviceField.HOST -> R.string.hub_add_invalid_host
        HubDeviceField.MAC -> R.string.hub_add_invalid_mac
        HubDeviceField.PROBE -> R.string.hub_add_invalid_probe
        HubDeviceField.PORT -> R.string.hub_add_invalid_port
    },
)

/** Cause d'un échec auprès du hub, rédigée pour lui (une phrase). */
@Composable
internal fun hubFailureText(failure: AgentResult.Failure, capitalize: Boolean = true): String {
    val text = when (failure.code) {
        ResponseCode.LAST_ADMIN -> stringResource(R.string.hub_last_admin)
        ResponseCode.INSUFFICIENT_ROLE -> stringResource(R.string.hub_error_insufficient_role)
        ResponseCode.BAD_REQUEST -> stringResource(R.string.hub_error_bad_request)
        HubManager.NO_HUB -> stringResource(R.string.hub_error_no_hub)
        else -> stringResource(
            when (failure.error) {
                AgentError.UNREACHABLE -> R.string.hub_error_unreachable
                AgentError.REFUSED -> R.string.hub_error_refused
                AgentError.UNREGISTERED -> R.string.hub_error_unregistered
                AgentError.PIN_MISMATCH -> R.string.hub_error_pin_mismatch
                AgentError.PAIRING_FAILED -> R.string.hub_error_pairing_failed
                AgentError.OTHER_SERVICE -> R.string.hub_error_other_service
                AgentError.PROTOCOL -> R.string.hub_error_protocol
                AgentError.REJECTED -> R.string.hub_error_rejected
                else -> failure.error.label()
            },
        )
    }
    return if (capitalize) text.replaceFirstChar { it.uppercase() } else text
}

/** Texte d'un résultat bref (message en bas de l'écran), null pour un résultat montré dans un dialogue. */
@Composable
private fun briefText(notice: HubNotice): String? = when (notice) {
    is HubNotice.Paired -> stringResource(
        if (notice.already) R.string.hub_already else R.string.hub_paired,
        notice.name,
        roleLabel(notice.role),
    )
    is HubNotice.Forgotten -> if (notice.revoked) stringResource(R.string.hub_forgotten) else null
    is HubNotice.Removed -> if (notice.kept) null else stringResource(R.string.hub_pc_removed, notice.name)
    is HubNotice.DeviceAdded -> stringResource(R.string.hub_device_added, notice.name)
    is HubNotice.Retention -> stringResource(R.string.hub_retention_done, retentionLabel(notice.days))
    else -> null
}

/** Titre et texte d'un résultat montré dans un dialogue (échec, retrait incomplet). */
@Composable
private fun noticeDialog(notice: HubNotice): Pair<String, String>? = when (notice) {
    is HubNotice.BadLink -> stringResource(R.string.hub_pair_failed) to stringResource(
        when (notice.reason) {
            HubLinkResult.AgentLink -> R.string.hub_link_agent_link
            HubLinkResult.Expired -> R.string.hub_link_expired
            else -> R.string.hub_link_invalid
        },
    )
    HubNotice.OtherHub -> stringResource(R.string.hub_pair_failed) to stringResource(R.string.hub_other_paired)
    is HubNotice.PairFailed -> stringResource(R.string.hub_pair_failed) to hubFailureText(notice.failure)
    is HubNotice.Failed -> stringResource(R.string.admin_failed) to hubFailureText(notice.failure)
    is HubNotice.Forgotten -> if (notice.revoked) null else stringResource(R.string.hub_forgotten_kept_title) to stringResource(R.string.hub_forgotten_kept)
    is HubNotice.Removed -> if (notice.kept) stringResource(R.string.hub_pc_kept_title) to stringResource(R.string.hub_pc_kept, notice.hubPin) else null
    else -> null
}
