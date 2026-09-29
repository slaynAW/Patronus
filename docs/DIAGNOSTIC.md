# Diagnostic

Quand quelque chose ne se passe pas comme prévu (plantage, partage qui disparaît, PC qui ne répond
pas…), les applications et l'agent produisent un **rapport de diagnostic chiffré** : il permet
d'analyser le problème en détail sans jamais exposer vos clés.

- Chaque application tient un **journal** de ses actions, des changements d'état des PC et de toutes
  ses erreurs (avec leur trace complète). Il est **chiffré sur l'appareil** et n'en sort jamais seul.
- **Rien n'est envoyé automatiquement**, ni à GitHub ni ailleurs : c'est vous qui exportez le
  rapport et choisissez à qui le transmettre.
- Le rapport est **chiffré par un mot de passe que vous choisissez** : sans lui, le fichier est
  illisible. Transmettez le fichier (pièce jointe, Google Drive…) et, **par un autre canal**, le
  mot de passe.

## Exporter un rapport

| Où | Comment |
|---|---|
| Téléphone | *Réglages* → *Diagnostic* → *Exporter le rapport de diagnostic*, choisissez un mot de passe (8 caractères au moins), puis l'emplacement du fichier. |
| Application Windows | *Réglages* → *Diagnostic* → *Exporter le rapport de diagnostic*, même principe. |
| Agent d'un PC | Dans un terminal **administrateur** : `wol-agent diagnostic` (le mot de passe est demandé deux fois, sans s'afficher). Le fichier est enregistré sur le Bureau ; `--out chemin.diag` pour le placer ailleurs. |

Le fichier s'appelle `patronus-diagnostic-AAAA-MM-JJ.diag` (agent : `patronus-diagnostic-agent-…`).
Pour un problème de partage, un rapport **de chaque appareil concerné** (celui qui partage et celui
qui reçoit) permet de suivre l'échange des deux côtés.

## Contenu du rapport

- Version de l'application, appareil ou système, langue, fuseau horaire.
- Réseau local (cartes, adresses, VPN) et autorisations.
- Réglages, et chaque PC : nom, adresse MAC, adresse IP, ports, présence d'un agent et d'une clé,
  état actuel (depuis quand, par quelle méthode, latence, version de l'agent, dernière erreur).
- Partage : nom, compte GitHub, révision publiée, personnes autorisées et PC accordés, partages
  reçus, erreurs de publication ou de vérification.
- Mises à jour, incidents sur les données (fichiers illisibles mis de côté), dernier plantage.
- Le **journal complet** (du plus ancien au plus récent) : actions, changements d'état des PC,
  étapes du partage, erreurs avec leur trace.

**Jamais dans le rapport** : clés des agents, mots de passe SecureOn, jeton GitHub, clés privées du
partage, mots de passe des sauvegardes ou du rapport (seulement leur *présence*). Les appareils
sont identifiés par une empreinte courte de leur clé **publique**, `k:1a2b3c4d` : le début de
l'identifiant du protocole, le même sur le téléphone, sur Windows et dans les noms des fichiers
d'accès du Gist (`acces-1a2b3c4d….json`), ce qui permet de relier les rapports de deux appareils.

Le rapport contient en revanche des informations personnelles (noms des PC et des personnes,
adresses IP et MAC, compte GitHub) : c'est pourquoi il est chiffré.

## Journal sur l'appareil

| Programme | Emplacement | Protection | Taille |
|---|---|---|---|
| Android | stockage privé de l'application, `diagnostics/` | chaque évènement chiffré en AES-256-GCM ; clé de données protégée par le **Keystore Android** (non extractible) | 2 × 512 Kio |
| Windows | `%APPDATA%\WakeOnLan\diagnostics\` | chaque évènement chiffré en AES-256-GCM ; clé de données protégée par **DPAPI** (votre compte Windows) | 2 × 1 Mio |
| Agent (Windows) | `C:\ProgramData\WolAgent\agent.log` | dossier réservé à SYSTEM et aux Administrateurs, comme la clé de l'agent | 2 × 1 Mio |
| Agent (Linux / macOS) | `journalctl -u wol-agent` / `/Library/Logs/wol-agent.log` | journal du système | — |

Quand un fichier du journal est plein, le plus ancien est remplacé. Si la clé du journal est perdue
(copie du dossier sur un autre appareil), les anciens évènements sont illisibles et le journal
repart de zéro.

**Plantages** : sur Android, l'erreur est écrite dans le journal avant la fermeture et proposée au
lancement suivant. Sur Windows, une erreur imprévue pendant une action est rattrapée (l'application
reste ouverte et affiche un message) ; un arrêt brutal du moteur est écrit par Go dans
`diagnostics\plantage.txt` (traces d'exécution seulement), versé dans le journal chiffré au
lancement suivant puis vidé. Les erreurs de l'interface Windows (page) sont notées aussi.

## Données illisibles

Si la liste des PC, le partage ou l'historique ne peuvent plus être relus (clé de chiffrement
perdue, fichier abîmé), l'application repart d'un état vide **sans effacer le fichier** : il est
mis de côté (`….illisible-AAAAMMJJ-HHMMSS`), l'incident est noté au journal et signalé à l'écran.

Pour tout retrouver : *Réglages* → *Importer* votre **dernière sauvegarde complète** (chiffrée).
Elle contient vos PC, leurs clés, l'historique et la clé de partage ; reconnectez-vous ensuite à
GitHub **avec le même compte** pour que les personnes invitées retrouvent leurs accès sans nouvelle
invitation.

## Format du fichier (`patronus-diagnostic/1`)

```json
{
  "format": "patronus-diagnostic",
  "version": 1,
  "app": "Patronus Android 1.5.4 (code 123)",
  "createdAt": "2026-09-29T17:34:15Z",
  "encryption": {
    "kdf": "PBKDF2WithHmacSHA256", "iterations": 600000, "salt": "…",
    "cipher": "AES-256-GCM", "iv": "…"
  },
  "data": "…"
}
```

- Clé : PBKDF2-HMAC-SHA256 du mot de passe (UTF-8), sel aléatoire de 16 octets, 600 000
  itérations (de 100 000 à 10 000 000 acceptées à la lecture), 32 octets.
- Chiffrement : AES-256-GCM, IV aléatoire de 12 octets, étiquette de 128 bits, données
  authentifiées additionnelles `patronus-diagnostic/1`. `data` est le texte du rapport chiffré.
- `app` et `createdAt` restent lisibles, à titre indicatif (ils ne protègent rien).
- Vecteur de test commun : [`protocol/diagnostic-vectors.json`](../protocol/diagnostic-vectors.json),
  vérifié par les tests Go (`agent/diagnostic`) et Kotlin (`core`).

## Ouvrir un rapport

```bash
cd agent
PATRONUS_DIAG_PASSWORD='mot de passe' go run ./cmd/diagnostic-open patronus-diagnostic-2026-09-29.diag rapport.txt
```

Sans la variable `PATRONUS_DIAG_PASSWORD`, le mot de passe est demandé ; sans fichier de sortie, le
texte est affiché. Un mot de passe incorrect ou un fichier modifié est refusé.
