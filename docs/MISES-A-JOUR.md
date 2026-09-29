# Mises à jour intégrées

Les applications Android et Windows proposent elles-mêmes les nouvelles versions officielles, avec
leurs nouveautés, et s'installent en un clic. Les PC enregistrés, leurs clés, les réglages et
l'historique sont conservés. L'agent Windows se met aussi à jour, après l'accord de l'utilisateur du
PC (voir [Agent](#agent)).

## Ce que voit l'utilisateur

- Au démarrage (Windows : peu après, puis une fois par jour ; Android : à l'ouverture, au plus toutes
  les 12 h), l'application regarde s'il existe une version plus récente. Si oui, une fenêtre
  **« Mise à jour disponible »** affiche le numéro, la date, la taille et les **nouveautés**, avec
  **Plus tard** et **Mettre à jour**.
- *Plus tard* : la version n'est plus proposée d'elle-même pendant 24 h (elle reste visible dans
  *Réglages → Mises à jour*, et une pastille apparaît sur l'onglet *Réglages*).
- *Réglages → Mises à jour* : **Rechercher une mise à jour** à tout moment, et l'option
  **Rechercher automatiquement** (activée par défaut).

**Windows** : l'application télécharge la nouvelle version, la vérifie, remplace son propre fichier
`.exe` puis redémarre (quelques secondes). L'ancienne version est gardée à côté (`.exe.old`) le temps
du redémarrage, puis supprimée. Les données sont dans `%APPDATA%\WakeOnLan`, jamais touchées.

**Android** : l'application télécharge et vérifie l'APK puis le confie à l'installateur d'Android.
Hors Play Store, Android demande une confirmation : la première fois, il faut aussi autoriser
*Patronus* à installer des applications (*Paramètres → Installer des applis inconnues*). À partir
d'Android 12, une fois que l'application s'est mise à jour elle-même, les mises à jour suivantes
peuvent se faire sans confirmation. Une mise à jour Android conserve toujours les données de
l'application (même clé de signature, numéro de version plus grand).

## Publier une nouvelle version

1. Compléter **`NOUVEAUTES.md`** : une section `## X.Y.Z` (titres `###`, listes `-`, `**gras**`).
   C'est ce texte qui s'affiche dans les applications (et en tête de la Release GitHub).
   Si l'agent a changé : ajouter aussi une section (numéro supérieur) en tête de
   **`agent/NOUVEAUTES.md`**, qui donne le numéro de l'agent.
2. Fusionner dans `main`, puis **Actions → Build → Run workflow** avec le numéro `X.Y.Z`
   (ou pousser un tag `vX.Y.Z`).
3. La CI publie la Release avec l'APK, l'application Windows, les agents et le manifeste
   **`update.json`** (+ `update.json.sig`). Les applications le trouvent à l'adresse
   `https://github.com/slaynAW/WakeOnLan/releases/latest/download/update.json`.

Seules les versions officielles sont proposées (la pré-version `dev` est ignorée). Le numéro comparé
est le code de build (numéro d'exécution de la CI), qui augmente à chaque build.

## Agent

L'agent a son propre numéro, donné par la première section de `agent/NOUVEAUTES.md` : les binaires
sont compilés avec ce numéro (`X.Y.Z-dev.N` pour les pré-versions), et le manifeste le reprend dans
sa section `agent` avec les nouveautés de l'agent et, pour Windows (`windows-amd64`,
`windows-arm64`), le nom, la taille et l'empreinte SHA-256 de chaque binaire.

Chaque jour, le service de l'agent (Windows) lit le manifeste de la dernière version officielle,
vérifie sa signature avec le même certificat que l'application Windows, et compare le numéro de
l'agent au sien. Une version plus récente est proposée à l'utilisateur connecté (fenêtre Oui / Non) ;
une fois acceptée, elle est téléchargée dans `C:\Program Files\WolAgent` (dossier réservé aux
administrateurs), contrôlée (taille, empreinte, numéro affiché par `wol-agent version`), mise à la
place de l'ancienne (gardée en `.old`), puis un processus détaché redémarre le service et vérifie
qu'il répond, sinon remet l'ancienne version. Voir [agent/README.md](../agent/README.md).

La CI teste ce parcours de bout en bout sous Windows : un vrai service est installé puis se met à
jour depuis un faux serveur de versions, signé avec une clé de test intégrée uniquement à ces
compilations de test.

## Sécurité

- `update.json` contient le numéro de version, le code de build, les nouveautés, et pour chaque
  plateforme le nom, la taille et l'empreinte **SHA-256** du fichier.
- Il est **signé par la CI avec la clé de signature de l'APK** (secrets `WOL_KEYSTORE_*`,
  RSA / SHA-256). Aucun nouveau secret n'est nécessaire.
- **Android** vérifie la signature avec le certificat de l'application installée ; **Windows** et
  **l'agent** avec le certificat `agent/update/release-cert.pem` qui leur est intégré. Un manifeste
  non signé ou modifié est refusé.
- Le fichier téléchargé doit avoir exactement la taille et l'empreinte annoncées, sinon il est
  supprimé sans être installé. Sur Android, l'installateur vérifie en plus que l'APK est signé avec
  la même clé que l'application installée.
- Seule la recherche de mises à jour contacte Internet (GitHub, en HTTPS) ; elle ne transmet aucune
  donnée personnelle et peut être désactivée.

### Certificat de l'application Windows et de l'agent

`agent/update/release-cert.pem` contient le certificat **public** de la clé de signature.
À chaque build, la CI vérifie qu'il correspond à la clé des secrets : sinon elle affiche le
certificat attendu dans le journal (étape « Manifeste de mise à jour ») et refuse de publier une
version officielle. Tant qu'il est absent, les mises à jour intégrées de l'application Windows sont
désactivées (elle l'indique dans ses réglages).

## Dépannage

| Situation | Cause et solution |
|---|---|
| « Indisponible pour cette version » | Version de développement (Windows compilé localement) ou APK signé avec une autre clé que la clé officielle : installer une fois la version officielle depuis GitHub. |
| « Mise à jour impossible : impossible de remplacer l'application » (Windows) | Le `.exe` est dans un dossier protégé (ex. `C:\Program Files`) : le déplacer dans un dossier personnel (ex. `Documents`) ou télécharger la version depuis GitHub. |
| « Installation annulée » (Android) | La confirmation d'Android a été refusée : relancer *Mettre à jour*. |
| Aucune proposition alors qu'une version existe | La Release ne contient pas `update.json` (versions antérieures à 1.2.0) ou elle est marquée « pré-version ». |
| L'agent ne propose pas sa mise à jour | Agent antérieur à 1.4.0 (à installer une fois à la main), recherche désactivée (`wol-agent status`), ou refus récent (reproposée le lendemain). Journal : `C:\ProgramData\WolAgent\agent.log` ; `wol-agent update` force la recherche. |
