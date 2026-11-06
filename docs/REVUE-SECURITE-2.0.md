# Revue de sécurité du hub (2.0)

Revue adverse du hub Patronus avant la préversion 2.0.0-beta ([HUB.md, §15, décision 4](HUB.md#15-décisions-à-prendre)) :
lecture critique de tout le code du hub et du code qu'il partage avec l'agent, puis fuzzing guidé par des propriétés
de sécurité. Ce n'est **pas une revue indépendante** : elle a été menée par l'assistant qui a écrit le code. Elle
réduit le risque avant un usage personnel de la préversion, mais une revue externe reste souhaitable avant une
diffusion large.

Base : `main` au commit `f0dfdb7` (agent 1.14.0, hub 2.0 complet), novembre 2026. Les corrections sont dans la même
PR que ce document.

## 1. Périmètre

| Partie | Code | Questions posées |
|---|---|---|
| Serveur `patronus-hub/1` | `agent/internal/hub/server`, `agent/hubprotocol` | Qui peut faire quoi ? Un appareil peut-il dépasser son rôle, une identité inconnue obtenir quoi que ce soit ? Limites de taille, de temps, de nombre |
| Appairage | `agent/internal/devices`, `agent/internal/guard`, `agent/cmd/patronus-hub` | Code devinable ? Réponses qui renseignent ? Que contient le journal du conteneur ? |
| Hub → agents | `agent/internal/hub/agents`, `agent/protocol2/client` | Épinglage de l'empreinte, rôle du hub sur les PC, réseaux joignables |
| Surveillance, sondes | `agent/internal/hub/monitor` | Le hub peut-il servir à sonder ailleurs (SSRF) ? Que fait-il des réponses ? |
| Programmations | `agent/internal/hub/schedule` | Extinction détournée, occurrences en trop ou manquées, fuseaux horaires |
| Stockage chiffré | `agent/internal/hub/store`, `archive`, `fleet`, `settings` | Confidentialité, intégrité, comportement face à un dossier de données manipulé |
| Conteneur | `.github/scripts/hub-image.py`, [HUB-SYNOLOGY.md](HUB-SYNOLOGY.md) | Privilèges, système de fichiers, ce que l'image contient |

## 2. Constats et corrections

| # | Gravité | Constat | Correction |
|---|---|---|---|
| R1 | Moyenne | **Stockage : les liens symboliques étaient suivis.** Qui peut écrire dans le dossier des données (partage mal réglé, [HUB.md §9.3](HUB.md#93-contre-quoi-protège-ce-chiffrement)) pouvait y placer un lien vers `/keys`. Le hub aurait alors écrasé la clé des données (fichier temporaire `hub.db.tmp` ouvert en troncature) ; avec un dossier `pc/<id>` pointant vers `/keys`, « Retirer du hub » aurait supprimé `data.key` et `identity.key`. Résultat : historique illisible, hub à réappairer partout | Chaque opération du stockage passe par un `os.Root` (Go 1.24+) : un lien qui sort du dossier des données est refusé, même posé pendant l'opération. Fichier temporaire créé en `O_EXCL` après retrait d'un reste. Journal tronqué par le descripteur déjà relu. Test `TestSymlinksStayInside`, qui échoue sur l'ancien code |
| R2 | Faible | **Programmations : jour faux quand l'heure d'été commence à minuit** (Santiago, La Havane, Asunción…). Go ramène une heure inexistante (« 0 h 30 ») sur la veille ; le jour de la semaine était lu après ce décalage. Une programmation du samedi à 0 h 30 se déclenchait aussi le samedi à 23 h 30, et une programmation du dimanche à 0 h 30 sautait une semaine (aucune occurrence pendant 7 jours). `Previous`, qu'utilise le planificateur, avait le même défaut | Le jour se lit sur le calendrier (à midi, jamais pendant un changement d'heure), et l'occurrence retenue est la plus proche parmi la veille, le jour même et les jours suivants. Trouvé par la propriété de `FuzzNext`, qui garde ces cas parmi ses exemples |
| R3 | Faible | **Sonde web : en-têtes de réponse jusqu'à 10 Mo** (valeur par défaut de Go). Un appareil sondé pouvait faire grossir la mémoire du conteneur, limitée à 128 Mo | En-têtes bornés à 64 Kio ; au-delà, l'appareil compte comme ne répondant pas. Test `TestProbeHeaderLimit` |
| R4 | Faible | **Limiteur des échecs (agent et hub) : mémoire non bornée.** Les adresses en échec et les blocages expirés n'étaient jamais purgés. Des échecs venus d'un grand nombre d'adresses (IPv6 du réseau local) la faisaient grossir sans fin | Purge régulière des entrées périmées. Au-delà de 65 536 adresses en échec à la fois, leurs compteurs repartent de zéro ; les blocages en cours sont gardés, et la suspension globale de l'appairage reste. Test `TestLimiterMemory` |
| R5 | Information | `hub.db` relu avec deux PC du même identifiant (accessible seulement avec la clé des données) | Refusé comme incohérent, comme le registre des appareils |
| R6 | Documentation | [HUB.md §9.2](HUB.md#92-format) disait qu'un enregistrement retiré rend un journal illisible : c'est vrai au milieu, pas à la fin. Une troncature ne se distingue pas d'un ajout interrompu par une coupure de courant | §9.2 précise que le chiffrement empêche de lire et de falsifier, pas d'effacer |
| R7 | Documentation | Rien ne date les fichiers entiers. Restaurer une sauvegarde de `/data` rétablit les appareils autorisés de sa date, y compris un appareil retiré depuis | [HUB.md §9.3](HUB.md#93-contre-quoi-protège-ce-chiffrement) et [HUB-SYNOLOGY.md §9](HUB-SYNOLOGY.md#9-à-savoir) demandent de revoir les appareils autorisés après une restauration ; menace T46 du [modèle de menace](MODELE-DE-MENACE.md) |

## 3. Vérifié, sans correction nécessaire

- **Authentification** :
  - TLS 1.3 seulement, ALPN `patronus-hub/1` exigé, un seul certificat P-256 dont l'empreinte est calculée par le hub.
  - Tickets de session désactivés : chaque connexion s'authentifie complètement, et une révocation s'applique tout de suite. Les connexions ouvertes d'un appareil retiré sont fermées.
- **Rôles** :
  - Une seule table (`minRoles`) donne le rôle minimal de chaque commande ; une commande inconnue est refusée.
  - Les rôles du hub sont `view`, `wake` et `admin`. `control` n'y existe pas, et `devices.role` le refuse.
  - `force` (retirer ou rétrograder le dernier administrateur) n'est permis qu'à la commande `patronus-hub` sur le NAS : identité du hub, depuis l'adresse de bouclage.
  - Réglages et programmations sont réservés aux administrateurs ; la lecture de l'historique est permise dès `view`.
- **Appairage** :
  - Code de 40 bits à usage unique ; seul son condensé est gardé.
  - Comparaison en temps constant sur toutes les invitations, et une seule réponse pour toutes les causes d'échec.
  - Blocage de l'adresse après 5 échecs en une minute (5 minutes). Suspension de l'appairage après 21 échecs en 10 minutes, toutes adresses confondues, avec annulation des invitations.
  - Deviner un code pendant sa validité est hors de portée (environ 21 essais pour 2⁴⁰ codes).
- **Requêtes** :
  - Trames bornées : 16 Kio par requête, 4 Mio par réponse.
  - Identifiants strictement croissants, 100 requêtes au plus par connexion, 16 connexions simultanées.
  - Une identité inconnue a 10 secondes entre deux requêtes, et ne peut faire que `pair` : toute autre commande ferme la connexion et compte un échec.
- **Hub → agents** :
  - L'empreinte de l'agent est vérifiée pendant la poignée de main TLS, avant tout envoi.
  - Le hub refuse le rôle `admin` sur un PC : il se rétrograde à `wake` ou se retire.
  - Il ne relaie aucune commande d'une application vers un PC.
  - Chaque adresse résolue est vérifiée avant la connexion, ce qui empêche le *DNS rebinding*. Seuls les réseaux privés sont joignables, l'adresse de bouclage exclue.
- **Sondes** : requête `HEAD` sans cookie, aucune redirection suivie, aucun mandataire, adresses privées seulement. Le certificat d'une page HTTPS n'est pas vérifié, mais rien n'est envoyé ni lu.
- **Évènements** : des codes seulement (T38), avec une règle commune en Go et en Kotlin, vérifiée par des vecteurs partagés.
- **Chemins** : les identifiants de PC sont opaques et validés, les jours de mesures suivent le format `AAAA-MM-JJ`, et chaque chemin du stockage suit un motif strict (ni `..`, ni chemin absolu, ni `.tmp`).
- **Fuseau horaire** : nom IANA lu dans la base embarquée, `Local` refusé ; Go refuse aussi `..` et les chemins absolus.
- **Journal du conteneur** : le seul secret qui y figure est le lien d'appairage (usage unique, 10 minutes). Les actions y sont notées (réveils, appareils, invitations, PC confiés), jamais une clé ni le code d'une invitation créée depuis une application.
- **Conteneur** :
  - Utilisateur 65532, système de fichiers en lecture seule, aucune capacité, `no-new-privileges`, limites de mémoire et de processus.
  - Le binaire appartient à root, en mode 0555 ; l'image ne contient ni shell ni gestionnaire de paquets.
  - L'image est reproductible, essayée à chaque build et attestée à la publication.

## 4. Risques acceptés

- **Saturation depuis le réseau local** :
  - 16 connexions occupées suffisent à rendre le hub indisponible quelques secondes à la fois.
  - 21 codes faux en 10 minutes annulent les invitations en attente : c'est voulu, contre la force brute.
  - Un attaquant sur le réseau local peut déjà perturber bien davantage (T9).
- **Agent compromis** : il peut remplir l'archive du hub (jusqu'à 256 Mio par mois et par PC avant que ce mois ne soit refusé) ou y noter des évènements trompeurs sur lui-même. Le PC est alors déjà compromis, et le journal d'un agent fait foi sur la période qu'il couvre.
- **Noms libres** (appareils, PC, hub) : les caractères de contrôle sont retirés, ceux de mise en forme bidirectionnelle restent. Un nom n'est jamais une preuve d'identité : les deux applications affichent l'empreinte courte à côté.
- **Condensé non salé des codes d'invitation** : il n'est lisible qu'avec la clé des données (hub) ou les droits d'administrateur du PC (agent), et un code ne vaut que 10 minutes par défaut.
- **Root sur le NAS** (T30), et clé posée sur le même volume que les données (vol du disque, sauf volume chiffré par DSM).
- **Effacement et retour en arrière** du dossier des données (R6, R7) : le hub les subit sans les détecter.

## 5. Fuzzing

Chaque cible vérifie des **propriétés**, pas seulement l'absence de plantage. Les exemples de départ sont rejoués à
chaque `go test`. Le job « Fuzzing (protocole et hub) » de la CI lance en plus chaque cible 15 secondes à chaque
build ; une entrée qui fait échouer une cible y est gardée en artefact.

| Cible | Propriété |
|---|---|
| `server.FuzzRequests` | Jusqu'à 8 requêtes quelconques sur une même session, au nom d'une identité inconnue, d'un appareil de chaque rôle ou de la commande locale. Le hub ne panique jamais et répond par un objet JSON, à l'identifiant de la requête. Une identité inconnue n'obtient rien ni ne change rien ; `view` ne change rien ; `wake` ne change que le journal des réveils ; aucune réponse ne contient le code d'une invitation existante. Le test échoue bien si l'on donne `hub.settings` au rôle `view` |
| `store.FuzzJournalIntegrity` | Un journal authentique, tronqué ou dont un octet change, ne rend jamais un enregistrement altéré : une erreur, ou le début intact du journal. Une modification est toujours visible, et un journal n'est jamais accepté sous un autre chemin |
| `store.FuzzOpenJournal`, `store.FuzzOpenFile` | Données quelconques : jamais de plantage, seulement l'erreur « illisible ». Ce qui est accepté se relit (journal) ou se rescelle (fichier) à l'identique |
| `schedule.FuzzNext` | Toute programmation valide, à toute date de 2000 à 2100, dans 12 fuseaux choisis pour leurs changements d'heure. La prochaine et la précédente occurrence existent, tombent un jour programmé, à moins de 8 jours, et sont cohérentes entre elles (R2) |
| `hubprotocol.FuzzParseLink` | Un lien du hub accepté est bien formé, n'annonce qu'un rôle du hub, se relit à l'identique, et n'est jamais pris pour un lien d'agent |
| `hubprotocol.FuzzSlot` | Une programmation acceptée a une heure valide et de 1 à 7 jours distincts |
| `devices.FuzzOpenStorage`, `fleet.FuzzOpen` | Registre ou liste de PC quelconque : jamais de plantage. Ce qui est accepté est cohérent (appareils et identifiants distincts, rôles valides, jamais le hub administrateur d'un PC) |
| `protocol2.FuzzReadFrame`, `FuzzParseLink`, `FuzzNormalizeCode`, `FuzzParsePin` | Cibles du protocole `wolagent/2`, déjà présentes ([PROTOCOLE-V2.md §12](PROTOCOLE-V2.md#12-comportements-de-sécurité-imposés)) |

Campagne locale de cette revue (4 cœurs), aucun échec :

| Cible | Durée | Essais |
|---|---|---|
| `server.FuzzRequests` | 5 min | 0,8 million |
| `store.FuzzJournalIntegrity` | 90 s | 4,3 millions |
| `store.FuzzOpenJournal` | 90 s | 7,9 millions |
| `store.FuzzOpenFile` | 60 s | 5,1 millions |
| `schedule.FuzzNext` | 2 min, plus 4 min avant | 3,4 millions sur les 2 min |
| `hubprotocol.FuzzParseLink` | 60 s | 4,4 millions |
| `hubprotocol.FuzzSlot` | 45 s | 3,1 millions |
| `devices.FuzzOpenStorage` | 60 s | 4,4 millions |
| `fleet.FuzzOpen` | 60 s | 4,7 millions |
| `protocol2.FuzzReadFrame`, `FuzzParseLink` | 30 s chacune | 1,9 million chacune |
| `protocol2.FuzzNormalizeCode`, `FuzzParsePin` | 20 s chacune | 1,2 million chacune |

R2 a été trouvé avant cette campagne. La propriété de `FuzzNext` échouait sur les cas de changement d'heure à
minuit, désormais gardés parmi ses exemples de départ. Les autres propriétés n'ont pas été prises en défaut. Pour
s'assurer qu'elles détecteraient une faute, `FuzzRequests` a été essayée avec une faille de droits introduite
exprès (`hub.settings` permise au rôle `view`) : elle l'a signalée dès ses exemples de départ.

## 6. Pour aller plus loin

- Une revue par une personne extérieure au projet, avant une diffusion large de la 2.0.
- Lancer de temps en temps des campagnes de fuzzing plus longues, par exemple `go test ./internal/hub/server -run '^$' -fuzz FuzzRequests -fuzztime 1h`.
- Le hub ne protège pas l'historique contre l'effacement (R6). S'il en avait besoin un jour, un compteur signé par fichier, gardé dans `/keys`, détecterait aussi les retours en arrière (R7).
