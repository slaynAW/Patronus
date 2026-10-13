package io.github.slaynaw.wakeonlan.ui.edit

import android.app.Activity
import android.view.WindowManager
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent2.PairingCode
import io.github.slaynaw.wakeonlan.core.agent2.Wolagent2
import io.github.slaynaw.wakeonlan.core.model.DeviceField
import io.github.slaynaw.wakeonlan.ui.common.BusySpinner
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.common.ScanFailure
import io.github.slaynaw.wakeonlan.ui.common.SectionLabel
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.label
import io.github.slaynaw.wakeonlan.ui.common.osLabel
import io.github.slaynaw.wakeonlan.ui.common.scanQrCode
import io.github.slaynaw.wakeonlan.ui.theme.MonoStyle
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EditDeviceScreen(deviceId: String?, onDone: () -> Unit) {
    val context = LocalContext.current
    val container = context.appContainer
    val vm: EditDeviceViewModel = viewModel(key = "edit-${deviceId ?: "new"}") { EditDeviceViewModel(container, deviceId) }
    val state by vm.state.collectAsStateWithLifecycle()
    val form = state.form

    var showPasteDialog by rememberSaveable { mutableStateOf(false) }
    var showDeleteDialog by rememberSaveable { mutableStateOf(false) }
    var showAdvanced by rememberSaveable { mutableStateOf(false) }
    var keyVisible by rememberSaveable { mutableStateOf(false) }
    var scanFailure by remember { mutableStateOf<ScanFailure?>(null) }

    LaunchedEffect(state.done) { if (state.done) onDone() }

    // Clé affichée en clair : on interdit les captures d'écran le temps de l'affichage.
    if (keyVisible) {
        DisposableEffect(Unit) {
            val window = (context as? Activity)?.window
            window?.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
            onDispose { window?.clearFlags(WindowManager.LayoutParams.FLAG_SECURE) }
        }
    }

    val scan = {
        scanQrCode(
            context,
            onResult = vm::applyPairing,
            onError = { failure -> scanFailure = failure },
        )
    }

    Scaffold(
        containerColor = WolPalette.Background,
        topBar = {
            TopAppBar(
                title = { Text(stringResource(if (state.isNew) R.string.edit_title_new else R.string.edit_title)) },
                navigationIcon = {
                    IconButton(onClick = onDone) {
                        Icon(WolIcons.ArrowBack, contentDescription = stringResource(R.string.back))
                    }
                },
                actions = {
                    if (!state.isNew) {
                        IconButton(onClick = { showDeleteDialog = true }) {
                            Icon(WolIcons.Delete, contentDescription = stringResource(R.string.action_delete), tint = WolPalette.DangerText)
                        }
                    }
                    TextButton(onClick = vm::save, enabled = !state.loading && !state.saving) {
                        Text(stringResource(R.string.save), color = WolPalette.Blue, style = MaterialTheme.typography.labelLarge)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = WolPalette.Background),
            )
        },
    ) { padding ->
        if (state.loading) return@Scaffold
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            PairingCard(onScan = scan, onPaste = { showPasteDialog = true })

            SectionLabel(stringResource(R.string.section_device), Modifier.padding(top = 6.dp))
            FormField(
                value = form.name,
                onValueChange = { vm.onFormChange(form.copy(name = it)) },
                label = stringResource(R.string.field_name),
                error = state.errors[DeviceField.NAME],
                keyboard = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
            )
            FormField(
                value = form.mac,
                onValueChange = { vm.onFormChange(form.copy(mac = it)) },
                label = stringResource(R.string.field_mac),
                supporting = stringResource(R.string.field_mac_help),
                error = state.errors[DeviceField.MAC],
                keyboard = KeyboardOptions(capitalization = KeyboardCapitalization.Characters, keyboardType = KeyboardType.Ascii),
                mono = true,
            )
            FormField(
                value = form.host,
                onValueChange = { vm.onFormChange(form.copy(host = it)) },
                label = stringResource(R.string.field_host),
                supporting = stringResource(R.string.field_host_help),
                error = state.errors[DeviceField.HOST],
                keyboard = KeyboardOptions(keyboardType = KeyboardType.Uri),
            )

            HorizontalDivider(Modifier.padding(vertical = 4.dp), color = WolPalette.Line)
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(stringResource(R.string.section_agent), style = MaterialTheme.typography.titleSmall)
                    Text(
                        stringResource(R.string.section_agent_help),
                        style = MaterialTheme.typography.bodySmall,
                        color = WolPalette.Text2,
                    )
                }
                Spacer(Modifier.width(12.dp))
                Switch(
                    checked = form.agentEnabled,
                    onCheckedChange = { vm.onFormChange(form.copy(agentEnabled = it)) },
                    colors = SwitchDefaults.colors(
                        checkedThumbColor = Color.White,
                        checkedTrackColor = WolPalette.Blue,
                        uncheckedThumbColor = WolPalette.Text2,
                        uncheckedTrackColor = WolPalette.Surface3,
                        uncheckedBorderColor = WolPalette.Line2,
                    ),
                )
            }
            AnimatedVisibility(form.agentEnabled) {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    FormField(
                        value = form.agentPort,
                        onValueChange = { vm.onFormChange(form.copy(agentPort = it)) },
                        label = stringResource(R.string.field_agent_port),
                        error = state.errors[DeviceField.AGENT_PORT],
                        keyboard = KeyboardOptions(keyboardType = KeyboardType.Number),
                    )
                    // Lien v=2 : ni clé ni test, l'appairage se fait à l'enregistrement.
                    state.pairing?.let { PendingPairingCard(it, onCancel = vm::cancelPairing) }
                    if (state.pairing == null) {
                        FormField(
                            value = form.agentKey,
                            onValueChange = { vm.onFormChange(form.copy(agentKey = it)) },
                            label = stringResource(R.string.field_agent_key),
                            supporting = state.agentPin?.let { stringResource(R.string.field_agent_key_help_v2, it) }
                                ?: stringResource(R.string.field_agent_key_help),
                            error = state.errors[DeviceField.AGENT_KEY],
                            keyboard = KeyboardOptions(keyboardType = KeyboardType.Password),
                            visualTransformation = if (keyVisible) VisualTransformation.None else PasswordVisualTransformation(),
                            trailing = {
                                TextButton(onClick = { keyVisible = !keyVisible }) {
                                    Text(stringResource(if (keyVisible) R.string.hide else R.string.show), color = WolPalette.Blue)
                                }
                            },
                            mono = true,
                        )
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            WolButton(
                                stringResource(R.string.action_test_agent),
                                onClick = vm::testAgent,
                                enabled = !state.testing,
                                icon = WolIcons.Shield,
                            )
                            if (state.testing) {
                                Spacer(Modifier.width(12.dp))
                                BusySpinner()
                            }
                        }
                        state.testResult?.let { TestResultCard(it) }
                    }
                }
            }

            HorizontalDivider(Modifier.padding(vertical = 4.dp), color = WolPalette.Line)
            TextButton(onClick = { showAdvanced = !showAdvanced }) {
                Text(stringResource(R.string.section_advanced), color = WolPalette.Blue)
                Spacer(Modifier.width(6.dp))
                Icon(
                    if (showAdvanced) WolIcons.ChevronUp else WolIcons.ChevronDown,
                    contentDescription = null,
                    tint = WolPalette.Blue,
                    modifier = Modifier.size(16.dp),
                )
            }
            AnimatedVisibility(showAdvanced) {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    FormField(
                        value = form.broadcast,
                        onValueChange = { vm.onFormChange(form.copy(broadcast = it)) },
                        label = stringResource(R.string.field_broadcast),
                        supporting = stringResource(R.string.field_broadcast_help),
                        error = state.errors[DeviceField.BROADCAST],
                        keyboard = KeyboardOptions(keyboardType = KeyboardType.Uri),
                    )
                    FormField(
                        value = form.wolPort,
                        onValueChange = { vm.onFormChange(form.copy(wolPort = it)) },
                        label = stringResource(R.string.field_wol_port),
                        supporting = stringResource(R.string.field_wol_port_help),
                        error = state.errors[DeviceField.WOL_PORT],
                        keyboard = KeyboardOptions(keyboardType = KeyboardType.Number),
                    )
                    FormField(
                        value = form.probePorts,
                        onValueChange = { vm.onFormChange(form.copy(probePorts = it)) },
                        label = stringResource(R.string.field_probe_ports),
                        supporting = stringResource(R.string.field_probe_ports_help),
                        error = state.errors[DeviceField.PROBE_PORTS],
                        keyboard = KeyboardOptions(keyboardType = KeyboardType.Ascii),
                    )
                    FormField(
                        value = form.secureOn,
                        onValueChange = { vm.onFormChange(form.copy(secureOn = it)) },
                        label = stringResource(R.string.field_secure_on),
                        supporting = stringResource(R.string.field_secure_on_help),
                        error = state.errors[DeviceField.SECURE_ON],
                        keyboard = KeyboardOptions(capitalization = KeyboardCapitalization.Characters, keyboardType = KeyboardType.Ascii),
                        mono = true,
                    )
                }
            }

            Row(verticalAlignment = Alignment.CenterVertically) {
                WolButton(
                    stringResource(R.string.save),
                    onClick = vm::save,
                    modifier = Modifier.weight(1f),
                    kind = ButtonKind.PRIMARY,
                    enabled = !state.saving,
                    height = 48.dp,
                )
                if (state.saving) {
                    Spacer(Modifier.width(12.dp))
                    BusySpinner()
                }
            }
            Spacer(Modifier.height(24.dp))
        }
    }

    if (showPasteDialog) {
        PasteLinkDialog(
            onConfirm = {
                showPasteDialog = false
                vm.applyPairing(it)
            },
            onDismiss = { showPasteDialog = false },
        )
    }

    if (state.pairingApplied || state.pairingError != null) {
        AlertDialog(
            onDismissRequest = vm::dismissPairingFeedback,
            title = {
                Text(stringResource(if (state.pairingApplied) R.string.pairing_ok_title else R.string.pairing_error_title))
            },
            text = {
                Text(
                    state.pairingError
                        ?: stringResource(if (state.pairing != null) R.string.pairing_ok_text_v2 else R.string.pairing_ok_text),
                )
            },
            confirmButton = { TextButton(onClick = vm::dismissPairingFeedback) { Text(stringResource(R.string.ok)) } },
        )
    }

    state.pairingFailure?.let { error ->
        AlertDialog(
            onDismissRequest = vm::dismissPairingFailure,
            title = { Text(stringResource(R.string.pairing_failed_title)) },
            text = { Text(pairingFailureText(error)) },
            confirmButton = { TextButton(onClick = vm::dismissPairingFailure) { Text(stringResource(R.string.ok)) } },
        )
    }

    scanFailure?.let { failure ->
        AlertDialog(
            onDismissRequest = { scanFailure = null },
            title = {
                Text(
                    stringResource(
                        if (failure is ScanFailure.ModuleDownloading) R.string.scan_module_title else R.string.scan_error_title,
                    ),
                )
            },
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

    if (showDeleteDialog) {
        AlertDialog(
            onDismissRequest = { showDeleteDialog = false },
            title = { Text(stringResource(R.string.confirm_delete_title, form.name)) },
            text = { Text(stringResource(R.string.confirm_delete_text)) },
            confirmButton = {
                TextButton(onClick = {
                    showDeleteDialog = false
                    vm.delete()
                }) { Text(stringResource(R.string.action_delete)) }
            },
            dismissButton = { TextButton(onClick = { showDeleteDialog = false }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
private fun PairingCard(onScan: () -> Unit, onPaste: () -> Unit) {
    val shape = RoundedCornerShape(14.dp)
    Column(
        Modifier
            .fillMaxWidth()
            .clip(shape)
            .background(Brush.linearGradient(listOf(Color(0xFF13243D), Color(0xFF161D2B))))
            .border(1.dp, Color(0xFF22385A), shape)
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Text(stringResource(R.string.pairing_title), style = MaterialTheme.typography.titleSmall)
        Text(stringResource(R.string.pairing_text), style = MaterialTheme.typography.bodyMedium, color = Color(0xFFB6C3D6))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(
                onClick = onScan,
                shape = RoundedCornerShape(10.dp),
                colors = ButtonDefaults.buttonColors(containerColor = WolPalette.Blue, contentColor = Color.White),
            ) {
                Icon(painterResource(R.drawable.ic_qr_code_scanner), null, Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text(stringResource(R.string.action_scan_qr), style = MaterialTheme.typography.labelLarge)
            }
            WolButton(stringResource(R.string.action_paste_link), onClick = onPaste, icon = WolIcons.Paste, height = 40.dp)
        }
    }
}

/** Cause d'un appairage refusé, avec ce qu'il faut faire (§7.6 pour un code déjà utilisé). */
@Composable
private fun pairingFailureText(error: AgentError): String = when (error) {
    AgentError.PAIRING_FAILED -> stringResource(R.string.pairing_error_failed)
    AgentError.PIN_MISMATCH -> stringResource(R.string.pairing_error_pin_mismatch)
    AgentError.UNREACHABLE -> stringResource(error.label()).replaceFirstChar { it.uppercase() } + ". " +
        stringResource(R.string.hint_unreachable)
    else -> stringResource(error.label()).replaceFirstChar { it.uppercase() }
}

/** Rôle annoncé par un lien d'appairage, tel que l'affiche l'application. */
@Composable
internal fun roleLabel(wire: String): String = when (wire) {
    "view" -> stringResource(R.string.role_view)
    "wake" -> stringResource(R.string.role_wake)
    "control" -> stringResource(R.string.role_control)
    "admin" -> stringResource(R.string.role_admin)
    else -> wire
}

private val clockFormat = DateTimeFormatter.ofPattern("HH:mm", Locale.FRENCH)
private val dayClockFormat = DateTimeFormatter.ofPattern("d MMM HH:mm", Locale.FRENCH)

/** Appairage wolagent/2 en attente : code, empreinte de l'agent et rôle annoncés par le lien. */
@Composable
internal fun PendingPairingCard(pairing: PendingPairing, onCancel: () -> Unit, now: Instant = Instant.now()) {
    val zone = ZoneId.systemDefault()
    val until = Instant.ofEpochSecond(pairing.expires).atZone(zone)
    val untilText = if (until.toLocalDate() == now.atZone(zone).toLocalDate()) clockFormat.format(until) else dayClockFormat.format(until)
    Card(
        colors = CardDefaults.cardColors(containerColor = WolPalette.SuccessBackground, contentColor = WolPalette.SuccessText),
        shape = RoundedCornerShape(10.dp),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(Modifier.padding(start = 12.dp, top = 12.dp, bottom = 12.dp), verticalAlignment = Alignment.Top) {
            Icon(WolIcons.Shield, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(stringResource(R.string.pairing_v2_title), style = MaterialTheme.typography.titleSmall)
                Text(
                    stringResource(
                        R.string.pairing_v2_text,
                        PairingCode.format(pairing.code),
                        pairing.agentShort,
                        roleLabel(pairing.role ?: "admin"),
                    ),
                    style = MaterialTheme.typography.bodySmall,
                )
                if (pairing.expires > 0) {
                    Text(stringResource(R.string.pairing_v2_until, untilText), style = MaterialTheme.typography.bodySmall)
                }
            }
            TextButton(onClick = onCancel) { Text(stringResource(R.string.pairing_v2_cancel), color = WolPalette.Blue) }
        }
    }
}

@Composable
private fun TestResultCard(result: AgentTestResult) {
    val success = result is AgentTestResult.Success
    val content = if (success) WolPalette.SuccessText else WolPalette.DangerText
    Card(
        colors = CardDefaults.cardColors(
            containerColor = if (success) WolPalette.SuccessBackground else WolPalette.DangerBackground,
            contentColor = content,
        ),
        shape = RoundedCornerShape(10.dp),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(if (success) WolIcons.Check else WolIcons.Warning, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(12.dp))
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    when (result) {
                        is AgentTestResult.Success -> stringResource(
                            if (result.status.proto == Wolagent2.ALPN) R.string.test_ok_v2 else R.string.test_ok,
                            result.status.hostname,
                            result.status.osLabel(),
                            result.status.version,
                        )
                        is AgentTestResult.Failure -> stringResource(result.error.label())
                        is AgentTestResult.Invalid -> result.message
                    },
                    style = MaterialTheme.typography.bodyMedium,
                )
                if (result is AgentTestResult.Failure && result.error == AgentError.UNREACHABLE) {
                    Text(stringResource(R.string.hint_unreachable), style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
}

@Composable
private fun PasteLinkDialog(onConfirm: (String) -> Unit, onDismiss: () -> Unit) {
    var text by rememberSaveable { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.paste_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.paste_text), style = MaterialTheme.typography.bodyMedium)
                OutlinedTextField(
                    value = text,
                    onValueChange = { text = it },
                    placeholder = { Text("wolagent://pair?…") },
                    minLines = 2,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                )
            }
        },
        confirmButton = { TextButton(onClick = { onConfirm(text) }, enabled = text.isNotBlank()) { Text(stringResource(R.string.ok)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun FormField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    error: String?,
    keyboard: KeyboardOptions,
    supporting: String? = null,
    visualTransformation: VisualTransformation = VisualTransformation.None,
    trailing: (@Composable () -> Unit)? = null,
    mono: Boolean = false,
) {
    val helper = error ?: supporting
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        label = { Text(label) },
        isError = error != null,
        supportingText = if (helper != null) {
            { Text(helper) }
        } else {
            null
        },
        singleLine = true,
        keyboardOptions = keyboard,
        visualTransformation = visualTransformation,
        trailingIcon = trailing,
        textStyle = if (mono) MonoStyle.copy(fontSize = 15.sp) else MaterialTheme.typography.bodyLarge,
        shape = RoundedCornerShape(10.dp),
        colors = OutlinedTextFieldDefaults.colors(
            focusedContainerColor = WolPalette.Background,
            unfocusedContainerColor = WolPalette.Background,
            focusedBorderColor = WolPalette.Blue,
            unfocusedBorderColor = WolPalette.Line2,
            focusedLabelColor = WolPalette.Blue,
            unfocusedLabelColor = WolPalette.Text2,
            unfocusedSupportingTextColor = WolPalette.Text3,
            focusedSupportingTextColor = WolPalette.Text3,
        ),
        modifier = Modifier.fillMaxWidth(),
    )
}
