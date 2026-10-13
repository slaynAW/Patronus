# Protocole `wolagent/2` (spécification)

> **Statut : en service depuis l'agent 1.11.0 et les applications 1.12.0 (étape 3 de la feuille de route), sauf le
> partage v2 (§10), prévu pour les applications 1.13.** Toute évolution de ce document s'accompagne des vecteurs de
> test. `wolagent/2` remplacera [`wolagent/1`](PROTOCOLE.md) après une période de transition (§11).

`wolagent/2` chiffre tous les échanges entre les applications et l'agent. Il donne à chaque appareil sa propre
identité, des droits (rôles) et une révocation individuelle. Il répond aux exigences R1 à R12 du
[modèle de menace](MODELE-DE-MENACE.md#7-exigences-de-sécurité).

- **Implémentation de référence** des fonctions pures (empreintes, codes, liens, preuves, rôles, trames) : paquet Go
  [`agent/protocol2`](../agent/protocol2) ; en Kotlin pour Android :
  [`core/…/agent2`](../core/src/main/kotlin/io/github/slaynaw/wakeonlan/core/agent2).
- **Vecteurs de test** communs Go et Kotlin : [`protocol/v2-vectors.json`](../protocol/v2-vectors.json).

Conventions :

- « doit » marque une obligation, « ne doit pas » une interdiction, « devrait » une recommandation.
- **B64** désigne le Base64 URL sans remplissage (`A-Z a-z 0-9 - _`).
- Les heures sont des secondes Unix, et le JSON est encodé en UTF-8.

| Exigence | Réponse |
|---|---|
| R1 chiffrement et authentification | TLS 1.3 uniquement (§3) |
| R2 authentification mutuelle sans autorité | Clés épinglées par leur empreinte (§2, §3) |
| R3 identité par appareil, clé non exportable | §2 |
| R4 rôles, liste blanche conservée | §5 |
| R5 révocation immédiate | §8 |
| R6 appairage sans secret durable | §7 |
| R7 migration sûre depuis `wolagent/1` | §9 |
| R8 pas de rétrogradation | §9, §11 |
| R9 limites | §3.3, §12 |
| R10 journal attribué | §8 |
| R11 pas de cryptographie maison | TLS de la plateforme ; deux HMAC à séparation de domaine (§9) |
| R12 vecteurs et cas négatifs | §12, §14 |

## 1. Vue d'ensemble

```
Application (identité D)                                   Agent (identité A)
  │── TCP vers le port 9770 ────────────────────────────────▶│ adresse dans un réseau autorisé ?
  │◀─ ligne de salut (comme en wolagent/1) ──────────────────│
  │── ClientHello (TLS 1.3, ALPN wolagent/2) ───────────────▶│ premier octet 0x16 : wolagent/2
  │◀─ certificat de A ── empreinte(A) = celle épinglée ? ────│
  │── certificat de D ──────────────────────────────────────▶│ D enregistré ? avec quel rôle ?
  │══ trames JSON chiffrées : requête → réponse, plusieurs fois ══│
```

Ce qui change par rapport à `wolagent/1` :

- **Chiffrement** : les requêtes et les réponses voyagent dans un tunnel TLS 1.3.
- **Identités** : chaque application, agent et hub a sa propre paire de clés, épinglée par l'autre côté. Il n'y a plus
  de secret partagé.
- **Rôles** : l'agent tient un registre des appareils autorisés, chacun avec son rôle, et le révoque un par un.
- **Appairage** : le QR code ne contient plus de secret durable. Un appareil déjà appairé en `wolagent/1` migre seul,
  sans rescanner.
- **Inchangé** : les commandes et leurs réponses (`status`, `history`, `metrics`, `specs`, `wakes`, `shutdown`,
  `reboot`, `sleep`).

## 2. Identités

### 2.1 Clés et empreintes

- Chaque **installation** d'une application (Android, Windows), chaque **agent** et chaque **hub** a une paire de clés
  **ECDSA P-256**. Elle est générée sur place et ne doit jamais quitter l'appareil (un nouveau téléphone a une nouvelle
  identité).
- **Empreinte** : `SHA-256(SubjectPublicKeyInfo en DER)`, soit 32 octets. C'est la valeur `pin-sha256` de la RFC 7469.
  Elle est écrite en B64 (43 caractères) dans les messages, les liens et les fichiers. Une valeur B64 n'a qu'une
  écriture admise : ni remplissage, ni retour à la ligne, ni bits de fin non nuls.
- **Empreinte courte** : les 60 premiers bits de l'empreinte (bits de poids fort d'abord), en base32 de Crockford
  (§7.2), groupés par quatre : `7KQ2-9MXP-4D1Z`. Elle ne sert qu'à la comparaison à l'œil (appairage manuel,
  `wol-agent devices`) et jamais d'identifiant dans le protocole.

### 2.2 Certificat

Chaque identité se présente dans TLS par un certificat X.509 v3 **autosigné** :

- sujet `CN=wolagent`, sans nom d'appareil ni aucune autre information personnelle ;
- numéro de série aléatoire (16 octets) ;
- validité : de la veille de sa création au 31/12/9999 ;
- signature ECDSA avec SHA-256.

Le certificat peut être régénéré à partir de la clé : l'empreinte ne change pas.

Chaque côté vérifie **uniquement** :

