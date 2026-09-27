package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentKey
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.model.MacAddress
import io.github.slaynaw.wakeonlan.core.status.HostProber
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.core.status.ProbeAvailability
import io.github.slaynaw.wakeonlan.core.status.ProbeMethod
import io.github.slaynaw.wakeonlan.core.status.ProbeResult
import io.github.slaynaw.wakeonlan.core.status.StatusMonitor
import io.github.slaynaw.wakeonlan.core.status.StatusNotice
import io.github.slaynaw.wakeonlan.core.status.StatusTracker
import io.github.slaynaw.wakeonlan.core.status.UnknownReason
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.net.InetAddress
import java.net.ServerSocket
import kotlin.time.TimeSource

class StatusTrackerTest {
    private var now = 1_000_000L
    private val tracker = StatusTracker(clock = { now }, wakeTimeoutMs = 60_000, shutdownTimeoutMs = 30_000)
    private val up = ProbeResult(true, 3, ProbeMethod.TCP)
    private val down = ProbeResult.UNREACHABLE

    private fun tick(ms: Long = 3_000) {
        now += ms
    }

    @Test
    fun `premier echec affiche hors ligne immediatement`() {
        assertEquals(PowerState.UNKNOWN, tracker.status.state)
        assertEquals(PowerState.OFFLINE, tracker.onProbe(down).state)
    }

    @Test
    fun `hysteresis avant de passer hors ligne`() {
        tracker.onProbe(up)
        tick()
        assertEquals(PowerState.ONLINE, tracker.onProbe(down).state, "un seul paquet perdu ne suffit pas")
        tick()
        assertEquals(PowerState.ONLINE, tracker.onProbe(up).state)
        tick()
        tracker.onProbe(down)
        tick()
        val s = tracker.onProbe(down)
        assertEquals(PowerState.OFFLINE, s.state)
        assertEquals(now, s.since)
        assertEquals(now - 6_000, s.lastSeen)
    }

    @Test
    fun `reveil reussi`() {
        tracker.onProbe(down)
        tick()
        assertEquals(PowerState.WAKING, tracker.onWakeSent().state)
        repeat(5) {
            tick()
            assertEquals(PowerState.WAKING, tracker.onProbe(down).state)
        }
        tick()
        val s = tracker.onProbe(up)
        assertEquals(PowerState.ONLINE, s.state)
        assertNull(s.actionStartedAt)
        assertNull(s.notice)
    }

    @Test
    fun `reveil sans reponse signale`() {
        tracker.onWakeSent()
        tick(61_000)
        val s = tracker.onProbe(down)
        assertEquals(PowerState.OFFLINE, s.state)
        assertEquals(StatusNotice.WAKE_TIMEOUT, s.notice)
        assertNull(tracker.clearNotice().notice)
    }

    @Test
    fun `extinction confirmee puis delai depasse`() {
        tracker.onProbe(up)
        tracker.onShutdownSent()
        tick()
        assertEquals(PowerState.SHUTTING_DOWN, tracker.onProbe(up).state)
        tick()
        assertEquals(PowerState.SHUTTING_DOWN, tracker.onProbe(down).state)
        tick()
        assertEquals(PowerState.OFFLINE, tracker.onProbe(down).state)

        tracker.onProbe(up)
        tracker.onShutdownSent()
        tick(31_000)
        val s = tracker.onProbe(up)
        assertEquals(PowerState.ONLINE, s.state)
        assertEquals(StatusNotice.SHUTDOWN_TIMEOUT, s.notice)
    }

    @Test
    fun `redemarrage attend la disparition puis le retour`() {
        tracker.onProbe(up)
        tracker.onRestartSent()
        tick()
        assertEquals(PowerState.RESTARTING, tracker.onProbe(up).state, "pas encore éteint")
        tick()
        assertEquals(PowerState.RESTARTING, tracker.onProbe(down).state)
        tick()
        assertEquals(PowerState.RESTARTING, tracker.onProbe(down).state)
        tick()
        assertEquals(PowerState.ONLINE, tracker.onProbe(up).state)
    }

