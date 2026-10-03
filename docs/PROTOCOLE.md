# Protocole `wolagent/1`

Protocole entre l'application Android (client) et l'agent installé sur un PC (serveur).
Implémentations : `core/src/main/kotlin/…/agent/` (Kotlin) et `agent/protocol` + `agent/internal/server` (Go, agent) et `desktop/internal/agentclient` (Go, application Windows).
Les deux implémentations sont vérifiées contre les mêmes vecteurs de test : [`protocol/test-vectors.json`](../protocol/test-vectors.json)
(calculés indépendamment), et par un test de bout en bout en CI (client Kotlin ↔ agent Go réel).

## Transport

- TCP, port **9770** par défaut.
- Chaque message est **un objet JSON sur une ligne**, encodé en UTF-8 et terminé par `\n`. Taille maximale : **8 Kio**.
- Une connexion = un seul échange (requête / réponse), puis fermeture. Délai maximal côté agent : 10 s.
- Toutes les valeurs binaires sont encodées en **Base64 URL sans remplissage** (`A-Z a-z 0-9 - _`).

## Clé

Une clé symétrique de **32 octets aléatoires** par PC, générée par l'agent (`wol-agent install` / `rotate-key`) et transmise
au téléphone par QR code ou lien d'appairage. Elle n'est **jamais** envoyée sur le réseau pendant les échanges.

## Échange

```
App                                                     Agent
 │ ─────────────── connexion TCP ─────────────────────▶ │  (adresse source hors réseaux autorisés → fermeture)
 │ ◀── {"proto":"wolagent/1","nonce":N} ─────────────── │  N : 32 octets aléatoires, nouveau à chaque connexion
 │ ─── {"cnonce":C,"body":B,"mac":M} ────────────────▶ │  C : 16 octets aléatoires ; B : requête JSON (chaîne)
 │                                                      │  M = HMAC(clé, "wolagent/1\nrequest\n" + N + "\n" + C + "\n" + B)
 │ ◀── {"body":R,"mac":M2} ──────────────────────────── │  M2 = HMAC(clé, "wolagent/1\nresponse\n" + N + "\n" + C + "\n" + R)
```

HMAC = **HMAC-SHA256**. Les signatures sont calculées sur les **chaînes exactes** `B` et `R` (pas de canonicalisation JSON),
et comparées en temps constant.

