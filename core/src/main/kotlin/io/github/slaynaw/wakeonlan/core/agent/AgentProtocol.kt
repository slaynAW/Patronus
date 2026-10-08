package io.github.slaynaw.wakeonlan.core.agent

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.security.MessageDigest
import java.util.Base64
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/**
 * Protocole « wolagent/1 » entre l'application et l'agent installé sur le PC.
 * Spécification complète : docs/PROTOCOLE.md. Vecteurs de test partagés : protocol/test-vectors.json.
 *
 * Résumé (TCP, une ligne JSON par message) :
 * 1. agent → app : `{"proto":"wolagent/1","nonce":"<32 octets aléatoires>"}`
 * 2. app → agent : `{"cnonce":"<16 octets>","body":"<JSON>","mac":"<HMAC>"}`
 * 3. agent → app : `{"body":"<JSON>","mac":"<HMAC>"}` ou `{"error":"unauthorized"}`
 *
 * Chaque message est authentifié par HMAC-SHA256 avec la clé partagée, et lié aux deux nonces :
 * un message intercepté ne peut ni être rejoué ni être modifié. La clé ne transite jamais.
 */
object AgentProtocol {
    const val PROTO = "wolagent/1"
    const val SERVER_NONCE_BYTES = 32
    const val CLIENT_NONCE_BYTES = 16
    const val MAX_LINE_BYTES = 8 * 1024

    /** Taille maximale de la réponse à la commande `history` (journal de 30 jours). */
    const val MAX_HISTORY_BYTES = 512 * 1024

    /** Taille maximale de la réponse à la commande `metrics` (une journée de mesures). */
    const val MAX_METRICS_BYTES = 512 * 1024

    /** Taille maximale de la réponse à la commande `specs` (fiche du PC, quelques Ko en pratique). */
    const val MAX_SPECS_BYTES = 64 * 1024

    /** Démarrages signalés en une fois (commande « wakes »). */
    const val MAX_WAKES = 50

    /** Longueur maximale du nom de l'appareil noté par l'agent. */
    const val MAX_BY_LENGTH = 40

    private val b64 = Base64.getUrlEncoder().withoutPadding()
    private val b64Decoder = Base64.getUrlDecoder()

    fun requestMac(key: ByteArray, nonce: String, cnonce: String, body: String): String =
        hmac(key, "$PROTO\nrequest\n$nonce\n$cnonce\n$body")

    fun responseMac(key: ByteArray, nonce: String, cnonce: String, body: String): String =
        hmac(key, "$PROTO\nresponse\n$nonce\n$cnonce\n$body")

    /** Comparaison en temps constant (évite les attaques par mesure du temps de réponse). */
    fun macEquals(expected: String, received: String): Boolean {
        val a = runCatching { b64Decoder.decode(expected) }.getOrNull() ?: return false
        val b = runCatching { b64Decoder.decode(received) }.getOrNull() ?: return false
        return MessageDigest.isEqual(a, b)
    }

    fun encodeNonce(bytes: ByteArray): String = b64.encodeToString(bytes)

    fun decodedLength(value: String): Int = runCatching { b64Decoder.decode(value).size }.getOrDefault(-1)

    private fun hmac(key: ByteArray, message: String): String {
        val mac = Mac.getInstance("HmacSHA256")
        mac.init(SecretKeySpec(key, "HmacSHA256"))
        return b64.encodeToString(mac.doFinal(message.toByteArray(Charsets.UTF_8)))
    }
}

/** Actions d'alimentation que l'agent sait exécuter. */
enum class PowerAction(val wire: String) {
    SHUTDOWN("shutdown"),
    REBOOT("reboot"),
    SLEEP("sleep"),
}

/** Informations renvoyées par l'agent (commande `status`). */
@Serializable
data class AgentStatus(
    val hostname: String = "",
    val os: String = "",
    val arch: String = "",
    val version: String = "",
    @SerialName("uptime") val uptimeSeconds: Long = 0,
    /** Températures du PC (agent 1.5.0 ou plus ; `null` : aucun capteur lisible). */
    val temperatures: AgentTemperatures? = null,
    /** Espace des lecteurs et santé des disques (agent 1.9.0 ou plus). */
    val disks: AgentDisks? = null,
)

/** Gravité d'un problème de disque. */
enum class DiskLevel { NONE, WARN, BAD }

