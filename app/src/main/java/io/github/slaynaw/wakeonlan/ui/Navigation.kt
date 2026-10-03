package io.github.slaynaw.wakeonlan.ui

import androidx.compose.runtime.Composable
import androidx.navigation.NavBackStackEntry
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.toRoute
import io.github.slaynaw.wakeonlan.ui.detail.DeviceDetailScreen
import io.github.slaynaw.wakeonlan.ui.edit.EditDeviceScreen
import io.github.slaynaw.wakeonlan.ui.history.HistoryScreen
import io.github.slaynaw.wakeonlan.ui.main.MainScreen
import io.github.slaynaw.wakeonlan.ui.metrics.MetricsScreen
import kotlinx.serialization.Serializable

/** Écran principal (onglets vue d'ensemble, appareils, réglages). */
@Serializable
object MainRoute

/** Fiche d'un PC. */
@Serializable
data class DeviceRoute(val deviceId: String)

/** Édition d'un terminal ; [deviceId] nul = nouveau terminal. */
@Serializable
data class EditDeviceRoute(val deviceId: String? = null)

/** Mesures d'un PC dans le temps (températures, utilisation, journal archivé). */
@Serializable
data class MetricsRoute(val deviceId: String)

/** Historique complet ; [deviceId] nul = tous les PC. */
@Serializable
data class HistoryRoute(val deviceId: String? = null)

@Composable
fun WolApp() {
    val nav = rememberNavController()
    NavHost(navController = nav, startDestination = MainRoute) {
        composable<MainRoute> {
            MainScreen(
                onOpenDevice = { id -> nav.navigate(DeviceRoute(id)) },
                onAddDevice = { nav.navigate(EditDeviceRoute()) },
                onEditDevice = { id -> nav.navigate(EditDeviceRoute(id)) },
                onOpenHistory = { id -> nav.navigate(HistoryRoute(id)) },
            )
        }
        composable<DeviceRoute> { entry ->
            DeviceDetailScreen(
                deviceId = entry.toRoute<DeviceRoute>().deviceId,
                onBack = { nav.leave(entry) },
                onEdit = { id -> nav.navigate(EditDeviceRoute(id)) },
                onOpenHistory = { id -> nav.navigate(HistoryRoute(id)) },
                onOpenMetrics = { id -> nav.navigate(MetricsRoute(id)) },
            )
        }
        composable<MetricsRoute> { entry ->
            MetricsScreen(
                deviceId = entry.toRoute<MetricsRoute>().deviceId,
                onBack = { nav.leave(entry) },
            )
        }
        composable<EditDeviceRoute> { entry ->
            EditDeviceScreen(
                deviceId = entry.toRoute<EditDeviceRoute>().deviceId,
                onDone = { nav.leave(entry) },
            )
        }
        composable<HistoryRoute> { entry ->
            HistoryScreen(
                deviceId = entry.toRoute<HistoryRoute>().deviceId,
                onBack = { nav.leave(entry) },
            )
        }
    }
}

/** Quitte l'écran [entry] s'il est toujours affiché (un double appui ne ferme pas l'écran d'en dessous). */
private fun NavHostController.leave(entry: NavBackStackEntry) {
    if (currentBackStackEntry == entry) popBackStack()
}
