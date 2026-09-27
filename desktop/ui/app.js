// Wake On LAN — interface Windows. Mêmes écrans, textes et comportements que l'application Android
// (app/src/main/java/.../ui) ; toute la logique (réseau, états, chiffrement) est dans le moteur Go.
"use strict";

(() => {
  // ---------------------------------------------------------------------------------------------
  // Textes : ceux de l'application Android (res/values/strings.xml), adaptés au PC si nécessaire.
  // ---------------------------------------------------------------------------------------------
  const S = {
    app_name: "Wake On LAN",
    ok: "OK",
    cancel: "Annuler",
    save: "Enregistrer",
    back: "Retour",
    show: "Afficher",
    hide: "Masquer",

    action_add_device: "Ajouter un PC",
    action_refresh: "Actualiser l’état",
    action_more: "Plus d’actions",
    action_wake: "Démarrer",
    action_wake_again: "Renvoyer",
    action_wake_menu: "Démarrer (Wake-on-LAN)",
    action_shutdown: "Éteindre",
    action_reboot: "Redémarrer",
    action_sleep: "Mettre en veille",
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
    action_import_help: "Restaurer une sauvegarde ou récupérer la configuration du téléphone (vous pouvez aussi glisser le fichier dans la fenêtre)",

    state_online: "Allumé",
    state_offline: "Éteint",
    state_offline_seen: "Éteint · vu il y a %1$s",
    state_unknown: "État inconnu",
    state_checking: "Vérification…",
    state_waking: "Démarrage en cours… %1$s",
    state_waking_short: "Démarrage en cours",
    state_shutting_down: "Arrêt en cours… %1$s",
    state_shutting_down_short: "Arrêt en cours",
    state_restarting: "Redémarrage en cours… %1$s",
    state_restarting_short: "Redémarrage en cours",
    state_with_detail: "%1$s · %2$s",
    latency_ms: "%1$d ms",
    unknown_no_host: "adresse IP non renseignée",
    unknown_no_network: "pas de réseau local",

    duration_seconds: "%1$d s",
    duration_minutes: "%1$d min",
    duration_hours: "%1$d h",
    duration_days: "%1$d j",

    agent_info: "%1$s · %2$s · allumé depuis %3$s",
    agent_problem: "Agent : %1$s",
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
    empty_text: "Ajoutez un PC à réveiller. Si l’agent est installé dessus, collez simplement son lien d’appairage : tout est rempli automatiquement. Vous pouvez aussi importer la configuration exportée depuis le téléphone (Réglages).",
    network_wifi: "Wi-Fi",
    network_ethernet: "Ethernet",
    banner_no_lan_title: "Pas de réseau local",
    banner_no_lan_text: "Connectez ce PC au réseau de la maison pour démarrer vos PC et voir leur état.",
    banner_no_lan_vpn_text: "Ce PC n’est pas sur le réseau local, mais un VPN est actif : l’état des PC est vérifié à travers le VPN. Le réveil à distance nécessite un relais sur le réseau local.",

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

    settings_title: "Réglages",
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
    section_agent_download: "Agent pour PC",
    agent_download: "Télécharger l’agent",
    agent_download_help: "Windows, Linux et macOS. Nécessaire uniquement pour éteindre à distance.",
    section_about: "À propos",
    about_version: "Version",
    about_source: "Code source",
    about_security: "Sécurité",
    about_security_text: "Configuration chiffrée sur ce PC (protection des données Windows), aucune donnée envoyée sur Internet, commandes d’extinction authentifiées (HMAC-SHA256) et protégées contre le rejeu.",
  };

  const REPO_URL = "https://github.com/slaynAW/WakeOnLan";
  const RELEASES_URL = REPO_URL + "/releases";
  const MIN_PASSWORD_LENGTH = 8;
  const MAX_IMPORT_BYTES = 1024 * 1024;

  /** Remplace %1$s, %2$d… comme String.format côté Android. */
  function fmt(template, ...args) {
    return template.replace(/%(\d)\$[sd]/g, (_, i) => String(args[Number(i) - 1]));
  }

  // ---------------------------------------------------------------------------------------------
  // Icônes Material (Apache 2.0), mêmes symboles que l'application Android.
  // ---------------------------------------------------------------------------------------------
  const ICONS = {
    power: "M13 3h-2v10h2V3zm4.83 2.17-1.42 1.42C17.99 7.86 19 9.81 19 12c0 3.87-3.13 7-7 7s-7-3.13-7-7c0-2.19 1.01-4.14 2.58-5.42L6.17 5.17C4.23 6.82 3 9.26 3 12c0 4.97 4.03 9 9 9s9-4.03 9-9c0-2.74-1.23-5.18-3.17-6.83z",
    refresh: "M17.65 6.35C16.2 4.9 14.21 4 12 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08c-.82 2.33-3.04 4-5.65 4-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z",
    settings: "M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z",
    add: "M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z",
    more: "M12 8c1.1 0 2-.9 2-2s-.9-2-2-2-2 .9-2 2 .9 2 2 2zm0 2c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2zm0 6c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2z",
    edit: "M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04c.39-.39.39-1.02 0-1.41l-2.34-2.34c-.39-.39-1.02-.39-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z",
    delete: "M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z",
    up: "M7.41 15.41 12 10.83l4.59 4.58L18 14l-6-6-6 6z",
    down: "M7.41 8.59 12 13.17l4.59-4.58L18 10l-6 6-6-6 1.41-1.41z",
    back: "M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z",
    warning: "M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z",
    check: "M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z",
    paste: "M19 2h-4.18C14.4.84 13.3 0 12 0c-1.3 0-2.4.84-2.82 2H5c-1.1 0-2 .9-2 2v16c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-7 0c.55 0 1 .45 1 1s-.45 1-1 1-1-.45-1-1 .45-1 1-1zm7 18H5V4h2v3h10V4h2v16z",
  };

  function icon(name, cls = "") {
    const ns = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(ns, "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("class", ("icon " + cls).trim());
    svg.setAttribute("aria-hidden", "true");
    const path = document.createElementNS(ns, "path");
    path.setAttribute("d", ICONS[name]);
    svg.append(path);
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

  function iconButton(name, label, onClick) {
    return h("button", { class: "icon-button", title: label, "aria-label": label, onClick }, icon(name));
  }

  function button(kind, label, onClick, iconName, extra = {}) {
    return h("button", { class: `btn ${kind}${iconName ? " with-icon" : ""}`, onClick, ...extra }, iconName ? icon(iconName, "small") : null, label);
  }

  function topBar(title, onBack, actions = []) {
    return h("header", { class: `top-bar${onBack ? " with-nav" : ""}` },
      onBack ? iconButton("back", S.back, onBack) : null,
      h("h1", { text: title }),
      actions);
  }

  function watchScroll(content, bar) {
    content.addEventListener("scroll", () => bar.classList.toggle("scrolled", content.scrollTop > 0));
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

  const osLabel = (os) => ({ windows: "Windows", linux: "Linux", darwin: "macOS" })[os] || os;
  const agentErrorLabel = (code) => S["agent_error_" + code] || S.agent_error_PROTOCOL;
  const actionLabel = (a) => ({ shutdown: S.action_shutdown, reboot: S.action_reboot, sleep: S.action_sleep })[a];
  const confirmTitle = (a) => ({ shutdown: S.confirm_shutdown_title, reboot: S.confirm_reboot_title, sleep: S.confirm_sleep_title })[a];
  const sentMessage = (a) => ({ shutdown: S.message_shutdown_sent, reboot: S.message_reboot_sent, sleep: S.message_sleep_sent })[a];
  const unknownLabel = (r) => ({ NO_HOST: S.unknown_no_host, NO_NETWORK: S.unknown_no_network })[r];

  function stateDescription(st) {
    return {
      ONLINE: S.state_online, OFFLINE: S.state_offline, UNKNOWN: S.state_unknown, WAKING: S.state_waking_short,
      SHUTTING_DOWN: S.state_shutting_down_short, RESTARTING: S.state_restarting_short,
    }[st];
  }

  function statusText(status, now) {
    const sinceAction = now - (status.actionStartedAt ?? now);
    switch (status.state) {
      case "ONLINE":
        return status.latencyMs != null
          ? fmt(S.state_with_detail, S.state_online, fmt(S.latency_ms, status.latencyMs))
          : S.state_online;
      case "OFFLINE":
        return status.lastSeen != null ? fmt(S.state_offline_seen, formatDuration(now - status.lastSeen)) : S.state_offline;
      case "WAKING":
        return fmt(S.state_waking, formatDuration(sinceAction));
      case "SHUTTING_DOWN":
        return fmt(S.state_shutting_down, formatDuration(sinceAction));
      case "RESTARTING":
        return fmt(S.state_restarting, formatDuration(sinceAction));
      default:
        return status.unknownReason
          ? fmt(S.state_with_detail, S.state_unknown, unknownLabel(status.unknownReason))
          : S.state_checking;
    }
  }

  // ---------------------------------------------------------------------------------------------
  // Superpositions : menus, dialogues, snackbar
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

  function openMenu(anchor, items) {
    closeMenus();
    const menu = h("div", { class: "menu", role: "menu" });
    const entry = { kind: "menu", dismiss: close };
    for (const item of items) {
      if (item === "divider") {
        menu.append(h("div", { class: "menu-divider" }));
        continue;
      }
      menu.append(h("button", {
        class: "menu-item", role: "menuitem",
        onClick: (e) => {
          e.stopPropagation();
          close();
          item.onClick();
        },
      }, item.icon ? icon(item.icon) : h("span", { class: "no-icon" }), item.label));
    }
    overlays.append(menu);
    const r = anchor.getBoundingClientRect();
    const width = menu.offsetWidth;
    const height = menu.offsetHeight;
    let top = r.bottom;
    if (top + height > window.innerHeight - 8) top = Math.max(8, r.top - height);
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
   * Ouvre un dialogue (AlertDialog). actions : [{label, onClick, disabled}] ; onDismiss : clic hors
   * du dialogue ou Échap.
   */
  function openDialog({ iconName, title, body, actions = [], onDismiss }) {
    const scrim = h("div", { class: "scrim" });
    const bodyEl = h("div", { class: "dialog-body selectable" }, body);
    const actionsEl = h("div", { class: "dialog-actions" });
    const dialog = h("div", { class: "dialog", role: "dialog", "aria-modal": "true" },
      iconName ? icon(iconName, "dialog-icon") : null,
      title ? h("h2", { text: title }) : null,
      bodyEl,
      actionsEl);
    const entry = { kind: "dialog", dismiss };
    const handle = { close, body: bodyEl, setActions };
    setActions(actions);
    scrim.append(dialog);
    scrim.addEventListener("mousedown", (e) => {
      if (e.target === scrim) dismiss();
    });
    closeMenus();
    stack.push(entry);
    overlays.append(scrim);
    const focusTarget = bodyEl.querySelector("input:not([type=checkbox]):not([type=radio]), textarea") ||
      actionsEl.querySelector("button:last-child");
    focusTarget?.focus();
    return handle;

    function setActions(list) {
      actionsEl.replaceChildren(...list.map((a) => a === "spacer"
        ? h("span", { class: "spacer" })
        : h("button", { class: "btn text", disabled: !!a.disabled, onClick: a.onClick }, a.label)));
    }
    function dismiss() {
      close();
      onDismiss?.();
    }
    function close() {
      const i = stack.indexOf(entry);
      if (i < 0) return;
      stack.splice(i, 1);
      scrim.remove();
    }
  }

  function alertDialog(title, text) {
    const d = openDialog({ title, body: h("p", { style: "margin:0", text }), actions: [{ label: S.ok, onClick: () => d.close() }] });
    return d;
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

  function field({ label, helper, value = "", type = "text", mono = false, onInput, trailing, multiline = false, placeholder }) {
    const input = multiline
      ? h("textarea", { placeholder: placeholder || " ", rows: 2, spellcheck: "false" })
      : h("input", { type, placeholder: " ", spellcheck: "false", autocomplete: "off", class: mono ? "mono" : null });
    input.value = value;
    const support = h("div", { class: "support" });
    const wrap = h("div", { class: `field${trailing ? " has-trailing" : ""}` },
      h("div", { class: "field-box" }, input, h("label", { text: label }), trailing ? h("div", { class: "trailing" }, trailing) : null),
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
    return ref;
  }

  function switchInput(checked, onChange, label) {
    const input = h("input", { type: "checkbox", role: "switch", checked, "aria-label": label });
    input.addEventListener("change", () => onChange(input.checked));
    return { el: h("span", { class: "switch" }, input, h("span", { class: "track" }), h("span", { class: "thumb" })), input };
  }

  // ---------------------------------------------------------------------------------------------
  // État global et navigation
  // ---------------------------------------------------------------------------------------------
  let state = null;
  let current = null;
  const appEl = document.getElementById("app");

  function setScreen(screen) {
    current?.dispose?.();
    closeMenus();
    appEl.replaceChildren(screen.el);
    current = screen;
    snackbarEl.classList.toggle("low", screen.name !== "list");
  }

  const errorMessage = (e) => fmt(S.message_error, e?.message || String(e));

  // ---------------------------------------------------------------------------------------------
  // Écran « liste des PC » (DevicesScreen)
  // ---------------------------------------------------------------------------------------------
  function showList() {
    const bar = topBar(S.app_name, null, [
      iconButton("refresh", S.action_refresh, () => api.call("refresh").catch(() => {})),
      iconButton("settings", S.settings_title, showSettings),
    ]);
    const networkSlot = h("div");
    const emptySlot = h("div");
    const cardsSlot = h("div", { style: "display:flex;flex-direction:column;gap:12px" });
    const content = h("main", { class: "content" }, h("div", { class: "column" }, networkSlot, emptySlot, cardsSlot));
    const fab = h("button", { class: "fab", onClick: () => showEdit(null) }, icon("add"), S.action_add_device);
    const el = h("div", { class: "screen" }, bar, content, fab);
    watchScroll(content, bar);

    const cards = new Map();
    let networkSig = "";
    let emptySig = null;

    function render() {
      const net = state.network;
      const sig = JSON.stringify(net);
      if (sig !== networkSig) {
        networkSig = sig;
        networkSlot.replaceChildren(networkBanner(net));
      }
      const empty = state.devices.length === 0;
      if (empty !== emptySig) {
        emptySig = empty;
        emptySlot.replaceChildren(empty ? emptyState() : "");
      }
      const seen = new Set();
      state.devices.forEach((d, i) => {
        let card = cards.get(d.id);
        if (!card) {
          card = deviceCard();
          cards.set(d.id, card);
        }
        card.update(d, i === 0, i === state.devices.length - 1);
        seen.add(d.id);
        if (cardsSlot.children[i] !== card.el) cardsSlot.insertBefore(card.el, cardsSlot.children[i] || null);
      });
      for (const [id, card] of cards) {
        if (!seen.has(id)) {
          card.el.remove();
          cards.delete(id);
        }
      }
    }

    render();
    setScreen({
      name: "list", el, onState: render,
      onTick: () => cards.forEach((c) => c.tick()),
      onRefreshKey: () => api.call("refresh").catch(() => {}),
    });
  }

  function networkBanner(net) {
    if (!net.connected) {
      return h("div", { class: "warning-card" }, icon("warning"),
        h("div", {}, h("h2", { text: S.banner_no_lan_title }), h("p", { text: net.vpn ? S.banner_no_lan_vpn_text : S.banner_no_lan_text })));
    }
    const transport = net.transport === "ethernet" ? S.network_ethernet : S.network_wifi;
    return h("div", { class: "network-line", text: [transport, net.address].filter(Boolean).join(" · ") });
  }

  function emptyState() {
    return h("div", { class: "empty" }, icon("power"), h("h2", { text: S.empty_title }), h("p", { text: S.empty_text }),
      button("filled", S.action_add_device, () => showEdit(null)));
  }

  function deviceCard() {
    let device = null;
    let first = false;
    let last = false;
    let detailsSig = "";
    let actionsSig = "";
    const dot = h("div", { class: "status-dot", role: "img" });
    const name = h("div", { class: "card-name" });
    const statusEl = h("div", { class: "card-status" });
    const more = iconButton("more", S.action_more, (e) => {
      e.stopPropagation();
      openDeviceMenu(more, device, first, last);
    });
    const details = h("div", { class: "card-details" });
    const actions = h("div", { class: "card-actions" });
    const el = h("div", {
      class: "card", tabindex: "0", role: "button",
      onClick: () => showEdit(device.id),
      onKeydown: (e) => {
        if (e.key === "Enter" && e.target === el) showEdit(device.id);
      },
    }, h("div", { class: "card-head" }, dot, h("div", { class: "card-title" }, name, statusEl), more), details, actions);

    function update(d, isFirst, isLast) {
      device = d;
      first = isFirst;
      last = isLast;
      el.className = "card state-" + d.status.state;
      dot.classList.toggle("transition", isTransitional(d.status.state));
      dot.setAttribute("aria-label", stateDescription(d.status.state));
      dot.title = stateDescription(d.status.state);
      name.textContent = d.name;
      tick();
      const s = d.status;
      const dSig = JSON.stringify([d.host, d.mac, d.hasAgent, s.state, s.agent, s.agentError, s.notice]);
      if (dSig !== detailsSig) {
        detailsSig = dSig;
        renderDetails();
      }
      const aSig = s.state + "|" + d.canShutdown;
      if (aSig !== actionsSig) {
        actionsSig = aSig;
        renderActions();
      }
    }

    function tick() {
      if (device) statusEl.textContent = statusText(device.status, Date.now());
    }

    function renderDetails() {
      const s = device.status;
      const parts = [h("div", { text: [device.host, device.mac].filter(Boolean).join(" · ") })];
      if (s.state === "ONLINE" && s.agent && !s.agentError) {
        parts.push(h("div", { text: fmt(S.agent_info, s.agent.hostname, osLabel(s.agent.os), formatDuration(s.agent.uptime * 1000)) }));
      }
      if (device.hasAgent && s.agentError && s.state === "ONLINE") {
        parts.push(h("div", { class: "error", text: fmt(S.agent_problem, agentErrorLabel(s.agentError)) }));
      }
      if (s.notice) {
        const id = device.id;
        parts.push(h("div", { class: "card-notice" },
          h("span", { text: s.notice === "WAKE_TIMEOUT" ? S.notice_wake_timeout : S.notice_shutdown_timeout }),
          button("text", S.ok, (e) => {
            e.stopPropagation();
            api.call("clearNotice", { id }).catch(() => {});
          })));
      }
      details.replaceChildren(...parts);
    }

    function renderActions() {
      const d = device;
      const stop = (f) => (e) => {
        e.stopPropagation();
        f();
      };
      const parts = [];
      switch (d.status.state) {
        case "ONLINE":
          if (d.canShutdown) parts.push(button("tonal", S.action_shutdown, stop(() => requestPower(device, "shutdown")), "power"));
          break;
        case "OFFLINE":
        case "UNKNOWN":
          parts.push(button("filled", S.action_wake, stop(() => wake(device)), "power"));
          break;
        case "WAKING":
          parts.push(h("div", { class: "spinner" }), button("outlined", S.action_wake_again, stop(() => wake(device)), "refresh"));
          break;
        default:
          parts.push(h("div", { class: "spinner" }));
      }
      actions.replaceChildren(...parts);
    }

    return { el, update, tick };
  }

  function openDeviceMenu(anchor, device, isFirst, isLast) {
    const items = [{ label: S.action_wake_menu, icon: "power", onClick: () => wake(device) }];
    if (device.canShutdown) {
      for (const action of ["shutdown", "reboot", "sleep"]) {
        items.push({ label: actionLabel(action), onClick: () => requestPower(device, action) });
      }
    }
    items.push("divider", { label: S.action_edit, icon: "edit", onClick: () => showEdit(device.id) });
    if (!isFirst) items.push({ label: S.action_move_up, icon: "up", onClick: () => move(device, -1) });
    if (!isLast) items.push({ label: S.action_move_down, icon: "down", onClick: () => move(device, 1) });
    items.push({ label: S.action_delete, icon: "delete", onClick: () => confirmDelete(device, false) });
    openMenu(anchor, items);
  }

  async function wake(device) {
    try {
      const r = await api.call("wake", { id: device.id });
      snackbar(r.ok ? fmt(S.message_wake_sent, device.name) : fmt(S.message_wake_error, r.error || ""));
    } catch (e) {
      snackbar(fmt(S.message_wake_error, e.message));
    }
  }

  function requestPower(device, action) {
    if (!device.canShutdown) agentHelp(device);
    else if (state.settings.confirmPowerActions) powerConfirm(device, action);
    else power(device, action, false);
  }

  async function power(device, action, force) {
    try {
      const r = await api.call("power", { id: device.id, action, force });
      snackbar(r.ok ? fmt(sentMessage(action), device.name) : fmt(S.message_agent_error, device.name, agentErrorLabel(r.code)));
    } catch (e) {
      snackbar(errorMessage(e));
    }
  }

  async function move(device, offset) {
    try {
      await api.call("moveDevice", { id: device.id, offset });
    } catch (e) {
      snackbar(errorMessage(e));
    }
  }

  function powerConfirm(device, action) {
    let force = false;
    const body = [h("p", { style: "margin:0", text: S.confirm_power_text })];
    if (action !== "sleep") {
      const box = h("input", { type: "checkbox" });
      box.addEventListener("change", () => (force = box.checked));
      body.push(h("label", { class: "check-row" }, box, S.confirm_power_force));
    }
    const d = openDialog({
      iconName: "power",
      title: fmt(confirmTitle(action), device.name),
      body,
      actions: [
        { label: S.cancel, onClick: () => d.close() },
        { label: actionLabel(action), onClick: () => { d.close(); power(device, action, force); } },
      ],
    });
  }

  function confirmDelete(device, fromEdit) {
    const d = openDialog({
      title: fmt(S.confirm_delete_title, device.name),
      body: h("p", { style: "margin:0", text: S.confirm_delete_text }),
      actions: [
        { label: S.cancel, onClick: () => d.close() },
        {
          label: S.action_delete,
          onClick: async () => {
            d.close();
            try {
              await api.call("deleteDevice", { id: device.id });
              if (fromEdit) showList();
              else snackbar(fmt(S.message_deleted, device.name));
            } catch (e) {
              snackbar(errorMessage(e));
            }
          },
        },
      ],
    });
  }

  function agentHelp(device) {
    const d = openDialog({
      title: S.agent_help_title,
      body: h("p", { style: "margin:0", text: fmt(S.agent_help_text, device.name) }),
      actions: [
        { label: S.cancel, onClick: () => d.close() },
        { label: S.action_configure, onClick: () => { d.close(); showEdit(device.id); } },
      ],
    });
  }

  // ---------------------------------------------------------------------------------------------
  // Écran « ajouter / modifier un PC » (EditDeviceScreen)
  // ---------------------------------------------------------------------------------------------
  async function showEdit(id) {
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
    let testResult = null;
    let keyVisible = false;
    let showAdvanced = false;

    const fields = {};
    const onChange = (key) => (value) => {
      form[key] = value;
      clearErrors();
      setTestResult(null);
    };
    const make = (key, label, helper, opts = {}) => (fields[key] = field({ label, helper, value: form[key], onInput: onChange(key), ...opts }));

    const saveTop = button("text", S.save, save);
    const bar = topBar(isNew ? S.edit_title_new : S.edit_title, showList, [
      isNew ? null : iconButton("delete", S.action_delete, () => confirmDelete({ id: deviceId, name: form.name }, true)),
      saveTop,
    ]);

    const pairingCard = h("section", { class: "pairing-card" },
      h("h2", { text: S.pairing_title }),
      h("p", { text: S.pairing_text }),
      h("div", { class: "row" }, button("filled", S.action_paste_link, pasteLink, "paste")));

    make("name", S.field_name, null);
    make("mac", S.field_mac, S.field_mac_help, { mono: true });
    make("host", S.field_host, S.field_host_help);
    make("agentPort", S.field_agent_port, null);
    const keyToggle = button("text", S.show, () => {
      keyVisible = !keyVisible;
      fields.agentKey.input.type = keyVisible ? "text" : "password";
      keyToggle.textContent = keyVisible ? S.hide : S.show;
    });
    make("agentKey", S.field_agent_key, S.field_agent_key_help, { type: "password", mono: true, trailing: keyToggle });
    make("broadcast", S.field_broadcast, S.field_broadcast_help);
    make("wolPort", S.field_wol_port, S.field_wol_port_help);
    make("probePorts", S.field_probe_ports, S.field_probe_ports_help);
    make("secureOn", S.field_secure_on, S.field_secure_on_help, { mono: true });

    const agentSwitch = switchInput(form.agentEnabled, (checked) => {
      form.agentEnabled = checked;
      agentSection.classList.toggle("hidden", !checked);
      clearErrors();
      setTestResult(null);
    }, S.section_agent);

    const testButton = button("outlined", S.action_test_agent, testAgent);
    const testSpinner = h("div", { class: "spinner hidden" });
    const testSlot = h("div");
    const agentSection = h("div", { class: `column${form.agentEnabled ? "" : " hidden"}`, style: "padding:0;gap:12px" },
      fields.agentPort.wrap, fields.agentKey.wrap, h("div", { class: "row" }, testButton, testSpinner), testSlot);

    const advancedIcon = h("span");
    const advancedButton = h("button", { class: "btn text expander", onClick: () => setAdvanced(!showAdvanced) }, S.section_advanced, advancedIcon);
    const advancedSection = h("div", { class: "column hidden", style: "padding:0;gap:12px" },
      fields.broadcast.wrap, fields.wolPort.wrap, fields.probePorts.wrap, fields.secureOn.wrap);

    const content = h("main", { class: "content" }, h("div", { class: "column form" },
      pairingCard,
      h("h2", { class: "section-title", text: S.section_device }),
      fields.name.wrap, fields.mac.wrap, fields.host.wrap,
      h("div", { class: "divider" }),
      h("div", { class: "row spread" },
        h("div", { class: "grow" }, h("h2", { class: "section-title", text: S.section_agent }), h("div", { class: "small muted", text: S.section_agent_help })),
        agentSwitch.el),
      agentSection,
      h("div", { class: "divider" }),
      advancedButton,
      advancedSection,
      button("filled full", S.save, save),
      h("div", { style: "height:24px" })));
    const el = h("div", { class: "screen" }, bar, content);
    watchScroll(content, bar);
    setAdvanced(false);
    setScreen({ name: "edit", el });
    if (isNew) fields.name.input.focus();

    function setAdvanced(open) {
      showAdvanced = open;
      advancedSection.classList.toggle("hidden", !open);
      advancedIcon.replaceChildren(icon(open ? "up" : "down"));
    }

    function clearErrors() {
      for (const f of Object.values(fields)) f.setError(null);
    }

    function setTestResult(result) {
      testResult = result;
      if (!result) {
        testSlot.replaceChildren();
        return;
      }
      let text;
      if (result.ok) text = fmt(S.test_ok, result.status.hostname, osLabel(result.status.os), result.status.version);
      else if (result.invalid) text = result.invalid;
      else text = agentErrorLabel(result.code);
      testSlot.replaceChildren(h("div", { class: `result-card ${result.ok ? "ok" : "ko"}` },
        icon(result.ok ? "check" : "warning"), h("span", { class: "selectable", text })));
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
        showList();
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
      const input = field({ label: "wolagent://pair?…", multiline: true, onInput: (v) => d.setActions(actions(v)) });
      const actions = (v) => [
        { label: S.cancel, onClick: () => d.close() },
        { label: S.ok, disabled: !v.trim(), onClick: () => { d.close(); applyPairing(input.input.value); } },
      ];
      const d = openDialog({
        title: S.paste_title,
        body: [h("p", { style: "margin:0", text: S.paste_text }), input.wrap],
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
  // Écran « réglages » (SettingsScreen)
  // ---------------------------------------------------------------------------------------------
  function showSettings() {
    let busy = false;
    const progress = h("div", { class: "progress hidden" });
    const bar = topBar(S.settings_title, showList);

    const poll = sliderSetting(S.setting_poll_interval, 1, 30, 1, (v) => updateSettings({ pollIntervalSeconds: v }));
    const wakeTimeout = sliderSetting(S.setting_wake_timeout, 60, 600, 30, (v) => updateSettings({ wakeTimeoutSeconds: v }));
    const confirmSwitch = switchInput(state.settings.confirmPowerActions, (checked) => updateSettings({ confirmPowerActions: checked }), S.setting_confirm);

    const exportSupporting = h("div", { class: "supporting" });
    const exportItem = listItem(S.action_export, exportSupporting, () => exportDialog());
    const importItem = listItem(S.action_import, S.action_import_help, () => pickImportFile());

    const content = h("main", { class: "content" }, h("div", { class: "column list-items" },
      progress,
      h("h2", { class: "header", text: S.section_monitoring }),
      poll.el,
      wakeTimeout.el,
      h("div", { class: "list-item" }, h("div", { class: "texts" }, h("div", { class: "headline", text: S.setting_confirm }), h("div", { class: "supporting", text: S.setting_confirm_help })), confirmSwitch.el),
      h("div", { class: "divider" }),
      h("h2", { class: "header", text: S.section_backup }),
      exportItem,
      importItem,
      h("div", { class: "divider" }),
      h("h2", { class: "header", text: S.section_agent_download }),
      listItem(S.agent_download, S.agent_download_help, () => openUrl(RELEASES_URL)),
      h("div", { class: "divider" }),
      h("h2", { class: "header", text: S.section_about }),
      listItem(S.about_version, state.version),
      listItem(S.about_source, REPO_URL, () => openUrl(REPO_URL)),
      listItem(S.about_security, S.about_security_text)));
    const el = h("div", { class: "screen" }, bar, content);
    watchScroll(content, bar);

    function render() {
      poll.set(state.settings.pollIntervalSeconds);
      wakeTimeout.set(state.settings.wakeTimeoutSeconds);
      confirmSwitch.input.checked = state.settings.confirmPowerActions;
      exportSupporting.textContent = fmt(S.action_export_help, state.devices.length);
      exportItem.classList.toggle("disabled", busy || state.devices.length === 0);
      importItem.classList.toggle("disabled", busy);
      progress.classList.toggle("hidden", !busy);
    }

    function setBusy(value) {
      busy = value;
      render();
    }

    async function updateSettings(patch) {
      try {
        await api.call("updateSettings", patch);
      } catch (e) {
        snackbar(errorMessage(e));
      }
    }

    function exportDialog() {
      let withSecrets = state.hasSecrets;
      let password = "";
      let confirmation = "";
      const passwordField = field({ label: S.field_password, helper: fmt(S.field_password_help, MIN_PASSWORD_LENGTH), type: "password", onInput: (v) => { password = v; refresh(); } });
      const confirmField = field({ label: S.field_password_confirm, type: "password", onInput: (v) => { confirmation = v; refresh(); } });
      const secretsBox = h("div", { class: "column", style: "padding:8px 0 0;gap:12px" }, passwordField.wrap, confirmField.wrap);
      const radioWith = h("input", { type: "radio", name: "export-kind", checked: withSecrets });
      const radioWithout = h("input", { type: "radio", name: "export-kind", checked: !withSecrets });
      radioWith.addEventListener("change", () => { withSecrets = true; refresh(); });
      radioWithout.addEventListener("change", () => { withSecrets = false; refresh(); });
      const choice = (radio, title, subtitle) => h("label", { class: "choice" }, radio,
        h("span", { class: "texts" }, h("span", { text: title }), h("span", { class: "subtitle", text: subtitle })));
      const d = openDialog({
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
        confirmField.setError(confirmation.length > 0 && password !== confirmation ? " " : null);
        d.setActions([
          { label: S.cancel, onClick: () => d.close() },
          { label: S.action_export_short, disabled: !valid(), onClick: submit },
        ]);
      }
      async function submit() {
        d.close();
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

    render();
    setScreen({ name: "settings", el, onState: render, setBusy });
  }

  function sliderSetting(title, min, max, step, onCommit) {
    const valueEl = h("span", { class: "value" });
    const input = h("input", { type: "range", min, max, step, "aria-label": title });
    input.addEventListener("input", () => (valueEl.textContent = fmt(S.setting_seconds_value, input.value)));
    input.addEventListener("change", () => onCommit(Number(input.value)));
    const el = h("div", { class: "slider-setting" }, h("div", { class: "row" }, h("span", { class: "headline", text: title }), valueEl), input);
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

  function listItem(headline, supporting, onClick) {
    return h("div", { class: `list-item${onClick ? " clickable" : ""}`, tabindex: onClick ? "0" : null, role: onClick ? "button" : null, onClick,
      onKeydown: onClick ? (e) => { if (e.key === "Enter") onClick(); } : null },
    h("div", { class: "texts" }, h("div", { class: "headline", text: headline }),
      supporting instanceof Node ? supporting : h("div", { class: "supporting selectable", text: supporting })));
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

  const setBusy = (v) => current?.setBusy?.(v);

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
    const d = openDialog({ title: S.import_password_title, body: pwd.wrap, onDismiss: cancelImport });
    refresh();

    function refresh() {
      d.setActions([
        { label: S.cancel, onClick: () => { d.close(); cancelImport(); } },
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
          d.close();
          handleImportStep(r);
        }
      } catch (e) {
        d.close();
        snackbar(errorMessage(e));
      } finally {
        busy = false;
        setBusy(false);
        refresh();
      }
    }
  }

  function importConfirmDialog(step) {
    const body = [h("p", { style: "margin:0", text: S.import_confirm_text })];
    if (step.missingKeys) body.push(h("p", { class: "error", style: "margin:0", text: S.import_missing_keys }));
    const d = openDialog({
      title: fmt(S.import_confirm_title, step.count),
      body,
      onDismiss: cancelImport,
      actions: [
        { label: S.cancel, onClick: () => { d.close(); cancelImport(); } },
        { label: S.import_replace, onClick: () => confirm(true) },
        { label: S.import_merge, onClick: () => confirm(false) },
      ],
    });
    async function confirm(replace) {
      d.close();
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
    if (e.key === "Escape" && closeTop()) {
      e.preventDefault();
      return;
    }
    // Pas de rechargement ni d'impression de la page : F5 actualise l'état des PC.
    const key = e.key.toLowerCase();
    if (e.key === "F5" || ((e.ctrlKey || e.metaKey) && (key === "r" || key === "p"))) {
      e.preventDefault();
      if (e.key === "F5") current?.onRefreshKey?.();
    }
  });
  document.addEventListener("contextmenu", (e) => {
    if (!e.target.closest("input, textarea, .selectable")) e.preventDefault();
  });
  document.addEventListener("visibilitychange", () => {
    api.call("setVisible", { visible: document.visibilityState === "visible" }).catch(() => {});
  });

  api.onState((s) => {
    state = s;
    current?.onState?.();
  });

  async function start() {
    try {
      state = await api.call("getState");
    } catch (e) {
      appEl.replaceChildren(h("p", { style: "padding:24px", text: errorMessage(e) }));
      return;
    }
    showList();
    setInterval(() => current?.onTick?.(), 1000);
    api.call("uiReady", { screen: current?.name, devices: state.devices.length, empty: !!document.querySelector(".empty") }).catch(() => {});
    if (state.startupMessage) {
      alertDialog(S.app_name, state.startupMessage);
      api.call("dismissStartupMessage").catch(() => {});
    }
  }

  start();
})();
