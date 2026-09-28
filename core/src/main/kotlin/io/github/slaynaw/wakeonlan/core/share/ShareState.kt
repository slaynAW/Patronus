package io.github.slaynaw.wakeonlan.core.share

import io.github.slaynaw.wakeonlan.core.config.ConfigCodec
import io.github.slaynaw.wakeonlan.core.model.Device
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.security.MessageDigest
import java.time.Instant

/** Droit accordé sur un PC partagé. */
@Serializable
enum class ShareRight {
    /** Démarrer et voir l'état (la clé de l'agent n'est pas transmise). */
    @SerialName("wake")
    WAKE,

    /** Démarrer, éteindre, redémarrer, mettre en veille. */
    @SerialName("full")
    FULL,
}

/** Personne (appareil) autorisée. */
@Serializable
data class SharePerson(
    val name: String,
    /** Clé publique de réception de son appareil. */
    val device: String,
    val added: Long = 0,
    /** Identifiant de chaque PC partagé → droit accordé. */
    val rights: Map<String, ShareRight> = emptyMap(),
)

/** Lot de fichiers à écrire dans le Gist (null : fichier à supprimer). */
data class SharePublication(val files: Map<String, String?>, val fingerprints: Map<String, String>, val revision: Long) {
    val isEmpty: Boolean get() = files.isEmpty()
}

/** Côté « je partage mes PC ». */
@Serializable
data class ShareOwner(
    /** Clé de signature (privée, puis publique). */
    val key: String,
    val publicKey: String,
    val name: String,
    /** Jeton GitHub (droit « gist ») ; jamais exporté. */
    val token: String = "",
    val user: String = "",
    val gist: String = "",
    val revision: Long = 0,
    val people: List<SharePerson> = emptyList(),
    /** Empreinte du dernier contenu publié pour chaque appareil. */
    val published: Map<String, String> = emptyMap(),
    /** Appareils dont le fichier reste à supprimer du Gist. */
    val withdrawn: List<String> = emptyList(),
) {
    val keyPair: ShareKeyPair get() = ShareCrypto.keyPair(key, publicKey)
    val isConnected: Boolean get() = token.isNotEmpty() && gist.isNotEmpty()

    fun person(device: String): SharePerson? = people.firstOrNull { it.device == device }

    /** Ce que reçoit une personne : ses PC, dans l'ordre, sans clé d'agent pour « démarrer ». */
    fun content(person: SharePerson, devices: List<Device>): ShareContent = ShareContent(
        ownerName = name,
        recipientName = person.name,
        devices = devices.mapNotNull { d ->
            when (person.rights[d.id]) {
                ShareRight.WAKE -> d.copy(agent = null)
                ShareRight.FULL -> d
                null -> null
            }
        },
    )

    /** Prépare la publication des fichiers qui ont changé (ou de tous si [all]). */
    fun prepare(devices: List<Device>, nowMillis: Long, all: Boolean = false): SharePublication {
        val pair = keyPair
        val revision = maxOf(revision + 1, nowMillis)
        val issued = Instant.ofEpochMilli(nowMillis).toString()
        val files = LinkedHashMap<String, String?>()
        val fingerprints = HashMap<String, String>()
        for (p in people) {
            val c = content(p, devices)
            val fp = fingerprint(c)
            fingerprints[p.device] = fp
            if (!all && published[p.device] == fp) continue
            files[ShareCrypto.fileName(p.device)] = ShareCrypto.seal(pair, p.device, revision, issued, c)
        }
        for (device in withdrawn) {
            if (person(device) == null) files[ShareCrypto.fileName(device)] = null
        }
        return SharePublication(files, fingerprints, if (files.isEmpty()) 0 else revision)
    }

    /** Enregistre une publication réussie (l'état a pu changer pendant l'envoi). */
    fun commit(p: SharePublication): ShareOwner = copy(
        revision = maxOf(revision, p.revision),
        published = published + p.fingerprints.filterKeys { person(it) != null },
        withdrawn = withdrawn.filterNot { d -> p.files.containsKey(ShareCrypto.fileName(d)) && p.files[ShareCrypto.fileName(d)] == null },
    )

    /** Autorise (ou met à jour) une personne. */
    fun grant(p: SharePerson): ShareOwner {
        val existing = person(p.device)
        if (existing == null && people.size >= MAX_PEOPLE) throw ShareException(ShareException.Reason.INVALID, "$MAX_PEOPLE personnes au maximum")
        val updated = if (existing != null) people.map { if (it.device == p.device) p.copy(added = existing.added) else it } else people + p
        return copy(people = updated, withdrawn = withdrawn - p.device, published = published - p.device)
    }

    /** Retire une personne ; son fichier sera supprimé à la prochaine publication. */
    fun withdraw(device: String): ShareOwner {
        if (person(device) == null) return this
        return copy(
            people = people.filterNot { it.device == device },
            withdrawn = if (device in withdrawn) withdrawn else withdrawn + device,
            published = published - device,
        )
    }

    override fun toString(): String = "ShareOwner(name=$name, user=$user, gist=$gist, people=${people.size}, key=***, token=***)"

    companion object {
        const val MAX_PEOPLE = 50

        fun fingerprint(c: ShareContent): String = ShareCrypto.hex(
            MessageDigest.getInstance("SHA-256").digest(ConfigCodec.json.encodeToString(ShareContent.serializer(), c).toByteArray()),
        )
    }
}

