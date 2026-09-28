package io.github.slaynaw.wakeonlan.ui.settings

import android.content.ActivityNotFoundException
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.net.toUri
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.share.QrCode
import io.github.slaynaw.wakeonlan.core.share.ShareAccess
import io.github.slaynaw.wakeonlan.core.share.ShareException
import io.github.slaynaw.wakeonlan.core.share.ShareInvite
import io.github.slaynaw.wakeonlan.core.share.ShareRight
import io.github.slaynaw.wakeonlan.share.ShareManager
import io.github.slaynaw.wakeonlan.share.ShareRequestView
import io.github.slaynaw.wakeonlan.share.ShareUiState
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.theme.MonoStyle
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.ScanFailure
import io.github.slaynaw.wakeonlan.ui.common.SectionLabel
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.formatDuration
import io.github.slaynaw.wakeonlan.ui.common.scanQrCode
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import kotlinx.coroutines.launch

private const val GITHUB_DEVICE_URL = "https://github.com/login/device"

/** Sections « Partager mes PC » et « PC partagés avec moi » des réglages (sans état propre). */
@Composable
fun ColumnScope.ShareSections(
    share: ShareUiState,
    ownDevices: List<Device>,
    now: Long,
    onDialog: (ShareDialog) -> Unit,
) {
    SectionLabel(stringResource(R.string.section_share_mine), Modifier.padding(top = 6.dp))
    WolCard {
        val owner = share.owner
        when {
            owner == null -> SettingItem(
                icon = WolIcons.Share,
                title = stringResource(R.string.share_start),
                text = stringResource(if (share.canLogin) R.string.share_start_help else R.string.share_unavailable),
                onClick = if (share.canLogin) ({ onDialog(ShareDialog.Setup) }) else null,
            )
            !owner.isConnected -> {
                SettingItem(
                    icon = WolIcons.Cloud,
                    title = stringResource(R.string.share_reconnect),
                    text = share.publishError ?: stringResource(R.string.share_reconnect_help),
                    onClick = if (share.canLogin) ({ onDialog(ShareDialog.Setup) }) else null,
                )
                RowDivider()
                SettingItem(WolIcons.Delete, stringResource(R.string.share_stop), stringResource(R.string.share_stop_help), { onDialog(ShareDialog.Stop) }, danger = true, chevron = false)
            }
            else -> {
                SettingItem(WolIcons.Share, stringResource(R.string.share_invite), stringResource(R.string.share_invite_help), { onDialog(ShareDialog.Invite) })
                RowDivider()
                SettingItem(
                    WolIcons.Scan,
                    stringResource(R.string.share_add_request),
                    stringResource(R.string.share_add_request_help),
                    { onDialog(if (ownDevices.isEmpty()) ShareDialog.NoDevices else ShareDialog.AddRequest) },
                )
                val names = ownDevices.associate { it.id to it.name }
                owner.people.forEach { person ->
                    RowDivider()
                    val summary = ownDevices.filter { person.rights.containsKey(it.id) }.joinToString(", ") { it.name }
                    SettingItem(
                        icon = WolIcons.Users,
                        title = person.name,
                        text = if (owner.published.containsKey(person.device) || share.publishing) summary else stringResource(R.string.share_person_waiting, summary),
                        onClick = { onDialog(ShareDialog.Rights(person.name, person.device, null, person.rights.filterKeys { it in names }, isNew = false)) },
                    )
                }
                RowDivider()
                SettingItem(
                    icon = WolIcons.Cloud,
                    title = stringResource(R.string.share_account, owner.user),
                    text = if (share.publishing) {
                        stringResource(R.string.share_publishing)
                    } else {
                        share.publishError ?: stringResource(R.string.share_account_help)
                    },
                    danger = share.publishError != null,
                )
                RowDivider()
                SettingItem(WolIcons.Delete, stringResource(R.string.share_stop), stringResource(R.string.share_stop_help), { onDialog(ShareDialog.Stop) }, danger = true, chevron = false)
            }
        }
    }

    SectionLabel(stringResource(R.string.section_share_received), Modifier.padding(top = 6.dp))
    WolCard {
        share.received.forEach { access ->
            val error = share.syncErrors[access.owner]
            SettingItem(
                icon = WolIcons.Monitor,
                title = stringResource(R.string.share_access_title, access.ownerName),
                text = when {
                    error != null -> error
                    access.removed -> stringResource(R.string.share_access_removed, access.ownerName)
                    !access.active -> stringResource(R.string.share_access_pending, access.ownerName)
                    else -> stringResource(
                        R.string.share_access_active,
                        access.devices.joinToString(", ") { it.name }.ifEmpty { stringResource(R.string.share_access_none) },
                        formatDuration(now - (access.synced.takeIf { it > 0 } ?: now)),
                    )
                },
                danger = error != null || access.removed,
                onClick = { onDialog(ShareDialog.Access(access)) },
            )
            RowDivider()
        }
        SettingItem(WolIcons.Download, stringResource(R.string.share_request), stringResource(R.string.share_request_help), { onDialog(ShareDialog.RequestAccess) })
    }
}

