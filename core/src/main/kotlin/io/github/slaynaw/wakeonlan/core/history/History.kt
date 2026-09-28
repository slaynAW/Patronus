package io.github.slaynaw.wakeonlan.core.history

import io.github.slaynaw.wakeonlan.core.agent.AgentHistory
import io.github.slaynaw.wakeonlan.core.agent.AgentHistoryEvent
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlin.math.abs

/*
 * Historique des démarrages et extinctions des PC sur 30 jours. Mêmes règles que l'application
 * Windows (desktop/internal/history), vérifiées par le même scénario : protocol/history-vectors.json.
 *
 * Deux sources sont fusionnées :
 * - l'application : ses propres demandes (démarrage, extinction...) et les changements d'état
 *   constatés pendant la surveillance (heure approximative) ;
 * - le journal de l'agent (commande « history ») : démarrages, arrêts, veille, à l'heure exacte,
 *   même quand l'application était fermée. Il fait foi sur la période qu'il couvre.
 */

/** Type d'un évènement (noms identiques côté Windows). */
@Serializable
enum class HistoryKind {
    /** Allumé. */
    @SerialName("on")
    ON,

    /** Éteint. */
    @SerialName("off")
    OFF,

    /** Arrêt non enregistré par le PC : coupure de courant, arrêt forcé ou plantage. */
    @SerialName("lost")
    LOST,

    /** Mis en veille. */
    @SerialName("sleep")
    SLEEP,

    /** Sorti de veille. */
    @SerialName("resume")
    RESUME,

    /** Démarrage demandé (paquet magique). */
    @SerialName("wake")
    WAKE_SENT,

    /** Extinction demandée. */
    @SerialName("shutdown_req")
    SHUTDOWN_SENT,

    /** Redémarrage demandé. */
    @SerialName("reboot_req")
    REBOOT_SENT,

    /** Mise en veille demandée. */
    @SerialName("sleep_req")
    SLEEP_SENT,

    /** Pas de réponse après le démarrage demandé. */
    @SerialName("wake_timeout")
    WAKE_TIMEOUT,
    ;

    /** Commande d'alimentation transmise à l'agent. */
    val isPowerRequest: Boolean get() = this == SHUTDOWN_SENT || this == REBOOT_SENT || this == SLEEP_SENT
}

/** Origine d'un évènement. */
@Serializable
enum class HistorySource {
    @SerialName("app")
    APP,

    @SerialName("agent")
    AGENT,
}

/** Évènement de l'historique ; [time] en millisecondes (heure Unix). */
@Serializable
data class HistoryEvent(
    @SerialName("d") val device: String,
    @SerialName("t") val time: Long,
    @SerialName("k") val kind: HistoryKind,
    @SerialName("s") val source: HistorySource,
    /** Heure constatée par l'application (à quelques secondes près), pas mesurée par le PC. */
    @SerialName("x") val approx: Boolean = false,
    /** Commande reçue par l'agent : adresse de l'appareil qui l'a envoyée. */
    @SerialName("c") val client: String? = null,
)

/** Période couverte par le journal de l'agent d'un PC (millisecondes). */
@Serializable
data class HistoryCoverage(val from: Long, val until: Long)

