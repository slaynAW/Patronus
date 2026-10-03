package io.github.slaynaw.wakeonlan.share

import android.content.Context
import io.github.slaynaw.wakeonlan.BuildConfig
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.share.ExportedShareOwner
import io.github.slaynaw.wakeonlan.core.share.GitHubDeviceCode
import io.github.slaynaw.wakeonlan.core.share.ShareAccess
import io.github.slaynaw.wakeonlan.core.share.ShareCrypto
import io.github.slaynaw.wakeonlan.core.share.ShareException
import io.github.slaynaw.wakeonlan.core.share.ShareGitHub
import io.github.slaynaw.wakeonlan.core.share.ShareInvite
import io.github.slaynaw.wakeonlan.core.share.ShareLinks
import io.github.slaynaw.wakeonlan.core.share.ShareOwner
import io.github.slaynaw.wakeonlan.core.share.SharePerson
import io.github.slaynaw.wakeonlan.core.share.ShareRequest
import io.github.slaynaw.wakeonlan.core.share.ShareRight
import io.github.slaynaw.wakeonlan.core.share.ShareSyncResult
import io.github.slaynaw.wakeonlan.data.ConfigRepository
import io.github.slaynaw.wakeonlan.data.ShareRepository
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import java.util.concurrent.ConcurrentHashMap

/** PC reçu d'une autre personne (identifiant local distinct des PC du téléphone). */
data class SharedDevice(val device: Device, val owner: String, val ownerName: String)

/** Connexion GitHub en cours : code à saisir sur github.com. */
data class ShareLogin(val code: String, val uri: String, val error: String? = null)

/** État du partage pour l'interface. */
data class ShareUiState(
    val loaded: Boolean = false,
    /** Connexion GitHub configurée dans cette version (sinon : réception seulement). */
    val canLogin: Boolean = false,
    val owner: ShareOwner? = null,
    val received: List<ShareAccess> = emptyList(),
    val login: ShareLogin? = null,
    val publishing: Boolean = false,
    val publishError: String? = null,
    val syncErrors: Map<String, String> = emptyMap(),
)

/** Demande d'accès lue par la personne qui partage. */
data class ShareRequestInfo(val request: ShareRequest, val code: String, val rights: Map<String, ShareRight>?)

/** Demande à transmettre à la personne qui partage. */
data class ShareRequestView(val owner: String, val ownerName: String, val link: String, val code: String)

/**
 * Partage des PC entre personnes (docs/PARTAGE.md), mêmes règles que l'application Windows :
 * la personne qui partage publie dans son Gist un fichier chiffré et signé par appareil autorisé ;
 * chaque appareil qui reçoit relit le sien régulièrement (application visible).
 */
class ShareManager(context: Context, private val scope: CoroutineScope, private val config: ConfigRepository) {
    private val repo = ShareRepository(context)
    private val github = ShareGitHub(BuildConfig.GITHUB_CLIENT_ID, "Patronus-Android/${BuildConfig.VERSION_NAME}")

    private data class Runtime(
        val login: ShareLogin? = null,
        val publishing: Boolean = false,
        val publishError: String? = null,
        val publishFailedAt: Long = 0,
        val syncErrors: Map<String, String> = emptyMap(),
    )

    private val runtime = MutableStateFlow(Runtime())
    private val publishMutex = Mutex()
    private val syncMutex = Mutex()
    private val lastSync = ConcurrentHashMap<String, Long>()
    private val wake = Channel<Unit>(Channel.CONFLATED)
    private val loginWake = Channel<Unit>(Channel.CONFLATED)
    private var loginJob: Job? = null

    val state: StateFlow<ShareUiState> = combine(repo.data, runtime) { st, rt ->
        ShareUiState(
            loaded = true,
            canLogin = github.clientId.isNotEmpty(),
            owner = st.owner,
            received = st.received,
            login = rt.login,
            publishing = rt.publishing,
            publishError = rt.publishError,
            syncErrors = rt.syncErrors,
        )
    }.stateIn(scope, SharingStarted.Eagerly, ShareUiState())

    /**
     * PC reçus. Flux froid : il n'émet qu'une fois l'état relu, pour que l'historique des PC reçus ne
     * soit pas effacé au démarrage (liste encore vide).
     */
    val sharedDevices: Flow<List<SharedDevice>> = repo.data.map { st ->
        st.received.filter { it.active }.flatMap { a -> a.devices.map { SharedDevice(it.copy(id = a.localId(it.id)), a.owner, a.ownerName) } }
    }.distinctUntilChanged()

