package io.github.slaynaw.wakeonlan.core.share

import io.github.slaynaw.wakeonlan.core.agent2.Role
import io.github.slaynaw.wakeonlan.core.config.ConfigCodec
import io.github.slaynaw.wakeonlan.core.model.AgentSettings
import io.github.slaynaw.wakeonlan.core.model.Device
import kotlinx.serialization.EncodeDefault
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.security.MessageDigest
import java.time.Instant

/** Droit accordé sur un PC partagé (docs/PARTAGE.md, « Partage v2 »). */
@Serializable
enum class ShareRight(
    /** Rôle wolagent/2 donné à l'appareil de la personne (partage v2) ; null : l'agent n'est pas joint. */
    val agentRole: Role?,
) {
    /** Démarrer et voir l'état. L'agent n'est pas joint : ni clé, ni enregistrement. */
    @SerialName("wake")
    WAKE(null),

    /** Démarrer et consulter (état, journal, mesures, fiche du PC). Partage v2 seulement : rôle « wake ». */
    @SerialName("view")
    VIEW(Role.WAKE),

    /** Démarrer, éteindre, redémarrer, mettre en veille. Partage v2 : rôle « control » ; v1 : clé de l'agent transmise. */
    @SerialName("full")
    FULL(Role.CONTROL),
}

/** Personne (appareil) autorisée. */
@OptIn(ExperimentalSerializationApi::class)
@Serializable
data class SharePerson(
    val name: String,
    /** Clé publique de réception de son appareil. */
    val device: String,
    val added: Long = 0,
    /** Identifiant de chaque PC partagé → droit accordé. */
    val rights: Map<String, ShareRight> = emptyMap(),
    /**
     * Empreinte wolagent/2 de son appareil, reçue dans sa demande (paramètre « id ») : vide pour une demande d'une
     * application qui ne connaît que le partage v1.
     */
    @EncodeDefault(EncodeDefault.Mode.NEVER) val identity: String = "",
)

/** Appairage wolagent/2 d'une personne sur un PC (partage v2). */
@OptIn(ExperimentalSerializationApi::class)
@Serializable
data class SharePairing(
    /** Clé de réception de la personne ([SharePerson.device]). */
    val person: String,
    /** Identifiant du PC. */
    val device: String,
    /** Empreinte wolagent/2 de l'appareil de la personne. */
    val identity: String,
    /** Rôle de l'invitation. */
    val role: String,
    /** Invitation en attente : identifiant, code (secret : jamais exporté ni journalisé), fin de validité (secondes Unix). */
    @EncodeDefault(EncodeDefault.Mode.NEVER) val invite: String = "",
    @EncodeDefault(EncodeDefault.Mode.NEVER) val code: String = "",
    @EncodeDefault(EncodeDefault.Mode.NEVER) val expires: Long = 0,
    /** L'appareil de la personne figure dans le registre du PC. */
    @EncodeDefault(EncodeDefault.Mode.NEVER) val registered: Boolean = false,
) {
    override fun toString(): String =
        "SharePairing(device=$device, role=$role, invite=$invite, code=${if (code.isEmpty()) "" else "***"}, expires=$expires, registered=$registered)"
}

/** Appareil à retirer d'un PC : invitations réservées à son empreinte à annuler, puis enregistrement à supprimer. */
@Serializable
data class ShareRevocation(
    /** Identifiant du PC. */
    val device: String,
    /** Empreinte wolagent/2 de l'appareil à retirer. */
    val identity: String,
)

/** Lot de fichiers à écrire dans le Gist (null : fichier à supprimer). */
data class SharePublication(val files: Map<String, String?>, val fingerprints: Map<String, String>, val revision: Long) {
    val isEmpty: Boolean get() = files.isEmpty()
}

