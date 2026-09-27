# Signature de l'APK

Android n'accepte d'installer une **mise à jour** que si elle est signée avec **la même clé** que la version déjà installée.
La clé de signature ne doit **jamais** se trouver dans le dépôt : elle est stockée dans les **secrets GitHub**, que seule
la CI peut lire.

Tant que les secrets ne sont pas configurés, la CI signe l'APK avec une **clé temporaire** (différente à chaque build) :
l'application fonctionne, mais pour installer une nouvelle version il faut d'abord désinstaller l'ancienne
(→ exportez votre configuration avant, puis réimportez-la).

## Configurer la clé (une seule fois, ~5 minutes)

### Option A — tout depuis GitHub (recommandé)

1. Sur GitHub, onglet **Actions** → workflow **« Créer une clé de signature »** → **Run workflow**.
   > Le workflow n'apparaît qu'une fois le code présent sur la branche principale (`main`).
2. À la fin de l'exécution, téléchargez l'artifact **`cle-de-signature`** (fichier ZIP) et ouvrez-le.
3. Dans le dépôt : **Settings → Secrets and variables → Actions → New repository secret**, créez :
   | Nom | Valeur |
   |---|---|
   | `WOL_KEYSTORE_BASE64` | le contenu de `WOL_KEYSTORE_BASE64.txt` |
   | `WOL_KEYSTORE_PASSWORD` | le contenu de `WOL_KEYSTORE_PASSWORD.txt` |
4. Rangez `wakeonlan-release.p12` et le mot de passe dans un endroit sûr (gestionnaire de mots de passe).
   Supprimez ensuite le ZIP et l'artifact (il expire de toute façon au bout d'un jour).
5. Relancez un build (nouveau commit ou *Re-run*). Le journal n'affiche plus l'avertissement « clé temporaire ».

### Option B — clé générée sur votre PC

Avec un JDK installé (ou celui d'Android Studio) :

```bash
keytool -genkeypair -keystore wakeonlan-release.p12 -storetype PKCS12 -alias wakeonlan \
        -keyalg RSA -keysize 4096 -validity 10950 -dname "CN=Wake On LAN"
# Linux / macOS
base64 -w0 wakeonlan-release.p12 > WOL_KEYSTORE_BASE64.txt
# Windows (PowerShell)
[Convert]::ToBase64String([IO.File]::ReadAllBytes("wakeonlan-release.p12")) > WOL_KEYSTORE_BASE64.txt
```

Puis créez les deux secrets comme à l'étape 3 (le mot de passe est celui saisi pour `keytool`, identique pour le magasin et la clé ;
l'alias doit être `wakeonlan`, ou définissez la variable `WOL_KEY_ALIAS`).

## Passage de la clé temporaire à la clé définitive

Les APK déjà installés avec une clé temporaire ne peuvent pas être mis à jour vers la clé définitive :
1. Dans l'application : **Réglages → Exporter la configuration** (« Complète, protégée par mot de passe »).
2. Désinstallez l'application, installez le nouvel APK.
3. **Réglages → Importer une configuration**.

## Pourquoi ne pas mettre la clé dans le dépôt ?

Quiconque possède la clé peut produire un APK que votre téléphone accepterait comme mise à jour légitime de
l'application — y compris un APK malveillant qui récupérerait les clés de vos agents. Les secrets GitHub ne sont
lisibles ni dans le code ni dans les journaux (ils y sont masqués).