    /** Lien de partage ouvert depuis un message ou un QR code, à traiter par l'interface. */
    val pendingLink = MutableStateFlow<String?>(null)

    /** Tant que l'application est visible : publication des changements, vérification des accès reçus. */
    suspend fun run(): Unit = coroutineScope {
        launch {
            combine(config.config, repo.data.map { it.owner }.distinctUntilChanged()) { c, o -> c.devices to o }
                .collect { (_, owner) -> if (owner?.isConnected == true) publish() }
        }
        while (true) {
            try {
                val rt = runtime.value
                if (rt.publishError != null && System.currentTimeMillis() - rt.publishFailedAt >= PUBLISH_RETRY_MS) publish()
                syncDue()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                // Jamais de fermeture de l'application pour une vérification de fond.
                DiagnosticLog.e(AREA, "vérification périodique en échec", e)
            }
            withTimeoutOrNull(TICK_MS) { wake.receive() }
        }
    }

    /**
     * Retour dans l'application (par exemple après la validation du code dans le navigateur) : la
     * connexion en cours interroge GitHub tout de suite, les partages reçus sont vérifiés.
     */
    fun onResume() {
        if (loginJob?.isActive == true) loginWake.trySend(Unit)
        wake.trySend(Unit)
    }

    // --- Je partage mes PC ---

    /** Démarre la connexion GitHub ; la suite (validation du code, création du Gist) continue en fond. */
    suspend fun startLogin(name: String): GitHubDeviceCode {
        val trimmed = name.trim()
        if (!ShareCrypto.isValidName(trimmed)) throw ShareException(ShareException.Reason.INVALID, "nom invalide (1 à ${ShareCrypto.MAX_NAME_LENGTH} caractères)")
        loginJob?.cancel()
        DiagnosticLog.i(AREA, "connexion GitHub : demande d'un code")
        val code = try {
            github.startLogin()
        } catch (e: ShareException) {
            DiagnosticLog.w(AREA, "connexion GitHub : code refusé (${e.reason})", e)
            throw e
        }
        runtime.update { it.copy(login = ShareLogin(code.userCode, code.verificationUri)) }
        loginJob = scope.launch { finishLogin(code, trimmed) }
        return code
    }

    fun cancelLogin() {
        loginJob?.cancel()
        loginJob = null
        runtime.update { it.copy(login = null) }
    }

    private suspend fun finishLogin(code: GitHubDeviceCode, name: String) {
        try {
            // En arrière-plan (navigateur ouvert), le système peut couper l'accès à Internet de
            // l'application : les étapes sont retentées tant que l'échec est passager.
            val token = github.waitLogin(code, loginWake)
            DiagnosticLog.i(AREA, "connexion GitHub : code validé")
            val user = retrying { github.user(token) }
            val existing = repo.current().owner
            if (existing != null && existing.gist.isNotEmpty() && existing.user.isNotEmpty() && !existing.user.equals(user, ignoreCase = true)) {
                throw ShareException(
                    ShareException.Reason.DENIED,
                    "connectez-vous avec le compte GitHub @${existing.user}, qui contient vos partages (compte utilisé : @$user)",
                )
            }
            val gist = existing?.gist?.takeIf { it.isNotEmpty() }
                ?: retrying { github.createGist(token, "Patronus – partage chiffré", mapOf("LISEZMOI.md" to GIST_NOTE)) }
            repo.update { st ->
                val owner = st.owner ?: withKey()
                st.copy(owner = owner.copy(name = name, token = token, user = user, gist = gist))
            }
            DiagnosticLog.i(AREA, "connexion GitHub réussie : compte @$user, gist ${gist.take(8)}…, partage existant : ${existing != null}")
            runtime.update { it.copy(login = null, publishError = null) }
            publish()
        } catch (e: CancellationException) {
            throw e
        } catch (e: ShareException) {
            DiagnosticLog.w(AREA, "connexion GitHub en échec (${e.reason})", e)
            runtime.update { rt -> rt.copy(login = rt.login?.copy(error = e.message)) }
        } catch (e: Exception) {
            DiagnosticLog.e(AREA, "connexion GitHub : erreur imprévue", e)
            runtime.update { rt -> rt.copy(login = rt.login?.copy(error = "erreur imprévue (${e.javaClass.simpleName}) : voir le rapport de diagnostic")) }
        }
    }

