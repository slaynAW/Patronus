# Partage des PC entre personnes

Le partage permet de donner à une autre personne l'accès à **certains** de vos PC, par exemple à votre conjoint le
seul « PC streaming », en **démarrage seulement**. Les applications Android et Windows savent partager et recevoir.

- Seule la personne qui partage décide qui a accès, à quels PC et avec quels droits.
- Les fichiers déposés sur GitHub sont **chiffrés pour un seul appareil** et **signés** par la personne qui partage :
  personne d'autre ne peut les lire ni en fabriquer, même si l'application et son code sont publics.
- Il n'y a ni serveur central ni clé maître : chacun gère ses propres partages ; le créateur de l'application n'a
  accès à rien.

## Utilisation

### Partager ses PC (une seule fois : connexion GitHub)

1. *Réglages* → *Partage* → **Partager mes PC**, puis choisissez le nom affiché aux autres (ex. votre prénom).
2. **Se connecter à GitHub** (compte gratuit) : l'application affiche un code ; **Ouvrir GitHub** le copie et ouvre
   `github.com/login/device`, où il suffit de le coller puis de valider. Revenez ensuite dans l'application : la
   connexion se termine seule (même si le téléphone a coupé Internet à l'application pendant ce temps).
   L'application ne reçoit que le droit de gérer des **Gists** (petits fichiers) : aucun accès à vos dépôts.
3. Un Gist **secret** « Patronus – partage chiffré » est créé (« Wake On LAN – partage chiffré » s'il date d'avant la version 1.5.1) : c'est là que seront déposés les fichiers d'accès.

### Donner un accès (exemple : Léa et le PC streaming)

1. Vous : *Partage* → **Inviter** : un QR code (ou un lien à envoyer par message) s'affiche. Il ne contient aucun secret.
2. Léa : *Réglages* → *Partage* → **Demander un accès** → scanne le QR code (ou colle le lien) et indique son prénom.
   Son application affiche à son tour un **QR code de demande** et un **code de vérification** (ex. `670 786`).
3. Vous : **Ajouter une demande** → scannez son QR code (ou collez le lien qu'elle vous a envoyé).
   **Vérifiez que le code affiché est le même que sur son téléphone** (de vive voix ou par téléphone) : c'est ce qui
   garantit que la demande vient bien de son appareil.
4. Choisissez pour chaque PC : *non partagé*, *démarrer*, *démarrer et consulter* ou *démarrer et éteindre*, puis
   **Autoriser**.
5. L'application de Léa télécharge elle-même son accès (en général dans la minute) : le PC streaming apparaît
   avec la mention « partagé par … ». Les changements que vous faites ensuite (adresse, nom…) lui parviennent seuls.

### Ce que la personne autorisée peut faire

| | Démarrer | Démarrer et consulter | Démarrer et éteindre |
|---|---|---|---|
| Voir le PC et son état (allumé, éteint…) | ✅ | ✅ | ✅ |
| Démarrer le PC | ✅ | ✅ | ✅ |
| Lire l'historique, les mesures et la fiche du PC | ❌ | ✅ | ✅ |
| Éteindre, redémarrer, mettre en veille | ❌ | ❌ | ✅ |
| Modifier, exporter, partager à son tour | ❌ | ❌ | ❌ |

« Démarrer et consulter » demande le partage sans clé (ci-dessous). Avec « démarrer », l'application de la personne
ne contacte jamais l'agent du PC.

### Partage sans clé (applications 1.13)

Quand la personne utilise une application 1.13 ou plus récente, et que le PC est en connexion chiffrée (`wolagent/2`),
la clé de l'agent n'est plus transmise : le PC **enregistre l'appareil de la personne**, avec ses seuls droits, grâce
à une invitation réservée à cet appareil. Tout se fait seul, à une condition : votre téléphone ou votre ordinateur doit
être **administrateur** du PC (c'est le cas d'un appareil appairé avec le QR code de `wol-agent pair`).

- Dans *Partage*, chaque PC partagé de cette façon indique où il en est : « invitation envoyée » (l'appareil de la
  personne l'utilisera dès qu'il joindra le PC), « en attente du PC » (votre appareil la créera dès que le PC
  répondra), ou rien une fois l'appareil enregistré.
- « … doit en être administrateur » : votre appareil n'est pas administrateur de ce PC (appareil passé de `wolagent/1`
  à `wolagent/2`, par exemple). Sur le PC, `wol-agent devices role <empreinte> admin`, avec l'empreinte affichée par
  l'application ; ou depuis un appareil administrateur : *Appareils autorisés* → *Administrer*. En attendant, le PC
  reste partagé en « démarrer ».
- Une personne dont la demande vient d'une application plus ancienne reste en partage par clé (droits « démarrer » et
  « démarrer et éteindre » ; un PC passé en connexion chiffrée sans clé, comme un NAS appairé par QR code, ne se partage
  alors qu'en « démarrer ») : après la mise à jour de son application, elle vous renvoie sa demande (*PC partagés avec
  moi* → votre partage → *Afficher ma demande*), que vous ajoutez avec *Ajouter une demande*. Ses accès actuels sont
  gardés, et tous les droits deviennent possibles.

