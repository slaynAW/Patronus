package io.github.slaynaw.wakeonlan.core.status

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
