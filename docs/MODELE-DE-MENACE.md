# Modèle de menace

Ce document décrit ce que Patronus protège, contre qui, et comment. Il sert de base au protocole chiffré
`wolagent/2` ([PROTOCOLE-V2.md](PROTOCOLE-V2.md)) et sera relu à chaque étape de la feuille de route (hub sur le NAS,
notifications, accès à distance).

- **État couvert** : Patronus 1.15.0 et agent 1.13.0 : `wolagent/2` en service, partage v2 compris, `wolagent/1`
  gardé pendant la transition (applications et agents plus anciens, téléphones sous Android 8 ou 9) ; verrouillage
  facultatif de l'application Android ; agent sur PC (Windows, Linux, macOS) et NAS Synology.
- **Signaler une faille** : [SECURITY.md](../SECURITY.md). Détail des mesures actuelles : [SECURITE.md](SECURITE.md).

## 1. Système étudié

| Composant | Rôle | Ce qu'il détient |
|---|---|---|
| Application Android | Démarre, éteint, surveille les PC ; partage, sauvegardes | Liste des PC, clés d'agent `wolagent/1`, historique, clés de partage, mot de passe des sauvegardes, jeton GitHub (Gists) |
| Application Windows | Mêmes fonctions | Idem (chiffré par DPAPI) |
| Agent (service Windows `SYSTEM`, systemd `root`, launchd) | Exécute les commandes nommées, tient le journal (30 j), les mesures (90 j), la fiche du PC | Clé `wolagent/1` du PC, journal, mesures ; écoute le port TCP 9770 |
| GitHub | Code, Releases (mises à jour), Gists (partage, sauvegardes, archives), connexion OAuth « appareil » | Fichiers chiffrés, métadonnées |
| CI GitHub Actions | Compile, signe et publie les versions | Clé de signature (environnement `publication`) |
| Hub sur le NAS (2.0, en conception : [HUB.md](HUB.md)) | Surveillance continue, historique long, détection des pannes, programmations ; conteneur non root | Son identité `wolagent/2`, la clé de ses données, l'historique d'un an des PC confiés, leurs adresses, MAC et empreintes ; rôle `wake` (`control` pour une extinction programmée) sur chaque PC confié |
| Futur : relais de notifications, VPN | Alertes, accès à distance | Étapes 2.1 et 2.2 |

Flux étudiés :

| Flux | Description | Canal |
|---|---|---|
| F1 | Application ↔ agent : état, journal, mesures, fiche, commandes | TCP, réseau local (`wolagent/1`, puis `wolagent/2`) |
| F2 | Application → PC : paquet magique | UDP (diffusion), non authentifié par nature |
| F3 | Application ↔ GitHub : partage, sauvegardes, archives des mesures | HTTPS |
| F4 | Applications et agent ← GitHub Releases : mises à jour | HTTPS + manifeste signé |
| F5 | Agent ↔ système local : capteurs (WMI, NVML, LibreHardwareMonitor en `127.0.0.1`), alimentation | Local |
| F6 | CI → Releases : publication | GitHub |
| F7 | Agent → application : appairage | Canal optique (QR code) ou lien copié |
| F8 | Propriétaire ↔ personne invitée : partage | Liens, voix (code de vérification), Gist |
| F9 | Application ↔ hub : états, évènements, historique long, administration du hub (2.0) | TCP 9771, réseau local (`patronus-hub/1`) |
| F10 | Hub ↔ agents : relevés, réveils notés, extinctions programmées (2.0) | `wolagent/2`, rôle `wake` ou `control` |
| F11 | Hub → appareils sans agent : sondes (2.0) | TCP, HTTP(S) sans données, ICMP facultatif |
| F12 | Hub → PC : paquet magique des réveils programmés (2.0) | UDP (diffusion) |

## 2. Actifs à protéger

