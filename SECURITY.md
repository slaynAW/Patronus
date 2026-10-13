# Sécurité

*To report a vulnerability, please use GitHub's private reporting: **Security → Report a vulnerability**. Do not open a public issue.*

## Signaler une faille

Utilisez le **signalement privé** de GitHub : onglet **Security** du dépôt → **Report a vulnerability**.
N'ouvrez pas d'issue publique : la faille resterait visible de tous jusqu'au correctif.

Indiquez si possible :

- la partie concernée (application Android, application Windows, agent PC) et sa version ;
- comment reproduire le problème, et ce qu'un attaquant pourrait en tirer ;
- votre configuration (version d'Android ou de Windows, réseau), sans jamais joindre de clé d'agent,
  de jeton ni de mot de passe.

Le correctif est publié dans une version officielle, puis la faille est décrite dans un avis de
sécurité du dépôt (*Security advisories*), avec votre nom si vous le souhaitez.

## Versions prises en charge

Seule la **dernière version officielle** (applications et agent) reçoit les correctifs. Les
applications et l'agent proposent eux-mêmes leurs mises à jour ([docs/MISES-A-JOUR.md](docs/MISES-A-JOUR.md)).
La pré-version « dev » sert aux essais : elle n'est ni signée avec la clé officielle ni attestée.

## Vérifier une version

Chaque version officielle est accompagnée de quoi vérifier ses fichiers, sans faire confiance à la
page de téléchargement :

| Vérification | Comment |
|---|---|
| Intégrité | `sha256sum -c SHA256SUMS.txt --ignore-missing` dans le dossier des fichiers téléchargés. |
| Provenance | `gh attestation verify <fichier> --repo slaynAW/Patronus` ([GitHub CLI](https://cli.github.com)) : prouve que le fichier a été compilé par la CI de ce dépôt, et indique le commit et le workflow d'origine. |
| Signature de l'APK | `apksigner verify --print-certs Patronus-X.Y.Z.apk` (outils du SDK Android) : l'empreinte SHA-256 du certificat doit être `eeca09a92f3de7e1353471cf4364e117508183ae25f9e97a31bac997268df411`. Android refuse de toute façon une mise à jour signée par une autre clé. |
| Composants | Inventaire au format CycloneDX (fichiers `.cdx.json`) : bibliothèques de l'application Android, modules Go de l'application Windows et de l'agent, version de Go. Lisible par les outils d'analyse de failles (OSV-Scanner, Dependency-Track…). |

Les mises à jour intégrées vérifient elles-mêmes la signature du manifeste et l'empreinte de chaque
fichier avant de l'installer ([docs/MISES-A-JOUR.md](docs/MISES-A-JOUR.md)).

## Chaîne de production

- **Actions GitHub épinglées par empreinte de commit**, tenues à jour par Dependabot, comme les
  dépendances Go et Gradle. Les nouvelles versions ne sont proposées que 14 jours après leur sortie.
- **Clé de signature réservée à la publication officielle** : elle n'est remise qu'au job
  « Publication officielle », dans l'environnement GitHub protégé `publication`, après validation
  manuelle. Les builds des branches et des pull requests n'y ont jamais accès ; ils signent avec une
  clé de test ([docs/SIGNATURE.md](docs/SIGNATURE.md)).
- **Jamais de retour en arrière** : une version officielle plus ancienne que la dernière publiée est
  refusée avant la signature (elle deviendrait celle que proposent les mises à jour intégrées).
- **Versions officielles compilées sans cache**, à partir du code du commit publié.
- **Failles connues** : `govulncheck` analyse à chaque build l'agent et l'application Windows, pour
  chaque système visé ; un build échoue si une faille publiée est atteignable depuis le code.
  La version de Go est épinglée (`toolchain` des `go.mod`).
- **Attestations de provenance** (Sigstore) et **inventaire des composants** joints à chaque version
  officielle ; outils d'analyse et d'inventaire isolés des jobs qui produisent les fichiers publiés.
- Jeton de la CI en lecture seule par défaut ; somme du wrapper Gradle vérifiée.

Conception de la sécurité des applications et de l'agent, et limites connues :
**[docs/SECURITE.md](docs/SECURITE.md)**.