/**
 * Disques du PC (agent 1.9.0) : [volumes] lecteurs et leur espace, [drives] disques physiques et leur
 * santé, [errors] erreurs d'accès signalées par Windows sur [ERROR_DAYS] jours, la dernière à [lastError] (s).
 */
@Serializable
data class AgentDisks(
    val volumes: List<AgentVolume> = emptyList(),
    val drives: List<AgentDrive> = emptyList(),
    val errors: Int = 0,
    val lastError: Long = 0,
) {
    val isEmpty: Boolean get() = volumes.isEmpty() && drives.isEmpty() && errors == 0

    /** Alerte la plus grave (liste des PC), ou null : mêmes règles que l'application Windows. */
    fun alert(): DiskAlert? {
        if (drives.any { it.level == DiskLevel.BAD }) return DiskAlert(DiskAlert.Kind.DRIVE_BAD, DiskLevel.BAD)
        val full = volumes.filter { it.level != DiskLevel.NONE }.maxByOrNull { it.usedPercent }
        if (full != null) return DiskAlert(DiskAlert.Kind.FULL, full.level, full)
        if (drives.any { it.level == DiskLevel.WARN }) return DiskAlert(DiskAlert.Kind.DRIVE_WATCH, DiskLevel.WARN)
        if (errors > 0) return DiskAlert(DiskAlert.Kind.ERRORS, DiskLevel.WARN)
        return null
    }

    companion object {
        const val ERROR_DAYS = 30
    }
}

/** Alerte disque : [volume] pour un lecteur presque plein. */
data class DiskAlert(val kind: Kind, val level: DiskLevel, val volume: AgentVolume? = null) {
    enum class Kind { DRIVE_BAD, FULL, DRIVE_WATCH, ERRORS }
}

/** Lecteur (« C: ») et son espace en octets. */
@Serializable
data class AgentVolume(val mount: String, val label: String = "", val fs: String = "", val total: Long = 0, val free: Long = 0) {
    val usedPercent: Double get() = if (total > 0) 100.0 * (total - free).coerceAtLeast(0) / total else 0.0

    /** Plein à 95 % ou plus : grave ; 90 % : à surveiller. */
    val level: DiskLevel
        get() = when {
            usedPercent >= 95 -> DiskLevel.BAD
            usedPercent >= 90 -> DiskLevel.WARN
            else -> DiskLevel.NONE
        }
}

/**
 * Disque physique : [media] « ssd » ou « hdd », [bus] « NVMe », « SATA »…, [health] état donné par le
 * système (« ok », « warning », « bad » ; vide : inconnu), températures (°C), [wear] usure (%),
 * [hours] heures de fonctionnement, erreurs de lecture et d'écriture non corrigées.
 */
@Serializable
data class AgentDrive(
    val name: String,
    val media: String = "",
    val bus: String = "",
    val size: Long = 0,
    val health: String = "",
    val temp: Double? = null,
    val tempMax: Double? = null,
    val wear: Int? = null,
    val hours: Long? = null,
    val readErrors: Long? = null,
    val writeErrors: Long? = null,
) {
    val uncorrected: Long get() = (readErrors ?: 0) + (writeErrors ?: 0)

    /** Température à partir de laquelle le disque chauffe (plus basse pour un disque dur). */
    val hotTemp: Double get() = if (media == HDD) 55.0 else 70.0

    val level: DiskLevel
        get() = when {
            health == UNHEALTHY || (wear ?: 0) >= 95 -> DiskLevel.BAD
            health == WARNING || uncorrected > 0 || (wear ?: 0) >= 80 || (temp ?: 0.0) >= hotTemp -> DiskLevel.WARN
            else -> DiskLevel.NONE
        }

    companion object {
        const val SSD = "ssd"
        const val HDD = "hdd"
        const val HEALTHY = "ok"
        const val WARNING = "warning"
        const val UNHEALTHY = "bad"
    }
}

/**
 * Températures du PC en °C et utilisation du processeur et de la carte graphique en % ([cpuLoad],
 * [gpuLoad] : mesurées comme le Gestionnaire des tâches, agent 1.7.0) ; un capteur illisible est `null`. [cpuHint] explique l'absence de la
 * température du processeur : [CPU_HINT_LHM] = LibreHardwareMonitor ne la fournit pas (Windows), et
 * [lhm] précise pourquoi (agent 1.6.0 ou plus ; vide avec un agent plus ancien).
 */
