package io.github.slaynaw.wakeonlan.ui.admin

import android.content.ClipData
import android.content.ClipboardManager
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
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
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
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.slaynaw.wakeonlan.AdminChange
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.core.agent2.DeviceInfo
import io.github.slaynaw.wakeonlan.core.agent2.InviteInfo
import io.github.slaynaw.wakeonlan.core.agent2.Pin
import io.github.slaynaw.wakeonlan.core.agent2.ResponseCode
import io.github.slaynaw.wakeonlan.ui.common.BusySpinner
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.SectionLabel
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.label
import io.github.slaynaw.wakeonlan.ui.edit.roleLabel
import io.github.slaynaw.wakeonlan.ui.settings.QrImage
import io.github.slaynaw.wakeonlan.ui.theme.MonoStyle
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

private val ROLES = listOf("view", "wake", "control", "admin")
private val INVITE_TTLS = listOf(600L, 3600L, 86400L, 604800L)
private val dayFormat = DateTimeFormatter.ofPattern("d MMM yyyy", Locale.FRENCH)
private val dayTimeFormat = DateTimeFormatter.ofPattern("d MMM yyyy 'à' HH:mm", Locale.FRENCH)

private fun formatDay(seconds: Long): String = dayFormat.format(Instant.ofEpochSecond(seconds).atZone(ZoneId.systemDefault()))

private fun formatDayTime(seconds: Long): String = dayTimeFormat.format(Instant.ofEpochSecond(seconds).atZone(ZoneId.systemDefault()))

/** Appareils autorisés par un PC (wolagent/2, rôle « admin »). */
@Composable
fun AdminScreen(deviceId: String, onBack: () -> Unit) {
    val context = LocalContext.current
    val resources = LocalResources.current
    val container = context.appContainer
    val vm: AdminViewModel = viewModel(key = "admin-$deviceId") { AdminViewModel(container, deviceId) }
    val state by vm.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    AdminContent(
        state = state,
        snackbar = snackbar,
        onBack = onBack,
        onRefresh = vm::load,
        onChange = vm::change,
        onInvite = vm::invite,
        onDismissCreated = vm::dismissCreated,
        onDismissFailure = vm::dismissFailure,
        onCopy = { text ->
            context.getSystemService(ClipboardManager::class.java)?.setPrimaryClip(ClipData.newPlainText("Patronus", text))
            scope.launch { snackbar.showSnackbar(resources.getString(R.string.share_copied)) }
        },
    )
}

