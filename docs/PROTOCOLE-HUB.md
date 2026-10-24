# Protocole `patronus-hub/1` (spécification)

Protocole entre les applications Patronus (Android, Windows) et le hub sur le NAS ([HUB.md](HUB.md)). Il reprend le
transport de [`wolagent/2`](PROTOCOLE-V2.md) (salut, TLS 1.3 à authentification mutuelle, trames JSON) avec son propre
protocole applicatif, son port, ses rôles et ses commandes. Les menaces auxquelles il répond sont T34 à T36 et T45 du
[modèle de menace](MODELE-DE-MENACE.md#f9-à-f12--hub-sur-le-nas-20-en-conception).

- Implémentation de référence : [`agent/hubprotocol`](../agent/hubprotocol) (fonctions pures),
  [`agent/internal/hub/server`](../agent/internal/hub/server) (serveur), commande `patronus-hub`
  ([`agent/cmd/patronus-hub`](../agent/cmd/patronus-hub)). Client des applications :
  [`core/…/core/hub`](../core/src/main/kotlin/io/github/slaynaw/wakeonlan/core/hub) (Kotlin), éprouvé à chaque
  build contre le vrai hub (`HubEndToEndTest`) ; client de l'application Windows :
  [`desktop/internal/agentclient/hub.go`](../desktop/internal/agentclient/hub.go), éprouvé contre le vrai hub
  ([`agent/testhub`](../agent/testhub)).
- Vecteurs communs : [`protocol/hub-vectors.json`](../protocol/hub-vectors.json) (liens, salut, rôles, autorisation),
  lus en Go et en Kotlin.
- **État** : étape 2.0-E. Le hub sert l'appairage, les appareils autorisés, les invitations, `hub.status`, les PC
  confiés (`hub.pcs.*`), leur archive (`hub.history`, `hub.metrics`), ses réglages (`hub.settings`), les évènements
  (`hub.events`) et le réveil (`hub.wake`) ; les programmations (§6) répondent `unsupported` jusqu'à leur étape
  ([HUB.md](HUB.md#14-plan-de-réalisation)).

## 1. Ce qui diffère de `wolagent/2`

| | `wolagent/2` (agent) | `patronus-hub/1` (hub) |
|---|---|---|
| Port par défaut | 9770 | **9771** |
| Salut | `{"proto":"wolagent/1",…}` ou `{"proto":"wolagent/2"}` | `{"proto":"patronus-hub/1"}` |
| ALPN | `wolagent/2` | `patronus-hub/1` |
| Rôles | `view`, `wake`, `control`, `admin` | `view`, `wake`, `admin` : pas de `control`, le hub ne relaie aucune commande vers les PC |
| `wolagent/1`, migration | Oui, pendant la transition | Non : `migrate` est une commande inconnue |
| Lien d'appairage | `wolagent://pair?v=2&…` | `patronus://hub?v=1&…` |

Distincts par le port, le salut et l'ALPN, les deux protocoles ne peuvent pas être confondus (T36) : un client
`wolagent/2` refuse le salut du hub, le hub refuse l'ALPN `wolagent/2`, et un lien du hub n'est jamais lu comme
celui d'un agent.

## 2. Transport

1. Connexion TCP au port du hub (9771). Seules les adresses des réseaux privés sont acceptées : les mêmes que la liste
   `allow` par défaut d'un agent (`127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.0.0/16`,
   `100.64.0.0/10`, `::1/128`, `fc00::/7`, `fe80::/10`). Une autre adresse est fermée sans réponse.
2. **Salut** du hub, une ligne terminée par `\n` : `{"proto":"patronus-hub/1"}`, ou `{"error":"rate_limited"}` à une
   adresse bloquée, puis fermeture. Le client lit au plus 256 octets, sans rien lire au-delà de la ligne, et exige
   exactement ce protocole.
3. **TLS 1.3** : mêmes paramètres que `wolagent/2` ([§3.2](PROTOCOLE-V2.md#32-tls)) — suites, échanges de clés,
   signature `ecdsa_secp256r1_sha256`, certificat client obligatoire, ni reprise de session ni 0-RTT — avec l'ALPN
   `patronus-hub/1`, exigé des deux côtés. Le client vérifie l'empreinte du hub épinglée (lien d'appairage) pendant la
   poignée de main, avant tout envoi.
4. **Trames** : celles de `wolagent/2` ([§4.1](PROTOCOLE-V2.md#41-trame)) ; une requête à la fois.
5. **Limites** : celles du [§3.3](PROTOCOLE-V2.md#33-durées-et-limites) — 10 s pour TLS, 30 s d'inactivité (10 s
   pour une identité inconnue), 100 requêtes par connexion, 16 connexions, blocage d'une adresse 5 minutes après
   5 échecs en une minute (TLS refusé, identité inconnue, code refusé), suspension de l'appairage 10 minutes après plus
   de 20 codes refusés en 10 minutes. Une simple sonde TCP n'est pas un échec.

## 3. Identité du hub

Paire de clés ECDSA P-256 générée au premier démarrage, présentée par un certificat autosigné
([§2.2](PROTOCOLE-V2.md#22-certificat)). Clé privée en PKCS#8 (PEM), fichier `identity.key` en `0600` dans le dossier
des clés (`/keys`), à part des données. Son empreinte (`pin-sha256`, B64) est celle du lien d'appairage ; l'empreinte
courte (`7KQ2-9MXP-4D1Z`) est affichée dans le journal du conteneur et par les applications.

## 4. Requêtes et réponses

Requête : `{"id":…,"cmd":"…", …}`, identifiants strictement croissants sur une connexion (sinon `bad_request` et
fermeture), comme en [§4.2](PROTOCOLE-V2.md#42-requête).

Réponse à un appareil enregistré :

```json
{"id":3,"ok":true,"code":"ok","role":"admin","hub":{"name":"Hub NAS","version":"2.0.0","pin":"<empreinte>","started":1792845451}}
```

| Champ | Contenu |
|---|---|
| `id`, `ok`, `code`, `message` | Comme en `wolagent/2` ; codes du [§4.4](PROTOCOLE-V2.md#44-codes-de-réponse) |
| `role` | Rôle de l'appareil qui demande |
| `device` | `pair` : empreinte de l'appareil enregistré |
| `devices`, `invites` | Comme en [§6.2](PROTOCOLE-V2.md#62-administration-rôle-admin) |
| `invite` | Réponse à `invite` : `id`, `code` (seule fois où il apparaît), `role`, `name`, `expires`, `device`, et `hub` (empreinte du hub) |
| `hub` | `hub.status` : `name`, `version`, `pin`, `started` (démarrage, secondes Unix) |

Une identité inconnue ne reçoit que `{"id","ok","code","message"}` : ni nom, ni version.

## 5. Rôles

Cumulatifs, comme en `wolagent/2` :

| Rôle | Permet |
|---|---|
| `view` | Consulter : état des PC et du hub, évènements, historique et mesures gardés par le hub |
| `wake` | En plus, demander au hub de démarrer un PC (`hub.wake`) |
| `admin` | En plus, gérer le hub : PC confiés, programmations, réglages, appareils autorisés, invitations |

Vérifications, dans l'ordre : identité enregistrée (sinon seule `pair` passe : `unknown_device`, échec compté,
fermeture) ; `pair` par une identité enregistrée : `already_paired` ; commande connue (nom exact) sinon `unsupported` ;
rôle suffisant sinon `insufficient_role`. Un rôle inconnu du hub (`control`) ne donne aucun droit.

Rôles par défaut : `admin` pour le lien affiché par le hub (premier démarrage, `patronus-hub pair`) ; `view` pour une
invitation qui n'en précise pas.

## 6. Commandes

| Commande | Rôle minimal | Étape |
|---|---|---|
| `hub.status` | `view` | 2.0-C : `hub` (§4) ; 2.0-D : `pcs` et `states` (§6.2) |
| `hub.history`, `hub.metrics` | `view` | 2.0-D : §6.3 |
| `hub.events` | `view` | 2.0-D : §6.5 |
| `hub.wake` | `wake` | 2.0-D : §6.6 |
| `hub.pcs`, `hub.pcs.add`, `hub.pcs.update`, `hub.pcs.remove` | `admin` | 2.0-D : §6.1 |
| `hub.schedules`, `hub.schedules.set` | `admin` | 2.0-H |
| `hub.settings` | `admin` | 2.0-D : §6.4 |
| `devices`, `devices.revoke`, `devices.role`, `devices.rename` | `admin` | 2.0-C |
| `invite`, `invites`, `invites.cancel` | `admin` | 2.0-C |
| `pair` | Identité inconnue | 2.0-C |

Les commandes d'appareils et d'invitations ont les champs et les règles de `wolagent/2`
([§6.2](PROTOCOLE-V2.md#62-administration-rôle-admin), [§6.3](PROTOCOLE-V2.md#63-appairage-identités-inconnues),
[§8](PROTOCOLE-V2.md#8-registre-des-appareils-et-révocation)), avec les rôles du hub : un rôle `control` est refusé
(`bad_request`). La révocation ferme sur-le-champ les connexions ouvertes de l'appareil. Le dernier administrateur ne
peut être ni retiré ni rétrogradé depuis une application (`last_admin`) ; sur le NAS, c'est possible avec
`"force":true`.

### 6.1 PC confiés et appareils sans agent

`hub.pcs` liste les PC confiés au hub et les appareils sans agent (`pcs`). Chacun :

| Champ | Contenu |
|---|---|
| `id` | Identifiant propre au hub (16 octets aléatoires, B64) |
| `name`, `host` | Nom (64 caractères au plus) et adresse (IPv4, IPv6 ou nom d'hôte, règles du lien d'appairage) |
| `mac` | Adresse MAC, pour le réveil (facultative) |
| `agent`, `port`, `role` | PC avec agent : empreinte de l'agent, port, rôle du hub sur l'agent (`view`, `wake`, ou `control` pour une extinction programmée) |
| `probe` | Appareil sans agent : `{"kind":"tcp","port":445}`, `{"kind":"http"}` ou `{"kind":"https"}` (ports 80 et 443 par défaut) |
| `added` | Date d'ajout (secondes Unix) |

**`hub.pcs.add`** : `name`, `host`, `mac`, puis soit `agent`, `port` (9770 par défaut) et `code`, soit `probe`.

Pour un PC avec agent, l'application, administrateur du PC, crée d'abord sur l'agent une invitation **réservée à
l'empreinte du hub** (`invite`, `"role":"wake"`, `"device":"<empreinte du hub>"`), puis transmet son code au hub.
Le hub s'appaire alors lui-même à l'agent (`pair`, sous son nom), l'empreinte de l'agent épinglée, et répond avec le
PC (`pc`). Une invitation au rôle `admin` est ramenée à `wake` par le hub lui-même (`devices.role`) : le hub n'est
administrateur d'aucun PC (R13). S'il ne peut s'en défaire (seul administrateur du PC), l'appairage échoue et le
message indique la commande à lancer sur le PC pour le retirer. Un agent déjà confié est refusé ; un agent qui connaît
déjà le hub (PC retiré puis confié de nouveau) garde le rôle qu'il lui donne.

**`hub.pcs.update`** `{"pc":…}` : change le nom, l'adresse, la MAC, le port de l'agent ou la sonde (seuls les champs
présents). L'agent d'un PC ne change pas : il faut le retirer puis le confier de nouveau. **`hub.pcs.remove`**
`{"pc":…}` : le hub oublie le PC. Le hub ne peut pas se retirer lui-même de l'agent (il n'en est pas administrateur) :
l'application le fait (`devices.revoke` sur l'agent), ou `wol-agent devices revoke` sur le PC.

Le hub ne joint que les adresses des réseaux privés, **sans l'adresse de bouclage** (le NAS se joint par son adresse
sur le réseau local) : un nom d'hôte est résolu, et seules ses adresses privées sont essayées (T40). 64 PC et appareils
au plus.

### 6.2 État des PC (`hub.status`)

En plus de `hub` (§4), la réponse porte `pcs` (comme `hub.pcs`, §6.1) et `states`, l'état de chacun, dans le même
ordre :

| Champ | Contenu |
|---|---|
| `id` | Identifiant du PC |
| `state` | `unknown` (pas encore relevé), `online`, `offline` (rien ne répond), `agent_down` (le PC refuse la connexion au port de l'agent : il est allumé, l'agent arrêté) |
| `since`, `checked` | Début de cet état, dernier relevé (secondes Unix, horloge du hub) |
| `latency` | Durée du dernier relevé réussi, en millisecondes |
| `problem` | Le PC est joint, mais pas relevé : `revoked` (l'agent ne connaît plus le hub), `pin_mismatch` (l'agent présente une autre empreinte : réinstallé, ou intermédiaire), `rate_limited` (l'agent bloque l'adresse du hub) |
| `agent` | Dernière réponse de l'agent à `status` ([PROTOCOLE.md](PROTOCOLE.md#réponse-r) : système, températures, disques, mesures…), lue au relevé `checked` quand l'état est `online` |

Les applications valident `agent` comme une réponse reçue directement d'un agent (mesures lues élément par élément,
limites des textes).

**Relevés** : `status` de chaque agent toutes les 15 s, sur une session gardée ouverte (rouverte quand l'agent la
ferme : inactivité, 100 requêtes) ; sonde de chaque appareil toutes les 30 s ; 5 s au plus par relevé, 8 en même
temps. Une sonde TCP compte comme réussie si le port accepte la connexion ; une page web, si elle répond (`HEAD /`,
sans cookie ni identifiant, redirections jamais suivies, certificat non vérifié : rien n'est envoyé ni lu). Après un
refus de l'agent, les relevés s'espacent : une minute après `revoked` ou `pin_mismatch`, délai doublé à chaque
nouveau refus, 30 minutes au plus ; 5 minutes après `rate_limited`.

### 6.3 Archive (`hub.history`, `hub.metrics`)

Le hub garde le journal de chaque agent confié et ses mesures minute par minute au-delà des 30 et 90 jours de
l'agent ([HUB.md §9](HUB.md#9-données-et-chiffrement-au-repos)) : il relit le journal (`history`) toutes les
10 minutes et à chaque retour en ligne, la liste des jours de mesures (`metrics`) toutes les heures, puis les jours
passés qui lui manquent, 5 par relevé (le jour en cours, qui s'écrit encore, attend le lendemain). Un évènement déjà
gardé n'est gardé qu'une fois ; la cause d'un arrêt anormal précisée plus tard par l'agent remplace l'évènement.

- **`hub.history`** `{"pc":…,"from":…}` : `history`, comme la réponse de l'agent ([PROTOCOLE.md](PROTOCOLE.md#journal-du-pc-history)) :
  `from` y est le plus ancien évènement gardé, `events` les évènements à partir du `from` demandé, dans l'ordre,
  2 000 au plus. `more` indique qu'il en reste : redemander avec `from` juste après le dernier reçu.
- **`hub.metrics`** `{"pc":…}` : `metrics.days`, les jours gardés ; `{"pc":…,"day":"2026-10-03"}` : `metrics.rows`,
  les lignes de ce jour (vides s'il n'est pas gardé), comme la commande `metrics` de l'agent.

Un PC retiré du hub emporte son archive.

### 6.4 Réglages (`hub.settings`)

`{}` lit les réglages, `{"retention":730}` change la durée de conservation de l'archive (en jours, de 90 à 1 095 ;
365 par défaut) ; la réponse porte `settings`. Les mois entièrement au-delà de cette durée sont retirés au démarrage
du hub puis toutes les 6 heures.

### 6.5 Évènements (`hub.events`)

`{"since":…}` : `events`, les évènements détectés par le hub depuis cette date (secondes Unix), dans l'ordre,
2 000 au plus (`more` : il en reste). Ce sont des **codes** : les applications rédigent le texte avec leurs propres
noms de PC, et n'affichent jamais de texte venu du hub (T38).

| `k` | Quand | `x`, `i` |
|---|---|---|
| `online` | Un PC ou appareil qui ne répondait plus répond de nouveau | |
| `offline` | Plus rien ne répond | |
| `agent_down` | Le PC est allumé mais refuse la connexion au port de l'agent depuis plus de 2 minutes | |
| `problem` | Un agent refuse le hub | `x` : `revoked`, `pin_mismatch`, `rate_limited` |
| `lost` | Arrêt inattendu noté par l'agent (survenu depuis que le PC est confié au hub), daté de l'arrêt ; précisé plus tard, il reparaît avec sa cause | `x` : `bsod`, `button`, `power`, `hardware`, ou absent |
| `disk` | Disque à surveiller, ou lecteur plein à 90 % | `x` : `health` ou `full` |
| `measure` | Mesure de gravité `crit` pendant plus de 5 minutes | `x` : identifiant de la mesure |
| `wake` | Paquet magique envoyé par le hub (`hub.wake`) | `i` : empreinte courte de l'appareil à l'origine, ou `local` |

Chaque épisode n'est signalé qu'une fois, jusqu'à ce qu'il cesse. Les évènements d'un PC retiré du hub ne sont plus
donnés. Le « réveil non demandé » viendra avec les notifications (2.1).

### 6.6 Réveil (`hub.wake`)

`{"pc":…}` : le hub envoie le paquet magique depuis le réseau local (diffusion limitée et diffusion dirigée de chaque
réseau privé du NAS, trois fois), ce qui permet de démarrer un PC même quand l'application n'est pas sur le même réseau
que lui (2.2). Le PC doit avoir une adresse MAC. Pour un PC avec agent, le hub signale ce réveil à l'agent (`wakes`)
dès qu'il répond : le journal du PC indique « démarrage demandé par » le hub.

## 7. Lien d'appairage

```
patronus://hub?v=1&n=<nom du hub>&h=<adresse>&p=<port>&a=<empreinte du hub>&c=<code>&r=<rôle>&e=<expiration>
```

Paramètres et règles du lien `v=2` d'un agent ([§7.1](PROTOCOLE-V2.md#71-lien-dappairage-v2)), à trois différences
près : début `patronus://hub?` (casse indifférente), `v=1`, port 9771 par défaut. `a` porte l'empreinte du hub ;
`r`, s'il est présent, est un rôle du hub (`view`, `wake`, `admin`). Le code est celui du
[§7.2](PROTOCOLE-V2.md#72-code-dappairage) : 8 caractères, à usage unique, 10 minutes par défaut. Le lien ne contient
aucun secret durable.

## 8. Administration sur le NAS

`patronus-hub pair` (dans le conteneur : `sudo docker exec patronus-hub patronus-hub pair`) se connecte au hub en marche
depuis `127.0.0.1`, avec **l'identité du hub lui-même** : seule une commande qui peut lire `/keys/identity.key`
l'obtient, et le hub ne l'accepte que depuis l'adresse de bouclage. Cette session a le rôle `admin` ; elle crée une
invitation (rôle `admin` par défaut, `--role` pour un autre) et affiche le lien et le code.

Au premier démarrage, tant qu'aucun administrateur n'est enregistré, le hub affiche lui-même un tel lien dans le journal
du conteneur. C'est le seul secret jamais écrit au journal : à usage unique, valable 10 minutes, lisible des seuls
administrateurs de DSM (T41).

## 9. Données du hub

Le registre des appareils et des invitations (`devices.db`, du code des invitations seul le condensé SHA-256), la
liste des PC confiés (`hub.db`), les réglages (`settings.db`) et l'archive de chaque PC (`pc/<id>/history-AAAA-MM.bin`,
`pc/<id>/metrics-AAAA-MM.bin`) sont gardés dans le stockage chiffré du hub
([HUB.md §9](HUB.md#9-données-et-chiffrement-au-repos)) ; un fichier illisible n'est jamais remplacé par un fichier
vide.
