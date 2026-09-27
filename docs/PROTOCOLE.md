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
{"cmd":"shutdown","delay":0,"force":false}
{"cmd":"reboot","delay":0,"force":true}
{"cmd":"sleep","delay":0}
```

| Champ | Description |
|---|---|
| `cmd` | `status`, `shutdown`, `reboot` ou `sleep` |
| `delay` | secondes avant l'action (0 – 3600). L'agent attend au minimum 1,5 s pour que la réponse parte d'abord. |
| `force` | fermer les applications sans attendre (Windows `/f`, Linux `--ignore-inhibitors`) |

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
