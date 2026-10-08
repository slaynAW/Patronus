package io.github.slaynaw.wakeonlan.core.status

import io.github.slaynaw.wakeonlan.core.agent.AgentStatus

/** Une mesure de latence ; [latencyMs] est nul si le terminal n'a pas répondu à cette sonde. */
data class LatencySample(val time: Long, val latencyMs: Long?)

/** Synthèse des mesures d'une période : [lost] sondes restées sans réponse sur [count]. */
data class LatencyStats(val min: Long, val average: Long, val max: Long, val lost: Int, val count: Int)

/**
 * Relevés de latence récents de chaque terminal, pour le tracé en direct (mêmes règles que
 * l'application Windows) : conservés [WINDOW_MS] en mémoire seulement.
 */
object LatencyLog {
    /** Durée des relevés conservés. */
    const val WINDOW_MS = 5 * 60_000L

    /** Nombre maximal de relevés conservés par terminal. */
    const val MAX_SAMPLES = 400

    /** Intervalle de vérification du terminal affiché en détail (tracé en direct). */
    const val LIVE_INTERVAL_MS = 1_000L

    /** Échelles du tracé (ms) : la plus petite qui contient les mesures affichées. */
    private val SCALES = longArrayOf(5, 10, 20, 50, 100, 200, 500, 1_000, 2_000, 5_000)

    /** Ajoute une mesure en oubliant celles de plus de [WINDOW_MS]. */
    fun append(samples: List<LatencySample>, sample: LatencySample): List<LatencySample> {
        val from = sample.time - WINDOW_MS
        val kept = samples.filter { it.time in from..sample.time }
        return (kept + sample).takeLast(MAX_SAMPLES)
    }

    /** Synthèse des mesures depuis [from] (ms), ou `null` s'il n'y en a aucune. */
    fun stats(samples: List<LatencySample>, from: Long): LatencyStats? {
        val period = samples.filter { it.time >= from }
        if (period.isEmpty()) return null
        val values = period.mapNotNull { it.latencyMs }
        return LatencyStats(
            min = values.minOrNull() ?: 0,
            average = if (values.isEmpty()) 0 else (values.sum() + values.size / 2) / values.size,
            max = values.maxOrNull() ?: 0,
            lost = period.size - values.size,
            count = period.size,
        )
    }

    /** Haut de l'échelle du tracé : un peu de marge au-dessus de [max], arrondi à une valeur ronde. */
    fun scaleMax(max: Long): Long {
        val target = max + (max + 5) / 6
        return SCALES.firstOrNull { it >= target } ?: SCALES.last()
    }
}

/**
 * Relevé de températures (°C) d'un terminal ; une valeur absente n'a pas été lue. [gpuShared] : la
 * carte graphique est intégrée au processeur, sans sonde à part, et [gpu] est la température de la
 * puce (celle du processeur).
 */
data class TempSample(val time: Long, val cpu: Double?, val gpu: Double?, val gpuShared: Boolean = false)

/** Moyenne et maximum d'une série de températures. */
data class TempStats(val average: Double, val max: Double)

/**
 * Relevés de températures récents de chaque terminal (agent 1.5.0 ou plus), pour le grand tracé du
 * détail (mêmes règles que l'application Windows) : conservés [WINDOW_MS] en mémoire seulement.
 */
object TempLog {
    /** Durée des relevés conservés (et tracés). */
    const val WINDOW_MS = 5 * 60_000L

    /**
     * Un relevé identique au précédent n'est gardé qu'après ce délai : l'agent renouvelle ses mesures
     * toutes les 5 à 10 s alors que le terminal affiché est sondé chaque seconde.
     */
    const val REPEAT_MS = 10_000L

    /** Nombre maximal de relevés conservés par terminal. */
    const val MAX_SAMPLES = 400

    /** Ajoute un relevé en oubliant ceux de plus de [WINDOW_MS] (un relevé répété est ignoré). */
    fun append(samples: List<TempSample>, sample: TempSample): List<TempSample> {
        val last = samples.lastOrNull()
        if (last != null && last.cpu == sample.cpu && last.gpu == sample.gpu && last.gpuShared == sample.gpuShared &&
            sample.time >= last.time && sample.time - last.time < REPEAT_MS
        ) {
            return samples
        }
        val from = sample.time - WINDOW_MS
        val kept = samples.filter { it.time in from..sample.time }
        return (kept + sample).takeLast(MAX_SAMPLES)
    }

    /**
     * Relevé tiré d'une réponse de l'agent (`null` sans aucune température). La puce graphique
     * intégrée sans sonde à part ([AgentTemperatures.gpuShared]) a la température du processeur :
     * elle est gardée, marquée (tracée en pointillés).
     */
    fun sampleOf(time: Long, agent: AgentStatus?): TempSample? {
        val t = agent?.temperatures ?: return null
        if (t.cpu == null && t.gpu == null) return null
        return TempSample(time, t.cpu, t.gpu, t.gpuShared && t.gpu != null)
    }

    /** Moyenne et maximum d'une série depuis [from] (ms), ou `null` sans relevé. */
    fun stats(samples: List<TempSample>, from: Long, value: (TempSample) -> Double?): TempStats? {
        val values = samples.filter { it.time >= from }.mapNotNull(value)
        if (values.isEmpty()) return null
        return TempStats(values.average(), values.max())
    }

    /** Bornes de l'échelle (°C) : dizaines rondes autour des relevés, 20 °C d'écart au moins. */
    fun range(min: Double, max: Double): Pair<Double, Double> {
        val lo = maxOf(0.0, kotlin.math.floor((min - 5) / 10) * 10)
        return lo to maxOf(lo + 20, kotlin.math.ceil((max + 5) / 10) * 10)
    }
}
