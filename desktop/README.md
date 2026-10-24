# Patronus pour Windows

Version Windows de l'application : **mêmes fonctions et même style** que l'application Android (thème sombre
« topologie & panneau de détail »).

- 🗺️ Plan du réseau : ce PC, le réseau et chaque PC, reliés selon leur état (allumé, en cours, éteint).
- ⚡ Démarrer les PC (Wake-on-LAN, envois répétés sur chaque carte réseau).
- 🟢 État en temps réel (allumé / éteint / démarrage / arrêt en cours, latence, « vu il y a… »).
- 📈 Latence en direct façon électrocardiogramme dans le panneau de détail (tracé de la dernière minute, une mesure par
  seconde, survol pour lire chaque mesure) et mini-tracés dans les listes.
- 🤝 Partage des PC avec d'autres personnes, PC par PC (démarrer, consulter, ou aussi éteindre), chiffré et signé ;
  invitation par QR code ou lien. Un PC en connexion chiffrée enregistre lui-même l'appareil de la personne, sans
  transmettre de clé. Voir [docs/PARTAGE.md](../docs/PARTAGE.md).
- 🔄 Mises à jour intégrées : nouvelle version proposée avec ses nouveautés, installée en un clic (l'application se
  remplace puis redémarre ; configuration et historique conservés). Voir [docs/MISES-A-JOUR.md](../docs/MISES-A-JOUR.md).
- ⏻ Éteindre, redémarrer, mettre en veille (avec l'[agent](../agent/README.md) installé sur les PC).
- 📦 Agent téléchargeable depuis l'application (*Réglages* → *Télécharger l'agent*) : installé sur cet ordinateur en un
  clic (invite administrateur), ou enregistré pour un autre PC (x64 ou ARM64). La fiche d'un PC signale un agent en
  retard sur la dernière version publiée.
