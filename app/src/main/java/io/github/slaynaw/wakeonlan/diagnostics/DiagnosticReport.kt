package io.github.slaynaw.wakeonlan.diagnostics

import android.content.Context
import android.os.Build
import io.github.slaynaw.wakeonlan.AppContainer
import io.github.slaynaw.wakeonlan.BuildConfig
import java.io.File
import java.time.Instant
import java.time.ZoneId
import java.util.Locale

/**
 * Rapport de diagnostic (texte, chiffré ensuite par un mot de passe) : versions, appareil, réseau,
 * PC et leur état, partage, mises à jour, incidents et tout le journal. Aucun secret : ni clé
 * d'agent, ni jeton GitHub, ni clé privée, ni mot de passe (seulement leur présence).
 */
object DiagnosticReport {

    fun app(): String = "Patronus Android ${BuildConfig.VERSION_NAME} (code ${BuildConfig.VERSION_CODE})"

    suspend fun build(context: Context, container: AppContainer): String = buildString {
        val now = Instant.now()
        appendLine("=== Patronus — rapport de diagnostic ===")
        appendLine("Application : ${app()}${if (BuildConfig.DEBUG) " [debug]" else ""}")
        appendLine("Appareil : ${Build.MANUFACTURER} ${Build.MODEL} (${Build.DEVICE}), Android ${Build.VERSION.RELEASE} (API ${Build.VERSION.SDK_INT})")
        appendLine("Langue : ${Locale.getDefault()}, fuseau : ${ZoneId.systemDefault()}")
        appendLine("Créé le : $now")
        runCatching {
            val info = context.packageManager.getPackageInfo(context.packageName, 0)
            appendLine("Installée le : ${Instant.ofEpochMilli(info.firstInstallTime)}, mise à jour le : ${Instant.ofEpochMilli(info.lastUpdateTime)}")
        }

        section("Réseau")
        val lan = container.network.state.value
        appendLine("Réseau local : ${if (lan.connected) "oui (${lan.transport})" else "non"}, VPN : ${if (lan.vpnActive) "oui" else "non"}")
        appendLine("Adresses : ${lan.addresses.joinToString { "${it.address.hostAddress}/${it.prefixLength}" }.ifEmpty { "aucune" }}")
        appendLine("Autorisation « réseau local » : ${if (container.localNetworkGranted.value) "accordée" else "refusée"}")

        section("PC")
        val config = runCatching { container.repository.current() }
        val statuses = container.statusMonitor.statuses.value
        config.onFailure { appendLine("Configuration illisible : ${it.javaClass.simpleName} ${it.message}") }
        config.getOrNull()?.let { c ->
            appendLine("Réglages : vérification ${c.settings.pollIntervalSeconds} s, attente du démarrage ${c.settings.wakeTimeoutSeconds} s, confirmation ${c.settings.confirmPowerActions}")
            appendLine("${c.devices.size} PC :")
            for (d in c.devices) {
                val st = statuses[d.id]
                appendLine(
                    "- « ${d.name} » [${d.id.take(8)}] MAC ${d.mac}, hôte ${d.host.ifEmpty { "-" }}, diffusion ${d.broadcastAddress ?: "auto"}, " +
                        "port WoL ${d.wolPort}, SecureOn ${if (d.secureOnPassword != null) "oui" else "non"}, sondes ${d.probePorts}, " +
                        "agent ${d.agent?.let { "port ${it.port}, clé ${if (it.hasKey) "présente" else "absente"}" } ?: "non"}",
                )
                if (st != null) {
                    appendLine(
                        "    état ${st.state} depuis ${instant(st.since)}, vu ${instant(st.lastSeen)}, via ${st.method ?: "-"}, latence ${st.latencyMs ?: "-"} ms" +
                            (st.agent?.let { ", agent ${it.version} (${it.os}/${it.arch}, ${it.hostname})" } ?: "") +
                            (st.agentError?.let { ", erreur agent $it" } ?: "") + (st.unknownReason?.let { ", raison $it" } ?: "") +
                            (st.notice?.let { ", avis $it" } ?: ""),
                    )
                }
            }
        }

        section("Partage")
        val share = container.share.state.value
        appendLine("État chargé : ${share.loaded}, connexion GitHub possible : ${share.canLogin}, publication en cours : ${share.publishing}")
        share.publishError?.let { appendLine("Dernière erreur de publication : $it") }
        share.login?.let { appendLine("Connexion en cours : code affiché${it.error?.let { e -> ", erreur : $e" } ?: ""}") }
        val owner = share.owner
        if (owner == null) {
            appendLine("Je partage mes PC : non (aucun partage enregistré)")
        } else {
            val names = config.getOrNull()?.devices?.associate { it.id to it.name }.orEmpty()
            appendLine(
                "Je partage mes PC : oui, nom « ${owner.name} », clé ${DiagnosticLog.keyId(owner.publicKey)}, compte @${owner.user.ifEmpty { "-" }}, " +
                    "gist ${owner.gist.take(8).ifEmpty { "-" }}, jeton ${if (owner.token.isNotEmpty()) "présent" else "absent"}, révision ${owner.revision}",
            )
            for (p in owner.people) {
                val rights = p.rights.entries.joinToString { (id, r) -> "${names[id] ?: "PC inconnu ${id.take(8)}"} = ${r.name.lowercase()}" }
                appendLine(
                    "- « ${p.name} » ${DiagnosticLog.keyId(p.device)}, ajoutée ${instant(p.added)}, " +
                        "publiée ${if (owner.published.containsKey(p.device)) "oui" else "non"} : $rights",
                )
            }
            if (owner.withdrawn.isNotEmpty()) appendLine("Accès retirés à supprimer du Gist : ${owner.withdrawn.size}")
        }
        appendLine("PC partagés avec moi : ${share.received.size}")
        for (a in share.received) {
            appendLine(
                "- de « ${a.ownerName} » ${DiagnosticLog.keyId(a.owner)} (@${a.user}), actif ${a.active}, retiré ${a.removed}, " +
                    "${a.devices.size} PC, révision ${a.revision}, demandé ${instant(a.requested)}, vérifié ${instant(a.synced)}" +
                    (share.syncErrors[a.owner]?.let { ", erreur : $it" } ?: ""),
            )
        }

        section("Mises à jour")
        val update = container.updater.state.value
        appendLine(
            "Activées ${update.enabled}, automatiques ${update.auto}, dernière recherche ${instant(update.lastCheck)}, " +
                "proposée ${update.available?.version ?: "-"}, étape ${update.stage}${update.error?.let { ", erreur : $it" } ?: ""}",
        )

        section("Incidents sur les données")
        val notices = DataNotices.notices.value
        appendLine(if (notices.isEmpty()) "Aucun signalé pendant cette session." else notices.joinToString())
        val kept = listOf("datastore", "history").flatMap { dir ->
            File(context.filesDir, dir).listFiles { f -> f.name.contains(".illisible-") }?.toList().orEmpty()
        }
        for (f in kept) appendLine("Copie mise de côté : ${f.parentFile?.name}/${f.name} (${f.length()} octets)")

        section("Dernier plantage non consulté")
        appendLine(CrashReporter.pending(context) ?: "Aucun.")

        val records = DiagnosticLog.records()
        section("Journal (${records.size} évènements, du plus ancien au plus récent)")
        records.forEach { appendLine(it) }
    }

    private fun StringBuilder.section(title: String) {
        appendLine()
        appendLine("=== $title ===")
    }

    private fun instant(millis: Long?): String = if (millis == null || millis <= 0) "-" else Instant.ofEpochMilli(millis).toString()
}
