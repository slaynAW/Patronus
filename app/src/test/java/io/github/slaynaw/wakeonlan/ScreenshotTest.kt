package io.github.slaynaw.wakeonlan

import android.app.Application
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onRoot
import com.github.takahirom.roborazzi.captureRoboImage
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.share.ShareAccess
import io.github.slaynaw.wakeonlan.core.share.ShareCrypto
import io.github.slaynaw.wakeonlan.core.share.ShareInvite
import io.github.slaynaw.wakeonlan.core.share.ShareOwner
import io.github.slaynaw.wakeonlan.core.share.SharePerson
import io.github.slaynaw.wakeonlan.core.share.ShareRight
import io.github.slaynaw.wakeonlan.share.ShareUiState
import io.github.slaynaw.wakeonlan.core.history.HistoryEvent
import io.github.slaynaw.wakeonlan.core.history.HistoryKind
import io.github.slaynaw.wakeonlan.core.history.HistorySource
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.AppSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import io.github.slaynaw.wakeonlan.core.status.DeviceStatus
import io.github.slaynaw.wakeonlan.core.status.LatencySample
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.core.status.ProbeMethod
import io.github.slaynaw.wakeonlan.core.status.UnknownReason
import io.github.slaynaw.wakeonlan.core.wol.InterfaceAddress4
import io.github.slaynaw.wakeonlan.network.LanState
import io.github.slaynaw.wakeonlan.network.LanTransport
import io.github.slaynaw.wakeonlan.ui.detail.DeviceDetailContent
import io.github.slaynaw.wakeonlan.ui.devices.DeviceItem
import io.github.slaynaw.wakeonlan.ui.devices.DevicesTab
import io.github.slaynaw.wakeonlan.ui.devices.DevicesUiState
import io.github.slaynaw.wakeonlan.ui.history.HistoryContent
import io.github.slaynaw.wakeonlan.ui.history.HistoryItem
import io.github.slaynaw.wakeonlan.ui.history.HistoryUiState
import io.github.slaynaw.wakeonlan.ui.main.MainScaffold
import io.github.slaynaw.wakeonlan.ui.main.MainTab
import io.github.slaynaw.wakeonlan.ui.overview.OverviewTab
import io.github.slaynaw.wakeonlan.ui.settings.QrImage
import io.github.slaynaw.wakeonlan.ui.settings.SettingsContent
import io.github.slaynaw.wakeonlan.ui.settings.ShareSections
import io.github.slaynaw.wakeonlan.ui.theme.WolTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import org.robolectric.shadows.ShadowNetwork
import java.net.Inet4Address
import java.net.InetAddress

