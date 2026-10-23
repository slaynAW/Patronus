package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentMeasure
import io.github.slaynaw.wakeonlan.core.agent.AgentMeasures
import io.github.slaynaw.wakeonlan.core.agent.MeasureLevel
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

/** Vecteurs communs avec l'agent et l'application Windows : protocol/measure-vectors.json. */
class AgentMeasuresTest {
    private fun vectors(): JsonObject {
        val dir = System.getProperty("wol.protocolDir") ?: "../protocol"
        return Json.parseToJsonElement(File(dir, "measure-vectors.json").readText()).jsonObject
    }

    private fun number(e: kotlinx.serialization.json.JsonElement?) = if (e == null || e is JsonNull) null else e.jsonPrimitive.doubleOrNull

    @Test
    fun `vecteurs communs`() {
        val cases = vectors()["cases"]!!.jsonArray
        assertTrue(cases.size >= 5)
        for (c in cases) {
            val case = c.jsonObject
            val name = case["name"]!!.jsonPrimitive.content
            val got = AgentMeasures.parse(case["input"])
            val expected = case["expected"]!!.jsonArray.map { it.jsonObject }
            assertEquals(expected.size, got.size, "$name : $got")
            expected.zip(got).forEachIndexed { i, (e, g) ->
                val want = AgentMeasure(
                    id = e["id"]!!.jsonPrimitive.content,
                    label = e["label"]!!.jsonPrimitive.content,
                    value = e["value"]!!.jsonPrimitive.doubleOrNull!!,
                    unit = e["unit"]!!.jsonPrimitive.content,
                    min = number(e["min"]),
                    max = number(e["max"]),
                    warn = number(e["warn"]),
                    crit = number(e["crit"]),
                    low = e["low"]!!.jsonPrimitive.boolean,
                    text = e["text"]!!.jsonPrimitive.content,
                )
                assertEquals(want, g, "$name, mesure $i")
                val level = when (e["level"]!!.jsonPrimitive.content) {
                    "warn" -> MeasureLevel.WARN
                    "crit" -> MeasureLevel.CRIT
                    else -> MeasureLevel.NONE
                }
                assertEquals(level, g.level, "$name, gravité de la mesure $i")
            }
        }
    }

    @Test
    fun `champ absent ou mal formé`() {
        assertEquals(emptyList<AgentMeasure>(), AgentMeasures.parse(null))
        assertEquals(emptyList<AgentMeasure>(), AgentMeasures.parse(JsonPrimitive("mem")))
        assertEquals(emptyList<AgentMeasure>(), AgentMeasures.parse(JsonObject(emptyMap())))
        assertEquals(emptyList<AgentMeasure>(), AgentMeasures.parse(JsonArray(emptyList())))
    }
}
