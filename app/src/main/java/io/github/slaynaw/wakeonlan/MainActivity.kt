package io.github.slaynaw.wakeonlan

import android.content.Intent
import android.graphics.Color
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import io.github.slaynaw.wakeonlan.ui.WolApp
import io.github.slaynaw.wakeonlan.ui.theme.WolTheme
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
                launch { container.share.run() }
                launch { container.backups.run() }
                launch { container.archives.run() }
                container.statusMonitor.run(container.allDevices, container.probeAvailability)
            }
        }

        if (savedInstanceState == null) handleLink(intent)

        setContent {
            WolTheme {
                WolApp()
            }
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

    override fun onResume() {
        super.onResume()
        // L'utilisateur a pu modifier l'autorisation « réseau local » dans les paramètres.
        appContainer.refreshPermissions()
        // Retour du navigateur après la validation du code GitHub : la connexion reprend aussitôt.
        appContainer.share.onResume()
        appContainer.backups.onResume()
    }
}
