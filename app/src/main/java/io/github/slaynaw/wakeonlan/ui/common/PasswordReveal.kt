package io.github.slaynaw.wakeonlan.ui.common

import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/**
 * Mot de passe affiché en clair tant qu'on appuie sur l'œil ([PasswordRevealIcon]) et masqué dès
 * qu'on relâche : on vérifie ce qu'on tape sans le laisser affiché.
 */
class PasswordReveal(val interaction: MutableInteractionSource, val visible: Boolean) {
    val transformation: VisualTransformation get() = if (visible) VisualTransformation.None else PasswordVisualTransformation()
}

@Composable
fun rememberPasswordReveal(): PasswordReveal {
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    return PasswordReveal(interaction, pressed)
}

/** Œil à maintenir appuyé (icône de fin d'un champ de mot de passe). */
@Composable
fun PasswordRevealIcon(reveal: PasswordReveal) {
    IconButton(onClick = {}, interactionSource = reveal.interaction) {
        Icon(WolIcons.Eye, contentDescription = stringResource(R.string.password_hold), tint = if (reveal.visible) WolPalette.Blue else WolPalette.Text3)
    }
}