    /**
     * GitHub refuse le jeton du partage ailleurs que dans la publication (connexion des sauvegardes) :
     * il est oublié, et l'interface propose de se reconnecter, comme après une publication refusée.
     */
    internal suspend fun tokenRejected(token: String, message: String?) {
        if (token.isEmpty()) return
        val before = repo.current().owner
        val after = repo.update { st -> st.owner?.takeIf { it.token == token }?.let { st.copy(owner = it.copy(token = "")) } ?: st }.owner
        if (before?.token == token && after?.token?.isEmpty() == true) {
            DiagnosticLog.w(AREA, "accès GitHub refusé, jeton oublié")
            runtime.update { it.copy(publishError = message, publishFailedAt = System.currentTimeMillis()) }
        }
    }

    /**
     * Reprend le jeton d'une nouvelle connexion GitHub (celle des sauvegardes) quand le partage a perdu
     * le sien et qu'il s'agit du même compte : une seule connexion par code répare les deux. La
     * publication repart d'elle-même (partage de nouveau connecté).
     */
    internal suspend fun adoptToken(token: String, user: String) {
        if (token.isEmpty()) return
        val adoptable = { o: ShareOwner -> o.token.isEmpty() && o.gist.isNotEmpty() && o.user.equals(user, ignoreCase = true) }
        if (repo.current().owner?.let(adoptable) != true) return
        val owner = repo.update { st -> st.owner?.takeIf(adoptable)?.let { st.copy(owner = it.copy(token = token)) } ?: st }.owner
        if (owner?.token == token) {
            DiagnosticLog.i(AREA, "connexion GitHub des sauvegardes reprise par le partage (@$user)")
            runtime.update { it.copy(publishError = null) }
        }
    }

    /** Réessaie une étape de la connexion (à intervalle croissant, ou au retour dans l'application). */
    private suspend fun <T> retrying(step: suspend () -> T): T {
        var attempt = 1
        while (true) {
            try {
                return step()
            } catch (e: ShareException) {
                if (!e.isTransient || attempt >= LOGIN_ATTEMPTS) throw e
            }
            withTimeoutOrNull(attempt * LOGIN_RETRY_MS) { loginWake.receive() }
            attempt++
        }
    }

    private fun withKey(): ShareOwner {
        val key = ShareCrypto.newKeyPair()
        return ShareOwner(key = key.privateEncoded, publicKey = key.publicEncoded, name = "")
    }

    /** Invitation (lien et QR code) de la personne qui partage. */
    fun invite(): ShareInvite {
        val o = state.value.owner?.takeIf { it.isConnected } ?: throw ShareException(ShareException.Reason.UNAUTHORIZED, "connectez-vous d'abord à GitHub")
        return ShareInvite(o.name, o.publicKey, o.user, o.gist)
    }

    /** Lit la demande d'accès d'une personne ; erreur explicite si elle ne répond pas à notre invitation. */
    fun readRequest(text: String): ShareRequestInfo {
        if (ShareLinks.kind(text) == "invite") {
            throw ShareException(ShareException.Reason.INVALID, "c'est un lien d'invitation : la personne invitée doit l'ouvrir dans son application, puis vous renvoyer sa demande")
        }
        val request = ShareLinks.parseRequest(text)
        val owner = state.value.owner ?: throw ShareException(ShareException.Reason.UNAUTHORIZED, "le partage n'est pas activé")
        if (request.owner != ShareCrypto.keyId(owner.publicKey)) {
            throw ShareException(ShareException.Reason.INVALID, "cette demande répond à l'invitation d'une autre personne")
        }
        val known = owner.person(request.device)
        DiagnosticLog.i(AREA, "demande lue : « ${request.name} » (${DiagnosticLog.keyId(request.device)}), ${if (known != null) "personne déjà autorisée" else "nouvelle personne"}")
        return ShareRequestInfo(request, ShareCrypto.verificationCode(owner.publicKey, request.device), known?.rights)
    }