1. qu'un seul certificat est présenté ;
2. que sa clé est ECDSA P-256 sous sa forme standard : `SubjectPublicKeyInfo` de 91 octets (courbe nommée
   `prime256v1`, point non compressé), sur lequel est calculée l'empreinte ;
3. son empreinte (§3.2).

L'émetteur, les dates, les extensions et la signature du certificat sont ignorés : la possession de la clé est prouvée
par TLS (message `CertificateVerify`).

### 2.3 Stockage des clés

| Plateforme | Stockage | Remarque |
|---|---|---|
| Android | AndroidKeyStore : EC P-256, usage `SIGN`, condensé SHA-256 ; StrongBox si disponible, sinon TEE | Non exportable. Sans authentification de l'utilisateur, pour les sondes en arrière-plan. Le certificat généré par le Keystore sert de certificat TLS. |
| Application Windows | PKCS#8 chiffré par DPAPI (utilisateur) dans le dossier des données ; plus tard CNG/TPM | Exclue de l'export complet |
| Agent Windows | PKCS#8 dans le dossier de configuration (accès `SYSTEM` et Administrateurs), chiffré par DPAPI (machine) ; plus tard TPM | |
| Agent Linux, macOS | PKCS#8, fichier `0600` appartenant à `root` | |
| Hub (2.0) | Fichier `0600` dans un volume de clés séparé des données | |

`wol-agent rotate-identity` remplace la clé de l'agent. Tous les appareils doivent alors s'appairer à nouveau. Ce n'est
utile qu'en cas de doute sur la clé.

## 3. Transport

### 3.1 Port et détection de la version

- Même port TCP que `wolagent/1` (**9770** par défaut). La liste des réseaux autorisés (`allow`) est vérifiée en
  premier, comme aujourd'hui.
- L'agent envoie toujours sa **ligne de salut** en premier, comme en `wolagent/1` :
  - `{"proto":"wolagent/1","nonce":"…"}` si `wolagent/1` est accepté, sinon `{"proto":"wolagent/2"}` ;
  - `{"error":"rate_limited"}` à une adresse bloquée (§3.3), puis fermeture.