/** Contenu de l'écran (sans état propre : captures d'écran). */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AdminContent(
    state: AdminUiState,
    snackbar: SnackbarHostState,
    onBack: () -> Unit,
    onRefresh: () -> Unit,
    onChange: (AdminChange) -> Unit,
    onInvite: (role: String, ttlSeconds: Long, name: String) -> Unit,
    onDismissCreated: () -> Unit,
    onDismissFailure: () -> Unit,
    onCopy: (String) -> Unit,
) {
    var selected by remember { mutableStateOf<DeviceInfo?>(null) }
    var showInvite by rememberSaveable { mutableStateOf(false) }
    Scaffold(
        containerColor = WolPalette.Background,
        snackbarHost = { SnackbarHost(snackbar) },
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text(stringResource(R.string.admin_title), style = MaterialTheme.typography.titleLarge)
                        if (state.deviceName.isNotEmpty()) {
                            Text(state.deviceName, style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                        }
                    }
                },
                navigationIcon = {
                    IconButton(onClick = onBack) { Icon(WolIcons.ArrowBack, contentDescription = stringResource(R.string.back)) }
                },
                actions = {
                    if (state.busy || state.loading) {
                        BusySpinner(Modifier.padding(end = 12.dp))
                    } else {
                        IconButton(onClick = onRefresh) {
                            Icon(WolIcons.Refresh, contentDescription = stringResource(R.string.history_refresh), tint = WolPalette.Text2)
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
            state.error?.let { failure ->
                WolCard {
                    Text(
                        failureText(failure),
                        style = MaterialTheme.typography.bodyMedium,
                        color = WolPalette.DangerText,
                        modifier = Modifier.padding(14.dp),
                    )
                    TextButton(onClick = onRefresh, modifier = Modifier.padding(start = 6.dp, bottom = 6.dp)) {
                        Text(stringResource(R.string.admin_retry), color = WolPalette.Blue)
                    }
                }
            }
            if (state.devices.isNotEmpty()) {
                SectionLabel(stringResource(R.string.admin_devices))
                WolCard {
                    state.devices.forEachIndexed { index, device ->
                        if (index > 0) RowDivider()
                        DeviceRow(device, onClick = { selected = device })
                    }
                }
            }
            if (state.invites.isNotEmpty()) {
                SectionLabel(stringResource(R.string.admin_invites), Modifier.padding(top = 6.dp))
                WolCard {
                    state.invites.forEachIndexed { index, invite ->
                        if (index > 0) RowDivider()
                        InviteRow(invite, onCancel = { onChange(AdminChange.CancelInvite(invite.id)) })
                    }
                }
            }
            if (state.error == null && !state.loading) {
                Text(stringResource(R.string.admin_help), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                WolButton(
                    stringResource(R.string.admin_invite),
                    onClick = { showInvite = true },
                    modifier = Modifier.fillMaxWidth(),
                    kind = ButtonKind.PRIMARY,
                    icon = WolIcons.Plus,
                    enabled = !state.busy,
                    height = 48.dp,
                )
            }
            Spacer(Modifier.height(24.dp))
        }
    }

    selected?.let { device ->
        DeviceDialog(device, onChange = onChange, onDismiss = { selected = null })
    }
    if (showInvite) {
        InviteDialog(
            onCreate = { role, ttl, name ->
                showInvite = false
                onInvite(role, ttl, name)
            },
            onDismiss = { showInvite = false },
        )
    }
    state.created?.let { created ->
        AlertDialog(
            onDismissRequest = onDismissCreated,
            title = { Text(stringResource(R.string.admin_invite)) },
            text = {
                Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text(stringResource(R.string.admin_invite_ready, formatDayTime(created.expires)), style = MaterialTheme.typography.bodyMedium)
                    QrImage(created.link)
                    Text(created.code, style = MonoStyle.copy(fontSize = 24.sp), color = WolPalette.Text)
                }
            },
            confirmButton = { TextButton(onClick = onDismissCreated) { Text(stringResource(R.string.close)) } },
            dismissButton = { TextButton(onClick = { onCopy(created.link) }) { Text(stringResource(R.string.admin_copy_link)) } },
        )
    }
    state.failure?.let { failure ->
        AlertDialog(
            onDismissRequest = onDismissFailure,
            title = { Text(stringResource(R.string.admin_failed)) },
            text = { Text(failureText(failure)) },
            confirmButton = { TextButton(onClick = onDismissFailure) { Text(stringResource(R.string.ok)) } },
        )
    }
}

@Composable
private fun failureText(failure: AdminFailure): String = when (failure.code) {
    ResponseCode.LAST_ADMIN -> stringResource(R.string.admin_last_admin)
    ResponseCode.INSUFFICIENT_ROLE -> stringResource(R.string.admin_not_admin)
    else -> stringResource(failure.error.label()).replaceFirstChar { it.uppercase() }
}

@Composable
private fun DeviceRow(device: DeviceInfo, onClick: () -> Unit) {
    val via = when (device.via) {
        "pair" -> stringResource(R.string.admin_via_pair)
        "migrate" -> stringResource(R.string.admin_via_migrate)
        "local" -> stringResource(R.string.admin_via_local)
        else -> device.via
    }
    val meta = buildList {
        add(stringResource(R.string.admin_added, formatDay(device.added), via))
        if (device.seen > 0) add(stringResource(R.string.admin_seen, formatDayTime(device.seen)))
    }.joinToString(" · ")
    Row(
        Modifier
            .fillMaxWidth()
            .selectable(selected = false, onClick = onClick, role = Role.Button)
            .padding(horizontal = 14.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            if (device.self) WolIcons.Monitor else WolIcons.Users,
            contentDescription = null,
            tint = WolPalette.Text2,
            modifier = Modifier.size(18.dp),
        )
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(
                if (device.self) stringResource(R.string.admin_self, device.name) else device.name,
                style = MaterialTheme.typography.bodyLarge,
            )
            Text(Pin.parse(device.device)?.short().orEmpty(), style = MonoStyle.copy(fontSize = 12.sp), color = WolPalette.Text2)
            Text(meta, style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
        }
        Spacer(Modifier.width(12.dp))
        Text(roleLabel(device.role), style = MaterialTheme.typography.labelLarge, color = WolPalette.Blue)
    }
}

@Composable
private fun InviteRow(invite: InviteInfo, onCancel: () -> Unit) {
    Row(Modifier.fillMaxWidth().padding(start = 14.dp, end = 4.dp, top = 6.dp, bottom = 6.dp), verticalAlignment = Alignment.CenterVertically) {
        Icon(WolIcons.Clock, contentDescription = null, tint = WolPalette.Text2, modifier = Modifier.size(18.dp))
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(invite.name.ifEmpty { stringResource(R.string.admin_invite_unnamed) }, style = MaterialTheme.typography.bodyLarge)
            Text(
                stringResource(R.string.admin_invite_until, roleLabel(invite.role), formatDayTime(invite.expires)),
                style = MaterialTheme.typography.bodySmall,
                color = WolPalette.Text2,
            )
        }
        TextButton(onClick = onCancel) { Text(stringResource(R.string.admin_invite_cancel), color = WolPalette.DangerText) }
    }
}

@Composable
private fun RoleChoice(selected: Boolean, label: String, onSelect: () -> Unit) {
    Row(
        Modifier.fillMaxWidth().selectable(selected = selected, onClick = onSelect, role = Role.RadioButton),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(selected = selected, onClick = null)
        Text(label, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.padding(start = 8.dp))
    }
}

