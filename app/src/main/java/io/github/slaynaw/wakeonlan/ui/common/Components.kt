package io.github.slaynaw.wakeonlan.ui.common

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.ui.theme.LocalStatusColors
import io.github.slaynaw.wakeonlan.ui.theme.MonoStyle
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/** Couleur du voyant d'un état. */
@Composable
fun statusColor(state: PowerState): Color {
    val colors = LocalStatusColors.current
    return when (state) {
        PowerState.ONLINE -> colors.online
        PowerState.OFFLINE -> colors.offline
        PowerState.UNKNOWN -> colors.unknown
        PowerState.WAKING, PowerState.SHUTTING_DOWN, PowerState.RESTARTING -> colors.transition
    }
}

/** Couleur du texte d'un état (le gris des états inconnus reste lisible). */
@Composable
fun statusTextColor(state: PowerState): Color = if (state == PowerState.UNKNOWN) WolPalette.Text2 else statusColor(state)

/** Voyant d'état : halo pour « allumé », clignotement pendant une action en cours. */
@Composable
fun StatusDot(state: PowerState, modifier: Modifier = Modifier, size: Dp = 8.dp) {
    val color = statusColor(state)
    val transition = rememberInfiniteTransition(label = "statusPulse")
    val pulse by transition.animateFloat(
        initialValue = 0.35f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(durationMillis = 800), RepeatMode.Reverse),
        label = "statusAlpha",
    )
    val description = stringResource(state.label())
    Box(
        modifier
            .size(size)
            .graphicsLayer { alpha = if (state.isTransitional) pulse else 1f }
            .drawBehind {
                if (state == PowerState.ONLINE) drawCircle(color.copy(alpha = 0.28f), radius = this.size.minDimension)
            }
            .background(color, CircleShape)
            .semantics { contentDescription = description },
    )
}

/** Carte : surface sombre et fine bordure ; cliquable si [onClick] est fourni. */
@Composable
fun WolCard(modifier: Modifier = Modifier, onClick: (() -> Unit)? = null, content: @Composable ColumnScope.() -> Unit) {
    val shape = RoundedCornerShape(14.dp)
    Column(
        modifier
            .fillMaxWidth()
            .clip(shape)
            .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
            .background(WolPalette.Surface)
            .border(1.dp, WolPalette.Line, shape),
        content = content,
    )
}

/** Séparateur entre deux lignes d'une carte. */
@Composable
fun RowDivider() {
    HorizontalDivider(thickness = 1.dp, color = WolPalette.Line)
}

/** Titre de section en petites capitales, avec une action facultative à droite. */
@Composable
fun SectionLabel(text: String, modifier: Modifier = Modifier, action: String? = null, onAction: () -> Unit = {}) {
    Row(modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            text.uppercase(),
            style = MaterialTheme.typography.labelSmall.copy(letterSpacing = 0.9.sp, fontWeight = FontWeight.SemiBold),
            color = WolPalette.Text3,
            modifier = Modifier.weight(1f),
        )
        if (action != null) {
            TextButton(onClick = onAction, contentPadding = PaddingValues(horizontal = 8.dp)) {
                Text(action, style = MaterialTheme.typography.labelLarge, color = WolPalette.Blue)
            }
        }
    }
}

enum class ButtonKind { PRIMARY, SECONDARY, DANGER }

