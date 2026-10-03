package io.github.slaynaw.wakeonlan.ui.settings

import android.content.ClipData
import android.content.ClipboardManager
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.archive.ArchiveManager
import io.github.slaynaw.wakeonlan.archive.ArchiveUiState
import io.github.slaynaw.wakeonlan.backup.BackupEntryView
import io.github.slaynaw.wakeonlan.backup.BackupManager
import io.github.slaynaw.wakeonlan.backup.BackupTarget
import io.github.slaynaw.wakeonlan.backup.BackupUiState
import io.github.slaynaw.wakeonlan.core.config.ExportCodec
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.SectionLabel
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.PasswordRevealIcon
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.rememberPasswordReveal
import io.github.slaynaw.wakeonlan.ui.common.formatDuration
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale

/** Dialogues des sauvegardes automatiques. */
sealed interface BackupDialog {
    data object Enable : BackupDialog
    /** Connexion GitHub, puis [then] (dialogue suivant) une fois connecté. */
    data class Connect(val then: BackupDialog? = null) : BackupDialog
    data object Choose : BackupDialog
    data class Login(val then: BackupDialog? = null) : BackupDialog
    data object GitHub : BackupDialog
    data object Folder : BackupDialog
    data object Disable : BackupDialog
    data object Restore : BackupDialog
    data object ArchiveEnable : BackupDialog
    data object Archive : BackupDialog
    data class Error(val message: String) : BackupDialog
}

/** Section « Sauvegarde automatique » des réglages (sans état propre). */
@Composable
fun ColumnScope.BackupSection(
    backup: BackupUiState,
    now: Long,
    onDialog: (BackupDialog) -> Unit,
    onPickFolder: () -> Unit,
    onBackupNow: () -> Unit,
    archive: ArchiveUiState = ArchiveUiState(),
) {
    SectionLabel(stringResource(R.string.section_backup_auto), Modifier.padding(top = 6.dp))
    WolCard {
        if (!backup.enabled) {
            SettingItem(WolIcons.Shield, stringResource(R.string.backup_enable), stringResource(R.string.backup_enable_help), { onDialog(BackupDialog.Enable) }, enabled = backup.loaded)
            RowDivider()
            SettingItem(WolIcons.Download, stringResource(R.string.backup_restore), stringResource(R.string.backup_restore_help), { onDialog(BackupDialog.Restore) })
        } else {
            EnabledItems(backup, archive, now, onDialog, onPickFolder, onBackupNow)
        }
    }
}

@Composable
private fun EnabledItems(
    backup: BackupUiState,
    archive: ArchiveUiState,
    now: Long,
    onDialog: (BackupDialog) -> Unit,
    onPickFolder: () -> Unit,
    onBackupNow: () -> Unit,
) {
    val github = backup.github
    SettingItem(
        icon = WolIcons.Cloud,
        title = stringResource(R.string.backup_github),
        text = if (github != null) "${github.label} · ${targetText(github, now)}" else stringResource(R.string.backup_github_off),
        onClick = { onDialog(if (github != null) BackupDialog.GitHub else BackupDialog.Connect()) },
    )
    RowDivider()
    val folder = backup.folder
    SettingItem(
        icon = WolIcons.History,
        title = stringResource(R.string.backup_folder),
        text = if (folder != null) "${folder.label} · ${targetText(folder, now)}" else stringResource(R.string.backup_folder_off),
        onClick = if (folder != null) ({ onDialog(BackupDialog.Folder) }) else onPickFolder,
    )
    RowDivider()
    SettingItem(
        icon = WolIcons.Upload,
        title = stringResource(R.string.backup_now),
        text = when {
            backup.running -> stringResource(R.string.backup_running)
            backup.paused != null -> stringResource(R.string.backup_paused, backup.paused)
            else -> stringResource(R.string.backup_now_help)
        },
        onClick = if (backup.running) null else onBackupNow,
    )
    RowDivider()
    SettingItem(
        icon = WolIcons.Chart,
        title = stringResource(R.string.archive_title),
        text = when {
            github == null -> stringResource(R.string.archive_need_github)
            !archive.enabled -> stringResource(R.string.archive_off)
            archive.running -> stringResource(R.string.archive_running)
            archive.error != null -> archive.error
            archive.last > 0 -> stringResource(R.string.archive_last, formatDuration((now - archive.last).coerceAtLeast(0)))
            else -> stringResource(R.string.archive_pending)
        },
        onClick = {
            onDialog(
                when {
                    github == null -> BackupDialog.Connect(then = BackupDialog.ArchiveEnable)
                    archive.enabled -> BackupDialog.Archive
                    else -> BackupDialog.ArchiveEnable
                },
            )
        },
    )
    RowDivider()
    SettingItem(WolIcons.Download, stringResource(R.string.backup_restore), stringResource(R.string.backup_restore_help), { onDialog(BackupDialog.Restore) })
    RowDivider()
    SettingItem(
        WolIcons.Delete,
        stringResource(R.string.backup_disable),
        stringResource(R.string.backup_disable_help),
        { onDialog(BackupDialog.Disable) },
        danger = true,
        chevron = false,
    )
}

