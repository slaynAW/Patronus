# Hub Patronus sur le NAS (2.0) : conception

> **Statut : conception validée le 24/10/2026**, décisions du §15 retenues telles que recommandées ; réalisation en
> cours (§14). Menaces ajoutées : [MODELE-DE-MENACE.md](MODELE-DE-MENACE.md#f9-à-f12--hub-sur-le-nas-20-en-conception).

Le hub est une présence permanente à la maison : il surveille, retient et agit même quand le téléphone et les PC sont
éteints. Il tourne dans un conteneur sur le NAS. Première cible : **Synology DS218+** (DSM 7.2, processeur Intel
Celeron J3355 x86-64, Container Manager), sans exclure les autres NAS ou un Raspberry Pi.

Les applications fonctionnent **avec ou sans hub** : sans lui, elles continuent de parler directement aux agents,
comme aujourd'hui.

## 1. Ce que fait le hub

| Fonction | Détail |
|---|---|
| Surveillance continue | Interroge chaque agent confié (état, journal, mesures, disques) et vérifie les appareils sans agent (box, imprimante, caméras, le NAS lui-même) |
| Historique longue durée | Garde le journal et les mesures **un an** (réglable de 3 mois à 3 ans), au-delà des 30 jours (journal) et 90 jours (mesures) de chaque agent |
| Détection des pannes | PC injoignable, agent arrêté, arrêt inattendu, disque à surveiller, mesure grave : un PC ne peut pas signaler sa propre panne |
| Programmations | Réveil à heure fixe (« 8 h en semaine ») ; extinction à heure fixe **si le PC est inactif** (« 23 h ») |
| Service aux applications | Une seule connexion au hub au lieu d'une par PC pour les relevés en arrière-plan ; « Pendant votre absence » : ce que le hub a vu depuis la dernière ouverture |

**Prêt quand** : un NAS compromis ne donne pas le contrôle des PC (§4) ; après une semaine téléphone éteint,
l'historique est complet ; la revue de sécurité (§15, décision 4) n'a laissé aucun point majeur ouvert.

## 2. Hors du périmètre de la 2.0

- **Notifications** sur le téléphone (2.1, Web Push chiffré) : en 2.0, les évènements détectés s'affichent à
  l'ouverture de l'application.
- **Accès à distance** (2.2, VPN) : le hub n'est jamais exposé sur Internet.
- **Interface web**, MQTT, canaux tiers : aucun. L'administration se fait depuis les applications.
- **Partage par le hub** : les personnes avec qui des PC sont partagés continuent de parler aux agents (partage v2) ;
  seuls les appareils du propriétaire s'appairent au hub.
- **Relais de commandes** : le hub ne transmet jamais une extinction ou un redémarrage demandé par une application
  (§4). Les applications les envoient elles-mêmes à l'agent.

## 3. Architecture

```
┌────────────────────────┐   patronus-hub/1 : TLS 1.3 mutuel, port 9771   ┌─────────────────────────────┐
│ Application Patronus   │═══════════════════════════════════════════════▶│ Hub : conteneur sur le NAS  │
│ téléphone, PC Windows  │                                                │ non root, identité H        │
└───────────┬────────────┘                                                │ /keys : identité, clé       │
            │                                                             │ /data : historique chiffré  │
            │ wolagent/2 (inchangé) :                                     └──────┬──────────────┬───────┘
            │ détail en direct, commandes          wolagent/2, rôle wake         │              │
            │                                      (control si extinction        │              │ sondes TCP / HTTP
            ▼                                      programmée) ; paquet magique  │              ▼
┌────────────────────────┐                                                       │      box, imprimante,
│ Agent sur chaque PC    │◀──────────────────────────────────────────────────────┘      caméras, NAS…
│ identité A             │
└────────────────────────┘
```

- **Même base de code que l'agent** : le hub vit dans le module de l'agent (`agent/cmd/patronus-hub`,
  `agent/internal/hub/…`), pour réutiliser sans les dupliquer son identité, son registre des appareils, son serveur
  TLS, `agent/protocol2` et son client. Les règles communes encore internes à l'application Windows (fusion de
  l'historique, `desktop/internal/history`) passent dans un paquet public de `agent/`, vérifié par les mêmes vecteurs.
- **Un seul binaire statique** (`CGO_ENABLED=0`), sans dépendance nouvelle, dans une image `scratch` d'environ
  10 Mo, multi-architecture (`amd64`, `arm64`). Le même binaire est publié seul pour les Linux sans conteneur.
- **Port 9771** (TCP), à côté de l'agent du NAS (9770) qui reste installé et inchangé.

## 4. Moindre privilège : ce que le hub peut et ne peut pas faire

| Action | Droit nécessaire sur l'agent | Le hub l'a ? |
|---|---|---|
| Lire état, journal, mesures, disques, fiche | `view` | Oui (`wake` inclut `view`) |
| Noter au journal de l'agent les réveils qu'il envoie | `wake` | Oui |
| Envoyer un paquet magique | Aucun (norme Wake-on-LAN) | Oui, comme tout appareil du réseau |
| Éteindre, redémarrer, mettre en veille | `control` | **Seulement** pour un PC dont l'extinction programmée est activée. Le hub ne s'en sert que pour éteindre, mais l'agent ne distingue pas : un hub compromis pourrait aussi redémarrer ou mettre en veille ce PC |
| Ajouter, retirer, changer le rôle d'un appareil ; créer une invitation | `admin` | **Jamais** |

- Les rôles du hub sur un agent ne se changent **que depuis une application administrateur du PC, directement
  auprès de l'agent** (`devices.role`). Le hub ne peut pas s'en donner davantage : il n'est administrateur d'aucun PC.
- Le hub **ne relaie aucune commande** des applications : une application qui veut éteindre un PC s'adresse à
  l'agent. Un hub compromis ne peut donc pas servir à éteindre les PC dont il n'a pas reçu le rôle `control`.
- Chaque action du hub sur un PC (réveil noté, extinction programmée) est **notée par l'agent** avec l'empreinte du
  hub (`i`) et son nom (« Hub NAS ») : un hub compromis laisse des traces sur chaque PC, hors de sa portée.
- Le rôle `control` reçu pour une extinction programmée est **retiré** quand la programmation est désactivée.

Ce qu'un attaquant qui a pris le NAS obtient au pire (risque résiduel de T30) : la lecture de tout ce que le hub sait
(états, historique, mesures, adresses), l'envoi de réveils, l'extinction (ou le redémarrage, la veille) des seuls PC
à extinction programmée, et le NAS lui-même. Ni prise de contrôle des autres PC, ni ajout d'appareil, ni effacement de leurs journaux.

## 5. Identité et appairage

### 5.1 Identité du hub

- Paire de clés **ECDSA P-256** générée au premier démarrage, comme un agent (`wolagent/2`, [§2](PROTOCOLE-V2.md#2-identités)) :
  fichier `0600` dans le volume **`/keys`**, séparé du volume de données, appartenant à l'utilisateur du conteneur.
- Empreinte affichée dans le journal du conteneur et dans les applications (empreinte courte `7KQ2-9MXP-4D1Z`).
- Le journal du conteneur ne contient aucun secret, hormis le code d'appairage à usage unique du §5.2 : ni clé, ni
  code d'invitation d'un agent, ni contenu des relevés.

### 5.2 Appairer une application au hub

1. Au premier démarrage (aucun administrateur), le hub écrit dans **le journal du conteneur** (Container Manager →
   *Conteneur* → *Journal*) un lien `patronus://hub?v=1&n=<nom>&h=<ip>&p=9771&a=<empreinte>&c=<code>&r=admin&e=<expiration>` et son code, valables
   **10 minutes, à usage unique** ; en SSH, `sudo docker exec patronus-hub patronus-hub pair` en affiche un autre.
2. L'application (*Réglages* → *Hub* → *Coller le lien*, ou saisie du code et de l'adresse) se connecte, vérifie
   l'empreinte, envoie `pair` : elle devient **administrateur du hub**.
3. Les appareils suivants sont invités depuis une application administrateur du hub (invitation à usage unique,
   QR code affiché par l'application), comme pour un PC ([§7.5](PROTOCOLE-V2.md#75-depuis-un-appareil-administrateur)).

Mêmes règles qu'un agent : code à usage unique, expirant, tentatives limitées par adresse et au total, alerte
« Ce code a déjà servi » et appareil intrus visible et révocable.

### 5.3 Confier un PC au hub

Depuis une application **administrateur du PC et du hub** :

1. l'application demande à l'agent une invitation **réservée à l'empreinte du hub** (`invite`, `role: wake`,
   `device: <empreinte du hub>`, 10 minutes) ;
2. elle transmet au hub l'adresse, le port, la MAC, l'empreinte de l'agent et ce code (`hub.pcs.add`) ;
3. le hub s'appaire à l'agent (`pair`), vérifie l'empreinte épinglée, puis commence la surveillance.

Une invitation interceptée ne sert à personne d'autre que le hub. Retirer un PC du hub : l'application retire le PC
de la liste du hub **et** retire le hub de l'agent (`devices.revoke`) ; sur le PC, `wol-agent devices revoke` suffit.

### 5.4 Appareils sans agent

Ajoutés par une application administrateur du hub : nom, adresse, type de sonde (§7.2), MAC facultative (réveil).

## 6. Protocole applications ↔ hub (`patronus-hub/1`)

Spécification complète : [PROTOCOLE-HUB.md](PROTOCOLE-HUB.md).

- **Transport identique à `wolagent/2`** ([PROTOCOLE-V2.md §3 et §4](PROTOCOLE-V2.md#3-transport)) : TLS 1.3 à
  authentification mutuelle, empreintes épinglées, mêmes trames, mêmes limites et blocages. Seuls changent l'**ALPN**
  (`patronus-hub/1`) et le **port** (9771) : une réponse du hub ne peut pas être prise pour celle d'un agent, ni
  l'inverse (T36).
- **Rôles sur le hub** : `view` (consulter), `wake` (demander un réveil au hub), `admin` (gérer le hub). Pas de
  `control` : le hub ne relaie pas les commandes.

| Commande | Rôle | Réponse |
|---|---|---|
| `hub.status` | `view` | État de chaque PC et appareil connu : dernier relevé de l'agent (comme `status`, mesures comprises) avec son heure, ou résultat de la sonde ; état du hub (version, place utilisée) |
| `hub.events` `{"since":…}` | `view` | Évènements détectés par le hub (§7.3) depuis une date : « Pendant votre absence » |
| `hub.history` `{"pc":…,"from":…}` | `view` | Journal du PC sur la durée gardée, déjà fusionné (mêmes règles que les applications) |
| `hub.metrics` `{"pc":…,"day":…}` | `view` | Mesures minute par minute d'un jour, comme `metrics`, sur la durée gardée |
| `hub.wake` `{"pc":…}` | `wake` | Le hub envoie le paquet magique depuis le réseau local et le note à l'agent |
| `hub.pcs`, `hub.pcs.add`, `hub.pcs.update`, `hub.pcs.remove` | `admin` | PC et appareils confiés au hub (§5.3, §5.4) |
| `hub.schedules`, `hub.schedules.set` | `admin` | Programmations (§8) |
| `hub.settings` | `admin` | Durée de conservation, fuseau horaire |
| `devices`, `devices.revoke`, `devices.role`, `devices.rename`, `invite`, `invites`, `invites.cancel` | `admin` | Appareils autorisés sur le hub, comme sur un agent |
| `pair` | Identité inconnue | Appairage (§5.2) |

Les réponses restent bornées : 8 Kio comme un agent, 512 Kio pour `hub.status`, `hub.history` et `hub.metrics`.
Le hub n'accepte que les adresses des réseaux privés, comme la liste `allow` d'un agent. Les
identifiants de PC sont opaques (16 octets aléatoires en B64), propres au hub.

## 7. Surveillance

### 7.1 PC avec agent

| Relevé | Fréquence | Remarque |
|---|---|---|
| `status` | toutes les 15 s, sur une connexion gardée ouverte (100 requêtes au plus, puis nouvelle connexion) | Délai doublé après un refus, comme les applications ([§3.3](PROTOCOLE-V2.md#33-durées-et-limites)) |
| `history` | toutes les 10 min et à chaque changement d'état | Fusionné avec ce que le hub garde |
| `metrics` | chaque jour pour la veille ; au premier appairage, les 90 jours disponibles | Lignes minute par minute, gardées telles quelles |
| `specs` | une fois par jour | |

### 7.2 Appareils sans agent

- **Connexion TCP** à un port (par défaut 80, 443, 445 ou 22), ou **page web** (HTTP ou HTTPS) : seule compte la
  réponse, jamais son contenu. Aucune donnée envoyée au-delà d'une requête `HEAD /` sans cookie ni identifiant.
  Pour une page HTTPS, le certificat (souvent autosigné sur une box ou une caméra) n'est pas vérifié : rien de
  confidentiel n'est envoyé ni lu.
- **Ping** (ICMP) seulement si l'utilisateur ajoute la capacité `NET_RAW` au conteneur (§10) ; sinon, TCP.
- Adresses **privées uniquement** (mêmes réseaux que la liste `allow` des agents : RFC 1918, lien local, ULA IPv6) :
  le hub ne sonde rien sur Internet.
- Toutes les 30 s ; résultat réduit à « répond / ne répond pas » et la latence.

### 7.3 Évènements détectés

| Évènement | Détection |
|---|---|
| PC injoignable | Agent et PC muets alors que le dernier état était « allumé », sans arrêt annoncé par l'agent |
| Agent arrêté | Le port de l'agent refuse la connexion (le PC répond) pendant plus de 2 min |
| Arrêt inattendu | Évènement `lost` dans le journal de l'agent (coupure, arrêt forcé, plantage) |
| Disque à surveiller, lecteur plein | Santé `warning` / `bad`, lecteur plein à 90 % |
| Mesure grave | Gravité `crit` d'une mesure ([PROTOCOLE.md](PROTOCOLE.md#mesures-measures-agent-1130-ou-plus)) pendant plus de 5 min |
| Réveil non demandé | Démarrage sans demande connue (ni application, ni hub) : 2.1, avec les notifications (T13) |
| Programmation | Réveil envoyé ; extinction faite, ou **reportée** car le PC n'était pas inactif |

## 8. Programmations

- **Réveil** : à l'heure dite, paquet magique (diffusion sur le réseau local, d'où le réseau de l'hôte, §10), puis
  `wakes` à l'agent dès qu'il répond : le journal du PC indique « démarrage demandé par Hub NAS ».
- **Extinction si inactif** : à l'heure dite, le hub éteint le PC **seulement** s'il est inactif : utilisation du
  processeur sous 10 % pendant les 15 dernières minutes, et aucune commande d'une application dans l'heure. Sinon il
  réessaie toutes les 15 min pendant 2 h, puis renonce (évènement « extinction reportée »). Extinction normale,
  **jamais forcée** : une application qui refuse de se fermer l'emporte. Nécessite le rôle `control` sur ce PC (§4),
  demandé à l'agent par l'application au moment où l'utilisateur active la programmation.
- Jours de la semaine et heure, dans le **fuseau horaire** réglé sur le hub (Europe/Paris par défaut), changements
  d'heure compris.

## 9. Données et chiffrement au repos

### 9.1 Ce que le hub garde

| Donnée | Où | Durée |
|---|---|---|
| Identité du hub (clé privée) | `/keys/identity.key` | Jusqu'à `patronus-hub rotate-identity` |
| Clé des données (32 octets aléatoires) | `/keys/data.key` | Jusqu'à l'effacement |
| Appareils autorisés, invitations (condensé du code seulement) | `/data/devices.db`, chiffré | — |
| PC confiés : nom, adresse, MAC, empreinte de l'agent, sondes, programmations | `/data/hub.db`, chiffré | — |
| Journal et mesures de chaque PC | `/data/pc/<identifiant opaque>/history-<AAAA-MM>.bin` et `metrics-<AAAA-MM>.bin`, chiffrés | 1 an par défaut (3 mois à 3 ans) |
| Réglages (durée de conservation) | `/data/settings.db`, chiffré | — |

Place estimée (mesures minute par minute, seulement quand le PC est allumé) : environ 10 Mo par an pour un PC
allumé 4 h par jour, 70 Mo pour un PC qui ne s'éteint jamais.

### 9.2 Format

- Pas de moteur de base de données (aucune dépendance) : des fichiers **ajout seul**, une suite d'enregistrements
  chiffrés un par un, ce qui résiste à une coupure de courant (un enregistrement tronqué en fin de fichier est ignoré).
- **AES-256-GCM**, nonce aléatoire de 96 bits par enregistrement ; une clé par fichier dérivée par **HKDF-SHA256** de
  la clé des données (sans sel ; information : `patronus-hub/1 file key`, un octet nul, puis le chemin du fichier) :
  un enregistrement déplacé d'un fichier à l'autre est refusé. `hub.db` et `devices.db` sont réécrits en entier à
  chaque changement (fichier temporaire, écriture sur le disque, puis renommage).
- **Fichier entier** : `PHF1`, nonce, texte chiffré et étiquette ; données authentifiées : `PHF1` puis le chemin.
- **Journal** : `PHJ1`, puis chaque enregistrement : longueur du reste (4 octets, gros-boutiste), nonce, texte chiffré
  et étiquette ; données authentifiées : `PHJ1`, rang de l'enregistrement (8 octets, gros-boutiste) puis le chemin.
  Un enregistrement retiré, interverti ou modifié rend le journal illisible ; un enregistrement incomplet en fin de
  journal est ignoré, puis remplacé au prochain ajout. 1 Mio au plus par enregistrement, 64 Mio par fichier entier.
- La clé des données (`/keys/data.key`, PEM, `0600`) n'est jamais remplacée si elle est illisible : une nouvelle clé
  rendrait toutes les données illisibles.
- Vecteurs de test dans `protocol/hub-store-vectors.json` (paquet `agent/internal/hub/store`), recalculés à chaque
  build par une seconde implémentation écrite d'après ce paragraphe, en Python sans bibliothèque externe
  (`.github/scripts/check-hub-store-vectors.py`).
- En clair : le nombre de PC, les mois couverts et la taille des fichiers.

### 9.3 Contre quoi protège ce chiffrement

- Copie du dossier de données : par un autre utilisateur du NAS, une sauvegarde Hyper Backup (le dossier de données
  peut être sauvegardé : sans la clé, il est illisible), un partage de fichiers mal réglé.
- **Ne protège pas** contre un disque volé si la clé est sur le même volume, sauf volume chiffré par DSM ; ni contre
  un attaquant root sur le NAS (T30).
- **Clé perdue = historique du hub perdu**. Les agents gardent leurs 30 jours de journal et 90 jours de mesures, et
  les archives GitHub des applications restent : rien d'essentiel ne disparaît. Le guide conseille de copier une fois
  le dossier `/keys` sur une clé USB rangée à part.

## 10. Conteneur et installation sur le DS218+

Fichier de projet pour Container Manager (*Projet* → *Créer*) :

```yaml
services:
  patronus-hub:
    image: ghcr.io/slaynaw/patronus-hub:2.0.0
    container_name: patronus-hub
    network_mode: host            # paquets magiques en diffusion, adresses réelles des applications
    user: "65532:65532"           # ni root, ni compte DSM
    read_only: true
    cap_drop: [ALL]
    # cap_add: [NET_RAW]          # seulement pour le ping des appareils sans agent (§7.2)
    security_opt: [no-new-privileges:true]
    mem_limit: 128m
    pids_limit: 64
    environment:
      TZ: Europe/Paris
    volumes:
      - /volume1/docker/patronus-hub/data:/data
      - /volume1/patronus-hub-keys:/keys
    restart: unless-stopped
```

- **Réseau de l'hôte** : seule exception au cloisonnement. Un paquet magique en diffusion ne sort pas d'un réseau
  Docker ponté ; et le hub doit voir l'adresse réelle des applications pour sa liste de réseaux autorisés.
- **Utilisateur 65532**, sans capacité, système de fichiers en lecture seule, `no-new-privileges`, **pas de socket
  Docker**, limites de mémoire et de processus.
- **Deux dossiers** préparés en SSH (`sudo mkdir`, `sudo chown 65532:65532`, `sudo chmod 700`) :
  `/volume1/docker/patronus-hub/data` (dans le dossier partagé `docker`, sauvegardable) et
  `/volume1/patronus-hub-keys` (hors de tout dossier partagé : ni visible sur le réseau, ni sauvegardé).
- **Pare-feu de DSM** : autoriser le port TCP **9771** depuis le réseau local seulement, comme 9770.
- **Mises à jour** : Container Manager → *Image* → mettre à jour, puis relancer le projet. Le hub ne se met pas à jour
  lui-même et ne contacte pas Internet.
- Guide pas à pas : `docs/HUB-SYNOLOGY.md`, écrit avec le code.

## 11. Chaîne de production

- Image et binaire construits par la CI à partir du même commit que les autres versions (`-trimpath`, sans cgo),
  publiés **seulement** par le job « Publication officielle », après ta validation.
- Provenance attestée pour l'image (empreinte `sha256`) et le binaire, inventaire des composants (SBOM),
  `govulncheck`. Vérification : `gh attestation verify oci://ghcr.io/slaynaw/patronus-hub:2.0.0 --repo slaynAW/Patronus`.
- Le fichier de projet du guide peut épingler l'image par son empreinte (`@sha256:…`) pour qui veut vérifier avant
  chaque mise à jour.

## 12. Applications

- *Réglages* → **Hub** : appairer, état du hub (version, place, dernier relevé), PC confiés, appareils sans agent,
  programmations, appareils autorisés sur le hub.
- Fiche d'un PC : *Confier au hub* (§5.3), *Programmer* (§8), historique et mesures **au-delà de 30 et 90 jours**
  lus sur le hub.
- Liste des PC : relevés en arrière-plan par le hub (une connexion) ; quand l'application est ouverte sur le réseau
  local, elle garde ses relevés directs en temps réel. Un état venu du hub porte son âge (« il y a 12 s, via le hub »).
- « **Pendant votre absence** » : évènements du hub depuis la dernière ouverture. Ce sont des **codes** (type,
  PC, heure), rédigés par l'application avec ses propres noms de PC : jamais de texte libre ni de lien venus du hub,
  qu'un hub compromis pourrait utiliser pour tromper l'utilisateur (T38).
- **Sans hub** ou hub injoignable : tout fonctionne comme en 1.15, directement avec les agents.
- Le journal d'un agent fait toujours foi sur la période qu'il couvre (règles de fusion communes) : un hub ne peut
  pas réécrire ce que les agents ont noté (T38).

## 13. Compatibilité et versions

- Le hub exige des agents en `wolagent/2` (agent 1.11.0 ou plus) ; il ne parle jamais `wolagent/1`.
- La 2.0 désactive aussi `wolagent/1` par défaut **sur les nouvelles installations** de l'agent
  ([PROTOCOLE-V2.md §11](PROTOCOLE-V2.md#11-coexistence-et-calendrier)) ; les installations existantes le gardent
  jusqu'à son retrait.
- Applications 2.0.0 (Android, Windows), hub 2.0.0, agent 1.14.0.

## 14. Plan de réalisation

Une PR par étape, chacune testée en CI et fusionnée quand tout est vert :

| Étape | Contenu |
|---|---|
| 2.0-A | Ce document et le modèle de menace (validation) : fait |
| 2.0-B | Stockage chiffré (§9) et ses vecteurs, recalculés par une seconde implémentation : fait |
| 2.0-C | Hub : identité, registre des appareils (dans le stockage chiffré), serveur `patronus-hub/1`, appairage, commande `patronus-hub` ; spécification du protocole ([PROTOCOLE-HUB.md](PROTOCOLE-HUB.md)) et vecteurs |
| 2.0-D | Surveillance : agents confiés (§5.3, §7.1), sondes (§7.2), évènements (§7.3), fusion de l'historique en paquet commun, conservation et nettoyage ; tests de bout en bout contre le vrai agent |
| 2.0-E | Module `core` (Kotlin) : client `patronus-hub/1`, vérifié par les mêmes vecteurs |
| 2.0-F | Application Windows : réglages Hub, PC confiés, historique long, « Pendant votre absence » |
| 2.0-G | Application Android : idem |
| 2.0-H | Programmations (§8), rôle `control` donné et retiré par l'application |
| 2.0-I | Image de conteneur multi-architecture, attestation, publication ; guide `docs/HUB-SYNOLOGY.md` |
| 2.0-J | Agent 1.14.0 (`wolagent/1` désactivé sur les nouvelles installations), revue de sécurité (§15), versions 2.0.0 |

## 15. Décisions à prendre

| # | Question | Recommandation |
|---|---|---|
| 1 | Distribution de l'image | **GitHub Container Registry** (`ghcr.io/slaynaw/patronus-hub`) : mise à jour en deux clics dans Container Manager. Il faudra rendre le paquet public une fois (réglage du paquet sur GitHub). Autre choix : un fichier `.tar` dans les Releases, importé à la main à chaque version. |
| 2 | Affichage du premier code d'appairage | **Journal du conteneur** (lisible des seuls administrateurs DSM), usage unique, 10 min ; `sudo docker exec` en SSH pour un nouveau code. |
| 3 | Sauvegarde de l'historique du hub | **Dossier de données sauvegardable par Hyper Backup** (chiffré), clé copiée une fois à part ; pas de sauvegarde propre au hub en 2.0. |
| 4 | Revue de sécurité indépendante avant la 2.0 | Si tu ne trouves personne : publier d'abord une **préversion 2.0.0-beta** pour ton usage, avec une revue adverse complète de ma part (protocole, stockage, conteneur) et des tests de fuzzing, puis la 2.0.0. |
| 5 | Critère d'inactivité de l'extinction programmée | Processeur sous 10 % pendant 15 min et aucune commande dans l'heure ; nouvel essai toutes les 15 min pendant 2 h. |
| 6 | Durée de conservation par défaut | **1 an**, réglable de 3 mois à 3 ans. |