/** Bouton aux couleurs du thème (bleu, gris bordé ou rouge sombre). */
@Composable
fun WolButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    kind: ButtonKind = ButtonKind.SECONDARY,
    icon: ImageVector? = null,
    enabled: Boolean = true,
    height: Dp = 44.dp,
) {
    val (container, content) = when (kind) {
        ButtonKind.PRIMARY -> WolPalette.Blue to Color.White
        ButtonKind.SECONDARY -> WolPalette.Surface2 to WolPalette.Text
        ButtonKind.DANGER -> WolPalette.DangerBackground to WolPalette.DangerText
    }
    Button(
        onClick = onClick,
        enabled = enabled,
        shape = RoundedCornerShape(10.dp),
        colors = ButtonDefaults.buttonColors(
            containerColor = container,
            contentColor = content,
            disabledContainerColor = container.copy(alpha = 0.4f),
            disabledContentColor = content.copy(alpha = 0.5f),
        ),
        border = if (kind == ButtonKind.SECONDARY) BorderStroke(1.dp, WolPalette.Line) else null,
        contentPadding = PaddingValues(horizontal = 14.dp),
        modifier = modifier.height(height),
    ) {
        if (icon != null) {
            Icon(icon, contentDescription = null, modifier = Modifier.size(17.dp))
            Spacer(Modifier.width(8.dp))
        }
        Text(text, style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

/** Bouton rond, icône seule. */
@Composable
fun RoundIconButton(
    icon: ImageVector,
    contentDescription: String?,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    container: Color = WolPalette.Surface2,
    content: Color = WolPalette.Text2,
    bordered: Boolean = true,
    size: Dp = 40.dp,
) {
    IconButton(
        onClick = onClick,
        colors = IconButtonDefaults.iconButtonColors(containerColor = container, contentColor = content),
        modifier = modifier
            .size(size)
            .then(if (bordered) Modifier.border(1.dp, WolPalette.Line, CircleShape) else Modifier),
    ) {
        Icon(icon, contentDescription = contentDescription, modifier = Modifier.size(19.dp))
    }
}

/** Petit indicateur d'activité (couleur « en cours »). */
@Composable
fun BusySpinner(modifier: Modifier = Modifier, size: Dp = 18.dp) {
    CircularProgressIndicator(
        modifier = modifier.size(size),
        color = WolPalette.Busy,
        trackColor = Color(0xFF3A3020),
        strokeWidth = 2.dp,
    )
}

/** Icône d'un PC dans un carré arrondi (bleue quand le PC est allumé). */
@Composable
fun DeviceGlyph(state: PowerState?, modifier: Modifier = Modifier, size: Dp = 36.dp, icon: ImageVector = WolIcons.Monitor) {
    Box(
        modifier
            .size(size)
            .clip(RoundedCornerShape(10.dp))
            .background(WolPalette.Soft),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            icon,
            contentDescription = null,
            tint = if (state == PowerState.ONLINE || state == null) WolPalette.Blue else WolPalette.Text2,
            modifier = Modifier.size(size * 0.55f),
        )
    }
}

/** Ligne « libellé — valeur » d'une carte d'informations. */
@Composable
fun KeyValueRow(
    label: String,
    value: String,
    modifier: Modifier = Modifier,
    mono: Boolean = false,
    valueColor: Color = WolPalette.Text,
    icon: ImageVector? = null,
    bold: Boolean = false,
) {
    Row(
        modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(label, style = MaterialTheme.typography.bodyMedium, color = WolPalette.Text2)
        Spacer(Modifier.weight(1f))
        if (icon != null) Icon(icon, contentDescription = null, tint = valueColor, modifier = Modifier.size(15.dp))
        val style: TextStyle = if (mono) MonoStyle else MaterialTheme.typography.bodyMedium
        Text(
            value,
            style = style,
            color = valueColor,
            fontWeight = if (bold) FontWeight.SemiBold else null,
            textAlign = TextAlign.End,
        )
    }
}

/** Bandeau d'alerte (réseau absent, autorisation manquante…). */
@Composable
fun WarningBanner(title: String, text: String, modifier: Modifier = Modifier, action: String? = null, onAction: () -> Unit = {}) {
    val shape = RoundedCornerShape(12.dp)
    Row(
        modifier
            .fillMaxWidth()
            .clip(shape)
            .background(WolPalette.DangerBackground)
            .border(1.dp, Color(0xFF4A2329), shape)
            .padding(14.dp),
    ) {
        Icon(WolIcons.Warning, contentDescription = null, tint = WolPalette.DangerText, modifier = Modifier.size(20.dp))
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.titleSmall, color = Color(0xFFFFD0D4))
            Text(text, style = MaterialTheme.typography.bodyMedium, color = WolPalette.DangerText)
            if (action != null) {
                TextButton(onClick = onAction, contentPadding = PaddingValues(0.dp)) {
                    Text(action, color = WolPalette.Text, style = MaterialTheme.typography.labelLarge)
                }
            }
        }
    }
}

/** Message à acquitter (pas de réponse au démarrage, PC qui ne s'éteint pas…). */
@Composable
fun NoticeBox(text: String, onDismiss: () -> Unit, modifier: Modifier = Modifier) {
    Row(
        modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(WolPalette.DangerBackground)
            .padding(start = 12.dp, top = 10.dp, bottom = 10.dp, end = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(WolIcons.Warning, contentDescription = null, tint = WolPalette.DangerText, modifier = Modifier.size(16.dp))
        Spacer(Modifier.width(8.dp))
        Text(text, style = MaterialTheme.typography.bodySmall, color = WolPalette.DangerText, modifier = Modifier.weight(1f))
        TextButton(onClick = onDismiss) { Text(stringResource(R.string.ok), color = WolPalette.Blue) }
    }
}

/** Espace vide en bas des listes (au-dessus de la barre de navigation). */
@Composable
fun BottomSpace(height: Dp = 24.dp) {
    Spacer(Modifier.height(height))
}
