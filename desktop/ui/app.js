// Wake On LAN — interface Windows, thème sombre « topologie & panneau de détail ».
// Mêmes fonctions et mêmes textes que l'application Android ; toute la logique (réseau, états,
// chiffrement, historique) est dans le moteur Go, appelé par api.call().
"use strict";

(() => {
  // ---------------------------------------------------------------------------------------------
  // Textes : ceux de l'application Android (res/values/strings.xml), adaptés au PC si nécessaire.
  // ---------------------------------------------------------------------------------------------
  const S = {
    app_name: "Wake On LAN",
    ok: "OK",
    cancel: "Annuler",
    close: "Fermer",
    save: "Enregistrer",
    show: "Afficher",
    hide: "Masquer",

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
    detail_latency: "Latence",
    detail_agent: "Agent",
    detail_not_set: "Non renseignée",
    agent_authenticated: "Authentifié · %1$s",
    agent_not_configured: "Non installé",
    agent_configured: "Configuré",
    hint_no_agent: "Pour éteindre, redémarrer ou mettre ce PC en veille d’ici, installez l’agent sur ce PC puis appairez-le (Modifier).",
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
    agent_help_text: "Pour éteindre « %1$s » à distance, installez l’agent Wake On LAN sur ce PC puis appairez-le (lien d’appairage) dans sa fiche.",

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
    export_title: "Exporter la configuration",
    export_with_secrets: "Complète, protégée par mot de passe",
    export_with_secrets_help: "Inclut les clés des agents. Fichier chiffré (AES-256).",
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
    agent_download_help: "Windows, Linux et macOS. Nécessaire uniquement pour éteindre à distance et pour l’historique complet.",
    section_about: "À propos",
    about_version: "Version",
    about_source: "Code source",
    about_security: "Sécurité",
    about_security_text: "Configuration et historique chiffrés sur ce PC (protection des données Windows), aucune donnée envoyée sur Internet, commandes d’extinction authentifiées (HMAC-SHA256) et protégées contre le rejeu.",
  };

  const REPO_URL = "https://github.com/slaynAW/WakeOnLan";
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
    send: '<path d="M21 3 10.5 13.5"/><path d="M21 3l-6.5 18-4-7.5L3 9.5z"/>',
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
  const versionLabel = (v) => (/^\d/.test(v || "") ? "v" + v : v || "");
  const methodLabel = (m) => ({ AGENT: "agent", TCP: "TCP", PING: "ping" })[m] || "";
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

  /** Ligne d'historique : icône, libellé (« · par 192.168.1.50 » pour une commande venue d'ailleurs), heure. */
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

  function field({ label, helper, value = "", type = "text", mono = false, onInput, onEnter, trailing, multiline = false, placeholder }) {
    const id = "f" + ++fieldSeq;
    const input = multiline
      ? h("textarea", { id, rows: 3, spellcheck: "false", placeholder })
      : h("input", { id, type, spellcheck: "false", autocomplete: "off", class: mono ? "mono" : null, placeholder });
    input.value = value;
    const support = h("div", { class: "support" });
    const wrap = h("div", { class: `field${trailing ? " has-trailing" : ""}` },
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
    h("div", { class: "brand" }, h("span", { class: "logo" }, icon("power")), S.app_name),
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
  }

  function updateNav() {
    for (const [id] of TABS) {
      tabButtons[id].classList.toggle("on", tab === id);
      tabButtons[id].setAttribute("aria-selected", String(tab === id));
    }
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
    const host = h("span", { class: "mono" });
    const info = h("span");
    const el = h("div", { class: "row", tabindex: "0", role: "button", onClick: () => select(id), onKeydown: activate(() => select(id)) },
      dot, name, host, info, icon("chevron", "small"));
    return {
      el,
      update(d, now) {
        id = d.id;
        const st = d.status.state;
        setClass(el, `row st-${st}${d.id === selectedId ? " sel" : ""}`);
        setClass(dot, dotClass(st));
        dot.title = stateName(st);
        setText(name, d.name);
        setText(host, d.host || "—");
        const online = st === "ONLINE";
        setClass(info, online ? "" : "state-text");
        setText(info, online && d.status.latencyMs != null ? fmt(S.latency_ms, d.status.latencyMs) : shortState(d.status, now));
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
          h("span", { class: "opt opt2", text: S.col_latency }), h("span")),
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
    const stateCell = h("span", { class: "cell state-text" }, dot, stateText);
    const mac = h("span", { class: "cell txt opt mono" });
    const system = h("span", { class: "cell txt opt opt2" });
    const latency = h("span", { class: "cell txt opt opt2" });
    const acts = h("span", { class: "cell actions" });
    const more = iconBtn("more", S.action_more, own(() => openDeviceMenu(more, id)), "flat");
    const el = h("div", { class: "tr", tabindex: "0", role: "row", onClick: () => select(id), onKeydown: activate(() => select(id)) },
      h("span", { class: "cell" }, h("span", { class: "ni" }, icon("monitor", "", 18)), h("span", { class: "t" }, name, capability)),
      host, stateCell, mac, system, latency, acts);
    return {
      el,
      update(d, now) {
        id = d.id;
        const s = d.status;
        const st = s.state;
        const online = st === "ONLINE";
        setClass(el, `tr st-${st}${d.id === selectedId ? " sel" : ""}`);
        setText(name, d.name);
        setText(capability, d.canShutdown ? S.capability_power : S.capability_wake);
        setText(host, d.host || "—");
        setClass(dot, dotClass(st));
        setText(stateText, shortState(s, now));
        setText(mac, d.mac || "—");
        setText(system, online && s.agent ? [osLabel(s.agent.os), archLabel(s.agent.arch)].filter(Boolean).join(" · ") : "—");
        setText(latency, online && s.latencyMs != null ? fmt(S.latency_ms, s.latencyMs) : "—");
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
        h("div", { class: "logo" }, icon("power", "", 30)),
        h("h2", { text: S.empty_title }),
        h("p", { text: S.empty_text }),
        h("div", { class: "row-btns" },
          btn("primary big", S.action_add_device, () => openEditSheet(null), "plus"),
          btn("sec big", S.action_import_short, pickImportFile, "upload"))));
    return { el, update: () => warning.update() };
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
    const recent = sideHistory();
    const inner = h("div", { class: "side-inner" }, h("div", { class: "side-top" }, more, closeButton), hero, notice, acts, hint, info, recent.el);
    const placeholder = h("div", { class: "placeholder" }, h("span", { class: "ni" }, icon("monitor", "", 28)), h("span", { text: S.select_hint }));
    sideEl.append(inner, placeholder);

    function update(now, visible) {
      sideEl.classList.toggle("hidden", !visible);
      sideEl.classList.toggle("open", visible && sideOpen);
      if (!visible) return;
      const d = deviceById(selectedId);
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

      const sig = [d.id, st, d.canShutdown].join("|");
      if (sig !== actsSig) {
        actsSig = sig;
        renderActions(d);
      }
      hint.classList.toggle("hidden", !(st === "ONLINE" && !d.canShutdown));
      renderInfo(d);
      recent.update(d, now);
    }

    function renderActions(d) {
      const id = d.id;
      const st = d.status.state;
      const edit = (cls) => btn("sec " + cls, S.action_edit, () => openEditSheet(id), "edit");
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
      }
      if (online && s.latencyMs != null) {
        const method = methodLabel(s.method);
        rows.push({ key: "lat", label: S.detail_latency, text: fmt(S.latency_ms, s.latencyMs) + (method ? " · " + method : "") });
      }
      if (!d.hasAgent) rows.push({ key: "agent", label: S.detail_agent, text: S.agent_not_configured, cls: "muted" });
      else if (online && s.agentError) rows.push({ key: "agent", label: S.detail_agent, text: agentErrorLabel(s.agentError), cls: "bad" });
      else if (online && s.agent) rows.push({ key: "agent", label: S.detail_agent, text: fmt(S.agent_authenticated, versionLabel(s.agent.version)), cls: "ok", iconName: "shield" });
      else rows.push({ key: "agent", label: S.detail_agent, text: S.agent_configured, cls: "muted" });

      const keys = rows.map((r) => r.key).join("|");
      if (info.dataset.keys !== keys) {
        info.dataset.keys = keys;
        info.replaceChildren(...rows.map((r) => h("div", { class: "kv" }, h("span", { text: r.label }), h("span"))));
      }
      rows.forEach((r, i) => {
        const valueEl = info.children[i].lastChild;
        const sig = [r.text, r.cls, r.iconName].join("|");
        if (valueEl.dataset.sig === sig) return;
        valueEl.dataset.sig = sig;
        valueEl.replaceChildren(h("span", { class: r.cls || null }, r.iconName ? icon(r.iconName, "small") : null, r.text));
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

  // ---------------------------------------------------------------------------------------------
  // Actions sur les PC
  // ---------------------------------------------------------------------------------------------
  function openDeviceMenu(anchor, id) {
    const d = deviceById(id);
    if (!d) return;
    const index = state.devices.indexOf(d);
    const items = [{ label: S.action_wake_menu, icon: "power", onClick: () => wake(id) }];
    if (d.canShutdown) {
      items.push(
        { label: S.action_shutdown, icon: "power", onClick: () => requestPower(id, "shutdown") },
        { label: S.action_reboot, icon: "restart", onClick: () => requestPower(id, "reboot") },
        { label: S.action_sleep, icon: "moon", onClick: () => requestPower(id, "sleep") });
    }
    items.push("sep",
      { label: S.action_edit, icon: "edit", onClick: () => openEditSheet(id) },
      { label: S.history_title, icon: "history", onClick: () => openHistorySheet(id) });
    if (index > 0) items.push({ label: S.action_move_up, icon: "arrowUp", onClick: () => move(id, -1) });
    if (index < state.devices.length - 1) items.push({ label: S.action_move_down, icon: "arrowDown", onClick: () => move(id, 1) });
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
        { label: S.action_configure, onClick: () => { dialog.close(); openEditSheet(d.id); } },
      ],
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
      testSlot.replaceChildren(h("div", { class: `result ${result.ok ? "ok" : "ko"}` },
        icon(result.ok ? "check" : "warning", "small"), h("span", { class: "selectable", text })));
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
  // Onglet « Réglages » (SettingsScreen)
  // ---------------------------------------------------------------------------------------------
  function settingsView() {
    const poll = sliderSetting(S.setting_poll_interval, 1, 30, 1, (v) => updateSettings({ pollIntervalSeconds: v }));
    const wakeTimeout = sliderSetting(S.setting_wake_timeout, 60, 600, 30, (v) => updateSettings({ wakeTimeoutSeconds: v }));
    const confirmSwitch = switchInput(state.settings.confirmPowerActions, (checked) => updateSettings({ confirmPowerActions: checked }), S.setting_confirm);
    const exportSupporting = h("div", { class: "supporting" });
    const exportItem = settingItem("download", S.action_export, exportSupporting, exportDialog);
    const importItem = settingItem("upload", S.action_import, S.action_import_help, pickImportFile);
    const clearItem = settingItem("delete", S.history_clear, S.history_clear_help, confirmClearHistory, { danger: true, chevron: false });
    const section = (title, ...items) => [h("h2", { class: "section-title", text: title }), h("section", { class: "card" }, items)];
    const el = h("div", { class: "main-inner narrow" },
      section(S.section_monitoring, poll.el, wakeTimeout.el,
        h("div", { class: "item" }, h("div", { class: "texts" }, h("div", { class: "headline", text: S.setting_confirm }), h("div", { class: "supporting", text: S.setting_confirm_help })), confirmSwitch.el)),
      section(S.section_backup, exportItem, importItem),
      section(S.section_history, settingItem("history", S.history_open, S.history_open_help, () => openHistorySheet("")), clearItem),
      section(S.section_agent_download, settingItem("download", S.agent_download, S.agent_download_help, () => openUrl(RELEASES_URL))),
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
        setText(exportSupporting, fmt(S.action_export_help, state.devices.length));
        exportItem.classList.toggle("disabled", busy || state.devices.length === 0);
        importItem.classList.toggle("disabled", busy);
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
