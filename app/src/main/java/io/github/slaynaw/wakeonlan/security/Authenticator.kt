package io.github.slaynaw.wakeonlan.security

import android.app.Activity
import android.app.KeyguardManager
import android.content.Context
import android.content.ContextWrapper
import android.content.Intent
import android.hardware.biometrics.BiometricManager
import android.hardware.biometrics.BiometricPrompt
import android.os.Build
import android.os.CancellationSignal
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.annotation.RequiresApi
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer

/**
 * Demande l'empreinte, le visage ou le code du téléphone, avec les API d'Android (aucune bibliothèque) : fenêtre
 * biométrique du système à partir d'Android 10 (code du téléphone accepté), écran de confirmation du code avant.
 */
class Authenticator internal constructor(
    private val activity: Activity?,
    private val launch: (Intent, (Boolean) -> Unit) -> Unit,
) {
    /** [onResult] reçoit true si la personne s'est authentifiée, false si elle a annulé ou échoué. */
    fun authenticate(title: String, reason: String, onResult: (Boolean) -> Unit) {
        val host = activity ?: return onResult(false)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            prompt(host, title, reason, onResult)
        } else {
            confirmCredential(host, title, reason, onResult)
        }
    }

    @RequiresApi(Build.VERSION_CODES.Q)
    private fun prompt(activity: Activity, title: String, reason: String, onResult: (Boolean) -> Unit) {
        val builder = BiometricPrompt.Builder(activity).setTitle(title).setSubtitle(reason)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            builder.setAllowedAuthenticators(BiometricManager.Authenticators.BIOMETRIC_STRONG or BiometricManager.Authenticators.DEVICE_CREDENTIAL)
        } else {
            @Suppress("DEPRECATION")
            builder.setDeviceCredentialAllowed(true)
        }
        val callback = object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult?) = onResult(true)

            // Annulation, trop d'essais, aucun moyen disponible : refus. Un doigt non reconnu laisse la fenêtre ouverte.
            override fun onAuthenticationError(errorCode: Int, errString: CharSequence?) = onResult(false)
        }
        builder.build().authenticate(CancellationSignal(), activity.mainExecutor, callback)
    }

    private fun confirmCredential(activity: Activity, title: String, reason: String, onResult: (Boolean) -> Unit) {
        @Suppress("DEPRECATION")
        val intent = activity.getSystemService(KeyguardManager::class.java)?.createConfirmDeviceCredentialIntent(title, reason)
        if (intent == null) {
            // Aucun verrouillage d'écran : rien ne peut être demandé.
            onResult(false)
            return
        }
        launch(intent, onResult)
    }
}

/** Demande en attente de l'écran de confirmation du code (Android 9 et avant). */
private class PendingResult {
    var callback: ((Boolean) -> Unit)? = null
}

/** [Authenticator] lié à l'activité affichée. */
@Composable
fun rememberAuthenticator(): Authenticator {
    val context = LocalContext.current
    val pending = remember { PendingResult() }
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        val callback = pending.callback
        pending.callback = null
        callback?.invoke(result.resultCode == Activity.RESULT_OK)
    }
    val activity = remember(context) { context.findActivity() }
    return remember(activity, launcher) {
        Authenticator(activity) { intent, onResult ->
            pending.callback = onResult
            launcher.launch(intent)
        }
    }
}

/**
 * Porte des actions sensibles : l'action s'exécute aussitôt si la confirmation n'est pas demandée (réglage désactivé,
 * pas de verrouillage d'écran, ou authentification récente), après l'empreinte, le visage ou le code sinon.
 */
class SensitiveGate internal constructor(
    private val lock: AppLock,
    private val authenticator: Authenticator,
    private val title: String,
) {
    fun run(reason: String, action: () -> Unit) {
        if (!lock.needsAuth()) {
            action()
            return
        }
        authenticator.authenticate(title, reason) { ok ->
            if (ok) {
                lock.onAuthenticated()
                action()
            }
        }
    }
}

@Composable
fun rememberSensitiveGate(): SensitiveGate {
    val lock = LocalContext.current.appContainer.appLock
    val authenticator = rememberAuthenticator()
    val title = stringResource(R.string.lock_title)
    return remember(lock, authenticator, title) { SensitiveGate(lock, authenticator, title) }
}

private tailrec fun Context.findActivity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.findActivity()
    else -> null
}
