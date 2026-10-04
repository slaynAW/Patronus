# wol-agent — agent PC

Petit programme (un seul fichier, sans dépendance) installé sur chaque PC pour que les applications (Android et Windows)
puissent **l'éteindre, le redémarrer ou le mettre en veille**, connaître son état de façon fiable et afficher
l'**historique de ses démarrages et extinctions** sur 30 jours, y compris quand l'application était fermée.

Téléchargement : section **Releases** du dépôt (`wol-agent-<système>-<architecture>`).

| Système | Fichier |
|---|---|
| Windows 10/11 (Intel/AMD) | `wol-agent-windows-amd64.exe` |
| Windows sur ARM | `wol-agent-windows-arm64.exe` |
| Linux x86-64 | `wol-agent-linux-amd64` |
| Linux ARM 64 bits (Raspberry Pi 4/5…) | `wol-agent-linux-arm64` |
| Linux ARM 32 bits | `wol-agent-linux-arm` |
| macOS Apple Silicon / Intel | `wol-agent-darwin-arm64` / `wol-agent-darwin-amd64` |

## Installation

**Windows** : double-cliquez sur le `.exe`, acceptez l'invite administrateur. Ou, dans un terminal administrateur :
```powershell
.\wol-agent-windows-amd64.exe install
```
- copie dans `C:\Program Files\WolAgent\`, configuration dans `C:\ProgramData\WolAgent\config.json` ;
- service **WolAgent** (démarrage automatique, relance en cas d'erreur), journal dans `C:\ProgramData\WolAgent\agent.log` (`wol-agent diagnostic` en fait un rapport chiffré par mot de passe, voir [docs/DIAGNOSTIC.md](../docs/DIAGNOSTIC.md)) ;
- règle de pare-feu « Patronus - Agent » : port 9770, **sous-réseau local**, profils privé et domaine. Si le réseau
  du PC est classé « **Public** » par Windows, l'agent reste bloqué (les applications voient le PC éteint) :
  l'installation le signale et propose de le classer en Privé, et le service le propose ensuite à l'utilisateur
  connecté (depuis l'agent 1.6.0). À la main : *Paramètres* → *Réseau et Internet* → ce réseau → *Type de profil
  réseau* : Privé.

**Linux** (systemd) / **macOS** (launchd) :
```bash
chmod +x wol-agent-linux-amd64
sudo ./wol-agent-linux-amd64 install
```
- copie dans `/usr/local/bin/wol-agent`, configuration dans `/etc/wol-agent/config.json` (macOS : `/Library/Application Support/WolAgent/`) ;
- Linux : service `wol-agent.service` durci ; ouverture du port si `firewalld` ou `ufw` est actif. Journal : `journalctl -u wol-agent`.

À la fin, un **QR code** s'affiche : dans l'application, *Ajouter un PC* → *Scanner le QR code*.

> Réinstaller (ou installer une nouvelle version) conserve la configuration et la clé : pas besoin de ré-appairer.

## Mises à jour (Windows)

À partir de la version 1.4.0, l'agent se tient à jour lui-même, **avec l'accord de l'utilisateur du PC** :

1. Une fois par jour (et quelques minutes après le démarrage), le service regarde si une nouvelle version de l'agent
   est publiée.
2. Si oui, une fenêtre s'affiche sur la session ouverte : « Une nouvelle version de l'agent Patronus est
   disponible : 1.4.0 → 1.5.0 », avec ses nouveautés. **Oui** l'installe ; **Non** (ou pas de réponse) la repropose le
   lendemain. Si personne n'est connecté, la question attend l'ouverture d'une session.
3. L'agent télécharge la nouvelle version, la vérifie, la met à la place de l'ancienne puis redémarre en quelques
   secondes ; une fenêtre confirme la mise à jour. S'il ne répond pas après le redémarrage, l'ancienne version est
   remise en place automatiquement.

La clé, l'appairage, la configuration et le journal sont conservés. Sécurité : l'agent n'accepte que les versions
décrites par le manifeste **signé** des versions officielles (même clé et mêmes contrôles que les applications, voir
[docs/MISES-A-JOUR.md](../docs/MISES-A-JOUR.md)), et le fichier doit avoir exactement l'empreinte SHA-256 annoncée.
Il ne contacte que GitHub, en HTTPS, et n'envoie aucune donnée.

- `wol-agent update` : recherche et installe tout de suite (`--check` : vérifier seulement).
- `wol-agent update --auto off` : désactive la recherche quotidienne (`--auto on` pour la rétablir).
- Agent 1.3 ou plus ancien : installez une fois la 1.4.0 à la main (double-clic sur le `.exe`), les suivantes seront
  proposées. Linux et macOS : relancez `sudo wol-agent install` avec la nouvelle version.

L'agent a son propre numéro de version, qui ne change que lorsqu'il évolue (voir [NOUVEAUTES.md](NOUVEAUTES.md)) :
une nouvelle version des applications ne provoque pas de mise à jour de l'agent si celui-ci n'a pas changé.

## Commandes

| Commande | Rôle |
|---|---|
| `wol-agent install [--port 9770] [--name "PC Bureau"] [--ip 192.168.1.20] [--no-firewall] [--firewall-public]` | Installe / met à jour le service |
| `wol-agent pair [--ip …] [--png qr.png] [--invert]` | Réaffiche le QR code (ou l'enregistre en PNG) et le lien d'appairage |
| `wol-agent status` | État du service, configuration, carte réseau détectée, type des réseaux Windows (alerte si « Public »), températures, disques (espace, santé, erreurs), derniers évènements du journal (avec la cause des arrêts anormaux) |
| `wol-agent diagnostic` | Rapport de diagnostic chiffré par un mot de passe (état, configuration sans la clé, journaux), à transmettre pour analyser un problème ([docs/DIAGNOSTIC.md](../docs/DIAGNOSTIC.md)). Terminal administrateur recommandé. |
| `wol-agent rotate-key` | Nouvelle clé ; l'ancienne est immédiatement refusée (ré-appairer) |
| `wol-agent update [--check] [--yes] [--auto on\|off]` | Recherche et installe une nouvelle version (Windows) ; `--auto` : recherche quotidienne |
| `wol-agent uninstall [--purge]` | Désinstalle (`--purge` supprime aussi la configuration) |
| `wol-agent run [--config …] [--dry-run]` | Lance l'agent au premier plan ; `--dry-run` n'éteint rien (test) |

`--ip` sert si le PC a plusieurs cartes réseau : indiquez l'IP de la carte **Ethernet** (celle qui gère le Wake-on-LAN),
afin que le QR code contienne la bonne adresse MAC.

## Configuration (`config.json`)

```json
{
  "name": "PC-BUREAU",
  "port": 9770,
  "key": "…43 caractères…",
  "allow": ["127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10", "::1/128", "fc00::/7", "fe80::/10"],
  "commands": ["status", "shutdown", "reboot", "sleep"]
}
```

- `autoUpdate` (facultatif, Windows) : `false` désactive la recherche quotidienne des mises à jour.
- `metrics` (facultatif) : `false` désactive l'enregistrement continu des mesures (90 jours, dossier `metrics/`).

- `allow` : réseaux autorisés à se connecter (par défaut uniquement des adresses privées).
- `commands` : retirez par exemple `"shutdown"` pour n'autoriser que la veille. La lecture du journal (`history`) est
  autorisée dès que `status` l'est.
- Après modification : redémarrez le service (`wol-agent install` le fait, ou `systemctl restart wol-agent`,
  ou *Services* → *Patronus - Agent* → *Redémarrer* sous Windows).

## Comportement des actions

| Action | Windows | Linux | macOS |
|---|---|---|---|
| Éteindre | `shutdown /s` (arrêt **complet**, sans démarrage rapide) | `systemctl poweroff` | `shutdown -h now` |
| Redémarrer | `shutdown /r` | `systemctl reboot` | `shutdown -r now` |
| Veille | `SetSuspendState` (veille S3 / moderne) | `systemctl suspend` | `pmset sleepnow` |
| Forcer | `/f` (ferme les applications) | `--ignore-inhibitors` | — |

L'agent répond au téléphone **avant** d'exécuter l'action (délai minimal 1,5 s).

## Journal des démarrages et extinctions

L'agent note dans `history.json` (à côté de `config.json`) les **30 derniers jours** : démarrages, arrêts, mises en veille
et sorties de veille, commandes reçues et démarrages demandés depuis les applications (avec le nom et l'adresse de
l'appareil à l'origine de la demande ; agent 1.4.0 ou plus). Toutes les applications relisent ce même journal : le
téléphone et le PC Windows affichent le même historique. Un arrêt qu'il n'a pas pu noter
(coupure de courant, arrêt forcé, plantage) apparaît comme « arrêt inattendu », daté du dernier signe de vie (`alive.json`,
mis à jour chaque minute). Les applications relisent ce journal dès que le PC répond.

> Mettre à jour l'agent (version 1.2 ou plus) suffit pour en profiter : relancez simplement `install` avec la nouvelle
> version, la configuration et la clé sont conservées. Avec un agent plus ancien, les applications n'affichent que les
> changements qu'elles constatent elles-mêmes, pendant qu'elles sont ouvertes.

## Températures et utilisation

Depuis la version 1.5.0, la réponse à `status` contient la température du **processeur** et de la **carte graphique**
(la plus chaude s'il y en a plusieurs), et depuis la version 1.7.0 leur **utilisation en %**, affichées par les
applications. Les capteurs ne sont lus que lorsqu'une application le demande, au plus toutes les 5 secondes, en
arrière-plan (la réponse n'attend jamais les capteurs).

**Utilisation** (agent 1.7.0), mesurée sur une seconde à chaque relevé :

| Système | Processeur | Carte graphique |
|---|---|---|
| Windows | Compteur de performance `Processor Information(_Total)\% Processor Utility`, **celui du Gestionnaire des tâches** (travail réellement fourni, selon la fréquence ; plafonné à 100 %), à défaut le temps d'activité (`GetSystemTimes`) | Compteurs `GPU Engine(*)\Utilization Percentage` (Windows 10 1709 ou plus), **calcul du Gestionnaire des tâches** : chaque moteur (3D, copie, vidéo…) additionné sur tous les programmes, puis moteur le plus occupé de la carte. Toutes marques, puces intégrées comprises. C'est la carte dont la température est affichée (sinon la plus occupée) |
| Linux | `/proc/stat` | `nvidia-smi` (NVIDIA), `gpu_busy_percent` (AMD, pilote `amdgpu`) |
| macOS | — | — |

Les compteurs sont lus par leur nom anglais : la mesure fonctionne quelle que soit la langue de Windows.

**Enregistrement continu** (agent 1.8.0) : un relevé toutes les 10 s (5 s quand une application regarde le PC),
résumé chaque minute (moyenne et maximum de chaque mesure), gardé **90 jours** dans `metrics/` (un fichier par jour
UTC, jours terminés compressés) à côté de la configuration. L'état renvoie alors le dernier relevé ; les applications
lisent les minutes enregistrées (commande `metrics`) pour les graphiques et les archives chiffrées sur GitHub
([docs/ARCHIVES.md](../docs/ARCHIVES.md)). `"metrics": false` dans `config.json` désactive l'enregistrement (lecture
à la demande, comme avant).

**Températures** :

| Système | Carte graphique | Processeur |
|---|---|---|
| Windows | **Toutes marques** (agent 1.6.0) : interface du noyau graphique de Windows, comme le Gestionnaire des tâches (cartes dédiées NVIDIA, AMD, Intel Arc ; pilote WDDM 2.4 ou plus) ; NVIDIA aussi par NVML (`nvml.dll`, fourni par le pilote) ; en secours, LibreHardwareMonitor ; puce graphique **intégrée au processeur** (Intel UHD Graphics, Iris Xe, Radeon des APU AMD), qui n'a pas de sonde à part : température de la puce, celle du processeur (agent 1.6.1) | **[LibreHardwareMonitor](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor)** (LHM), par son **serveur web** (`data.json`), ou par WMI (`root\LibreHardwareMonitor`) pour ses versions 0.9.4 et plus anciennes |
| Linux | `nvidia-smi` (pilote NVIDIA), capteurs du noyau (`amdgpu`, `nouveau`, `radeon`) ; puce Intel intégrée : température du processeur | capteurs du noyau (`/sys/class/hwmon` : `coretemp`, `k10temp`, `zenpower`) |
| macOS | — | — |

Windows ne donne pas accès aux sondes du processeur sans pilote : l'agent n'en installe **aucun** et s'appuie sur
LibreHardwareMonitor s'il tourne. Pour l'installer :

1. Téléchargez la dernière version sur sa page GitHub et décompressez-la (par exemple dans `C:\Program Files\LibreHardwareMonitor`).
2. Lancez `LibreHardwareMonitor.exe` **en administrateur** (clic droit → *Exécuter en tant qu'administrateur*). S'il
   propose d'installer son pilote **PawnIO**, acceptez : sans lui, il ne lit pas le processeur.
3. **Activez son serveur web** : *Options* → *Remote Web Server* → *Run*. Depuis la version 0.9.5, LHM ne publie
   plus rien par WMI : c'est le seul moyen pour l'agent de le lire. Gardez le port (8085) et l'interface proposés,
   sans *Authentication* (l'agent ne connaît pas ce mot de passe). Il est **inutile d'ouvrir ce port dans le
   pare-feu** : l'agent le lit sur le PC même, et ce serveur sans mot de passe permet de piloter les ventilateurs.
4. Dans *Options*, cochez **Start Minimized**, **Minimize To Tray** et **Run On Windows Startup** : il démarre avec
   Windows, discrètement, et l'agent retrouve la température du processeur à chaque démarrage.

L'agent trouve LHM même si son serveur web écoute sur une autre adresse du PC ou un autre port (il lit ses réglages,
`LibreHardwareMonitor.config`, à côté du programme). Quand la température du processeur manque, les applications
disent pourquoi :

| Message | Cause | Que faire |
|---|---|---|
| LibreHardwareMonitor requis | LHM ne tourne pas | Le lancer en administrateur (étapes ci-dessus) |
| Serveur web LHM à activer | LHM tourne, son serveur web ne répond pas | *Options* → *Remote Web Server* → *Run* |
| Mot de passe LHM à retirer | Le serveur web demande un mot de passe | Décocher *Options* → *Remote Web Server* → *Authentication* |
| Non lue par LHM | LHM répond sans température du processeur | Accepter le pilote PawnIO au démarrage de LHM, ou mettre LHM à jour |

`wol-agent status` affiche les températures lues et le détail : cartes graphiques vues par Windows, LHM trouvé (ou
non), ses réglages, la réponse de son serveur web et la présence du pilote PawnIO.

## Compilation

```bash
cd agent
go test ./...
go build -ldflags "-X main.version=1.0.0" .
GOOS=windows GOARCH=amd64 go build -o wol-agent.exe .
```

Protocole : [docs/PROTOCOLE.md](../docs/PROTOCOLE.md). Sécurité : [docs/SECURITE.md](../docs/SECURITE.md).