| Actif | Sensibilité | Propriété visée |
|---|---|---|
| A1 · Commande des PC (éteindre, redémarrer, mettre en veille) | Élevée | Intégrité, autorisation |
| A2 · Clé d'agent `wolagent/1` (secret partagé de 256 bits, une par PC) | Critique : elle donne tout | Confidentialité |
| A3 · Clés d'identité `wolagent/2` (une paire de clés par appareil, agent ou hub) | Critique | Confidentialité ; non exportables quand la plateforme le permet |
| A4 · Données de présence : état, journal (heures de démarrage et d'arrêt, qui a fait quoi), mesures, disques, fiche du PC | Élevée : elles révèlent la présence au domicile et les habitudes | Confidentialité |
| A5 · Inventaire du réseau : noms des PC, adresses IP et MAC | Moyenne | Confidentialité |
| A6 · Clés de partage (signature du propriétaire, réception de chaque appareil) | Élevée | Confidentialité |
| A7 · Mot de passe des sauvegardes et jeton GitHub (droit `gist`) | Élevée | Confidentialité |
| A8 · Clé de signature des versions (APK et manifeste des mises à jour) | Critique | Confidentialité, usage contrôlé |
| A9 · Intégrité des logiciels installés et de leurs mises à jour | Critique | Intégrité |
| A10 · Disponibilité : pouvoir allumer, éteindre et consulter au besoin | Moyenne | Disponibilité |
| A11 · Rapports de diagnostic | Moyenne | Confidentialité (déjà chiffrés et sans secret) |
| A12 · Historique longue durée du hub (journal et mesures d'un an ou plus, évènements) | Élevée : comme A4, sur une durée bien plus longue | Confidentialité, intégrité |
| A13 · Clé des données du hub | Critique pour A12 | Confidentialité ; hors du dossier de données et des sauvegardes |

## 3. Adversaires

| Id | Qui | Capacités | Ce qu'il ne doit jamais obtenir |
|---|---|---|---|
| M1 | Appareil espion sur le réseau local : objet connecté piraté, Wi-Fi invité mal isolé | Écoute, injection, usurpation ARP/DHCP (homme du milieu), balayage de ports, paquets magiques | Lire états, journal ou mesures ; commander un PC ; se faire passer pour un agent ou pour une application |
| M2 | Quelqu'un sur Internet | Atteint ce qui est exposé (rien par défaut), lit le dépôt public | Atteindre un agent ou le hub |
| M3 | Autre utilisateur du projet | Même application, même code ; peut fabriquer liens et fichiers | Voir les appareils ou les données d'un autre ; s'inviter |
| M4 | Intermédiaire : GitHub, relais de notifications, service VPN, FAI | Voit et peut altérer ce qui passe par lui | Lire le contenu transporté ; le modifier sans être détecté |
| M5 | Personne dont l'accès a été retiré : ancien partage, appareil remplacé, ex-colocataire | Garde des copies (clés `wolagent/1`, fichiers, téléphone) | Continuer à consulter ou commander |
| M6 | Voleur d'un téléphone ou d'un PC portable | Appareil verrouillé, ou déverrouillé un moment | Extraire des clés ; garder l'accès après révocation |
| M7 | Attaquant ayant pris le NAS (futur hub) | Contrôle du conteneur et de ses fichiers | Prendre la main sur tous les PC sans laisser de trace |
| M8 | Pirate de la chaîne de production : compte GitHub, action de CI, dépendance, outil de compilation | Modifier le code ou la compilation | Diffuser une version piégée acceptée par les mises à jour |
| M9 | Logiciel malveillant ou utilisateur sans droits d'administrateur sur un PC équipé de l'agent | Fichiers et processus de l'utilisateur, connexions locales (`127.0.0.1`) | Lire les clés de l'agent ; se servir de l'agent pour obtenir plus de droits |
| M10 | L'auteur du projet | Publie le code et les versions | Voir quoi que ce soit chez les utilisateurs |
| M11 | Autre utilisateur du NAS : compte DSM sans droits d'administrateur, ou accès aux dossiers partagés et aux sauvegardes | Lit les dossiers partagés auxquels il a droit, les copies Hyper Backup | Lire l'historique du hub ; se faire passer pour le hub |

## 4. Hypothèses

- **H1** Les systèmes et leurs magasins de clés (Keystore et StrongBox d'Android, DPAPI, TPM) ne sont pas compromis.
  Un administrateur du PC est de confiance : il contrôle de toute façon la machine.
- **H2** Les primitives utilisées (TLS 1.3, AES-GCM, ECDSA et ECDH P-256, HMAC-SHA256, HKDF, PBKDF2) et leurs
  implémentations de plateforme (BoringSSL via Android, bibliothèque standard de Go) sont sûres.
- **H3** L'utilisateur scanne le QR code de son propre PC, et compare les codes de vérification quand on le lui demande.
- **H4** Le réseau local est privé : aucun agent n'est exposé sur Internet. Un appareil hostile peut s'y trouver (M1).
- **H5** GitHub authentifie correctement ses serveurs en HTTPS. Son contenu n'est pas confidentiel vis-à-vis de GitHub.
- **H6** Seule l'horloge de l'agent fait foi pour les expirations (codes d'appairage). Celle des applications ne sert
  qu'à l'affichage.

## 5. Menaces et protections

« Actuel » désigne `wolagent/1` (Patronus 1.11, et les appareils pas encore passés en `wolagent/2`) ; « Prévu »
`wolagent/2`, **en service depuis Patronus 1.13.0 et l'agent 1.11.0**, partage v2 compris, sauf ce qui est marqué
*(à venir)* ou attribué à une étape suivante.

### F1 · Application ↔ agent

| Id | Menace | Adv. | Actuel | Prévu | Résiduel |
|---|---|---|---|---|---|
| T1 | Lecture du trafic : noms des PC, état, journal, mesures, fiche | M1 | **Aucune** : authentifié, non chiffré | TLS 1.3, chiffrement AEAD, confidentialité persistante | Métadonnées : existence d'un agent, horaires et volumes des connexions |
| T2 | Commande forgée ou modifiée | M1 | HMAC-SHA256 en défi-réponse | TLS mutuel + rôle de l'appareil | — |
| T3 | Rejeu d'une requête ou d'une réponse | M1 | Nonces des deux côtés | TLS 1.3, sans 0-RTT ni reprise de session | — |
| T4 | Faux agent : réponses mensongères, collecte de ce que l'application envoie | M1 | Réponses signées par la clé partagée | Empreinte de l'agent épinglée (lue dans le QR code) | Appairage manuel sans QR code : comparaison visuelle d'une empreinte courte |
| T5 | Homme du milieu pendant l'appairage | M1 | Le QR code transmet la clé par un canal optique ; un lien envoyé par message l'expose | Empreinte de l'agent dans le QR code ; code à usage unique transmis dans le tunnel déjà authentifié | — |
| T6 | Photo ou copie du QR code ou du lien | M1, M5 | La clé est valable jusqu'à `wol-agent rotate-key` | Code à usage unique, 10 min ; inutile ensuite. Un usage par un tiers avant l'utilisateur fait échouer son appairage (alerte), et l'appareil intrus est visible et révocable | Fenêtre de 10 min si quelqu'un d'autre utilise le code avant l'utilisateur |
| T7 | Copie de la clé : téléphone, export complet, fichier de partage | M5, M6 | Toute copie donne tout ; retirer un accès oblige à changer la clé et à la redonner aux autres appareils | Clé propre à chaque appareil, non exportable ; révocation individuelle immédiate | Plateformes sans stockage matériel : clé protégée par le système uniquement |
| T8 | Droits trop larges : partager la surveillance donne le droit d'éteindre | M5 | Liste blanche de commandes par PC ; partage « démarrer » sans clé d'agent | Rôles par appareil : consulter, démarrer, contrôler, administrer | — |
| T9 | Force brute, saturation du port | M1 | Réseaux privés seulement, 5 échecs par minute → blocage 5 min, 16 connexions, 8 Kio, 10 s | Idem, plus : tentatives d'appairage limitées par adresse et au total, identités inconnues réduites aux commandes d'appairage | Saturation depuis le réseau local (disponibilité seulement) |
| T10 | Usurpation d'adresse IP pour fausser le journal | M1 | Nom déclaré par l'application, non vérifié | Journal attribué à l'identité authentifiée de l'appareil | — |
| T11 | Rétrogradation : forcer l'application à parler `wolagent/1` | M1 | — | Une application qui connaît l'empreinte `wolagent/2` d'un PC ne parle plus jamais `wolagent/1` à ce PC ; `wolagent/1` désactivable puis retiré | Appareils pas encore migrés |
| T12 | Homme du milieu pendant la migration `wolagent/1` → `wolagent/2` | M1 | — | Preuves croisées avec la clé `wolagent/1`, liées aux empreintes TLS vues de chaque côté | — |

### F2 · Paquet magique

| Id | Menace | Adv. | Actuel | Prévu | Résiduel |
|---|---|---|---|---|---|
| T13 | Réveil d'un PC par n'importe qui sur le réseau | M1 | Accepté (norme Wake-on-LAN) | Le hub signalera les réveils non demandés (2.1) | Accepté : un réveil ne donne aucun autre accès |

### F3, F8 · GitHub : partage, sauvegardes, archives

| Id | Menace | Adv. | Actuel | Prévu | Résiduel |
|---|---|---|---|---|---|
| T14 | Lecture des Gists | M2, M4 | Chiffrement de bout en bout : pour la clé d'un appareil (partage), par mot de passe (sauvegardes, archives) | Partage v2 (1.13.0) : plus de clé d'agent dans les fichiers, seulement une invitation à usage unique, réservée à l'appareil destinataire | GitHub voit l'existence, la taille et les dates des Gists ; clé encore transmise en partage v1 (application antérieure à 1.13 chez la personne) |
| T15 | Faux fichier d'accès, fausse invitation, fausse demande | M3, M4 | Signature ECDSA du propriétaire, code de vérification comparé de vive voix | Inchangé | Utilisateur qui ne compare pas le code (H3) |
| T16 | Retour à une ancienne révision d'un accès | M4 | Numéro de révision croissant | Inchangé ; et révocation sur l'agent lui-même | — |
| T17 | Vol du jeton GitHub | M6, M9 | Droit `gist` seulement, chiffré sur l'appareil, jamais exporté | Inchangé | Lecture et suppression des Gists (contenu chiffré) |
| T18 | Attaque hors ligne d'une sauvegarde | M4 | PBKDF2 (600 000 itérations) + AES-256-GCM | Inchangé | Mot de passe faible |

### F4, F6 · Mises à jour et chaîne de production

| Id | Menace | Adv. | Actuel | Prévu | Résiduel |
|---|---|---|---|---|---|
| T19 | Version piégée publiée | M8 | Étape 1 : actions épinglées, clé de signature derrière une validation manuelle, provenance attestée, inventaire des composants, govulncheck, double authentification | Vérification de la provenance par les mises à jour intégrées *(à venir)* | Compte du propriétaire compromis avec son second facteur |
| T20 | Faux manifeste, fichier modifié en route | M1, M4 | Manifeste signé, empreinte SHA-256 de chaque fichier, même clé exigée par Android | Inchangé | — |
| T21 | Retour à une version vulnérable | M4 | Seule une version plus récente est proposée et installée | Inchangé | — |
| T22 | Remplacement de l'agent pendant sa mise à jour | M9 | Dossier réservé aux administrateurs, empreinte vérifiée, retour arrière automatique | Inchangé | — |

### F5 · Agent sur le PC

| Id | Menace | Adv. | Actuel | Prévu | Résiduel |
|---|---|---|---|---|---|
| T23 | Lecture de la clé ou de la configuration de l'agent | M9 | Fichiers réservés à `root` / `SYSTEM` / Administrateurs | Clé d'identité protégée de même, plus DPAPI (machine) sous Windows, puis TPM *(à venir)* | Administrateur local (H1) |
| T24 | Se servir de l'agent pour obtenir plus de droits | M9 | Commandes nommées uniquement, jamais de commande libre ; `127.0.0.1` exige quand même la clé | Une connexion locale exige une identité enregistrée ou un code d'appairage | — |
| T25 | Pilotage des ventilateurs via le serveur web de LibreHardwareMonitor | M1, M9 | L'agent le lit en local ; conseil de ne pas l'ouvrir dans le pare-feu | Inchangé | Configuration de LibreHardwareMonitor par l'utilisateur |

### Appareils des utilisateurs

| Id | Menace | Adv. | Actuel | Prévu | Résiduel |
|---|---|---|---|---|---|
| T26 | Vol d'un téléphone verrouillé | M6 | Données chiffrées par une clé du Keystore | Identité dans StrongBox ; révocation depuis un autre appareil | — |
| T27 | Téléphone déverrouillé utilisé par un tiers | M6 | L'application est utilisable | Révocation ; verrouillage facultatif de l'application Android (1.14.0) : empreinte, visage ou code du téléphone avant les actions sensibles et/ou à l'ouverture | Sans ce verrouillage, tant que l'appareil n'est pas révoqué ; avec, la minute qui suit une authentification, ou une personne qui connaît le code du téléphone. Windows : verrouillage de la session |
| T28 | Fuite d'un export complet | M4, M6 | Chiffré par mot de passe | Les identités `wolagent/2` ne sont pas exportées : un nouvel appareil s'appaire à nouveau (QR code ou invitation d'un appareil administrateur) | Clés `wolagent/1` encore présentes tant que la migration n'est pas finie |
| T29 | Rapport de diagnostic transmis à un tiers | M4 | Sans clé, sans jeton, sans mot de passe ; chiffré | Inchangé | — |

### F9 à F12 · Hub sur le NAS (2.0, en conception)

Conception : [HUB.md](HUB.md). Rien de cela n'est encore réalisé.

| Id | Menace | Adv. | Protection prévue | Résiduel |
|---|---|---|---|---|
| T30 | NAS compromis (root sur DSM ou conteneur pris) : prise de contrôle de tous les PC | M7 | Hub sans privilège sur les agents : `wake` par défaut, `control` seulement pour les PC à extinction programmée, **jamais** `admin` ; aucune commande des applications relayée ; ses rôles ne changent que par une application administrateur, directement auprès de l'agent ; chaque action notée par l'agent avec l'empreinte du hub | Lecture de tout ce que sait le hub (A4, A5, A12) ; réveils ; extinction, redémarrage ou veille des PC à extinction programmée ; le NAS lui-même |
| T34 | Faux hub sur le réseau : collecte de ce que les applications envoient, faux états | M1 | Empreinte du hub épinglée à l'appairage (code à usage unique, 10 min) ; TLS 1.3 mutuel | — |
| T35 | Appareil non autorisé ou révoqué qui interroge ou administre le hub | M1, M5 | Registre et rôles du hub comme sur un agent (`view`, `wake`, `admin`) ; révocation immédiate, connexions fermées | — |
| T36 | Confusion agent / hub : une réponse de l'un prise pour celle de l'autre | M7 | ALPN (`patronus-hub/1`) et port (9771) distincts, empreintes distinctes | — |
| T37 | Copie du dossier de données du hub ou d'une sauvegarde | M11, M4 | Fichiers chiffrés (AES-256-GCM, une clé par fichier dérivée par HKDF) ; clé dans un dossier séparé, hors des dossiers partagés et des sauvegardes | Disque du NAS volé : la clé est sur le même disque, sauf volume chiffré par DSM ; en clair, nombre de PC, mois couverts, tailles |
| T38 | Hub compromis qui ment aux applications : faux états, historique réécrit | M7 | Le journal de l'agent fait foi sur la période qu'il couvre (règles de fusion communes) ; états du hub datés et marqués « via le hub » ; évènements réduits à des codes, rédigés par l'application avec ses propres noms de PC, toute précision inattendue écartée (application Windows, 2.0-F3) ; commandes toujours envoyées aux agents | Appareils sans agent ; historique au-delà des 30 jours de l'agent |
| T39 | Évasion du conteneur, élévation de privilèges sur le NAS | M7 | Utilisateur 65532, système de fichiers en lecture seule, aucune capacité, `no-new-privileges`, pas de socket Docker, limites de mémoire et de processus | Réseau de l'hôte (nécessaire aux paquets magiques) : le hub joint tout le réseau local, comme n'importe quel appareil (M1) |
| T40 | Hub détourné pour sonder d'autres réseaux ou Internet | M7 | Sondes limitées aux adresses privées ; aucune donnée envoyée ; résultat réduit à « répond » et latence | — |
| T41 | Premier code d'appairage lu dans le journal du conteneur | M7, M11 | Journal visible des seuls administrateurs DSM ; usage unique, 10 min ; « Ce code a déjà servi » et appareil intrus visible et révocable | Administrateur DSM (de confiance, comme H1) |
| T42 | Programmation détournée : extinction en pleine utilisation | M7 | Extinction seulement si le PC est inactif, jamais forcée ; rôle `control` donné PC par PC et retiré avec la programmation ; chaque extinction notée « par Hub NAS » | Hub compromis : extinction, redémarrage ou veille des PC à extinction programmée |
| T43 | Image du hub piégée | M8 | Construite en CI depuis le même commit, publiée après validation manuelle, provenance attestée, SBOM ; image épinglable par empreinte | Comme T19 |
| T44 | Saturation des agents ou du NAS par le hub | M7 | Fréquences de relevé bornées, une connexion gardée par agent, délais doublés après un refus, limites du conteneur | Disponibilité seulement |
| T45 | Hub encore autorisé sur un PC après son retrait | M5, M7 | Retirer un PC du hub retire aussi le hub de l'agent (`devices.revoke`) ; `wol-agent devices revoke` sur le PC | — |

### Étapes suivantes (rappel)

| Id | Menace | Adv. | Prévu |
|---|---|---|---|
| T31 | Lecture des alertes par le relais de notifications | M4 | Web Push chiffré pour la clé du téléphone, contenu minimal, identifiants opaques (2.1) |
| T32 | Agent ou hub joignable depuis Internet | M2 | Jamais exposés ; accès par VPN, TLS mutuel maintenu dans le VPN (2.2) |
| T33 | L'auteur du projet voit les données | M10 | Aucun serveur ni télémétrie ; code et provenance des versions vérifiables |

## 6. Risques acceptés et hors périmètre

- **Paquet magique non authentifié** (T13) : n'importe qui sur le réseau local peut allumer un PC, sans aucun autre accès.
- **Administrateur du PC** : il peut lire la clé de l'agent, le remplacer ou l'arrêter (H1).
- **Saturation depuis le réseau local** (T9) : elle peut empêcher temporairement de piloter un PC, sans rien révéler.
- **Métadonnées réseau** : un observateur du réseau local voit qu'un appareil parle à un PC, quand, et combien.
- **Exécutable Windows non signé par un éditeur** jusqu'à l'étape 2.3 (avertissement SmartScreen).
- **Code source public** : aucune sécurité ne repose sur le secret du code.
- **Android 8 et 9** : sans TLS 1.3 natif, ils ne pourront pas utiliser `wolagent/2`. Ils restent en `wolagent/1`
  (authentifié, non chiffré) avec les PC qui l'acceptent ; l'application exigera Android 10 quand `wolagent/2` deviendra
  obligatoire ([PROTOCOLE-V2.md](PROTOCOLE-V2.md#13-décisions)).
- **Root sur le NAS** (T30) : il lit tout ce que sait le hub, réveille les PC et peut éteindre ceux à extinction
  programmée ; il ne prend pas la main sur les autres PC.
- **Réseau de l'hôte pour le conteneur du hub** (T39) : nécessaire aux paquets magiques en diffusion.

## 7. Exigences de sécurité

Exigences tirées des menaces ci-dessus. La spécification `wolagent/2` indique comment elle répond à chacune ; la
dernière colonne donne leur état dans Patronus 1.15.0 et l'agent 1.13.0.

| Id | Exigence | Menaces | 1.15.0 |
|---|---|---|---|
| R1 | Tout échange application ↔ agent est chiffré et authentifié (TLS 1.3 uniquement, AEAD, confidentialité persistante). | T1, T2, T3 | Oui en `wolagent/2` ; Android 8 et 9 restent en `wolagent/1` |
| R2 | Authentification mutuelle par clés publiques épinglées, sans autorité de certification. | T4, T5 | Oui |
| R3 | Une identité par appareil, agent et hub ; clé privée générée sur place, non exportable quand la plateforme le permet. | T7, T26, T28 | Oui : Keystore Android (StrongBox si présent), DPAPI sous Windows ; TPM à venir |
| R4 | Autorisation par rôle, au plus juste par défaut ; la liste blanche de commandes du PC reste appliquée par-dessus. | T8, T30 | Oui, partage compris : « démarrer et consulter » donne le rôle `wake`, « démarrer et éteindre » le rôle `control` |
| R5 | Révocation individuelle, effective immédiatement et vérifiée par l'agent lui-même. | T7, T16, T27 | Oui : sur le PC ou depuis un appareil administrateur ; un accès partagé retiré l'est aussi sur les PC (partage v2) |
| R6 | Appairage sans secret durable dans le QR code ou le lien : code à usage unique, expirant, limité en tentatives. | T5, T6 | Oui ; l'ancien QR code reste possible sur demande (`--v1`) |
| R7 | Migration depuis `wolagent/1` sans rescanner, résistante à l'homme du milieu, sans élargir les droits. | T12 | Oui |
| R8 | Aucune rétrogradation silencieuse vers `wolagent/1` une fois `wolagent/2` établi avec un PC. | T11 | Oui |
| R9 | Limites de taille, de durée et de débit ; une identité inconnue n'accède qu'aux commandes d'appairage. | T9, T24 | Oui |
| R10 | Chaque action notée au journal de l'agent est attribuée à une identité authentifiée. | T10, T30 | Oui en `wolagent/2` |
| R11 | Aucune cryptographie maison : TLS de la plateforme ; seules des constructions HMAC documentées, à séparation de domaine, testées par vecteurs. | Toutes | Oui |
| R12 | Vecteurs de test communs Go et Kotlin, cas négatifs et fuzzing des analyseurs, en CI. | Toutes | Oui, plus des tests de bout en bout contre le vrai agent |
| R13 | Le hub n'est administrateur d'aucun agent et ne relaie aucune commande : `wake` par défaut, `control` seulement pour une extinction programmée, donné et retiré par une application administrateur auprès de l'agent. | T30, T42 | 2.0 (conception) |
| R14 | Toute action du hub sur un PC est attribuée à son identité dans le journal de l'agent. | T30, T42 | 2.0 (conception) ; l'agent le fait déjà (R10) |
| R15 | Applications ↔ hub : mêmes garanties que `wolagent/2` (R1, R2, R3, R5, R6, R9), avec ALPN et port distincts. | T34, T35, T36 | En cours : côté hub fait (2.0-C, [PROTOCOLE-HUB.md](PROTOCOLE-HUB.md)) ; applications : client Kotlin (2.0-E), application Windows (2.0-F, appairage et administration), Android à venir |
| R16 | Données du hub chiffrées au repos, clé hors du dossier de données et des sauvegardes. | T37 | En cours : stockage chiffré (2.0-B), registre des appareils du hub (2.0-C) |
| R17 | Conteneur sans privilège : non root, lecture seule, aucune capacité, pas de socket Docker ; seule exception, le réseau de l'hôte. | T39 | 2.0 (conception) |
| R18 | Sondes limitées aux adresses privées, sans données envoyées, résultat réduit à « répond » et latence. | T40 | 2.0 (conception) |
| R19 | Image et binaire du hub construits et publiés comme les autres versions : validation manuelle, provenance, SBOM. | T43 | 2.0 (conception) |
| R20 | Le journal de l'agent prime sur l'historique du hub ; les applications fonctionnent sans hub. | T38, T44 | 2.0 (conception) |