    @Test
    fun `inconnu plutot que faux eteint sans reseau`() {
        tracker.onProbe(up)
        val s = tracker.onUnavailable(UnknownReason.NO_NETWORK)
        assertEquals(PowerState.UNKNOWN, s.state)
        assertEquals(UnknownReason.NO_NETWORK, s.unknownReason)
        assertTrue(s.lastSeen != null)
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
class StatusMonitorTest {
    private val device = Device(id = "pc", name = "PC", mac = MacAddress.parse("AA:BB:CC:DD:EE:01"), host = "10.0.0.2")

    @Test
    fun `sondage periodique, rafraichissement et reseau`() = runTest {
        var reachable = false
        var probes = 0
        val monitor = StatusMonitor(
            prober = { probes++; ProbeResult(reachable, 1, ProbeMethod.TCP).takeIf { reachable } ?: ProbeResult.UNREACHABLE },
            clock = { testScheduler.currentTime },
        )
        val config = MutableStateFlow(AppConfig(devices = listOf(device)))
        val network = MutableStateFlow(ProbeAvailability.AVAILABLE)
        val job = launch { monitor.run(config, network) }
        runCurrent()
        assertEquals(PowerState.OFFLINE, monitor.statuses.value["pc"]?.state)
        assertEquals(1, probes)

        advanceTimeBy(3_001)
        assertEquals(2, probes, "une sonde toutes les 3 s par défaut")

        reachable = true
        monitor.onWakeSent("pc")
        runCurrent()
        assertEquals(PowerState.ONLINE, monitor.statuses.value["pc"]?.state, "rafraîchissement immédiat après action")

        network.value = ProbeAvailability(false, UnknownReason.NO_NETWORK)
        runCurrent()
        assertEquals(PowerState.UNKNOWN, monitor.statuses.value["pc"]?.state)

        network.value = ProbeAvailability.AVAILABLE
        config.value = AppConfig(devices = emptyList())
        runCurrent()
        assertTrue(monitor.statuses.value.isEmpty(), "terminal supprimé")
        job.cancel()
    }

    @Test
    fun `transition rapide pendant un reveil`() = runTest {
        var probes = 0
        val monitor = StatusMonitor(prober = { probes++; ProbeResult.UNREACHABLE }, clock = { testScheduler.currentTime })
        val job = launch { monitor.run(MutableStateFlow(AppConfig(devices = listOf(device))), MutableStateFlow(ProbeAvailability.AVAILABLE)) }
        testScheduler.runCurrent()
        monitor.onWakeSent("pc")
        testScheduler.runCurrent()
        val before = probes
        testScheduler.advanceTimeBy(3_001)
        assertTrue(probes - before >= 3, "une sonde par seconde pendant le réveil ($probes)")
        job.cancel()
    }
}

class HostProberTest {
    private fun device(ports: List<Int>, host: String = "127.0.0.1", agent: AgentSettings? = null) =
        Device(id = "x", name = "x", mac = MacAddress.parse("AA:BB:CC:DD:EE:01"), host = host, probePorts = ports, agent = agent)

    private val noPing: suspend (InetAddress, Int) -> Boolean = { _, _ -> false }

    @Test
    fun `port ouvert ou refuse = allume`() = runBlocking {
        ServerSocket(0, 5, InetAddress.getLoopbackAddress()).use { open ->
            val result = HostProber(ping = noPing).probe(device(listOf(open.localPort)))
            assertTrue(result.reachable)
            assertEquals(ProbeMethod.TCP, result.method)
        }
        val closed = ServerSocket(0).use { it.localPort }
        assertTrue(HostProber(ping = noPing).probe(device(listOf(closed))).reachable, "connexion refusée = machine allumée")
    }

    @Test
    fun `machine muette = eteinte, dans le delai`() = runBlocking {
        // Simule une machine éteinte : aucune réponse, ni TCP ni ping.
        val silent: suspend (InetAddress, Int, Int) -> Boolean = { _, _, _ -> delay(60_000); false }
        val slowPing: suspend (InetAddress, Int) -> Boolean = { _, _ -> delay(60_000); false }
        val start = TimeSource.Monotonic.markNow()
        val result = HostProber(timeoutMs = 400, ping = slowPing, tcp = silent).probe(device(listOf(9, 22)))
        assertFalse(result.reachable)
        assertTrue(start.elapsedNow().inWholeMilliseconds < 2_000, "respect du délai")
    }

    @Test
    fun `agent prioritaire avec informations`() = runBlocking {
        val key = AgentKey.generate()
        FakeAgentServer(AgentKey.decode(key)).use { server ->
            val result = HostProber(ping = noPing).probe(device(listOf(server.port), agent = AgentSettings(server.port, key)))
            assertTrue(result.reachable)
            assertEquals(ProbeMethod.AGENT, result.method)
            assertEquals("PC-TEST", result.agent?.hostname)
        }
    }

    @Test
    fun `ping seul suffit`() = runBlocking {
        val result = HostProber(timeoutMs = 400, ping = { _, _ -> true }).probe(device(emptyList()))
        assertTrue(result.reachable)
        assertEquals(ProbeMethod.PING, result.method)
    }

    @Test
    fun `sans adresse`() = runBlocking {
        assertFalse(HostProber(ping = noPing).probe(device(listOf(1), host = "")).reachable)
    }
}