@Serializable
data class AgentTemperatures(
    val cpu: Double? = null,
    val gpu: Double? = null,
    val cpuLoad: Double? = null,
    val gpuLoad: Double? = null,
    val gpuName: String = "",
    /** Carte graphique intégrée au processeur, sans sonde à part : [gpu] est la température de la puce (agent 1.6.1). */
    val gpuShared: Boolean = false,
    val cpuHint: String = "",
    val lhm: String = "",
) {
    companion object {
        const val CPU_HINT_LHM = "lhm"

        /** LibreHardwareMonitor ne tourne pas. */
        const val LHM_NOT_RUNNING = "not-running"

        /** Il tourne, mais son serveur web (Options → Remote Web Server → Run) est désactivé. */
        const val LHM_WEB_OFF = "web-off"

        /** Son serveur web demande un mot de passe. */
        const val LHM_AUTH = "auth"

        /** Il répond sans température du processeur (pilote PawnIO absent…). */
        const val LHM_NO_SENSOR = "no-sensor"

        /** Seuils d'affichage : chaud, puis très chaud. */
        const val WARM = 80.0
        const val HOT = 90.0
    }
}

/**
 * Journal du PC renvoyé par la commande `history` : 30 jours au plus, heures en secondes (Unix).
 * [from] : début de la période couverte (installation de l'agent, ou 30 jours).
 */
@Serializable
data class AgentHistory(val from: Long = 0, val events: List<AgentHistoryEvent> = emptyList())

/**
 * Évènement du journal : [t] heure (s), [k] type (`boot`, `shutdown`, `lost`, `sleep`, `resume`,
 * `cmd`, `wake`), [a] action d'une commande reçue, [c] adresse de l'appareil à l'origine d'une
 * demande et [b] son nom (agent 1.4.0 ou plus).
 */
@Serializable
data class AgentHistoryEvent(
    val t: Long,
    val k: String,
    val a: String? = null,
    val c: String? = null,
    val b: String? = null,
    /** Cause d'un arrêt non enregistré (« bsod », « button », « power », « hardware » ; agent 1.9.0). */
    val r: String? = null,
    /** Précision de la cause : code de l'écran bleu (« 0x7E SYSTEM_THREAD_EXCEPTION_NOT_HANDLED »)… */
    val d: String? = null,
) {
    /** Même évènement, cause mise à part (l'agent la précise après coup). */
    fun same(o: AgentHistoryEvent) = t == o.t && k == o.k && a == o.a && c == o.c && b == o.b

    /** Le même évènement que [o], avec une cause plus précise. */
    fun better(o: AgentHistoryEvent) = (o.r.isNullOrEmpty() && !r.isNullOrEmpty()) || (o.d.isNullOrEmpty() && !d.isNullOrEmpty())

    companion object {
        const val LOST_BSOD = "bsod"
        const val LOST_BUTTON = "button"
        const val LOST_POWER = "power"
        const val LOST_HARDWARE = "hardware"

        /** Ajoute [e] à [events], ou remplace le même évènement s'il en dit plus long ; vrai si changé. */
        fun merge(events: MutableList<AgentHistoryEvent>, e: AgentHistoryEvent): Boolean {
            val i = events.indexOfFirst { it.same(e) }
            if (i < 0) {
                events += e
                return true
            }
            if (!e.better(events[i])) return false
            events[i] = e
            return true
        }

        /** Chaque évènement une fois, avec la cause la plus précise. */
        fun merged(events: List<AgentHistoryEvent>): List<AgentHistoryEvent> =
            ArrayList<AgentHistoryEvent>().also { out -> events.forEach { merge(out, it) } }
    }
}

/**
 * Mesures d'un jour renvoyées par la commande `metrics` (agent 1.8.0) : [day] jour demandé (UTC,
 * « 2026-10-03 », vide pour la seule liste des jours), [days] jours enregistrés sur le PC (90 au
 * plus), [rows] une ligne par minute.
 */
@Serializable
data class AgentMetrics(val day: String = "", val days: List<String> = emptyList(), val rows: List<MetricsRow> = emptyList())

/**
 * Mesures d'une minute : [t] début (s), [n] nombre de relevés, puis moyenne et maximum de la
 * température du processeur ([ct], [ctx]) et de la carte graphique ([gt], [gtx]), de l'utilisation du
 * processeur ([cl], [clx]) et de la carte graphique ([gl], [glx]) ; `null` si non lue.
 */