@Composable
private fun targetText(target: BackupTarget, now: Long): String = when {
    target.error != null -> target.error
    target.last > 0 -> stringResource(R.string.backup_last, formatDuration((now - target.last).coerceAtLeast(0)))
    else -> stringResource(R.string.backup_never)
}

/** Affiche le dialogue en cours et exécute les actions des sauvegardes. */
@Composable
fun BackupDialogHost(
    dialog: BackupDialog?,
    onDialog: (BackupDialog?) -> Unit,
    backup: BackupUiState,
    manager: BackupManager,
    archives: ArchiveManager,
    snackbar: SnackbarHostState,
    onPickFolder: () -> Unit,
    onRestore: (String) -> Unit,
) {
    val context = LocalContext.current
    val resources = LocalResources.current
    val scope = rememberCoroutineScope()
    val close = { onDialog(null) }
    fun toast(id: Int, vararg args: Any) = scope.launch { snackbar.showSnackbar(resources.getString(id, *args)) }
    fun fail(e: Throwable) = onDialog(BackupDialog.Error(e.message.orEmpty().ifEmpty { e.javaClass.simpleName }.replaceFirstChar { it.uppercase() }))
    fun copy(text: String) {
        context.getSystemService(ClipboardManager::class.java)?.setPrimaryClip(ClipData.newPlainText("Patronus", text))
        toast(R.string.share_copied)
    }
    fun connect(then: BackupDialog? = null) = scope.launch {
        try {
            val user = manager.connect()
            if (user != null) {
                toast(R.string.backup_login_reused, user)
                onDialog(then)
            } else {
                onDialog(BackupDialog.Login(then))
            }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            fail(e)
        }
    }

    when (dialog) {
        null -> Unit
        BackupDialog.Enable -> EnableDialog(
            onConfirm = { password ->
                scope.launch {
                    try {
                        manager.enable(password)
                        onDialog(if (backup.github == null && backup.folder == null) BackupDialog.Choose else null)
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        fail(e)
                    }
                }
            },
            onDismiss = close,
        )
        is BackupDialog.Connect -> LaunchedEffect(dialog) { connect(dialog.then) }
        BackupDialog.Choose -> AlertDialog(
            onDismissRequest = close,
            icon = { Icon(WolIcons.Cloud, contentDescription = null, tint = WolPalette.Blue) },
            title = { Text(stringResource(R.string.backup_choose_title)) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text(stringResource(R.string.backup_choose_text))
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        WolButton(stringResource(R.string.backup_folder), { close(); onPickFolder() }, Modifier.weight(1f))
                        WolButton(stringResource(R.string.backup_github), { connect() }, Modifier.weight(1f), ButtonKind.PRIMARY)
                    }
                }
            },
            confirmButton = {},
            dismissButton = { TextButton(onClick = close) { Text(stringResource(R.string.later)) } },
        )
        is BackupDialog.Login -> BackupLoginDialog(
            backup = backup,
            onCopy = ::copy,
            onOpen = {
                backup.login?.let { copy(it.code) }
                openUrl(context, GITHUB_DEVICE_URL)
            },
            onDone = {
                onDialog(dialog.then)
                toast(R.string.backup_login_done)
            },
            onCancel = {
                manager.cancelLogin()
                close()
            },
        )
        BackupDialog.GitHub -> AlertDialog(
            onDismissRequest = close,
            icon = { Icon(WolIcons.Cloud, contentDescription = null, tint = WolPalette.Blue) },
            title = { Text(stringResource(R.string.backup_github_title)) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(stringResource(R.string.backup_github_text, backup.github?.label.orEmpty()))
                    backup.github?.error?.let { Text(it, color = WolPalette.DangerText, style = MaterialTheme.typography.bodySmall) }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        WolButton(stringResource(R.string.backup_disconnect), {
                            close()
                            scope.launch { runCatching { manager.disconnect() }.onFailure(::fail) }
                        }, Modifier.weight(1f))
                        WolButton(stringResource(R.string.backup_reconnect), { connect() }, Modifier.weight(1f))
                    }
                }
            },
            confirmButton = { TextButton(onClick = close) { Text(stringResource(R.string.close)) } },
        )
        BackupDialog.Folder -> AlertDialog(
            onDismissRequest = close,
            icon = { Icon(WolIcons.History, contentDescription = null, tint = WolPalette.Blue) },
            title = { Text(stringResource(R.string.backup_folder_title)) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(backup.folder?.label.orEmpty())
                    backup.folder?.error?.let { Text(it, color = WolPalette.DangerText, style = MaterialTheme.typography.bodySmall) }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        WolButton(stringResource(R.string.backup_folder_remove), {
                            close()
                            scope.launch { runCatching { manager.removeFolder() }.onFailure(::fail) }
                        }, Modifier.weight(1f))
                        WolButton(stringResource(R.string.backup_folder_change), { close(); onPickFolder() }, Modifier.weight(1f))
                    }
                }
            },
            confirmButton = { TextButton(onClick = close) { Text(stringResource(R.string.close)) } },
        )
        BackupDialog.Disable -> AlertDialog(
            onDismissRequest = close,
            icon = { Icon(WolIcons.Delete, contentDescription = null, tint = WolPalette.DangerText) },
            title = { Text(stringResource(R.string.backup_disable)) },
            text = { Text(stringResource(R.string.backup_disable_text)) },
            confirmButton = {
                TextButton(onClick = {
                    close()
                    scope.launch { runCatching { manager.disable() }.onFailure(::fail) }
                }) { Text(stringResource(R.string.backup_disable_ok), color = WolPalette.DangerText) }
            },
            dismissButton = { TextButton(onClick = close) { Text(stringResource(R.string.cancel)) } },
        )
        BackupDialog.Restore -> {
            if (backup.github == null) {
                // Connexion d'abord (compte du partage ou code), puis la liste.
                LaunchedEffect(Unit) { connect(then = BackupDialog.Restore) }
            } else {
                RestoreDialog(manager = manager, onRestore = { close(); onRestore(it) }, onError = ::fail, onDismiss = close)
            }
        }
        BackupDialog.ArchiveEnable -> AlertDialog(
            onDismissRequest = close,
            icon = { Icon(WolIcons.Chart, contentDescription = null, tint = WolPalette.Blue) },
            title = { Text(stringResource(R.string.archive_enable_title)) },
            text = { Text(stringResource(R.string.archive_enable_text), Modifier.verticalScroll(rememberScrollState())) },
            confirmButton = {
                TextButton(onClick = {
                    close()
                    scope.launch { runCatching { archives.enable(true) }.onFailure(::fail) }
                }) { Text(stringResource(R.string.archive_enable_ok)) }
            },
            dismissButton = { TextButton(onClick = close) { Text(stringResource(R.string.cancel)) } },
        )
        BackupDialog.Archive -> AlertDialog(
            onDismissRequest = close,
            icon = { Icon(WolIcons.Chart, contentDescription = null, tint = WolPalette.Blue) },
            title = { Text(stringResource(R.string.archive_title)) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(stringResource(R.string.archive_dialog_text))
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        WolButton(stringResource(R.string.archive_disable), {
                            close()
                            scope.launch { runCatching { archives.enable(false) }.onFailure(::fail) }
                        }, Modifier.weight(1f))
                        WolButton(stringResource(R.string.archive_now), {
                            close()
                            scope.launch {
                                try {
                                    val r = archives.archiveNow()
                                    if (r.skipped.isEmpty()) {
                                        toast(R.string.archive_done, r.minutes)
                                    } else {
                                        onDialog(BackupDialog.Error(resources.getString(R.string.archive_done, r.minutes) + "\n" +
                                            resources.getString(R.string.archive_skipped, r.skipped.joinToString())))
                                    }
                                } catch (e: CancellationException) {
                                    throw e
                                } catch (e: Exception) {
                                    fail(e)
                                }
                            }
                        }, Modifier.weight(1f), ButtonKind.PRIMARY)
                    }
                }
            },
            confirmButton = { TextButton(onClick = close) { Text(stringResource(R.string.close)) } },
        )
        is BackupDialog.Error -> MessageDialog(stringResource(R.string.section_backup_auto), dialog.message, close)
    }
}

