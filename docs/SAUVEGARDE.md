# Sauvegarde automatique

Sans sauvegarde, perdre ou réinitialiser un appareil fait perdre ses PC, les clés de leurs agents,
la clé de partage (les personnes invitées perdent leurs accès) et l'historique. La sauvegarde
automatique fait une **sauvegarde complète chiffrée** sans y penser, sur le téléphone comme sur
l'application Windows.

## Activer

*Réglages* → *Sauvegarde automatique* → *Activer la sauvegarde automatique* :

1. **Choisissez le mot de passe des sauvegardes** (8 caractères au moins). Il chiffre chaque
   sauvegarde et sera demandé pour la restaurer. **Notez-le en lieu sûr** (gestionnaire de mots de
   passe) : sans lui, les sauvegardes sont illisibles, pour vous comme pour quiconque.
2. **Choisissez où sauvegarder** (l'un, l'autre ou les deux) :
   - **Sur GitHub** : dans un **Gist secret** de votre compte (« Patronus – sauvegardes chiffrées »),
     commun à tous vos appareils. Les sauvegardes vous suivent si vous perdez ou changez d'appareil.
     Si le partage est déjà connecté sur l'appareil, son compte est repris ; sinon, connexion par
     code comme pour le partage (droit « gist » uniquement). Si GitHub n'accepte plus la connexion du
     partage (expirée ou révoquée), la connexion par code est proposée et sa nouvelle connexion sert
     aussi au partage (même compte).
   - **Dans un dossier** : sur Windows, de préférence un dossier synchronisé (Google Drive, OneDrive…) ;
     sur le téléphone, un dossier choisi dans le sélecteur d'Android.

## Quand

- **Après chaque changement** : PC ajouté, modifié ou supprimé, réglages, partage (connexion,
  personne autorisée ou retirée) ; plusieurs changements rapprochés ne font qu'une sauvegarde.
- **Au moins une fois par jour**, pour l'historique.
- *Sauvegarder maintenant* à tout moment.
- Sur le téléphone, les sauvegardes se font quand l'application est ouverte (pas d'activité en
  arrière-plan, comme la surveillance des PC).

Chaque appareil écrit un fichier par jour, `patronus-<appareil>-AAAA-MM-JJ.json`, et garde les **7
derniers** ; les plus anciens sont supprimés. Le Gist garde en plus l'historique de ses révisions (GitHub).

Les noms des fichiers sont visibles en clair : `<appareil>` n'est que le **type d'appareil et un
identifiant aléatoire** (par exemple `patronus-android-3fa29c1e-2026-09-30.json`), jamais le nom du
téléphone ou du PC. Avant la 1.9.0, le nom y figurait : à la première sauvegarde de la 1.9.0, les
fichiers de l'appareil sont renommés et le Gist est recopié dans un Gist neuf (l'ancien est supprimé
avec son historique), pour que l'ancien nom disparaisse aussi de GitHub.

## Changer le mot de passe

*Réglages* → *Sauvegarde automatique* → *Changer le mot de passe* : mot de passe actuel, puis le
nouveau (deux fois). **Rien n'est perdu** :

- tout ce que l'ancien mot de passe protège est **rechiffré** par le nouveau : les sauvegardes sur
  GitHub (celles de vos autres appareils comprises, si elles utilisaient le même mot de passe), celles
  du dossier, et les **archives des mesures** de tous les mois ;
- sur GitHub, chaque Gist est **recopié** dans un Gist neuf, la copie est **vérifiée**, puis l'ancien
  Gist est **supprimé avec son historique** : plus aucune version chiffrée par l'ancien mot de passe ne
  reste sur GitHub ;
- un changement interrompu (Internet coupé, application fermée) **reprend tout seul** où il s'était
  arrêté ; sauvegardes et archives attendent qu'il soit terminé ;
- vos **autres appareils** s'en aperçoivent à leur sauvegarde suivante (leurs fichiers ne s'ouvrent plus
  avec l'ancien mot de passe) : leurs sauvegardes et archives s'arrêtent et *Nouveau mot de passe à
  saisir* apparaît dans leurs réglages. Saisissez-y le nouveau mot de passe : tout reprend.

Mettez d'abord tous vos appareils en 1.9.0 : une version plus ancienne ne reconnaît pas le changement.

## Protections

- **Rien n'est lisible sans le mot de passe** : même format que l'export complet (PBKDF2-HMAC-SHA256
  600 000 itérations, AES-256-GCM, voir [SECURITE.md](SECURITE.md)). Le Gist est secret, mais même
  s'il était trouvé, il ne révélerait rien.
- **Le mot de passe et le jeton GitHub** sont gardés sur l'appareil, chiffrés comme le reste
  (Keystore Android, DPAPI sous Windows), pour sauvegarder sans vous le redemander.
- **Pas de sauvegarde vide** : une sauvegarde automatique sans aucun PC ni partage n'est pas écrite
  (elle remplacerait la dernière bonne sauvegarde du jour).
- **Pause après un incident** : si la liste des PC, le partage ou l'historique étaient illisibles au
  démarrage, les sauvegardes automatiques se mettent en pause. Vérifiez vos PC (ou restaurez), puis
  *Sauvegarder maintenant* : la pause est levée.

## Restaurer

- **Depuis GitHub** : *Réglages* → *Sauvegarde automatique* → *Restaurer depuis GitHub* : connexion à
  votre compte si nécessaire, liste des sauvegardes de tous vos appareils (de la plus récente à la
  plus ancienne), choix, mot de passe des sauvegardes, puis *Remplacer* ou *Fusionner* comme pour un
  import. La clé de partage est reprise si l'appareil ne partage pas encore : reconnectez-vous à GitHub
  (même compte) dans *Partager mes PC* pour que les personnes invitées gardent leurs accès.
- **Depuis un dossier** : *Réglages* → *Importer* et choisissez le fichier voulu.

Une sauvegarde de Windows se restaure sur le téléphone et inversement (même format).

## Désactiver

*Désactiver la sauvegarde automatique* : plus aucune sauvegarde, et le mot de passe est oublié par
l'appareil. Les sauvegardes déjà faites restent (Gist et dossier) ; supprimez-les vous-même si
besoin (gist.github.com, ou le dossier).