    /** Autorise une personne (ou modifie ses droits) puis publie. */
    suspend fun grant(device: String, name: String, rights: Map<String, ShareRight>) {
        val devices = config.current().devices.associateBy { it.id }
        val granted = rights.mapNotNull { (id, right) ->
            val d = devices[id] ?: return@mapNotNull null
            id to if (right == ShareRight.FULL && !(d.canShutdown && d.agent?.hasKey == true)) ShareRight.WAKE else right
        }.toMap()
        val summary = granted.entries.joinToString(", ") { (id, right) -> "${devices[id]?.name} = ${right.name.lowercase()}" }
        DiagnosticLog.i(AREA, "autorisation de « $name » (${DiagnosticLog.keyId(device)}) : ${granted.size} PC retenus sur ${rights.size} ($summary)")
        if (granted.isEmpty()) throw ShareException(ShareException.Reason.INVALID, "choisissez au moins un PC à partager")
        if (!ShareCrypto.isValidPublicKey(device) || !ShareCrypto.isValidName(name)) {
            DiagnosticLog.w(AREA, "autorisation refusée : demande invalide (clé valide : ${ShareCrypto.isValidPublicKey(device)}, nom valide : ${ShareCrypto.isValidName(name)})")
            throw ShareException(ShareException.Reason.INVALID, "demande invalide")
        }
        try {
            repo.update { st ->
                val owner = st.owner ?: throw ShareException(ShareException.Reason.UNAUTHORIZED, "le partage n'est pas activé")
                st.copy(owner = owner.grant(SharePerson(name, device, System.currentTimeMillis(), granted)))
            }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            DiagnosticLog.e(AREA, "autorisation non enregistrée", e)
            throw e
        }
        DiagnosticLog.i(AREA, "autorisation enregistrée ; publication lancée")
        scope.launch { publish() }
    }

    /** Retire l'accès d'une personne (son fichier est supprimé à la publication). */
    suspend fun revoke(device: String) {
        DiagnosticLog.i(AREA, "retrait de l'accès de ${DiagnosticLog.keyId(device)}")
        repo.update { st -> st.copy(owner = st.owner?.withdraw(device)) }
        scope.launch { publish() }
    }

    /** Arrête de partager : le Gist est supprimé, la clé oubliée. */
    suspend fun stop() {
        val owner = repo.current().owner ?: return
        DiagnosticLog.i(AREA, "arrêt du partage (${owner.people.size} personne(s), connecté : ${owner.isConnected})")
        if (owner.isConnected) {
            try {
                github.deleteGist(owner.token, owner.gist)
            } catch (e: ShareException) {
                if (e.reason != ShareException.Reason.NOT_FOUND) throw e
            }
        }
        repo.update { it.copy(owner = null) }
        runtime.update { it.copy(publishError = null) }
    }

    /** Publie les fichiers d'accès qui ont changé. */
    suspend fun publish() = publishMutex.withLock {
        val owner = repo.current().owner?.takeIf { it.isConnected } ?: return@withLock
        val devices = config.current().devices
        val publication = try {
            withContext(Dispatchers.Default) { owner.prepare(devices, System.currentTimeMillis()) }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            DiagnosticLog.e(AREA, "publication : préparation des fichiers impossible (${owner.people.size} personne(s), ${devices.size} PC)", e)
            runtime.update {
                it.copy(publishError = "erreur imprévue (${e.javaClass.simpleName}) : voir le rapport de diagnostic", publishFailedAt = System.currentTimeMillis())
            }
            return@withLock
        }
        if (publication.isEmpty) {
            runtime.update { it.copy(publishError = null) }
            return@withLock
        }
        runtime.update { it.copy(publishing = true) }
        val deletions = publication.files.count { it.value == null }
        DiagnosticLog.i(AREA, "publication : ${publication.files.size - deletions} fichier(s) d'accès à écrire, $deletions à supprimer, révision ${publication.revision}")
        try {
            github.updateGist(owner.token, owner.gist, publication.files)
            repo.update { st -> st.owner?.takeIf { it.key == owner.key }?.let { st.copy(owner = it.commit(publication)) } ?: st }
            runtime.update { it.copy(publishing = false, publishError = null) }
            DiagnosticLog.i(AREA, "publication réussie")
        } catch (e: CancellationException) {
            runtime.update { it.copy(publishing = false) }
            throw e
        } catch (e: ShareException) {
            DiagnosticLog.w(AREA, "publication en échec (${e.reason})", e)
            if (e.reason == ShareException.Reason.UNAUTHORIZED) {
                // Jeton expiré ou révoqué : l'interface propose de se reconnecter (même compte, même Gist).
                repo.update { st -> st.owner?.takeIf { it.key == owner.key }?.let { st.copy(owner = it.copy(token = "")) } ?: st }
            }
            val message = if (e.reason == ShareException.Reason.NOT_FOUND) {
                "espace de partage introuvable sur GitHub (Gist supprimé ?) : arrêtez puis réactivez le partage"
            } else {
                e.message
            }
            runtime.update { it.copy(publishing = false, publishError = message, publishFailedAt = System.currentTimeMillis()) }
        } catch (e: Exception) {
            DiagnosticLog.e(AREA, "publication : erreur imprévue", e)
            runtime.update {
                it.copy(
                    publishing = false,
                    publishError = "erreur imprévue (${e.javaClass.simpleName}) : voir le rapport de diagnostic",
                    publishFailedAt = System.currentTimeMillis(),
                )
            }
        }
    }

