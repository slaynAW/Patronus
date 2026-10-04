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
 *   même quand l'application était fermée. Il fait foi sur la période qu'il couvre. Depuis l'agent
 *   1.4.0, il contient aussi les demandes de tous les appareils (démarrages signalés par chaque
 *   application, commandes reçues) avec le nom de l'appareil : chacun voit le même historique.
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

    /** Demande faite depuis une application (notée aussi par l'agent, voir [HistoryData.view]). */
    val isRequest: Boolean get() = isPowerRequest || this == WAKE_SENT
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
    /** Demande notée par l'agent : nom de l'appareil qui l'a faite (à défaut, son adresse). */
    @SerialName("c") val client: String? = null,
    /** Arrêt anormal : cause lue par l'agent dans le journal d'événements de Windows (« bsod »…). */
    @SerialName("r") val cause: String? = null,
    /** Précision de la cause (code de l'écran bleu). */
    @SerialName("rd") val detail: String? = null,
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

    /**
     * Ajoute un historique importé (sauvegarde d'un autre appareil) : les évènements déjà présents ne
     * sont pas dupliqués ; la période couverte par le journal de chaque agent est étendue.
     */
    fun merge(other: HistoryData, now: Long): HistoryData {
        val known = events.toHashSet()
        val merged = coverage.toMutableMap()
        for ((device, c) in other.coverage) {
            val mine = merged[device]
            merged[device] = if (mine == null) c else HistoryCoverage(minOf(mine.from, c.from), maxOf(mine.until, c.until))
        }
        return copy(events = events + other.events.filter { known.add(it) }, coverage = merged).pruned(now)
    }

    /**
     * Historique joint à une sauvegarde complète : les [MAX_BACKUP_EVENTS] évènements les plus récents,
     * pour que le fichier reste sous la taille acceptée à l'import (1 Mo), même par les anciennes versions.
     */
    fun forBackup(now: Long): HistoryData {
        val recent = pruned(now)
        return if (recent.events.size <= MAX_BACKUP_EVENTS) recent else recent.copy(events = recent.events.sortedBy { it.time }.takeLast(MAX_BACKUP_EVENTS))
    }

    /**
     * Démarrages demandés depuis cette application que le journal de l'agent d'un PC ne contient pas
     * encore (heures en secondes), à lui signaler (agent 1.4.0 ou plus). Ceux qui précèdent le début
     * du journal (à [WAKE_LEAD_MS] près) sont ignorés : l'agent les refuserait.
     */
    fun unreportedWakes(device: String, journal: AgentHistory): List<Long> {
        val known = journal.events.filter { it.k == "wake" }.map { it.t * 1000 }
        val from = journal.from * 1000 - WAKE_LEAD_MS
        return events
            .filter { it.device == device && it.source == HistorySource.APP && it.kind == HistoryKind.WAKE_SENT && it.time >= from }
            .filter { e -> known.none { abs(it - e.time) <= MATCH_WINDOW_MS } }
            .map { it.time / 1000 }
            .distinct()
            .takeLast(MAX_REPORTED_WAKES)
    }

    /** Ne conserve que les PC encore configurés. */
    fun keep(ids: Set<String>): HistoryData =
        copy(events = events.filter { it.device in ids }, coverage = coverage.filterKeys { it in ids })

    /**
     * Évènements à afficher ([device], ou tous les PC si `null`), du plus récent au plus ancien :
     * - le journal de l'agent fait foi sur la période qu'il couvre : les changements d'état constatés
     *   par l'application pendant cette période sont masqués ;
     * - une demande notée par l'agent qui correspond à une demande faite depuis cette application
     *   n'est affichée qu'une fois (celle de l'application) ; celles des autres appareils restent.
     */
    fun view(device: String?, now: Long): List<HistoryEvent> {
        val limit = now - RETENTION_MS
        val ownRequests = events
            .filter { it.source == HistorySource.APP && it.kind.isRequest }
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
            if (e.source == HistorySource.AGENT && e.kind.isRequest &&
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

        /** Un démarrage demandé est accepté par l'agent jusqu'à cet écart avant le début de son journal. */
        const val WAKE_LEAD_MS = 10 * 60_000L

        /** Démarrages signalés à l'agent en une fois (protocole : 50 au plus). */
        const val MAX_REPORTED_WAKES = 50

        /** Évènements joints à une sauvegarde complète (environ 300 Ko). */
        const val MAX_BACKUP_EVENTS = 5_000

        val EMPTY = HistoryData(version = VERSION)

        private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }

        fun encode(data: HistoryData): String = json.encodeToString(serializer(), data)

        /** Relit un historique enregistré ; `null` s'il est illisible. */
        fun decode(text: String): HistoryData? = runCatching { json.decodeFromString(serializer(), text) }.getOrNull()

        /** Convertit un évènement du journal de l'agent (`null` s'il est inconnu de cette version). */
        /** Causes d'arrêt anormal connues ; longueur maximale de leur précision. */
        private val LOST_CAUSES = setOf(
            AgentHistoryEvent.LOST_BSOD, AgentHistoryEvent.LOST_BUTTON, AgentHistoryEvent.LOST_POWER, AgentHistoryEvent.LOST_HARDWARE,
        )
        const val MAX_DETAIL = 128

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
                "wake" -> HistoryKind.WAKE_SENT
                else -> return null
            }
            val cause = raw.r?.takeIf { kind == HistoryKind.LOST && it in LOST_CAUSES }
            return HistoryEvent(
                device = device,
                time = raw.t * 1000,
                kind = kind,
                source = HistorySource.AGENT,
                client = if (kind.isRequest) raw.b?.takeIf { it.isNotBlank() } ?: raw.c else null,
                cause = cause,
                detail = raw.d?.takeIf { cause != null && it.isNotBlank() && it.length <= MAX_DETAIL },
            )
        }
    }
}