/** Côté « je partage mes PC ». */
@OptIn(ExperimentalSerializationApi::class)
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
    /**
     * Appairages des personnes, par PC partagé en v2 : invitation créée sur l'agent ou appareil enregistré
     * (docs/PARTAGE.md, « Partage v2 »). Jamais exportés.
     */
    @EncodeDefault(EncodeDefault.Mode.NEVER) val pairings: List<SharePairing> = emptyList(),
    /** Appareils à retirer des PC (accès retiré ou réduit), dès que cet appareil joint le PC en administrateur. */
    @EncodeDefault(EncodeDefault.Mode.NEVER) val revocations: List<ShareRevocation> = emptyList(),
) {
    val keyPair: ShareKeyPair get() = ShareCrypto.keyPair(key, publicKey)
    val isConnected: Boolean get() = token.isNotEmpty() && gist.isNotEmpty()

    fun person(device: String): SharePerson? = people.firstOrNull { it.device == device }

    /**
     * Ce que reçoit une personne : ses PC autorisés, dans l'ordre de la configuration (règles : docs/PARTAGE.md,
     * « Partage v2 ») :
     * - « démarrer » : le PC sans agent ;
     * - partage v2 (demande avec empreinte, agent joint par la sienne) : l'empreinte de l'agent, sans clé, une fois
     *   l'invitation créée ou l'appareil enregistré, et l'invitation tant qu'elle est valable ; avant, le PC sans agent ;
     * - partage v1 : la clé de l'agent pour « démarrer et éteindre », jamais son empreinte.
     *
     * [nowMillis] sert à écarter les invitations expirées.
     */
    fun content(person: SharePerson, devices: List<Device>, nowMillis: Long): ShareContent {
        val invites = ArrayList<AgentInvite>()
        val shared = devices.mapNotNull { d ->
            val right = person.rights[d.id] ?: return@mapNotNull null
            val agent = d.agent
            d.copy(
                agent = when {
                    right == ShareRight.WAKE || agent == null -> null
                    person.identity.isNotEmpty() && agent.hasPin -> pairing(person, d.id)?.let { p ->
                        if (p.code.isNotEmpty() && p.expires > nowMillis / 1000) invites += AgentInvite(d.id, p.code, p.role, p.expires)
                        AgentSettings(port = agent.port, key = "", pin = agent.pin)
                    }
                    right == ShareRight.FULL && d.canShutdown && agent.hasKey -> AgentSettings(port = agent.port, key = agent.key)
                    else -> null
                },
            )
        }
        return ShareContent(ownerName = name, recipientName = person.name, devices = shared, identity = person.identity, invites = invites)
    }

    /** Appairage v2 de la personne sur un PC, pour son empreinte actuelle. */
    private fun pairing(person: SharePerson, device: String): SharePairing? =
        pairings.firstOrNull { it.person == person.device && it.device == device && it.identity == person.identity }

    /** Une personne autorisée doit joindre ce PC par l'agent avec cette empreinte. */
    private fun wants(people: List<SharePerson>, device: String, identity: String): Boolean =
        people.any { it.identity == identity && it.rights[device]?.agentRole != null }

    /** Prépare la publication des fichiers qui ont changé (ou de tous si [all]). */
    fun prepare(devices: List<Device>, nowMillis: Long, all: Boolean = false): SharePublication {
        val pair = keyPair
        val revision = maxOf(revision + 1, nowMillis)
        val issued = Instant.ofEpochMilli(nowMillis).toString()
        val files = LinkedHashMap<String, String?>()
        val fingerprints = HashMap<String, String>()
        for (p in people) {
            val c = content(p, devices, nowMillis)
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

    /**
     * Autorise (ou met à jour) une personne. Son appareil est retiré des PC qu'il ne doit plus joindre par l'agent :
     * droit réduit à « démarrer » ou retiré, ou demande arrivée avec une autre empreinte.
     */
    fun grant(p: SharePerson): ShareOwner {
        val existing = person(p.device)
        if (existing == null && people.size >= MAX_PEOPLE) throw ShareException(ShareException.Reason.INVALID, "$MAX_PEOPLE personnes au maximum")
        val updated = if (existing != null) people.map { if (it.device == p.device) p.copy(added = existing.added) else it } else people + p
        val next = copy(people = updated, withdrawn = withdrawn - p.device, published = published - p.device)
            .let { if (existing != null) it.revokeUnwanted(existing) else it }
        // Un retrait en attente ne doit pas défaire un accès accordé de nouveau.
        return next.copy(revocations = next.revocations.filterNot { wants(next.people, it.device, it.identity) })
    }

    /** Retire une personne ; son fichier sera supprimé à la prochaine publication, son appareil retiré des PC. */
    fun withdraw(device: String): ShareOwner {
        val old = person(device) ?: return this
        return copy(
            people = people.filterNot { it.device == device },
            withdrawn = if (device in withdrawn) withdrawn else withdrawn + device,
            published = published - device,
        ).revokeUnwanted(old)
    }

    /**
     * Suit la modification (ou le retrait) d'une personne, dont [old] est l'état précédent : son appareil est mis en
     * file de retrait des PC qu'il ne doit plus joindre, et ses appairages devenus inutiles sont oubliés. Un PC accordé
     * en v2 est retiré même sans appairage connu (sauvegarde restaurée) : retirer un appareil absent ne change rien.
     */
    private fun revokeUnwanted(old: SharePerson): ShareOwner {
        val queue = LinkedHashSet(revocations)
        if (old.identity.isNotEmpty()) {
            for ((device, right) in old.rights) {
                if (right.agentRole != null && !wants(people, device, old.identity)) queue += ShareRevocation(device, old.identity)
            }
        }
        val current = person(old.device)
        val kept = pairings.filter { x ->
            val keep = x.person != old.device ||
                (current != null && current.identity == x.identity && current.rights[x.device]?.agentRole != null)
            if (!keep && !wants(people, x.device, x.identity)) queue += ShareRevocation(x.device, x.identity)
            keep
        }
        return copy(pairings = kept, revocations = queue.sortedWith(compareBy({ it.device }, { it.identity })))
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
 * jeton GitHub, à reconnecter sur le nouvel appareil. Les appairages ne sont pas exportés : l'application les
 * retrouve dans le registre des PC ; les retraits pas encore appliqués, si.
 */
@OptIn(ExperimentalSerializationApi::class)
@Serializable
data class ExportedShareOwner(
    val key: String,
    val publicKey: String,
    val name: String,
    val user: String = "",
    val gist: String = "",
    val revision: Long = 0,
    val people: List<SharePerson> = emptyList(),
    @EncodeDefault(EncodeDefault.Mode.NEVER) val revocations: List<ShareRevocation> = emptyList(),
) {
    /** Vérifie la sauvegarde et renvoie l'état correspondant (connexion GitHub à refaire). */
    fun toOwner(): ShareOwner {
        val valid = runCatching { ShareCrypto.keyPair(key, publicKey) }.isSuccess &&
            ShareCrypto.isValidName(name) && people.size <= ShareOwner.MAX_PEOPLE &&
            (user.isEmpty() || ShareLinks.LOGIN.matches(user)) && (gist.isEmpty() || ShareLinks.GIST.matches(gist)) &&
            people.all {
                ShareCrypto.isValidName(it.name) && ShareCrypto.isValidPublicKey(it.device) &&
                    (it.identity.isEmpty() || ShareCrypto.isValidIdentity(it.identity))
            } &&
            revocations.all { it.device.isNotEmpty() && ShareCrypto.isValidIdentity(it.identity) }
        if (!valid) throw ShareException(ShareException.Reason.INVALID, "partage de la sauvegarde invalide")
        return ShareOwner(
            key = key, publicKey = publicKey, name = name, user = user, gist = gist, revision = revision, people = people,
            revocations = revocations,
        )
    }

    override fun toString(): String = "ExportedShareOwner(name=$name, people=${people.size}, key=***)"

    companion object {
        fun of(owner: ShareOwner) =
            ExportedShareOwner(owner.key, owner.publicKey, owner.name, owner.user, owner.gist, owner.revision, owner.people, owner.revocations)
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
