package io.github.slaynaw.wakeonlan.ui.settings

import android.content.ActivityNotFoundException
import android.content.Intent
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import io.github.slaynaw.wakeonlan.BuildConfig
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.core.config.ExportCodec
import io.github.slaynaw.wakeonlan.core.model.AppSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.share.ShareException
import io.github.slaynaw.wakeonlan.core.share.ShareLinks
import io.github.slaynaw.wakeonlan.share.ShareUiState
import io.github.slaynaw.wakeonlan.ui.common.rememberNow
import kotlinx.coroutines.flow.map
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.SectionLabel
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.formatDuration
import io.github.slaynaw.wakeonlan.ui.overview.ScreenHeader
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import io.github.slaynaw.wakeonlan.update.UpdateUiState
import kotlinx.coroutines.launch
import java.time.LocalDate

private const val REPO_URL = "https://github.com/slaynAW/WakeOnLan"
private const val RELEASES_URL = "$REPO_URL/releases"

/** Onglet « Réglages » : surveillance, sauvegarde, historique, agent, à propos. */
@Composable
fun SettingsTab(
    contentPadding: PaddingValues,
    snackbar: SnackbarHostState,
    onOpenHistory: () -> Unit,
    onShowUpdate: () -> Unit = {},
) {
    val context = LocalContext.current
    val resources = LocalResources.current
    val container = context.appContainer
    val vm: SettingsViewModel = viewModel { SettingsViewModel(container) }
    val state by vm.state.collectAsStateWithLifecycle()
    val update by container.updater.state.collectAsStateWithLifecycle()
    val share by container.share.state.collectAsStateWithLifecycle()
    val ownDevices by remember(container) { container.repository.config.map { it.devices } }.collectAsStateWithLifecycle(emptyList())
    val pendingLink by container.share.pendingLink.collectAsStateWithLifecycle()
    var shareDialog by remember { mutableStateOf<ShareDialog?>(null) }
    val now = rememberNow(periodMs = 30_000)
    val scope = rememberCoroutineScope()
    val busy by vm.busy.collectAsStateWithLifecycle()
    val importStep by vm.importStep.collectAsStateWithLifecycle()
    val sharingImported by vm.sharingImported.collectAsStateWithLifecycle()

    var showExportDialog by remember { mutableStateOf(false) }
    var showClearHistory by remember { mutableStateOf(false) }
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

    // Lien de partage ouvert depuis un message : invitation (demander l'accès) ou demande (autoriser).
    LaunchedEffect(pendingLink, share.loaded) {
        val link = pendingLink ?: return@LaunchedEffect
        if (!share.loaded) return@LaunchedEffect
        container.share.pendingLink.value = null
        shareDialog = try {
            if (ShareLinks.kind(link) == "request") {
                val info = container.share.readRequest(link)
                ShareDialog.Rights(info.request.name, info.request.device, info.code, info.rights.orEmpty(), isNew = info.rights == null)
            } else {
                ShareDialog.Name(container.share.readInvite(link))
            }
        } catch (e: ShareException) {
            ShareDialog.Error(e.message.orEmpty().replaceFirstChar { it.uppercase() })
        }
    }

    fun openUrl(url: String) {
        try {
            context.startActivity(Intent(Intent.ACTION_VIEW, url.toUri()))
        } catch (_: ActivityNotFoundException) {
            // Aucun navigateur : rien à faire.
        }
    }

    SettingsContent(
        contentPadding = contentPadding,
        settings = state.settings,
        deviceCount = state.deviceCount,
        busy = busy,
        version = BuildConfig.VERSION_NAME,
        onUpdateSettings = { vm.updateSettings(it) },
        onExport = { showExportDialog = true },
        onImport = { importLauncher.launch(arrayOf("application/json", "text/*", "application/octet-stream")) },
        onOpenHistory = onOpenHistory,
        onClearHistory = { showClearHistory = true },
        onOpenUrl = ::openUrl,
        update = update,
        onCheckUpdate = {
            if (update.available != null) {
                onShowUpdate()
            } else {
                scope.launch {
                    val result = container.updater.check()
                    when {
                        result.available != null -> onShowUpdate()
                        result.error == null -> snackbar.showSnackbar(resources.getString(R.string.update_up_to_date))
                    }
                }
            }
        },
        onAutoUpdate = container.updater::setAuto,
        share = share,
        ownDevices = ownDevices,
        now = now,
        onShareDialog = { shareDialog = it },
    )

    if (sharingImported) {
        AlertDialog(
            onDismissRequest = vm::dismissSharingImported,
            icon = { Icon(WolIcons.Share, contentDescription = null, tint = WolPalette.Blue) },
            title = { Text(stringResource(R.string.section_share_mine)) },
            text = { Text(stringResource(R.string.import_sharing_done)) },
            confirmButton = { TextButton(onClick = vm::dismissSharingImported) { Text(stringResource(R.string.ok)) } },
        )
    }

    ShareDialogHost(
        dialog = shareDialog,
        onDialog = { shareDialog = it },
        share = share,
        ownDevices = ownDevices,
        manager = container.share,
        snackbar = snackbar,
    )

    if (showExportDialog) {
        ExportDialog(
            hasSecrets = state.hasSecrets,
            onConfirm = { password ->
                showExportDialog = false
                pendingExportPassword = password
                exportLauncher.launch("patronus-${LocalDate.now()}.json")
            },
            onDismiss = { showExportDialog = false },
        )
    }

    if (showClearHistory) {
        AlertDialog(
            onDismissRequest = { showClearHistory = false },
            icon = { Icon(WolIcons.History, contentDescription = null, tint = WolPalette.DangerText) },
            title = { Text(stringResource(R.string.history_clear_title)) },
            text = { Text(stringResource(R.string.history_clear_text)) },
            confirmButton = {
                WolButton(
                    text = stringResource(R.string.history_clear_action),
                    kind = ButtonKind.DANGER,
                    onClick = {
                        showClearHistory = false
                        vm.clearHistory()
                    },
                )
            },
            dismissButton = { TextButton(onClick = { showClearHistory = false }) { Text(stringResource(R.string.cancel)) } },
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
            icon = { Icon(WolIcons.Upload, contentDescription = null, tint = WolPalette.Blue) },
            title = { Text(stringResource(R.string.import_confirm_title, step.config.devices.size)) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(stringResource(R.string.import_confirm_text))
                    if (step.missingKeys) {
                        Text(stringResource(R.string.import_missing_keys), color = WolPalette.DangerText)
                    }
                    step.sharing?.let { Text(stringResource(R.string.import_sharing, it.name, it.people.size)) }
                    step.history?.let { Text(stringResource(R.string.import_history, it.events.size)) }
                }
            },
            confirmButton = {
                Row {
                    TextButton(onClick = { vm.confirmImport(replace = true) }) {
                        Text(stringResource(R.string.import_replace), color = WolPalette.DangerText)
                    }
                    TextButton(onClick = { vm.confirmImport(replace = false) }) { Text(stringResource(R.string.import_merge)) }
                }
            },
            dismissButton = { TextButton(onClick = vm::cancelImport) { Text(stringResource(R.string.cancel)) } },
        )
        null -> Unit
    }
}

