# Signature de l'APK

Android n'accepte d'installer une **mise à jour** que si elle est signée avec **la même clé** que la version déjà installée.
La clé de signature ne doit **jamais** se trouver dans le dépôt : elle est stockée dans les **secrets GitHub**, et seul le
job **« Publication officielle »** de la CI la reçoit, après ta validation (environnement protégé `publication`, voir
plus bas). Ce même job signe aussi le manifeste des mises à jour intégrées ([MISES-A-JOUR.md](MISES-A-JOUR.md)).

Les builds des branches et des pull requests, et donc la pré-version « dev », n'ont jamais accès à la clé : leur APK est
signé avec une **clé de test** (différente à chaque build). Il fonctionne, mais ne s'installe pas par-dessus l'application
officielle : il faut d'abord la désinstaller (→ exportez votre configuration avant, puis réimportez-la).

## Configurer la clé (une seule fois, ~5 minutes)

> **Dépôt public** : utilisez l'option B. Les artefacts d'un dépôt public sont téléchargeables par tous ;
> le workflow de l'option A refuse donc de s'exécuter dans ce cas.

### Option A — tout depuis GitHub (dépôt privé uniquement)

1. Sur GitHub, onglet **Actions** → workflow **« Créer une clé de signature »** → **Run workflow**.
   > Le workflow n'apparaît qu'une fois le code présent sur la branche principale (`main`).
2. À la fin de l'exécution, téléchargez l'artifact **`cle-de-signature`** (fichier ZIP) et ouvrez-le.
3. Créez les deux secrets dans l'environnement `publication` (voir [Protéger la clé](#protéger-la-clé--environnement-publication)) :
   | Nom | Valeur |
   |---|---|
   | `WOL_KEYSTORE_BASE64` | le contenu de `WOL_KEYSTORE_BASE64.txt` |
   | `WOL_KEYSTORE_PASSWORD` | le contenu de `WOL_KEYSTORE_PASSWORD.txt` |
4. Rangez `wakeonlan-release.p12` et le mot de passe dans un endroit sûr (gestionnaire de mots de passe).
   Supprimez ensuite le ZIP et l'artifact (il expire de toute façon au bout d'un jour).

### Option B — clé générée sur votre PC

Avec un JDK installé (ou celui d'Android Studio) :

```bash
keytool -genkeypair -keystore wakeonlan-release.p12 -storetype PKCS12 -alias wakeonlan \
        -keyalg RSA -keysize 4096 -validity 10950 -dname "CN=Patronus"
# Linux / macOS
base64 -w0 wakeonlan-release.p12 > WOL_KEYSTORE_BASE64.txt
# Windows (PowerShell)
[Convert]::ToBase64String([IO.File]::ReadAllBytes("wakeonlan-release.p12")) > WOL_KEYSTORE_BASE64.txt
```

Puis créez les deux secrets comme à l'étape 3 (le mot de passe est celui saisi pour `keytool`, identique pour le magasin et la clé ;
l'alias doit être `wakeonlan`, ou définissez la variable `WOL_KEY_ALIAS`).

Le certificat **public** de la clé doit aussi figurer dans `agent/update/release-cert.pem` (application Windows et agent) :
la première publication officielle l'affiche dans son journal s'il ne correspond pas ([MISES-A-JOUR.md](MISES-A-JOUR.md)).

## Protéger la clé : environnement `publication`

Des secrets « du dépôt » sont lisibles par n'importe quel workflow d'une branche : un workflow modifié, ou une action
détournée, pourrait les lire et signer une version piégée. Rangés dans un **environnement protégé**, ils ne sont remis
qu'au job « Publication officielle », lancé depuis `main` (ou un tag `v…`), et seulement après ta validation.

1. **Settings → Environments → New environment** : nom `publication`.
   (Il existe peut-être déjà : GitHub le crée, sans protection, à la première publication qui l'utilise.)
2. **Required reviewers** : coche la case et ajoute-toi. Laisse *Prevent self-review* décoché, sinon tu ne pourrais pas
   valider tes propres publications. **Save protection rules**.
3. **Deployment branches and tags** → *Selected branches and tags* → **Add deployment branch or tag rule** :
   branche `main`, puis tag `v*`.
4. **Environment secrets → Add environment secret** : `WOL_KEYSTORE_BASE64` et `WOL_KEYSTORE_PASSWORD`, avec les mêmes
   valeurs que les secrets du dépôt (contenus des fichiers rangés dans ton gestionnaire de mots de passe : GitHub ne permet
   pas de relire un secret).
5. Après une **première publication officielle réussie** avec ces secrets, supprime les secrets du dépôt :
   **Settings → Secrets and variables → Actions → Repository secrets** → `WOL_KEYSTORE_BASE64` et `WOL_KEYSTORE_PASSWORD`.

> Sans les fichiers d'origine, ne supprime rien : la publication continue de fonctionner avec les secrets du dépôt
> (les secrets de l'environnement sont prioritaires quand ils existent), avec une protection moindre.

### Valider une publication

Une fois les builds terminés, l'exécution s'arrête sur **« Waiting for review »** (onglet **Actions**, ou la notification
de GitHub) : **Review deployments** → coche `publication` → **Approve and deploy**. Le job signe alors l'APK et le manifeste,
atteste la provenance de chaque fichier, puis crée le tag et la Release. **Reject** annule la publication sans rien créer.

## Comment la CI signe l'APK

1. Le job « Application Android » compile l'APK et le signe avec une clé de test. Il rejoue aussi la signature de
   publication avec une clé jetable (`.github/scripts/sign-apk.sh`) : un problème se voit à chaque build.
2. Le job « Publication officielle » re-signe cet APK avec la clé définitive (`apksigner`, schémas v2 et v3), puis vérifie
   que la signature est valide, que le seul signataire est bien cette clé, et que l'alignement des fichiers est intact.

## Passage d'un APK de test à la version officielle

Un APK de test (pré-version « dev », ou ancienne version signée avec une clé temporaire) ne peut pas être mis à jour vers
une version officielle :
1. Dans l'application : **Réglages → Exporter la configuration** (« Complète, protégée par mot de passe »).
2. Désinstallez l'application, installez l'APK officiel.
3. **Réglages → Importer une configuration**.

## Pourquoi ne pas mettre la clé dans le dépôt ?

Quiconque possède la clé peut produire un APK que votre téléphone accepterait comme mise à jour légitime de
l'application — y compris un APK malveillant qui récupérerait les clés de vos agents. Les secrets GitHub ne sont
lisibles ni dans le code ni dans les journaux (ils y sont masqués), et l'environnement `publication` limite leur usage
aux publications que tu as validées.
