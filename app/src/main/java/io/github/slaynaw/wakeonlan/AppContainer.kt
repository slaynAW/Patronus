package io.github.slaynaw.wakeonlan

import android.content.Context
import android.os.Build
import android.provider.Settings
import io.github.slaynaw.wakeonlan.core.agent.AgentClient
import io.github.slaynaw.wakeonlan.core.hub.HubManager
import io.github.slaynaw.wakeonlan.core.hub.HubService
import io.github.slaynaw.wakeonlan.core.model.AppConfig
import io.github.slaynaw.wakeonlan.core.status.HostProber
import io.github.slaynaw.wakeonlan.core.status.ProbeAvailability
import io.github.slaynaw.wakeonlan.core.status.StatusMonitor
import io.github.slaynaw.wakeonlan.archive.ArchiveManager
import io.github.slaynaw.wakeonlan.backup.BackupManager
import io.github.slaynaw.wakeonlan.core.status.UnknownReason
import io.github.slaynaw.wakeonlan.core.wol.WakeOnLanSender
import io.github.slaynaw.wakeonlan.data.ConfigRepository
import io.github.slaynaw.wakeonlan.data.HistoryRepository
import io.github.slaynaw.wakeonlan.data.HubRepository
import io.github.slaynaw.wakeonlan.data.KeystoreIdentity
import io.github.slaynaw.wakeonlan.data.SpecsRepository
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import io.github.slaynaw.wakeonlan.network.LanNetworkMonitor
import io.github.slaynaw.wakeonlan.network.LocalNetworkAccess
import io.github.slaynaw.wakeonlan.security.AppLock
import io.github.slaynaw.wakeonlan.share.ShareManager
import io.github.slaynaw.wakeonlan.update.AppUpdater
import kotlinx.coroutines.CoroutineExceptionHandler
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine

/**
 * Assemble les composants de l'application (injection de dépendances manuelle, volontairement
 * simple). Une seule instance, créée par [WolApplication].
 */
class AppContainer(private val context: Context) {
    /**
     * Tâches de fond de l'application (enregistrement de l'historique, lecture du journal des agents,
     * publication du partage). Une erreur imprévue y est notée au journal de diagnostic au lieu de
     * fermer l'application.
     */
    private val scope = CoroutineScope(
        SupervisorJob() + Dispatchers.Default + CoroutineExceptionHandler { _, e ->
            DiagnosticLog.e("tâche", "erreur imprévue dans une tâche de fond", e)
        },
    )

    val network = LanNetworkMonitor(context)
    val repository = ConfigRepository(context)

    /** Verrouillage facultatif par empreinte, visage ou code du téléphone (réglage propre à ce téléphone). */
    val appLock = AppLock.create(context)

    /** Identité wolagent/2 de ce téléphone (Android 10 ou plus), créée dans le Keystore au premier besoin. */
    private val identity = KeystoreIdentity()

    /** Partage des PC entre personnes (PC reçus, accès accordés). */
    val share = ShareManager(context, scope, repository, identityPin = { identity.get()?.pin?.toString() })

    /** PC du téléphone puis PC reçus d'autres personnes : ce que surveille et affiche l'application. */
    val allDevices: Flow<AppConfig> = combine(repository.config, share.sharedDevices) { config, shared ->
        if (shared.isEmpty()) config else config.copy(devices = config.devices + shared.map { it.device })
    }

    /**
     * Le nom du téléphone accompagne chaque demande wolagent/1 et l'enregistrement auprès des agents (wolagent/2) : les
     * agents le notent dans l'historique commun.
     */
    private val agentClient = AgentClient(binder = network, deviceName = ::deviceName, identity = identity::get)
    private val prober = HostProber(binder = network, agentClient = agentClient)
    val statusMonitor = StatusMonitor(prober = prober)
    val history = HistoryRepository(context, scope)
    val historyTracker = HistoryTracker(allDevices, statusMonitor.statuses, agentClient, history, scope)

    /** Fiche de chaque PC (agent 1.10.0), gardée pour l'afficher PC éteint. */
    val specs = SpecsRepository(context, scope)
    val specsTracker = SpecsTracker(allDevices, statusMonitor.statuses, agentClient, specs, scope)
    val actions = DeviceActions(network, WakeOnLanSender(binder = network), agentClient, statusMonitor, historyTracker, prober)

    /** Passage automatique des PC du téléphone en wolagent/2 (connexion chiffrée, identité propre au téléphone). */
    val agentUpgrades = AgentUpgradeTracker(repository, statusMonitor.statuses, agentClient, prober, share, scope)

    /** Partage v2 : invitations, rôles et retraits sur les PC administrés par ce téléphone. */
    val shareAgents = ShareAgentTracker(
        repository, statusMonitor.statuses, agentClient, share, scope,
        selfPin = { runCatching { identity.get()?.pin?.short() }.getOrNull() },
    )

    /** Partage v2 : invitations reçues utilisées dès que le PC répond. */
    val receivedPairing = ReceivedPairingTracker(statusMonitor.statuses, agentClient, share, scope)

    /** Sauvegardes automatiques (GitHub et dossier), chiffrées par un mot de passe dédié. */
    val backups = BackupManager(context, scope, repository, share, history)

    /**
     * Hub sur le NAS (docs/HUB.md, §12), facultatif : appairage chiffré par le Keystore, relevé de son état tant que
     * l'interface le regarde, administration et PC confiés. Même identité que pour les agents (wolagent/2).
     */
    val hub = HubManager(
        scope = scope,
        store = HubRepository(context, scope),
        service = HubService(binder = network, deviceName = ::deviceName, identity = identity::get),
        agents = agentClient,
        selfPin = { runCatching { identity.get()?.pin?.toString() }.getOrNull() },
        log = { DiagnosticLog.i("hub", it) },
    )

    /** Archives chiffrées des mesures et du journal des PC (même compte et même mot de passe). */
    val archives = ArchiveManager(scope, repository, allDevices, backups, agentClient)

    /** Mises à jour intégrées (versions officielles publiées sur GitHub). */
    val updater = AppUpdater(context, scope)

    private val _localNetworkGranted = MutableStateFlow(LocalNetworkAccess.isGranted(context))

    /** Autorisation Android 17 « réseau local » accordée (toujours vrai avant Android 17). */
    val localNetworkGranted: StateFlow<Boolean> = _localNetworkGranted.asStateFlow()

    /** Empreinte courte de ce téléphone (« Pendant votre absence » : ses propres demandes de réveil), null sans identité. */
    fun selfShortPin(): String? = runCatching { identity.get()?.pin?.short() }.getOrNull()

    /** Nom de ce téléphone (réglages Android « Nom de l'appareil »), à défaut son modèle. */
    private fun deviceName(): String =
        Settings.Global.getString(context.contentResolver, Settings.Global.DEVICE_NAME)?.takeIf { it.isNotBlank() } ?: Build.MODEL

    fun refreshPermissions() {
        _localNetworkGranted.value = LocalNetworkAccess.isGranted(context)
    }

    /** Peut-on vérifier l'état des PC en ce moment ? Sinon on affiche « inconnu » (jamais un faux « éteint »). */
    val probeAvailability: Flow<ProbeAvailability> =
        combine(network.state, localNetworkGranted) { lan, granted ->
            when {
                !granted -> ProbeAvailability(false, UnknownReason.NO_PERMISSION)
                lan.connected || lan.vpnActive -> ProbeAvailability.AVAILABLE
                else -> ProbeAvailability(false, UnknownReason.NO_NETWORK)
            }
        }
}
