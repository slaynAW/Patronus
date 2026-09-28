# Wake On LAN pour Windows

Version Windows de l'application : **mêmes fonctions et même style** que l'application Android (thème sombre
« topologie & panneau de détail »).

- 🗺️ Plan du réseau : ce PC, le réseau et chaque PC, reliés selon leur état (allumé, en cours, éteint).
- ⚡ Démarrer les PC (Wake-on-LAN, envois répétés sur chaque carte réseau).
- 🟢 État en temps réel (allumé / éteint / démarrage / arrêt en cours, latence, « vu il y a… »).
- 📈 Latence en direct façon électrocardiogramme dans le panneau de détail (tracé de la dernière minute, une mesure par
  seconde, survol pour lire chaque mesure) et mini-tracés dans les listes.
- 🔄 Mises à jour intégrées : nouvelle version proposée avec ses nouveautés, installée en un clic (l'application se
  remplace puis redémarre ; configuration et historique conservés). Voir [docs/MISES-A-JOUR.md](../docs/MISES-A-JOUR.md).
- ⏻ Éteindre, redémarrer, mettre en veille (avec l'[agent](../agent/README.md) installé sur les PC).
- 🕘 Historique discret des démarrages et extinctions sur 30 jours (complet avec l'agent 1.2 ou plus, même quand
  l'application était fermée).
- 💾 Export / import **compatibles avec le téléphone** : une sauvegarde du téléphone s'importe sur le PC, et inversement.
- 🔒 Configuration chiffrée pour votre compte Windows, aucune donnée envoyée sur Internet.

**Très légère** : un seul fichier `.exe` d'environ 8 Mo, sans installation. L'affichage utilise Microsoft Edge WebView2,
déjà présent dans Windows 11 et installé par les mises à jour de Windows 10.

## Installation

1. Dans les **[Releases](https://github.com/slaynAW/WakeOnLan/releases)**, téléchargez
   `WakeOnLan-Windows-<version>-x64.exe` (ou `-arm64.exe` pour un PC ARM, ex. Snapdragon).
2. Rangez-le où vous voulez (ex. `Documents\WakeOnLan.exe`) et double-cliquez dessus.
3. Au premier lancement, **SmartScreen** peut afficher « Windows a protégé votre ordinateur » (le programme n'est pas
   signé par un éditeur payant) : *Informations complémentaires* → *Exécuter quand même*.
4. Facultatif : clic droit sur le fichier → *Épingler à l'écran de démarrage* / *à la barre des tâches*.

Mise à jour : à partir de la 1.2.0, l'application propose elle-même les nouvelles versions (*Réglages* → *Mises à
jour*). Pour passer d'une version plus ancienne, remplacez simplement le fichier (la configuration est conservée à part).

## Ajouter les PC

| Méthode | Comment |
|---|---|
| **Depuis le téléphone** (le plus simple) | Téléphone : *Réglages* → *Exporter la configuration* → « Complète, protégée par mot de passe ». Transférez le fichier sur le PC, puis *Réglages* → *Importer une configuration* (ou **glissez le fichier dans la fenêtre**). |
| **Lien d'appairage de l'agent** | Sur le PC à contrôler : `wol-agent pair` affiche un lien `wolagent://…`. Copiez-le, puis *Ajouter un PC* → **Coller le lien** : adresse, MAC, port et clé sont remplis. |
| **Saisie manuelle** | Nom + adresse MAC suffisent pour démarrer ; ajoutez l'IP pour l'état en temps réel, et la clé de l'agent pour l'extinction. |

## Où sont mes données ?

| Élément | Emplacement |
|---|---|
| Configuration (PC, clés des agents, réglages) | `%APPDATA%\WakeOnLan\config.dat`, chiffrée avec **DPAPI** (liée à votre session Windows : illisible depuis un autre compte ou un autre PC). |
| Historique (30 jours) | `%APPDATA%\WakeOnLan\history.dat`, chiffré avec DPAPI. *Réglages* → *Effacer l'historique* le vide (le journal tenu par l'agent de chaque PC est conservé). |
| Journal technique | `%APPDATA%\WakeOnLan\wakeonlan.log` |
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
    ├── agentclient/  client du protocole wolagent/1 (réutilise agent/protocol)
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
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w -X main.version=1.2.0" -o WakeOnLan.exe .
```

Mode développement (Linux / macOS) : `go run .` affiche une adresse `http://127.0.0.1:…/#token=…`
à ouvrir dans un navigateur (interface et moteur réels, boîtes de dialogue remplacées par le navigateur).

Polices : [Inter](https://github.com/rsms/inter) et [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono),
sous licence SIL Open Font License 1.1 (textes dans `ui/fonts/`).

La CI compile les versions x64 et ARM64, exécute les tests sous Linux **et** Windows (DPAPI, ping, cartes réseau), le test
de bout en bout contre le vrai agent, puis **lance réellement l'application** sur Windows (autotest `WOL_SELFTEST`).