    /** Côté « je partage » à inclure dans une sauvegarde complète (null : pas de partage). */
    suspend fun exportOwner(): ExportedShareOwner? = repo.current().owner?.let { ExportedShareOwner.of(it) }

    /**
     * Reprend le partage d'une sauvegarde si le téléphone ne partage pas encore ; il reste à se
     * reconnecter à GitHub (même compte) pour que les accès continuent. Renvoie vrai s'il est repris.
     */
    suspend fun importOwner(exported: ExportedShareOwner): Boolean {
        val owner = exported.toOwner()
        var adopted = false
        repo.update { st ->
            adopted = st.owner == null
            if (adopted) st.copy(owner = owner) else st
        }
        DiagnosticLog.i(AREA, "partage de la sauvegarde ${if (adopted) "repris" else "ignoré (partage déjà actif)"} : ${owner.people.size} personne(s)")
        return adopted
    }

    // --- Je reçois les PC d'une autre personne ---

    /** Lit une invitation (avant de demander l'accès). */
    fun readInvite(text: String): ShareInvite {
        if (ShareLinks.kind(text) == "request") {
            throw ShareException(ShareException.Reason.INVALID, "c'est une demande d'accès : c'est la personne qui partage qui doit l'ajouter")
        }
        val invite = ShareLinks.parseInvite(text)
        if (invite.owner == state.value.owner?.publicKey) throw ShareException(ShareException.Reason.INVALID, "c'est votre propre invitation")
        return invite
    }

    /** Enregistre la demande d'accès et renvoie le lien et le code à transmettre. */
    suspend fun request(invite: ShareInvite, myName: String): ShareRequestView {
        val name = myName.trim()
        if (!ShareCrypto.isValidName(name)) throw ShareException(ShareException.Reason.INVALID, "indiquez votre nom (1 à ${ShareCrypto.MAX_NAME_LENGTH} caractères)")
        repo.update { st ->
            var next = st
            if (next.deviceKey.isEmpty()) {
                val key = ShareCrypto.newKeyPair()
                next = next.copy(deviceKey = key.privateEncoded, devicePublic = key.publicEncoded)
            }
            val existing = next.access(invite.owner)
            val now = System.currentTimeMillis()
            if (existing != null) {
                next.replaceAccess(
                    existing.copy(
                        myName = name, ownerName = invite.name, user = invite.user, gist = invite.gist,
                        requested = if (existing.active) existing.requested else now,
                    ),
                )
            } else {
                next.copy(received = next.received + ShareAccess(invite.owner, invite.name, invite.user, invite.gist, name, requested = now))
            }
        }
        lastSync.remove(invite.owner)
        wake.trySend(Unit)
        DiagnosticLog.i(AREA, "demande d'accès enregistrée auprès de « ${invite.name} » (${DiagnosticLog.keyId(invite.owner)}, @${invite.user})")
        return requestView(invite.owner) ?: throw ShareException(ShareException.Reason.INVALID, "demande introuvable")
    }

    /** Lien, QR code et code de vérification d'une demande déjà faite. */
    suspend fun requestView(owner: String): ShareRequestView? {
        val st = repo.current()
        val access = st.access(owner) ?: return null
        if (st.devicePublic.isEmpty()) return null
        val request = ShareRequest(access.myName, st.devicePublic, ShareCrypto.keyId(owner))
        return ShareRequestView(owner, access.ownerName, request.link, ShareCrypto.verificationCode(owner, st.devicePublic))
    }

    /** Supprime un partage reçu (ou une demande) du téléphone. */
    suspend fun leave(owner: String) {
        DiagnosticLog.i(AREA, "partage reçu supprimé : ${DiagnosticLog.keyId(owner)}")
        repo.update { st -> st.copy(received = st.received.filterNot { it.owner == owner }) }
        lastSync.remove(owner)
        runtime.update { it.copy(syncErrors = it.syncErrors - owner) }
    }