- Le **premier octet** du client choisit ensuite le protocole, sans délai d'attente : `{` (requête `wolagent/1`) ou
  `0x16` (début d'un `ClientHello` TLS : `wolagent/2`). Un autre octet, ou une requête `wolagent/1` quand celui-ci est
  désactivé : fermeture. Une simple sonde TCP (connexion puis fermeture) n'est pas un échec.
- Un client `wolagent/2` lit la ligne de salut (256 octets au plus, sans rien lire au-delà), puis ouvre TLS. Si c'est
  une erreur, il s'arrête et l'affiche. La ligne n'est pas authentifiée : elle ne sert qu'à cela.
- Pourquoi le salut d'abord : les clients `wolagent/1` attendent la parole de l'agent. Attendre de leur part un premier
  octet qui ne viendra pas ajouterait ce délai à chaque échange, et à la latence affichée par les applications.

### 3.2 TLS

| Paramètre | Valeur |
|---|---|
| Version | TLS 1.3 uniquement ; TLS 1.2 et antérieurs refusés |
| Suites | `TLS_AES_128_GCM_SHA256`, `TLS_AES_256_GCM_SHA384`, `TLS_CHACHA20_POLY1305_SHA256` |
| Échange de clés | `X25519MLKEM768` (hybride post-quantique) si les deux côtés le connaissent, sinon `X25519`, `secp256r1` |
| Signatures | `ecdsa_secp256r1_sha256` uniquement |
| ALPN | Le client propose exactement `wolagent/2` ; l'agent l'exige (sinon poignée de main refusée) |
| SNI | Non envoyé (agents désignés par leur adresse) ; ignoré s'il est présent |
| Certificat du client | Obligatoire. L'agent accepte au niveau TLS toute identité P-256, puis décide au niveau des commandes (§5). |
| Reprise de session, tickets | Désactivés : chaque connexion s'authentifie complètement, une révocation s'applique tout de suite |
| 0-RTT | Interdit (rejeu) |

Vérifications, avant tout envoi de données :

- **par le client** : profil du certificat (§2.2) et `empreinte(agent) = empreinte épinglée`. En cas d'écart, la
  connexion est fermée sans rien envoyer. Seule exception, l'étape 1 de la migration (§9) : l'empreinte n'est pas encore
  connue, et rien d'autre que l'échange de migration n'est envoyé avant la preuve de l'agent.
- **par l'agent** : profil du certificat et calcul de l'empreinte de l'appareil, puis recherche dans le registre (§8).

### 3.3 Durées et limites

| Limite | Valeur |
|---|---|
| Établissement de TLS | 10 s au plus |
| Inactivité | Fermeture par l'agent après 30 s sans requête ; 10 s pour une identité inconnue |
| Requêtes par connexion | 100 au plus |
| Connexions simultanées | 16, partagées avec `wolagent/1` |
| Échecs par adresse | 5 en une minute (TLS refusé par l'agent, identité inconnue hors appairage, `pair` ou `migrate` refusé), partagés avec `wolagent/1` : blocage de l'adresse 5 min. Un refus du client (empreinte différente) n'est pas compté. |
| Échecs d'appairage, toutes adresses | Plus de 20 `pair` refusés en 10 min : invitations en attente annulées et appairage suspendu 10 min (noté au journal) |

Les applications sondent leurs PC toutes les quelques secondes. Pour ne pas faire bloquer leur propre adresse, elles
espacent leurs demandes à un agent qui les refuse (`unknown_device`, clé `wolagent/1` refusée) : une minute après le
premier refus, délai doublé à chaque nouveau refus, 30 minutes au plus ; 5 minutes après `rate_limited`. Entre-temps,
une simple connexion TCP au port de l'agent, sans rien envoyer (l'agent ne la compte pas), dit si le PC est allumé. Un
changement des réglages du PC (nouvelle clé, nouvelle empreinte, autre adresse) ou une action réussie remet le délai à
zéro.

## 4. Trames et messages

### 4.1 Trame

Chaque message est une trame : **longueur** sur 4 octets (entier non signé, gros-boutiste), puis cette longueur
d'octets formant **un objet JSON** en UTF-8.

| | Longueur maximale |
|---|---|
| Requête | 16 Kio (16 384 octets) |
| Réponse | 4 Mio (4 194 304 octets) |

Une longueur nulle, une longueur au-delà du maximum ou une trame tronquée ferme la connexion sans réponse.

### 4.2 Requête

```json
{"id":1,"cmd":"status"}
{"id":2,"cmd":"shutdown","delay":0,"force":false}
{"id":3,"cmd":"metrics","day":"2026-10-03"}
```

- `id` : entier de 1 à 2 147 483 647, **strictement croissant** sur la connexion. Une requête illisible (JSON invalide
  ou qui n'est pas un objet) ou dont l'`id` est invalide reçoit `bad_request` (avec l'`id` 0 si elle est illisible),
  puis la connexion est fermée.
- `cmd` et les champs propres à chaque commande : ceux de `wolagent/1` (`delay`, `force`, `wakes`, `day`) et ceux des
  nouvelles commandes (§6).
- `by` est ignoré : le nom de l'appareil vient du registre de l'agent.
- Les champs inconnus sont ignorés.

### 4.3 Réponse

```json
{"id":1,"ok":true,"code":"ok","message":"","role":"control","hostname":"PC-BUREAU","os":"windows","arch":"amd64","version":"1.12.0","uptime":3600,"temperatures":{"cpu":48.5}}
```

- `id` est celui de la requête. Un client qui reçoit un autre `id` ferme la connexion.
- Les champs de `wolagent/1` sont conservés : `ok`, `code`, `message`, `hostname`, `os`, `arch`, `version`, `uptime`,
  `history`, `metrics`, `temperatures`, `disks`, `specs`.
- Nouveaux champs : `role` (rôle de l'appareil qui demande, dans chaque réponse à un appareil enregistré), `device`,
  `devices`, `invite`, `invites`, `nonce`, `proof` (§6, §9).
- À une identité **inconnue**, l'agent ne répond que `id`, `ok`, `code` et `message` : ni le nom du PC, ni le système,
  ni la version.
- Une seule requête à la fois : le client attend la réponse avant d'envoyer la suivante.

### 4.4 Codes de réponse

| `code` | Sens | Connexion |
|---|---|---|
| `ok` | Succès | Gardée |
| `bad_request` | Paramètre invalide (empreinte, rôle, durée…), appareil ou invitation introuvable ; requête illisible ou `id` invalide | Gardée ; fermée si la requête est illisible ou son `id` invalide |
| `unsupported` | Commande inconnue ou indisponible sur ce PC | Gardée |
| `busy` | À redemander dans un instant (fiche du PC pas encore lue) | Gardée |
| `forbidden` | Commande désactivée sur ce PC (liste blanche `commands`) | Gardée |
| `insufficient_role` | Le rôle de l'appareil ne permet pas cette commande | Gardée |
| `already_paired` | Cet appareil est déjà enregistré | Gardée |
| `last_admin` | L'opération retirerait le dernier administrateur | Gardée |
| `unknown_device` | Identité non enregistrée (jamais appairée, ou révoquée) | Fermée après la réponse |
| `pairing_failed` | Code invalide, expiré, déjà utilisé ou réservé à un autre appareil (même réponse dans tous les cas) | Fermée |
| `migration_failed` | Preuve invalide ou migration indisponible | Fermée |
| `rate_limited` | Trop d'échecs depuis cette adresse : envoyé à la place du salut, avant TLS (§3.1) | Fermée |

## 5. Rôles

Les rôles sont **cumulatifs** : chacun donne aussi les droits des rôles précédents.

| Rôle | Libellé | Permet en plus |
|---|---|---|
| `view` | Consulter | État, journal, mesures, fiche du PC |
| `wake` | Démarrer | Signaler les démarrages demandés (`wakes`). Le paquet magique, lui, ne passe pas par l'agent. |
| `control` | Contrôler | Éteindre, redémarrer, mettre en veille |
| `admin` | Administrer | Gérer les appareils autorisés et les invitations (puis les réglages) |

| Commande | Rôle minimal |
|---|---|
| `status`, `history`, `metrics`, `specs` | `view` |
| `wakes` | `wake` |
| `shutdown`, `reboot`, `sleep` | `control` |
| `devices`, `devices.revoke`, `devices.role`, `devices.rename`, `invite`, `invites`, `invites.cancel` | `admin` |
| `pair`, `migrate` | Aucun : réservées aux identités **inconnues** (appairage) |

Ordre des vérifications de l'agent :

1. Identité enregistrée ? Sinon `unknown_device`, sauf pour `pair` et `migrate`. Une identité enregistrée qui envoie
   `pair` ou `migrate` reçoit `already_paired`.
2. Commande connue (nom exact, casse comprise) ? Sinon `unsupported`.
3. Rôle suffisant ? Sinon `insufficient_role`.
4. Commande permise par la liste blanche du PC (`commands`) ? Sinon `forbidden`. Elle ne concerne que les commandes de
   `wolagent/1` ; comme avant, celles en lecture seule sont permises dès que `status` l'est.
5. Exécution.

Rôles par défaut :

| Cas | Rôle |
|---|---|
| QR code affiché sur le PC (`wol-agent pair`) | `admin` : la personne devant le PC en est l'administrateur |
| Invitation créée par un appareil administrateur | Au choix, `control` par défaut |
| Migration depuis `wolagent/1` | `control`, les droits d'une clé `wolagent/1` (§9) |
| Hub (2.0) | `wake` ; `control` seulement si une extinction programmée est activée pour ce PC |
| Partage « démarrer » | Pas d'enregistrement : paquet magique seulement, comme aujourd'hui |
| Partage « démarrer et éteindre » | `control` |
| Partage « consulter » (nouveau) | `view` |

## 6. Commandes

### 6.1 Commandes de `wolagent/1`

`status`, `history`, `wakes`, `metrics`, `specs`, `shutdown`, `reboot` et `sleep` gardent leurs champs et leurs
réponses ([PROTOCOLE.md](PROTOCOLE.md)). Le journal attribue désormais chaque action à l'identité de l'appareil (§8).

### 6.2 Administration (rôle `admin`)

**`devices`** : liste des appareils enregistrés.

```json
{"id":4,"cmd":"devices"}
{"id":4,"ok":true,"code":"ok","role":"admin","devices":[
  {"device":"<empreinte>","name":"Pixel 8","role":"admin","added":1790000000,"via":"pair","seen":1790500000,"self":true},
  {"device":"<empreinte>","name":"PC portable","role":"control","added":1790001000,"via":"migrate","seen":1790400000}]}
```

`via` vaut `pair` (code d'appairage), `migrate` (migration) ou `local` (ajout sur le PC). `seen` est la dernière
connexion. `self` désigne l'appareil qui demande.

**`devices.revoke`** `{"device":"<empreinte>"}` : retire l'appareil. Ses connexions ouvertes sont fermées sur-le-champ.
Un appareil peut se retirer lui-même, sauf s'il est le dernier administrateur (`last_admin`).

**`devices.role`** `{"device":"<empreinte>","role":"view"}` et **`devices.rename`**
`{"device":"<empreinte>","name":"Tablette"}` : changent le rôle ou le nom (40 caractères au plus, comme `by`).
Rétrograder le dernier administrateur est refusé (`last_admin`).

**`invite`** : crée une invitation à usage unique.

```json
{"id":5,"cmd":"invite","role":"control","name":"Pixel de Léa","ttl":604800,"device":"<empreinte, facultatif>"}
{"id":5,"ok":true,"code":"ok","role":"admin","invite":{"id":"<B64, 8 octets>","code":"7KQ29MXP","role":"control","name":"Pixel de Léa","expires":1790604800,"device":"<empreinte>","agent":"<empreinte de l'agent>"}}
```

- `role` : celui qui sera donné, `admin` au plus.
- `ttl` : durée de validité en secondes, de 60 à 604 800 (7 jours) ; 600 par défaut.
- `device` (facultatif) : réserve l'invitation à cette identité, qui seule pourra l'utiliser.
- `name` (facultatif) : le nom que portera l'appareil invité ; à défaut, celui qu'il donne en s'appairant.
- 10 invitations en attente au plus.
- L'agent ne garde que le **condensé SHA-256** du code. Le code n'apparaît que dans cette réponse.

**`invites`** liste les invitations en attente, **sans leur code** (`id`, `role`, `name`, `expires`, `device`).
**`invites.cancel`** `{"invite":"<id>"}` en annule une.

### 6.3 Appairage (identités inconnues)

**`pair`** `{"code":"7KQ29MXP","name":"Pixel 8"}`. L'agent normalise le code (§7.2), puis vérifie l'invitation : elle
existe, n'est ni utilisée ni expirée, et n'est pas réservée à une autre identité. Il enregistre alors l'appareil avec
le rôle et le nom prévus (sinon le nom donné ici), consomme l'invitation et répond :

```json
{"id":1,"ok":true,"code":"ok","role":"admin","device":"<empreinte de l'appareil>","hostname":"PC-BUREAU","os":"windows","arch":"amd64","version":"1.12.0","uptime":3600}
```

Un appareil déjà enregistré reçoit `already_paired`, et l'invitation n'est pas consommée.

**`migrate`** : voir §9.

## 7. Appairage

### 7.1 Lien d'appairage `v=2`

```
wolagent://pair?v=2&n=<nom>&h=<hôte>&p=<port>&m=<mac>&a=<empreinte de l'agent>&c=<code>&r=<rôle>&e=<expiration>
```

| Paramètre | Obligatoire | Valeur |
|---|---|---|
| `v` | oui | `2` |
| `h` | oui | Adresse IPv4, IPv6 (zone permise : `fe80::1%eth0`) ou nom d'hôte, mêmes règles que pour un PC ajouté à la main |
| `a` | oui | Empreinte de l'agent (B64, 32 octets) |
| `c` | oui | Code d'appairage (§7.2), avec ou sans tiret |
| `p` | non | Port : 1 à 5 chiffres, de 1 à 65 535 ; 9770 par défaut |
| `n` | non | Nom du PC, nettoyé (ci-dessous) ; à défaut, les 64 premiers caractères de l'hôte |
| `m` | non | Adresse MAC, pour le réveil : `AA:BB:CC:DD:EE:FF` ou `AA-BB-CC-DD-EE-FF`, casse indifférente, un seul séparateur ; ni `00:00:00:00:00:00` ni `FF:FF:FF:FF:FF:FF` |
| `r` | non | Rôle annoncé, en minuscules (`view`, `wake`, `control`, `admin`). Purement indicatif : l'agent applique celui de l'invitation. |
| `e` | non | Expiration (secondes Unix) pour l'affichage : 1 à 12 chiffres, de 1 à 253 402 300 799 (31/12/9999). L'agent applique la sienne. |

Lecture d'un lien, dans cet ordre :

1. Retirer les espaces, tabulations et retours à la ligne au début et à la fin. Le lien doit alors faire au plus
   **2 048 octets** et commencer par `wolagent://pair?`, en majuscules ou en minuscules (lettres ASCII seulement).
2. Découper la suite selon `&`, en ignorant les morceaux vides. Chaque morceau est `nom=valeur`, avec un nom non vide.
   La valeur est décodée comme un formulaire (`application/x-www-form-urlencoded` : `+` pour une espace, `%` suivi de
   deux chiffres hexadécimaux) et doit être de l'UTF-8 valide. Un **paramètre en double** rend le lien invalide ; les
   paramètres inconnus sont ignorés.
3. Vérifier les paramètres dans l'ordre `v`, `h`, `a`, `c`, `p`, `m`, `r`, `e`. Un paramètre présent doit être valide,
   même vide : seul `n` peut l'être.
4. Nettoyer le nom : retirer les caractères de contrôle (U+0000 à U+001F, U+007F à U+009F), puis les espaces (U+0020)
   au début et à la fin ; garder au plus 64 unités UTF-16 sans couper de caractère, et retirer les espaces de fin.

En cas d'erreur, le lien est refusé en entier ; l'implémentation de référence indique la cause : `format` (étapes 1 et
2), `duplicate`, ou le paramètre en cause. L'agent écrit les paramètres dans l'ordre du modèle ci-dessus, sans ceux qui
sont vides, et garde `:` tel quel dans les valeurs pour un QR code plus petit.

- Une application qui ne connaît que `v=1` affiche « version de lien non prise en charge ». Une application qui connaît
  les deux lit le paramètre `v` et choisit le lecteur.
- Le lien **ne contient aucun secret durable** : `a` est publique, et `c` ne sert qu'une fois, pendant quelques minutes.

### 7.2 Code d'appairage

- **Format** : 8 caractères de l'alphabet base32 de Crockford (`0123456789ABCDEFGHJKMNPQRSTVWXYZ`), soit 40 bits tirés
  par un générateur cryptographique, écrits bits de poids fort d'abord. Il s'affiche `7KQ2-9MXP`.
- **Normalisation** de la saisie :
  1. retirer les espaces (U+0020) et les tirets (U+002D) ;
  2. passer les lettres ASCII en majuscules ;
  3. remplacer `O` par `0`, et `I` et `L` par `1` ;
  4. exiger exactement 8 caractères de l'alphabet. `U`, une tabulation, une espace insécable ou tout caractère non
     ASCII rend le code invalide : il n'est jamais transformé (un `ı` ne devient pas `I`, donc `1`).
- 40 bits, au plus 5 essais par minute et par adresse et 20 essais en 10 minutes au total (§3.3) : deviner un code est
  hors de portée.

### 7.3 Avec le QR code affiché sur le PC

1. Sur le PC (droits d'administrateur) : `wol-agent pair [--role admin|control|wake|view] [--ttl 600]`. L'agent crée
   une invitation et affiche le QR code, le lien et son empreinte courte. Il le fait aussi à la fin de son installation.
   Pendant la coexistence (§11), `wol-agent pair --v1` affiche à la place l'ancien lien `v=1`, avec la clé
   `wolagent/1`, pour les applications qui ne connaissent que lui.
2. L'application scanne, ouvre TLS, vérifie que l'empreinte de l'agent est `a`, puis envoie `pair` avec le code et son
   nom d'appareil.
3. L'agent enregistre l'appareil (§6.3) et note `device added` au journal.
4. L'application garde l'adresse, le port, la MAC, l'empreinte de l'agent et son rôle. Aucun autre secret que sa propre
   clé, qui reste dans le Keystore.

### 7.4 Sans caméra (saisie manuelle)

L'utilisateur tape l'adresse du PC et le code affiché par `wol-agent pair`. L'application se connecte et affiche
l'**empreinte courte** de l'agent, que l'utilisateur compare à celle affichée sur le PC avant de confirmer.
L'application épingle alors l'empreinte complète. Cette méthode repose sur la comparaison faite par l'utilisateur : le
QR code est à privilégier.

### 7.5 Depuis un appareil administrateur

L'application administrateur envoie `invite`. Elle affiche ensuite le QR code du lien (son empreinte d'agent, son
adresse, le code) pour qu'un autre appareil le scanne, ou le transmet par le partage (§10).

### 7.6 Code déjà utilisé

Si l'appairage échoue alors que le code vient d'être affiché, quelqu'un d'autre l'a peut-être utilisé. L'application
le signale : « Ce code a déjà servi. Si ce n'est pas vous, retirez l'appareil inconnu de la liste des appareils du
PC. » Cet appareil apparaît dans `devices` et dans le journal.

## 8. Registre des appareils et révocation

L'agent tient `devices.json` dans son dossier de configuration (accès réservé à `root` / `SYSTEM`). Chaque entrée
contient :

| Champ | Contenu |
|---|---|
| `device` | Empreinte de l'appareil |
| `name` | Nom de l'appareil |
| `role` | Rôle |
| `added`, `via` | Date et moyen de l'ajout |
| `by` | Empreinte de l'administrateur à l'origine de l'invitation, ou `local` |
| `seen` | Dernière connexion (écrite sur le disque au plus toutes les 10 minutes) |

Le même fichier garde les invitations en attente : `id`, condensé SHA-256 du code (`digest`), `role`, `name`,
`expires`, `device` et `by`. Un registre illisible n'est jamais remplacé en silence : `wolagent/2` reste désactivé
jusqu'à sa réparation (l'agent continue en `wolagent/1`).

- **Révocation** : l'entrée est retirée. Les connexions ouvertes de cette identité sont fermées immédiatement, et ses
  requêtes suivantes reçoivent `unknown_device`. Aucune autre clé ne change.
- **Dernier administrateur** : par le réseau, impossible de le retirer ou de le rétrograder (`last_admin`). Sur le PC,
  c'est possible après confirmation.
- **Sur le PC**, avec les droits d'administrateur. `wol-agent` se connecte alors au service depuis l'adresse de
  bouclage, avec **l'identité de l'agent lui-même** : seul un administrateur du PC peut lire sa clé, et l'agent ne
  l'accepte que depuis l'adresse de bouclage. Cette session a le rôle `admin` et peut, après confirmation
  (`"force":true`), retirer ou rétrograder le dernier administrateur. Service arrêté, la commande modifie directement
  le registre, relu au démarrage.
  - `wol-agent devices` : liste, avec les empreintes courtes ;
  - `wol-agent devices revoke <début d'empreinte>` ;
  - `wol-agent devices role <début d'empreinte> <rôle>` ;
  - `wol-agent devices rename <début d'empreinte> <nom>` ;
  - `wol-agent pair`.
- **Journal** :
  - nouveaux évènements `{"k":"device","a":"added|revoked|role|renamed","b":"<nom>","i":"<empreinte courte>","x":"<rôle>"}` ;
  - les évènements `cmd` et `wake` portent aussi `i`, l'empreinte courte de l'appareil authentifié.

## 9. Migration depuis `wolagent/1`

Un appareil déjà appairé en `wolagent/1` passe à `wolagent/2` sans rescanner, quand :

- l'application connaît l'adresse, le port et la clé `K` du PC ;
- l'agent connaît `wolagent/2`. Les agents qui le connaissent ajoutent à leurs réponses `wolagent/1`, qui sont signées
  par `K`, la liste `"protocols":["wolagent/1","wolagent/2"]` et leur empreinte `"pin"`.

L'application procède en trois temps, sans rien demander :

1. elle lit l'empreinte de l'agent dans une réponse `wolagent/1` : signée par `K`, elle est authentique ;
2. elle ouvre une session `wolagent/2` où cette empreinte est exigée. Si l'agent connaît déjà son identité
   (migration interrompue, configuration restaurée sur la même installation), elle passe à l'étape 3 ; sinon
   (`unknown_device`), elle migre, comme ci-dessous ;
3. elle enregistre l'empreinte et efface `K`, en une seule écriture. Une interruption avant laisse le PC en
   `wolagent/1` : l'essai suivant reprend, et l'étape 2 reconnaît un enregistrement déjà fait.

Une application qui ne connaît pas encore l'empreinte (agent qui ne l'annonce pas) peut migrer sans elle : elle
retient alors l'empreinte vue dans sa poignée de main, que la preuve `Pa` authentifie.

```
Application                                                          Agent
 │══ TLS 1.3 (ALPN wolagent/2) : l'application présente son identité ═══▶│
 │   et vérifie l'empreinte de l'agent (ou la retient, §9 ci-dessus)      │
 │── {"id":1,"cmd":"migrate","nonce":Nc,"name":"Pixel 8"} ──────────────▶│
 │◀─ {"id":1,"ok":true,"code":"ok","nonce":Na,"proof":Pa} ───────────────│
 │   vérifie Pa : sinon fermeture, aucun autre envoi                       │
 │── {"id":2,"cmd":"migrate","proof":Pd} ─────────────────────────────────▶│ vérifie Pd
 │◀─ {"id":2,"ok":true,"code":"ok","role":"control","device":"<D>"} ──────│ enregistre D
```

`Nc` et `Na` font 32 octets aléatoires chacun (B64). Les empreintes entrent dans le calcul en octets bruts (32) :

```
Pa = HMAC-SHA256(K, "wolagent/2 migrate agent"  ‖ 0x00 ‖ empreinte(agent) ‖ empreinte(appareil) ‖ Nc ‖ Na)
Pd = HMAC-SHA256(K, "wolagent/2 migrate device" ‖ 0x00 ‖ empreinte(agent) ‖ empreinte(appareil) ‖ Nc ‖ Na)
```

Chaque côté calcule les preuves avec les empreintes **telles que vues dans sa propre poignée de main TLS**, et compare
en temps constant.

**Pourquoi c'est sûr :**

- **Homme du milieu.** Un intermédiaire sans `K` ne peut pas calculer `Pa`. S'il relaie vers le vrai agent, il présente
  son propre certificat à l'application, et sa propre identité à l'agent : les empreintes diffèrent d'un côté à
  l'autre, et la preuve de l'agent ne passe pas chez l'application.
- **Réflexion.** Les libellés distincts empêchent de renvoyer une preuve comme l'autre.
- **Autre protocole.** Les messages signés par `K` en `wolagent/1` commencent tous par `wolagent/1` suivi d'un retour à
  la ligne, les preuves par `wolagent/2 migrate` : une signature de l'un ne vaut jamais preuve dans l'autre.
- **Fraîcheur.** Les deux nonces rendent chaque preuve unique.
- **Force brute.** Un échec compte dans le limiteur (§3.3).
- **Empreinte annoncée.** Un intermédiaire sans `K` ne peut ni la modifier ni la remplacer dans la réponse
  `wolagent/1`, signée par `K` ; la preuve `Pa` la confirme ensuite dans la session `wolagent/2`.

**Ensuite :**

- l'application garde l'empreinte de l'agent et ne parle plus jamais `wolagent/1` à ce PC (R8). Elle efface `K` de sa
  configuration, **sauf** tant qu'elle partage ce PC avec le droit « démarrer et éteindre » : ce partage
  (`wolagent/1`) transmet `K` aux personnes autorisées. `K` ne sert alors plus qu'à ce partage, jusqu'au partage v2
  (§10), et elle est effacée dès que le partage est retiré.
- L'appareil migré reçoit le rôle `control` : les droits qu'avait sa clé. Le rôle `admin` ne s'obtient jamais par
  migration, seulement sur le PC (`wol-agent pair` ou `wol-agent devices role`).
- Une application qui n'a plus `K` et que l'agent ne reconnaît plus (`unknown_device` : appareil retiré) ne se
  réenregistre jamais d'elle-même : il faut l'appairer à nouveau.
- Une nouvelle clé saisie pour ce PC (lien d'appairage collé après une réinstallation de l'agent) fait oublier
  l'empreinte : le passage reprend depuis cette clé, qui authentifie l'empreinte de l'agent réinstallé.
- Une application ne migre que ses propres PC. Ceux qu'elle a reçus d'une autre personne restent en `wolagent/1`
  (leur clé vient du partage, sans l'empreinte de l'agent) jusqu'au partage v2.

**Qui peut migrer.** Tout détenteur de `K` : les appareils du propriétaire, et les personnes ayant un partage « démarrer
et éteindre ». C'est cohérent avec leurs droits en `wolagent/1`. Chaque migration est notée au journal et visible dans
`devices`.

**Fin de la transition.** L'administrateur désactive `wolagent/1` (`"protocols":["wolagent/2"]`). L'agent efface alors
`K`, qui ne sert plus à rien, et refuse `migrate` (`migration_failed`).

## 10. Partage v2

- Le fichier d'accès (`wakeonlan-share/1`, chiffré pour l'appareil destinataire) ne contient plus la clé de l'agent.
  Pour les droits « consulter » et « démarrer et éteindre », il contient une **invitation** :
  `{"host","port","mac","agent","code","role","expires"}`.
- L'application du propriétaire (administrateur du PC) la crée par `invite`, **réservée à l'identité `wolagent/2` du
  destinataire** (`device`), valable 7 jours et renouvelée tant qu'elle n'a pas servi. L'empreinte du destinataire
  arrive par sa demande d'accès, qui gagne un paramètre `id` : `wolshare://request?…&id=<empreinte>`.
- L'application du destinataire utilise l'invitation (`pair`) dès qu'elle joint le PC. Elle ne garde ensuite que
  l'adresse, le port, la MAC et l'empreinte de l'agent.
- Réservée à une identité, une invitation divulguée ne sert à personne d'autre.
- **Retirer l'accès** : l'application du propriétaire envoie `devices.revoke` aux PC concernés, et supprime le fichier
  d'accès. La révocation prend effet dès qu'un appareil administrateur joint le PC, ou tout de suite avec
  `wol-agent devices revoke`. Le hub (2.0) l'appliquera sans attendre.
- **En attendant** : le partage `wolagent/1` continue de transmettre `K` pour le droit « démarrer et éteindre »,
  jamais l'empreinte de l'agent. Un PC dont l'application a effacé `K` ne se partage qu'avec le droit « démarrer ».

## 11. Coexistence et calendrier

| Étape | Agent | Applications |
|---|---|---|
| 3 (première version `wolagent/2`) | `wolagent/1` et `wolagent/2` sur le même port ; `"protocols":["wolagent/1","wolagent/2"]` par défaut | `wolagent/2` pour les PC dont elles connaissent l'empreinte ; migration automatique des autres quand l'agent le permet ; `wolagent/1` pour les agents pas encore à jour |
| 2.0 | `wolagent/1` désactivé par défaut sur les nouvelles installations | Refus de `wolagent/1` pour tout PC passé en `wolagent/2` |
| Ensuite | `wolagent/1` retiré | Android 10 au minimum (§13) |

Les agents se mettent à jour d'eux-mêmes (mises à jour intégrées), ce qui raccourcit la transition.

## 12. Comportements de sécurité imposés

Cas négatifs à couvrir par des tests automatisés (étape 3), côté Go et côté Kotlin :

1. TLS 1.2 ou antérieur : refusé.
2. ALPN absent ou différent : refusé.
3. Pas de certificat client, clé autre que P-256, plusieurs certificats : refusé.
4. Empreinte de l'agent différente de celle épinglée : le client ferme sans rien envoyer.
5. Identité inconnue et commande autre que `pair` ou `migrate` : `unknown_device`, fermeture, échec compté.
6. Identité révoquée : `unknown_device`. Ses connexions ouvertes sont fermées au moment de la révocation.
7. Rôle insuffisant : `insufficient_role`. Commande hors liste blanche : `forbidden`.
8. Code invalide, expiré, déjà utilisé ou réservé à une autre identité : `pairing_failed`, la même réponse dans tous les
   cas. Puis blocage par adresse et suspension globale (§3.3).
9. Preuve de migration fausse : `migration_failed`. Homme du milieu simulé (empreintes différentes) : détecté par
   l'application.
10. Trame de longueur nulle, trop longue ou tronquée : fermeture. JSON invalide ou qui n'est pas un objet : `bad_request`
    et fermeture.
11. `id` non croissant : `bad_request` et fermeture.
12. Rétrogradation : une application qui connaît l'empreinte d'un PC ne tente jamais `wolagent/1` avec lui.
13. Dernier administrateur protégé.
14. Fuzzing (natif en Go, tests de propriétés en Kotlin) : lecture des trames, requêtes, liens d'appairage,
    normalisation des codes. Les cibles Go des fonctions pures existent déjà (`agent/protocol2`, `FuzzReadFrame`,
    `FuzzParseLink`, `FuzzNormalizeCode`, `FuzzParsePin`).

## 13. Décisions

Prises le 12/10/2026, avant l'étape 3 :

1. **Android 8 et 9** (API 26 à 28) n'ont pas TLS 1.3 natif. Aucune bibliothèque TLS n'est embarquée (Conscrypt :
   environ 3 Mo et des bibliothèques natives en plus). Ces appareils restent en `wolagent/1` avec les PC qui l'acceptent ;
   la version minimale de l'application passera à Android 10 (API 29, sorti en 2019) quand `wolagent/2` deviendra
   obligatoire, au retrait de `wolagent/1` (§11).
2. **Rôle des appareils migrés** : `control`, les droits qu'avait leur clé (§9). Donner `admin` au premier appareil
   migré aurait permis à une simple clé `wolagent/1` de faire de son détenteur un administrateur.
3. **Durée maximale d'une invitation** : 7 jours (§6.2), pour le partage comme pour toute autre invitation.
4. **Clés Windows** : chiffrées par DPAPI à l'étape 3 (§2.3) ; le passage au TPM (CNG) viendra plus tard.
5. **Rôle par défaut de `wol-agent pair`** : `admin`, la personne devant le PC en étant l'administrateur (§5).

## 14. Vecteurs de test

[`protocol/v2-vectors.json`](../protocol/v2-vectors.json) est généré par le paquet [`agent/protocol2`](../agent/protocol2)
(`go test ./protocol2 -run TestVectors -update`, entrées fixes : le fichier ne change pas d'une génération à l'autre).
À chaque build, la CI vérifie :

- le paquet Go contre le fichier (`go test`) ;
- le fichier contre une seconde implémentation, écrite d'après cette spécification et non d'après le code Go :
  [`check-v2-vectors.py`](../.github/scripts/check-v2-vectors.py) (Python et OpenSSL).

Il couvre :

| Section | Contenu |
|---|---|
| `pins` | Certificats P-256 avec leur empreinte et leur empreinte courte, dont deux certificats différents d'une même clé (même empreinte) ; certificats RSA, P-384 et Ed25519 refusés (`key_type`) ; certificat illisible, tronqué, ou dont le point de la clé n'est pas sur la courbe (`certificate`) |
| `codes` | Normalisation des codes saisis, affichage, codes invalides |
| `links` | Liens `v=2` valides (champs attendus ; `canonical` : écrit par l'agent, le réécrire redonne le même texte), invalides (cause), expiration à une date donnée |
| `migration` | Preuves `Pa` et `Pd` pour des clés, empreintes et nonces donnés |
| `roles` | Rôle minimal de chaque commande, et la réponse pour chaque rôle et pour une identité inconnue, commandes inconnues comprises |
| `frames` | Flux de trames : messages lus puis fin du flux (`eof`, `empty`, `too_large`, `truncated`) |

L'implémentation Kotlin (`core`, paquet `agent2`) est vérifiée contre les mêmes vecteurs (`Wolagent2Test`), comme
pour `wolagent/1`.

## Annexe : pourquoi ces choix

- **TLS 1.3 de la plateforme** plutôt qu'un protocole sur mesure : des implémentations auditées (BoringSSL, bibliothèque
  standard de Go), la confidentialité persistante, et aucune cryptographie maison (R11).
- **Clés autosignées épinglées** plutôt qu'une autorité de certification : il n'y a pas d'autorité de confiance dans
  une maison. L'empreinte de la clé publique est le mécanisme standard d'épinglage (RFC 7469).
- **Code à usage unique** plutôt qu'un PAKE (échange de clés authentifié par mot de passe) : aucun PAKE n'existe dans
  les bibliothèques des plateformes. Le code voyage dans un tunnel déjà authentifié par l'empreinte lue dans le QR code.
- **Même port** que `wolagent/1` : aucune nouvelle règle de pare-feu, et une détection par le premier octet.
