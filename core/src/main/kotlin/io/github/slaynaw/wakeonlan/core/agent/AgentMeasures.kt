package io.github.slaynaw.wakeonlan.core.agent

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlin.math.abs

/** Gravité d'une mesure, d'après ses seuils. */
enum class MeasureLevel { NONE, WARN, CRIT }

/**
 * Mesure décrite par l'agent lui-même (réponse à `status`, agent 1.13.0) : l'application l'affiche même si elle ne
 * la connaît pas (docs/PROTOCOLE.md, « Mesures »). [min] et [max] : bornes d'une jauge, ensemble ou pas du tout ;
 * [warn] et [crit] : seuils à surveiller et grave, atteints vers le haut, ou vers le bas avec [low].
 */
@Serializable
data class AgentMeasure(
    val id: String,
    val label: String,
    val value: Double,
    val unit: String = "",
    val min: Double? = null,
    val max: Double? = null,
    val warn: Double? = null,
    val crit: Double? = null,
    val low: Boolean = false,
    val text: String = "",
) {
    val level: MeasureLevel
        get() {
            fun reached(t: Double?) = t != null && (if (low) value <= t else value >= t)
            return when {
                reached(crit) -> MeasureLevel.CRIT
                reached(warn) -> MeasureLevel.WARN
                else -> MeasureLevel.NONE
            }
        }
}

/** Lecture des mesures, mêmes règles que l'agent et l'application Windows (vecteurs : protocol/measure-vectors.json). */
object AgentMeasures {
    const val MAX = 8
    const val MAX_LABEL = 24
    const val MAX_UNIT = 8
    const val MAX_TEXT = 32
    private const val MAX_VALUE = 1e12
    private val ID = Regex("^[a-z0-9][a-z0-9._-]{0,23}$")

    /**
     * Lit les mesures élément par élément : une mesure invalide est ignorée sans rendre la réponse illisible, un champ
     * facultatif invalide aussi (sauf l'unité : sans elle, la valeur serait trompeuse). Seule la première mesure d'un
     * identifiant compte ; [MAX] au plus.
     */
    fun parse(raw: JsonElement?): List<AgentMeasure> {
        val items = raw as? JsonArray ?: return emptyList()
        val out = ArrayList<AgentMeasure>()
        val seen = HashSet<String>()
        for (item in items) {
            val m = parseOne(item) ?: continue
            if (!seen.add(m.id)) continue
            out += m
            if (out.size == MAX) break
        }
        return out
    }

    private fun parseOne(item: JsonElement): AgentMeasure? {
        val o = item as? JsonObject ?: return null
        val id = string(o["id"])?.takeIf { ID.matches(it) } ?: return null
        val label = string(o["label"])?.takeIf { validText(it, 1, MAX_LABEL) && !it.isBlank() } ?: return null
        val value = number(o["value"]) ?: return null
        val unitRaw = o["unit"]
        val unit = if (unitRaw == null || unitRaw is JsonNull) {
            ""
        } else {
            string(unitRaw)?.takeIf { validText(it, 0, MAX_UNIT) } ?: return null
        }
        var min = number(o["min"])
        var max = number(o["max"])
        if (min == null || max == null || min >= max) {
            min = null
            max = null
        }
        val low = (o["low"] as? JsonPrimitive)?.takeIf { !it.isString }?.booleanOrNull ?: false
        val text = string(o["text"])?.takeIf { validText(it, 0, MAX_TEXT) }.orEmpty()
        return AgentMeasure(id, label, value, unit, min, max, number(o["warn"]), number(o["crit"]), low, text)
    }

    /** Chaîne JSON (pas un nombre, ni null). */
    private fun string(e: JsonElement?): String? = (e as? JsonPrimitive)?.takeIf { it.isString }?.content

    /** Nombre JSON fini et raisonnable (pas une chaîne, ni null, ni booléen). */
    private fun number(e: JsonElement?): Double? =
        (e as? JsonPrimitive)?.takeIf { !it.isString }?.content?.toDoubleOrNull()?.takeIf { it.isFinite() && abs(it) <= MAX_VALUE }

    /**
     * Entre [min] et [max] caractères, sans caractère de contrôle ou de mise en forme (inversion du sens de lecture…),
     * ni moitié de caractère ou caractère de remplacement (texte mal encodé).
     */
    private fun validText(s: String, min: Int, max: Int): Boolean {
        val n = s.codePointCount(0, s.length)
        if (n < min || n > max) return false
        var i = 0
        while (i < s.length) {
            val cp = s.codePointAt(i)
            when (Character.getType(cp).toByte()) {
                Character.CONTROL, Character.FORMAT, Character.SURROGATE -> return false
            }
            if (cp == 0xFFFD) return false
            i += Character.charCount(cp)
        }
        return true
    }
}
