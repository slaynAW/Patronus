# Sécurité

## Principes

1. **Rien ne sort du réseau local.** Pas de serveur, pas de compte, pas de télémétrie, pas de publicité. L'application ne contacte
   que les adresses de vos PC (et Google Play Services, uniquement quand vous ouvrez le lecteur de QR code).
2. **Moindre privilège.** L'application ne demande que : Internet (sockets), état du réseau, et l'autorisation « réseau local »
   d'Android 17. Pas de caméra (le QR code est lu par Play Services), pas de localisation, pas de stockage.
3. **Défense en profondeur** côté agent : filtrage réseau + authentification forte + limitation des tentatives + pare-feu.

## Sur le téléphone

| Mesure | Détail |
|---|---|
| Chiffrement au repos | Toute la configuration (PC, clés d'agent) est chiffrée en AES-256-GCM avec une clé générée dans le **Keystore Android** (TEE / Titan M2 sur Pixel). La clé n'est pas extractible, même par root. |
| Pas de sauvegarde cloud | `allowBackup=false` et règles d'extraction excluant tout : les secrets ne partent pas dans Google Drive. L'export chiffré sert de sauvegarde. |
| Captures d'écran | Bloquées pendant l'affichage en clair d'une clé d'agent (`FLAG_SECURE`). |
| Journaux | Les objets contenant des secrets masquent ceux-ci dans `toString()` (testé). |
| Réseau | Trafic HTTP en clair interdit (`network_security_config`) ; sockets attachées au Wi-Fi/Ethernet. |
| Import | Tout fichier importé est validé champ par champ (MAC, IP, ports, tailles) ; taille maximale 1 Mio. |
| Code | R8 (minification) activé ; dépendances limitées à AndroidX, Kotlin et Play Services code scanner. |

## Export / import

- **Complet** : chiffré avec une clé dérivée du mot de passe par **PBKDF2-HMAC-SHA256 (600 000 itérations, sel de 16 octets)**,
  puis **AES-256-GCM** (IV de 12 octets, en-tête authentifié). Un mauvais mot de passe ou une modification du fichier sont détectés.
- **Sans les clés** : fichier lisible, clés d'agent et mots de passe SecureOn retirés.

## Agent

| Mesure | Détail |
|---|---|
| Authentification | HMAC-SHA256 avec une clé aléatoire de 256 bits par PC ; défi-réponse (nonces) → pas de rejeu, pas de clé sur le réseau. |
| Authentification mutuelle | Les réponses sont signées : un faux agent ne peut pas faire croire qu'une extinction a réussi. |
| Anti force brute | 5 échecs par minute et par adresse IP → blocage 5 minutes. |
| Filtrage | Seules les adresses privées sont acceptées (`10/8`, `172.16/12`, `192.168/16`, `169.254/16`, `100.64/10`, IPv6 locales), modifiable dans `config.json` (`allow`). |
| Pare-feu Windows | Règle limitée à l'exécutable de l'agent, au port 9770, au **sous-réseau local**, profils privé/domaine uniquement. |
| Robustesse | Messages de 8 Kio maximum, délai de 10 s, 16 connexions simultanées maximum. |
| Commandes | Liste blanche configurable (`commands` dans `config.json`), par ex. pour n'autoriser que la veille. |
| Fichier de configuration | Lisible uniquement par root (Linux/macOS, `0600`) ou SYSTEM/Administrateurs (Windows, ACL posées via les SID). |
| Linux | Service systemd durci (`NoNewPrivileges`, `ProtectSystem`, `ProtectHome`, `PrivateTmp`…). |
| Chaîne de production | Binaires compilés par la CI (`-trimpath`, sans CGO), sommes SHA-256 publiées ; somme du wrapper Gradle vérifiée. |

## Limites connues (et parades)

- **Le paquet magique n'est pas authentifié** (norme Wake-on-LAN) : n'importe qui sur votre réseau local peut allumer vos PC.
  C'est sans risque en soi (il ne fait qu'allumer). Le mot de passe SecureOn n'offre qu'une protection symbolique (transmis en clair).
- **Le trafic avec l'agent n'est pas chiffré**, seulement authentifié : quelqu'un sur votre réseau peut voir qu'un PC s'appelle
  « PC-BUREAU » et depuis quand il est allumé, mais ne peut ni lire la clé ni forger de commande.
- **Le QR code d'appairage contient la clé** : ne le partagez pas, ne le photographiez pas avec un autre appareil. En cas de doute :
  `wol-agent rotate-key`.
- **APK hors Play Store** : vérifiez que vous téléchargez l'APK depuis les Releases de votre dépôt. Configurez la clé de signature
  ([SIGNATURE.md](SIGNATURE.md)) pour qu'Android refuse toute « mise à jour » signée par quelqu'un d'autre.
- **Accès à distance** : n'ouvrez **jamais** le port de l'agent sur Internet (redirection de port). Utilisez un VPN (voir
  [AMELIORATIONS.md](AMELIORATIONS.md)).