/** Historique enregistré (sans limite de PC ; 30 jours, [MAX_EVENTS] évènements au plus). */
@Serializable
data class HistoryData(
    val version: Int,
    val events: List<HistoryEvent> = emptyList(),
    val coverage: Map<String, HistoryCoverage> = emptyMap(),
) {
    /** Ajoute un évènement. */
    fun add(event: HistoryEvent, now: Long): HistoryData = copy(events = events + event).pruned(now)

    /** Remplace le journal de l'agent d'un PC par sa dernière version lue. */
    fun replaceAgent(device: String, journal: AgentHistory, fetchedAt: Long, now: Long): HistoryData {
        val kept = events.filterNot { it.device == device && it.source == HistorySource.AGENT }
        return copy(
            events = kept + journal.events.mapNotNull { fromAgent(device, it) },
            coverage = coverage + (device to HistoryCoverage(journal.from * 1000, fetchedAt)),
        ).pruned(now)
    }

    /** Ne conserve que les PC encore configurés. */
    fun keep(ids: Set<String>): HistoryData =
        copy(events = events.filter { it.device in ids }, coverage = coverage.filterKeys { it in ids })

    /**
     * Évènements à afficher ([device], ou tous les PC si `null`), du plus récent au plus ancien :
     * - le journal de l'agent fait foi sur la période qu'il couvre : les changements d'état constatés
     *   par l'application pendant cette période sont masqués ;
     * - une commande reçue par l'agent qui correspond à une demande faite depuis cette application
     *   n'est affichée qu'une fois (la demande).
     */
    fun view(device: String?, now: Long): List<HistoryEvent> {
        val limit = now - RETENTION_MS
        val ownRequests = events
            .filter { it.source == HistorySource.APP && it.kind.isPowerRequest }
            .groupBy({ it.device to it.kind }, { it.time })
        val out = ArrayList<HistoryEvent>()
        // Parcours du plus récent ajouté au plus ancien : à heure égale, le dernier enregistré passe devant.
        for (i in events.indices.reversed()) {
            val e = events[i]
            if ((device != null && e.device != device) || e.time < limit) continue
            if (e.source == HistorySource.APP && (e.kind == HistoryKind.ON || e.kind == HistoryKind.OFF)) {
                val c = coverage[e.device]
                if (c != null && e.time >= c.from && e.time <= c.until) continue
            }
            if (e.source == HistorySource.AGENT && e.kind.isPowerRequest &&
                ownRequests[e.device to e.kind].orEmpty().any { abs(it - e.time) <= MATCH_WINDOW_MS }
            ) {
                continue
            }
            out += e
        }
        out.sortByDescending { it.time } // tri stable
        return out
    }

    /** Retire les évènements de plus de 30 jours et borne la taille (les plus anciens partent). */
    private fun pruned(now: Long): HistoryData {
        val limit = now - RETENTION_MS
        var kept = events.filter { it.time >= limit }
        if (kept.size > MAX_EVENTS) kept = kept.sortedBy { it.time }.takeLast(MAX_EVENTS)
        return copy(version = VERSION, events = kept)
    }

    companion object {
        const val VERSION = 1

        /** Durée de conservation. */
        const val RETENTION_MS = 30L * 24 * 3600 * 1000

        /** Taille maximale de l'historique enregistré. */
        const val MAX_EVENTS = 20_000

        /** Une commande vue par l'agent à moins de cet écart d'une demande de l'application est la même. */
        const val MATCH_WINDOW_MS = 60_000L

        val EMPTY = HistoryData(version = VERSION)

        private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }

        fun encode(data: HistoryData): String = json.encodeToString(serializer(), data)

        /** Relit un historique enregistré ; `null` s'il est illisible. */
        fun decode(text: String): HistoryData? = runCatching { json.decodeFromString(serializer(), text) }.getOrNull()

        /** Convertit un évènement du journal de l'agent (`null` s'il est inconnu de cette version). */
        fun fromAgent(device: String, raw: AgentHistoryEvent): HistoryEvent? {
            val kind = when (raw.k) {
                "boot" -> HistoryKind.ON
                "shutdown" -> HistoryKind.OFF
                "lost" -> HistoryKind.LOST
                "sleep" -> HistoryKind.SLEEP
                "resume" -> HistoryKind.RESUME
                "cmd" -> when (raw.a) {
                    "shutdown" -> HistoryKind.SHUTDOWN_SENT
                    "reboot" -> HistoryKind.REBOOT_SENT
                    "sleep" -> HistoryKind.SLEEP_SENT
                    else -> return null
                }
                else -> return null
            }
            return HistoryEvent(
                device = device,
                time = raw.t * 1000,
                kind = kind,
                source = HistorySource.AGENT,
                client = if (raw.k == "cmd") raw.c else null,
            )
        }
    }
}