- 🕘 Historique discret des démarrages et extinctions sur 30 jours (complet avec l'agent 1.2 ou plus, même quand
  l'application était fermée).
- 💾 Export / import **compatibles avec le téléphone** : une sauvegarde du téléphone s'importe sur le PC, et inversement.
- 🔒 Configuration chiffrée pour votre compte Windows, aucune donnée envoyée sur Internet.

**Très légère** : un seul fichier `.exe` d'environ 8 Mo, sans installation. L'affichage utilise Microsoft Edge WebView2,
déjà présent dans Windows 11 et installé par les mises à jour de Windows 10.

## Installation

1. Dans les **[Releases](https://github.com/slaynAW/Patronus/releases)**, téléchargez
   `Patronus-Windows-<version>-x64.exe` (ou `-arm64.exe` pour un PC ARM, ex. Snapdragon).
2. Rangez-le où vous voulez (ex. `Documents\Patronus.exe`) et double-cliquez dessus.
3. Au premier lancement, **SmartScreen** peut afficher « Windows a protégé votre ordinateur » (le programme n'est pas
   signé par un éditeur payant) : *Informations complémentaires* → *Exécuter quand même*.
4. Facultatif : clic droit sur le fichier → *Épingler à l'écran de démarrage* / *à la barre des tâches*.

Mise à jour : à partir de la 1.2.0, l'application propose elle-même les nouvelles versions (*Réglages* → *Mises à
jour*). Pour passer d'une version plus ancienne, remplacez simplement le fichier (la configuration est conservée à part).

## Ajouter les PC

| Méthode | Comment |
|---|---|
| **Depuis le téléphone** (le plus simple) | Téléphone : *Réglages* → *Exporter la configuration* → « Complète, protégée par mot de passe ». Transférez le fichier sur le PC, puis *Réglages* → *Importer une configuration* (ou **glissez le fichier dans la fenêtre**). |
| **Lien d'appairage de l'agent** | Sur le PC à contrôler : `wol-agent pair` affiche un lien `wolagent://…`. Copiez-le, puis *Ajouter un PC* → **Coller le lien** : adresse, MAC et port sont remplis. À l'enregistrement, cet ordinateur s'appaire avec le PC en connexion chiffrée, grâce au code à usage unique du lien (valable 10 minutes), envoyé seulement à l'agent dont le lien donne l'empreinte. Le lien d'un agent antérieur à 1.11 remplit la clé, comme avant. |
| **Saisie manuelle** | Nom + adresse MAC suffisent pour démarrer ; ajoutez l'IP pour l'état en temps réel, et la clé de l'agent pour l'extinction. |

## Connexion chiffrée (wolagent/2)

Avec l'agent 1.11.0 ou plus, l'application passe d'elle-même au protocole chiffré
[`wolagent/2`](../docs/PROTOCOLE-V2.md), sans rien rescanner :

- l'agent annonce son **empreinte** dans une réponse signée par sa clé : l'application la vérifie, puis s'enregistre
  auprès de lui avec sa propre identité (rôle « contrôle », les droits qu'avait la clé) ;
- tous les échanges sont ensuite **chiffrés** (TLS 1.3), et l'application vérifie l'empreinte du PC à chaque connexion :
  une autre machine à la même adresse est refusée avant tout envoi. Elle ne revient jamais à l'ancien protocole avec ce PC ;
- la **clé de l'agent est effacée** de l'application, qui n'en a plus besoin. Seule exception : un PC partagé par clé
  (droit « démarrer et éteindre » donné à une personne dont l'application est antérieure à 1.13) la garde tant que ce
  partage la transmet ;
- la fiche du PC affiche *Chiffré et authentifié* et son **empreinte** (`7KQ2-9MXP-4D1Z`), à comparer avec celle de
  `wol-agent status` sur le PC. `wol-agent devices` y liste les appareils autorisés, et retire celui qu'on veut ;
- si cet ordinateur est **administrateur** du PC (appairé avec le QR code de `wol-agent pair`), la ligne *Droits de cet
  ordinateur* de la fiche ouvre *Appareils autorisés* : rôle, nom et retrait de chaque appareil, invitations en attente,
  et *Inviter un appareil* (QR code et lien à usage unique, de 10 minutes à 7 jours). Le dernier administrateur ne peut
  être retiré ou rétrogradé que sur le PC.

L'**identité de cet ordinateur** (clé ECDSA P-256) est créée au premier besoin, chiffrée par DPAPI, et ne quitte jamais
ce PC : elle n'est ni dans les sauvegardes ni dans le partage. Sur un nouveau PC (ou après une réinstallation de
Windows), restaurer une sauvegarde ne suffit donc pas pour les PC déjà passés en `wolagent/2` : la fiche indique
*cet ordinateur n'est pas autorisé par ce PC*. Il suffit de l'appairer à nouveau (`wol-agent pair` sur le PC, puis
coller le lien dans la fiche du PC) : le code du lien sert alors à le réenregistrer.

## Hub sur le NAS (2.0, en cours)

Le [hub](../docs/HUB.md) tourne dans un conteneur sur le NAS : il surveille les PC en continu et garde leur historique
un an. *Réglages* → **Hub sur le NAS** → *Appairer un hub* : collez le lien `patronus://hub?…` affiché dans le journal
du conteneur (ou par `patronus-hub pair`), ou celui d'une invitation créée depuis un autre appareil. Comme pour un PC,
l'application vérifie l'empreinte du hub avant d'envoyer le code, qui ne sert qu'une fois.

- La fiche du hub montre sa version, son adresse, son empreinte (à comparer avec celle du journal du conteneur) et le
  rôle de cet ordinateur : *Consulter*, *Démarrer* ou *Administrer* (pas de *Contrôler* : le hub ne relaie aucune
  commande).
- Un administrateur du hub y gère ses appareils autorisés et les invitations (QR code et lien à usage unique), et choisit
  la durée de conservation de l'historique (3 mois à 3 ans, 1 an par défaut).
- *Oublier ce hub* : un administrateur s'en retire (sauf le dernier) ; sinon, un administrateur du hub le retire.
- L'appairage appartient à cette installation, comme son identité : il n'est ni exporté ni sauvegardé.

**Confier un PC au hub** : dans la fiche du PC, la ligne *Hub* → *Confier au hub*, quand cet ordinateur administre le
PC et le hub, PC allumé. L'agent crée une invitation à usage unique **réservée à l'empreinte du hub**, au rôle
*Démarrer* : le hub s'en sert pour s'appairer lui-même au PC. Il peut ainsi lire son état, son journal et ses mesures,
et le démarrer, mais ni l'éteindre ni gérer ses appareils. La fiche du hub liste les PC confiés et leur état vu par le
hub. *Retirer du hub* efface ce que le hub en gardait et retire le hub des appareils autorisés du PC (s'il est allumé ;
sinon `wol-agent devices revoke <empreinte du hub>` sur le PC).

**Appareils sans agent** (box, imprimante, caméra, le NAS lui-même) : fiche du hub → *Ajouter un appareil*, avec une
vérification par connexion à un port ou par page web (HTTP, HTTPS). Le hub vérifie seulement qu'il répond.

**Réveil par le hub** : pour un PC confié dont l'adresse n'est sur aucun réseau de cet ordinateur (VPN, autre réseau),
*Démarrer* demande aussi au hub d'envoyer le paquet magique depuis le réseau du NAS (rôle *Démarrer* sur le hub).

Les évènements « Pendant votre absence » et l'historique au-delà de 30 jours viennent à l'étape suivante.

## Où sont mes données ?

| Élément | Emplacement |
|---|---|
| Configuration (PC, clés des agents, réglages) | `%APPDATA%\WakeOnLan\config.dat`, chiffrée avec **DPAPI** (liée à votre session Windows : illisible depuis un autre compte ou un autre PC). |
| Historique (30 jours) | `%APPDATA%\WakeOnLan\history.dat`, chiffré avec DPAPI. *Réglages* → *Effacer l'historique* le vide (le journal tenu par l'agent de chaque PC est conservé). |
| Partage (clés, jeton GitHub, accès reçus) | `%APPDATA%\WakeOnLan\share.dat`, chiffré avec DPAPI. |
| Identité `wolagent/2` de cet ordinateur | `%APPDATA%\WakeOnLan\identity.dat`, chiffrée avec DPAPI ; jamais exportée ni sauvegardée. |
| Hub appairé (nom, adresse, empreinte, rôle) | `%APPDATA%\WakeOnLan\hub.dat`, chiffré avec DPAPI ; jamais exporté ni sauvegardé. |
| Sauvegarde automatique (mot de passe, jeton GitHub, dossier) | `%APPDATA%\WakeOnLan\backup.dat`, chiffré avec DPAPI ; les sauvegardes elles-mêmes sont dans le Gist secret et/ou le dossier choisis ([docs/SAUVEGARDE.md](../docs/SAUVEGARDE.md)). |
| Journal de diagnostic | `%APPDATA%\WakeOnLan\diagnostics\`, chiffré (clé protégée par DPAPI). *Réglages* → *Diagnostic* exporte un rapport chiffré par mot de passe ([docs/DIAGNOSTIC.md](../docs/DIAGNOSTIC.md)). |
| Cache d'affichage WebView2 | `%LOCALAPPDATA%\WakeOnLan\WebView2` |

Pour changer de PC, utilisez l'**export chiffré** (le fichier `config.dat` n'est pas transférable, par sécurité).
Désinstaller : supprimez le `.exe` et les deux dossiers ci-dessus.

## Différences avec le téléphone

| Téléphone | Windows |
|---|---|
| Scanner le QR code de l'agent | Coller le lien d'appairage (ou importer la configuration du téléphone) |
| Surveillance arrêtée quand l'application est en arrière-plan | Surveillance arrêtée quand la fenêtre est **réduite** |
| Configuration chiffrée par le Keystore Android | Configuration chiffrée par DPAPI (Windows) |
| Autorisation « réseau local » d'Android 17 | Aucune autorisation nécessaire |

Raccourcis : **F5** actualise l'état, **Échap** ferme un menu, un panneau ou un dialogue. Une seule fenêtre à la fois :
relancer le programme ramène la fenêtre existante au premier plan. Fenêtre étroite : le panneau de détail passe
par-dessus le plan du réseau.

## Fonctionnement

```
desktop/
├── main.go, main_windows.go   fenêtre WebView2, pont JavaScript ↔ Go, autotest de démarrage
├── win32_windows.go           boîte « Enregistrer sous », presse-papiers, instance unique, DPI
├── main_other.go              mode développement (hors Windows) : interface servie sur 127.0.0.1
├── ui/                        interface (HTML/CSS/JS sans dépendance) et polices, intégrées au .exe
├── winres/                    icône, manifeste (DPI, contrôles modernes), informations de version
└── internal/
    ├── model/        PC, réglages, validation (mêmes règles et messages qu'Android)
    ├── config/       format JSON, export/import chiffré (PBKDF2 + AES-256-GCM), stockage DPAPI
    ├── wol/          paquet magique, diffusion par carte réseau
    ├── agentclient/  client des agents : wolagent/2 (TLS 1.3, empreinte épinglée) ou wolagent/1 ; client du hub
                      (patronus-hub/1, même transport)
    ├── hubstore/     hub appairé (DPAPI)
    ├── identity/     identité wolagent/2 de cet ordinateur (ECDSA P-256, DPAPI)
    ├── history/      historique sur 30 jours (mêmes règles que le téléphone), stockage DPAPI
    ├── status/       sondes (agent, TCP, ping ICMP), machine à états, surveillance
    ├── netstate/     cartes Ethernet / Wi-Fi / VPN (GetAdaptersAddresses)
    ├── pairing/      lien d'appairage
    └── app/          actions de l'interface (équivalent des ViewModels Android)
```

Particularités Windows prises en compte :

- **Réveil sur chaque carte réseau** : Windows n'émet la diffusion `255.255.255.255` que sur une seule carte ; l'application
  ouvre une socket par carte (Ethernet, Wi-Fi) et envoie diffusion dirigée + diffusion limitée depuis chacune.
- **Connexion refusée détectée immédiatement** : Windows réessaie normalement ~2 s après un refus (RST), ce qui ferait passer un
  PC allumé pour éteint ; l'option `SIO_TCP_INITIAL_RTO` supprime ces nouvelles tentatives pour les sondes.
- **Ping sans droits administrateur** via `IcmpSendEcho`.
- Cartes des machines virtuelles (Hyper-V, WSL, VirtualBox, VMware) ignorées ; VPN détecté.

## Compilation

```bash
cd desktop
go test ./...
# Ressources (icône, manifeste) puis exécutable Windows sans console :
go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64,arm64
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w -X main.version=1.2.0" -o Patronus.exe .
```

Mode développement (Linux / macOS) : `go run .` affiche une adresse `http://127.0.0.1:…/#token=…`
à ouvrir dans un navigateur (interface et moteur réels, boîtes de dialogue remplacées par le navigateur).

Polices : [Inter](https://github.com/rsms/inter) et [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono),
sous licence SIL Open Font License 1.1 (textes dans `ui/fonts/`).

La CI compile les versions x64 et ARM64, exécute les tests sous Linux **et** Windows (DPAPI, ping, cartes réseau), le test
de bout en bout contre le vrai agent, puis **lance réellement l'application** sur Windows (autotest `WOL_SELFTEST`).