/** Contenu de l'onglet (sans état propre : aperçus et captures d'écran). */
@Composable
fun SettingsContent(
    contentPadding: PaddingValues,
    settings: AppSettings,
    deviceCount: Int,
    busy: Boolean,
    version: String,
    onUpdateSettings: ((AppSettings) -> AppSettings) -> Unit,
    onExport: () -> Unit,
    onImport: () -> Unit,
    onOpenHistory: () -> Unit,
    onClearHistory: () -> Unit,
    onOpenUrl: (String) -> Unit,
    update: UpdateUiState = UpdateUiState(),
    onCheckUpdate: () -> Unit = {},
    onAutoUpdate: (Boolean) -> Unit = {},
    share: ShareUiState = ShareUiState(loaded = true),
    ownDevices: List<Device> = emptyList(),
    now: Long = System.currentTimeMillis(),
    onShareDialog: (ShareDialog) -> Unit = {},
) {
    Column(
        Modifier
            .fillMaxSize()
            .padding(contentPadding)
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Spacer(Modifier.size(2.dp))
        ScreenHeader(stringResource(R.string.tab_settings))
        if (busy) LinearProgressIndicator(Modifier.fillMaxWidth(), color = WolPalette.Blue, trackColor = WolPalette.BlueSoft)

        SectionLabel(stringResource(R.string.section_monitoring), Modifier.padding(top = 6.dp))
        WolCard {
            SliderSetting(
                title = stringResource(R.string.setting_poll_interval),
                valueLabel = { stringResource(R.string.setting_poll_interval_value, it) },
                value = settings.pollIntervalSeconds,
                range = AppSettings.POLL_INTERVAL_RANGE.first..30,
                step = 1,
                onChange = { v -> onUpdateSettings { it.copy(pollIntervalSeconds = v) } },
            )
            RowDivider()
            SliderSetting(
                title = stringResource(R.string.setting_wake_timeout),
                valueLabel = { stringResource(R.string.setting_wake_timeout_value, it) },
                value = settings.wakeTimeoutSeconds,
                range = 60..600,
                step = 30,
                onChange = { v -> onUpdateSettings { it.copy(wakeTimeoutSeconds = v) } },
            )
            RowDivider()
            Row(Modifier.padding(horizontal = 16.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(stringResource(R.string.setting_confirm), style = MaterialTheme.typography.titleSmall)
                    Text(stringResource(R.string.setting_confirm_help), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                }
                Spacer(Modifier.width(12.dp))
                Switch(
                    checked = settings.confirmPowerActions,
                    onCheckedChange = { checked -> onUpdateSettings { it.copy(confirmPowerActions = checked) } },
                    colors = SwitchDefaults.colors(
                        checkedThumbColor = Color.White,
                        checkedTrackColor = WolPalette.Blue,
                        uncheckedThumbColor = WolPalette.Text2,
                        uncheckedTrackColor = WolPalette.Surface3,
                        uncheckedBorderColor = WolPalette.Line2,
                    ),
                )
            }
        }

        ShareSections(share = share, ownDevices = ownDevices, now = now, onDialog = onShareDialog)

        SectionLabel(stringResource(R.string.section_backup), Modifier.padding(top = 6.dp))
        WolCard {
            SettingItem(
                icon = WolIcons.Download,
                title = stringResource(R.string.action_export),
                text = stringResource(R.string.action_export_help, deviceCount),
                enabled = !busy && deviceCount > 0,
                onClick = onExport,
            )
            RowDivider()
            SettingItem(
                icon = WolIcons.Upload,
                title = stringResource(R.string.action_import),
                text = stringResource(R.string.action_import_help),
                enabled = !busy,
                onClick = onImport,
            )
        }

        SectionLabel(stringResource(R.string.section_history), Modifier.padding(top = 6.dp))
        WolCard {
            SettingItem(
                icon = WolIcons.History,
                title = stringResource(R.string.history_open),
                text = stringResource(R.string.history_open_help),
                onClick = onOpenHistory,
            )
            RowDivider()
            SettingItem(
                icon = WolIcons.Delete,
                title = stringResource(R.string.history_clear),
                text = stringResource(R.string.history_clear_help),
                danger = true,
                chevron = false,
                onClick = onClearHistory,
            )
        }

        SectionLabel(stringResource(R.string.section_agent_download), Modifier.padding(top = 6.dp))
        WolCard {
            SettingItem(
                icon = WolIcons.Download,
                title = stringResource(R.string.agent_download),
                text = stringResource(R.string.agent_download_help),
                onClick = { onOpenUrl(RELEASES_URL) },
            )
        }

        SectionLabel(stringResource(R.string.section_updates), Modifier.padding(top = 6.dp))
        WolCard {
            val available = update.available
            SettingItem(
                icon = WolIcons.Download,
                title = stringResource(R.string.update_check),
                text = when {
                    !update.enabled -> stringResource(R.string.update_disabled)
                    update.checking -> stringResource(R.string.update_checking)
                    available != null -> stringResource(R.string.update_available, available.version)
                    update.error != null -> update.error
                    update.lastCheck > 0 -> stringResource(R.string.update_last_check, formatDuration(System.currentTimeMillis() - update.lastCheck))
                    else -> stringResource(R.string.update_never)
                },
                accent = available != null,
                onClick = if (update.enabled) onCheckUpdate else ({ onOpenUrl(RELEASES_URL) }),
            )
            if (update.enabled) {
                RowDivider()
                Row(Modifier.padding(horizontal = 16.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(stringResource(R.string.update_auto), style = MaterialTheme.typography.titleSmall)
                        Text(stringResource(R.string.update_auto_help), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                    }
                    Spacer(Modifier.width(12.dp))
                    Switch(
                        checked = update.auto,
                        onCheckedChange = onAutoUpdate,
                        colors = SwitchDefaults.colors(
                            checkedThumbColor = Color.White,
                            checkedTrackColor = WolPalette.Blue,
                            uncheckedThumbColor = WolPalette.Text2,
                            uncheckedTrackColor = WolPalette.Surface3,
                            uncheckedBorderColor = WolPalette.Line2,
                        ),
                    )
                }
            }
        }

        SectionLabel(stringResource(R.string.section_about), Modifier.padding(top = 6.dp))
        WolCard {
            SettingItem(icon = WolIcons.Info, title = stringResource(R.string.about_version), text = version)
            RowDivider()
            SettingItem(
                icon = WolIcons.Code,
                title = stringResource(R.string.about_source),
                text = REPO_URL,
                onClick = { onOpenUrl(REPO_URL) },
            )
            RowDivider()
            SettingItem(icon = WolIcons.Lock, title = stringResource(R.string.about_security), text = stringResource(R.string.about_security_text))
            RowDivider()
            SettingItem(icon = WolIcons.Info, title = stringResource(R.string.about_fonts), text = stringResource(R.string.about_fonts_text))
        }
        Spacer(Modifier.size(20.dp))
    }
}

/** Ligne de réglage : icône, titre, texte d'aide et chevron si elle ouvre quelque chose. */
@Composable
internal fun SettingItem(
    icon: ImageVector,
    title: String,
    text: String,
    onClick: (() -> Unit)? = null,
    enabled: Boolean = true,
    danger: Boolean = false,
    chevron: Boolean = onClick != null,
    accent: Boolean = false,
) {
    val alpha = if (enabled) 1f else 0.4f
    val titleColor = (if (danger) WolPalette.DangerText else WolPalette.Text).copy(alpha = alpha)
    Row(
        Modifier
            .fillMaxWidth()
            .then(if (onClick != null) Modifier.clickable(enabled = enabled, onClick = onClick) else Modifier)
            .padding(horizontal = 16.dp, vertical = 13.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, tint = (if (danger) WolPalette.DangerText else WolPalette.Text2).copy(alpha = alpha), modifier = Modifier.size(20.dp))
        Spacer(Modifier.width(14.dp))
        Column(Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.titleSmall, color = titleColor)
            Text(
                text,
                style = MaterialTheme.typography.bodySmall,
                color = (if (accent) WolPalette.Blue else WolPalette.Text2).copy(alpha = alpha),
            )
        }
        if (chevron) {
            Spacer(Modifier.width(8.dp))
            Icon(WolIcons.Chevron, contentDescription = null, tint = WolPalette.Text3, modifier = Modifier.size(16.dp))
        }
    }
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
    Column(Modifier.padding(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 4.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(title, style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
            Text(valueLabel(current.toInt()), style = MaterialTheme.typography.labelLarge, color = WolPalette.Blue)
        }
        Slider(
            value = current,
            onValueChange = { current = it },
            onValueChangeFinished = { onChange(current.toInt()) },
            valueRange = range.first.toFloat()..range.last.toFloat(),
            steps = ((range.last - range.first) / step - 1).coerceAtLeast(0),
            colors = SliderDefaults.colors(
                thumbColor = WolPalette.Blue,
                activeTrackColor = WolPalette.Blue,
                inactiveTrackColor = WolPalette.Surface3,
                activeTickColor = Color.Transparent,
                inactiveTickColor = Color.Transparent,
            ),
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