- `N` (choisi par l'agent) empêche le **rejeu** d'une requête capturée : elle ne serait valable que pour une connexion passée.
- `C` (choisi par l'app) empêche le rejeu d'une **réponse** : l'app sait qu'elle parle au vrai agent (authentification mutuelle).

### Requête `B`

```json
{"cmd":"status"}
{"cmd":"history"}
{"cmd":"wakes","wakes":[1790563950],"by":"Pixel 8"}
{"cmd":"shutdown","delay":0,"force":false,"by":"Pixel 8"}
{"cmd":"reboot","delay":0,"force":true}
{"cmd":"sleep","delay":0}
```

| Champ | Description |
|---|---|
| `cmd` | `status`, `history` (journal, voir plus bas), `wakes` (démarrages demandés, agent 1.4.0 ou plus), `shutdown`, `reboot` ou `sleep` |
| `delay` | secondes avant l'action (0 – 3600). L'agent attend au minimum 1,5 s pour que la réponse parte d'abord. |
| `force` | fermer les applications sans attendre (Windows `/f`, Linux `--ignore-inhibitors`) |
| `by` | facultatif : nom de l'appareil qui envoie la demande (40 caractères au plus), noté au journal (agent 1.4.0 ou plus ; ignoré avant) |
| `wakes` | pour `wakes` : heures (secondes Unix) des démarrages demandés, 50 au plus |

### Réponse `R`

```json
{"ok":true,"code":"ok","message":"Extinction dans 0 s","hostname":"PC-BUREAU","os":"windows","arch":"amd64","version":"1.0.0","uptime":3600}
```

| Champ | Description |
|---|---|
| `ok` / `code` | `ok`, ou erreur : `forbidden` (commande désactivée), `unsupported`, `bad_request` |
| `message` | texte lisible |
| `hostname`, `os`, `arch`, `version` | informations sur le PC et l'agent (`os` = `windows`, `linux`, `darwin`) |
| `uptime` | secondes depuis le démarrage du système |
| `temperatures` | réponse à `status`, agent 1.5.0 ou plus, absent si aucun capteur n'est lisible : `cpu` et `gpu` en °C (arrondis au dixième, absents si illisibles), `gpuName` (nom de la carte), `gpuShared` = `true` quand la carte graphique est intégrée au processeur sans sonde à part et que `gpu` est la température de la puce, celle du processeur (agent 1.6.1), `cpuHint` = `lhm` quand la température du processeur manque parce que LibreHardwareMonitor ne la fournit pas (Windows), précisé par `lhm` (agent 1.6.0) : `not-running` (ne tourne pas), `web-off` (serveur web désactivé), `auth` (serveur web protégé par mot de passe), `no-sensor` (pas de capteur du processeur : pilote PawnIO absent…) |
| `history` | réponse à `history` uniquement (voir ci-dessous) |

### Journal du PC (`history`)

L'agent tient un journal local de **30 jours** (2 000 évènements au plus) : démarrages, arrêts et mises en veille
du PC, et commandes reçues. Les applications le relisent pour afficher l'historique, y compris ce qui s'est passé
pendant qu'elles étaient fermées. Commande en lecture seule, autorisée dès que `status` l'est.

```json
{"ok":true,"code":"ok","message":"Journal : 3 évènements","hostname":"PC-BUREAU","os":"windows","arch":"amd64","version":"1.2.0","uptime":3600,
 "history":{"from":1790000000,"events":[{"t":1790500000,"k":"boot"},{"t":1790510000,"k":"cmd","a":"shutdown","c":"192.168.1.37"},{"t":1790510004,"k":"shutdown"}]}}
```

| Champ | Description |
|---|---|
| `from` | début de la période couverte (secondes Unix) : installation de l'agent, ou 30 jours |
| `t` | heure de l'évènement (secondes Unix, horloge du PC) |
| `k` | `boot` démarrage, `shutdown` arrêt propre, `lost` arrêt non enregistré (coupure de courant, arrêt forcé, plantage ; daté du dernier signe de vie), `sleep` / `resume` mise en veille / sortie de veille, `cmd` commande reçue, `wake` démarrage demandé par une application |
| `a` | pour `cmd` : action (`shutdown`, `reboot`, `sleep`) |
| `c`, `b` | pour `cmd` et `wake` : adresse et nom (`by`, s'il a été indiqué) de l'appareil à l'origine de la demande |

La réponse peut dépasser la limite ordinaire d'une ligne : les clients acceptent jusqu'à **512 Kio** pour cette seule
commande. Un agent antérieur à la version 1.2 répond `forbidden` ou `unsupported` : les applications affichent alors
« agent à mettre à jour » et se contentent des changements d'état qu'elles constatent elles-mêmes.

### Démarrages demandés (`wakes`, agent 1.4.0 ou plus)

Le paquet magique ne passe pas par l'agent : il ne peut pas savoir qui a démarré le PC. Après chaque lecture du
journal, l'application lui signale donc les démarrages qu'elle a demandés et qu'il ne contient pas encore (à une
minute près). L'agent les ajoute (type `wake`, avec l'adresse et le nom de l'appareil) et répond comme à `history`,
avec le journal à jour. Il ignore les heures futures, celles qui précèdent le début de son journal de plus de
10 minutes (une demande précède le démarrage qu'elle provoque) et celles déjà connues à une minute près. Commande
autorisée dès que `status` l'est, comme la lecture du journal. Un agent plus ancien répond `forbidden` : les
applications n'insistent pas.

Ainsi, téléphone, PC Windows et personnes avec qui les PC sont partagés (droit « démarrer et éteindre ») voient le
même historique : qui a demandé quoi, et quand.

Règles d'affichage communes à Android et Windows (vérifiées par le scénario `protocol/history-vectors.json`) :
le journal de l'agent fait foi sur la période qu'il couvre (les allumages / extinctions constatés par l'application
pendant cette période sont masqués), et une demande notée par l'agent (commande ou démarrage) qui correspond à une
demande faite depuis l'application dans la minute n'est affichée qu'une fois ; celles des autres appareils sont
affichées avec leur nom (« par Pixel 8 »), à défaut leur adresse.

### Erreurs non signées

Quand l'authentification est impossible, l'agent répond sans signature puis ferme la connexion :

| Message | Cause |
|---|---|
| `{"error":"unauthorized"}` | signature invalide (mauvaise clé) |
| `{"error":"rate_limited"}` | 5 échecs en 1 minute depuis cette adresse : blocage 5 minutes (envoyé à la place du `hello`) |
| `{"error":"bad_request"}` | message illisible ou `cnonce` de mauvaise taille |

## Lien d'appairage

```
wolagent://pair?v=1&n=<nom>&h=<ip>&p=<port>&m=<mac>&k=<clé>
```

Valeurs encodées comme une URL (`application/x-www-form-urlencoded`, espace = `+`). `m` est facultatif.
Le lien contient la clé : il ne doit pas être partagé.

## Évolutions

Toute modification incompatible passera par un nouvel identifiant (`wolagent/2`) ; l'agent pourra accepter plusieurs versions
pendant une transition. Les champs JSON inconnus sont ignorés des deux côtés (ajouts compatibles possibles).