/** Dialogues du partage. */
sealed interface ShareDialog {
    data object Setup : ShareDialog
    data object Login : ShareDialog
    data object Invite : ShareDialog
    data object NoDevices : ShareDialog
    data object AddRequest : ShareDialog
    data class Paste(val invite: Boolean) : ShareDialog
    data class Rights(val name: String, val device: String, val code: String?, val rights: Map<String, ShareRight>, val isNew: Boolean) : ShareDialog
    data class Revoke(val name: String, val device: String) : ShareDialog
    data object Stop : ShareDialog
    data object RequestAccess : ShareDialog
    data class Name(val invite: ShareInvite) : ShareDialog
    data class Request(val view: ShareRequestView) : ShareDialog
    data class Access(val access: ShareAccess) : ShareDialog
    data class Leave(val access: ShareAccess) : ShareDialog
    data class Error(val message: String) : ShareDialog
}

/** Affiche le dialogue en cours et exécute les actions du partage. */
@Composable
fun ShareDialogHost(
    dialog: ShareDialog?,
    onDialog: (ShareDialog?) -> Unit,
    share: ShareUiState,
    ownDevices: List<Device>,
    manager: ShareManager,
    snackbar: SnackbarHostState,
) {
    val context = LocalContext.current
    val resources = LocalResources.current
    val scope = rememberCoroutineScope()
    val close = { onDialog(null) }
    fun fail(e: Throwable) = onDialog(ShareDialog.Error(e.message.orEmpty().replaceFirstChar { it.uppercase() }))
    fun toast(id: Int, vararg args: Any) = scope.launch { snackbar.showSnackbar(resources.getString(id, *args)) }
    fun copy(text: String) {
        context.getSystemService(ClipboardManager::class.java)?.setPrimaryClip(ClipData.newPlainText("Wake On LAN", text))
        toast(R.string.share_copied)
    }
    fun readRequest(text: String) {
        try {
            val info = manager.readRequest(text)
            onDialog(ShareDialog.Rights(info.request.name, info.request.device, info.code, info.rights.orEmpty(), isNew = info.rights == null))
        } catch (e: ShareException) {
            fail(e)
        }
    }
    fun readInvite(text: String) {
        try {
            onDialog(ShareDialog.Name(manager.readInvite(text)))
        } catch (e: ShareException) {
            fail(e)
        }
    }
    fun scan(onText: (String) -> Unit) {
        scanQrCode(context, onResult = onText, onError = { failure ->
            if (failure is ScanFailure.Error) onDialog(ShareDialog.Error(failure.message))
        })
    }

    when (dialog) {
        null -> Unit
        ShareDialog.Setup -> SetupDialog(
            initialName = share.owner?.name.orEmpty(),
            onConfirm = { name ->
                scope.launch {
                    try {
                        manager.startLogin(name)
                        onDialog(ShareDialog.Login)
                        openUrl(context, GITHUB_DEVICE_URL)
                    } catch (e: ShareException) {
                        fail(e)
                    }
                }
            },
            onDismiss = close,
        )
        ShareDialog.Login -> LoginDialog(
            share = share,
            onCopy = ::copy,
            onOpen = { openUrl(context, GITHUB_DEVICE_URL) },
            onDone = {
                close()
                toast(R.string.share_login_done)
            },
            onCancel = {
                manager.cancelLogin()
                close()
            },
        )
        ShareDialog.Invite -> {
            val invite = remember { runCatching { manager.invite() } }
            val inv = invite.getOrNull()
            if (inv != null) {
                LinkDialog(
                    title = stringResource(R.string.share_invite),
                    text = stringResource(R.string.share_invite_text),
                    link = inv.link,
                    code = null,
                    footer = null,
                    onCopy = ::copy,
                    onSend = { send(context, inv.link) },
                    onDismiss = close,
                )
            } else {
                LaunchedEffect(Unit) { invite.exceptionOrNull()?.let(::fail) }
            }
        }
        ShareDialog.NoDevices -> MessageDialog(stringResource(R.string.share_add_request), stringResource(R.string.share_no_devices), close)
        ShareDialog.AddRequest -> ChoiceDialog(
            title = stringResource(R.string.share_add_request),
            text = stringResource(R.string.share_add_request_help),
            onScan = { scan(::readRequest) },
            onPaste = { onDialog(ShareDialog.Paste(invite = false)) },
            onDismiss = close,
        )
        ShareDialog.RequestAccess -> ChoiceDialog(
            title = stringResource(R.string.share_request),
            text = stringResource(R.string.share_invite_paste_text),
            onScan = { scan(::readInvite) },
            onPaste = { onDialog(ShareDialog.Paste(invite = true)) },
            onDismiss = close,
        )
        is ShareDialog.Paste -> PasteDialog(
            title = stringResource(if (dialog.invite) R.string.share_request else R.string.share_add_request),
            text = stringResource(if (dialog.invite) R.string.share_invite_paste_text else R.string.share_request_paste_text),
            placeholder = if (dialog.invite) "wolshare://invite?…" else "wolshare://request?…",
            onConfirm = { if (dialog.invite) readInvite(it) else readRequest(it) },
            onDismiss = close,
        )
        is ShareDialog.Rights -> RightsDialog(
            dialog = dialog,
            devices = ownDevices,
            onGrant = { rights ->
                scope.launch {
                    try {
                        manager.grant(dialog.device, dialog.name, rights)
                        close()
                        toast(R.string.share_granted, dialog.name)
                    } catch (e: ShareException) {
                        fail(e)
                    }
                }
            },
            onRevoke = { onDialog(ShareDialog.Revoke(dialog.name, dialog.device)) },
            onDismiss = close,
        )
        is ShareDialog.Revoke -> ConfirmDialog(
            title = stringResource(R.string.share_revoke_title, dialog.name),
            text = stringResource(R.string.share_revoke_text),
            action = stringResource(R.string.share_revoke),
            onConfirm = {
                scope.launch {
                    manager.revoke(dialog.device)
                    close()
                    toast(R.string.share_revoked, dialog.name)
                }
            },
            onDismiss = close,
        )
        ShareDialog.Stop -> ConfirmDialog(
            title = stringResource(R.string.share_stop_title),
            text = stringResource(R.string.share_stop_text),
            action = stringResource(R.string.share_stop),
            onConfirm = {
                scope.launch {
                    try {
                        manager.stop()
                        close()
                    } catch (e: ShareException) {
                        fail(e)
                    }
                }
            },
            onDismiss = close,
        )
        is ShareDialog.Name -> NameDialog(
            ownerName = dialog.invite.name,
            onConfirm = { name ->
                scope.launch {
                    try {
                        onDialog(ShareDialog.Request(manager.request(dialog.invite, name)))
                    } catch (e: ShareException) {
                        fail(e)
                    }
                }
            },
            onDismiss = close,
        )
        is ShareDialog.Request -> LinkDialog(
            title = stringResource(R.string.share_request_title, dialog.view.ownerName),
            text = stringResource(R.string.share_request_text, dialog.view.ownerName),
            link = dialog.view.link,
            code = stringResource(R.string.share_request_code, dialog.view.ownerName) to dialog.view.code,
            footer = stringResource(R.string.share_request_wait, dialog.view.ownerName),
            onCopy = ::copy,
            onSend = { send(context, dialog.view.link) },
            onDismiss = close,
        )
        is ShareDialog.Access -> AccessDialog(
            access = share.received.firstOrNull { it.owner == dialog.access.owner } ?: dialog.access,
            error = share.syncErrors[dialog.access.owner],
            onShowRequest = {
                scope.launch { manager.requestView(dialog.access.owner)?.let { onDialog(ShareDialog.Request(it)) } }
            },
            onSync = {
                manager.syncNow()
                close()
            },
            onLeave = { onDialog(ShareDialog.Leave(dialog.access)) },
            onDismiss = close,
        )
        is ShareDialog.Leave -> ConfirmDialog(
            title = stringResource(R.string.share_leave_title, dialog.access.ownerName),
            text = stringResource(R.string.share_leave_text),
            action = stringResource(
                when {
                    dialog.access.active -> R.string.share_leave
                    dialog.access.removed -> R.string.share_remove
                    else -> R.string.share_cancel_request
                },
            ),
            onConfirm = {
                scope.launch {
                    manager.leave(dialog.access.owner)
                    close()
                }
            },
            onDismiss = close,
        )
        is ShareDialog.Error -> MessageDialog(stringResource(R.string.section_share_mine), dialog.message, close)
    }
}

