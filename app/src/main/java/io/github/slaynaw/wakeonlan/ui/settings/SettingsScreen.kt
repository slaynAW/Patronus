package io.github.slaynaw.wakeonlan.ui.settings

import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Slider
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.slaynaw.wakeonlan.BuildConfig
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.core.config.ExportCodec
import io.github.slaynaw.wakeonlan.core.model.AppSettings
import java.time.LocalDate

private const val REPO_URL = "https://github.com/slaynAW/WakeOnLan"
private const val RELEASES_URL = "$REPO_URL/releases"

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(onBack: () -> Unit) {
    val context = LocalContext.current
    val resources = LocalResources.current
    val container = context.appContainer
    val vm: SettingsViewModel = viewModel { SettingsViewModel(container) }
    val state by vm.state.collectAsStateWithLifecycle()
    val busy by vm.busy.collectAsStateWithLifecycle()
    val importStep by vm.importStep.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }

    var showExportDialog by remember { mutableStateOf(false) }
    // Mot de passe choisi, conservé le temps que l'utilisateur choisisse le fichier de destination.
    var pendingExportPassword by remember { mutableStateOf<CharArray?>(null) }

    val exportLauncher = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("application/json")) { uri ->
        val password = pendingExportPassword
        pendingExportPassword = null
        if (uri != null) vm.export(context.contentResolver, uri, password) else password?.fill(' ')
    }
    val importLauncher = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) vm.startImport(context.contentResolver, uri)
    }

    LaunchedEffect(vm) {
        vm.messages.collect { message ->
            snackbar.showSnackbar(resources.getString(message.text, *message.args.toTypedArray()))
        }
    }

    fun openUrl(url: String) {
        try {
            context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
        } catch (_: ActivityNotFoundException) {
            // Aucun navigateur : rien à faire.
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.settings_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = stringResource(R.string.back))
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState()),
        ) {
            if (busy) LinearProgressIndicator(Modifier.fillMaxWidth())

            Header(stringResource(R.string.section_monitoring))
            SliderSetting(
                title = stringResource(R.string.setting_poll_interval),
                valueLabel = { stringResource(R.string.setting_poll_interval_value, it) },
                value = state.settings.pollIntervalSeconds,
                range = AppSettings.POLL_INTERVAL_RANGE.first..30,
                step = 1,
                onChange = { v -> vm.updateSettings { it.copy(pollIntervalSeconds = v) } },
            )
            SliderSetting(
                title = stringResource(R.string.setting_wake_timeout),
                valueLabel = { stringResource(R.string.setting_wake_timeout_value, it) },
                value = state.settings.wakeTimeoutSeconds,
                range = 60..600,
                step = 30,
                onChange = { v -> vm.updateSettings { it.copy(wakeTimeoutSeconds = v) } },
            )
            ListItem(
                headlineContent = { Text(stringResource(R.string.setting_confirm)) },
                supportingContent = { Text(stringResource(R.string.setting_confirm_help)) },
                trailingContent = {
                    Switch(
                        checked = state.settings.confirmPowerActions,
                        onCheckedChange = { checked -> vm.updateSettings { it.copy(confirmPowerActions = checked) } },
                    )
                },
            )

            HorizontalDivider()
            Header(stringResource(R.string.section_backup))
            ListItem(
                headlineContent = { Text(stringResource(R.string.action_export)) },
                supportingContent = { Text(stringResource(R.string.action_export_help, state.deviceCount)) },
                modifier = Modifier.clickableItem(enabled = !busy && state.deviceCount > 0) { showExportDialog = true },
            )
            ListItem(
                headlineContent = { Text(stringResource(R.string.action_import)) },
                supportingContent = { Text(stringResource(R.string.action_import_help)) },
                modifier = Modifier.clickableItem(enabled = !busy) {
                    importLauncher.launch(arrayOf("application/json", "text/*", "application/octet-stream"))
                },
            )

            HorizontalDivider()
            Header(stringResource(R.string.section_agent_download))
            ListItem(
                headlineContent = { Text(stringResource(R.string.agent_download)) },
                supportingContent = { Text(stringResource(R.string.agent_download_help)) },
                modifier = Modifier.clickableItem { openUrl(RELEASES_URL) },
            )

            HorizontalDivider()
            Header(stringResource(R.string.section_about))
            ListItem(
                headlineContent = { Text(stringResource(R.string.about_version)) },
                supportingContent = { Text(BuildConfig.VERSION_NAME) },
            )
            ListItem(
                headlineContent = { Text(stringResource(R.string.about_source)) },
                supportingContent = { Text(REPO_URL) },
                modifier = Modifier.clickableItem { openUrl(REPO_URL) },
            )
            ListItem(
                headlineContent = { Text(stringResource(R.string.about_security)) },
                supportingContent = { Text(stringResource(R.string.about_security_text)) },
            )
        }
    }

    if (showExportDialog) {
        ExportDialog(
            hasSecrets = state.hasSecrets,
            onConfirm = { password ->
                showExportDialog = false
                pendingExportPassword = password
                exportLauncher.launch("wakeonlan-${LocalDate.now()}.json")
            },
            onDismiss = { showExportDialog = false },
        )
    }

    when (val step = importStep) {
        is ImportStep.NeedPassword -> ImportPasswordDialog(
            wrongPassword = step.wrongPassword,
            busy = busy,
            onConfirm = vm::submitPassword,
            onDismiss = vm::cancelImport,
        )
        is ImportStep.Confirm -> AlertDialog(
            onDismissRequest = vm::cancelImport,
            title = { Text(stringResource(R.string.import_confirm_title, step.config.devices.size)) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(stringResource(R.string.import_confirm_text))
                    if (step.missingKeys) {
                        Text(stringResource(R.string.import_missing_keys), color = MaterialTheme.colorScheme.error)
                    }
                }
            },
            confirmButton = {
                Row {
                    TextButton(onClick = { vm.confirmImport(replace = true) }) { Text(stringResource(R.string.import_replace)) }
                    TextButton(onClick = { vm.confirmImport(replace = false) }) { Text(stringResource(R.string.import_merge)) }
                }
            },
            dismissButton = { TextButton(onClick = vm::cancelImport) { Text(stringResource(R.string.cancel)) } },
        )
        null -> Unit
    }
}

