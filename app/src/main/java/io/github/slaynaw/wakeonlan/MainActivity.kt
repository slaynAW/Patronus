package io.github.slaynaw.wakeonlan

import android.content.Intent
import android.graphics.Color
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.annotation.RequiresApi
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import io.github.slaynaw.wakeonlan.security.AppLock
import io.github.slaynaw.wakeonlan.ui.WolApp
import io.github.slaynaw.wakeonlan.ui.lock.LockedApp
import io.github.slaynaw.wakeonlan.ui.theme.WolTheme
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        // Thème toujours sombre : icônes claires dans les barres système.
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.dark(Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.dark(Color.TRANSPARENT),
        )
        super.onCreate(savedInstanceState)
        val container = appContainer

        // La surveillance ne tourne que lorsque l'application est visible : pas de consommation
        // de batterie en arrière-plan, et un état rafraîchi dès le retour dans l'application.
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                container.updater.checkIfDue()
                launch { container.historyTracker.run() }
                launch { container.specsTracker.run() }
                launch { container.agentUpgrades.run() }
                launch { container.shareAgents.run() }
                launch { container.receivedPairing.run() }
                launch { container.share.run() }
                launch { container.backups.run() }
                launch { container.archives.run() }
                container.statusMonitor.run(container.allDevices, container.probeAvailability)
            }
        }

        if (savedInstanceState == null) handleLink(intent)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) hideFromRecentsWhenLocked(container.appLock)

        setContent {
            WolTheme {
                // Verrouillage à l'ouverture (facultatif) : l'application reste dessous, masquée et inaccessible.
                LockedApp(container.appLock) { WolApp() }
            }
        }
    }

    /** Verrouillage à l'ouverture : pas d'aperçu de l'application dans les applications récentes. */
    @RequiresApi(Build.VERSION_CODES.TIRAMISU)
    private fun hideFromRecentsWhenLocked(lock: AppLock) {
        lifecycleScope.launch {
            lock.state.map { it.open && it.available }.distinctUntilChanged().collect { setRecentsScreenshotEnabled(!it) }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleLink(intent)
    }

    /** Lien de partage (wolshare://…) ouvert depuis un message : traité par l'onglet Réglages. */
    private fun handleLink(intent: Intent?) {
        val data = intent?.takeIf { it.action == Intent.ACTION_VIEW }?.dataString ?: return
        if (data.startsWith("wolshare://", ignoreCase = true)) appContainer.share.pendingLink.value = data
    }

    override fun onStart() {
        super.onStart()
        appContainer.appLock.onStart()
    }

    override fun onStop() {
        super.onStop()
        appContainer.appLock.onStop()
    }

    override fun onResume() {
        super.onResume()
        // L'utilisateur a pu modifier l'autorisation « réseau local » dans les paramètres.
        appContainer.refreshPermissions()
        // Retour du navigateur après la validation du code GitHub : la connexion reprend aussitôt.
        appContainer.share.onResume()
        appContainer.backups.onResume()
    }
}
