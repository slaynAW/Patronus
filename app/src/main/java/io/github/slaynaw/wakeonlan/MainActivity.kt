package io.github.slaynaw.wakeonlan

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
                launch { container.historyTracker.run() }
                container.statusMonitor.run(container.repository.config, container.probeAvailability)
            }
        }

        setContent {
            WolTheme {
                WolApp()
            }
        }
    }

    override fun onResume() {
        super.onResume()
        // L'utilisateur a pu modifier l'autorisation « réseau local » dans les paramètres.
        appContainer.refreshPermissions()
    }
}