// --- Dialogues ---

@Composable
private fun SetupDialog(initialName: String, onConfirm: (String) -> Unit, onDismiss: () -> Unit) {
    var name by remember { mutableStateOf(initialName) }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(WolIcons.Share, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(R.string.share_start)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(stringResource(R.string.share_setup_text))
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text(stringResource(R.string.share_field_name)) },
                    singleLine = true,
                )
            }
        },
        confirmButton = { TextButton(onClick = { onConfirm(name.trim()) }, enabled = name.isNotBlank()) { Text(stringResource(R.string.share_connect)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun LoginDialog(share: ShareUiState, onCopy: (String) -> Unit, onOpen: () -> Unit, onDone: () -> Unit, onCancel: () -> Unit) {
    // Fermeture une fois la connexion terminée (code vu, puis plus de connexion en cours et compte relié).
    var seen by remember { mutableStateOf(false) }
    val login = share.login
    LaunchedEffect(login, share.owner?.isConnected) {
        if (login != null) seen = true else if (seen && share.owner?.isConnected == true) onDone()
    }
    AlertDialog(
        onDismissRequest = onCancel,
        icon = { Icon(WolIcons.Cloud, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(R.string.share_login_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
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

/** Lien de partage : QR code, lien (copier / envoyer) et, pour une demande, le code de vérification. */
@Composable
private fun LinkDialog(
    title: String,
    text: String,
    link: String,
    code: Pair<String, String>?,
    footer: String?,
    onCopy: (String) -> Unit,
    onSend: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(text)
                QrImage(link, Modifier.align(Alignment.CenterHorizontally))
                Row(
                    Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(8.dp))
                        .background(WolPalette.Background)
                        .padding(start = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(link, style = MonoStyle.copy(fontSize = 11.sp), color = WolPalette.Text2, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                    TextButton(onClick = { onCopy(link) }) {
                        Icon(WolIcons.Copy, contentDescription = stringResource(R.string.share_copy_link), modifier = Modifier.size(18.dp))
                    }
                }
                code?.let { (label, value) ->
                    Text(label)
                    CodeBox(value, Modifier.align(Alignment.CenterHorizontally))
                }
                footer?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2) }
            }
        },
        confirmButton = { TextButton(onClick = onSend) { Text(stringResource(R.string.share_send_link)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.close)) } },
    )
}

@Composable
private fun ChoiceDialog(title: String, text: String, onScan: () -> Unit, onPaste: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(WolIcons.Scan, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(title) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(text)
                WolButton(
                    stringResource(R.string.share_scan),
                    {
                        onDismiss()
                        onScan()
                    },
                    Modifier.fillMaxWidth(),
                    ButtonKind.PRIMARY,
                    WolIcons.Scan,
                )
                WolButton(stringResource(R.string.share_paste), onPaste, Modifier.fillMaxWidth(), icon = WolIcons.Paste)
            }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun PasteDialog(title: String, text: String, placeholder: String, onConfirm: (String) -> Unit, onDismiss: () -> Unit) {
    var value by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(WolIcons.Paste, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(title) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(text)
                OutlinedTextField(value = value, onValueChange = { value = it }, placeholder = { Text(placeholder) }, minLines = 2, maxLines = 4)
            }
        },
        confirmButton = { TextButton(onClick = { onConfirm(value.trim()) }, enabled = value.isNotBlank()) { Text(stringResource(R.string.ok)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun RightsDialog(
    dialog: ShareDialog.Rights,
    devices: List<Device>,
    onGrant: (Map<String, ShareRight>) -> Unit,
    onRevoke: () -> Unit,
    onDismiss: () -> Unit,
) {
    val chosen = remember { mutableStateMapOf<String, ShareRight>().apply { putAll(dialog.rights) } }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(WolIcons.Users, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(if (dialog.isNew) R.string.share_grant_title else R.string.share_edit_title, dialog.name)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                dialog.code?.let { code ->
                    Text(stringResource(R.string.share_verify, dialog.name))
                    CodeBox(code, Modifier.align(Alignment.CenterHorizontally))
                    Text(stringResource(R.string.share_verify_help), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                }
                SectionLabel(stringResource(R.string.share_rights), Modifier.padding(top = 4.dp))
                devices.forEach { d ->
                    val full = d.canShutdown && d.agent?.hasKey == true
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(WolIcons.Monitor, contentDescription = null, tint = WolPalette.Text2, modifier = Modifier.size(18.dp))
                        Spacer(Modifier.width(10.dp))
                        Text(d.name, style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                        RightSelector(
                            current = chosen[d.id]?.let { if (it == ShareRight.FULL && !full) ShareRight.WAKE else it },
                            allowFull = full,
                            onChange = { if (it == null) chosen.remove(d.id) else chosen[d.id] = it },
                        )
                    }
                }
                if (!dialog.isNew) {
                    WolButton(stringResource(R.string.share_revoke), onRevoke, Modifier.fillMaxWidth(), ButtonKind.DANGER)
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { onGrant(chosen.toMap()) }, enabled = chosen.isNotEmpty()) {
                Text(stringResource(if (dialog.isNew) R.string.share_grant else R.string.save))
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun RightSelector(current: ShareRight?, allowFull: Boolean, onChange: (ShareRight?) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    val label = @Composable { right: ShareRight? ->
        stringResource(
            when (right) {
                null -> R.string.share_right_none
                ShareRight.WAKE -> R.string.share_right_wake
                ShareRight.FULL -> R.string.share_right_full
            },
        )
    }
    Box {
        TextButton(onClick = { expanded = true }) {
            Text(label(current), color = if (current == null) WolPalette.Text2 else WolPalette.Blue, maxLines = 1)
            Icon(WolIcons.ChevronDown, contentDescription = null, modifier = Modifier.size(16.dp))
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            (listOf(null, ShareRight.WAKE) + listOfNotNull(ShareRight.FULL.takeIf { allowFull })).forEach { right ->
                DropdownMenuItem(text = { Text(label(right)) }, onClick = {
                    expanded = false
                    onChange(right)
                })
            }
        }
    }
}

@Composable
private fun NameDialog(ownerName: String, onConfirm: (String) -> Unit, onDismiss: () -> Unit) {
    var name by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(WolIcons.Users, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(R.string.share_request)) },
        text = {
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                label = { Text(stringResource(R.string.share_field_my_name, ownerName)) },
                singleLine = true,
            )
        },
        confirmButton = { TextButton(onClick = { onConfirm(name.trim()) }, enabled = name.isNotBlank()) { Text(stringResource(R.string.ok)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun AccessDialog(
    access: ShareAccess,
    error: String?,
    onShowRequest: () -> Unit,
    onSync: () -> Unit,
    onLeave: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(WolIcons.Monitor, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(R.string.share_access_title, access.ownerName)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(
                    when {
                        access.active -> if (access.devices.isEmpty()) {
                            stringResource(R.string.share_access_none)
                        } else {
                            stringResource(R.string.share_access_devices, access.devices.joinToString(", ") { it.name })
                        }
                        access.removed -> stringResource(R.string.share_access_removed, access.ownerName)
                        else -> stringResource(R.string.share_request_wait, access.ownerName)
                    },
                )
                error?.let { Text(it, color = WolPalette.DangerText, style = MaterialTheme.typography.bodySmall) }
                if (!access.active && !access.removed) {
                    WolButton(stringResource(R.string.share_show_request), onShowRequest, Modifier.fillMaxWidth(), icon = WolIcons.Share)
                }
                WolButton(stringResource(R.string.share_sync_now), onSync, Modifier.fillMaxWidth(), icon = WolIcons.Refresh)
                WolButton(
                    stringResource(
                        when {
                            access.active -> R.string.share_leave
                            access.removed -> R.string.share_remove
                            else -> R.string.share_cancel_request
                        },
                    ),
                    onLeave,
                    Modifier.fillMaxWidth(),
                    ButtonKind.DANGER,
                )
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.close)) } },
    )
}

@Composable
private fun ConfirmDialog(title: String, text: String, action: String, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(WolIcons.Warning, contentDescription = null, tint = WolPalette.DangerText) },
        title = { Text(title) },
        text = { Text(text) },
        confirmButton = { WolButton(action, onConfirm, kind = ButtonKind.DANGER) },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun MessageDialog(title: String, text: String, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = { Text(text) },
        confirmButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.ok)) } },
    )
}

/** Code (connexion GitHub, vérification) en grands caractères. */
@Composable
private fun CodeBox(code: String, modifier: Modifier = Modifier) {
    Text(
        code,
        style = MonoStyle.copy(fontSize = 26.sp, fontWeight = FontWeight.Bold, letterSpacing = 3.sp),
        color = WolPalette.Text,
        modifier = modifier
            .clip(RoundedCornerShape(10.dp))
            .background(WolPalette.Soft)
            .padding(horizontal = 16.dp, vertical = 6.dp),
    )
}

/** QR code d'un lien, dessiné module par module (noir sur blanc, marge de 4 modules). */
@Composable
fun QrImage(text: String, modifier: Modifier = Modifier) {
    val code = remember(text) { QrCode.encode(text) }
    val description = stringResource(R.string.share_qr_description)
    Canvas(
        modifier
            .size(216.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(Color.White)
            .semantics { contentDescription = description },
    ) {
        val modules = code.size + 8
        val cell = kotlin.math.floor(size.minDimension / modules)
        val origin = (size.minDimension - cell * code.size) / 2
        for (y in 0 until code.size) {
            for (x in 0 until code.size) {
                if (code.isDark(x, y)) drawRect(Color.Black, Offset(origin + x * cell, origin + y * cell), Size(cell, cell))
            }
        }
    }
}

private fun openUrl(context: Context, url: String) {
    try {
        context.startActivity(Intent(Intent.ACTION_VIEW, url.toUri()))
    } catch (_: ActivityNotFoundException) {
        // Aucun navigateur : le code reste affiché.
    }
}

private fun send(context: Context, link: String) {
    val intent = Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, link)
    try {
        context.startActivity(Intent.createChooser(intent, null))
    } catch (_: ActivityNotFoundException) {
        // Aucune application de partage.
    }
}
