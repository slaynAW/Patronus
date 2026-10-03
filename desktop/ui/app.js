// Patronus — interface Windows, thème sombre « topologie & panneau de détail ».
// Mêmes fonctions et mêmes textes que l'application Android ; toute la logique (réseau, états,
// chiffrement, historique) est dans le moteur Go, appelé par api.call().
"use strict";

(() => {
  // ---------------------------------------------------------------------------------------------
  // Textes : ceux de l'application Android (res/values/strings.xml), adaptés au PC si nécessaire.
  // ---------------------------------------------------------------------------------------------
  const S = {
    app_name: "Patronus",
    ok: "OK",
    cancel: "Annuler",
    close: "Fermer",
    later: "Plus tard",
    save: "Enregistrer",
    show: "Afficher",
    hide: "Masquer",
    password_hold: "Maintenir pour afficher le mot de passe",

    tab_overview: "Vue d’ensemble",
    tab_devices: "Appareils",
    tab_settings: "Réglages",

    action_add: "Ajouter",
    action_add_device: "Ajouter un PC",
    action_refresh: "Actualiser l’état",
    action_more: "Plus d’actions",
    action_wake: "Démarrer",
    action_wake_again: "Renvoyer",
    action_wake_menu: "Démarrer (Wake-on-LAN)",
    action_shutdown: "Éteindre",
    action_reboot: "Redémarrer",
    action_sleep: "Mettre en veille",
    action_sleep_short: "Veille",
    action_edit: "Modifier",
    action_move_up: "Monter",
    action_move_down: "Descendre",
    action_delete: "Supprimer",
    action_configure: "Configurer",
    action_paste_link: "Coller le lien",
    action_test_agent: "Tester la connexion",
    action_export: "Exporter la configuration",
    action_export_short: "Exporter",
    action_export_help: "Sauvegarde dans un fichier (PC enregistrés : %1$d)",
    action_import: "Importer une configuration",
    action_import_short: "Importer",
    action_import_help: "Restaurer une sauvegarde ou récupérer la configuration du téléphone (vous pouvez aussi glisser le fichier dans la fenêtre)",

    state_online: "Allumé",
    state_offline: "Éteint",
    state_offline_seen: "Éteint · vu il y a %1$s",
    state_offline_ago: "Éteint · il y a %1$s",
    state_unknown: "État inconnu",
    state_unknown_short: "Inconnu",
    state_checking: "Vérification…",
    state_waking: "Démarrage en cours… %1$s",
    state_waking_short: "Démarrage… %1$s",
    state_waking_name: "Démarrage en cours",
    state_shutting_down: "Arrêt en cours… %1$s",
    state_shutting_down_short: "Arrêt… %1$s",
    state_shutting_down_name: "Arrêt en cours",
    state_restarting: "Redémarrage en cours… %1$s",
    state_restarting_short: "Redémarrage… %1$s",
    state_restarting_name: "Redémarrage en cours",
    state_with_detail: "%1$s · %2$s",
    latency_ms: "%1$d ms",
    unknown_no_host: "adresse IP non renseignée",
    unknown_no_network: "pas de réseau local",

    duration_seconds: "%1$d s",
    duration_minutes: "%1$d min",
    duration_hours: "%1$d h",
    duration_days: "%1$d j",

    notice_wake_timeout: "Aucune réponse après l’envoi du paquet magique. Vérifiez que le Wake-on-LAN est activé (BIOS/UEFI et carte réseau).",
    notice_shutdown_timeout: "Le PC répond toujours après la demande d’arrêt (application bloquante ?). Essayez l’option « Forcer ».",

    agent_error_NO_KEY: "aucune clé configurée",
    agent_error_UNKNOWN_HOST: "nom d’hôte introuvable",
    agent_error_UNREACHABLE: "PC injoignable",
    agent_error_REFUSED: "agent arrêté sur le PC",
    agent_error_UNAUTHORIZED: "clé refusée par l’agent",
    agent_error_RATE_LIMITED: "trop de tentatives, réessayez dans quelques minutes",
    agent_error_PROTOCOL: "réponse invalide de l’agent",
    agent_error_REJECTED: "commande refusée par le PC",

    empty_title: "Aucun PC pour l’instant",
    empty_text: "Ajoutez un PC à réveiller. Si l’agent est installé dessus, collez simplement son lien d’appairage : tout est rempli automatiquement. Vous pouvez aussi importer la configuration exportée depuis le téléphone.",
    network_wifi: "Wi-Fi",
    network_ethernet: "Ethernet",
    network_vpn: "VPN",
    network_disconnected: "Déconnecté",
    banner_no_lan_title: "Pas de réseau local",
    banner_no_lan_text: "Connectez ce PC au réseau de la maison pour démarrer vos PC et voir leur état.",
    banner_no_lan_vpn_text: "Ce PC n’est pas sur le réseau local, mais un VPN est actif : l’état des PC est vérifié à travers le VPN. Le réveil à distance nécessite un relais sur le réseau local.",

    topology_title: "Réseau local",
    topology_this_pc: "Ce PC",
    topology_network: "Réseau",
    legend_on: "Allumé",
    legend_busy: "En cours",
    legend_off: "Éteint",
    devices_title: "Appareils",
    devices_summary: "%1$d PC · vérification toutes les %2$d s",
    col_name: "Nom",
    col_host: "Adresse IP",
    col_state: "État",
    col_mac: "Adresse MAC",
    col_system: "Système",
    col_latency: "Latence",
    capability_power: "Extinction à distance",
    capability_wake: "Démarrage uniquement",

    detail_ip: "Adresse IP",
    detail_mac: "Adresse MAC",
    detail_hostname: "Nom du PC",
    detail_system: "Système",
    detail_uptime: "Allumé depuis",
    detail_temp_cpu: "Température CPU",
    detail_temp_gpu: "Température GPU",
    detail_load_cpu: "Utilisation CPU",
    detail_load_gpu: "Utilisation GPU",
    detail_metrics: "Mesures",
    metrics_open: "Températures et utilisation dans le temps",
    metrics_title: "Mesures",
    metrics_period: "Période",
    metrics_24h: "24 dernières heures",
    metrics_7d: "7 derniers jours",
    metrics_30d: "30 derniers jours",
    metrics_90d: "90 derniers jours",
    metrics_months: "Mois archivés",
    metrics_temp: "Températures",
    metrics_load: "Utilisation",
    metrics_stats: "moy. %1$s · max %2$s",
    metrics_empty: "Aucune mesure sur cette période.",
    metrics_loading: "Chargement des mesures…",
    metrics_journal: "Journal",
    metrics_journal_empty: "Aucun évènement sur cette période.",
    metrics_sources: "Une ligne par minute (trait plein : moyenne, trait fin : maximum), lue sur le PC (90 derniers jours) ou dans les archives chiffrées GitHub.",
    metrics_coverage: "%1$s de mesures",
    temperature_gpu_shared: "intégré au CPU",
    temperature_gpu_shared_help: "%1$s est intégré au processeur et n’a pas de sonde à part : c’est la température de la puce, la même que celle du processeur.",
    temperature_cpu_lhm: "LibreHardwareMonitor requis",
    temperature_cpu_lhm_help: "Sur ce PC, lancez LibreHardwareMonitor en administrateur et activez son serveur web : Options → Remote Web Server → Run. Dans Options, cochez aussi Run On Windows Startup.",
    temperature_cpu_lhm_web: "Serveur web LHM à activer",
    temperature_cpu_lhm_web_help: "LibreHardwareMonitor tourne sur ce PC, mais son serveur web est désactivé : depuis sa version 0.9.5, c’est le seul moyen pour l’agent de le lire. Activez Options → Remote Web Server → Run (port 8085 par défaut, inutile de l’ouvrir dans le pare-feu).",
    temperature_cpu_lhm_auth: "Mot de passe LHM à retirer",
    temperature_cpu_lhm_auth_help: "Le serveur web de LibreHardwareMonitor demande un mot de passe, que l’agent ne connaît pas. Désactivez-le : Options → Remote Web Server → Authentication.",
    temperature_cpu_lhm_sensor: "Non lue par LHM",
    temperature_cpu_lhm_sensor_help: "LibreHardwareMonitor répond, mais ne lit pas le processeur : il lui manque son pilote PawnIO (proposé au démarrage de LHM 0.9.5 ou plus, acceptez-le), ou cette version de LHM ne connaît pas ce processeur (mettez-la à jour).",
    temperature_cpu_short: "CPU %1$s",
    temperature_gpu_short: "GPU %1$s",
    latency_title: "Latence",
    latency_live: "En direct",
    latency_via_agent: "via l’agent",
    latency_via_tcp: "via TCP",
    latency_via_ping: "via ping",
    latency_no_answer: "Pas de réponse",
    latency_not_measured: "Non mesurée",
    latency_waiting: "Mesure en cours…",
    latency_axis_start: "il y a 1 min",
    latency_axis_end: "maintenant",
    latency_min: "Min",
    latency_avg: "Moyenne",
    latency_max: "Max",
    latency_lost: "Pertes",
    latency_ago: "il y a %1$d s",
    latency_just_now: "à l’instant",
    latency_aria: "Latence de la dernière minute : moyenne %1$s, maximum %2$s, sans réponse %3$d sur %4$d",
    detail_agent: "Agent",
    detail_not_set: "Non renseignée",
    agent_authenticated: "Authentifié · %1$s",
    agent_not_configured: "Non installé",
    agent_configured: "Configuré",
    hint_no_agent: "Pour éteindre, redémarrer ou mettre ce PC en veille d’ici, installez l’agent sur ce PC puis appairez-le (Modifier).",
    hint_offline_agent: "Allumé mais affiché éteint ? Sur ce PC, le réseau est peut-être classé « Public » dans Windows : le pare-feu bloque alors l’agent. Paramètres Windows → Réseau et Internet → ce réseau → Type de profil réseau : Privé (l’agent 1.6.0 le propose de lui-même). Vérifiez aussi que les deux appareils sont sur le même réseau.",
    hint_unreachable: "S’il est allumé : sur ce PC, le réseau est peut-être classé « Public » dans Windows (le pare-feu bloque alors l’agent), ou les deux appareils ne sont pas sur le même réseau.",
    select_hint: "Sélectionnez un PC pour afficher son état et ses actions.",

    history_title: "Historique",
    history_show_all: "Tout afficher",
    history_empty: "Aucun évènement sur les 30 derniers jours.",
    history_subtitle: "Démarrages et extinctions · 30 derniers jours",
    history_all_devices: "Tous les PC",
    history_filter: "PC affichés",
    history_refresh: "Relire le journal des agents",
    history_more: "Afficher plus",
    history_note_outdated: "Mettez à jour l’agent de ce PC pour un historique complet, même quand cette application est fermée.",
    history_note_no_agent: "Sans agent, seuls les changements vus pendant que cette application est ouverte sont notés.",
    history_sources: "Heures relevées par l’agent de chaque PC ; « ≈ » : heure constatée par cette application.",
    history_today: "Aujourd’hui",
    history_yesterday: "Hier",
    history_yesterday_at: "hier %1$s",
    history_by: "par %1$s",
    history_source_agent: "Journal du PC (agent)",
    history_source_request: "Demandé depuis cette application",
    history_source_seen: "Constaté par cette application",
    history_source_seen_approx: "Constaté par cette application (heure approximative)",
    history_lost_help: "Coupure de courant, arrêt forcé ou plantage : heure du dernier signe de vie.",
    kind_on: "Allumé",
    kind_off: "Éteint",
    kind_lost: "Arrêt inattendu",
    kind_sleep: "Mis en veille",
    kind_resume: "Sorti de veille",
    kind_wake: "Démarrage demandé",
    kind_shutdown_req: "Extinction demandée",
    kind_reboot_req: "Redémarrage demandé",
    kind_sleep_req: "Mise en veille demandée",
    kind_wake_timeout: "Pas de réponse au démarrage",

    confirm_shutdown_title: "Éteindre « %1$s » ?",
    confirm_reboot_title: "Redémarrer « %1$s » ?",
    confirm_sleep_title: "Mettre « %1$s » en veille ?",
    confirm_power_text: "Les documents non enregistrés sur ce PC pourraient être perdus.",
    confirm_power_force: "Forcer la fermeture des applications",
    confirm_delete_title: "Supprimer « %1$s » ?",
    confirm_delete_text: "Le PC sera retiré de la liste. L’agent éventuellement installé dessus n’est pas désinstallé.",
    agent_help_title: "Agent nécessaire",
    agent_help_text: "Pour éteindre « %1$s » à distance, installez l’agent Patronus sur ce PC puis appairez-le (lien d’appairage) dans sa fiche.",

    message_wake_sent: "Paquet magique envoyé à « %1$s »",
    message_wake_error: "Échec de l’envoi : %1$s",
    message_shutdown_sent: "Extinction demandée à « %1$s »",
    message_reboot_sent: "Redémarrage demandé à « %1$s »",
    message_sleep_sent: "Mise en veille demandée à « %1$s »",
    message_agent_error: "« %1$s » : %2$s",
    message_deleted: "« %1$s » supprimé",
    message_export_done: "Configuration exportée (PC : %1$d)",
    message_import_done: "Configuration importée (PC : %1$d)",
    message_history_cleared: "Historique effacé",
    message_error: "Erreur : %1$s",

    edit_title_new: "Ajouter un PC",
    edit_title: "Modifier le PC",
    pairing_title: "Appairage rapide",
    pairing_text: "Sur le PC à contrôler, lancez « wol-agent pair » (ou l’installation de l’agent), copiez le lien « wolagent://… » affiché, puis cliquez sur « Coller le lien ».",
    pairing_ok_title: "Appairage réussi",
    pairing_ok_text: "Adresse, port et clé de l’agent ont été remplis. Vérifiez l’adresse MAC puis enregistrez.",
    pairing_error_title: "Lien d’appairage invalide",
    paste_title: "Coller le lien d’appairage",
    paste_text: "Collez le lien « wolagent://… » affiché par l’agent sur le PC.",
    section_device: "PC",
    section_agent: "Agent d’extinction",
    section_agent_help: "Petit programme installé sur le PC pour l’éteindre, le redémarrer ou le mettre en veille depuis cet ordinateur.",
    section_advanced: "Options avancées",
    field_name: "Nom",
    field_mac: "Adresse MAC",
    field_mac_help: "Celle de la carte réseau filaire, ex. AA:BB:CC:DD:EE:FF",
    field_host: "Adresse IP ou nom du PC",
    field_host_help: "Pour l’état en temps réel et l’extinction. Réservez l’IP dans votre box (bail DHCP fixe).",
    field_agent_port: "Port de l’agent",
    field_agent_key: "Clé de l’agent",
    field_agent_key_help: "Remplie automatiquement par le lien d’appairage. Ne la partagez pas.",
    field_broadcast: "Adresse de diffusion",
    field_broadcast_help: "Vide = automatique (ex. 192.168.1.255)",
    field_wol_port: "Port Wake-on-LAN",
    field_wol_port_help: "9 en général (parfois 7)",
    field_probe_ports: "Ports de détection",
    field_probe_ports_help: "Ports TCP testés pour savoir si le PC est allumé (sans agent)",
    field_secure_on: "Mot de passe SecureOn",
    field_secure_on_help: "Rarement utilisé. 6 octets hexadécimaux.",
    test_ok: "Connexion réussie : %1$s (%2$s, agent %3$s)",

    section_monitoring: "Surveillance",
    setting_poll_interval: "Vérifier l’état toutes les",
    setting_seconds_value: "%1$d s",
    setting_wake_timeout: "Attendre le démarrage jusqu’à",
    setting_confirm: "Confirmer avant d’éteindre",
    setting_confirm_help: "Demande une confirmation avant d’éteindre, redémarrer ou mettre en veille",
    section_backup: "Sauvegarde",
    section_backup_auto: "Sauvegarde automatique",
    backup_unavailable: "Indisponible dans cette version.",
    backup_enable: "Activer la sauvegarde automatique",
    backup_enable_help: "Sauvegarde complète chiffrée (PC, clés, partage, historique) après chaque changement et chaque jour, sur GitHub et/ou dans un dossier.",
    backup_enable_title: "Sauvegarde automatique",
    backup_enable_text: "Choisissez le mot de passe des sauvegardes. Il chiffre chaque sauvegarde et sera demandé pour la restaurer : notez-le en lieu sûr (gestionnaire de mots de passe). Sans lui, les sauvegardes sont illisibles.",
    backup_enable_ok: "Activer",
    backup_choose_title: "Où sauvegarder ?",
    backup_choose_text: "Sur GitHub, les sauvegardes sont dans un Gist secret de votre compte : elles vous suivent sur tous vos appareils. Dans un dossier, choisissez de préférence un dossier synchronisé (Google Drive, OneDrive…). Les deux sont possibles.",
    backup_github: "Sur GitHub",
    backup_github_off: "Non connecté : cliquez pour vous connecter (Gist secret de votre compte)",
    backup_folder: "Dans un dossier",
    backup_folder_off: "Aucun : cliquez pour en choisir un (par exemple un dossier synchronisé Google Drive ou OneDrive)",
    backup_last: "dernière sauvegarde il y a %1$s",
    backup_never: "pas encore sauvegardé",
    archive_title: "Archives des mesures",
    archive_off: "Températures et utilisation des PC minute par minute, et journal des démarrages : archivés chiffrés sur GitHub",
    archive_need_github: "Les archives utilisent la connexion GitHub des sauvegardes : cliquez pour la connecter",
    archive_running: "Archivage en cours…",
    archive_last: "dernier archivage il y a %1$s",
    archive_pending: "premier archivage dans quelques minutes",
    archive_enable_title: "Archiver les mesures sur GitHub ?",
    archive_enable_text: "Chaque PC équipé de l’agent 1.8.0 enregistre en continu ses températures et l’utilisation du processeur et de la carte graphique (une ligne par minute, 90 jours gardés sur le PC). Cette application les range, avec le journal des démarrages et arrêts, dans un Gist secret par mois de votre compte GitHub, chiffrés par le mot de passe des sauvegardes : illisibles sans lui. L’archivage a lieu toutes les 30 minutes tant que l’application est ouverte et rattrape ce que les PC ont gardé.",
    archive_enable_ok: "Activer",
    archive_dialog_text: "Les mesures de vos PC sont archivées toutes les 30 minutes dans des Gists secrets chiffrés (« Patronus – archives chiffrées AAAA-MM »). Consultez-les depuis la fiche d’un PC → Mesures.",
    archive_now: "Archiver maintenant",
    archive_done: "Archivage terminé : %1$s minute(s) ajoutée(s)",
    archive_skipped: "Non archivés pour l’instant : %1$s",
    archive_disable: "Arrêter l’archivage",
    backup_now: "Sauvegarder maintenant",
    backup_now_help: "Sauvegarde automatique après chaque changement et chaque jour ; 7 versions gardées.",
    backup_running: "Sauvegarde en cours…",
    backup_paused: "En pause (%1$s) : vérifiez vos PC, puis cliquez ici pour sauvegarder.",
    backup_done: "Sauvegarde terminée",
    backup_disable: "Désactiver la sauvegarde automatique",
    backup_disable_help: "Les sauvegardes déjà faites sont conservées.",
    backup_disable_text: "Plus aucune sauvegarde ne sera faite et le mot de passe des sauvegardes sera oublié par ce PC. Les sauvegardes existantes restent (GitHub et dossier).",
    backup_disable_ok: "Désactiver",
    backup_restore: "Restaurer depuis GitHub",
    backup_restore_help: "Retrouver vos PC, clés, partage et historique à partir d’une sauvegarde automatique.",
    backup_restore_title: "Restaurer une sauvegarde",
    backup_restore_text: "Sauvegardes du compte %1$s, de la plus récente à la plus ancienne. Le mot de passe des sauvegardes vous sera demandé.",
    backup_restore_empty: "Aucune sauvegarde sur ce compte GitHub pour le moment.",
    backup_restore_mine: "ce PC",
    backup_login_done: "GitHub connecté : les sauvegardes y seront enregistrées.",
    backup_login_reused: "Compte GitHub @%1$s du partage utilisé pour les sauvegardes.",
    backup_login_share_rejected: "GitHub n’accepte plus la connexion du partage (@%1$s) : elle a expiré ou a été révoquée. Connectez-vous avec ce compte : le partage et les sauvegardes utiliseront cette nouvelle connexion.",
    backup_github_title: "Sauvegardes sur GitHub",
    backup_github_text: "Compte %1$s. Les sauvegardes sont dans un Gist secret de ce compte, chiffrées par votre mot de passe.",
    backup_disconnect: "Déconnecter",
    backup_reconnect: "Reconnecter",
    backup_folder_title: "Dossier des sauvegardes",
    backup_folder_change: "Changer de dossier",
    backup_folder_remove: "Ne plus y sauvegarder",
    backup_folder_done: "Dossier choisi : sauvegarde en cours",
    export_title: "Exporter la configuration",
    export_with_secrets: "Complète, protégée par mot de passe",
    export_with_secrets_help: "Inclut les clés des agents, votre clé de partage et l’historique. Fichier chiffré (AES-256).",
    export_without_secrets: "Sans les clés",
    export_without_secrets_help: "Fichier lisible ; les agents devront être ré-appairés.",
    field_password: "Mot de passe",
    field_password_help: "%1$d caractères minimum. Il ne pourra pas être récupéré.",
    field_password_confirm: "Confirmer le mot de passe",
    import_password_title: "Sauvegarde protégée",
    import_wrong_password: "Mot de passe incorrect",
    import_confirm_title: "Importer la sauvegarde (PC : %1$d) ?",
    import_confirm_text: "« Remplacer » supprime la liste actuelle. « Fusionner » ajoute les PC et met à jour ceux déjà présents.",
    import_missing_keys: "Cette sauvegarde ne contient pas les clés des agents : il faudra ré-appairer les PC concernés.",
    import_replace: "Remplacer",
    import_merge: "Fusionner",
    import_too_big: "Fichier trop volumineux",
    section_history: "Historique",
    history_open: "Afficher l’historique",
    history_open_help: "Démarrages, extinctions et mises en veille des 30 derniers jours",
    history_clear: "Effacer l’historique",
    history_clear_help: "Efface les évènements notés par cette application. Le journal tenu par l’agent de chaque PC est conservé.",
    history_clear_title: "Effacer l’historique ?",
    history_clear_text: "Les évènements notés par cette application seront supprimés. Le journal tenu par l’agent de chaque PC n’est pas modifié : il sera relu à la prochaine connexion.",
    history_clear_action: "Effacer",
    section_agent_download: "Agent pour PC",
    agent_download: "Télécharger l’agent",
    agent_download_help: "Installez-le sur cet ordinateur ou enregistrez-le pour un autre PC. Nécessaire pour éteindre à distance, l’historique complet et les températures.",
    agent_dl_title: "Agent Patronus",
    agent_dl_loading: "Recherche de la dernière version…",
    agent_dl_meta: "Version %1$s · %2$s Mo",
    agent_dl_verified: "Vérifié par l’application (signature du projet, empreinte SHA-256). Installé par-dessus un agent existant, il garde sa clé : pas besoin de ré-appairer.",
    agent_dl_arch: "Processeur du PC à équiper",
    agent_dl_arch_amd64: "x64 (Intel, AMD)",
    agent_dl_arch_arm64: "ARM64 (Snapdragon…)",
    agent_dl_this_pc: "%1$s · cet ordinateur",
    agent_dl_other_os: "Linux ou macOS : page des versions",
    agent_dl_install: "Installer sur cet ordinateur",
    agent_dl_save: "Enregistrer…",
    agent_dl_close: "Fermer",
    agent_dl_retry: "Réessayer",
    agent_dl_downloading: "Téléchargement et vérification… %1$d %",
    agent_dl_started_title: "Installation lancée",
    agent_dl_started_text: "Acceptez l’invite administrateur. La fenêtre de l’agent affiche ensuite un QR code et un lien d’appairage : pour piloter cet ordinateur depuis Patronus, ajoutez-le avec « Ajouter un PC » puis « Coller le lien ». Un agent déjà installé est simplement mis à jour.",
    agent_dl_saved_title: "Agent enregistré",
    agent_dl_saved_text: "Enregistré dans %1$s. Copiez ce fichier sur le PC à équiper, double-cliquez dessus et acceptez l’invite administrateur.",
    detail_agent_update: "Mise à jour de l’agent",
    agent_update_available: "%1$s disponible",
    section_diagnostic: "Diagnostic",
    diagnostic_export: "Exporter le rapport de diagnostic",
    diagnostic_export_help: "Journal détaillé et chiffré des actions et des erreurs, à transmettre pour analyser un problème. Protégé par un mot de passe.",
    diagnostic_title: "Rapport de diagnostic",
    diagnostic_text: "Le rapport contient l’état de l’application, vos PC (noms, adresses), le partage et le journal des dernières actions, sans aucune clé ni jeton. Il est chiffré par ce mot de passe : transmettez le fichier et, séparément, le mot de passe.",
    diagnostic_save: "Enregistrer",
    message_diagnostic_done: "Rapport de diagnostic enregistré",
    share_load_error_title: "Partage illisible au démarrage",
    share_load_error: "%1$s. Importez votre dernière sauvegarde complète (Réglages → Importer) pour retrouver votre clé de partage, puis reconnectez-vous à GitHub.",
    section_about: "À propos",
    about_version: "Version",
    about_source: "Code source",
    about_security: "Sécurité",
    about_security_text: "Configuration et historique chiffrés sur ce PC (protection des données Windows), aucune donnée personnelle envoyée sur Internet (GitHub n’est contacté que pour les mises à jour et, si vous l’utilisez, le partage : fichiers chiffrés), commandes d’extinction authentifiées (HMAC-SHA256) et protégées contre le rejeu.",

    section_updates: "Mises à jour",
    update_title: "Mise à jour disponible",
    update_meta_version: "Version %1$s",
    update_meta_size: "%1$s Mo",
    update_keep_data: "Vos PC, vos réglages et l’historique sont conservés.",
    update_later: "Plus tard",
    update_install: "Mettre à jour",
    update_retry: "Réessayer",
    update_downloading: "Téléchargement… %1$d %",
    update_installing: "Installation… l’application va redémarrer.",
    update_check: "Rechercher une mise à jour",
    update_checking: "Recherche en cours…",
    update_available: "Version %1$s disponible : voir les nouveautés",
    update_up_to_date: "Vous avez la dernière version.",
    update_last_check: "À jour · dernière vérification il y a %1$s",
    update_never: "Aucune vérification pour l’instant.",
    update_auto: "Rechercher automatiquement",
    update_auto_help: "Au démarrage puis une fois par jour, sur GitHub. Rien n’est installé sans votre accord.",
    update_disabled: "Indisponible pour cette version (version de développement) : téléchargez les versions sur GitHub.",

    section_share_mine: "Partager mes PC",
    section_share_received: "PC partagés avec moi",
    share_start: "Partager mes PC",
    share_start_help: "Choisissez qui peut démarrer (ou éteindre) chacun de vos PC. Nécessite un compte GitHub gratuit.",
    share_unavailable: "Connexion GitHub non configurée dans cette version : vous pouvez seulement recevoir des accès.",
    share_setup_text: "Vos PC restent sur vos appareils. Pour chaque personne autorisée, un fichier chiffré que seul son appareil peut lire est déposé dans un espace privé de votre compte GitHub (un « Gist » secret). L’application ne reçoit que le droit de gérer ces fichiers : aucun accès à vos dépôts.",
    share_field_name: "Votre nom, affiché aux personnes invitées",
    share_connect: "Se connecter à GitHub",
    share_login_title: "Connexion à GitHub",
    share_login_text: "Sur la page GitHub qui s’ouvre, connectez-vous puis collez (ou saisissez) ce code :",
    share_login_open: "Ouvrir GitHub",
    share_login_copy: "Copier le code",
    share_login_waiting: "En attente de la validation sur GitHub…",
    share_login_done: "Partage activé : invitez maintenant une personne.",
    share_copied: "Copié dans le presse-papiers",
    share_reconnect: "Se reconnecter à GitHub",
    share_reconnect_help: "Nécessaire pour publier les accès (connexion expirée ou nouvel appareil).",
    share_account: "Compte GitHub : @%1$s",
    share_account_help: "Accès publiés, chiffrés pour chaque personne.",
    share_publishing: "Publication en cours…",
    share_invite: "Inviter une personne",
    share_invite_help: "QR code ou lien à lui envoyer (aucun secret dedans).",
    share_invite_text: "Sur son téléphone ou son PC : Patronus → Réglages → « Demander un accès », puis scanner ce QR code ou coller ce lien. Elle vous renverra ensuite sa demande.",
    share_copy_link: "Copier le lien",
    share_add_request: "Ajouter une demande d’accès",
    share_add_request_help: "Collez le lien de demande reçu de la personne.",
    share_request_paste_text: "Collez le lien « wolshare://request… » reçu de la personne.",
    share_grant_title: "Autoriser %1$s",
    share_edit_title: "Accès de %1$s",
    share_verify: "Vérifiez avec %1$s que son application affiche ce code :",
    share_verify_help: "Comparez-le de vive voix ou par téléphone. S’il est différent, la demande a été modifiée : n’autorisez pas.",
    share_rights: "PC partagés",
    share_right_none: "Non partagé",
    share_right_wake: "Démarrer",
    share_right_full: "Démarrer et éteindre",
    share_grant: "Autoriser",
    share_person_waiting: "%1$s · publication en attente",
    share_revoke: "Retirer l’accès",
    share_revoke_title: "Retirer l’accès de %1$s ?",
    share_revoke_text: "Ses PC partagés disparaîtront de son application à sa prochaine vérification. S’il pouvait éteindre certains PC, changez aussi la clé de leur agent.",
    share_stop: "Arrêter le partage",
    share_stop_help: "Supprime l’espace de partage : tous les accès sont retirés.",
    share_stop_title: "Arrêter le partage ?",
    share_stop_text: "L’espace de partage sera supprimé de GitHub : toutes les personnes autorisées perdront l’accès à vos PC.",
    share_granted: "Accès accordé à %1$s",
    share_revoked: "Accès retiré à %1$s",
    share_no_devices: "Ajoutez d’abord un PC à partager.",
    share_request: "Demander un accès",
    share_request_help: "Accédez aux PC d’une autre personne, avec son accord.",
    share_invite_paste_text: "Collez le lien d’invitation « wolshare://invite… » reçu de la personne qui partage.",
    share_field_my_name: "Votre nom, affiché à %1$s",
    share_request_title: "Demande envoyée à %1$s",
    share_request_text: "Envoyez ce lien à %1$s (message, e-mail), ou montrez-lui ce QR code. Il ne contient aucun secret.",
    share_request_code: "Code de vérification à comparer avec %1$s :",
    share_request_wait: "Les PC apparaîtront dans la liste dès que %1$s aura autorisé l’accès.",
    share_access_title: "PC de %1$s",
    share_access_pending: "En attente de l’autorisation de %1$s",
    share_access_active: "%1$s · vérifié il y a %2$s",
    share_access_removed: "Accès retiré par %1$s",
    share_access_devices: "PC reçus : %1$s",
    share_access_none: "Aucun PC pour l’instant.",
    share_show_request: "Afficher ma demande",
    share_sync_now: "Vérifier maintenant",
    share_leave: "Quitter ce partage",
    share_leave_title: "Quitter le partage de %1$s ?",
    share_leave_text: "Ses PC disparaîtront de cette application. Pour y accéder à nouveau, il faudra une nouvelle invitation.",
    share_cancel_request: "Annuler la demande",
    share_remove: "Retirer de la liste",
    share_by: "Partagé par %1$s",
    detail_shared: "Partagé par",
    hint_shared: "PC partagé par %1$s : lui seul peut le modifier.",
    import_sharing: "Contient aussi votre partage (%1$s, personnes autorisées : %2$d), repris si cet ordinateur ne partage pas déjà ses PC.",
    import_history: "Contient aussi l’historique (%1$d évènements), ajouté à celui de cet ordinateur.",
    import_sharing_done: "Votre partage a été repris. Reconnectez-vous à GitHub (Réglages → Partager mes PC) avec le même compte pour que les accès continuent.",
  };

  const REPO_URL = "https://github.com/slaynAW/Patronus";
  const GITHUB_DEVICE_URL = "https://github.com/login/device";
  const RELEASES_URL = REPO_URL + "/releases";
  const MIN_PASSWORD_LENGTH = 8;
  const MAX_IMPORT_BYTES = 1024 * 1024;
  const SIDE_HISTORY_COUNT = 6;
  const HISTORY_PAGE = 300;

  /** Remplace %1$s, %2$d… comme String.format côté Android. */
  function fmt(template, ...args) {
    return template.replace(/%(\d)\$[sd]/g, (_, i) => String(args[Number(i) - 1]));
  }

  // ---------------------------------------------------------------------------------------------
  // Icônes (traits, 24 × 24)
  // ---------------------------------------------------------------------------------------------
  const SVG_NS = "http://www.w3.org/2000/svg";
  const ICONS = {
    power: '<path d="M12 3.5v7.5"/><path d="M6.7 6.9a7.5 7.5 0 1 0 10.6 0"/>',
    topology: '<rect x="9" y="3" width="6" height="5" rx="1.2"/><rect x="3" y="16" width="6" height="5" rx="1.2"/><rect x="15" y="16" width="6" height="5" rx="1.2"/><path d="M12 8v4M6 16v-2.5h12V16"/>',
    list: '<path d="M9 6h11M9 12h11M9 18h11"/><circle cx="4.5" cy="6" r=".8"/><circle cx="4.5" cy="12" r=".8"/><circle cx="4.5" cy="18" r=".8"/>',
    settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    refresh: '<path d="M20.5 12a8.5 8.5 0 1 1-2.5-6l2.5 2.5"/><path d="M20.5 3.5v5h-5"/>',
    restart: '<path d="M3.5 12a8.5 8.5 0 1 0 2.5-6L3.5 8.5"/><path d="M3.5 3.5v5h5"/>',
    moon: '<path d="M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2.5v2M12 19.5v2M4.6 4.6 6 6M18 18l1.4 1.4M2.5 12h2M19.5 12h2M4.6 19.4 6 18M18 6l1.4-1.4"/>',
    edit: '<path d="M4 20h4L19 9a2.1 2.1 0 0 0-3-3L5 17z"/><path d="M14 8l2 2"/>',
    shield: '<path d="M12 3l7.5 3v5.5c0 4.5-3.2 8-7.5 9.5-4.3-1.5-7.5-5-7.5-9.5V6z"/><path d="M8.8 12l2.2 2.2 4.2-4.4"/>',
    monitor: '<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8.5 20h7M12 16v4"/>',
    hub: '<rect x="2.5" y="8" width="19" height="8" rx="2"/><path d="M6 12h.01M9 12h.01M12 12h.01M15 12h.01M18 12h1"/>',
    wifi: '<path d="M12 19.5h.01"/><path d="M8.6 16a4.8 4.8 0 0 1 6.8 0"/><path d="M5.5 12.9a9.2 9.2 0 0 1 13 0"/><path d="M2.5 9.7a13.4 13.4 0 0 1 19 0"/>',
    chevron: '<path d="M9 5l7 7-7 7"/>',
    down: '<path d="M6 9l6 6 6-6"/>',
    up: '<path d="M6 15l6-6 6 6"/>',
    arrowUp: '<path d="M12 19V5M6 11l6-6 6 6"/>',
    arrowDown: '<path d="M12 5v14M6 13l6 6 6-6"/>',
    more: '<circle cx="5.5" cy="12" r="1.2"/><circle cx="12" cy="12" r="1.2"/><circle cx="18.5" cy="12" r="1.2"/>',
    delete: '<path d="M4 7h16M9.5 7V4.5h5V7M6.5 7l1 13h9l1-13"/><path d="M10 11v5M14 11v5"/>',
    close: '<path d="M6 6l12 12M18 6 6 18"/>',
    warning: '<path d="M12 3.5 2.5 20h19z"/><path d="M12 10v4.5M12 17.5h.01"/>',
    check: '<circle cx="12" cy="12" r="9"/><path d="M8.5 12.2l2.4 2.4 4.8-5"/>',
    paste: '<rect x="5" y="4.5" width="14" height="16.5" rx="2"/><path d="M9 4.5V3h6v1.5M9 11h6M9 15h4"/>',
    history: '<path d="M3.5 12a8.5 8.5 0 1 0 2.6-6.1"/><path d="M3.5 4.5v4h4"/><path d="M12 7.5V12l3 2"/>',
    download: '<path d="M12 4v11M7 10.5l5 5 5-5"/><path d="M4.5 19.5h15"/>',
    upload: '<path d="M12 16V5M7 9.5l5-5 5 5"/><path d="M4.5 19.5h15"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5.5M12 7.5h.01"/>',
    code: '<path d="M8.5 7 3.5 12l5 5M15.5 7l5 5-5 5"/>',
    lock: '<rect x="5" y="10.5" width="14" height="10" rx="2"/><path d="M8 10.5V7.5a4 4 0 0 1 8 0v3"/>',
    clock: '<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/>',
    bolt: '<path d="M13 2.5 4.5 13.5H12l-1 8 8.5-11H12z"/>',
    thermometer: '<path d="M14 14.8V4.5a2 2 0 0 0-4 0v10.3a4 4 0 1 0 4 0z"/><path d="M12 9.5v7.5"/>',
    send: '<path d="M21 3 10.5 13.5"/><path d="M21 3l-6.5 18-4-7.5L3 9.5z"/>',
    share: '<circle cx="18" cy="5.5" r="2.5"/><circle cx="6" cy="12" r="2.5"/><circle cx="18" cy="18.5" r="2.5"/><path d="M8.2 10.8l7.6-4.1M8.2 13.2l7.6 4.1"/>',
    users: '<circle cx="9" cy="8" r="3.5"/><path d="M2.5 20a6.5 6.5 0 0 1 13 0"/><path d="M16 4.6a3.5 3.5 0 0 1 0 6.8M18.5 14.2a6.5 6.5 0 0 1 3 5.8"/>',
    copy: '<rect x="8.5" y="8.5" width="12" height="12" rx="2"/><path d="M15.5 8.5v-3a2 2 0 0 0-2-2h-8a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h3"/>',
    cloud: '<path d="M7 18.5h10.5a4 4 0 0 0 .6-8 6 6 0 0 0-11.6-1.4A4.8 4.8 0 0 0 7 18.5z"/>',
    chart: '<path d="M3.5 20.5h17"/><path d="M4.5 16.5l4.5-5 3.5 3 6.5-8"/>',
    eye: '<path d="M2.5 12s3.5-6.5 9.5-6.5 9.5 6.5 9.5 6.5-3.5 6.5-9.5 6.5S2.5 12 2.5 12z"/><circle cx="12" cy="12" r="2.8"/>',
  };

  function icon(name, cls = "", size = 0) {
    const svg = document.createElementNS(SVG_NS, "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("fill", "none");
    svg.setAttribute("stroke", "currentColor");
    svg.setAttribute("stroke-width", "1.7");
    svg.setAttribute("stroke-linecap", "round");
    svg.setAttribute("stroke-linejoin", "round");
    svg.setAttribute("class", ("icon " + cls).trim());
    svg.setAttribute("aria-hidden", "true");
    if (size) {
      svg.style.width = size + "px";
      svg.style.height = size + "px";
    }
    svg.innerHTML = ICONS[name] || ""; // dessins fixes ci-dessus, jamais de données
    return svg;
  }

  // Logo « Topologie » (source : branding/logo.svg ; variante simplifiée pour les petites tailles).
  const LOGO = `<rect x="0.75" y="0.75" width="118.5" height="118.5" rx="26.5" fill="#12161B" stroke="#262B33" stroke-width="1.5"/>
<path d="M60 76 L28 42 M60 76 L60 34" fill="none" stroke="#3A4250" stroke-width="4" stroke-linecap="round"/>
<path d="M60 76 L92 42" fill="none" stroke="#4A94FF" stroke-width="4" stroke-linecap="round"/>
<circle cx="28" cy="42" r="8" fill="#12161B" stroke="#6B7380" stroke-width="4"/>
<circle cx="60" cy="34" r="8" fill="#12161B" stroke="#6B7380" stroke-width="4"/>
<circle cx="92" cy="42" r="15" fill="#4A94FF" fill-opacity="0.22"/>
<circle cx="92" cy="42" r="9" fill="#4A94FF"/>
<rect x="42" y="70" width="36" height="20" rx="6" fill="#E7E9EC"/>
<circle cx="52" cy="80" r="2.5" fill="#2FD27A"/>`;
  const LOGO_SMALL = `<rect x="0.75" y="0.75" width="118.5" height="118.5" rx="26.5" fill="#12161B" stroke="#262B33" stroke-width="1.5"/>
<path d="M60 76 L28 42 M60 76 L60 34" fill="none" stroke="#4A5260" stroke-width="7" stroke-linecap="round"/>
<path d="M60 76 L92 42" fill="none" stroke="#4A94FF" stroke-width="7" stroke-linecap="round"/>
<circle cx="28" cy="42" r="9" fill="#6B7380"/>
<circle cx="60" cy="34" r="9" fill="#6B7380"/>
<circle cx="92" cy="42" r="12" fill="#4A94FF"/>
<rect x="40" y="68" width="40" height="24" rx="7" fill="#E7E9EC"/>`;

  function logo(size) {
    const svg = document.createElementNS(SVG_NS, "svg");
    svg.setAttribute("viewBox", "0 0 120 120");
    svg.setAttribute("width", String(size));
    svg.setAttribute("height", String(size));
    svg.setAttribute("class", "logo");
    svg.setAttribute("aria-hidden", "true");
    svg.innerHTML = size <= 32 ? LOGO_SMALL : LOGO; // dessins fixes ci-dessus, jamais de données
    return svg;
  }

  // ---------------------------------------------------------------------------------------------
  // Construction du DOM
  // ---------------------------------------------------------------------------------------------
  function h(tag, props, ...children) {
    const el = document.createElement(tag);
    for (const [key, value] of Object.entries(props || {})) {
      if (value == null || value === false) continue;
      if (key === "class") el.className = value;
      else if (key === "text") el.textContent = value;
      else if (key.startsWith("on") && typeof value === "function") el.addEventListener(key.slice(2).toLowerCase(), value);
      else if (key === "value" || key === "checked" || key === "disabled") el[key] = value;
      else el.setAttribute(key, value === true ? "" : String(value));
    }
    append(el, children);
    return el;
  }

  function append(el, children) {
    for (const child of children.flat(Infinity)) {
      if (child == null || child === false) continue;
      el.append(child instanceof Node ? child : document.createTextNode(String(child)));
    }
  }

  const setText = (el, text) => {
    if (el.textContent !== text) el.textContent = text;
  };
  const setClass = (el, cls) => {
    if (el.className !== cls) el.className = cls;
  };

  function btn(kind, label, onClick, iconName, extra = {}) {
    return h("button", { class: "btn " + kind, type: "button", onClick, ...extra }, iconName ? icon(iconName, "small") : null, label);
  }

  function iconBtn(name, label, onClick, cls = "") {
    return h("button", { class: ("icon-btn " + cls).trim(), type: "button", title: label, "aria-label": label, onClick }, icon(name));
  }

  /** Entrée / espace sur un élément focalisable qui se comporte comme un bouton. */
  const activate = (f) => (e) => {
    if ((e.key === "Enter" || e.key === " ") && e.target === e.currentTarget) {
      e.preventDefault();
      f();
    }
  };

  /** Clic qui ne remonte pas à la ligne (qui, elle, sélectionne le PC). */
  const own = (f) => (e) => {
    e.stopPropagation();
    f(e);
  };

  /** Met à jour une liste d'éléments identifiés par id, dans l'ordre donné, sans tout reconstruire. */
  function syncList(container, map, items, create, update) {
    const seen = new Set();
    items.forEach((item, i) => {
      let entry = map.get(item.id);
      if (!entry) {
        entry = create();
        map.set(item.id, entry);
      }
      update(entry, item, i);
      seen.add(item.id);
      if (container.children[i] !== entry.el) container.insertBefore(entry.el, container.children[i] || null);
    });
    for (const [id, entry] of map) {
      if (!seen.has(id)) {
        entry.el.remove();
        map.delete(id);
      }
    }
  }

  // ---------------------------------------------------------------------------------------------
  // Communication avec le moteur Go
  // ---------------------------------------------------------------------------------------------
  const api = (() => {
    const listeners = new Set();
    const emit = (s) => listeners.forEach((f) => f(s));
    if (typeof window.goInvoke === "function") {
      // Application Windows (WebView2) : appels asynchrones, réponses via __goResolve.
      let seq = 0;
      const pending = new Map();
      window.__goResolve = (id, ok, payload) => {
        const p = pending.get(id);
        if (!p) return;
        pending.delete(id);
        if (ok) p.resolve(payload);
        else p.reject(new Error(payload));
      };
      window.__goState = (s) => emit(s);
      return {
        call: (method, params = {}) => new Promise((resolve, reject) => {
          const id = ++seq;
          pending.set(id, { resolve, reject });
          Promise.resolve(window.goInvoke(id, method, JSON.stringify(params))).catch((e) => {
            pending.delete(id);
            reject(e instanceof Error ? e : new Error(String(e)));
          });
        }),
        onState: (f) => listeners.add(f),
      };
    }
    // Mode développement : petit serveur HTTP local (voir main_other.go).
    const token = new URLSearchParams(location.hash.slice(1)).get("token") || "";
    const events = new EventSource("/events?token=" + encodeURIComponent(token));
    events.onmessage = (e) => emit(JSON.parse(e.data));
    return {
      call: async (method, params = {}) => {
        const res = await fetch("/rpc", {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Token": token },
          body: JSON.stringify({ method, params }),
        });
        const data = await res.json();
        if (!data.ok) throw new Error(data.error || "erreur inconnue");
        return data.result;
      },
      onState: (f) => listeners.add(f),
    };
  })();

  // Erreurs de la page : notées dans le journal de diagnostic chiffré du moteur (nombre limité).
  window.addEventListener("error", (e) => {
    const where = e.filename ? ` (${e.filename.slice(0, 80)}:${e.lineno}:${e.colno})` : ` (ligne ${e.lineno}:${e.colno})`;
    logClient("error", `${e.message}${where}\n${e.error?.stack || ""}`);
  });
  window.addEventListener("unhandledrejection", (e) => {
    const r = e.reason;
    logClient("error", "promesse rejetée : " + (r instanceof Error ? `${r.message}\n${r.stack || ""}` : String(r)));
  });

  function logClient(level, text) {
    try {
      api.call("logClient", { level, text: String(text).slice(0, 8000) }).catch(() => {});
    } catch {
      // Moteur indisponible : rien à faire.
    }
  }

  // ---------------------------------------------------------------------------------------------
  // Petits utilitaires d'affichage (Texts.kt)
  // ---------------------------------------------------------------------------------------------
  const isTransitional = (st) => st === "WAKING" || st === "SHUTTING_DOWN" || st === "RESTARTING";

  function formatDuration(millis) {
    const seconds = Math.max(0, Math.floor(millis / 1000));
    if (seconds < 60) return fmt(S.duration_seconds, seconds);
    if (seconds < 3600) return fmt(S.duration_minutes, Math.floor(seconds / 60));
    if (seconds < 86400) return fmt(S.duration_hours, Math.floor(seconds / 3600));
    return fmt(S.duration_days, Math.floor(seconds / 86400));
  }

  /** Durée détaillée : « 3 h 12 min », « 2 j 4 h ». */
  function formatLong(totalSeconds) {
    const s = Math.max(0, Math.floor(totalSeconds));
    const days = Math.floor(s / 86400);
    const hours = Math.floor((s % 86400) / 3600);
    const minutes = Math.floor((s % 3600) / 60);
    if (days > 0) return hours > 0 ? `${days} j ${hours} h` : `${days} j`;
    if (hours > 0) return minutes > 0 ? `${hours} h ${minutes} min` : `${hours} h`;
    if (minutes > 0) return `${minutes} min`;
    return `${s} s`;
  }

  const osLabel = (os) => ({ windows: "Windows", linux: "Linux", darwin: "macOS" })[os] || os || "";
  const archLabel = (arch) => ({ amd64: "x64", "386": "x86", arm64: "ARM64", arm: "ARM" })[arch] || arch || "";

  // Températures envoyées par l'agent (1.5.0 ou plus) : « 54 °C », orange dès 80 °C, rouge dès 90 °C.
  const TEMP_WARM = 80;
  const TEMP_HOT = 90;
  const celsius = (value) => `${Math.round(value)}\u00a0°C`;
  const tempClass = (value) => (value >= TEMP_HOT ? "temp-hot" : value >= TEMP_WARM ? "temp-warm" : null);
  /** Texte court et explication quand LibreHardwareMonitor ne donne pas la température du processeur. */
  const lhmHint = (state) => ({
    "web-off": [S.temperature_cpu_lhm_web, S.temperature_cpu_lhm_web_help],
    auth: [S.temperature_cpu_lhm_auth, S.temperature_cpu_lhm_auth_help],
    "no-sensor": [S.temperature_cpu_lhm_sensor, S.temperature_cpu_lhm_sensor_help],
  })[state] || [S.temperature_cpu_lhm, S.temperature_cpu_lhm_help];
  const percentText = (value) => `${Math.round(value)}\u00a0%`;
  /** « 54 °C (23 %) », « 54 °C » ou « 23 % » ; texte vide si rien n'est connu. */
  const sensorText = (temp, load) =>
    temp != null && load != null ? `${celsius(temp)} (${percentText(load)})` : temp != null ? celsius(temp) : load != null ? percentText(load) : "";
  /** « CPU 54 °C (23 %) · GPU 61 °C (41 %) » et la classe de la plus chaude ; texte vide si rien n'est connu. */
  function temperatureSummary(t) {
    const parts = [];
    const cpu = sensorText(t?.cpu, t?.cpuLoad);
    if (cpu) parts.push(fmt(S.temperature_cpu_short, cpu));
    // Puce graphique intégrée : même température que le processeur, pas répétée.
    const gpu = sensorText(t?.gpuShared && t?.cpu != null ? null : t?.gpu, t?.gpuLoad);
    if (gpu) parts.push(fmt(S.temperature_gpu_short, gpu));
    const values = [t?.cpu, t?.gpu].filter((v) => v != null);
    return { text: parts.join(" · "), cls: values.length ? tempClass(Math.max(...values)) : null };
  }
  const versionLabel = (v) => (/^\d/.test(v || "") ? "v" + v : v || "");
  /** Compare deux versions « X.Y.Z » (suffixe « -dev.N » ignoré) ; null si l'une n'est pas numérotée. */
  function compareVersions(a, b) {
    const parse = (v) => (/^(\d+)\.(\d+)\.(\d+)/.exec(v || "") || []).slice(1).map(Number);
    const x = parse(a);
    const y = parse(b);
    if (x.length !== 3 || y.length !== 3) return null;
    for (let i = 0; i < 3; i++) if (x[i] !== y[i]) return x[i] - y[i];
    return 0;
  }
  const agentErrorLabel = (code) => S["agent_error_" + code] || S.agent_error_PROTOCOL;
  const actionLabel = (a) => ({ shutdown: S.action_shutdown, reboot: S.action_reboot, sleep: S.action_sleep })[a];
  const confirmTitle = (a) => ({ shutdown: S.confirm_shutdown_title, reboot: S.confirm_reboot_title, sleep: S.confirm_sleep_title })[a];
  const sentMessage = (a) => ({ shutdown: S.message_shutdown_sent, reboot: S.message_reboot_sent, sleep: S.message_sleep_sent })[a];
  const unknownLabel = (r) => ({ NO_HOST: S.unknown_no_host, NO_NETWORK: S.unknown_no_network })[r];

  function stateName(st) {
    return {
      ONLINE: S.state_online, OFFLINE: S.state_offline, UNKNOWN: S.state_unknown, WAKING: S.state_waking_name,
      SHUTTING_DOWN: S.state_shutting_down_name, RESTARTING: S.state_restarting_name,
    }[st] || S.state_unknown;
  }

  /** Texte complet de l'état (panneau de détail). */
  function statusText(status, now) {
    const sinceAction = now - (status.actionStartedAt ?? now);
    switch (status.state) {
      case "ONLINE":
        return status.latencyMs != null ? fmt(S.state_with_detail, S.state_online, fmt(S.latency_ms, status.latencyMs)) : S.state_online;
      case "OFFLINE":
        return status.lastSeen != null ? fmt(S.state_offline_seen, formatDuration(now - status.lastSeen)) : S.state_offline;
      case "WAKING":
        return fmt(S.state_waking, formatDuration(sinceAction));
      case "SHUTTING_DOWN":
        return fmt(S.state_shutting_down, formatDuration(sinceAction));
      case "RESTARTING":
        return fmt(S.state_restarting, formatDuration(sinceAction));
      default:
        return status.unknownReason ? fmt(S.state_with_detail, S.state_unknown, unknownLabel(status.unknownReason)) : S.state_checking;
    }
  }

  /** Texte court de l'état (plan du réseau, listes). */
  function shortState(status, now) {
    const sinceAction = now - (status.actionStartedAt ?? now);
    switch (status.state) {
      case "ONLINE":
        return S.state_online;
      case "OFFLINE":
        return status.lastSeen != null ? fmt(S.state_offline_ago, formatDuration(now - status.lastSeen)) : S.state_offline;
      case "WAKING":
        return fmt(S.state_waking_short, formatDuration(sinceAction));
      case "SHUTTING_DOWN":
        return fmt(S.state_shutting_down_short, formatDuration(sinceAction));
      case "RESTARTING":
        return fmt(S.state_restarting_short, formatDuration(sinceAction));
      default:
        return status.unknownReason ? S.state_unknown_short : S.state_checking;
    }
  }

  const dotClass = (st) => "dot" + (st === "ONLINE" ? " glow" : "") + (isTransitional(st) ? " pulse" : "");

  function networkLabel(net) {
    if (!net.connected) return net.vpn ? S.network_vpn : S.network_disconnected;
    return net.transport === "wifi" ? S.network_wifi : S.network_ethernet;
  }

  // Dates de l'historique.
  const clockFmt = new Intl.DateTimeFormat("fr-FR", { hour: "2-digit", minute: "2-digit" });
  const shortDateFmt = new Intl.DateTimeFormat("fr-FR", { day: "numeric", month: "short" });
  const longDateFmt = new Intl.DateTimeFormat("fr-FR", { weekday: "long", day: "numeric", month: "long" });
  const fullFmt = new Intl.DateTimeFormat("fr-FR", { dateStyle: "full", timeStyle: "medium" });

  function startOfDay(ms, offsetDays = 0) {
    const d = new Date(ms);
    return new Date(d.getFullYear(), d.getMonth(), d.getDate() + offsetDays).getTime();
  }

  function eventTime(ms, now) {
    const clock = clockFmt.format(ms);
    if (ms >= startOfDay(now)) return clock;
    if (ms >= startOfDay(now, -1)) return fmt(S.history_yesterday_at, clock);
    return shortDateFmt.format(ms) + " " + clock;
  }

  function dayLabel(ms, now) {
    if (ms >= startOfDay(now)) return S.history_today;
    if (ms >= startOfDay(now, -1)) return S.history_yesterday;
    return longDateFmt.format(ms);
  }

  const KIND_ICONS = {
    on: "power", off: "power", lost: "bolt", sleep: "moon", resume: "sun", wake: "send",
    shutdown_req: "power", reboot_req: "restart", sleep_req: "moon", wake_timeout: "clock",
  };
  const REQUEST_KINDS = new Set(["wake", "shutdown_req", "reboot_req", "sleep_req"]);

  function eventTooltip(e) {
    let source;
    if (e.source === "agent") source = S.history_source_agent;
    else if (REQUEST_KINDS.has(e.kind)) source = S.history_source_request;
    else source = e.approx ? S.history_source_seen_approx : S.history_source_seen;
    const lines = [fullFmt.format(e.time), source];
    if (e.kind === "lost") lines.push(S.history_lost_help);
    return lines.join("\n");
  }

  /** Ligne d'historique : icône, libellé (« · par Pixel 8 » pour une demande venue d'ailleurs), heure. */
  function eventRow(e, now, { withName = false, clockOnly = false } = {}) {
    const by = e.client ? fmt(S.history_by, e.client) : "";
    const when = (e.approx ? "≈ " : "") + (clockOnly ? clockFmt.format(e.time) : eventTime(e.time, now));
    return h("div", { class: "ev k-" + e.kind, title: eventTooltip(e) },
      h("span", { class: "ic" }, icon(KIND_ICONS[e.kind] || "info")),
      h("span", { class: "lbl" }, S["kind_" + e.kind] || e.kind, by ? h("small", { text: " · " + by }) : null),
      withName ? h("span", { class: "who", text: e.name }) : null,
      h("time", { datetime: new Date(e.time).toISOString(), text: when }));
  }

  // ---------------------------------------------------------------------------------------------
  // Superpositions : menus, dialogues, panneaux, snackbar
  // ---------------------------------------------------------------------------------------------
  const overlays = document.getElementById("overlays");
  const stack = [];

  function closeTop() {
    const top = stack[stack.length - 1];
    if (top) top.dismiss();
    return !!top;
  }

  function closeMenus() {
    for (const entry of [...stack]) if (entry.kind === "menu") entry.dismiss();
  }

  /** items : [{label, icon, onClick, danger}] ou "sep". */
  function openMenu(anchor, items) {
    closeMenus();
    const menu = h("div", { class: "menu", role: "menu" });
    const entry = { kind: "menu", dismiss: close };
    for (const item of items) {
      if (item === "sep") {
        menu.append(h("div", { class: "menu-sep" }));
        continue;
      }
      menu.append(h("button", {
        class: "menu-item" + (item.danger ? " danger" : ""), type: "button", role: "menuitem",
        onClick: (e) => {
          e.stopPropagation();
          close();
          item.onClick();
        },
      }, item.icon ? icon(item.icon, "small") : h("span", { class: "no-icon" }), item.label));
    }
    overlays.append(menu);
    const r = anchor.getBoundingClientRect();
    const width = menu.offsetWidth;
    const height = menu.offsetHeight;
    let top = r.bottom + 4;
    if (top + height > window.innerHeight - 8) top = Math.max(8, r.top - height - 4);
    const left = Math.min(Math.max(8, r.right - width), window.innerWidth - width - 8);
    menu.style.top = top + "px";
    menu.style.left = left + "px";
    const onDown = (e) => {
      if (!menu.contains(e.target)) close();
    };
    setTimeout(() => document.addEventListener("mousedown", onDown, true));
    window.addEventListener("resize", close);
    document.addEventListener("scroll", close, true);
    stack.push(entry);
    menu.querySelector(".menu-item")?.focus();
    function close() {
      const i = stack.indexOf(entry);
      if (i < 0) return;
      stack.splice(i, 1);
      menu.remove();
      document.removeEventListener("mousedown", onDown, true);
      window.removeEventListener("resize", close);
      document.removeEventListener("scroll", close, true);
    }
  }

  /**
   * Ouvre un dialogue. actions : [{label, onClick, disabled, kind}] (la dernière est l'action
   * principale) ; onDismiss : clic hors du dialogue ou Échap.
   */
  function openDialog({ iconName, title, body, actions = [], onDismiss }) {
    const bodyEl = h("div", { class: "d-body selectable" }, body);
    const actionsEl = h("div", { class: "d-actions" });
    const dialog = h("div", { class: "dialog", role: "dialog", "aria-modal": "true", "aria-label": title || null },
      iconName ? h("div", { class: "d-icon" }, icon(iconName)) : null,
      title ? h("h2", { text: title }) : null,
      bodyEl,
      actionsEl);
    const wrap = h("div", { class: "dialog-wrap" }, dialog);
    const entry = { kind: "dialog", dismiss };
    const handle = { close, body: bodyEl, setActions };
    setActions(actions);
    wrap.addEventListener("mousedown", (e) => {
      if (e.target === wrap) dismiss();
    });
    closeMenus();
    stack.push(entry);
    overlays.append(wrap);
    const focusTarget = bodyEl.querySelector("input:not([type=checkbox]):not([type=radio]), textarea") ||
      actionsEl.querySelector("button:last-child");
    focusTarget?.focus();
    return handle;

    function setActions(list) {
      actionsEl.replaceChildren(...list.map((a, i) => h("button", {
        type: "button",
        class: "btn " + (a.kind || (i === list.length - 1 ? "primary" : "sec")),
        disabled: !!a.disabled,
        onClick: a.onClick,
      }, a.label)));
    }
    function dismiss() {
      close();
      onDismiss?.();
    }
    function close() {
      const i = stack.indexOf(entry);
      if (i < 0) return;
      stack.splice(i, 1);
      wrap.remove();
    }
  }

  function alertDialog(title, text) {
    const d = openDialog({ title, body: h("p", { text }), actions: [{ label: S.ok, onClick: () => d.close() }] });
    return d;
  }

  /** Panneau latéral (fiche d'un PC, historique complet). */
  function openSheet({ title, subtitle, headExtra = [], body, foot, onClose, closeOnScrim = true }) {
    const scrim = h("div", { class: "scrim" });
    const titleEl = h("h2", {}, title, subtitle ? h("small", { text: subtitle }) : null);
    const bodyEl = h("div", { class: "sheet-body" }, body);
    const sheet = h("div", { class: "sheet", role: "dialog", "aria-modal": "true", "aria-label": title },
      h("div", { class: "sheet-head" }, titleEl, headExtra, iconBtn("close", S.close, () => close(), "flat")),
      bodyEl,
      foot ? h("div", { class: "sheet-foot" }, foot) : null);
    const entry = { kind: "sheet", dismiss: close };
    if (closeOnScrim) scrim.addEventListener("mousedown", () => close());
    closeMenus();
    stack.push(entry);
    overlays.append(scrim, sheet);
    return { el: sheet, body: bodyEl, close };

    function close() {
      const i = stack.indexOf(entry);
      if (i < 0) return;
      stack.splice(i, 1);
      scrim.remove();
      sheet.remove();
      onClose?.();
    }
  }

  const snackbarEl = document.getElementById("snackbar");
  const snackQueue = [];
  let snackShowing = false;

  function snackbar(text) {
    snackQueue.push(text);
    if (!snackShowing) nextSnack();
  }

  function nextSnack() {
    const text = snackQueue.shift();
    if (text == null) {
      snackShowing = false;
      return;
    }
    snackShowing = true;
    snackbarEl.textContent = text;
    snackbarEl.classList.add("visible");
    setTimeout(() => {
      snackbarEl.classList.remove("visible");
      setTimeout(nextSnack, 200);
    }, 4000);
  }

  let fieldSeq = 0;

  /**
   * Bouton « œil » d'un mot de passe : affiché en clair tant qu'on maintient l'appui (souris, doigt,
   * ou Espace / Entrée au clavier), masqué dès qu'on relâche. Le curseur reste dans le champ.
   */
  function revealButton(input) {
    const button = h("button", { type: "button", class: "reveal", title: S.password_hold, "aria-label": S.password_hold }, icon("eye"));
    const show = (on) => {
      input.type = on ? "text" : "password";
      button.classList.toggle("on", on);
    };
    button.addEventListener("pointerdown", (e) => {
      e.preventDefault(); // garde le focus (et le curseur) dans le champ
      button.setPointerCapture?.(e.pointerId);
      show(true);
    });
    for (const ev of ["pointerup", "pointercancel", "lostpointercapture"]) button.addEventListener(ev, () => show(false));
    button.addEventListener("keydown", (e) => {
      if (e.key === " " || e.key === "Enter") {
        e.preventDefault();
        show(true);
      }
    });
    button.addEventListener("keyup", () => show(false));
    button.addEventListener("blur", () => show(false));
    return button;
  }

  function field({ label, helper, value = "", type = "text", mono = false, onInput, onEnter, trailing, multiline = false, placeholder }) {
    const id = "f" + ++fieldSeq;
    const input = multiline
      ? h("textarea", { id, rows: 3, spellcheck: "false", placeholder })
      : h("input", { id, type, spellcheck: "false", autocomplete: "off", class: mono ? "mono" : null, placeholder });
    input.value = value;
    // Mot de passe sans bouton propre : œil à maintenir pour le voir.
    if (type === "password" && !trailing && !multiline) trailing = revealButton(input);
    const support = h("div", { class: "support" });
    const reveal = trailing?.classList?.contains("reveal");
    const wrap = h("div", { class: `field${trailing ? (reveal ? " has-reveal" : " has-trailing") : ""}` },
      label ? h("label", { for: id, text: label }) : null,
      h("div", { class: "box" }, input, trailing ? h("div", { class: "trailing" }, trailing) : null),
      support);
    const ref = {
      wrap, input,
      setError(message) {
        wrap.classList.toggle("error", !!message);
        const text = message || helper || "";
        support.textContent = text;
        support.classList.toggle("hidden", !text);
      },
    };
    ref.setError(null);
    input.addEventListener("input", () => onInput?.(input.value));
    if (onEnter && !multiline) {
      input.addEventListener("keydown", (e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          onEnter();
        }
      });
    }
    return ref;
  }

  function switchInput(checked, onChange, label) {
    const input = h("input", { type: "checkbox", role: "switch", checked, "aria-label": label });
    input.addEventListener("change", () => onChange(input.checked));
    return { el: h("span", { class: "switch" }, input, h("span", { class: "track" }), h("span", { class: "thumb" })), input };
  }

  // ---------------------------------------------------------------------------------------------
  // État global
  // ---------------------------------------------------------------------------------------------
  let state = null;
  let view = null; // contenu de l'onglet affiché
  let selectedId = null; // PC affiché dans le panneau de détail
  let wantedId = null; // PC à sélectionner dès qu'il apparaît (juste après son ajout)
  let sideOpen = false; // panneau ouvert par-dessus (fenêtre étroite)
  let busyCount = 0;
  const stateListeners = new Set();
  const narrow = window.matchMedia("(max-width: 1040px)");

  const TABS = [
    ["overview", "topology", S.tab_overview],
    ["devices", "list", S.tab_devices],
    ["settings", "settings", S.tab_settings],
  ];
  let tab = (() => {
    try {
      const saved = localStorage.getItem("tab");
      return TABS.some(([id]) => id === saved) ? saved : "overview";
    } catch {
      return "overview";
    }
  })();

  const deviceById = (id) => state?.devices.find((d) => d.id === id);
  const errorMessage = (e) => fmt(S.message_error, e?.message || String(e));

  function setTab(id) {
    if (tab === id) return;
    tab = id;
    try {
      localStorage.setItem("tab", id);
    } catch {
      // Préférence non conservée : sans conséquence.
    }
    closeMenus();
    render();
  }

  function select(id, open = true) {
    if (deviceById(id)) {
      selectedId = id;
      wantedId = null; // le choix de l'utilisateur l'emporte sur un ajout en attente
    } else {
      wantedId = id;
    }
    if (open && narrow.matches) sideOpen = true;
    render();
  }

  function setBusy(on) {
    busyCount = Math.max(0, busyCount + (on ? 1 : -1));
    render();
  }

  function refreshAll() {
    refreshButton.classList.remove("turn");
    void refreshButton.offsetWidth; // relance l'animation
    refreshButton.classList.add("turn");
    api.call("refresh").catch(() => {});
  }

  // ---------------------------------------------------------------------------------------------
  // Structure : barre de navigation, contenu, panneau de détail
  // ---------------------------------------------------------------------------------------------
  const appEl = document.getElementById("app");
  const tabButtons = {};
  const tabsEl = h("nav", { class: "tabs", role: "tablist" }, TABS.map(([id, iconName, label]) =>
    (tabButtons[id] = h("button", { class: "tab", type: "button", role: "tab", title: label, onClick: () => setTab(id) },
      icon(iconName), h("span", { class: "label", text: label })))));
  const netDot = h("span", { class: "dot" });
  const netText = h("span");
  const netChip = h("div", { class: "chip net" }, netDot, netText);
  const refreshButton = iconBtn("refresh", S.action_refresh, refreshAll);
  const navEl = h("header", { class: "nav" },
    h("div", { class: "brand" }, logo(30), S.app_name),
    tabsEl,
    h("div", { class: "right" }, netChip, refreshButton, btn("primary", S.action_add, () => openEditSheet(null), "plus")));
  const busyBar = h("div", { class: "progress busy-bar hidden" });
  const mainEl = h("main", { class: "main" });
  const sideEl = h("aside", { class: "side" });
  const bodyEl = h("div", { class: "body" }, busyBar, mainEl, sideEl);

  // Fenêtre étroite : un clic à côté du panneau de détail le referme.
  mainEl.addEventListener("mousedown", (e) => {
    if (narrow.matches && sideOpen && !e.target.closest(".node, .row, .tr")) {
      sideOpen = false;
      render();
    }
  });
  narrow.addEventListener("change", () => {
    sideOpen = false;
    render();
  });

  function render() {
    if (!state) return;
    const now = Date.now();
    if (wantedId && deviceById(wantedId)) {
      selectedId = wantedId;
      wantedId = null;
    }
    if (!deviceById(selectedId)) {
      selectedId = state.devices[0]?.id ?? null;
      sideOpen = false;
    }
    updateNav();
    busyBar.classList.toggle("hidden", busyCount === 0);
    const empty = state.devices.length === 0;
    const kind = tab === "settings" ? "settings" : empty ? "empty" : tab;
    if (!view || view.kind !== kind) {
      view?.dispose?.();
      view = { overview: overviewView, devices: devicesView, settings: settingsView, empty: emptyView }[kind]();
      view.kind = kind;
      mainEl.replaceChildren(view.el);
      mainEl.scrollTop = 0;
    }
    view.update(now);
    side.update(now, kind !== "settings" && !empty);
    updates.render();
    agentDownload.render();
  }

  function updateNav() {
    for (const [id] of TABS) {
      tabButtons[id].classList.toggle("on", tab === id);
      tabButtons[id].setAttribute("aria-selected", String(tab === id));
    }
    // Pastille sur « Réglages » quand une nouvelle version est disponible.
    tabButtons.settings.classList.toggle("badge", !!state.update?.available);
    const net = state.network;
    if (net.connected) {
      setClass(netChip, "chip net st-ONLINE");
      setClass(netDot, "dot glow");
      setText(netText, [networkLabel(net), net.address].filter(Boolean).join(" · "));
    } else {
      setClass(netChip, "chip net warn st-OFFLINE");
      setClass(netDot, "dot");
      setText(netText, net.vpn ? S.network_vpn + " · " + S.banner_no_lan_title : S.banner_no_lan_title);
    }
  }

  /** Bandeau « pas de réseau local ». */
  function networkWarning() {
    const title = h("b", { text: S.banner_no_lan_title });
    const text = h("span");
    const el = h("div", { class: "warning hidden" }, icon("warning"), h("div", {}, title, text));
    return {
      el,
      update() {
        const net = state.network;
        el.classList.toggle("hidden", net.connected);
        setText(text, net.vpn ? S.banner_no_lan_vpn_text : S.banner_no_lan_text);
      },
    };
  }

  // ---------------------------------------------------------------------------------------------
  // Onglet « Vue d'ensemble » : plan du réseau et liste compacte
  // ---------------------------------------------------------------------------------------------
  function overviewView() {
    const warning = networkWarning();
    const topo = topology();
    const legendItem = (color, style, label) => h("span", { class: "legend" }, h("i", { style: `border-color:${color};border-top-style:${style}` }), label);
    const topoCard = h("section", { class: "card" },
      h("div", { class: "card-head" }, icon("topology", "small"), S.topology_title,
        h("span", { class: "aside" }, legendItem("var(--on)", "solid", S.legend_on), legendItem("var(--tr)", "dashed", S.legend_busy), legendItem("var(--off)", "dotted", S.legend_off))),
      topo.el);
    const summary = h("span", { class: "aside" });
    const rows = h("div", { class: "rows" });
    const rowMap = new Map();
    const listCard = h("section", { class: "card" }, h("div", { class: "card-head" }, icon("list", "small"), S.devices_title, summary), rows);
    const el = h("div", { class: "main-inner" }, warning.el, topoCard, listCard);
    return {
      el,
      update(now) {
        warning.update();
        topo.update(now);
        setText(summary, fmt(S.devices_summary, state.devices.length, state.settings.pollIntervalSeconds));
        syncList(rows, rowMap, state.devices, listRow, (row, d) => row.update(d, now));
      },
      dispose: () => topo.dispose(),
    };
  }

  function listRow() {
    let id = null;
    const dot = h("span", { class: "dot" });
    const name = h("b");
    const sharedIcon = icon("share", "small shared-ic");
    const host = h("span", { class: "mono" });
    const info = latencyCell("lat-cell");
    const el = h("div", { class: "row", tabindex: "0", role: "button", onClick: () => select(id), onKeydown: activate(() => select(id)) },
      dot, h("span", { class: "row-name" }, name, sharedIcon), host, info.el, icon("chevron", "small"));
    return {
      el,
      update(d, now) {
        id = d.id;
        const st = d.status.state;
        setClass(el, `row st-${st}${d.id === selectedId ? " sel" : ""}`);
        setClass(dot, dotClass(st));
        dot.title = stateName(st);
        setText(name, d.name);
        sharedIcon.classList.toggle("hidden", !d.shared);
        el.title = d.shared ? fmt(S.share_by, d.shared.ownerName) : "";
        setText(host, d.host || "—");
        const online = st === "ONLINE";
        setClass(info.el, "lat-cell" + (online ? "" : " state-text"));
        info.update(d, shortState(d.status, now));
      },
    };
  }

  // Styles des liaisons du plan : pleine (allumé), tirets animés (en cours), pointillés (éteint).
  const LINKS = {
    ONLINE: { color: "var(--on)", width: 2.2, glow: true },
    busy: { color: "var(--tr)", width: 1.8, dash: "6 5", glow: true, moving: true },
    OFFLINE: { color: "var(--off)", width: 1.6, dash: "3 5", glow: true, opacity: 0.9 },
    UNKNOWN: { color: "var(--unk)", width: 1.6, dash: "3 5", opacity: 0.6 },
    lan: { color: "var(--blue)", width: 2.2 },
    noLan: { color: "var(--off)", width: 1.6, dash: "3 5", opacity: 0.8 },
  };

  function svgPath() {
    return document.createElementNS(SVG_NS, "path");
  }

  function styleLink(path, spec) {
    path.setAttribute("class", spec.moving ? "link-busy" : "");
    path.style.stroke = spec.color;
    path.style.strokeWidth = String(spec.width);
    path.style.strokeDasharray = spec.dash || "none";
    path.style.opacity = String(spec.opacity ?? 1);
    path.style.filter = spec.glow ? `drop-shadow(0 0 4px ${spec.color})` : "none";
  }

  function setD(path, d) {
    if (path.getAttribute("d") !== d) path.setAttribute("d", d);
  }

  function setPos(el, x, y) {
    el.style.left = Math.round(x) + "px";
    el.style.top = Math.round(y) + "px";
  }

  /** Plan du réseau : « Ce PC » — « Réseau » — un nœud par PC, reliés selon leur état. */
  function topology() {
    const svg = document.createElementNS(SVG_NS, "svg");
    svg.setAttribute("class", "links");
    svg.setAttribute("aria-hidden", "true");
    const lanLink = svgPath();
    svg.append(lanLink);
    const anchorAddress = h("small", { class: "mono" });
    const anchor = h("div", { class: "anchor" },
      h("span", { class: "ni" }, icon("monitor", "", 18)),
      h("span", { class: "nt" }, h("b", { text: S.topology_this_pc }), anchorAddress));
    const hubIcon = h("span", { class: "ni" });
    const hubText = h("small");
    const hub = h("div", { class: "anchor hub" }, hubIcon, h("span", { class: "nt" }, h("b", { text: S.topology_network }), hubText));
    const el = h("div", { class: "topo" }, svg, anchor, hub);
    const nodes = new Map();
    let order = [];
    let hubIconName = "";
    let lanKind = "";
    let orderSig = "";
    const observer = new ResizeObserver(() => layout());
    observer.observe(el);

    function update(now) {
      const net = state.network;
      setText(anchorAddress, (net.address || "").split("/")[0] || "—");
      const iconName = net.transport === "wifi" && net.connected ? "wifi" : "hub";
      if (iconName !== hubIconName) {
        hubIconName = iconName;
        hubIcon.replaceChildren(icon(iconName, "", 18));
      }
      setText(hubText, networkLabel(net));
      const kind = net.connected ? "lan" : "noLan";
      if (kind !== lanKind) {
        lanKind = kind;
        styleLink(lanLink, LINKS[kind]);
      }
      order = state.devices.map((d) => d.id);
      for (const d of state.devices) {
        let node = nodes.get(d.id);
        if (!node) {
          node = topologyNode();
          nodes.set(d.id, node);
          el.append(node.el);
          svg.append(node.path);
        }
        node.update(d, now);
      }
      for (const [id, node] of nodes) {
        if (!order.includes(id)) {
          node.el.remove();
          node.path.remove();
          nodes.delete(id);
        }
      }
      if (order.join("|") !== orderSig) {
        orderSig = order.join("|");
        for (const id of order) el.append(nodes.get(id).el);
      }
      layout();
    }

    function layout() {
      const W = el.clientWidth;
      if (!W || !order.length) return;
      const list = order.map((id) => nodes.get(id));
      // Contenu centré et borné en largeur : liaisons lisibles même sur grand écran.
      const CW = Math.min(W, 900);
      const ox = (W - CW) / 2;
      const nodeW = Math.round(Math.min(340, Math.max(220, CW * 0.42)));
      for (const node of list) node.el.style.width = nodeW + "px";
      const nodeH = list[0].el.offsetHeight || 50;
      const gap = 12;
      const pad = 14;
      const colH = list.length * nodeH + (list.length - 1) * gap;
      const aw = anchor.offsetWidth;
      const ah = anchor.offsetHeight;
      const hw = hub.offsetWidth;
      const hh = hub.offsetHeight;
      const H = Math.max(colH, ah, 110) + pad * 2;
      el.style.height = H + "px";
      const cy = Math.round(H / 2);
      const nodeX = ox + CW - nodeW;
      // Fenêtre étroite : « Ce PC » s'efface pour laisser la place aux liaisons.
      const showAnchor = nodeX - ox - aw - hw >= 80;
      const hubX = showAnchor ? ox + aw + Math.round((nodeX - ox - aw - hw) * 0.42) : ox + Math.round(Math.max(0, nodeX - ox - hw) * 0.25);
      anchor.style.visibility = showAnchor ? "" : "hidden";
      setPos(anchor, ox, cy - ah / 2);
      setPos(hub, hubX, cy - hh / 2);
      setD(lanLink, showAnchor ? `M${Math.round(ox + aw)} ${cy} L${hubX} ${cy}` : "");
      const x1 = hubX + hw;
      const dx = Math.max(24, (nodeX - x1) * 0.55);
      const top0 = Math.round((H - colH) / 2);
      list.forEach((node, i) => {
        const y = top0 + i * (nodeH + gap);
        setPos(node.el, nodeX, y);
        const ny = Math.round(y + nodeH / 2);
        setD(node.path, `M${x1} ${cy} C${Math.round(x1 + dx)} ${cy} ${Math.round(nodeX - dx)} ${ny} ${Math.round(nodeX)} ${ny}`);
      });
    }

    return { el, update, dispose: () => observer.disconnect() };
  }

  function topologyNode() {
    let id = null;
    let linkKind = "";
    const dot = h("span", { class: "dot" });
    const name = h("b");
    const stateText = h("span");
    const ip = h("span", { class: "ip mono" });
    const el = h("div", { class: "node", tabindex: "0", role: "button", onClick: () => select(id), onKeydown: activate(() => select(id)) },
      h("span", { class: "ni" }, icon("monitor", "", 18)),
      h("span", { class: "nt" }, name, h("small", {}, dot, stateText)),
      ip);
    const path = svgPath();
    return {
      el,
      path,
      update(d, now) {
        id = d.id;
        const st = d.status.state;
        setClass(el, `node st-${st}${d.id === selectedId ? " sel" : ""}`);
        setClass(dot, dotClass(st));
        setText(name, d.name);
        setText(stateText, shortState(d.status, now));
        setText(ip, d.host || "—");
        const kind = isTransitional(st) ? "busy" : st;
        if (kind !== linkKind) {
          linkKind = kind;
          styleLink(path, LINKS[kind] || LINKS.UNKNOWN);
        }
      },
    };
  }

  // ---------------------------------------------------------------------------------------------
  // Onglet « Appareils » : tableau
  // ---------------------------------------------------------------------------------------------
  function devicesView() {
    const warning = networkWarning();
    const summary = h("span", { class: "aside" });
    const rows = h("div");
    const rowMap = new Map();
    const el = h("div", { class: "main-inner" }, warning.el,
      h("section", { class: "card table" },
        h("div", { class: "card-head" }, icon("list", "small"), S.devices_title, summary),
        h("div", { class: "th", role: "row" },
          h("span", { text: S.col_name }), h("span", { text: S.col_host }), h("span", { text: S.col_state }),
          h("span", { class: "opt", text: S.col_mac }), h("span", { class: "opt opt2", text: S.col_system }),
          h("span", { class: "opt", text: S.col_latency }), h("span")),
        rows));
    return {
      el,
      update(now) {
        warning.update();
        setText(summary, fmt(S.devices_summary, state.devices.length, state.settings.pollIntervalSeconds));
        syncList(rows, rowMap, state.devices, tableRow, (row, d) => row.update(d, now));
      },
    };
  }

  function tableRow() {
    let id = null;
    let actsSig = "";
    const name = h("b");
    const capability = h("small");
    const host = h("span", { class: "cell txt mono" });
    const dot = h("span", { class: "dot" });
    const stateText = h("span");
    const temps = h("small", { class: "temps hidden" });
    const stateCell = h("span", { class: "cell state-text" }, dot, h("span", { class: "t" }, stateText, temps));
    const mac = h("span", { class: "cell txt opt mono" });
    const system = h("span", { class: "cell txt opt opt2" });
    const latency = latencyCell("cell lat-cell opt");
    const acts = h("span", { class: "cell actions" });
    const more = iconBtn("more", S.action_more, own(() => openDeviceMenu(more, id)), "flat");
    const el = h("div", { class: "tr", tabindex: "0", role: "row", onClick: () => select(id), onKeydown: activate(() => select(id)) },
      h("span", { class: "cell" }, h("span", { class: "ni" }, icon("monitor", "", 18)), h("span", { class: "t" }, name, capability)),
      host, stateCell, mac, system, latency.el, acts);
    return {
      el,
      update(d, now) {
        id = d.id;
        const s = d.status;
        const st = s.state;
        const online = st === "ONLINE";
        setClass(el, `tr st-${st}${d.id === selectedId ? " sel" : ""}`);
        setText(name, d.name);
        setText(capability, d.shared ? fmt(S.share_by, d.shared.ownerName) : d.canShutdown ? S.capability_power : S.capability_wake);
        setText(host, d.host || "—");
        setClass(dot, dotClass(st));
        setText(stateText, shortState(s, now));
        const t = temperatureSummary(online && s.agent ? s.agent.temperatures : null);
        setText(temps, t.text);
        temps.title = t.text; // texte complet si la colonne est étroite
        setClass(temps, `temps${t.cls ? " " + t.cls : ""}${t.text ? "" : " hidden"}`);
        setText(mac, d.mac || "—");
        setText(system, online && s.agent ? [osLabel(s.agent.os), archLabel(s.agent.arch)].filter(Boolean).join(" · ") : "—");
        latency.update(d, "—");
        const sig = st + "|" + d.canShutdown;
        if (sig !== actsSig) {
          actsSig = sig;
          const parts = [];
          if (online) {
            if (d.canShutdown) parts.push(btn("sec", S.action_shutdown, own(() => requestPower(id, "shutdown")), "power"));
          } else if (st === "OFFLINE" || st === "UNKNOWN") {
            parts.push(btn("primary", S.action_wake, own(() => wake(id)), "power"));
          } else {
            parts.push(h("span", { class: "spin", title: stateName(st) }));
          }
          acts.replaceChildren(...parts, more);
        }
      },
    };
  }

  function emptyView() {
    const warning = networkWarning();
    const el = h("div", { class: "main-inner narrow" }, warning.el,
      h("div", { class: "empty" },
        logo(72),
        h("h2", { text: S.empty_title }),
        h("p", { text: S.empty_text }),
        h("div", { class: "row-btns" },
          btn("primary big", S.action_add_device, () => openEditSheet(null), "plus"),
          btn("sec big", S.action_import_short, pickImportFile, "upload"))));
    return { el, update: () => warning.update() };
  }

  // ---------------------------------------------------------------------------------------------
  // Tracé de la latence en direct (façon électrocardiogramme)
  // ---------------------------------------------------------------------------------------------
  // La dernière minute défile en continu vers la gauche ; le tracé suit les mesures avec un léger
  // retard (l'intervalle entre deux mesures) pour que la « plume », au bord droit, glisse d'une
  // mesure à l'autre au lieu de sauter. Une impulsion marque chaque nouvelle mesure.
  const TRACE_WINDOW = 60_000;
  const TRACE_SCALES = [5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000];
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
  const traces = new Set();
  let traceFrame = 0;
  let traceDrawnAt = 0;
  let traceColors = null;

  /** Haut de l'échelle (ms) : un peu de marge au-dessus du maximum, arrondi à une valeur ronde (comme Android). */
  function latencyScale(max) {
    const target = max + Math.floor((max + 5) / 6);
    return TRACE_SCALES.find((v) => v >= target) ?? TRACE_SCALES[TRACE_SCALES.length - 1];
  }

  /** Synthèse de la dernière minute : min / moyenne / max et sondes restées sans réponse. */
  function latencyStats(samples, now) {
    const period = (samples || []).filter((s) => s.t >= now - TRACE_WINDOW);
    if (!period.length) return null;
    const values = period.map((s) => s.ms).filter((v) => v != null);
    return {
      min: values.length ? Math.min(...values) : null,
      avg: values.length ? Math.round(values.reduce((a, b) => a + b, 0) / values.length) : null,
      max: values.length ? Math.max(...values) : null,
      lost: period.length - values.length,
      count: period.length,
    };
  }

  function colors() {
    if (!traceColors) {
      const css = getComputedStyle(document.documentElement);
      const v = (name) => css.getPropertyValue(name).trim();
      traceColors = {
        on: v("--on"), off: v("--off"), grid: v("--line"), label: v("--text-3"), cross: v("--text-2"), surface: v("--surface"),
        font: "500 11px " + v("--font"),
      };
    }
    return traceColors;
  }

  function withAlpha(hex, alpha) {
    const n = parseInt(hex.slice(1), 16);
    return `rgba(${n >> 16},${(n >> 8) & 255},${n & 255},${alpha})`;
  }

  function scheduleTraces() {
    if (traceFrame || document.hidden) return;
    if (reducedMotion.matches) traceFrame = setTimeout(() => drawTraces(), 1000);
    else traceFrame = requestAnimationFrame(() => drawTraces());
  }

  function drawTraces() {
    traceFrame = 0;
    const now = Date.now();
    // Une trentaine d'images par seconde suffisent : le tracé avance de quelques pixels par seconde.
    if (!reducedMotion.matches && now - traceDrawnAt < 30) {
      scheduleTraces();
      return;
    }
    traceDrawnAt = now;
    for (const t of traces) {
      if (t.canvas.isConnected) t.draw(now);
      else traces.delete(t); // ligne ou panneau retiré : se réinscrit à la prochaine mise à jour
    }
    if (traces.size) scheduleTraces();
  }
  document.addEventListener("visibilitychange", scheduleTraces);

  /**
   * Tracé de latence : `detailed` pour la carte du panneau de détail (grille, échelle, survol),
   * sinon mini-tracé d'une ligne de liste.
   */
  function latencyTrace(detailed, onHover) {
    const canvas = h("canvas", { class: detailed ? "trace" : "spark", "aria-hidden": "true" });
    const ctx = canvas.getContext("2d");
    let samples = [];
    let key = null;
    let delay = 0;
    let scale = 0;
    let lastFrame = 0;
    let hoverX = null;

    if (detailed) {
      canvas.addEventListener("pointermove", (e) => {
        hoverX = e.offsetX;
        scheduleTraces();
      });
      canvas.addEventListener("pointerleave", () => {
        hoverX = null;
        onHover?.(null);
      });
    }

    /** Intervalle habituel entre deux mesures (médiane des derniers écarts). */
    function expectedGap() {
      const gaps = [];
      for (let i = samples.length - 1; i > 0 && gaps.length < 6; i--) gaps.push(samples[i].t - samples[i - 1].t);
      if (!gaps.length) return 1000;
      gaps.sort((a, b) => a - b);
      return Math.min(6000, Math.max(800, gaps[gaps.length >> 1]));
    }

    /** Mesures à tracer ; `id` identifie le PC (l'échelle repart de zéro quand il change). */
    function set(list, id) {
      if (id !== key) {
        key = id;
        delay = 0;
        scale = 0;
        hoverX = null;
        onHover?.(null);
      }
      samples = list || [];
      traces.add(trace);
      scheduleTraces();
    }

    function draw(now) {
      const w = canvas.clientWidth;
      const hgt = canvas.clientHeight;
      if (!w || !hgt) return;
      const dpr = window.devicePixelRatio || 1;
      if (canvas.width !== Math.round(w * dpr) || canvas.height !== Math.round(hgt * dpr)) {
        canvas.width = Math.round(w * dpr);
        canvas.height = Math.round(hgt * dpr);
      }
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, w, hgt);
      const c = colors();
      const dt = lastFrame ? Math.min(250, now - lastFrame) : 1000;
      lastFrame = now;

      // Retard du tracé : suit (en douceur) l'intervalle entre deux mesures.
      const gap = expectedGap();
      const targetDelay = gap + 200;
      delay = delay ? delay + (targetDelay - delay) * Math.min(1, dt / 700) : targetDelay;
      const rt = now - delay; // instant tracé au bord droit
      const gapBreak = Math.max(4000, gap * 2.5);

      const top = detailed ? 18 : 3;
      const bottom = hgt - (detailed ? 6 : 3);
      const right = w - (detailed ? 8 : 4);
      const xOf = (t) => right - ((rt - t) / TRACE_WINDOW) * right;

      // Échelle : maximum des mesures visibles (et de la prochaine, qui entre par la droite).
      let max = 0;
      let measured = false;
      let next = null;
      for (const s of samples) {
        if (s.t < rt - TRACE_WINDOW - gapBreak) continue;
        if (s.t > rt) {
          next ??= s;
          if (s !== next) continue;
        }
        if (s.ms != null) {
          max = Math.max(max, s.ms);
          measured = true;
        }
      }
      const targetScale = latencyScale(max);
      scale = scale ? scale + (targetScale - scale) * Math.min(1, dt / 350) : targetScale;
      const yOf = (v) => bottom - (Math.min(v, scale) / scale) * (bottom - top);

      if (detailed) {
        // Grille « papier d'électrocardiogramme » : repères horizontaux et un trait toutes les 10 s.
        ctx.lineWidth = 1;
        ctx.strokeStyle = c.grid;
        ctx.beginPath();
        for (const f of [0, 0.5, 1]) {
          const y = Math.round(bottom - f * (bottom - top)) + 0.5;
          ctx.moveTo(0, y);
          ctx.lineTo(w, y);
        }
        for (let t = Math.ceil((rt - TRACE_WINDOW) / 10_000) * 10_000; t <= rt; t += 10_000) {
          const x = Math.round(xOf(t)) + 0.5;
          ctx.moveTo(x, top);
          ctx.lineTo(x, bottom);
        }
        ctx.stroke();
        if (measured) {
          ctx.fillStyle = c.label;
          ctx.font = c.font;
          ctx.textBaseline = "alphabetic";
          ctx.fillText(fmt(S.latency_ms, Math.round(scale)), 0, top - 5);
        }
      }

      // Segments continus (une sonde sans réponse ou une longue interruption coupe le tracé).
      const segments = [];
      const lost = [];
      let seg = null;
      let prev = null;
      let head = null; // plume : point tracé à l'instant rt
      for (const s of samples) {
        if (s.t > rt) {
          if (seg && prev && s.ms != null && s.t - prev.t <= gapBreak) {
            const v = prev.ms + ((s.ms - prev.ms) * (rt - prev.t)) / (s.t - prev.t);
            seg.push([right, yOf(v)]);
            head = seg[seg.length - 1];
          }
          break;
        }
        if (s.ms == null) {
          lost.push(xOf(s.t));
          seg = null;
        } else {
          if (!seg || (prev && s.t - prev.t > gapBreak)) segments.push((seg = []));
          seg.push([xOf(s.t), yOf(s.ms)]);
        }
        prev = s;
      }
      if (!head && prev && prev.ms != null && seg) head = seg[seg.length - 1];

      for (const points of segments) {
        if (points[points.length - 1][0] < -4) continue;
        // Voile sous la courbe, puis la courbe (2 px) avec une lueur de moniteur.
        ctx.beginPath();
        ctx.moveTo(points[0][0], bottom);
        for (const [x, y] of points) ctx.lineTo(x, y);
        ctx.lineTo(points[points.length - 1][0], bottom);
        ctx.closePath();
        const wash = ctx.createLinearGradient(0, top, 0, bottom);
        wash.addColorStop(0, withAlpha(c.on, detailed ? 0.16 : 0.12));
        wash.addColorStop(1, withAlpha(c.on, 0));
        ctx.fillStyle = wash;
        ctx.fill();

        ctx.beginPath();
        points.forEach(([x, y], i) => (i ? ctx.lineTo(x, y) : ctx.moveTo(x, y)));
        ctx.lineWidth = detailed ? 2 : 1.5;
        ctx.lineJoin = "round";
        ctx.lineCap = "round";
        ctx.strokeStyle = c.on;
        ctx.shadowColor = withAlpha(c.on, 0.55);
        ctx.shadowBlur = detailed ? 8 : 4;
        if (points.length === 1) {
          ctx.moveTo(points[0][0] - 1, points[0][1]);
          ctx.lineTo(points[0][0], points[0][1]);
        }
        ctx.stroke();
        ctx.shadowBlur = 0;
      }

      // Sondes restées sans réponse : petits traits rouges sur la ligne de base.
      ctx.fillStyle = c.off;
      for (const x of lost) if (x > -2) ctx.fillRect(Math.round(x) - 1, bottom - (detailed ? 7 : 4), 2, detailed ? 7 : 4);

      // Plume : point au bout du tracé, avec une impulsion à chaque nouvelle mesure.
      if (head) {
        const age = prev ? rt - prev.t : Infinity;
        if (!reducedMotion.matches && age >= 0 && age < 700) {
          const k = age / 700;
          ctx.beginPath();
          ctx.arc(head[0], head[1], (detailed ? 4 : 2.5) + (detailed ? 10 : 5) * k, 0, Math.PI * 2);
          ctx.fillStyle = withAlpha(c.on, 0.45 * (1 - k));
          ctx.fill();
        }
        dot(head[0], head[1], detailed ? 4 : 2.5, c.on, detailed ? c.surface : null);
      }

      if (detailed && hoverX != null) drawHover(rt, xOf, yOf, top, bottom, c);
    }

    function dot(x, y, r, fill, ring) {
      if (ring) {
        ctx.beginPath();
        ctx.arc(x, y, r + 2, 0, Math.PI * 2);
        ctx.fillStyle = ring;
        ctx.fill();
      }
      ctx.beginPath();
      ctx.arc(x, y, r, 0, Math.PI * 2);
      ctx.fillStyle = fill;
      ctx.fill();
    }

    /** Survol : réticule sur la mesure la plus proche, valeur dans l'infobulle. */
    function drawHover(rt, xOf, yOf, top, bottom, c) {
      let best = null;
      for (const s of samples) {
        if (s.t > rt || s.t < rt - TRACE_WINDOW) continue;
        if (!best || Math.abs(xOf(s.t) - hoverX) < Math.abs(xOf(best.t) - hoverX)) best = s;
      }
      if (!best || Math.abs(xOf(best.t) - hoverX) > 24) {
        onHover?.(null);
        return;
      }
      const x = Math.round(xOf(best.t)) + 0.5;
      ctx.strokeStyle = c.cross;
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(x, top);
      ctx.lineTo(x, bottom);
      ctx.stroke();
      if (best.ms != null) dot(x, yOf(best.ms), 4, c.on, c.surface);
      onHover?.({ sample: best, x });
    }

    /** Arrête l'animation (tracé masqué). */
    function stop() {
      traces.delete(trace);
    }

    const trace = { canvas, set, draw, stop };
    return trace;
  }

  /** Carte « Latence » du panneau de détail : valeur en direct, tracé de la dernière minute, synthèse. */
  function latencyCard() {
    const value = h("b", { class: "lat-value" });
    const via = h("span", { class: "lat-via" });
    const live = h("span", { class: "live" }, h("i"), S.latency_live);
    const tipValue = h("b");
    const tipTime = h("span");
    const tip = h("div", { class: "lat-tip hidden" }, tipValue, tipTime);
    const trace = latencyTrace(true, (hover) => {
      tip.classList.toggle("hidden", !hover);
      if (!hover) return;
      const { sample, x } = hover;
      setText(tipValue, sample.ms != null ? fmt(S.latency_ms, sample.ms) : S.latency_no_answer);
      const ago = Math.round((Date.now() - sample.t) / 1000);
      setText(tipTime, ago <= 1 ? S.latency_just_now : fmt(S.latency_ago, ago));
      tip.style.left = x + "px";
    });
    const plot = h("div", { class: "lat-plot", role: "img" }, trace.canvas, tip);
    const stat = (label) => {
      const v = h("b");
      return { el: h("div", {}, h("span", { text: label }), v), v };
    };
    const stats = [stat(S.latency_min), stat(S.latency_avg), stat(S.latency_max), stat(S.latency_lost)];
    const el = h("section", { class: "latency" },
      h("div", { class: "lat-head" },
        h("div", { class: "lat-title" }, h("span", { class: "hist-head", text: S.latency_title }), live),
        h("div", { class: "lat-now" }, value, via)),
      plot,
      h("div", { class: "lat-axis" }, h("span", { text: S.latency_axis_start }), h("span", { text: S.latency_axis_end })),
      h("div", { class: "lat-stats" }, stats.map((s) => s.el)));

    return {
      el,
      update(d, now) {
        const s = d.status;
        const online = s.state === "ONLINE";
        const checking = s.state !== "UNKNOWN";
        live.classList.toggle("hidden", !checking);
        const current = online && s.latencyMs != null;
        setClass(value, "lat-value" + (current ? "" : " none"));
        setText(value, current ? fmt(S.latency_ms, s.latencyMs) : "—");
        const method = { AGENT: S.latency_via_agent, TCP: S.latency_via_tcp, PING: S.latency_via_ping }[s.method];
        setText(via, current ? method || "" : checking ? S.latency_no_answer : S.latency_not_measured);
        trace.set(d.latency, d.id);
        const st = latencyStats(d.latency, now);
        const ms = (v) => (v == null ? "—" : fmt(S.latency_ms, v));
        setText(stats[0].v, ms(st?.min));
        setText(stats[1].v, ms(st?.avg));
        setText(stats[2].v, ms(st?.max));
        setText(stats[3].v, st ? `${st.lost}/${st.count}` : "—");
        plot.setAttribute("aria-label", st ? fmt(S.latency_aria, ms(st.avg), ms(st.max), st.lost, st.count) : S.latency_waiting);
      },
    };
  }

  /** Cellule de liste : mini-tracé de la dernière minute puis latence actuelle (PC allumé). */
  function latencyCell(cls) {
    const trace = latencyTrace(false);
    const text = h("span");
    const el = h("span", { class: cls }, trace.canvas, text);
    return {
      el,
      update(d, fallback) {
        const s = d.status;
        const online = s.state === "ONLINE" && s.latencyMs != null;
        trace.canvas.classList.toggle("hidden", !online);
        if (online) trace.set(d.latency, d.id);
        else trace.stop();
        setText(text, online ? fmt(S.latency_ms, s.latencyMs) : fallback);
      },
    };
  }

  // ---------------------------------------------------------------------------------------------
  // Panneau de détail du PC sélectionné
  // ---------------------------------------------------------------------------------------------
  const side = (() => {
    let current = null;
    let actsSig = "";
    const more = iconBtn("more", S.action_more, own(() => openDeviceMenu(more, selectedId)), "flat");
    const closeButton = iconBtn("close", S.close, () => {
      sideOpen = false;
      render();
    }, "flat side-close");
    const ring = h("div", { class: "ring" }, icon("monitor", "", 44));
    const name = h("h2", { class: "selectable" });
    const lineDot = h("span", { class: "dot" });
    const lineText = h("span");
    const hero = h("div", { class: "hero" }, ring, name, h("div", { class: "line" }, lineDot, lineText));
    const noticeText = h("span");
    const notice = h("div", { class: "notice hidden" }, icon("warning", "small"), noticeText,
      btn("ghost", S.ok, () => api.call("clearNotice", { id: selectedId }).catch(() => {})));
    const acts = h("div", { class: "acts" });
    const hint = h("div", { class: "hint hidden", text: S.hint_no_agent });
    const info = h("div", { class: "info selectable" });
    const latency = latencyCard();
    const recent = sideHistory();
    const inner = h("div", { class: "side-inner" }, h("div", { class: "side-top" }, more, closeButton), hero, notice, acts, hint, latency.el, info, recent.el);
    const placeholder = h("div", { class: "placeholder" }, h("span", { class: "ni" }, icon("monitor", "", 28)), h("span", { text: S.select_hint }));
    sideEl.append(inner, placeholder);

    let liveId = "";

    /** PC affiché en détail : sondé chaque seconde par le moteur pour le tracé de latence. */
    function setLive(id) {
      if (id === liveId) return;
      liveId = id;
      api.call("setLive", { id }).catch(() => {});
    }

    function update(now, visible) {
      sideEl.classList.toggle("hidden", !visible);
      sideEl.classList.toggle("open", visible && sideOpen);
      const d = visible ? deviceById(selectedId) : null;
      const shown = !!d && (!narrow.matches || sideOpen);
      setLive(shown && d.host ? d.id : "");
      if (!visible) return;
      inner.classList.toggle("hidden", !d);
      placeholder.classList.toggle("hidden", !!d);
      if (!d) return;
      if (current !== d.id) {
        current = d.id;
        actsSig = "";
        sideEl.scrollTop = 0;
      }
      const s = d.status;
      const st = s.state;
      setClass(inner, "side-inner st-" + st);
      setText(name, d.name);
      setClass(lineDot, dotClass(st));
      setText(lineText, statusText(s, now));
      ring.title = stateName(st);

      notice.classList.toggle("hidden", !s.notice);
      if (s.notice) setText(noticeText, s.notice === "WAKE_TIMEOUT" ? S.notice_wake_timeout : S.notice_shutdown_timeout);

      const sig = [d.id, st, d.canShutdown, !!d.shared].join("|");
      if (sig !== actsSig) {
        actsSig = sig;
        renderActions(d);
      }
      // Agent configuré mais aucune réponse : le plus souvent un réseau « Public » sous Windows.
      const offlineAgent = !d.shared && st === "OFFLINE" && d.hasAgent;
      setText(hint, d.shared ? fmt(S.hint_shared, d.shared.ownerName) : offlineAgent ? S.hint_offline_agent : S.hint_no_agent);
      hint.classList.toggle("hidden", !d.shared && !offlineAgent && !(st === "ONLINE" && !d.canShutdown));
      latency.el.classList.toggle("hidden", !d.host);
      if (d.host) latency.update(d, now);
      renderInfo(d);
      recent.update(d, now);
    }

    function renderActions(d) {
      const id = d.id;
      const st = d.status.state;
      // Un PC reçu d'une autre personne ne se modifie pas.
      const edit = (cls) => (d.shared ? null : btn("sec " + cls, S.action_edit, () => openEditSheet(id), "edit"));
      let parts;
      if (st === "ONLINE") {
        parts = d.canShutdown
          ? [
            btn("danger", S.action_shutdown, () => requestPower(id, "shutdown"), "power"),
            btn("sec", S.action_reboot, () => requestPower(id, "reboot"), "restart"),
            btn("sec", S.action_sleep_short, () => requestPower(id, "sleep"), "moon"),
            edit(""),
          ]
          : [edit("wide")];
      } else if (st === "WAKING") {
        parts = [btn("sec", S.action_wake_again, () => wake(id), "refresh"), edit("")];
      } else if (isTransitional(st)) {
        parts = [edit("wide")];
      } else {
        parts = [btn("primary wide", S.action_wake, () => wake(id), "power"), edit("wide")];
      }
      parts = parts.filter(Boolean);
      if (parts.length % 2 === 1) parts[parts.length - 1].classList.add("wide");
      acts.replaceChildren(...parts);
    }

    function renderInfo(d) {
      const s = d.status;
      const online = s.state === "ONLINE";
      const rows = [
        { key: "ip", label: S.detail_ip, text: d.host || S.detail_not_set, cls: d.host ? "mono" : "muted" },
        { key: "mac", label: S.detail_mac, text: d.mac, cls: "mono" },
      ];
      if (online && s.agent) {
        if (s.agent.hostname) rows.push({ key: "name", label: S.detail_hostname, text: s.agent.hostname });
        rows.push({ key: "sys", label: S.detail_system, text: [osLabel(s.agent.os), archLabel(s.agent.arch)].filter(Boolean).join(" · ") });
        rows.push({ key: "up", label: S.detail_uptime, text: formatLong(s.agent.uptime) });
        const t = s.agent.temperatures;
        if (t?.cpu != null) rows.push({ key: "cpu", label: S.detail_temp_cpu, text: celsius(t.cpu), cls: tempClass(t.cpu) });
        else if (t?.cpuHint === "lhm") {
          const [text, help] = lhmHint(t.lhm);
          rows.push({ key: "cpu", label: S.detail_temp_cpu, text, cls: "muted", title: help, onClick: () => alertDialog(S.detail_temp_cpu, help) });
        }
        if (t?.cpuLoad != null) rows.push({ key: "cpuLoad", label: S.detail_load_cpu, text: percentText(t.cpuLoad) });
        if (t?.gpu != null && t.gpuShared) {
          const help = fmt(S.temperature_gpu_shared_help, t.gpuName || "GPU");
          rows.push({ key: "gpu", label: S.detail_temp_gpu, text: `${celsius(t.gpu)} · ${S.temperature_gpu_shared}`, cls: tempClass(t.gpu), title: help });
        } else if (t?.gpu != null) rows.push({ key: "gpu", label: S.detail_temp_gpu, text: celsius(t.gpu), cls: tempClass(t.gpu), title: t.gpuName });
        if (t?.gpuLoad != null) rows.push({ key: "gpuLoad", label: S.detail_load_gpu, text: percentText(t.gpuLoad), title: t.gpuName });
      }
      if (d.hasAgent) rows.push({ key: "metrics", label: S.detail_metrics, text: S.metrics_open, iconName: "chart", onClick: () => openMetricsSheet(d.id) });
      if (d.shared) rows.push({ key: "shared", label: S.detail_shared, text: d.shared.ownerName, cls: "shared-by", iconName: "share" });
      if (!d.hasAgent) {
        if (!d.shared) rows.push({ key: "agent", label: S.detail_agent, text: S.agent_not_configured, cls: "muted" });
      }
      else if (online && s.agentError) rows.push({ key: "agent", label: S.detail_agent, text: agentErrorLabel(s.agentError), cls: "bad" });
      else if (online && s.agent) {
        rows.push({ key: "agent", label: S.detail_agent, text: fmt(S.agent_authenticated, versionLabel(s.agent.version)), cls: "ok", iconName: "shield" });
        const latest = state.agent?.latest;
        if (latest && compareVersions(s.agent.version, latest) < 0) {
          rows.push({ key: "agentUpdate", label: S.detail_agent_update, text: fmt(S.agent_update_available, versionLabel(latest)),
            iconName: "download", onClick: state.agent.enabled ? () => agentDownload.open() : () => openUrl(RELEASES_URL) });
        }
      }
      else rows.push({ key: "agent", label: S.detail_agent, text: S.agent_configured, cls: "muted" });

      const keys = rows.map((r) => r.key).join("|");
      if (info.dataset.keys !== keys) {
        info.dataset.keys = keys;
        info.replaceChildren(...rows.map((r) => h("div", { class: "kv" }, h("span", { text: r.label }), h("span"))));
      }
      rows.forEach((r, i) => {
        const valueEl = info.children[i].lastChild;
        const sig = [r.text, r.cls, r.iconName, r.title, !!r.onClick].join("|");
        if (valueEl.dataset.sig === sig) return;
        valueEl.dataset.sig = sig;
        const content = [r.iconName ? icon(r.iconName, "small") : null, r.text];
        valueEl.replaceChildren(r.onClick
          ? h("button", { type: "button", class: "linkish", title: r.title || null, onClick: r.onClick }, content)
          : h("span", { class: r.cls || null, title: r.title || null }, content));
      });
    }

    return { update };
  })();

  /** Historique discret du panneau de détail : derniers évènements du PC sélectionné. */
  function sideHistory() {
    let id = null;
    let version = -1;
    let seq = 0;
    let data = null;
    let renderedDay = 0;
    const showAll = btn("ghost hidden", S.history_show_all, () => openHistorySheet(id));
    const list = h("div", { class: "hist" });
    const note = h("div", { class: "hist-note hidden" });
    const el = h("section", { class: "side-hist" }, h("div", { class: "hist-head" }, S.history_title, showAll), list, note);

    function update(d, now) {
      if (d.id !== id) {
        id = d.id;
        data = null;
        version = state.historyVersion;
        list.replaceChildren();
        note.classList.add("hidden");
        showAll.classList.add("hidden");
        load(true); // relit aussi le journal de l'agent (au plus toutes les 20 s)
      } else if (state.historyVersion !== version) {
        version = state.historyVersion;
        load(false);
      } else if (data && startOfDay(now) !== renderedDay) {
        show(now); // « aujourd'hui » devient « hier » à minuit
      }
    }

    async function load(refresh) {
      const mine = ++seq;
      try {
        const r = await api.call("getHistory", { id, refresh });
        if (mine !== seq) return;
        data = r;
        show(Date.now());
      } catch (e) {
        // Historique momentanément indisponible : l'affichage précédent reste.
        console.warn("historique :", e);
      }
    }

    function show(now) {
      renderedDay = startOfDay(now);
      const events = data.events.slice(0, SIDE_HISTORY_COUNT);
      list.replaceChildren(...(events.length ? events.map((e) => eventRow(e, now)) : [h("div", { class: "hist-note", text: S.history_empty })]));
      let text = "";
      if (!data.hasAgent) text = S.history_note_no_agent;
      else if (data.agent === "outdated") text = S.history_note_outdated;
      note.textContent = text;
      note.classList.toggle("hidden", !text);
      showAll.classList.toggle("hidden", data.events.length === 0);
    }

    return { el, update };
  }

  /** Historique complet (30 jours), par jour, pour un PC ou pour tous. */
  function openHistorySheet(deviceId) {
    let filter = deviceById(deviceId) ? deviceId : "";
    let data = null;
    let version = -1;
    let seq = 0;
    let limit = HISTORY_PAGE;
    let devicesSig = "";
    const selectEl = h("select", { "aria-label": S.history_filter });
    selectEl.addEventListener("change", () => {
      filter = selectEl.value;
      limit = HISTORY_PAGE;
      load(true);
    });
    const note = h("div", { class: "hist-note hidden" });
    const list = h("div", { class: "hist" });
    const moreButton = btn("sec hist-more hidden", S.history_more, () => {
      limit += HISTORY_PAGE;
      show();
    });
    const onState = () => {
      if (filter && !deviceById(filter)) filter = "";
      fillSelect();
      if (state.historyVersion !== version) load(false);
    };
    openSheet({
      title: S.history_title,
      subtitle: S.history_subtitle,
      headExtra: [selectEl, iconBtn("refresh", S.history_refresh, () => load(true), "flat")],
      body: [note, list, moreButton],
      foot: h("div", { class: "hist-note grow", text: S.history_sources }),
      onClose: () => stateListeners.delete(onState),
    });
    stateListeners.add(onState);
    fillSelect();
    load(true);

    function fillSelect() {
      const sig = state.devices.map((d) => d.id + "\u0000" + d.name).join("\u0001") + "\u0002" + filter;
      if (sig === devicesSig) return;
      devicesSig = sig;
      selectEl.replaceChildren(h("option", { value: "", text: S.history_all_devices }),
        ...state.devices.map((d) => h("option", { value: d.id, text: d.name })));
      selectEl.value = filter;
    }

    async function load(refresh) {
      version = state.historyVersion;
      const mine = ++seq;
      try {
        const r = await api.call("getHistory", { id: filter, refresh });
        if (mine !== seq) return;
        data = r;
        show();
      } catch (e) {
        snackbar(errorMessage(e));
      }
    }

    function show() {
      const now = Date.now();
      let text = "";
      if (filter && !data.hasAgent) text = S.history_note_no_agent;
      else if (filter && data.agent === "outdated") text = S.history_note_outdated;
      note.textContent = text;
      note.classList.toggle("hidden", !text);
      if (!data.events.length) {
        list.replaceChildren(h("div", { class: "placeholder" }, h("span", { class: "ni" }, icon("history", "", 28)), h("span", { text: S.history_empty })));
        moreButton.classList.add("hidden");
        return;
      }
      const nodes = [];
      let day = null;
      for (const e of data.events.slice(0, limit)) {
        const key = startOfDay(e.time);
        if (key !== day) {
          day = key;
          nodes.push(h("div", { class: "day", text: dayLabel(e.time, now) }));
        }
        nodes.push(eventRow(e, now, { withName: !filter, clockOnly: true }));
      }
      list.replaceChildren(...nodes);
      moreButton.classList.toggle("hidden", data.events.length <= limit);
    }
  }

  /** Élément SVG (graphiques des mesures). */
  function svgEl(tag, attrs = {}, text) {
    const el = document.createElementNS(SVG_NS, tag);
    for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, String(v));
    if (text != null) el.textContent = text;
    return el;
  }

  const percentAxis = (v) => `${Math.round(v)}\u00a0%`;
  const celsiusAxis = (v) => `${Math.round(v)}°`;

  /**
   * Graphique des mesures : une courbe par série (trait plein : moyenne, trait fin : maximum), coupée
   * quand le PC n'a rien enregistré (éteint). Info-bulle au survol.
   */
  function metricsChart({ points, from, to, step, series, percent }) {
    const W = 516, H = 176, L = 36, R = 8, T = 10, B = 22;
    const wrap = h("div", { class: "mchart" });
    const svg = svgEl("svg", { viewBox: `0 0 ${W} ${H}`, role: "img" });
    const values = [];
    for (const p of points) for (const s of series) for (const k of [s.key, s.maxKey]) if (p[k] != null) values.push(p[k]);
    let lo = 0;
    let hi = 100;
    if (!percent) {
      // Graduations rondes : 4 intervalles de 5, 10, 15 ou 20 °C.
      lo = Math.max(0, Math.floor((Math.min(...values) - 5) / 10) * 10);
      const span = Math.max(20, Math.max(...values) + 5 - lo);
      hi = lo + Math.ceil(span / 20) * 20;
    }
    const x = (t) => L + ((t - from) / (to - from)) * (W - L - R);
    const y = (v) => T + (1 - (Math.min(hi, Math.max(lo, v)) - lo) / (hi - lo)) * (H - T - B);
    for (let i = 0; i <= 4; i++) {
      const v = lo + ((hi - lo) * i) / 4;
      svg.append(svgEl("line", { x1: L, x2: W - R, y1: y(v), y2: y(v), class: "grid" }),
        svgEl("text", { x: L - 6, y: y(v) + 3.5, class: "axis", "text-anchor": "end" }, (percent ? percentAxis : celsiusAxis)(v)));
    }
    const short = to - from <= 36 * 3600e3;
    const fmtX = (t) => (short ? clockFmt : shortDateFmt).format(t);
    for (let i = 0; i <= 4; i++) {
      const t = from + ((to - from) * i) / 4;
      svg.append(svgEl("text", { x: x(t), y: H - 6, class: "axis", "text-anchor": i === 0 ? "start" : i === 4 ? "end" : "middle" }, fmtX(t)));
    }
    const gap = step * 1000 * 2.5;
    for (const s of series) {
      for (const [key, kind] of [[s.maxKey, "max"], [s.key, "avg"]]) {
        let d = "";
        let prev = null;
        for (const p of points) {
          const v = p[key];
          if (v == null) {
            prev = null;
            continue;
          }
          d += (prev == null || p.t - prev > gap ? "M" : "L") + x(p.t).toFixed(1) + " " + y(v).toFixed(1);
          prev = p.t;
        }
        if (d) svg.append(svgEl("path", { d, class: `mline ${s.cls} ${kind}` }));
      }
    }
    const cursor = svgEl("line", { y1: T, y2: H - B, class: "cursor hidden" });
    svg.append(cursor);
    const tip = h("div", { class: "mtip hidden" });
    wrap.append(svg, tip);
    svg.addEventListener("mousemove", (e) => {
      const r = svg.getBoundingClientRect();
      const t = from + ((((e.clientX - r.left) / r.width) * W - L) / (W - L - R)) * (to - from);
      let best = null;
      for (const p of points) if (best == null || Math.abs(p.t - t) < Math.abs(best.t - t)) best = p;
      if (!best || Math.abs(best.t - t) > gap) {
        cursor.classList.add("hidden");
        tip.classList.add("hidden");
        return;
      }
      cursor.setAttribute("x1", x(best.t));
      cursor.setAttribute("x2", x(best.t));
      cursor.classList.remove("hidden");
      const fmtV = (v) => (v == null ? "—" : percent ? percentText(v) : celsius(v));
      tip.replaceChildren(h("b", { text: (short ? clockFmt : fullFmt).format(best.t) }),
        ...series.filter((s) => best[s.key] != null).map((s) => h("div", {},
          h("i", { class: s.cls }), `${s.label} ${fmtV(best[s.key])}`, h("small", { text: ` max ${fmtV(best[s.maxKey])}` }))));
      tip.classList.remove("hidden");
      const left = ((x(best.t) / W) * r.width);
      tip.style.left = Math.min(Math.max(0, left - tip.offsetWidth / 2), r.width - tip.offsetWidth) + "px";
    });
    svg.addEventListener("mouseleave", () => {
      cursor.classList.add("hidden");
      tip.classList.add("hidden");
    });
    return wrap;
  }

  /** Mesures d'un PC dans le temps : températures, utilisation et journal, du PC ou des archives. */
  function openMetricsSheet(deviceId) {
    const d = deviceById(deviceId);
    if (!d) return;
    const DAY = 86400e3;
    const presets = [["24h", S.metrics_24h, DAY], ["7d", S.metrics_7d, 7 * DAY], ["30d", S.metrics_30d, 30 * DAY], ["90d", S.metrics_90d, 90 * DAY]];
    let period = "24h";
    let seq = 0;
    const select = h("select", { "aria-label": S.metrics_period });
    const presetOptions = presets.map(([v, label]) => h("option", { value: v, text: label }));
    select.replaceChildren(...presetOptions);
    select.addEventListener("change", () => {
      period = select.value;
      load();
    });
    const notes = h("div", { class: "hist-note hidden" });
    const charts = h("div", { class: "mcharts" });
    const journal = h("div", { class: "hist" });
    openSheet({
      title: S.metrics_title,
      subtitle: d.name,
      headExtra: [select, iconBtn("refresh", S.history_refresh, () => load(), "flat")],
      body: [notes, charts, h("div", { class: "hist-head", text: S.metrics_journal }), journal],
      foot: h("div", { class: "hist-note grow", text: S.metrics_sources }),
    });
    loadMonths();
    load();

    async function loadMonths() {
      try {
        const r = await api.call("metricsMonths", {});
        if (!r.months?.length) return;
        const group = h("optgroup", { label: S.metrics_months });
        const monthFmt = new Intl.DateTimeFormat("fr-FR", { month: "long", year: "numeric" });
        for (const m of r.months) {
          const [yy, mm] = m.split("-").map(Number);
          group.append(h("option", { value: "m:" + m, text: monthFmt.format(new Date(yy, mm - 1, 1)) }));
        }
        select.replaceChildren(...presetOptions, group);
        select.value = period;
      } catch {
        // Archives illisibles : seules les périodes récentes sont proposées.
      }
    }

    function range() {
      const now = Date.now();
      if (period.startsWith("m:")) {
        const [yy, mm] = period.slice(2).split("-").map(Number);
        return [new Date(yy, mm - 1, 1).getTime(), Math.min(new Date(yy, mm, 1).getTime(), now)];
      }
      const preset = presets.find(([v]) => v === period) || presets[0];
      return [now - preset[2], now];
    }

    async function load() {
      const mine = ++seq;
      const [from, to] = range();
      charts.replaceChildren(h("div", { class: "placeholder" }, h("span", { text: S.metrics_loading })));
      journal.replaceChildren();
      try {
        const [m, j] = await Promise.all([
          api.call("metricsRange", { id: deviceId, from, to, points: 600 }),
          api.call("metricsJournal", { id: deviceId, from, to }),
        ]);
        if (mine !== seq) return;
        render(m, j.events || []);
      } catch (e) {
        if (mine !== seq) return;
        charts.replaceChildren(h("div", { class: "placeholder" }, h("span", { text: errorMessage(e) })));
      }
    }

    function render(m, events) {
      const lines = [...(m.notes || [])];
      if (m.minutes) lines.unshift(fmt(S.metrics_coverage, formatLong(m.minutes * 60)) + ".");
      notes.replaceChildren(...lines.map((text) => h("div", { text })));
      notes.classList.toggle("hidden", !lines.length);
      if (!m.points.length) {
        charts.replaceChildren(h("div", { class: "placeholder" }, h("span", { class: "ni" }, icon("chart", "", 28)), h("span", { text: S.metrics_empty })));
      } else {
        const block = (title, series, percent) => {
          const fmtV = percent ? percentText : celsius;
          const legend = series.filter((s) => m.summary[s.key] != null).map((s) => h("span", { class: "mleg" },
            h("i", { class: s.cls }), h("b", { text: s.label }), fmt(S.metrics_stats, fmtV(m.summary[s.key]), fmtV(m.summary[s.maxKey]))));
          if (!legend.length) return null;
          return h("section", { class: "mblock" }, h("div", { class: "hist-head", text: title }),
            metricsChart({ points: m.points, from: m.from, to: m.to, step: m.step, series: series.filter((s) => m.summary[s.key] != null), percent }),
            h("div", { class: "mlegend" }, legend));
        };
        charts.replaceChildren(...[
          block(S.metrics_temp, [{ key: "ct", maxKey: "ctx", label: "CPU", cls: "s-cpu" }, { key: "gt", maxKey: "gtx", label: "GPU", cls: "s-gpu" }], false),
          block(S.metrics_load, [{ key: "cl", maxKey: "clx", label: "CPU", cls: "s-cpu" }, { key: "gl", maxKey: "glx", label: "GPU", cls: "s-gpu" }], true),
        ].filter(Boolean));
      }
      if (!events.length) {
        journal.replaceChildren(h("div", { class: "hist-note", text: S.metrics_journal_empty }));
        return;
      }
      const now = Date.now();
      const nodes = [];
      let day = null;
      for (const e of events.slice(0, 300)) {
        const key = startOfDay(e.time);
        if (key !== day) {
          day = key;
          nodes.push(h("div", { class: "day", text: dayLabel(e.time, now) }));
        }
        nodes.push(eventRow(e, now, { clockOnly: true }));
      }
      journal.replaceChildren(...nodes);
    }
  }

  // ---------------------------------------------------------------------------------------------
  // Actions sur les PC
  // ---------------------------------------------------------------------------------------------
  function openDeviceMenu(anchor, id) {
    const d = deviceById(id);
    if (!d) return;
    const own = state.devices.filter((x) => !x.shared);
    const index = own.indexOf(d);
    const items = [{ label: S.action_wake_menu, icon: "power", onClick: () => wake(id) }];
    if (d.canShutdown) {
      items.push(
        { label: S.action_shutdown, icon: "power", onClick: () => requestPower(id, "shutdown") },
        { label: S.action_reboot, icon: "restart", onClick: () => requestPower(id, "reboot") },
        { label: S.action_sleep, icon: "moon", onClick: () => requestPower(id, "sleep") });
    }
    if (d.shared) {
      items.push("sep", { label: S.history_title, icon: "history", onClick: () => openHistorySheet(id) });
      openMenu(anchor, items);
      return;
    }
    items.push("sep",
      { label: S.action_edit, icon: "edit", onClick: () => openEditSheet(id) },
      { label: S.history_title, icon: "history", onClick: () => openHistorySheet(id) });
    if (index > 0) items.push({ label: S.action_move_up, icon: "arrowUp", onClick: () => move(id, -1) });
    if (index < own.length - 1) items.push({ label: S.action_move_down, icon: "arrowDown", onClick: () => move(id, 1) });
    items.push("sep", { label: S.action_delete, icon: "delete", danger: true, onClick: () => confirmDelete(id) });
    openMenu(anchor, items);
  }

  async function wake(id) {
    const d = deviceById(id);
    if (!d) return;
    try {
      const r = await api.call("wake", { id });
      snackbar(r.ok ? fmt(S.message_wake_sent, d.name) : fmt(S.message_wake_error, r.error || ""));
    } catch (e) {
      snackbar(fmt(S.message_wake_error, e.message));
    }
  }

  function requestPower(id, action) {
    const d = deviceById(id);
    if (!d) return;
    if (!d.canShutdown) agentHelp(d);
    else if (state.settings.confirmPowerActions) powerConfirm(d, action);
    else power(d, action, false);
  }

  async function power(d, action, force) {
    try {
      const r = await api.call("power", { id: d.id, action, force });
      snackbar(r.ok ? fmt(sentMessage(action), d.name) : fmt(S.message_agent_error, d.name, agentErrorLabel(r.code)));
    } catch (e) {
      snackbar(errorMessage(e));
    }
  }

  async function move(id, offset) {
    try {
      await api.call("moveDevice", { id, offset });
    } catch (e) {
      snackbar(errorMessage(e));
    }
  }

  function powerConfirm(d, action) {
    let force = false;
    const body = [h("p", { text: S.confirm_power_text })];
    if (action !== "sleep") {
      const box = h("input", { type: "checkbox" });
      box.addEventListener("change", () => (force = box.checked));
      body.push(h("label", { class: "check-row" }, box, S.confirm_power_force));
    }
    const dialog = openDialog({
      iconName: action === "reboot" ? "restart" : action === "sleep" ? "moon" : "power",
      title: fmt(confirmTitle(action), d.name),
      body,
      actions: [
        { label: S.cancel, onClick: () => dialog.close() },
        {
          label: actionLabel(action),
          kind: action === "shutdown" ? "danger" : "primary",
          onClick: () => {
            dialog.close();
            power(d, action, force);
          },
        },
      ],
    });
  }

  function confirmDelete(id, onDeleted) {
    const d = deviceById(id);
    if (!d) return;
    const dialog = openDialog({
      title: fmt(S.confirm_delete_title, d.name),
      body: h("p", { text: S.confirm_delete_text }),
      actions: [
        { label: S.cancel, onClick: () => dialog.close() },
        {
          label: S.action_delete,
          kind: "danger",
          onClick: async () => {
            dialog.close();
            try {
              await api.call("deleteDevice", { id });
              onDeleted?.();
              snackbar(fmt(S.message_deleted, d.name));
            } catch (e) {
              snackbar(errorMessage(e));
            }
          },
        },
      ],
    });
  }

  function agentHelp(d) {
    const dialog = openDialog({
      iconName: "shield",
      title: S.agent_help_title,
      body: h("p", { text: fmt(S.agent_help_text, d.name) }),
      actions: [
        { label: S.cancel, onClick: () => dialog.close() },
        state.agent?.enabled ? { label: S.agent_download, kind: "sec", onClick: () => { dialog.close(); agentDownload.open(); } } : null,
        { label: S.action_configure, onClick: () => { dialog.close(); openEditSheet(d.id); } },
      ].filter(Boolean),
    });
  }

  // ---------------------------------------------------------------------------------------------
  // Fiche « ajouter / modifier un PC » (EditDeviceScreen)
  // ---------------------------------------------------------------------------------------------
  async function openEditSheet(id) {
    let data;
    try {
      data = await api.call("getDevice", { id: id || "" });
    } catch (e) {
      snackbar(errorMessage(e));
      return;
    }
    const isNew = data.isNew;
    const deviceId = isNew ? "" : id;
    const form = data.form;
    let testing = false;
    let keyVisible = false;
    let showAdvanced = false;

    const fields = {};
    const onChange = (key) => (value) => {
      form[key] = value;
      clearErrors();
      setTestResult(null);
    };
    const make = (key, label, helper, opts = {}) =>
      (fields[key] = field({ label, helper, value: form[key], onInput: onChange(key), onEnter: save, ...opts }));

    make("name", S.field_name, null);
    make("mac", S.field_mac, S.field_mac_help, { mono: true });
    make("host", S.field_host, S.field_host_help);
    make("agentPort", S.field_agent_port, null, { mono: true });
    const keyToggle = btn("ghost", S.show, () => {
      keyVisible = !keyVisible;
      fields.agentKey.input.type = keyVisible ? "text" : "password";
      keyToggle.textContent = keyVisible ? S.hide : S.show;
    });
    make("agentKey", S.field_agent_key, S.field_agent_key_help, { type: "password", mono: true, trailing: keyToggle });
    make("broadcast", S.field_broadcast, S.field_broadcast_help, { mono: true });
    make("wolPort", S.field_wol_port, S.field_wol_port_help, { mono: true });
    make("probePorts", S.field_probe_ports, S.field_probe_ports_help, { mono: true });
    make("secureOn", S.field_secure_on, S.field_secure_on_help, { mono: true });

    const agentSwitch = switchInput(form.agentEnabled, (checked) => {
      form.agentEnabled = checked;
      agentSection.classList.toggle("hidden", !checked);
      clearErrors();
      setTestResult(null);
    }, S.section_agent);

    const testButton = btn("sec", S.action_test_agent, testAgent, "shield");
    const testSpinner = h("span", { class: "spin hidden" });
    const testSlot = h("div");
    const agentSection = h("div", { class: `form-section${form.agentEnabled ? "" : " hidden"}` },
      fields.agentPort.wrap, fields.agentKey.wrap, h("div", { class: "test-row" }, testButton, testSpinner), testSlot);

    const advancedIcon = h("span", { class: "chev-slot" });
    const advancedButton = h("button", { class: "btn ghost expander", type: "button", onClick: () => setAdvanced(!showAdvanced) }, S.section_advanced, advancedIcon);
    const advancedSection = h("div", { class: "form-section hidden" },
      fields.broadcast.wrap, fields.wolPort.wrap, fields.probePorts.wrap, fields.secureOn.wrap);

    const sheet = openSheet({
      title: isNew ? S.edit_title_new : S.edit_title,
      subtitle: isNew ? null : form.name,
      closeOnScrim: false,
      body: [
        h("div", { class: "pairing" },
          h("b", { text: S.pairing_title }),
          h("p", { text: S.pairing_text }),
          h("div", {}, btn("primary", S.action_paste_link, pasteLink, "paste"))),
        h("div", { class: "form-section" }, h("div", { class: "form-title", text: S.section_device }), fields.name.wrap, fields.mac.wrap, fields.host.wrap),
        h("div", { class: "form-section" },
          h("div", { class: "toggle-row" }, h("div", { class: "texts" }, h("b", { text: S.section_agent }), h("span", { text: S.section_agent_help })), agentSwitch.el),
          agentSection),
        advancedButton,
        advancedSection,
      ],
      foot: [
        isNew ? null : btn("danger", S.action_delete, () => confirmDelete(deviceId, () => sheet.close()), "delete"),
        h("div", { class: "grow" }),
        btn("sec", S.cancel, () => sheet.close()),
        btn("primary", S.save, save),
      ],
    });
    setAdvanced(false);
    if (isNew) fields.name.input.focus();

    function setAdvanced(open) {
      showAdvanced = open;
      advancedSection.classList.toggle("hidden", !open);
      advancedIcon.replaceChildren(icon(open ? "up" : "down", "small"));
    }

    function clearErrors() {
      for (const f of Object.values(fields)) f.setError(null);
    }

    function setTestResult(result) {
      if (!result) {
        testSlot.replaceChildren();
        return;
      }
      let text;
      if (result.ok) text = fmt(S.test_ok, result.status.hostname, osLabel(result.status.os), result.status.version);
      else if (result.invalid) text = result.invalid;
      else text = agentErrorLabel(result.code);
      const detail = result.code === "UNREACHABLE" ? h("small", { class: "result-help", text: S.hint_unreachable }) : null;
      testSlot.replaceChildren(h("div", { class: `result ${result.ok ? "ok" : "ko"}` },
        icon(result.ok ? "check" : "warning", "small"), h("span", { class: "selectable" }, h("span", { text }), detail)));
    }

    async function testAgent() {
      if (testing) return;
      testing = true;
      testButton.disabled = true;
      testSpinner.classList.remove("hidden");
      setTestResult(null);
      try {
        setTestResult(await api.call("testAgent", { host: form.host, port: form.agentPort, key: form.agentKey }));
      } catch (e) {
        snackbar(errorMessage(e));
      } finally {
        testing = false;
        testButton.disabled = false;
        testSpinner.classList.add("hidden");
      }
    }

    async function save() {
      let r;
      try {
        r = await api.call("saveDevice", { id: deviceId, form });
      } catch (e) {
        snackbar(errorMessage(e));
        return;
      }
      if (r.ok) {
        sheet.close();
        if (r.id) select(r.id, false);
        return;
      }
      const map = { NAME: "name", MAC: "mac", HOST: "host", AGENT_PORT: "agentPort", AGENT_KEY: "agentKey", BROADCAST: "broadcast", WOL_PORT: "wolPort", PROBE_PORTS: "probePorts", SECURE_ON: "secureOn" };
      let firstError = null;
      for (const [fieldName, message] of Object.entries(r.errors || {})) {
        const f = fields[map[fieldName]];
        if (!f) continue;
        f.setError(message);
        if (["broadcast", "wolPort", "probePorts", "secureOn"].includes(map[fieldName])) setAdvanced(true);
        firstError = firstError || f;
      }
      firstError?.input.focus();
    }

    async function pasteLink() {
      let text = "";
      try {
        text = (await api.call("readClipboard")).text || "";
      } catch {
        // Presse-papiers indisponible : on passe par la saisie.
      }
      if (!text && navigator.clipboard?.readText) {
        // Le navigateur peut demander une autorisation sans jamais répondre : attente bornée.
        const timeout = new Promise((resolve) => setTimeout(() => resolve(""), 800));
        text = await Promise.race([navigator.clipboard.readText().catch(() => ""), timeout]);
      }
      text = text.trim();
      if (/^wolagent:\/\//i.test(text)) applyPairing(text);
      else pasteDialog();
    }

    function pasteDialog() {
      const input = field({ multiline: true, placeholder: "wolagent://pair?…", onInput: (v) => dialog.setActions(actions(v)) });
      const actions = (v) => [
        { label: S.cancel, onClick: () => dialog.close() },
        { label: S.ok, disabled: !v.trim(), onClick: () => { dialog.close(); applyPairing(input.input.value); } },
      ];
      const dialog = openDialog({
        iconName: "paste",
        title: S.paste_title,
        body: [h("p", { text: S.paste_text }), input.wrap],
        actions: actions(""),
      });
    }

    async function applyPairing(text) {
      let r;
      try {
        r = await api.call("parsePairing", { text });
      } catch (e) {
        snackbar(errorMessage(e));
        return;
      }
      if (!r.ok) {
        alertDialog(S.pairing_error_title, r.error);
        return;
      }
      const info = r.info;
      const set = (key, value) => {
        form[key] = value;
        fields[key].input.value = value;
      };
      if (!form.name.trim()) set("name", info.name);
      if (info.mac) set("mac", info.mac);
      set("host", info.host);
      set("agentPort", String(info.port));
      set("agentKey", info.key);
      form.agentEnabled = true;
      agentSwitch.input.checked = true;
      agentSection.classList.remove("hidden");
      clearErrors();
      setTestResult(null);
      alertDialog(S.pairing_ok_title, S.pairing_ok_text);
    }
  }

  // ---------------------------------------------------------------------------------------------
  // Mises à jour intégrées : proposition (nouveautés), téléchargement vérifié, redémarrage
  // ---------------------------------------------------------------------------------------------
  const updates = (() => {
    let dialog = null;
    let prompted = ""; // version déjà proposée d'elle-même pendant cette session

    /** Nouveautés : titres « ### », listes « - », « **gras** » (texte uniquement, jamais de HTML). */
    function notesView(text) {
      const inline = (line) => line.split("**").map((part, i) => (i % 2 ? h("b", { text: part }) : part));
      const out = [];
      let list = null;
      for (const raw of (text || "").split("\n")) {
        const line = raw.trim();
        if (!line) {
          list = null;
        } else if (line.startsWith("### ")) {
          list = null;
          out.push(h("h3", {}, inline(line.slice(4))));
        } else if (/^[-*] /.test(line)) {
          if (!list) out.push((list = h("ul")));
          list.append(h("li", {}, inline(line.slice(2))));
        } else {
          list = null;
          out.push(h("p", {}, inline(line)));
        }
      }
      return h("div", { class: "notes" }, out);
    }

    function dateText(iso) {
      const d = new Date(iso + "T12:00:00");
      return isNaN(d) ? "" : d.toLocaleDateString("fr-FR", { day: "numeric", month: "long", year: "numeric" });
    }

    function open() {
      const a = state.update?.available;
      if (!a || dialog) return;
      const bar = h("i");
      const progress = h("div", { class: "upd-bar hidden" }, bar);
      const status = h("p", { class: "upd-status hidden" });
      const meta = [fmt(S.update_meta_version, a.version), dateText(a.date), fmt(S.update_meta_size, (a.size / 1048576).toFixed(1).replace(".", ","))];
      const handle = openDialog({
        iconName: "download",
        title: S.update_title,
        body: [
          h("p", { class: "upd-meta", text: meta.filter(Boolean).join(" · ") }),
          notesView(a.notes),
          h("p", { class: "upd-keep" }, icon("shield", "small"), S.update_keep_data),
          progress,
          status,
        ],
        onDismiss: later,
      });
      let sig = "";
      dialog = {
        refresh() {
          const u = state.update;
          if (!u?.available) return close();
          const busy = !!u.stage;
          const s = [u.stage, Math.round((u.progress || 0) * 100), u.error].join("|");
          if (s === sig) return;
          sig = s;
          progress.classList.toggle("hidden", !busy);
          bar.style.width = Math.round((u.progress || 0) * 100) + "%";
          const text = u.stage === "downloading" ? fmt(S.update_downloading, Math.round((u.progress || 0) * 100))
            : u.stage === "installing" ? S.update_installing : u.error || "";
          status.textContent = text;
          status.classList.toggle("hidden", !text);
          status.classList.toggle("bad", !busy && !!u.error);
          handle.setActions([
            { label: S.update_later, disabled: busy, onClick: later },
            { label: u.error ? S.update_retry : S.update_install, disabled: busy, onClick: install },
          ]);
        },
      };
      dialog.refresh();

      function close() {
        handle.close();
        dialog = null;
      }
      function later() {
        if (state.update?.stage) return;
        close();
        api.call("postponeUpdate").catch(() => {});
      }
      async function install() {
        try {
          await api.call("installUpdate");
        } catch (e) {
          snackbar(errorMessage(e));
        }
      }
    }

    /** Recherche demandée depuis les réglages. */
    async function checkNow() {
      if (state.update?.available) return open();
      try {
        const u = await api.call("checkUpdate");
        if (u.available) {
          prompted = u.available.version;
          open();
        } else {
          snackbar(S.update_up_to_date);
        }
      } catch (e) {
        snackbar(errorMessage(e));
      }
    }

    /** Après chaque changement d'état : propose une nouvelle version une fois par session. */
    function render() {
      const u = state.update;
      dialog?.refresh();
      if (!u?.available || u.postponed || dialog || prompted === u.available.version) return;
      if (document.querySelector(".dialog-wrap, .sheet")) return; // pas par-dessus une autre fenêtre
      prompted = u.available.version;
      open();
    }

    return { open, checkNow, render, notesView };
  })();

  // ---------------------------------------------------------------------------------------------
  // Agent : dernière version publiée, vérifiée par le moteur, installée sur ce PC ou enregistrée
  // ---------------------------------------------------------------------------------------------
  const agentDownload = (() => {
    let current = null;

    function open() {
      if (current) return;
      const PLATFORMS = ["windows-amd64", "windows-arm64"];
      const archName = (p) => (p === "windows-arm64" ? S.agent_dl_arch_arm64 : S.agent_dl_arch_amd64);
      let info = null;
      let error = "";
      let loading = true;
      let target = "";
      const meta = h("p", { class: "upd-meta" });
      const notes = h("div");
      const selectEl = h("select", { "aria-label": S.agent_dl_arch });
      selectEl.addEventListener("change", () => { target = selectEl.value; refresh(); });
      const archRow = h("div", { class: "agent-arch" }, h("span", { text: S.agent_dl_arch }), selectEl);
      const verified = h("p", { class: "upd-keep" }, icon("shield", "small"), S.agent_dl_verified);
      const otherOs = h("button", { type: "button", class: "linkish", onClick: () => openUrl(RELEASES_URL) }, S.agent_dl_other_os);
      const bar = h("i");
      const progress = h("div", { class: "upd-bar hidden" }, bar);
      const status = h("p", { class: "upd-status" });
      const details = h("div", { class: "agent-dl hidden" }, meta, notes, archRow, verified, otherOs);
      const handle = openDialog({
        iconName: "download",
        title: S.agent_dl_title,
        body: [details, progress, status],
        // Échap ou clic à côté : le dialogue est déjà fermé (un téléchargement en cours se termine).
        onDismiss: () => { current = null; },
      });
      current = { refresh };
      refresh();
      load();

      async function load() {
        loading = true;
        error = "";
        refresh();
        try {
          info = await api.call("agentInfo");
          target = target || info.platform;
          selectEl.replaceChildren(...PLATFORMS.filter((p) => info.sizes[p]).map((p) =>
            h("option", { value: p, text: p === info.platform ? fmt(S.agent_dl_this_pc, archName(p)) : archName(p) })));
          selectEl.value = target;
          notes.replaceChildren(updates.notesView(info.notes));
        } catch (e) {
          error = errorMessage(e);
        }
        loading = false;
        refresh();
      }

      function refresh() {
        const busy = !!state.agent?.busy;
        const pct = Math.round((state.agent?.progress || 0) * 100);
        details.classList.toggle("hidden", !info);
        if (info) {
          const size = info.sizes[target] || 0;
          meta.textContent = fmt(S.agent_dl_meta, info.version, (size / 1048576).toFixed(1).replace(".", ","));
        }
        progress.classList.toggle("hidden", !busy);
        bar.style.width = pct + "%";
        const text = loading ? S.agent_dl_loading : busy ? fmt(S.agent_dl_downloading, pct) : error;
        status.textContent = text;
        status.classList.toggle("hidden", !text);
        status.classList.toggle("bad", !loading && !busy && !!error);
        selectEl.disabled = busy;
        const actions = [{ label: S.agent_dl_close, disabled: busy, onClick: close }];
        if (!info && !loading) actions.push({ label: S.agent_dl_retry, onClick: load });
        if (info) {
          actions.push({ label: S.agent_dl_save, kind: "sec", disabled: busy, onClick: () => run(false) });
          actions.push({ label: S.agent_dl_install, disabled: busy || target !== info.platform, onClick: () => run(true) });
        }
        handle.setActions(actions);
      }

      async function run(install) {
        error = "";
        refresh();
        try {
          const r = await api.call("agentDownload", { platform: install ? info.platform : target, install });
          if (r?.cancelled) return refresh();
          close();
          if (install) alertDialog(S.agent_dl_started_title, S.agent_dl_started_text);
          else alertDialog(S.agent_dl_saved_title, fmt(S.agent_dl_saved_text, r.path));
        } catch (e) {
          error = errorMessage(e);
          refresh();
        }
      }

      function close() {
        handle.close();
        current = null;
      }
    }

    /** Après chaque changement d'état : avancement du téléchargement. */
    function render() {
      current?.refresh();
    }

    return { open, render };
  })();

  // ---------------------------------------------------------------------------------------------
  // Onglet « Réglages » (SettingsScreen)
  // ---------------------------------------------------------------------------------------------
  // ---------------------------------------------------------------------------------------------
  // Partage des PC entre personnes (docs/PARTAGE.md) : tout le chiffrement est dans le moteur Go.
  // ---------------------------------------------------------------------------------------------
  const sharing = (() => {
    const ownDevices = () => state.devices.filter((d) => !d.shared);

    async function copy(text) {
      try {
        await api.call("copyText", { text });
      } catch {
        try {
          await navigator.clipboard.writeText(text);
        } catch (e) {
          snackbar(errorMessage(e));
          return;
        }
      }
      snackbar(S.share_copied);
    }

    /** QR code dessiné par le moteur (SVG de carrés, sans texte ni script). */
    function qrView(svg) {
      const box = h("div", { class: "qr" });
      box.innerHTML = svg;
      return box;
    }

    function linkBox(link) {
      return h("div", { class: "share-link" },
        h("code", { class: "mono selectable", text: link }),
        iconBtn("copy", S.share_copy_link, () => copy(link), "flat"));
    }

    function codeView(code) {
      return h("div", { class: "share-code mono", text: code });
    }

    /** Dialogue « coller un lien » ; le presse-papiers est proposé d'emblée s'il contient un lien de partage. */
    async function pasteDialog({ title, text, placeholder, onLink }) {
      let initial = "";
      try {
        initial = (await api.call("readClipboard")).text || "";
      } catch {
        // Presse-papiers indisponible : saisie manuelle.
      }
      if (!/^wolshare:\/\//i.test(initial.trim())) initial = "";
      const input = field({ multiline: true, placeholder, value: initial.trim(), onInput: (v) => dialog.setActions(actions(v)) });
      const actions = (v) => [
        { label: S.cancel, onClick: () => dialog.close() },
        { label: S.ok, disabled: !v.trim(), onClick: () => onLink(input.input.value.trim(), dialog) },
      ];
      const dialog = openDialog({ iconName: "paste", title, body: [h("p", { text }), input.wrap], actions: actions(initial) });
    }

    // --- Je partage mes PC ---

    function setupDialog() {
      let name = state.share.owner?.name || "";
      const nameField = field({ label: S.share_field_name, value: name, onInput: (v) => { name = v; refresh(); }, onEnter: () => name.trim() && go() });
      const dialog = openDialog({
        iconName: "share",
        title: S.share_start,
        body: [h("p", { text: S.share_setup_text }), nameField.wrap],
        actions: [],
      });
      refresh();
      function refresh() {
        dialog.setActions([
          { label: S.cancel, onClick: () => dialog.close() },
          { label: S.share_connect, disabled: !name.trim(), onClick: go },
        ]);
      }
      function go() {
        dialog.close();
        startLogin(name.trim());
      }
    }

    async function startLogin(name) {
      let r;
      try {
        r = await api.call("shareLogin", { name });
      } catch (e) {
        alertDialog(S.share_login_title, e.message);
        return;
      }
      const status = h("p", { class: "muted", text: S.share_login_waiting });
      // Code copié d'abord : il n'y a plus qu'à le coller sur la page de GitHub.
      const openGitHub = async () => {
        await copy(r.code);
        openUrl(GITHUB_DEVICE_URL);
      };
      const onState = () => {
        const sh = state.share;
        if (sh.login?.error) {
          status.className = "bad";
          setText(status, sh.login.error);
        } else if (!sh.login && sh.owner?.connected) {
          close();
          snackbar(S.share_login_done);
        }
      };
      const close = () => {
        stateListeners.delete(onState);
        dialog.close();
      };
      const dialog = openDialog({
        iconName: "cloud",
        title: S.share_login_title,
        body: [h("p", { text: S.share_login_text }), codeView(r.code), status],
        actions: [
          { label: S.cancel, onClick: () => { api.call("shareCancelLogin").catch(() => {}); close(); } },
          { label: S.share_login_copy, kind: "sec", onClick: () => copy(r.code) },
          { label: S.share_login_open, onClick: openGitHub },
        ],
        onDismiss: () => {
          stateListeners.delete(onState);
          api.call("shareCancelLogin").catch(() => {});
        },
      });
      stateListeners.add(onState);
      openGitHub();
    }

    async function inviteDialog() {
      let r;
      try {
        r = await api.call("shareInvite");
      } catch (e) {
        snackbar(errorMessage(e));
        return;
      }
      const dialog = openDialog({
        iconName: "share",
        title: S.share_invite,
        body: [h("p", { text: S.share_invite_text }), qrView(r.qr), linkBox(r.link)],
        actions: [{ label: S.share_copy_link, kind: "sec", onClick: () => copy(r.link) }, { label: S.close, onClick: () => dialog.close() }],
      });
    }

    function requestPasteDialog() {
      if (!ownDevices().length) {
        alertDialog(S.share_add_request, S.share_no_devices);
        return;
      }
      pasteDialog({
        title: S.share_add_request,
        text: S.share_request_paste_text,
        placeholder: "wolshare://request?…",
        onLink: async (text, dialog) => {
          try {
            const r = await api.call("shareReadRequest", { text });
            if (!r.ok) {
              alertDialog(S.share_add_request, r.error);
              return;
            }
            dialog.close();
            rightsDialog({ name: r.name, device: r.device, code: r.code, rights: r.rights || {}, isNew: !r.rights });
          } catch (e) {
            snackbar(errorMessage(e));
          }
        },
      });
    }

    /** Choix des PC partagés et des droits (nouvelle personne ou modification). */
    function rightsDialog({ name, device, code, rights, isNew }) {
      const chosen = { ...rights };
      const rows = ownDevices().map((d) => {
        const selectEl = h("select", { "aria-label": d.name },
          h("option", { value: "", text: S.share_right_none }),
          h("option", { value: "wake", text: S.share_right_wake }),
          d.canShutdown ? h("option", { value: "full", text: S.share_right_full }) : null);
        selectEl.value = chosen[d.id] === "full" && !d.canShutdown ? "wake" : chosen[d.id] || "";
        selectEl.addEventListener("change", () => {
          if (selectEl.value) chosen[d.id] = selectEl.value;
          else delete chosen[d.id];
          refresh();
        });
        return h("div", { class: "right-row" }, h("span", { class: "ni" }, icon("monitor", "", 16)), h("b", { text: d.name }), selectEl);
      });
      const body = [];
      if (code) {
        body.push(h("p", { text: fmt(S.share_verify, name) }), codeView(code), h("p", { class: "muted small", text: S.share_verify_help }));
      }
      body.push(h("div", { class: "form-title", text: S.share_rights }), h("div", { class: "rights" }, rows));
      const dialog = openDialog({ iconName: "users", title: fmt(isNew ? S.share_grant_title : S.share_edit_title, name), body, actions: [] });
      refresh();
      function refresh() {
        const actions = [{ label: S.cancel, onClick: () => dialog.close() }];
        if (!isNew) actions.unshift({ label: S.share_revoke, kind: "danger", onClick: () => { dialog.close(); revokeDialog(name, device); } });
        actions.push({
          label: isNew ? S.share_grant : S.save,
          disabled: !Object.keys(chosen).length,
          onClick: async () => {
            try {
              await api.call("shareGrant", { device, name, rights: chosen });
              dialog.close();
              snackbar(fmt(S.share_granted, name));
            } catch (e) {
              snackbar(errorMessage(e));
            }
          },
        });
        dialog.setActions(actions);
      }
    }

    function revokeDialog(name, device) {
      const dialog = openDialog({
        iconName: "users",
        title: fmt(S.share_revoke_title, name),
        body: h("p", { text: S.share_revoke_text }),
        actions: [
          { label: S.cancel, onClick: () => dialog.close() },
          {
            label: S.share_revoke,
            kind: "danger",
            onClick: async () => {
              dialog.close();
              try {
                await api.call("shareRevoke", { device });
                snackbar(fmt(S.share_revoked, name));
              } catch (e) {
                snackbar(errorMessage(e));
              }
            },
          },
        ],
      });
    }

    function stopDialog() {
      const dialog = openDialog({
        iconName: "warning",
        title: S.share_stop_title,
        body: h("p", { text: S.share_stop_text }),
        actions: [
          { label: S.cancel, onClick: () => dialog.close() },
          {
            label: S.share_stop,
            kind: "danger",
            onClick: async () => {
              dialog.close();
              try {
                await api.call("shareStop");
              } catch (e) {
                alertDialog(S.share_stop, e.message);
              }
            },
          },
        ],
      });
    }

    // --- Je reçois les PC d'une autre personne ---

    function invitePasteDialog() {
      pasteDialog({
        title: S.share_request,
        text: S.share_invite_paste_text,
        placeholder: "wolshare://invite?…",
        onLink: async (text, dialog) => {
          try {
            const r = await api.call("shareReadInvite", { text });
            if (!r.ok) {
              alertDialog(S.share_request, r.error);
              return;
            }
            dialog.close();
            nameDialog(text, r.name);
          } catch (e) {
            snackbar(errorMessage(e));
          }
        },
      });
    }

    function nameDialog(link, ownerName) {
      let name = "";
      const nameField = field({ label: fmt(S.share_field_my_name, ownerName), onInput: (v) => { name = v; refresh(); }, onEnter: () => name.trim() && send() });
      const dialog = openDialog({ iconName: "users", title: S.share_request, body: [nameField.wrap], actions: [] });
      refresh();
      function refresh() {
        dialog.setActions([{ label: S.cancel, onClick: () => dialog.close() }, { label: S.ok, disabled: !name.trim(), onClick: send }]);
      }
      async function send() {
        try {
          const r = await api.call("shareRequest", { text: link, name: name.trim() });
          dialog.close();
          requestDialog(r);
        } catch (e) {
          nameField.setError(e.message);
        }
      }
    }

    function requestDialog(r) {
      const dialog = openDialog({
        iconName: "send",
        title: fmt(S.share_request_title, r.ownerName),
        body: [
          h("p", { text: fmt(S.share_request_text, r.ownerName) }),
          qrView(r.qr),
          linkBox(r.link),
          h("p", { text: fmt(S.share_request_code, r.ownerName) }),
          codeView(r.code),
          h("p", { class: "muted small", text: fmt(S.share_request_wait, r.ownerName) }),
        ],
        actions: [{ label: S.share_copy_link, kind: "sec", onClick: () => copy(r.link) }, { label: S.close, onClick: () => dialog.close() }],
      });
    }

    async function showRequest(owner) {
      try {
        requestDialog(await api.call("shareRequestView", { owner }));
      } catch (e) {
        snackbar(errorMessage(e));
      }
    }

    function accessDialog(a) {
      const body = [];
      if (a.active) body.push(h("p", { text: a.devices.length ? fmt(S.share_access_devices, a.devices.join(", ")) : S.share_access_none }));
      else if (a.removed) body.push(h("p", { text: fmt(S.share_access_removed, a.ownerName) }));
      else body.push(h("p", { text: fmt(S.share_request_wait, a.ownerName) }));
      if (a.error) body.push(h("p", { class: "bad", text: a.error }));
      const leaveLabel = a.active ? S.share_leave : a.removed ? S.share_remove : S.share_cancel_request;
      const actions = [{ label: leaveLabel, kind: "danger", onClick: () => { dialog.close(); leaveDialog(a, leaveLabel); } }];
      if (!a.active && !a.removed) actions.push({ label: S.share_show_request, kind: "sec", onClick: () => { dialog.close(); showRequest(a.owner); } });
      actions.push({ label: S.share_sync_now, kind: "sec", onClick: () => { api.call("shareSync").catch(() => {}); dialog.close(); } });
      actions.push({ label: S.close, onClick: () => dialog.close() });
      const dialog = openDialog({ iconName: "monitor", title: fmt(S.share_access_title, a.ownerName), body, actions });
    }

    function leaveDialog(a, label) {
      const dialog = openDialog({
        title: fmt(S.share_leave_title, a.ownerName),
        body: h("p", { text: S.share_leave_text }),
        actions: [
          { label: S.cancel, onClick: () => dialog.close() },
          {
            label,
            kind: "danger",
            onClick: async () => {
              dialog.close();
              try {
                await api.call("shareLeave", { owner: a.owner });
              } catch (e) {
                snackbar(errorMessage(e));
              }
            },
          },
        ],
      });
    }

    // --- Sections des réglages ---

    /** Partage illisible au démarrage (mis de côté par le moteur) : signalé en tête de section. */
    function loadErrorItems(sh) {
      if (!sh.error) return [];
      const item = settingItem("warning", S.share_load_error_title, fmt(S.share_load_error, sh.error));
      item.querySelector(".supporting").classList.add("bad");
      return [item];
    }

    function ownerItems(sh) {
      const o = sh.owner;
      if (!o) {
        return [...loadErrorItems(sh),
          settingItem("share", S.share_start, sh.canLogin ? S.share_start_help : S.share_unavailable, sh.canLogin ? setupDialog : null)];
      }
      if (!o.connected) {
        return [...loadErrorItems(sh),
          settingItem("cloud", S.share_reconnect, o.error || S.share_reconnect_help, sh.canLogin ? () => startLogin(o.name) : null),
          settingItem("delete", S.share_stop, S.share_stop_help, stopDialog, { danger: true, chevron: false }),
        ];
      }
      const names = new Map(ownDevices().map((d) => [d.id, d.name]));
      const items = [...loadErrorItems(sh),
        settingItem("share", S.share_invite, S.share_invite_help, inviteDialog),
        settingItem("paste", S.share_add_request, S.share_add_request_help, requestPasteDialog),
      ];
      for (const p of o.people) {
        const summary = p.devices.join(", ");
        items.push(settingItem("users", p.name, p.published || o.publishing ? summary : fmt(S.share_person_waiting, summary),
          () => rightsDialog({ name: p.name, device: p.device, rights: Object.fromEntries(Object.entries(p.rights).filter(([id]) => names.has(id))), isNew: false })));
      }
      const status = o.publishing ? S.share_publishing : o.error || S.share_account_help;
      const account = settingItem("cloud", fmt(S.share_account, o.user), status);
      if (o.error) account.querySelector(".supporting").classList.add("bad");
      items.push(account, settingItem("delete", S.share_stop, S.share_stop_help, stopDialog, { danger: true, chevron: false }));
      return items;
    }

    function accessSummary(a, now) {
      if (a.error) return a.error;
      if (a.removed) return fmt(S.share_access_removed, a.ownerName);
      if (!a.active) return fmt(S.share_access_pending, a.ownerName);
      const list = a.devices.length ? a.devices.join(", ") : S.share_access_none;
      return fmt(S.share_access_active, list, formatDuration(Math.max(0, now - (a.synced || now))));
    }

    function receivedItems(sh, now) {
      const items = sh.received.map((a) => {
        const item = settingItem("monitor", fmt(S.share_access_title, a.ownerName), accessSummary(a, now), () => accessDialog(a));
        if (a.error || a.removed) item.querySelector(".supporting").classList.add("bad");
        return item;
      });
      items.push(settingItem("download", S.share_request, S.share_request_help, invitePasteDialog));
      return items;
    }

    /** Sections « Partager mes PC » et « PC partagés avec moi », reconstruites quand l'état change. */
    function sections() {
      const mine = h("section", { class: "card" });
      const received = h("section", { class: "card" });
      let sig = "";
      let minute = -1;
      return {
        els: [h("h2", { class: "section-title", text: S.section_share_mine }), mine,
          h("h2", { class: "section-title", text: S.section_share_received }), received],
        update() {
          const sh = state.share || { received: [] };
          const now = Date.now();
          const next = JSON.stringify([sh, ownDevices().map((d) => [d.id, d.name, d.canShutdown])]);
          // Les durées (« vérifié il y a… ») sont rafraîchies chaque minute.
          if (next === sig && Math.floor(now / 60000) === minute) return;
          sig = next;
          minute = Math.floor(now / 60000);
          mine.replaceChildren(...ownerItems(sh));
          received.replaceChildren(...receivedItems(sh, now));
        },
      };
    }

    return { sections, copy, codeView };
  })();

  // ---------------------------------------------------------------------------------------------
  // Sauvegardes automatiques (GitHub et dossier) et restauration depuis GitHub
  // ---------------------------------------------------------------------------------------------
  const backups = (() => {
    function lastText(target) {
      if (target.error) return target.error;
      return target.last ? fmt(S.backup_last, formatDuration(Math.max(0, Date.now() - target.last))) : S.backup_never;
    }

    function items(b) {
      if (!b.available) return [settingItem("cloud", S.section_backup_auto, S.backup_unavailable)];
      const restore = settingItem("download", S.backup_restore, S.backup_restore_help, restoreDialog);
      if (!b.enabled) {
        return [settingItem("shield", S.backup_enable, S.backup_enable_help, enableDialog), restore];
      }
      const github = settingItem("cloud", S.backup_github,
        b.github ? `${b.github.label} · ${lastText(b.github)}` : S.backup_github_off,
        b.github ? githubDialog : connect);
      if (b.github?.error) github.querySelector(".supporting").classList.add("bad");
      const folder = settingItem("history", S.backup_folder,
        b.folder ? `${b.folder.label} · ${lastText(b.folder)}` : S.backup_folder_off,
        b.folder ? folderDialog : pickFolder);
      if (b.folder?.error) folder.querySelector(".supporting").classList.add("bad");
      const now = settingItem("upload", S.backup_now,
        b.running ? S.backup_running : b.paused ? fmt(S.backup_paused, b.paused) : S.backup_now_help,
        b.running ? null : backupNow);
      if (b.paused) now.querySelector(".supporting").classList.add("bad");
      const a = b.archive || {};
      const archiveText = !b.github ? S.archive_need_github : !a.enabled ? S.archive_off : a.running ? S.archive_running :
        a.error ? a.error : a.last ? fmt(S.archive_last, formatDuration(Math.max(0, Date.now() - a.last))) : S.archive_pending;
      const archiveItem = settingItem("chart", S.archive_title, archiveText,
        !b.github ? () => connect(archiveEnableDialog) : a.enabled ? archiveDialog : archiveEnableDialog);
      if (a.enabled && a.error) archiveItem.querySelector(".supporting").classList.add("bad");
      return [github, folder, now, archiveItem, restore,
        settingItem("delete", S.backup_disable, S.backup_disable_help, disableDialog, { danger: true, chevron: false })];
    }

    /** Section des réglages, reconstruite quand l'état change (et chaque minute pour les durées). */
    function section() {
      const card = h("section", { class: "card" });
      let sig = "";
      let minute = -1;
      return {
        els: [h("h2", { class: "section-title", text: S.section_backup_auto }), card],
        update() {
          const b = state.backup || {};
          const now = Date.now();
          const next = JSON.stringify(b);
          if (next === sig && Math.floor(now / 60000) === minute) return;
          sig = next;
          minute = Math.floor(now / 60000);
          card.replaceChildren(...items(b));
        },
      };
    }

    function enableDialog() {
      let password = "";
      let confirmation = "";
      const passwordField = field({ label: S.field_password, helper: fmt(S.field_password_help, MIN_PASSWORD_LENGTH), type: "password", onInput: (v) => { password = v; refresh(); } });
      const confirmField = field({ label: S.field_password_confirm, type: "password", onInput: (v) => { confirmation = v; refresh(); } });
      const dialog = openDialog({
        iconName: "shield",
        title: S.backup_enable_title,
        body: [h("p", { text: S.backup_enable_text }), h("div", { class: "form-section" }, passwordField.wrap, confirmField.wrap)],
      });
      refresh();
      [passwordField.input, confirmField.input].forEach((i) => i.addEventListener("keydown", (e) => {
        if (e.key === "Enter" && valid()) submit();
      }));
      function valid() {
        return [...password].length >= MIN_PASSWORD_LENGTH && password === confirmation;
      }
      function refresh() {
        passwordField.wrap.classList.toggle("error", password.length > 0 && [...password].length < MIN_PASSWORD_LENGTH);
        confirmField.wrap.classList.toggle("error", confirmation.length > 0 && password !== confirmation);
        dialog.setActions([
          { label: S.cancel, onClick: () => dialog.close() },
          { label: S.backup_enable_ok, disabled: !valid(), onClick: submit },
        ]);
      }
      async function submit() {
        dialog.close();
        try {
          await api.call("backupEnable", { password });
          // Première destination proposée tout de suite.
          if (!state.backup?.github && !state.backup?.folder) chooseTargetDialog();
        } catch (e) {
          snackbar(errorMessage(e));
        } finally {
          password = confirmation = "";
        }
      }
    }

    function chooseTargetDialog() {
      const dialog = openDialog({
        iconName: "cloud",
        title: S.backup_choose_title,
        body: [h("p", { text: S.backup_choose_text })],
        actions: [
          { label: S.later, kind: "sec", onClick: () => dialog.close() },
          { label: S.backup_folder, kind: "sec", onClick: () => { dialog.close(); pickFolder(); } },
          { label: S.backup_github, onClick: () => { dialog.close(); connect(); } },
        ],
      });
    }

    /** Connexion GitHub des sauvegardes (compte du partage s'il est connecté, sinon par code). */
    async function connect(then) {
      let r;
      try {
        r = await api.call("backupConnect");
      } catch (e) {
        alertDialog(S.share_login_title, errorMessage(e));
        return;
      }
      if (r.connected) {
        snackbar(fmt(S.backup_login_reused, r.user));
        if (then) then();
        return;
      }
      const status = h("p", { class: "muted", text: S.share_login_waiting });
      const openGitHub = async () => {
        await sharing.copy(r.code);
        openUrl(GITHUB_DEVICE_URL);
      };
      const onState = () => {
        const b = state.backup || {};
        if (b.login?.error) {
          status.className = "bad";
          setText(status, b.login.error);
        } else if (!b.login && b.github) {
          close();
          snackbar(S.backup_login_done);
          if (then) then();
        }
      };
      const close = () => {
        stateListeners.delete(onState);
        dialog.close();
      };
      const dialog = openDialog({
        iconName: "cloud",
        title: S.share_login_title,
        body: [r.shareUser ? h("p", { class: "muted", text: fmt(S.backup_login_share_rejected, r.shareUser) }) : null,
          h("p", { text: S.share_login_text }), sharing.codeView(r.code), status],
        actions: [
          { label: S.cancel, onClick: () => { api.call("backupCancelLogin").catch(() => {}); close(); } },
          { label: S.share_login_copy, kind: "sec", onClick: () => sharing.copy(r.code) },
          { label: S.share_login_open, onClick: openGitHub },
        ],
        onDismiss: () => {
          stateListeners.delete(onState);
          api.call("backupCancelLogin").catch(() => {});
        },
      });
      stateListeners.add(onState);
      openGitHub();
    }

    function githubDialog() {
      const b = state.backup;
      const dialog = openDialog({
        iconName: "cloud",
        title: S.backup_github_title,
        body: [h("p", { text: fmt(S.backup_github_text, b.github.label) }), b.github.error ? h("p", { class: "bad", text: b.github.error }) : null],
        actions: [
          { label: S.backup_disconnect, kind: "sec", onClick: async () => {
            dialog.close();
            try {
              await api.call("backupDisconnect");
            } catch (e) {
              snackbar(errorMessage(e));
            }
          } },
          { label: S.backup_reconnect, kind: "sec", onClick: () => { dialog.close(); connect(); } },
          { label: S.close, onClick: () => dialog.close() },
        ],
      });
    }

    async function pickFolder() {
      try {
        const r = await api.call("backupFolder");
        if (r.path) snackbar(S.backup_folder_done);
      } catch (e) {
        snackbar(errorMessage(e));
      }
    }

    function folderDialog() {
      const b = state.backup;
      const dialog = openDialog({
        iconName: "history",
        title: S.backup_folder_title,
        body: [h("p", { class: "mono selectable", text: b.folder.label }), b.folder.error ? h("p", { class: "bad", text: b.folder.error }) : null],
        actions: [
          { label: S.backup_folder_remove, kind: "sec", onClick: async () => {
            dialog.close();
            try {
              await api.call("backupRemoveFolder");
            } catch (e) {
              snackbar(errorMessage(e));
            }
          } },
          { label: S.backup_folder_change, kind: "sec", onClick: () => { dialog.close(); pickFolder(); } },
          { label: S.close, onClick: () => dialog.close() },
        ],
      });
    }

    function archiveEnableDialog() {
      const dialog = openDialog({
        iconName: "chart",
        title: S.archive_enable_title,
        body: [h("p", { text: S.archive_enable_text })],
        actions: [
          { label: S.cancel, onClick: () => dialog.close() },
          { label: S.archive_enable_ok, onClick: async () => {
            dialog.close();
            try {
              await api.call("archiveEnable", { enabled: true });
            } catch (e) {
              alertDialog(S.archive_title, errorMessage(e));
            }
          } },
        ],
      });
    }

    function archiveDialog() {
      const dialog = openDialog({
        iconName: "chart",
        title: S.archive_title,
        body: [h("p", { text: S.archive_dialog_text })],
        actions: [
          { label: S.archive_disable, kind: "danger", onClick: async () => {
            dialog.close();
            try {
              await api.call("archiveEnable", { enabled: false });
            } catch (e) {
              snackbar(errorMessage(e));
            }
          } },
          { label: S.archive_now, kind: "sec", onClick: () => { dialog.close(); archiveNow(); } },
          { label: S.close, onClick: () => dialog.close() },
        ],
      });
    }

    async function archiveNow() {
      try {
        const r = await api.call("archiveNow");
        if (r.skipped?.length) alertDialog(S.archive_now, fmt(S.archive_done, r.minutes) + "\n" + fmt(S.archive_skipped, r.skipped.join(", ")));
        else snackbar(fmt(S.archive_done, r.minutes));
      } catch (e) {
        alertDialog(S.archive_now, errorMessage(e));
      }
    }

    async function backupNow() {
      try {
        const r = await api.call("backupNow");
        if (r.errors?.length) alertDialog(S.backup_now, r.errors.join("\n"));
        else snackbar(S.backup_done);
      } catch (e) {
        alertDialog(S.backup_now, errorMessage(e));
      }
    }

    function disableDialog() {
      const dialog = openDialog({
        iconName: "delete",
        title: S.backup_disable,
        body: [h("p", { text: S.backup_disable_text })],
        actions: [
          { label: S.cancel, onClick: () => dialog.close() },
          { label: S.backup_disable_ok, kind: "danger", onClick: async () => {
            dialog.close();
            try {
              await api.call("backupDisable");
            } catch (e) {
              snackbar(errorMessage(e));
            }
          } },
        ],
      });
    }

    /** Liste des sauvegardes du compte GitHub ; une fois choisie, l'import habituel prend le relais. */
    async function restoreDialog() {
      if (!state.backup?.github) {
        connect(restoreDialog);
        return;
      }
      setBusy(true);
      let r;
      try {
        r = await api.call("backupList");
      } catch (e) {
        alertDialog(S.backup_restore_title, errorMessage(e));
        return;
      } finally {
        setBusy(false);
      }
      if (!r.connected) {
        connect(restoreDialog);
        return;
      }
      const rows = (r.entries || []).map((e) => h("button", {
        class: "restore-row", type: "button",
        onClick: () => { dialog.close(); restore(e.name); },
      },
      icon("history", "small"),
      h("span", { class: "texts" },
        h("span", { class: "headline", text: `${formatDay(e.date)} · ${deviceLabel(e.device)}` }),
        h("small", { text: `${Math.max(1, Math.round(e.size / 1024))} Ko${e.mine ? " · " + S.backup_restore_mine : ""}` }))));
      const dialog = openDialog({
        iconName: "download",
        title: S.backup_restore_title,
        body: rows.length
          ? [h("p", { text: fmt(S.backup_restore_text, "@" + r.user) }), h("div", { class: "restore-list" }, rows)]
          : [h("p", { text: S.backup_restore_empty })],
        actions: [{ label: S.close, onClick: () => dialog.close() }],
      });
    }

    async function restore(name) {
      setBusy(true);
      try {
        handleImportStep(await api.call("backupRestore", { name }));
      } catch (e) {
        snackbar(errorMessage(e));
      } finally {
        setBusy(false);
      }
    }

    /** « windows-bureau-3fa2 » → « windows · bureau ». */
    function deviceLabel(device) {
      const parts = device.split("-");
      if (parts.length > 2 && /^[0-9a-f]{4}$/.test(parts[parts.length - 1])) parts.pop();
      return `${parts[0]} · ${parts.slice(1).join(" ")}`;
    }

    function formatDay(date) {
      const [y, m, d] = date.split("-").map(Number);
      return new Date(y, m - 1, d).toLocaleDateString("fr-FR", { weekday: "short", day: "numeric", month: "long", year: "numeric" });
    }

    return { section };
  })();

  function settingsView() {
    const poll = sliderSetting(S.setting_poll_interval, 1, 30, 1, (v) => updateSettings({ pollIntervalSeconds: v }));
    const wakeTimeout = sliderSetting(S.setting_wake_timeout, 60, 600, 30, (v) => updateSettings({ wakeTimeoutSeconds: v }));
    const confirmSwitch = switchInput(state.settings.confirmPowerActions, (checked) => updateSettings({ confirmPowerActions: checked }), S.setting_confirm);
    const exportSupporting = h("div", { class: "supporting" });
    const exportItem = settingItem("download", S.action_export, exportSupporting, exportDialog);
    const importItem = settingItem("upload", S.action_import, S.action_import_help, pickImportFile);
    const clearItem = settingItem("delete", S.history_clear, S.history_clear_help, confirmClearHistory, { danger: true, chevron: false });
    const updateSupporting = h("div", { class: "supporting" });
    const updateItem = settingItem("download", S.update_check, updateSupporting,
      () => (state.update?.enabled ? updates.checkNow() : openUrl(RELEASES_URL)));
    const autoUpdate = switchInput(!!state.update?.auto, (checked) => api.call("setAutoUpdate", { enabled: checked }).catch((e) => snackbar(errorMessage(e))), S.update_auto);
    const autoUpdateItem = h("div", { class: "item" },
      h("div", { class: "texts" }, h("div", { class: "headline", text: S.update_auto }), h("div", { class: "supporting", text: S.update_auto_help })),
      autoUpdate.el);
    const section = (title, ...items) => [h("h2", { class: "section-title", text: title }), h("section", { class: "card" }, items)];
    const shareSections = sharing.sections();
    const backupSection = backups.section();
    const el = h("div", { class: "main-inner narrow" },
      section(S.section_monitoring, poll.el, wakeTimeout.el,
        h("div", { class: "item" }, h("div", { class: "texts" }, h("div", { class: "headline", text: S.setting_confirm }), h("div", { class: "supporting", text: S.setting_confirm_help })), confirmSwitch.el)),
      shareSections.els,
      section(S.section_backup, exportItem, importItem),
      backupSection.els,
      section(S.section_history, settingItem("history", S.history_open, S.history_open_help, () => openHistorySheet("")), clearItem),
      section(S.section_agent_download, settingItem("download", S.agent_download, S.agent_download_help,
        () => (state.agent?.enabled ? agentDownload.open() : openUrl(RELEASES_URL)))),
      section(S.section_updates, updateItem, autoUpdateItem),
      section(S.section_diagnostic, settingItem("upload", S.diagnostic_export, S.diagnostic_export_help, diagnosticDialog)),
      section(S.section_about,
        settingItem("info", S.about_version, state.version),
        settingItem("code", S.about_source, REPO_URL, () => openUrl(REPO_URL)),
        settingItem("lock", S.about_security, S.about_security_text)));
    return {
      el,
      update() {
        const busy = busyCount > 0;
        poll.set(state.settings.pollIntervalSeconds);
        wakeTimeout.set(state.settings.wakeTimeoutSeconds);
        confirmSwitch.input.checked = state.settings.confirmPowerActions;
        const ownCount = state.devices.filter((d) => !d.shared).length;
        setText(exportSupporting, fmt(S.action_export_help, ownCount));
        exportItem.classList.toggle("disabled", busy || ownCount === 0);
        importItem.classList.toggle("disabled", busy);
        const u = state.update || {};
        let text;
        if (!u.enabled) text = S.update_disabled;
        else if (u.checking) text = S.update_checking;
        else if (u.available) text = fmt(S.update_available, u.available.version);
        else if (u.error) text = u.error;
        else text = u.lastCheck ? fmt(S.update_last_check, formatDuration(Date.now() - u.lastCheck)) : S.update_never;
        setText(updateSupporting, text);
        updateSupporting.classList.toggle("accent", !!u.available);
        autoUpdateItem.classList.toggle("hidden", !u.enabled);
        if (document.activeElement !== autoUpdate.input) autoUpdate.input.checked = !!u.auto;
        shareSections.update();
        backupSection.update();
      },
    };
  }

  function settingItem(iconName, headline, supporting, onClick, { danger = false, chevron = !!onClick } = {}) {
    return h("div", {
      class: `item${onClick ? " click" : ""}${danger ? " danger" : ""}`,
      tabindex: onClick ? "0" : null,
      role: onClick ? "button" : null,
      onClick: onClick ? () => onClick() : null,
      onKeydown: onClick ? activate(onClick) : null,
    },
    icon(iconName),
    h("div", { class: "texts" }, h("div", { class: "headline", text: headline }),
      supporting instanceof Node ? supporting : h("div", { class: "supporting selectable", text: supporting })),
    chevron ? icon("chevron", "small chev") : null);
  }

  function sliderSetting(title, min, max, step, onCommit) {
    const valueEl = h("span");
    const input = h("input", { type: "range", min, max, step, "aria-label": title });
    input.addEventListener("input", () => (valueEl.textContent = fmt(S.setting_seconds_value, input.value)));
    input.addEventListener("change", () => onCommit(Number(input.value)));
    const el = h("div", { class: "slider" }, h("div", { class: "top" }, h("span", { text: title }), valueEl), input);
    return {
      el,
      set(value) {
        if (document.activeElement === input) return;
        const v = Math.min(max, Math.max(min, value));
        input.value = String(v);
        valueEl.textContent = fmt(S.setting_seconds_value, v);
      },
    };
  }

  async function updateSettings(patch) {
    try {
      await api.call("updateSettings", patch);
    } catch (e) {
      snackbar(errorMessage(e));
    }
  }

  function confirmClearHistory() {
    const dialog = openDialog({
      iconName: "history",
      title: S.history_clear_title,
      body: h("p", { text: S.history_clear_text }),
      actions: [
        { label: S.cancel, onClick: () => dialog.close() },
        {
          label: S.history_clear_action,
          kind: "danger",
          onClick: async () => {
            dialog.close();
            try {
              await api.call("clearHistory");
              snackbar(S.message_history_cleared);
            } catch (e) {
              snackbar(errorMessage(e));
            }
          },
        },
      ],
    });
  }

  function exportDialog() {
    let withSecrets = state.hasSecrets;
    let password = "";
    let confirmation = "";
    const passwordField = field({ label: S.field_password, helper: fmt(S.field_password_help, MIN_PASSWORD_LENGTH), type: "password", onInput: (v) => { password = v; refresh(); } });
    const confirmField = field({ label: S.field_password_confirm, type: "password", onInput: (v) => { confirmation = v; refresh(); } });
    const secretsBox = h("div", { class: "form-section" }, passwordField.wrap, confirmField.wrap);
    const radioWith = h("input", { type: "radio", name: "export-kind", checked: withSecrets });
    const radioWithout = h("input", { type: "radio", name: "export-kind", checked: !withSecrets });
    radioWith.addEventListener("change", () => { withSecrets = true; refresh(); });
    radioWithout.addEventListener("change", () => { withSecrets = false; refresh(); });
    const choice = (radio, title, subtitle) => h("label", { class: "choice" }, radio,
      h("span", { class: "texts" }, h("span", { text: title }), h("small", { text: subtitle })));
    const dialog = openDialog({
      iconName: "download",
      title: S.export_title,
      body: [choice(radioWith, S.export_with_secrets, S.export_with_secrets_help), choice(radioWithout, S.export_without_secrets, S.export_without_secrets_help), secretsBox],
    });
    refresh();
    [passwordField.input, confirmField.input].forEach((i) => i.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && valid()) submit();
    }));

    function valid() {
      return !withSecrets || (password.length >= MIN_PASSWORD_LENGTH && password === confirmation);
    }
    function refresh() {
      secretsBox.classList.toggle("hidden", !withSecrets);
      passwordField.wrap.classList.toggle("error", password.length > 0 && password.length < MIN_PASSWORD_LENGTH);
      confirmField.wrap.classList.toggle("error", confirmation.length > 0 && password !== confirmation);
      dialog.setActions([
        { label: S.cancel, onClick: () => dialog.close() },
        { label: S.action_export_short, disabled: !valid(), onClick: submit },
      ]);
    }
    async function submit() {
      dialog.close();
      setBusy(true);
      try {
        const r = await api.call("exportConfig", { withSecrets, password: withSecrets ? password : "" });
        if (r.download) download(r.download.name, r.download.content);
        if (r.ok) snackbar(fmt(S.message_export_done, r.count));
      } catch (e) {
        snackbar(errorMessage(e));
      } finally {
        password = confirmation = "";
        setBusy(false);
      }
    }
  }

  /** Rapport de diagnostic : chiffré par un mot de passe choisi ici, puis enregistré. */
  function diagnosticDialog() {
    let password = "";
    let confirmation = "";
    const passwordField = field({ label: S.field_password, helper: fmt(S.field_password_help, MIN_PASSWORD_LENGTH), type: "password", onInput: (v) => { password = v; refresh(); } });
    const confirmField = field({ label: S.field_password_confirm, type: "password", onInput: (v) => { confirmation = v; refresh(); } });
    const dialog = openDialog({
      iconName: "upload",
      title: S.diagnostic_title,
      body: [h("p", { text: S.diagnostic_text }), h("div", { class: "form-section" }, passwordField.wrap, confirmField.wrap)],
    });
    refresh();
    [passwordField.input, confirmField.input].forEach((i) => i.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && valid()) submit();
    }));

    function valid() {
      return [...password].length >= MIN_PASSWORD_LENGTH && password === confirmation;
    }
    function refresh() {
      passwordField.wrap.classList.toggle("error", password.length > 0 && [...password].length < MIN_PASSWORD_LENGTH);
      confirmField.wrap.classList.toggle("error", confirmation.length > 0 && password !== confirmation);
      dialog.setActions([
        { label: S.cancel, onClick: () => dialog.close() },
        { label: S.diagnostic_save, disabled: !valid(), onClick: submit },
      ]);
    }
    async function submit() {
      dialog.close();
      setBusy(true);
      try {
        const r = await api.call("exportDiagnostic", { password });
        if (r.download) download(r.download.name, r.download.content);
        if (r.ok) snackbar(S.message_diagnostic_done);
      } catch (e) {
        snackbar(errorMessage(e));
      } finally {
        password = confirmation = "";
        setBusy(false);
      }
    }
  }

  function openUrl(url) {
    api.call("openUrl", { url }).catch((e) => snackbar(errorMessage(e)));
  }

  function download(name, content) {
    const url = URL.createObjectURL(new Blob([content], { type: "application/json" }));
    const a = h("a", { href: url, download: name });
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  // ---------------------------------------------------------------------------------------------
  // Import d'une sauvegarde (fichier choisi ou glissé dans la fenêtre)
  // ---------------------------------------------------------------------------------------------
  const importInput = document.getElementById("import-file");
  importInput.addEventListener("change", () => {
    const file = importInput.files?.[0];
    importInput.value = "";
    if (file) importFromFile(file);
  });

  function pickImportFile() {
    importInput.click();
  }

  async function importFromFile(file) {
    if (file.size > MAX_IMPORT_BYTES) {
      snackbar(fmt(S.message_error, S.import_too_big));
      return;
    }
    setBusy(true);
    try {
      const text = await file.text();
      handleImportStep(await api.call("importFile", { text }));
    } catch (e) {
      snackbar(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  function handleImportStep(step) {
    if (step.step === "password") importPasswordDialog();
    else if (step.step === "confirm") importConfirmDialog(step);
  }

  function importPasswordDialog() {
    let busy = false;
    const pwd = field({ label: S.field_password, type: "password", onInput: () => { pwd.setError(null); refresh(); } });
    pwd.input.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && pwd.input.value && !busy) submit();
    });
    const dialog = openDialog({ iconName: "lock", title: S.import_password_title, body: pwd.wrap, onDismiss: cancelImport });
    refresh();

    function refresh() {
      dialog.setActions([
        { label: S.cancel, onClick: () => { dialog.close(); cancelImport(); } },
        { label: S.ok, disabled: !pwd.input.value || busy, onClick: submit },
      ]);
    }
    async function submit() {
      busy = true;
      refresh();
      setBusy(true);
      try {
        const r = await api.call("importPassword", { password: pwd.input.value });
        if (r.step === "password") {
          pwd.setError(S.import_wrong_password);
          pwd.input.select();
        } else {
          dialog.close();
          handleImportStep(r);
        }
      } catch (e) {
        dialog.close();
        snackbar(errorMessage(e));
      } finally {
        busy = false;
        setBusy(false);
        refresh();
      }
    }
  }

  function importConfirmDialog(step) {
    const body = [h("p", { text: S.import_confirm_text })];
    if (step.missingKeys) body.push(h("p", { class: "error", text: S.import_missing_keys }));
    if (step.sharing) body.push(h("p", { text: fmt(S.import_sharing, step.sharing.name, step.sharing.people) }));
    if (step.history) body.push(h("p", { text: fmt(S.import_history, step.history) }));
    const dialog = openDialog({
      iconName: "upload",
      title: fmt(S.import_confirm_title, step.count),
      body,
      onDismiss: cancelImport,
      actions: [
        { label: S.cancel, onClick: () => { dialog.close(); cancelImport(); } },
        { label: S.import_replace, kind: "danger", onClick: () => confirm(true) },
        { label: S.import_merge, onClick: () => confirm(false) },
      ],
    });
    async function confirm(replace) {
      dialog.close();
      setBusy(true);
      try {
        const r = await api.call("importConfirm", { replace });
        snackbar(fmt(S.message_import_done, r.count));
        if (r.sharing) alertDialog(S.section_share_mine, S.import_sharing_done);
      } catch (e) {
        snackbar(errorMessage(e));
      } finally {
        setBusy(false);
      }
    }
  }

  function cancelImport() {
    api.call("importCancel").catch(() => {});
  }

  // Glisser-déposer : un fichier de sauvegarde déposé dans la fenêtre est importé
  // (et la fenêtre ne doit jamais « naviguer » vers le fichier déposé).
  window.addEventListener("dragover", (e) => e.preventDefault());
  window.addEventListener("drop", (e) => {
    e.preventDefault();
    const file = e.dataTransfer?.files?.[0];
    if (file) importFromFile(file);
  });

  // ---------------------------------------------------------------------------------------------
  // Démarrage
  // ---------------------------------------------------------------------------------------------
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      if (closeTop()) {
        e.preventDefault();
        return;
      }
      if (narrow.matches && sideOpen) {
        sideOpen = false;
        render();
        e.preventDefault();
        return;
      }
    }
    // Pas de rechargement ni d'impression de la page : F5 / Ctrl+R actualisent l'état des PC.
    const key = e.key.toLowerCase();
    if (e.key === "F5" || ((e.ctrlKey || e.metaKey) && (key === "r" || key === "p"))) {
      e.preventDefault();
      if (key !== "p" && state) refreshAll();
    }
  });
  document.addEventListener("contextmenu", (e) => {
    if (!e.target.closest("input, textarea, .selectable")) e.preventDefault();
  });
  document.addEventListener("visibilitychange", () => {
    api.call("setVisible", { visible: document.visibilityState === "visible" }).catch(() => {});
  });

  let started = false;
  api.onState((s) => {
    state = s;
    if (!started) return;
    render();
    stateListeners.forEach((f) => f());
  });

  async function start() {
    try {
      state = await api.call("getState");
    } catch (e) {
      appEl.replaceChildren(h("p", { style: "padding:24px", text: errorMessage(e) }));
      return;
    }
    appEl.replaceChildren(navEl, bodyEl);
    started = true;
    render();
    setInterval(render, 1000);
    if (state.startupMessage) {
      alertDialog(S.app_name, state.startupMessage);
      api.call("dismissStartupMessage").catch(() => {});
    }
    // Signal pour l'autotest de démarrage (CI) : interface affichée, polices chargées.
    await Promise.race([
      new Promise((resolve) => requestAnimationFrame(resolve)).then(() => document.fonts.ready),
      new Promise((resolve) => setTimeout(resolve, 3000)),
    ]);
    api.call("uiReady", {
      screen: view?.kind,
      devices: state.devices.length,
      empty: !!document.querySelector(".empty"),
      fonts: document.fonts.check('600 14px "Inter"'),
    }).catch(() => {});
  }

  start();
})();