@Serializable
data class MetricsRow(
    val t: Long,
    val n: Int = 0,
    val ct: Double? = null,
    val ctx: Double? = null,
    val gt: Double? = null,
    val gtx: Double? = null,
    val cl: Double? = null,
    val clx: Double? = null,
    val gl: Double? = null,
    val glx: Double? = null,
)

/**
 * Fiche du PC renvoyée par la commande `specs` (agent 1.10.0) : [model] fabricant et modèle d'un PC de
 * marque ou d'un portable, puis processeur, mémoire, cartes graphiques, carte mère et système. Une
 * information illisible est absente ; aucun numéro de série n'y figure.
 */
@Serializable
data class AgentSpecs(
    val model: String = "",
    val cpu: SpecsCpu? = null,
    val memory: SpecsMemory? = null,
    val gpus: List<SpecsGpu> = emptyList(),
    val board: SpecsBoard? = null,
    val os: SpecsOs? = null,
)

/** Processeur : [cores] cœurs et [threads] processeurs logiques, [mhz] fréquence de base, [count] processeurs (> 1). */
@Serializable
data class SpecsCpu(val name: String = "", val cores: Int = 0, val threads: Int = 0, val mhz: Int = 0, val count: Int = 0)

/** Mémoire vive : [total] installée (octets), [slots] emplacements (0 : inconnu), [modules] barrettes. */
@Serializable
data class SpecsMemory(val total: Long = 0, val slots: Int = 0, val modules: List<SpecsModule> = emptyList())

/** Barrette : [slot] emplacement, [size] octets, [type] « DDR4 », [mts] vitesse (MT/s). */
@Serializable
data class SpecsModule(
    val slot: String = "",
    val size: Long = 0,
    val type: String = "",
    val mts: Int = 0,
    val maker: String = "",
    val part: String = "",
) {
    /** « DDR4-3200 », « DDR5 », « 4800 MT/s » ou vide. */
    val kind: String
        get() = when {
            type.isNotEmpty() && mts > 0 -> "$type-$mts"
            type.isNotEmpty() -> type
            mts > 0 -> "$mts MT/s"
            else -> ""
        }
}

/** Carte graphique : [vram] mémoire dédiée (octets, 0 : aucune ou inconnue), [integrated] puce intégrée au processeur. */
@Serializable
data class SpecsGpu(val name: String = "", val vram: Long = 0, val driver: String = "", val integrated: Boolean = false)

/** Carte mère et BIOS ([biosDate] : « 2024-03-12 »). */
@Serializable
data class SpecsBoard(val maker: String = "", val model: String = "", val bios: String = "", val biosDate: String = "")

/** Système d'exploitation : « Windows 11 Pro », « 24H2, build 26100.2314 ». */
@Serializable
data class SpecsOs(val name: String = "", val version: String = "")

@Serializable
internal data class HelloMessage(val proto: String, val nonce: String)

@Serializable
internal data class RequestBody(
    val cmd: String,
    val delay: Int? = null,
    val force: Boolean? = null,
    /** Nom de cet appareil, noté par l'agent avec la demande (ignoré avant l'agent 1.4.0). */
    val by: String? = null,
    /** Heures (s) des démarrages demandés, pour la commande « wakes ». */
    val wakes: List<Long>? = null,
    /** Jour des mesures demandées (UTC), pour la commande « metrics ». */
    val day: String? = null,
)

@Serializable
internal data class RequestMessage(val cnonce: String, val body: String, val mac: String)

@Serializable
internal data class ResponseMessage(
    val body: String? = null,
    val mac: String? = null,
    val error: String? = null,
)

@Serializable
internal data class ResponseBody(
    val ok: Boolean,
    val code: String,
    val message: String = "",
    val hostname: String = "",
    val os: String = "",
    val arch: String = "",
    val version: String = "",
    val uptime: Long = 0,
    val history: AgentHistory? = null,
    val temperatures: AgentTemperatures? = null,
    val metrics: AgentMetrics? = null,
    val disks: AgentDisks? = null,
    val specs: AgentSpecs? = null,
) {
    fun toStatus() = AgentStatus(hostname, os, arch, version, uptime, temperatures, disks)
}