/**
 * Captures d'écran des principaux écrans, rendues sur la JVM (Robolectric, rendu natif) avec des
 * données d'exemple : vérifie aussi que chaque écran se compose sans erreur. Images produites dans
 * app/build/captures/android (artefact « captures-android » de la CI).
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = "w411dp-h914dp-420dpi", application = Application::class)
class ScreenshotTest {
    @get:Rule
    val compose = createComposeRule()

    private val now = System.currentTimeMillis()
    private val key = "A".repeat(43)

    private val bureau = Device(
        id = "bureau",
        name = "PC Bureau",
        mac = MacAddress.parse("AA:BB:CC:DD:EE:01"),
        host = "192.168.1.20",
        agent = AgentSettings(key = key),
    )

    /** Mesures de latence d'exemple sur 75 s, une toutes les [stepMs]. */
    private fun series(stepMs: Long, value: (Int) -> Long?) =
        (0..(75_000 / stepMs).toInt()).map { i -> LatencySample(now - 75_000 + i * stepMs, value(i)) }

    private val items = listOf(
        DeviceItem(
            bureau,
            DeviceStatus(
                state = PowerState.ONLINE,
                since = now - 3_600_000,
                lastSeen = now,
                latencyMs = 2,
                method = ProbeMethod.AGENT,
                agent = AgentStatus("BUREAU", "windows", "amd64", "1.2.0", 11_520L),
            ),
            series(1_000) { i ->
                when {
                    i == 41 -> null
                    i % 23 == 5 -> 11L + i % 5
                    else -> 2L + (i * 7) % 3
                }
            },
        ),
        DeviceItem(
            Device("jeux", "Serveur de jeux", MacAddress.parse("AA:BB:CC:DD:EE:02"), host = "192.168.1.30", agent = AgentSettings(key = key)),
            DeviceStatus(PowerState.ONLINE, since = now - 7_200_000, lastSeen = now, latencyMs = 5, method = ProbeMethod.AGENT,
                agent = AgentStatus("SERVEUR", "linux", "amd64", "1.2.0", 266_400L)),
            series(3_000) { i -> 4L + (i * 5) % 3 },
        ),
        DeviceItem(
            Device("nas", "NAS du salon", MacAddress.parse("AA:BB:CC:DD:EE:03"), host = "192.168.1.40"),
            DeviceStatus(PowerState.WAKING, since = now - 12_000, actionStartedAt = now - 12_000),
        ),
        DeviceItem(
            Device("salon", "PC Salon", MacAddress.parse("AA:BB:CC:DD:EE:04"), host = "192.168.1.21", agent = AgentSettings(key = key)),
            DeviceStatus(PowerState.OFFLINE, since = now - 7_200_000, lastSeen = now - 7_200_000),
            series(3_000) { null },
        ),
        DeviceItem(
            Device("atelier", "Portable atelier", MacAddress.parse("AA:BB:CC:DD:EE:05")),
            DeviceStatus(PowerState.UNKNOWN, unknownReason = UnknownReason.NO_HOST),
        ),
    )

    private val devicesState = DevicesUiState(
        loaded = true,
        items = items,
        settings = AppSettings(),
        lan = LanState(
            network = ShadowNetwork.newInstance(100),
            transport = LanTransport.WIFI,
            addresses = listOf(InterfaceAddress4(InetAddress.getByName("192.168.1.23") as Inet4Address, 24)),
        ),
    )

    private fun event(device: String, name: String, minutesAgo: Long, kind: HistoryKind, source: HistorySource, approx: Boolean = false, client: String? = null) =
        HistoryItem(HistoryEvent(device, now - minutesAgo * 60_000, kind, source, approx, client), name)

    private val events = listOf(
        event("nas", "NAS du salon", 0, HistoryKind.WAKE_SENT, HistorySource.APP),
        event("bureau", "PC Bureau", 192, HistoryKind.ON, HistorySource.AGENT),
        event("bureau", "PC Bureau", 193, HistoryKind.WAKE_SENT, HistorySource.APP),
        event("salon", "PC Salon", 240, HistoryKind.OFF, HistorySource.AGENT),
        event("salon", "PC Salon", 241, HistoryKind.SHUTDOWN_SENT, HistorySource.AGENT, client = "192.168.1.37"),
        event("jeux", "Serveur de jeux", 1_130, HistoryKind.RESUME, HistorySource.AGENT),
        event("jeux", "Serveur de jeux", 1_560, HistoryKind.SLEEP, HistorySource.AGENT),
        event("bureau", "PC Bureau", 1_700, HistoryKind.OFF, HistorySource.APP, approx = true),
        event("bureau", "PC Bureau", 1_702, HistoryKind.SHUTDOWN_SENT, HistorySource.APP),
        event("salon", "PC Salon", 3_900, HistoryKind.LOST, HistorySource.AGENT),
    )

    private fun capture(name: String, content: @Composable () -> Unit) {
        compose.mainClock.autoAdvance = false
        compose.setContent { WolTheme { content() } }
        compose.mainClock.advanceTimeBy(1_000)
        compose.onRoot().captureRoboImage("build/captures/android/$name.png")
    }

    @Test
    fun vueEnsemble() = capture("1-vue-ensemble") {
        MainScaffold(MainTab.OVERVIEW, {}, remember { SnackbarHostState() }, showAddButton = false, onAddDevice = {}) { padding ->
            OverviewTab(devicesState, padding, {}, {}, {}, {}, {}, {})
        }
    }

    @Test
    fun appareils() = capture("2-appareils") {
        MainScaffold(MainTab.DEVICES, {}, remember { SnackbarHostState() }, showAddButton = true, onAddDevice = {}) { padding ->
            DevicesTab(devicesState, padding, {}, {}, {}, {}, {}, { _, _ -> }, { _, _ -> }, {}, {}, {})
        }
    }

    @Test
    fun fiche() = capture("3-fiche-pc") {
        DeviceDetailContent(
            item = items.first(),
            isFirst = true,
            isLast = false,
            history = HistoryUiState(
                loaded = true,
                filter = "bureau",
                events = events.filter { it.event.device == "bureau" },
                hasAgent = true,
            ),
            now = now,
            snackbar = remember { SnackbarHostState() },
            onBack = {},
            onWake = {},
            onPower = {},
            onEdit = {},
            onOpenHistory = {},
            onMove = {},
            onDelete = {},
            onClearNotice = {},
        )
    }

    @Test
    fun ficheEteinte() = capture("3b-fiche-pc-eteint") {
        DeviceDetailContent(
            item = items[3],
            isFirst = false,
            isLast = false,
            history = HistoryUiState(loaded = true, filter = "salon", events = events.filter { it.event.device == "salon" }, hasAgent = true),
            now = now,
            snackbar = remember { SnackbarHostState() },
            onBack = {},
            onWake = {},
            onPower = {},
            onEdit = {},
            onOpenHistory = {},
            onMove = {},
            onDelete = {},
            onClearNotice = {},
        )
    }

    @Test
    fun historique() = capture("4-historique") {
        HistoryContent(
            state = HistoryUiState(loaded = true, events = events, devices = items.map { it.device.id to it.device.name }),
            now = now,
            onBack = {},
            onRefresh = {},
            onSelect = {},
        )
    }

    @Test
    fun reglages() = capture("5-reglages") {
        MainScaffold(MainTab.SETTINGS, {}, remember { SnackbarHostState() }, showAddButton = false, onAddDevice = {}) { padding ->
            SettingsContent(padding, AppSettings(), deviceCount = items.size, busy = false, version = "1.2.0", {}, {}, {}, {}, {}, {})
        }
    }

    /** Partage : Hugo partage le PC Bureau avec Léa, et reçoit le PC de Paul. */
    private val shareState: ShareUiState by lazy {
        val key = ShareCrypto.newKeyPair()
        val lea = ShareCrypto.newKeyPair()
        ShareUiState(
            loaded = true,
            canLogin = true,
            owner = ShareOwner(key = key.privateEncoded, publicKey = key.publicEncoded, name = "Hugo", token = "t", user = "hugo", gist = "0123456789abcdef0123456789abcdef")
                .grant(SharePerson("Léa", lea.publicEncoded, now, mapOf("bureau" to ShareRight.WAKE)))
                .let { it.copy(published = it.people.associate { p -> p.device to "x" }) },
            received = listOf(
                ShareAccess(
                    owner = ShareCrypto.newKeyPair().publicEncoded, ownerName = "Paul", user = "paul", gist = "fedcba9876543210fedcba9876543210",
                    myName = "Hugo", active = true, devices = listOf(items[2].device), synced = now - 120_000,
                ),
            ),
        )
    }

    @Test
    fun partage() = capture("7-reglages-partage") {
        MainScaffold(MainTab.SETTINGS, {}, remember { SnackbarHostState() }, showAddButton = false, onAddDevice = {}) { padding ->
            Column(Modifier.padding(padding).verticalScroll(rememberScrollState()).padding(16.dp)) {
                ShareSections(share = shareState, ownDevices = items.map { it.device }, now = now, onDialog = {})
            }
        }
    }

    @Test
    fun fichePartagee() = capture("7b-fiche-pc-partage") {
        DeviceDetailContent(
            item = items[3].copy(sharedBy = "Paul", device = items[3].device.copy(agent = null)),
            isFirst = false,
            isLast = false,
            history = HistoryUiState(loaded = true, filter = "salon"),
            now = now,
            snackbar = remember { SnackbarHostState() },
            onBack = {},
            onWake = {},
            onPower = {},
            onEdit = {},
            onOpenHistory = {},
            onMove = {},
            onDelete = {},
            onClearNotice = {},
        )
    }

    @Test
    fun qrCode() = capture("7c-qr-invitation") {
        Box(Modifier.padding(24.dp)) {
            QrImage(ShareInvite("Hugo", ShareCrypto.newKeyPair().publicEncoded, "hugo", "0123456789abcdef0123456789abcdef").link)
        }
    }

    @Test
    fun vide() = capture("6-premier-lancement") {
        MainScaffold(MainTab.OVERVIEW, {}, remember { SnackbarHostState() }, showAddButton = false, onAddDevice = {}) { padding ->
            OverviewTab(devicesState.copy(items = emptyList()), padding, {}, {}, {}, {}, {}, {})
        }
    }
}
