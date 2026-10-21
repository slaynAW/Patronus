package io.github.slaynaw.wakeonlan.ui.lock

import android.app.Activity
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.Image
import androidx.compose.foundation.focusGroup
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBarsPadding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.security.AppLock
import io.github.slaynaw.wakeonlan.security.rememberAuthenticator
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/**
 * Application verrouillée à l'ouverture (réglage facultatif) : [content] reste composée dessous, pour garder la
 * navigation, mais masquée et inaccessible (ni toucher, ni lecteur d'écran, ni clavier) tant que l'authentification
 * n'a pas réussi.
 */
@Composable
fun LockedApp(lock: AppLock, content: @Composable () -> Unit) {
    val state by lock.state.collectAsStateWithLifecycle()
    val locked = state.locked
    val focus = LocalFocusManager.current
    // Ce qui avait le focus (clavier physique) le perd : aucune touche ne doit atteindre l'application masquée.
    LaunchedEffect(locked) { if (locked) focus.clearFocus(force = true) }
    Box(Modifier.fillMaxSize()) {
        Box(
            Modifier
                .fillMaxSize()
                .then(if (locked) Modifier.clearAndSetSemantics {} else Modifier)
                // Ni Tab, ni flèches, ni demande de focus du code n'y entrent tant qu'elle est verrouillée.
                .focusProperties { onEnter = { if (locked) cancelFocusChange() } }
                .focusGroup(),
        ) { content() }
        if (locked) {
            val context = LocalContext.current
            val authenticator = rememberAuthenticator()
            val title = stringResource(R.string.lock_title)
            val reason = stringResource(R.string.lock_reason_open)
            val unlock = { authenticator.authenticate(title, reason) { ok -> if (ok) lock.onAuthenticated() } }
            // Demande aussitôt ; le bouton permet de recommencer après une annulation.
            LaunchedEffect(Unit) { unlock() }
            // Retour : l'application passe en arrière-plan, sans rien montrer de ce qu'il y a dessous.
            BackHandler { (context as? Activity)?.moveTaskToBack(true) }
            LockScreenContent(onUnlock = unlock)
        }
    }
}

/** Écran de déverrouillage (sans état : aperçus et captures d'écran). */
@Composable
fun LockScreenContent(onUnlock: () -> Unit) {
    Surface(
        // Aucun toucher ne traverse vers l'application masquée.
        Modifier.fillMaxSize().pointerInput(Unit) {},
        color = WolPalette.Background,
    ) {
        Column(
            Modifier.fillMaxSize().systemBarsPadding().padding(horizontal = 32.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Image(painterResource(R.drawable.ic_logo), contentDescription = null, modifier = Modifier.size(72.dp))
            Spacer(Modifier.height(24.dp))
            Text(
                stringResource(R.string.lock_screen_title),
                style = MaterialTheme.typography.titleLarge,
                color = WolPalette.Text,
                textAlign = TextAlign.Center,
            )
            Spacer(Modifier.height(8.dp))
            Text(
                stringResource(R.string.lock_screen_text),
                style = MaterialTheme.typography.bodyMedium,
                color = WolPalette.Text2,
                textAlign = TextAlign.Center,
            )
            Spacer(Modifier.height(28.dp))
            WolButton(
                stringResource(R.string.lock_screen_unlock),
                onClick = onUnlock,
                modifier = Modifier.fillMaxWidth(),
                kind = ButtonKind.PRIMARY,
                icon = WolIcons.Lock,
                height = 48.dp,
            )
        }
    }
}
