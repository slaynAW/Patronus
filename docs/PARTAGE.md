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
2. **Se connecter à GitHub** (compte gratuit) : l'application affiche un code, à saisir sur `github.com/login/device`.
   L'application ne reçoit que le droit de gérer des **Gists** (petits fichiers) : aucun accès à vos dépôts.
3. Un Gist **secret** « Wake On LAN – partage chiffré » est créé : c'est là que seront déposés les fichiers d'accès.

### Donner un accès (exemple : Léa et le PC streaming)

1. Vous : *Partage* → **Inviter** : un QR code (ou un lien à envoyer par message) s'affiche. Il ne contient aucun secret.
2. Léa : *Réglages* → *Partage* → **Demander un accès** → scanne le QR code (ou colle le lien) et indique son prénom.
   Son application affiche à son tour un **QR code de demande** et un **code de vérification** (ex. `670 786`).
3. Vous : **Ajouter une demande** → scannez son QR code (ou collez le lien qu'elle vous a envoyé).
   **Vérifiez que le code affiché est le même que sur son téléphone** (de vive voix ou par téléphone) : c'est ce qui
   garantit que la demande vient bien de son appareil.
4. Choisissez pour chaque PC : *non partagé*, *démarrer* ou *démarrer et éteindre*, puis **Autoriser**.
5. L'application de Léa télécharge elle-même son accès (en général dans la minute) : le PC streaming apparaît
   avec la mention « partagé par … ». Les changements que vous faites ensuite (adresse, nom…) lui parviennent seuls.

### Ce que la personne autorisée peut faire

| | Démarrer | Démarrer et éteindre |
|---|---|---|
| Voir le PC et son état (allumé, éteint…) | ✅ | ✅ |
| Démarrer le PC | ✅ | ✅ |
| Éteindre, redémarrer, mettre en veille | ❌ (la clé de l'agent n'est pas envoyée) | ✅ |
| Modifier, exporter, partager à son tour | ❌ | ❌ |

### Retirer un accès

*Partage* → la personne → **Retirer l'accès** : son fichier est supprimé et les PC disparaissent de son application à
la vérification suivante. Si elle avait le droit d'éteindre, changez aussi la clé de l'agent des PC concernés
(`wol-agent rotate-key`, puis mettez la nouvelle clé dans votre application) : un appareil qui ne se connecte plus
garderait sinon la dernière version reçue.

### Changer de téléphone ou de PC

- **Personne qui partage** : l'export complet (protégé par mot de passe) contient votre clé de partage et la liste des
  personnes autorisées. Après l'import sur le nouvel appareil, reconnectez-vous à GitHub : les accès continuent.
  Sans cette sauvegarde, il faudrait inviter à nouveau tout le monde.
- **Personne qui reçoit** : la clé de réception ne quitte jamais l'appareil ; refaites une demande d'accès.

## Sécurité

| Menace | Protection |
|---|---|
| Quelqu'un télécharge les fichiers du Gist | Chaque fichier est chiffré (AES-256-GCM) pour la clé d'un seul appareil ; la clé privée de cet appareil ne le quitte jamais (Keystore Android, DPAPI Windows). |
| Quelqu'un installe l'application et « s'invite » | Impossible : un fichier d'accès doit être signé par la clé de la personne qui partage, qui ne quitte pas son appareil (sauf dans son export chiffré). |
| Une fausse invitation ou une fausse demande (message intercepté) | Code de vérification à comparer : il dépend des deux clés et diffère si l'une a été remplacée. |
| Une personne autorisée transmet son accès | Le fichier ne se lit qu'avec la clé de **son** appareil ; l'application ne permet ni d'exporter ni d'afficher les PC reçus hors de la liste, et n'affiche jamais la clé d'agent reçue. |
| Retour à une ancienne version d'un fichier (réaccorder un accès retiré) | Numéro de révision croissant, vérifié par l'appareil. |
| Vol du jeton GitHub de la personne qui partage | Droit « gist » uniquement : pas d'accès aux dépôts (donc pas au code de l'application). Le jeton est chiffré sur l'appareil et jamais exporté. |

Limites : une personne autorisée voit les PC qu'on lui partage (nom, adresse, adresse MAC) ; l'historique des
révisions du Gist conserve d'anciennes versions, chacune lisible seulement par l'appareil auquel elle était destinée ;
GitHub voit l'existence du Gist, sa taille et ses dates de mise à jour.

## Format (wakeonlan-share/1)

Vecteurs de test communs Windows / Android : [`protocol/share-vectors.json`](../protocol/share-vectors.json).

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
wolshare://request?v=1&name=<nom>&device=<clé de réception>&owner=<KeyID de la clé de signature>
```

Paramètres encodés en `application/x-www-form-urlencoded` (UTF-8). Noms : 1 à 40 caractères, sans caractère de
contrôle ni espace en début ou fin.

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
  pour un accès « démarrer »).

Ordre de vérification à la réception : format et version → signature avec la clé de l'invitation → `owner` identique →
`device` = sa propre clé → `revision` ≥ dernière reçue → déchiffrement → validation de chaque PC (mêmes règles qu'un
import). Un fichier absent du Gist signifie : demande pas encore acceptée, ou accès retiré.

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
   - *Application name* : `Wake On LAN`
   - *Homepage URL* et *Authorization callback URL* : `https://github.com/slaynAW/WakeOnLan` (l'adresse de retour
     n'est pas utilisée, mais GitHub l'exige)
   - cochez **Enable Device Flow**
3. **Register application**, puis copiez le *Client ID* (ne générez pas de *client secret* : il est inutile).
4. Mettez-le dans `gradle.properties` (`wol.githubClientId=…`) : la CI l'intègre à l'APK et à l'application Windows.

Sans identifiant, les applications restent capables de **recevoir** des accès ; seul le bouton « Partager mes PC » est
indisponible. Chaque personne qui partage autorise cette application OAuth une fois, sur son propre compte GitHub : le
propriétaire du dépôt n'obtient aucun accès aux comptes ni aux Gists des autres.