@Composable
private fun EnableDialog(onConfirm: (CharArray) -> Unit, onDismiss: () -> Unit) {
    var password by remember { mutableStateOf("") }
    var confirmation by remember { mutableStateOf("") }
    val tooShort = password.length < ExportCodec.MIN_PASSWORD_LENGTH
    val mismatch = password != confirmation
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(WolIcons.Shield, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(R.string.backup_enable_title)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.backup_enable_text), style = MaterialTheme.typography.bodyMedium)
                val reveal1 = rememberPasswordReveal()
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text(stringResource(R.string.field_password)) },
                    singleLine = true,
                    isError = password.isNotEmpty() && tooShort,
                    supportingText = { Text(stringResource(R.string.field_password_help, ExportCodec.MIN_PASSWORD_LENGTH)) },
                    visualTransformation = reveal1.transformation,
                    trailingIcon = { PasswordRevealIcon(reveal1) },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                )
                val reveal2 = rememberPasswordReveal()
                OutlinedTextField(
                    value = confirmation,
                    onValueChange = { confirmation = it },
                    label = { Text(stringResource(R.string.field_password_confirm)) },
                    singleLine = true,
                    isError = confirmation.isNotEmpty() && mismatch,
                    visualTransformation = reveal2.transformation,
                    trailingIcon = { PasswordRevealIcon(reveal2) },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                )
            }
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(password.toCharArray()) }, enabled = !tooShort && !mismatch) {
                Text(stringResource(R.string.backup_enable_ok))
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun BackupLoginDialog(backup: BackupUiState, onCopy: (String) -> Unit, onOpen: () -> Unit, onDone: () -> Unit, onCancel: () -> Unit) {
    // Fermeture une fois la connexion terminée (code vu, puis plus de connexion en cours et compte relié).
    var seen by remember { mutableStateOf(false) }
    val login = backup.login
    LaunchedEffect(login, backup.github) {
        if (login != null) seen = true else if (seen && backup.github != null) onDone()
    }
    AlertDialog(
        onDismissRequest = onCancel,
        icon = { Icon(WolIcons.Cloud, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(R.string.share_login_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                backup.loginShareUser?.let {
                    Text(stringResource(R.string.backup_login_share_rejected, it), color = WolPalette.Text2, style = MaterialTheme.typography.bodySmall)
                }
                Text(stringResource(R.string.share_login_text))
                login?.let { CodeBox(it.code) }
                val error = login?.error
                Text(
                    error?.replaceFirstChar { it.uppercase() } ?: stringResource(R.string.share_login_waiting),
                    color = if (error != null) WolPalette.DangerText else WolPalette.Text2,
                    style = MaterialTheme.typography.bodySmall,
                )
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    WolButton(stringResource(R.string.share_login_copy), { login?.let { onCopy(it.code) } }, Modifier.weight(1f), icon = WolIcons.Copy)
                    WolButton(stringResource(R.string.share_login_open), onOpen, Modifier.weight(1f), ButtonKind.PRIMARY)
                }
            }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = onCancel) { Text(stringResource(R.string.cancel)) } },
    )
}

