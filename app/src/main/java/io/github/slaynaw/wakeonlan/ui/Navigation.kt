package io.github.slaynaw.wakeonlan.ui

import androidx.compose.runtime.Composable
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.toRoute
import io.github.slaynaw.wakeonlan.ui.devices.DevicesScreen
import io.github.slaynaw.wakeonlan.ui.edit.EditDeviceScreen
import io.github.slaynaw.wakeonlan.ui.settings.SettingsScreen
import kotlinx.serialization.Serializable

@Serializable
object DevicesRoute

/** Édition d'un terminal ; [deviceId] nul = nouveau terminal. */
@Serializable
data class EditDeviceRoute(val deviceId: String? = null)

@Serializable
object SettingsRoute

@Composable
fun WolApp() {
    val nav = rememberNavController()
    NavHost(navController = nav, startDestination = DevicesRoute) {
        composable<DevicesRoute> {
            DevicesScreen(
                onAddDevice = { nav.navigate(EditDeviceRoute()) },
                onEditDevice = { id -> nav.navigate(EditDeviceRoute(id)) },
                onOpenSettings = { nav.navigate(SettingsRoute) },
            )
        }
        composable<EditDeviceRoute> { entry ->
            EditDeviceScreen(
                deviceId = entry.toRoute<EditDeviceRoute>().deviceId,
                onDone = { nav.popBackStack() },
            )
        }
        composable<SettingsRoute> {
            SettingsScreen(onBack = { nav.popBackStack() })
        }
    }
}