    /** Vérifie tout de suite les partages reçus. */
    fun syncNow() {
        lastSync.clear()
        wake.trySend(Unit)
    }

    private suspend fun syncDue() {
        val now = System.currentTimeMillis()
        for (a in repo.current().received) {
            val interval = when {
                !a.active && !a.removed && now - a.requested < PENDING_FAST_MS -> SYNC_PENDING_MS
                !a.active -> SYNC_WAITING_MS
                else -> SYNC_ACTIVE_MS
            }
            if (now - (lastSync[a.owner] ?: 0L) >= interval) sync(a.owner)
        }
    }

    private suspend fun sync(owner: String) = syncMutex.withLock {
        val st = repo.current()
        val access = st.access(owner) ?: return@withLock
        val key = st.deviceKeyPair ?: return@withLock
        val now = System.currentTimeMillis()
        lastSync[owner] = now
        var files: Map<String, String> = emptyMap()
        var etag = access.etag
        try {
            val snap = github.fetchGist(access.gist, access.etag)
            if (snap.notModified) {
                store(owner) { it.copy(synced = now) }
                return@withLock
            }
            files = snap.files
            etag = snap.etag
        } catch (e: ShareException) {
            if (e.reason != ShareException.Reason.NOT_FOUND) {
                DiagnosticLog.w(AREA, "partage reçu de ${DiagnosticLog.keyId(owner)} : lecture impossible (${e.reason})", e)
                runtime.update { it.copy(syncErrors = it.syncErrors + (owner to e.message.orEmpty())) }
                return@withLock
            }
            DiagnosticLog.w(AREA, "partage reçu de ${DiagnosticLog.keyId(owner)} : espace de partage introuvable")
            // Gist supprimé (fichiers vides) : même effet qu'un accès retiré.
            etag = ""
        }
        try {
            val (next, result) = withContext(Dispatchers.Default) { access.apply(files, key, now) }
            if (result != ShareSyncResult.UNCHANGED) {
                DiagnosticLog.i(AREA, "partage reçu de ${DiagnosticLog.keyId(owner)} : $result (${next.devices.size} PC, révision ${next.revision})")
            }
            store(owner) {
                it.copy(
                    active = next.active, removed = next.removed, revision = next.revision, devices = next.devices,
                    ownerName = next.ownerName, synced = next.synced, etag = etag,
                )
            }
            runtime.update { it.copy(syncErrors = it.syncErrors - owner) }
        } catch (e: ShareException) {
            // Fichier refusé (signature, version plus ancienne…) : pas relu tant qu'il n'a pas changé.
            DiagnosticLog.w(AREA, "partage reçu de ${DiagnosticLog.keyId(owner)} : fichier refusé (${e.reason})", e)
            store(owner) { it.copy(etag = etag, synced = now) }
            runtime.update { it.copy(syncErrors = it.syncErrors + (owner to e.message.orEmpty())) }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            DiagnosticLog.e(AREA, "partage reçu de ${DiagnosticLog.keyId(owner)} : erreur imprévue", e)
            store(owner) { it.copy(etag = etag, synced = now) }
            runtime.update { it.copy(syncErrors = it.syncErrors + (owner to "erreur imprévue (${e.javaClass.simpleName})")) }
        }
    }

    /** Modifie un partage reçu s'il existe toujours (il a pu être supprimé pendant la lecture). */
    private suspend fun store(owner: String, transform: (ShareAccess) -> ShareAccess) {
        repo.update { cur -> cur.access(owner)?.let { cur.replaceAccess(transform(it)) } ?: cur }
    }

    private companion object {
        const val AREA = "partage"
        const val TICK_MS = 15_000L
        // GitHub limite la lecture sans compte à 60 requêtes par heure et par adresse IP.
        const val SYNC_PENDING_MS = 45_000L
        const val PENDING_FAST_MS = 15 * 60_000L
        const val SYNC_WAITING_MS = 5 * 60_000L
        const val SYNC_ACTIVE_MS = 10 * 60_000L
        const val PUBLISH_RETRY_MS = 2 * 60_000L
        const val LOGIN_ATTEMPTS = 5
        const val LOGIN_RETRY_MS = 5_000L
        const val GIST_NOTE = "# Patronus – partage chiffré\n\nFichiers d'accès chiffrés de l'application Patronus " +
            "(https://github.com/slaynAW/Patronus). Chacun n'est lisible que par l'appareil auquel il est destiné.\n"
    }
}