/** Liste des sauvegardes du compte GitHub ; une fois choisie, l'import habituel prend le relais. */
@Composable
private fun RestoreDialog(manager: BackupManager, onRestore: (String) -> Unit, onError: (Throwable) -> Unit, onDismiss: () -> Unit) {
    var entries by remember { mutableStateOf<List<BackupEntryView>?>(null) }
    LaunchedEffect(Unit) {
        try {
            entries = manager.list().orEmpty()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            onError(e)
        }
    }
    val dates = remember { DateTimeFormatter.ofLocalizedDate(FormatStyle.FULL).withLocale(Locale.FRENCH) }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(WolIcons.Download, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(R.string.backup_restore_title)) },
        text = {
            val list = entries
            when {
                list == null -> Row(verticalAlignment = Alignment.CenterVertically) {
                    CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                    Spacer(Modifier.width(12.dp))
                    Text(stringResource(R.string.backup_restore_loading))
                }
                list.isEmpty() -> Text(stringResource(R.string.backup_restore_empty))
                else -> Column(Modifier.heightIn(max = 360.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(stringResource(R.string.backup_restore_text), style = MaterialTheme.typography.bodyMedium)
                    Spacer(Modifier.size(4.dp))
                    for (e in list) {
                        Column(
                            Modifier
                                .fillMaxWidth()
                                .clickable { onRestore(e.name) }
                                .padding(vertical = 10.dp),
                        ) {
                            Text("${e.date.format(dates)} · ${deviceLabel(e.device)}", style = MaterialTheme.typography.titleSmall)
                            Text(
                                "${(e.size / 1024).coerceAtLeast(1)} Ko" + if (e.mine) " · " + stringResource(R.string.backup_restore_mine) else "",
                                style = MaterialTheme.typography.bodySmall,
                                color = WolPalette.Text2,
                            )
                        }
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.close)) } },
    )
}

/** « android-pixel-8-3fa2 » → « android · pixel 8 ». */
private fun deviceLabel(device: String): String {
    val parts = device.split("-").toMutableList()
    if (parts.size > 2 && Regex("^[0-9a-f]{4}$").matches(parts.last())) parts.removeAt(parts.lastIndex)
    return parts.first() + " · " + parts.drop(1).joinToString(" ")
}