@Composable
private fun Header(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.titleSmall,
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 20.dp, bottom = 4.dp),
    )
}

@Composable
private fun SliderSetting(
    title: String,
    valueLabel: @Composable (Int) -> String,
    value: Int,
    range: IntRange,
    step: Int,
    onChange: (Int) -> Unit,
) {
    // Valeur locale pendant le glissement ; enregistrement uniquement au relâchement.
    var current by remember(value) { mutableFloatStateOf(value.coerceIn(range).toFloat()) }
    Column(Modifier.padding(horizontal = 16.dp, vertical = 8.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(title, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
            Text(valueLabel(current.toInt()), style = MaterialTheme.typography.labelLarge)
        }
        Slider(
            value = current,
            onValueChange = { current = it },
            onValueChangeFinished = { onChange(current.toInt()) },
            valueRange = range.first.toFloat()..range.last.toFloat(),
            steps = ((range.last - range.first) / step - 1).coerceAtLeast(0),
        )
    }
}

@Composable
private fun ExportDialog(hasSecrets: Boolean, onConfirm: (CharArray?) -> Unit, onDismiss: () -> Unit) {
    var withSecrets by remember { mutableStateOf(hasSecrets) }
    var password by remember { mutableStateOf("") }
    var confirmation by remember { mutableStateOf("") }
    val tooShort = password.length < ExportCodec.MIN_PASSWORD_LENGTH
    val mismatch = password != confirmation
    val valid = !withSecrets || (!tooShort && !mismatch)

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.export_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                ChoiceRow(
                    selected = withSecrets,
                    title = stringResource(R.string.export_with_secrets),
                    subtitle = stringResource(R.string.export_with_secrets_help),
                    onSelect = { withSecrets = true },
                )
                ChoiceRow(
                    selected = !withSecrets,
                    title = stringResource(R.string.export_without_secrets),
                    subtitle = stringResource(R.string.export_without_secrets_help),
                    onSelect = { withSecrets = false },
                )
                if (withSecrets) {
                    OutlinedTextField(
                        value = password,
                        onValueChange = { password = it },
                        label = { Text(stringResource(R.string.field_password)) },
                        singleLine = true,
                        isError = password.isNotEmpty() && tooShort,
                        supportingText = { Text(stringResource(R.string.field_password_help, ExportCodec.MIN_PASSWORD_LENGTH)) },
                        visualTransformation = PasswordVisualTransformation(),
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                    )
                    OutlinedTextField(
                        value = confirmation,
                        onValueChange = { confirmation = it },
                        label = { Text(stringResource(R.string.field_password_confirm)) },
                        singleLine = true,
                        isError = confirmation.isNotEmpty() && mismatch,
                        visualTransformation = PasswordVisualTransformation(),
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(if (withSecrets) password.toCharArray() else null) },
                enabled = valid,
            ) { Text(stringResource(R.string.action_export_short)) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun ImportPasswordDialog(
    wrongPassword: Boolean,
    busy: Boolean,
    onConfirm: (CharArray) -> Unit,
    onDismiss: () -> Unit,
) {
    var password by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.import_password_title)) },
        text = {
            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                label = { Text(stringResource(R.string.field_password)) },
                singleLine = true,
                isError = wrongPassword,
                supportingText = if (wrongPassword) {
                    { Text(stringResource(R.string.import_wrong_password)) }
                } else {
                    null
                },
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
            )
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(password.toCharArray()) }, enabled = password.isNotEmpty() && !busy) {
                Text(stringResource(R.string.ok))
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun ChoiceRow(selected: Boolean, title: String, subtitle: String, onSelect: () -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .selectable(selected = selected, onClick = onSelect, role = Role.RadioButton),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(selected = selected, onClick = null)
        Column(Modifier.padding(start = 8.dp)) {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            Text(subtitle, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

private fun Modifier.clickableItem(enabled: Boolean = true, onClick: () -> Unit): Modifier =
    clickable(enabled = enabled, onClick = onClick)