Du côté de la personne qui reçoit l'accès, rien à faire : sa demande porte l'empreinte de son appareil (sur Android,
à partir d'Android 10). Le PC apparaît d'abord en « démarrer » ; dès qu'il répond, son appareil utilise l'invitation
et le PC lui donne ses droits : état, historique et mesures pour « démarrer et consulter », extinction en plus pour
« démarrer et éteindre ». Si l'application indique « Demande modifiée en route », quittez ce partage (*Quitter ce
partage*) et refaites la demande : l'empreinte reçue n'était pas celle de l'appareil, et aucune invitation n'est
utilisée.

### Retirer un accès

*Partage* → la personne → **Retirer l'accès** : son fichier est supprimé et les PC disparaissent de son application à
la vérification suivante. Partagés sans clé, ces PC retirent aussi son appareil dès que le vôtre les joint
(« Retraits en attente » jusque-là) ; pour un effet immédiat : `wol-agent devices revoke` sur le PC. Si la personne
avait reçu la clé d'un agent (partage par clé, droit d'éteindre), changez aussi cette clé (`wol-agent rotate-key`,
puis mettez la nouvelle clé dans votre application) : un appareil qui ne se connecte plus garderait sinon la dernière
version reçue.

### Changer de téléphone ou de PC

- **Personne qui partage** : l'export complet (protégé par mot de passe) contient votre clé de partage, la liste des
  personnes autorisées et les retraits pas encore appliqués sur les PC. Après l'import sur le nouvel appareil,
  reconnectez-vous à GitHub : les accès continuent. Pour le partage sans clé, le nouvel appareil doit aussi être
  administrateur des PC. Sans cette sauvegarde, il faudrait inviter à nouveau tout le monde.
- **Personne qui reçoit** : la clé de réception ne quitte jamais l'appareil ; refaites une demande d'accès.

## Sécurité

| Menace | Protection |
|---|---|
| Quelqu'un télécharge les fichiers du Gist | Chaque fichier est chiffré (AES-256-GCM) pour la clé d'un seul appareil ; la clé privée de cet appareil ne le quitte jamais (Keystore Android, DPAPI Windows). |
| Quelqu'un installe l'application et « s'invite » | Impossible : un fichier d'accès doit être signé par la clé de la personne qui partage, qui ne quitte pas son appareil (sauf dans son export chiffré). |
| Une fausse invitation ou une fausse demande (message intercepté) | Code de vérification à comparer : il dépend des deux clés et diffère si l'une a été remplacée. |
| Une personne autorisée transmet son accès | Le fichier ne se lit qu'avec la clé de **son** appareil ; l'application ne permet ni d'exporter ni d'afficher les PC reçus hors de la liste, et n'affiche jamais la clé d'agent reçue. |
| Retour à une ancienne version d'un fichier (réaccorder un accès retiré) | Numéro de révision croissant, vérifié par l'appareil. |
| Une invitation (partage sans clé) lue ou interceptée | Réservée à l'identité de l'appareil destinataire : inutilisable par un autre appareil, même avec le code. Valable 7 jours au plus, à usage unique. |
| Accès retiré, mais l'appareil garde ses données | Partage sans clé : l'appareil est retiré du PC lui-même ; il n'a jamais eu de clé d'agent à réutiliser. |
| Vol du jeton GitHub de la personne qui partage | Droit « gist » uniquement : pas d'accès aux dépôts (donc pas au code de l'application). Le jeton est chiffré sur l'appareil et jamais exporté. |

