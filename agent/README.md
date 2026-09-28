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
- service **WolAgent** (démarrage automatique, relance en cas d'erreur), journal dans `C:\ProgramData\WolAgent\agent.log` ;
- règle de pare-feu « Wake On LAN - Agent » : port 9770, **sous-réseau local**, profils privé et domaine.

**Linux** (systemd) / **macOS** (launchd) :
```bash
chmod +x wol-agent-linux-amd64
sudo ./wol-agent-linux-amd64 install
```
- copie dans `/usr/local/bin/wol-agent`, configuration dans `/etc/wol-agent/config.json` (macOS : `/Library/Application Support/WolAgent/`) ;
- Linux : service `wol-agent.service` durci ; ouverture du port si `firewalld` ou `ufw` est actif. Journal : `journalctl -u wol-agent`.

À la fin, un **QR code** s'affiche : dans l'application, *Ajouter un PC* → *Scanner le QR code*.

> Réinstaller (ou installer une nouvelle version) conserve la configuration et la clé : pas besoin de ré-appairer.

## Commandes

| Commande | Rôle |
|---|---|
| `wol-agent install [--port 9770] [--name "PC Bureau"] [--ip 192.168.1.20] [--no-firewall] [--firewall-public]` | Installe / met à jour le service |
| `wol-agent pair [--ip …] [--png qr.png] [--invert]` | Réaffiche le QR code (ou l'enregistre en PNG) et le lien d'appairage |
| `wol-agent status` | État du service, configuration, carte réseau détectée, derniers évènements du journal |
| `wol-agent rotate-key` | Nouvelle clé ; l'ancienne est immédiatement refusée (ré-appairer) |
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

- `allow` : réseaux autorisés à se connecter (par défaut uniquement des adresses privées).
- `commands` : retirez par exemple `"shutdown"` pour n'autoriser que la veille. La lecture du journal (`history`) est
  autorisée dès que `status` l'est.
- Après modification : redémarrez le service (`wol-agent install` le fait, ou `systemctl restart wol-agent`,
  ou *Services* → *Wake On LAN - Agent* → *Redémarrer* sous Windows).

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
et sorties de veille, commandes reçues (avec l'adresse de l'appareil qui les a envoyées). Un arrêt qu'il n'a pas pu noter
(coupure de courant, arrêt forcé, plantage) apparaît comme « arrêt inattendu », daté du dernier signe de vie (`alive.json`,
mis à jour chaque minute). Les applications relisent ce journal dès que le PC répond.

> Mettre à jour l'agent (version 1.2 ou plus) suffit pour en profiter : relancez simplement `install` avec la nouvelle
> version, la configuration et la clé sont conservées. Avec un agent plus ancien, les applications n'affichent que les
> changements qu'elles constatent elles-mêmes, pendant qu'elles sont ouvertes.

## Compilation

```bash
cd agent
go test ./...
go build -ldflags "-X main.version=1.0.0" .
GOOS=windows GOARCH=amd64 go build -o wol-agent.exe .
```

Protocole : [docs/PROTOCOLE.md](../docs/PROTOCOLE.md). Sécurité : [docs/SECURITE.md](../docs/SECURITE.md).
