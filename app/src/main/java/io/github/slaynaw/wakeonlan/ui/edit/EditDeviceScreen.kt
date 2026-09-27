package io.github.slaynaw.wakeonlan.ui.edit

import android.app.Activity
import android.view.WindowManager
import androidx.compose.animation.AnimatedVisibility
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
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.core.model.DeviceField
import io.github.slaynaw.wakeonlan.ui.common.ScanFailure
import io.github.slaynaw.wakeonlan.ui.common.label
import io.github.slaynaw.wakeonlan.ui.common.osLabel
import io.github.slaynaw.wakeonlan.ui.common.scanQrCode

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
        topBar = {
            TopAppBar(
                title = { Text(stringResource(if (state.isNew) R.string.edit_title_new else R.string.edit_title)) },
                navigationIcon = {
                    IconButton(onClick = onDone) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.back))
                    }
                },
                actions = {
                    if (!state.isNew) {
                        IconButton(onClick = { showDeleteDialog = true }) {
                            Icon(Icons.Default.Delete, contentDescription = stringResource(R.string.action_delete))
                        }
                    }
                    TextButton(onClick = vm::save, enabled = !state.loading) { Text(stringResource(R.string.save)) }
                },
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

            SectionTitle(stringResource(R.string.section_device))
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
            )
            FormField(
                value = form.host,
                onValueChange = { vm.onFormChange(form.copy(host = it)) },
                label = stringResource(R.string.field_host),
                supporting = stringResource(R.string.field_host_help),
                error = state.errors[DeviceField.HOST],
                keyboard = KeyboardOptions(keyboardType = KeyboardType.Uri),
            )

            HorizontalDivider(Modifier.padding(vertical = 4.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    SectionTitle(stringResource(R.string.section_agent))
                    Text(
                        stringResource(R.string.section_agent_help),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Switch(
                    checked = form.agentEnabled,
                    onCheckedChange = { vm.onFormChange(form.copy(agentEnabled = it)) },
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
                    FormField(
                        value = form.agentKey,
                        onValueChange = { vm.onFormChange(form.copy(agentKey = it)) },
                        label = stringResource(R.string.field_agent_key),
                        supporting = stringResource(R.string.field_agent_key_help),
                        error = state.errors[DeviceField.AGENT_KEY],
                        keyboard = KeyboardOptions(keyboardType = KeyboardType.Password),
                        visualTransformation = if (keyVisible) VisualTransformation.None else PasswordVisualTransformation(),
                        trailing = {
                            TextButton(onClick = { keyVisible = !keyVisible }) {
                                Text(stringResource(if (keyVisible) R.string.hide else R.string.show))
                            }
                        },
                    )
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        OutlinedButton(onClick = vm::testAgent, enabled = !state.testing) {
                            Text(stringResource(R.string.action_test_agent))
                        }
                        if (state.testing) {
                            Spacer(Modifier.width(12.dp))
                            CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                        }
                    }
                    state.testResult?.let { TestResultCard(it) }
                }
            }

            HorizontalDivider(Modifier.padding(vertical = 4.dp))
            TextButton(onClick = { showAdvanced = !showAdvanced }) {
                Text(stringResource(R.string.section_advanced))
                Icon(if (showAdvanced) Icons.Default.KeyboardArrowUp else Icons.Default.KeyboardArrowDown, null)
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
                    )
                }
            }

            Button(onClick = vm::save, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.save)) }
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
            text = { Text(state.pairingError ?: stringResource(R.string.pairing_ok_text)) },
            confirmButton = { TextButton(onClick = vm::dismissPairingFeedback) { Text(stringResource(R.string.ok)) } },
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
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.secondaryContainer),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.pairing_title), style = MaterialTheme.typography.titleSmall)
            Text(stringResource(R.string.pairing_text), style = MaterialTheme.typography.bodyMedium)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = onScan) {
                    Icon(painterResource(R.drawable.ic_qr_code_scanner), null, Modifier.size(18.dp))
                    Spacer(Modifier.width(8.dp))
                    Text(stringResource(R.string.action_scan_qr))
                }
                OutlinedButton(onClick = onPaste) { Text(stringResource(R.string.action_paste_link)) }
            }
        }
    }
}

@Composable
private fun TestResultCard(result: AgentTestResult) {
    val success = result is AgentTestResult.Success
    Card(
        colors = CardDefaults.cardColors(
            containerColor = if (success) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.errorContainer,
        ),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(Modifier.padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(if (success) Icons.Default.CheckCircle else Icons.Default.Warning, contentDescription = null)
            Spacer(Modifier.width(12.dp))
            Text(
                when (result) {
                    is AgentTestResult.Success -> stringResource(
                        R.string.test_ok,
                        result.status.hostname,
                        result.status.osLabel(),
                        result.status.version,
                    )
                    is AgentTestResult.Failure -> stringResource(result.error.label())
                    is AgentTestResult.Invalid -> result.message
                },
                style = MaterialTheme.typography.bodyMedium,
            )
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
private fun SectionTitle(text: String) {
    Text(text, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary)
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
        modifier = Modifier.fillMaxWidth(),
    )
}