/**
 * Côté « je partage » dans une sauvegarde complète (« sharing », même format que Windows) : sans le
 * jeton GitHub, à reconnecter sur le nouvel appareil.
 */
@Serializable
data class ExportedShareOwner(
    val key: String,
    val publicKey: String,
    val name: String,
    val user: String = "",
    val gist: String = "",
    val revision: Long = 0,
    val people: List<SharePerson> = emptyList(),
) {
    /** Vérifie la sauvegarde et renvoie l'état correspondant (connexion GitHub à refaire). */
    fun toOwner(): ShareOwner {
        val valid = runCatching { ShareCrypto.keyPair(key, publicKey) }.isSuccess &&
            ShareCrypto.isValidName(name) && people.size <= ShareOwner.MAX_PEOPLE &&
            (user.isEmpty() || ShareLinks.LOGIN.matches(user)) && (gist.isEmpty() || ShareLinks.GIST.matches(gist)) &&
            people.all { ShareCrypto.isValidName(it.name) && ShareCrypto.isValidPublicKey(it.device) }
        if (!valid) throw ShareException(ShareException.Reason.INVALID, "partage de la sauvegarde invalide")
        return ShareOwner(key = key, publicKey = publicKey, name = name, user = user, gist = gist, revision = revision, people = people)
    }

    override fun toString(): String = "ExportedShareOwner(name=$name, people=${people.size}, key=***)"

    companion object {
        fun of(owner: ShareOwner) = ExportedShareOwner(owner.key, owner.publicKey, owner.name, owner.user, owner.gist, owner.revision, owner.people)
    }
}

/** Résultat de la lecture du Gist d'un partage reçu. */
enum class ShareSyncResult { UNCHANGED, UPDATED, GRANTED, WITHDRAWN }

/** Partage reçu (ou demandé) d'une autre personne. */
@Serializable
data class ShareAccess(
    /** Clé de signature de la personne qui partage (épinglée à l'invitation). */
    val owner: String,
    val ownerName: String,
    val user: String,
    val gist: String,
    /** Nom indiqué dans la demande. */
    val myName: String,
    /** Accès accordé ; sinon demande en attente (ou retirée, voir [removed]). */
    val active: Boolean = false,
    val removed: Boolean = false,
    val revision: Long = 0,
    val devices: List<Device> = emptyList(),
    val requested: Long = 0,
    val synced: Long = 0,
    val etag: String = "",
) {
    /** Applique le contenu d'un Gist ; [files] vide si le fichier (ou le Gist) n'existe plus. */
    fun apply(files: Map<String, String>, device: ShareKeyPair, nowMillis: Long): Pair<ShareAccess, ShareSyncResult> {
        val synced = copy(synced = nowMillis)
        val text = files[ShareCrypto.fileName(device.publicEncoded)]
            ?: return if (active) synced.copy(active = false, removed = true, devices = emptyList()) to ShareSyncResult.WITHDRAWN
            else synced to ShareSyncResult.UNCHANGED
        val opened = ShareCrypto.open(text, owner, device, revision)
        val next = synced.copy(active = true, removed = false, revision = opened.revision, ownerName = opened.content.ownerName, devices = opened.content.devices)
        val result = when {
            !active -> ShareSyncResult.GRANTED
            opened.revision != revision -> ShareSyncResult.UPDATED
            else -> ShareSyncResult.UNCHANGED
        }
        return next to result
    }

    /** Identifiant local d'un PC reçu : stable et distinct des PC de l'appareil (comme Windows). */
    fun localId(deviceId: String): String = sharedId(owner, deviceId)

    companion object {
        fun sharedId(owner: String, deviceId: String): String =
            "s-" + ShareCrypto.hex(MessageDigest.getInstance("SHA-256").digest("$owner/$deviceId".toByteArray()).copyOf(16))
    }
}

/** État du partage d'un appareil : partager ses PC et recevoir ceux des autres (stocké chiffré). */
@Serializable
data class ShareState(
    val version: Int = 1,
    val owner: ShareOwner? = null,
    /** Clé de réception de cet appareil (créée à la première demande). */
    val deviceKey: String = "",
    val devicePublic: String = "",
    val received: List<ShareAccess> = emptyList(),
) {
    val deviceKeyPair: ShareKeyPair? get() = if (deviceKey.isEmpty()) null else ShareCrypto.keyPair(deviceKey, devicePublic)

    fun access(owner: String): ShareAccess? = received.firstOrNull { it.owner == owner }

    fun replaceAccess(a: ShareAccess): ShareState = copy(received = received.map { if (it.owner == a.owner) a else it })

    override fun toString(): String = "ShareState(owner=$owner, received=${received.size}, deviceKey=***)"
}