Limites : une personne autorisée voit les PC qu'on lui partage (nom, adresse, adresse MAC) ; l'historique des
révisions du Gist conserve d'anciennes versions, chacune lisible seulement par l'appareil auquel elle était destinée ;
GitHub voit l'existence du Gist, sa taille et ses dates de mise à jour.

## Format (wakeonlan-share/1)

Vecteurs de test communs Windows / Android : [`protocol/share-vectors.json`](../protocol/share-vectors.json), et
[`protocol/share-v2-vectors.json`](../protocol/share-v2-vectors.json) pour le partage v2.

### Clés

- Personne qui partage : clé de **signature** ECDSA P-256.
- Chaque appareil qui reçoit : clé de **réception** ECDH P-256, créée sur l'appareil.
- Clés publiques : point non compressé (65 octets) en Base64 URL sans remplissage. Clés privées stockées : scalaire de
  32 octets, même encodage.
- `KeyID(clé)` : 32 caractères hexadécimaux = 16 premiers octets du SHA-256 du point non compressé.
- Code de vérification : 4 premiers octets (entier non signé, gros-boutiste) de
  `SHA-256("wakeonlan-share/1 verify" ‖ clé du propriétaire ‖ clé de l'appareil)`, modulo 1 000 000, sur 6 chiffres,
  affiché `123 456`.

### Liens

```
wolshare://invite?v=1&name=<nom>&owner=<clé de signature>&user=<compte GitHub>&gist=<identifiant du Gist>
wolshare://request?v=1&name=<nom>&device=<clé de réception>&owner=<KeyID de la clé de signature>[&id=<empreinte>]
```

