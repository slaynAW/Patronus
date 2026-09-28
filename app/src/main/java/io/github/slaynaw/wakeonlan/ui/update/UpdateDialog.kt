package io.github.slaynaw.wakeonlan.ui.update

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.DialogProperties
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.update.UpdateManifest
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import io.github.slaynaw.wakeonlan.update.UpdateStage
import io.github.slaynaw.wakeonlan.update.UpdateUiState
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.time.format.DateTimeParseException
import java.util.Locale

/** « Mise à jour disponible » : nouveautés, puis téléchargement et installation en un clic. */
@Composable
fun UpdateDialog(state: UpdateUiState, onLater: () -> Unit, onInstall: () -> Unit) {
    val manifest = state.available ?: return
    val busy = state.stage != UpdateStage.IDLE
    AlertDialog(
        onDismissRequest = { if (!busy) onLater() },
        properties = DialogProperties(dismissOnClickOutside = !busy, dismissOnBackPress = !busy),
        containerColor = WolPalette.Surface2,
        icon = { Icon(WolIcons.Download, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(R.string.update_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(metaText(manifest), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text3)
                UpdateNotes(manifest.notes)
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(WolIcons.Shield, contentDescription = null, tint = WolPalette.On, modifier = Modifier.size(16.dp))
                    Spacer(Modifier.width(8.dp))
                    Text(stringResource(R.string.update_keep_data), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                }
                if (busy) {
                    LinearProgressIndicator(
                        progress = { state.progress },
                        modifier = Modifier.fillMaxWidth(),
                        color = WolPalette.Blue,
                        trackColor = WolPalette.Surface3,
                    )
                    Text(
                        if (state.stage == UpdateStage.DOWNLOADING) {
                            stringResource(R.string.update_downloading, (state.progress * 100).toInt())
                        } else {
                            stringResource(R.string.update_installing)
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = WolPalette.Text2,
                    )
                } else if (state.error != null) {
                    Text(state.error, style = MaterialTheme.typography.bodySmall, color = WolPalette.DangerText)
                }
            }
        },
        confirmButton = {
            WolButton(
                text = stringResource(if (state.error != null) R.string.update_retry else R.string.update_install),
                onClick = onInstall,
                kind = ButtonKind.PRIMARY,
                enabled = !busy,
            )
        },
        dismissButton = {
            TextButton(onClick = onLater, enabled = !busy) { Text(stringResource(R.string.update_later), color = WolPalette.Text2) }
        },
    )
}

@Composable
private fun metaText(manifest: UpdateManifest): String {
    val size = manifest.file("android")?.size ?: 0
    val date = try {
        LocalDate.parse(manifest.date).format(DateTimeFormatter.ofPattern("d MMMM yyyy", Locale.FRENCH))
    } catch (e: DateTimeParseException) {
        ""
    }
    return listOf(
        stringResource(R.string.update_meta_version, manifest.version),
        date,
        stringResource(R.string.update_meta_size, String.format(Locale.FRENCH, "%.1f", size / 1_048_576.0)),
    ).filter { it.isNotEmpty() }.joinToString(" · ")
}

/** Nouveautés : titres « ### », listes « - », « **gras** » (texte uniquement). */
@Composable
fun UpdateNotes(text: String, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(10.dp)
    Column(
        modifier
            .fillMaxWidth()
            .heightIn(max = 260.dp)
            .background(WolPalette.Surface, shape)
            .border(1.dp, WolPalette.Line, shape)
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        for (raw in text.lines()) {
            val line = raw.trim()
            when {
                line.isEmpty() -> Unit
                line.startsWith("### ") -> Text(
                    line.removePrefix("### ").uppercase(),
                    style = MaterialTheme.typography.labelSmall.copy(letterSpacing = 0.8.sp, fontWeight = FontWeight.SemiBold),
                    color = WolPalette.Text3,
                    modifier = Modifier.padding(top = 4.dp),
                )
                line.startsWith("- ") || line.startsWith("* ") -> Row {
                    Text("•", color = WolPalette.Blue, style = MaterialTheme.typography.bodyMedium)
                    Spacer(Modifier.width(8.dp))
                    Text(bold(line.drop(2)), style = MaterialTheme.typography.bodyMedium, color = WolPalette.Text)
                }
                else -> Text(bold(line), style = MaterialTheme.typography.bodyMedium, color = WolPalette.Text)
            }
        }
    }
}

private fun bold(text: String): AnnotatedString = buildAnnotatedString {
    text.split("**").forEachIndexed { i, part ->
        if (i % 2 == 1) withStyle(SpanStyle(fontWeight = FontWeight.SemiBold)) { append(part) } else append(part)
    }
}
