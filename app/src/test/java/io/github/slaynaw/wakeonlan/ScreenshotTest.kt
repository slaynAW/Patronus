package io.github.slaynaw.wakeonlan

import android.app.Application
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onRoot
import com.github.takahirom.roborazzi.captureRoboImage
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.core.agent.AgentDisks
import io.github.slaynaw.wakeonlan.core.agent.AgentDrive
import io.github.slaynaw.wakeonlan.core.agent.AgentSpecs
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.AgentTemperatures
import io.github.slaynaw.wakeonlan.core.agent.AgentVolume
import io.github.slaynaw.wakeonlan.core.agent.SpecsBoard
import io.github.slaynaw.wakeonlan.core.agent.SpecsCpu
import io.github.slaynaw.wakeonlan.core.agent.SpecsGpu
import io.github.slaynaw.wakeonlan.core.agent.SpecsMemory
import io.github.slaynaw.wakeonlan.core.agent.SpecsModule
import io.github.slaynaw.wakeonlan.core.agent.SpecsOs
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
import io.github.slaynaw.wakeonlan.core.status.TempSample
import io.github.slaynaw.wakeonlan.core.status.UnknownReason
import io.github.slaynaw.wakeonlan.core.wol.InterfaceAddress4
import io.github.slaynaw.wakeonlan.data.StoredSpecs
import io.github.slaynaw.wakeonlan.network.LanState
import io.github.slaynaw.wakeonlan.network.LanTransport
import io.github.slaynaw.wakeonlan.ui.detail.DeviceDetailContent
import io.github.slaynaw.wakeonlan.ui.detail.SpecsCard
import io.github.slaynaw.wakeonlan.ui.detail.TempsCard
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
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
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
import kotlin.math.sin

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

    /** Relevés de températures d'exemple sur 5 min, un toutes les 5 s (processeur et carte graphique). */
    private val temps = (0..60).map { i ->
        TempSample(now - 300_000 + i * 5_000L, 52.0 + 9 * sin(i / 9.0) + i % 3, 61.0 + 6 * sin(i / 14.0 + 1))
    }

    private val specs = StoredSpecs(
        AgentSpecs(
            cpu = SpecsCpu("AMD Ryzen 7 5800X", cores = 8, threads = 16, mhz = 3_800),
            memory = SpecsMemory(
                total = 32L shl 30,
                slots = 4,
                modules = listOf(
                    SpecsModule("DIMM_A2", 16L shl 30, "DDR4", 3_200, "G.Skill", "F4-3200C16-16GVK"),
                    SpecsModule("DIMM_B2", 16L shl 30, "DDR4", 3_200, "G.Skill", "F4-3200C16-16GVK"),
                ),
            ),
            gpus = listOf(SpecsGpu("NVIDIA GeForce RTX 4070", 12L shl 30, "32.0.15.6094")),
            board = SpecsBoard("MSI", "MAG B550 TOMAHAWK (MS-7C91)", "1.A0", "2024-03-12"),
            os = SpecsOs("Windows 11 Pro", "24H2, build 26100.2314"),
        ),
        fetched = now - 3_600_000,
        agent = "1.10.0",
    )

    private val items = listOf(
        DeviceItem(
            bureau,
            DeviceStatus(
                state = PowerState.ONLINE,
                since = now - 3_600_000,
                lastSeen = now,
                latencyMs = 2,
                method = ProbeMethod.AGENT,
                agent = AgentStatus(
                    "BUREAU", "windows", "amd64", "1.10.0", 11_520L,
                    temperatures = AgentTemperatures(cpu = 58.0, gpu = 64.0, cpuLoad = 37.0, gpuLoad = 12.0, gpuName = "NVIDIA GeForce RTX 4070"),
                    disks = AgentDisks(
                        volumes = listOf(
                            AgentVolume("C:", "Windows", "NTFS", total = 999_000_000_000, free = 312_000_000_000),
                            AgentVolume("D:", "Jeux", "NTFS", total = 2_000_000_000_000, free = 70_000_000_000),
                        ),
                        drives = listOf(
                            AgentDrive("Samsung SSD 980 PRO 1TB", AgentDrive.SSD, "NVMe", 1_000_204_886_016, AgentDrive.HEALTHY,
                                temp = 41.0, tempMax = 68.0, wear = 3, hours = 1_234, readErrors = 0, writeErrors = 0),
                            AgentDrive("WDC WD20EZRZ-00Z5HB0", AgentDrive.HDD, "SATA", 2_000_398_934_016, AgentDrive.WARNING,
                                temp = 44.0, hours = 31_877, readErrors = 12, writeErrors = 0),
                        ),
                        errors = 5,
                        lastError = now / 1000 - 26 * 3600,
                    ),
                ),
            ),
            series(1_000) { i ->
                when {
                    i == 41 -> null
                    i % 23 == 5 -> 11L + i % 5
                    else -> 2L + (i * 7) % 3
                }
            },
            temps = temps,
            specs = specs,
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

    private fun event(
        device: String,
        name: String,
        minutesAgo: Long,
        kind: HistoryKind,
        source: HistorySource,
        approx: Boolean = false,
        client: String? = null,
        cause: String? = null,
        detail: String? = null,
    ) = HistoryItem(HistoryEvent(device, now - minutesAgo * 60_000, kind, source, approx, client, cause, detail), name)

    private val events = listOf(
        event("nas", "NAS du salon", 0, HistoryKind.WAKE_SENT, HistorySource.APP),
        event("bureau", "PC Bureau", 192, HistoryKind.ON, HistorySource.AGENT),
        event("bureau", "PC Bureau", 193, HistoryKind.WAKE_SENT, HistorySource.APP),
        event("salon", "PC Salon", 240, HistoryKind.OFF, HistorySource.AGENT),
        event("salon", "PC Salon", 241, HistoryKind.SHUTDOWN_SENT, HistorySource.AGENT, client = "192.168.1.37"),
        event("jeux", "Serveur de jeux", 1_130, HistoryKind.RESUME, HistorySource.AGENT),
        event("jeux", "Serveur de jeux", 1_560, HistoryKind.SLEEP, HistorySource.AGENT),
        event("bureau", "PC Bureau", 1_500, HistoryKind.ON, HistorySource.AGENT),
        event("bureau", "PC Bureau", 1_510, HistoryKind.LOST, HistorySource.AGENT, cause = "bsod", detail = "0x7E SYSTEM_THREAD_EXCEPTION_NOT_HANDLED"),
        event("bureau", "PC Bureau", 1_700, HistoryKind.OFF, HistorySource.APP, approx = true),
        event("bureau", "PC Bureau", 1_702, HistoryKind.SHUTDOWN_SENT, HistorySource.APP),
        event("salon", "PC Salon", 3_900, HistoryKind.LOST, HistorySource.AGENT, cause = "power"),
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
    fun temperaturesEtFiche() = capture("3c-temperatures-et-fiche-pc") {
        Column(
            Modifier.background(WolPalette.Background).verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            TempsCard(items.first(), now)
            SpecsCard(items.first(), now)
        }
    }

    /** Puce graphique intégrée : même température que le processeur, courbe GPU en pointillés. */
    @Test
    fun temperaturesGpuIntegre() = capture("3d-temperatures-gpu-integre") {
        val base = items.first()
        val integrated = AgentTemperatures(cpu = 58.0, gpu = 58.0, gpuShared = true, gpuName = "Intel(R) UHD Graphics 770")
        val item = base.copy(
            status = base.status.copy(agent = base.status.agent?.copy(temperatures = integrated)),
            temps = temps.map { it.copy(gpu = it.cpu, gpuShared = true) },
        )
        Column(Modifier.background(WolPalette.Background).padding(16.dp)) {
            TempsCard(item, now)
        }
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
