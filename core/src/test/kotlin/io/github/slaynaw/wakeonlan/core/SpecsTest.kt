package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.agent.AgentError
import io.github.slaynaw.wakeonlan.core.agent.AgentKey
import io.github.slaynaw.wakeonlan.core.agent.AgentResult
import io.github.slaynaw.wakeonlan.core.agent.AgentSpecs
import io.github.slaynaw.wakeonlan.core.agent.AgentStatus
import io.github.slaynaw.wakeonlan.core.agent.AgentTemperatures
import io.github.slaynaw.wakeonlan.core.agent.SpecsCpu
import io.github.slaynaw.wakeonlan.core.agent.SpecsModule
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.status.TempLog
import io.github.slaynaw.wakeonlan.core.status.TempSample
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Fiche du PC (agent 1.10.0) et relevés de températures : mêmes règles que l'application Windows. */
class SpecsTest {
    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }

    @Test
    fun `lecture de la fiche envoyee par l agent`() {
        // Format de l'agent Go (protocol.Specs), champs absents compris.
        val specs = json.decodeFromString(
            AgentSpecs.serializer(),
            """{"model":"Dell XPS 15 9520","cpu":{"name":"Intel Core i7-12700H","cores":14,"threads":20,"mhz":2300},
               "memory":{"total":34359738368,"slots":2,"modules":[{"slot":"DIMM A","size":17179869184,"type":"DDR5","mts":4800,"maker":"SK hynix"},
               {"slot":"DIMM B","size":17179869184}]},
               "gpus":[{"name":"NVIDIA GeForce RTX 3050 Ti Laptop GPU","vram":4294967296,"driver":"32.0.15.6094"},{"name":"Intel Iris Xe Graphics","integrated":true}],
               "board":{"maker":"Dell","model":"0RH1JY","bios":"1.22.0","biosDate":"2024-05-14"},"os":{"name":"Windows 11 Pro","version":"24H2"},"futur":1}""",
        )
        assertEquals(14, specs.cpu?.cores)
        val memory = specs.memory!!
        assertEquals(32L shl 30, memory.total)
        assertEquals("DDR5-4800", memory.modules[0].kind)
        assertEquals("", memory.modules[1].kind)
        assertEquals("4800 MT/s", SpecsModule(mts = 4800).kind)
        assertTrue(specs.gpus[1].integrated)
        assertEquals(0L, specs.gpus[1].vram)
        assertEquals("2024-05-14", specs.board?.biosDate)
        val minimal = json.decodeFromString(AgentSpecs.serializer(), """{"os":{"name":"Linux"}}""")
        assertNull(minimal.cpu)
        assertTrue(minimal.gpus.isEmpty())
    }

    @Test
    fun `commande specs`() = runBlocking {
        val key = AgentKey.generate()
        FakeAgentServer(AgentKey.decodeOrNull(key)!!).use { server ->
            val settings = AgentSettings(port = server.port, key = key)
            // Agent antérieur à 1.10.0 : commande refusée.
            val old = AgentClient().specs("127.0.0.1", settings)
            assertEquals(AgentError.REJECTED, (old as AgentResult.Failure).error)
            // Première lecture en cours : refusée avec le code « busy », à redemander.
            server.specs = AgentSpecs(cpu = SpecsCpu(name = "AMD Ryzen 7 5800X", cores = 8, threads = 16))
            server.specsBusy = true
            val busy = AgentClient().specs("127.0.0.1", settings) as AgentResult.Failure
            assertEquals(AgentError.REJECTED, busy.error)
            assertEquals("busy", busy.code)
            server.specsBusy = false
            val sheet = AgentClient().specs("127.0.0.1", settings)
            assertEquals(8, (sheet as AgentResult.Success).value.cpu?.cores)
        }
    }

    @Test
    fun `releves de temperatures`() {
        var samples = emptyList<TempSample>()
        // Sondé chaque seconde, mesures renouvelées toutes les 5 s : seuls les changements (et une
        // répétition toutes les 10 s) sont gardés.
        for (i in 0 until 30) {
            val cpu = if (i >= 20) 53.0 else 50.0 + i / 5
            samples = TempLog.append(samples, TempSample(i * 1_000L, cpu, 60.0))
        }
        assertEquals(listOf(0L, 5L, 10L, 15L, 25L), samples.map { it.time / 1_000 })
        samples = TempLog.append(samples, TempSample(26_000, 53.0, null))
        assertEquals(6, samples.size)
        samples = TempLog.append(samples, TempSample(26_000 + TempLog.WINDOW_MS, 40.0, null))
        assertEquals(listOf(26_000L, 26_000 + TempLog.WINDOW_MS), samples.map { it.time })
        for (i in 0 until 1_000) samples = TempLog.append(samples, TempSample(400_000L + i, i.toDouble(), null))
        assertEquals(TempLog.MAX_SAMPLES, samples.size)

        // Puce graphique intégrée : sa température (celle de la puce) est gardée, marquée.
        val shared = AgentStatus(temperatures = AgentTemperatures(cpu = 55.0, gpu = 55.0, gpuShared = true))
        val sample = TempLog.sampleOf(7, shared)
        assertEquals(TempSample(7, 55.0, 55.0, gpuShared = true), sample)
        // Passage de la puce intégrée à la carte dédiée (même valeur) : nouveau relevé.
        assertEquals(2, TempLog.append(listOfNotNull(sample), TempSample(8, 55.0, 55.0)).size)
        assertNull(TempLog.sampleOf(7, AgentStatus(temperatures = AgentTemperatures(cpuLoad = 5.0))))
        assertNull(TempLog.sampleOf(7, null))

        val stats = TempLog.stats(listOf(TempSample(0, 50.0, null), TempSample(1, 60.0, 70.0), TempSample(2, null, 72.0)), 0) { it.cpu }!!
        assertEquals(55.0, stats.average, 0.001)
        assertEquals(60.0, stats.max, 0.001)
        assertNull(TempLog.stats(listOf(TempSample(0, 50.0, null)), 1) { it.cpu })
        assertEquals(30.0 to 80.0, TempLog.range(42.0, 71.0))
        assertEquals(40.0 to 60.0, TempLog.range(50.0, 52.0))
        assertEquals(0.0 to 20.0, TempLog.range(2.0, 3.0))
    }
}