Paramètres encodés en `application/x-www-form-urlencoded` (UTF-8). Noms : 1 à 40 caractères, sans caractère de
contrôle ni espace en début ou fin. Un paramètre en double rend le lien invalide ; les paramètres inconnus sont
ignorés. `id`, facultatif, sert au [partage v2](#partage-v2--appareils-enregistrés-par-le-pc).

### Fichier d'accès

Un fichier par appareil autorisé, nommé `acces-<KeyID(clé de réception)>.json`, dans le Gist :

```json
{ "format": "wakeonlan-share", "version": 1, "payload": "<Base64>", "signature": "<Base64>" }
```

- `signature` : ECDSA P-256 / SHA-256, encodage DER, sur `"wakeonlan-share/1\n" ‖ payload décodé`.
- `payload` (JSON) : `owner` (clé de signature), `device` (clé de réception), `revision` (entier croissant),
  `issuedAt` (date ISO 8601), `epk` (clé ECDH éphémère), `iv` (12 octets, Base64), `data` (Base64).
- Chiffrement : `secret = ECDH(éphémère, clé de réception)` (coordonnée x, 32 octets) ;
  `clé = HKDF-SHA256(secret, sel = epk ‖ clé de réception, info = "wakeonlan-share/1 access", 32 octets)` ;
  `data = AES-256-GCM(clé, iv, contenu, AAD = "wakeonlan-share/1" ‖ clé de signature ‖ clé de réception)`
  (étiquette de 16 octets à la fin).
- Contenu déchiffré (JSON) : `ownerName`, `recipientName`, `devices` (PC au format de la configuration ; sans `agent`
  pour un accès « démarrer »), et pour le [partage v2](#partage-v2--appareils-enregistrés-par-le-pc) `identity` et
  `invites`.

Ordre de vérification à la réception : format et version → signature avec la clé de l'invitation → `owner` identique →
`device` = sa propre clé → `revision` ≥ dernière reçue → déchiffrement → validation de chaque PC (mêmes règles qu'un
import). Un fichier absent du Gist signifie : demande pas encore acceptée, ou accès retiré.

### Partage v2 : appareils enregistrés par le PC

Avec le partage v2 (applications 1.13), les fichiers d'accès ne contiennent plus la clé `wolagent/1` des agents :
chaque PC enregistre l'appareil de la personne autorisée, avec un rôle à sa mesure, grâce à une invitation réservée à
cet appareil ([PROTOCOLE-V2.md, §10](PROTOCOLE-V2.md#10-partage-v2)). Le format `wakeonlan-share/1` ne change pas :
le contenu gagne deux champs facultatifs, que les versions précédentes ignorent. Vecteurs de test communs :
[`protocol/share-v2-vectors.json`](../protocol/share-v2-vectors.json).

**Droits** (`rights` de chaque personne) :

| Droit | Libellé | Partage v2 : rôle donné par l'agent | Partage v1 |
|---|---|---|---|
| `wake` | Démarrer | Aucun : l'agent n'est pas joint | Idem |
| `view` | Démarrer et consulter | `wake` : état, journal, mesures, fiche du PC, et ses démarrages notés au journal | Comme `wake` |
| `full` | Démarrer et éteindre | `control` | Clé de l'agent transmise |

**Demande d'accès** : le paramètre `id` porte l'empreinte `wolagent/2` de l'appareil qui demande (B64, 43 caractères,
sous sa forme canonique ; [PROTOCOLE-V2.md, §2.1](PROTOCOLE-V2.md#21-clés-et-empreintes)). Présent, il doit être
valide, sinon le lien est refusé. Le code de vérification ne le couvre pas, pour rester celui des versions
précédentes : l'appareil destinataire vérifie dans son fichier d'accès que l'empreinte reçue est bien la sienne.
Modifiée en route, elle ne donne donc rien à personne : les invitations seraient réservées à une autre identité, et
leur code n'est lisible que par l'appareil prévu.

**Contenu** :

```json
{"ownerName":"Hugo","recipientName":"Léa","identity":"<empreinte de l'appareil de Léa>",
 "devices":[{"id":"c","name":"Salon",…,"agent":{"port":9770,"key":"","pin":"<empreinte de l'agent>"}}],
 "invites":[{"device":"c","code":"7KQ29MXP","role":"control","expires":1790604800}]}
```

- `identity` : l'`id` de la demande ; absent en partage v1.
- `invites` : invitations en attente, réservées à `identity`. `device` désigne un PC de `devices` dont `agent` porte
  une empreinte (une invitation par PC au plus) ; `code` est un code d'appairage
  ([PROTOCOLE-V2.md, §7.2](PROTOCOLE-V2.md#72-code-dappairage)), `role` le rôle que donnera l'agent, `expires` la fin
  de validité (secondes Unix).

**Côté propriétaire**, pour chaque PC partagé avec la personne, dans l'ordre de la configuration :

1. `wake` : le PC sans `agent`.
2. `view` ou `full`, demande avec `id`, PC dont l'empreinte de l'agent est connue : **partage v2**.
   - Une invitation a été créée pour cette empreinte, ou l'appareil est enregistré par le PC : `agent` avec le port et
     l'empreinte, **sans clé**, et une entrée `invites` tant que l'invitation n'a ni servi ni expiré.
   - Sinon (invitation pas encore créée) : le PC sans `agent`. La clé n'est jamais transmise à la place.
3. Sinon, **partage v1** (demande sans `id`, ou agent qui ne connaît que `wolagent/1`) : pour `full`, `agent` avec le
   port et la clé quand l'application la connaît, sans empreinte ; sinon le PC sans `agent`.

L'application du propriétaire, quand elle est administratrice du PC
([PROTOCOLE-V2.md, §6.2](PROTOCOLE-V2.md#62-administration-rôle-admin)) :

- crée l'invitation : `invite` avec `device` = l'empreinte de la personne, le rôle du droit, `ttl` = 604 800 (7 jours)
  et le nom de la personne. Elle garde le code, chiffré sur l'appareil, pour le fichier d'accès ; il n'est ni exporté
  ni écrit dans les journaux ;
- la renouvelle tant qu'elle n'a pas servi (expirée, ou droit changé) ;
- constate l'enregistrement dans `devices` : le code est alors oublié et quitte le fichier d'accès ;
- aligne le rôle (`devices.role`) quand le droit passe de `view` à `full` ou l'inverse ;
- **retire** l'appareil quand l'accès est retiré, réduit à `wake`, ou demandé de nouveau avec une autre empreinte :
  elle annule les invitations réservées à cette empreinte (`invites.cancel`), puis envoie `devices.revoke`. Ces
  retraits attendent dans une file, gardée dans l'export complet, jusqu'à ce que l'application joigne le PC ; un PC
  accordé en v2 y figure même sans enregistrement connu (sauvegarde restaurée), car retirer un appareil absent ne
  change rien.

Sans le rôle `admin` sur le PC, l'application ne peut pas créer d'invitation : le PC reste partagé en « démarrer »
avec les personnes dont la demande porte une empreinte, jusqu'à ce que l'appareil du propriétaire en devienne
administrateur (`wol-agent devices role <empreinte> admin` sur le PC, ou depuis un appareil administrateur).

**Côté destinataire** :

- `identity` différente de sa propre empreinte : la demande a été modifiée en route. L'application n'utilise ni les
  `agent` portant une empreinte ni `invites` (ces PC restent en « démarrer ») et propose de refaire la demande.
- PC dont `agent` porte une empreinte : déjà enregistré par ce PC, l'appareil le joint en `wolagent/2`, comme ses
  propres PC. Sinon, il utilise l'invitation de ce PC (`pair`) dès qu'il le joint, si elle n'a pas expiré, une seule
  fois par code ; `already_paired` vaut succès. Sans enregistrement, il ne contacte pas l'agent : une identité
  inconnue compterait comme un échec ([PROTOCOLE-V2.md, §3.3](PROTOCOLE-V2.md#33-durées-et-limites)).
- Il ne garde que l'adresse, le port, la MAC et l'empreinte de l'agent, jamais de clé.

**Vérifications à la lecture**, en plus de celles du partage v1 ; une seule erreur fait refuser le fichier :
`identity` est une empreinte valide ; chaque invitation désigne un PC du contenu dont `agent` porte une empreinte,
une seule fois ; son code est valide (il est normalisé), son rôle connu, son expiration comprise entre 1 et
253 402 300 799 ; des invitations sans `identity` sont refusées.

### GitHub

- Connexion : flux OAuth « appareil » (`POST https://github.com/login/device/code`, droit `gist`), avec l'identifiant
  public de l'application OAuth du projet.
- Écriture : `POST /gists` (Gist secret), `PATCH /gists/{id}` (ajout, remplacement, suppression de fichiers).
- Lecture, sans compte : `GET https://api.github.com/gists/{id}` avec `If-None-Match` (les réponses 304 ne comptent pas
  dans la limite de 60 requêtes par heure).

## Mise en place (une seule fois, par le propriétaire du dépôt)

La connexion à GitHub depuis les applications utilise une **application OAuth** enregistrée au nom du dépôt. Son
identifiant (*Client ID*) est public ; aucun secret n'est nécessaire (flux « appareil »).

1. Sur GitHub : *Settings* → *Developer settings* → *OAuth Apps* → **New OAuth App**
   (<https://github.com/settings/applications/new>).
2. Renseignez :
   - *Application name* : `Patronus`
   - *Homepage URL* et *Authorization callback URL* : `https://github.com/slaynAW/Patronus` (l'adresse de retour
     n'est pas utilisée, mais GitHub l'exige)
   - cochez **Enable Device Flow**
3. **Register application**, puis copiez le *Client ID* (ne générez pas de *client secret* : il est inutile).
4. Mettez-le dans `gradle.properties` (`wol.githubClientId=…`) : la CI l'intègre à l'APK et à l'application Windows.

Sans identifiant, les applications restent capables de **recevoir** des accès ; seul le bouton « Partager mes PC » est
indisponible. Chaque personne qui partage autorise cette application OAuth une fois, sur son propre compte GitHub : le
propriétaire du dépôt n'obtient aucun accès aux comptes ni aux Gists des autres.
