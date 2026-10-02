# Sécurité

## Principes

1. **Rien de personnel ne sort du réseau local en clair.** Pas de serveur, pas de télémétrie, pas de publicité. L'application
   ne contacte que les adresses de vos PC, Google Play Services (uniquement quand vous ouvrez le lecteur de QR code) et GitHub en
   HTTPS pour la **recherche de mises à jour** (désactivable ; aucune donnée transmise, voir
   [MISES-A-JOUR.md](MISES-A-JOUR.md)) et, **si vous l'utilisez**, pour le **partage des PC** : uniquement des fichiers
   chiffrés pour un seul appareil et signés (voir [PARTAGE.md](PARTAGE.md)).
2. **Moindre privilège.** L'application ne demande que : Internet (sockets), état du réseau, l'autorisation « réseau local »
   d'Android 17, et l'installation de sa propre mise à jour (toujours confirmée par l'utilisateur). Pas de caméra (le QR code est lu par Play Services), pas de localisation, pas de stockage.
3. **Défense en profondeur** côté agent : filtrage réseau + authentification forte + limitation des tentatives + pare-feu.

## Sur le téléphone

| Mesure | Détail |
|---|---|
| Chiffrement au repos | Toute la configuration (PC, clés d'agent) est chiffrée en AES-256-GCM avec une clé générée dans le **Keystore Android** (TEE / Titan M2 sur Pixel). La clé n'est pas extractible, même par root. L'historique (30 jours) est chiffré de la même façon, avec une clé distincte. |
| Pas de sauvegarde cloud | `allowBackup=false` et règles d'extraction excluant tout : les secrets ne partent pas dans Google Drive. L'export chiffré sert de sauvegarde. |
| Captures d'écran | Bloquées pendant l'affichage en clair d'une clé d'agent (`FLAG_SECURE`). |
| Journaux | Les objets contenant des secrets masquent ceux-ci dans `toString()` (testé). Journal de diagnostic chiffré (AES-256-GCM, clé de données protégée par le Keystore), sans aucun secret ; il ne quitte le téléphone que dans un rapport exporté par l'utilisateur et chiffré par son mot de passe ([DIAGNOSTIC.md](DIAGNOSTIC.md)). |
| Données illisibles | Configuration, partage ou historique illisibles (clé Keystore perdue, fichier abîmé) : fichier mis de côté, jamais écrasé, et incident signalé à l'utilisateur. |
| Réseau | Trafic HTTP en clair interdit (`network_security_config`) ; sockets attachées au Wi-Fi/Ethernet. |
| Import | Tout fichier importé est validé champ par champ (MAC, IP, ports, tailles) ; taille maximale 1 Mio. |
| Partage | Clés de partage générées sur le téléphone et chiffrées avec l'état du partage par le Keystore ; jeton GitHub limité aux Gists, jamais exporté ; PC reçus validés comme un import, non modifiables ni exportables, clé d'agent jamais affichée. |
| Mises à jour | Manifeste signé vérifié avec le certificat de l'application installée, APK vérifié (taille + SHA-256, paquet et version) puis installé par l'installateur d'Android, qui exige la même clé de signature. Permission « installer des applications » utilisée uniquement pour cela. |
| Code | R8 (minification) activé ; dépendances limitées à AndroidX, Kotlin et Play Services code scanner. |

## Sur le PC (application Windows)

| Mesure | Détail |
|---|---|
| Chiffrement au repos | Configuration chiffrée par **DPAPI** (`CryptProtectData`, liée à la session Windows + entropie propre à l'application) : un fichier `config.dat` copié ailleurs est illisible. L'historique (`history.dat`) est chiffré de la même façon, avec une entropie distincte. |
| Réseau | Vos PC, et GitHub (HTTPS) pour la recherche de mises à jour (désactivable) et le partage (si utilisé). Aucun port en écoute. |
| Partage | Clés de partage et jeton GitHub (droit « gist » seulement) chiffrés par DPAPI (`share.dat`) ; PC reçus validés comme un import, non modifiables ni exportables. |
| Mises à jour | Manifeste signé avec la clé de signature de l'APK (certificat intégré à l'exécutable), fichier vérifié (taille + SHA-256) avant de remplacer l'exécutable ; rien n'est installé sans clic. |
| Téléchargement de l'agent | Même vérification que les mises à jour (manifeste signé, taille, SHA-256), uniquement à la demande. Pour l'installer sur cet ordinateur, le fichier (dans `%LOCALAPPDATA%\WakeOnLan\Agent`, vidé au démarrage) est verrouillé contre toute modification, son empreinte revérifiée sur le fichier verrouillé, puis l'installation est lancée avec l'invite administrateur ; le verrou tient jusqu'à la fin de l'installation, puis le fichier est effacé. |
| Interface | Page intégrée à l'exécutable, sans contenu distant ; seules les adresses du dépôt peuvent être ouvertes dans le navigateur (liste blanche). Le presse-papiers n'est lu que pour un lien `wolagent://`. |
| Clés | Jamais envoyées à la liste des PC affichée ; seulement au formulaire de modification. |
| Journal de diagnostic | `diagnostics\` : chaque évènement chiffré en AES-256-GCM, clé de données protégée par DPAPI ; aucun secret (paramètres des actions filtrés : ni mot de passe, ni clé, ni texte collé). Rapport exporté uniquement à la demande, chiffré par mot de passe ([DIAGNOSTIC.md](DIAGNOSTIC.md)). Un arrêt brutal du moteur est d'abord écrit en clair par Go dans `diagnostics\plantage.txt` (traces d'exécution), puis versé dans le journal chiffré au lancement suivant. |
| Exécutable | Compilé par la CI (`-trimpath`, sans CGO), somme SHA-256 publiée. Non signé par un éditeur (avertissement SmartScreen au premier lancement). |

## Export / import

- **Complet** : chiffré avec une clé dérivée du mot de passe par **PBKDF2-HMAC-SHA256 (600 000 itérations, sel de 16 octets)**,
  puis **AES-256-GCM** (IV de 12 octets, en-tête authentifié). Un mauvais mot de passe ou une modification du fichier sont détectés.
- **Complet** : contient aussi la clé de partage et l'historique (5 000 derniers évènements), uniquement dans ce
  fichier chiffré.
- **Sans les clés** : fichier lisible, clés d'agent et mots de passe SecureOn retirés.
- Format identique sur Android et Windows (vérifié par des sauvegardes de référence produites indépendamment).
- **Sauvegarde automatique** : même chiffrement que l'export complet, par un mot de passe propre aux sauvegardes,
  gardé chiffré sur l'appareil (Keystore / DPAPI) ; fichiers dans un Gist **secret** du compte GitHub (droit « gist »
  seulement) et/ou un dossier choisi ; jamais de sauvegarde automatique vide, pause après des données illisibles
  ([SAUVEGARDE.md](SAUVEGARDE.md)).
- **Rapport de diagnostic** : même chiffrement (PBKDF2-HMAC-SHA256 600 000 itérations + AES-256-GCM), format
  `patronus-diagnostic/1` commun aux applications et à l'agent ; jamais de clé, de jeton ni de mot de passe dedans
  ([DIAGNOSTIC.md](DIAGNOSTIC.md)).

## Agent

| Mesure | Détail |
|---|---|
| Authentification | HMAC-SHA256 avec une clé aléatoire de 256 bits par PC ; défi-réponse (nonces) → pas de rejeu, pas de clé sur le réseau. |
| Authentification mutuelle | Les réponses sont signées : un faux agent ne peut pas faire croire qu'une extinction a réussi. |
| Anti force brute | 5 échecs par minute et par adresse IP → blocage 5 minutes. |
| Filtrage | Seules les adresses privées sont acceptées (`10/8`, `172.16/12`, `192.168/16`, `169.254/16`, `100.64/10`, IPv6 locales), modifiable dans `config.json` (`allow`). |
| Pare-feu Windows | Règle limitée à l'exécutable de l'agent, au port 9770, au **sous-réseau local**, profils privé/domaine uniquement. Sur un réseau classé « Public », l'agent reste bloqué : le service le détecte et propose à l'utilisateur connecté (fenêtre « Oui / Non ») de classer ce réseau en Privé ; rien n'est changé sans son accord, et un refus n'est reposé qu'une semaine plus tard. |
| Robustesse | Messages de 8 Kio maximum (réponse `history` : 512 Kio au plus côté client), délai de 10 s, 16 connexions simultanées maximum. |
| Commandes | Liste blanche configurable (`commands` dans `config.json`), par ex. pour n'autoriser que la veille. Le journal (`history`, lecture seule) et l'ajout des démarrages demandés (`wakes`) sont accessibles dès que `status` l'est. |
| Journal du PC | `history.json` et `alive.json` rangés avec la configuration, dans un dossier réservé à root / SYSTEM et aux administrateurs. Le journal ne contient que des heures, des types d'évènement et, pour chaque demande, l'adresse IP et le nom de l'appareil qui l'a faite (nom indiqué par l'application : « Pixel 8 », nom du PC) ; il est borné (30 jours, 2 000 évènements, noms de 40 caractères). Toute personne qui a la clé de l'agent (vous, et les personnes à qui vous avez partagé le PC avec le droit « démarrer et éteindre ») peut le lire. Les démarrages signalés ne sont acceptés qu'authentifiés, et seulement pour la période couverte par le journal. |
| Températures | Lecture seule des capteurs, jointe à `status` (donc réservée aux détenteurs de la clé). L'agent n'installe aucun pilote : les cartes graphiques sont lues par l'interface du noyau graphique de Windows (comme le Gestionnaire des tâches) et, pour NVIDIA, par la bibliothèque de son pilote (`nvml.dll`, dossier système) ; le processeur par LibreHardwareMonitor s'il tourne (WMI local, ou son serveur web : `127.0.0.1`, ou l'adresse choisie dans LHM si c'est l'une de celles du PC ; jamais d'autre machine, ni proxy ni redirection), ou par les capteurs du noyau sous Linux. Le serveur web de LibreHardwareMonitor n'a pas de mot de passe par défaut et permet de piloter les ventilateurs : ne l'ouvrez pas dans le pare-feu, l'agent le lit en local. Relevés limités à un toutes les 5 s. |
| Fichier de configuration | Lisible uniquement par root (Linux/macOS, `0600`) ou SYSTEM/Administrateurs (Windows, ACL posées via les SID). |
| Linux | Service systemd durci (`NoNewPrivileges`, `ProtectSystem`, `ProtectHome`, `PrivateTmp`…). |
| Chaîne de production | Binaires compilés par la CI (`-trimpath`, sans CGO), sommes SHA-256 publiées ; somme du wrapper Gradle vérifiée. |

## Limites connues (et parades)

- **Le paquet magique n'est pas authentifié** (norme Wake-on-LAN) : n'importe qui sur votre réseau local peut allumer vos PC.
  C'est sans risque en soi (il ne fait qu'allumer). Le mot de passe SecureOn n'offre qu'une protection symbolique (transmis en clair).
- **Le trafic avec l'agent n'est pas chiffré**, seulement authentifié : quelqu'un sur votre réseau peut voir qu'un PC s'appelle
  « PC-BUREAU », depuis quand il est allumé et, lors d'une lecture du journal, ses heures de démarrage et d'arrêt ; il ne peut
  ni lire la clé ni forger de commande.
- **Le QR code d'appairage contient la clé** : ne le partagez pas, ne le photographiez pas avec un autre appareil. En cas de doute :
  `wol-agent rotate-key`.
- **APK hors Play Store** : vérifiez que vous téléchargez l'APK depuis les Releases de votre dépôt. Configurez la clé de signature
  ([SIGNATURE.md](SIGNATURE.md)) pour qu'Android refuse toute « mise à jour » signée par quelqu'un d'autre.
- **Accès à distance** : n'ouvrez **jamais** le port de l'agent sur Internet (redirection de port). Utilisez un VPN (voir
  [AMELIORATIONS.md](AMELIORATIONS.md)).