/** Appareil choisi : rôle, nom, retrait (retrait ou rétrogradation de ce téléphone confirmés). */
@Composable
private fun DeviceDialog(device: DeviceInfo, onChange: (AdminChange) -> Unit, onDismiss: () -> Unit) {
    var step by remember { mutableStateOf("main") }
    var pendingRole by remember { mutableStateOf(device.role) }
    var name by remember { mutableStateOf(device.name) }
    when (step) {
        "rename" -> AlertDialog(
            onDismissRequest = onDismiss,
            title = { Text(stringResource(R.string.admin_rename_title, device.name)) },
            text = {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text(stringResource(R.string.field_name)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
                )
            },
            confirmButton = {
                TextButton(onClick = { onDismiss(); onChange(AdminChange.Rename(device.device, name.trim())) }, enabled = name.isNotBlank()) {
                    Text(stringResource(R.string.save))
                }
            },
            dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
        )
        "revoke" -> AlertDialog(
            onDismissRequest = onDismiss,
            title = { Text(stringResource(R.string.admin_revoke_title, device.name)) },
            text = { Text(stringResource(if (device.self) R.string.admin_revoke_self_text else R.string.admin_revoke_text)) },
            confirmButton = {
                TextButton(onClick = { onDismiss(); onChange(AdminChange.Revoke(device.device)) }) {
                    Text(stringResource(R.string.admin_revoke), color = WolPalette.DangerText)
                }
            },
            dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
        )
        "demote" -> AlertDialog(
            onDismissRequest = onDismiss,
            title = { Text(stringResource(R.string.admin_demote_self_title, roleLabel(pendingRole))) },
            text = { Text(stringResource(R.string.admin_demote_self_text)) },
            confirmButton = {
                TextButton(onClick = { onDismiss(); onChange(AdminChange.SetRole(device.device, pendingRole)) }) {
                    Text(stringResource(R.string.admin_demote_self), color = WolPalette.DangerText)
                }
            },
            dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
        )
        else -> AlertDialog(
            onDismissRequest = onDismiss,
            title = { Text(if (device.self) stringResource(R.string.admin_self, device.name) else device.name) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(Pin.parse(device.device)?.short().orEmpty(), style = MonoStyle.copy(fontSize = 13.sp), color = WolPalette.Text2)
                    Text(stringResource(R.string.admin_role), style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 8.dp))
                    ROLES.forEach { role ->
                        RoleChoice(role == device.role, roleLabel(role)) {
                            when {
                                role == device.role -> Unit
                                // Ce téléphone renonce à son rôle d'administrateur : il ne pourra plus revenir ici.
                                device.self && device.role == "admin" -> {
                                    pendingRole = role
                                    step = "demote"
                                }
                                else -> {
                                    onDismiss()
                                    onChange(AdminChange.SetRole(device.device, role))
                                }
                            }
                        }
                    }
                    Row(Modifier.padding(top = 8.dp)) {
                        TextButton(onClick = { step = "rename" }) { Text(stringResource(R.string.admin_rename), color = WolPalette.Blue) }
                        TextButton(onClick = { step = "revoke" }) { Text(stringResource(R.string.admin_revoke), color = WolPalette.DangerText) }
                    }
                }
            },
            confirmButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.close)) } },
        )
    }
}

/** Nouvelle invitation : rôle, validité et nom facultatif. */
@Composable
private fun InviteDialog(onCreate: (String, Long, String) -> Unit, onDismiss: () -> Unit) {
    var role by rememberSaveable { mutableStateOf("control") }
    var ttl by rememberSaveable { mutableStateOf(600L) }
    var name by rememberSaveable { mutableStateOf("") }
    val ttlLabels = mapOf(
        600L to R.string.admin_ttl_600,
        3600L to R.string.admin_ttl_3600,
        86400L to R.string.admin_ttl_86400,
        604800L to R.string.admin_ttl_604800,
    )
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.admin_invite)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(stringResource(R.string.admin_invite_text), style = MaterialTheme.typography.bodyMedium)
                Text(stringResource(R.string.admin_invite_role), style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 10.dp))
                ROLES.forEach { r -> RoleChoice(r == role, roleLabel(r)) { role = r } }
                Text(stringResource(R.string.admin_invite_ttl), style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 10.dp))
                INVITE_TTLS.forEach { t -> RoleChoice(t == ttl, stringResource(ttlLabels.getValue(t))) { ttl = t } }
                Box(Modifier.padding(top = 10.dp)) {
                    OutlinedTextField(
                        value = name,
                        onValueChange = { name = it.take(40) },
                        label = { Text(stringResource(R.string.admin_invite_name)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
                    )
                }
            }
        },
        confirmButton = { TextButton(onClick = { onCreate(role, ttl, name) }) { Text(stringResource(R.string.admin_invite_create)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}
