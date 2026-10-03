# Nouveautés

Chaque section « ## X.Y.Z » est affichée dans les applications au moment de proposer la mise à jour
(titres « ### » et listes « - » ; `**gras**` accepté). À compléter avant de publier une version.

## 1.9.0

- **Changer le mot de passe des sauvegardes sans rien perdre** : *Réglages* → *Sauvegarde automatique* → *Changer le mot de passe*. Sauvegardes (GitHub et dossier) et archives des mesures sont rechiffrées par le nouveau mot de passe ; sur GitHub, chaque Gist est recopié, vérifié, puis l'ancien est supprimé avec son historique : l'ancien mot de passe n'ouvre plus rien. Un changement interrompu reprend tout seul.
- **Vos autres appareils suivent** : ils s'aperçoivent du changement et affichent *Nouveau mot de passe à saisir* ; sauvegardes et archives y reprennent dès qu'il est saisi.
- **Plus de nom d'appareil sur GitHub** : les fichiers de sauvegarde ne portent plus le nom de votre téléphone ou de votre PC, seulement son type et un identifiant aléatoire. Les fichiers existants sont renommés automatiquement, et l'ancien nom disparaît aussi de l'historique du Gist.
- Mettez à jour tous vos appareils avant de changer le mot de passe.

## 1.8.1

- **Connexion GitHub des sauvegardes et des archives** : quand GitHub n'acceptait plus la connexion du partage (expirée ou révoquée), *Sur GitHub* et *Archives des mesures* affichaient « connexion GitHub expirée ou révoquée » sans proposer de se reconnecter. La connexion par code s'ouvre désormais, et elle reconnecte aussi le partage (même compte). Après la connexion depuis *Archives des mesures*, l'activation de l'archivage est proposée directement.

## 1.8.0

- **Mesures dans le temps** : fiche d'un PC → *Mesures*. Températures et utilisation du processeur et de la carte graphique sur 24 h, 7, 30 ou 90 jours, minute par minute (moyenne et maximum), avec le journal des démarrages et arrêts de la période (avec l'agent 1.8.0, qui les enregistre en continu, même application fermée).
- **Archives chiffrées sur GitHub** : *Réglages* → *Sauvegarde automatique* → *Archives des mesures*. Toutes les 30 minutes, les mesures et le journal de vos PC sont rangés dans un Gist secret par mois, chiffrés par le mot de passe des sauvegardes, pour les consulter des mois plus tard, même PC éteint ou depuis un autre appareil.
- **Œil sur les mots de passe** : maintenez l'œil d'un champ de mot de passe (sauvegarde, export, import, diagnostic) pour voir ce que vous tapez ; il se masque dès que vous relâchez.

## 1.7.0

- **Utilisation du processeur et de la carte graphique en %** de chaque PC allumé, sur sa fiche et dans les listes (« CPU 54 °C (23 %) · GPU 61 °C (41 %) »). Les chiffres sont mesurés comme dans le Gestionnaire des tâches de Windows, pour toutes les marques de cartes graphiques, y compris les puces intégrées (avec l'agent 1.7.0, qui se propose tout seul aux PC Windows).

## 1.6.1

- **Température GPU des PC à graphique intégré** (Intel UHD Graphics, Iris Xe, Radeon des processeurs AMD) : ces puces font partie du processeur et n'ont pas de sonde à part ; la fiche du PC affiche désormais la température de la puce, en le précisant (agent 1.6.1).

## 1.6.0

- **Sauvegarde automatique chiffrée** : *Réglages* → *Sauvegarde automatique*. Après chaque changement (PC, réglages, partage) et chaque jour, une sauvegarde complète est chiffrée par votre mot de passe et enregistrée dans un Gist secret de votre compte GitHub et/ou dans un dossier ; les 7 dernières versions sont gardées.
- **Restaurer depuis GitHub** : sur un nouveau téléphone ou PC, retrouvez vos PC, leurs clés, votre partage et l'historique en quelques gestes.
- **Températures** : avec l'agent 1.6.0, la carte graphique est lue pour toutes les marques (NVIDIA, AMD, Intel Arc), et quand la température du processeur manque, la fiche du PC dit exactement quoi faire dans LibreHardwareMonitor (serveur web à activer, mot de passe, pilote).
- **PC allumé mais affiché éteint ?** L'application explique la cause la plus fréquente (réseau classé « Public » dans Windows), et l'agent 1.6.0 propose de lui-même de corriger ce réglage sur le PC.

## 1.5.4

- **Rapport de diagnostic** : *Réglages* → *Diagnostic* → *Exporter le rapport de diagnostic*. Chaque application tient désormais un journal détaillé de ses actions et de ses erreurs, chiffré sur l'appareil ; le rapport exporté est protégé par un mot de passe que vous choisissez, pour l'envoyer en toute sécurité lors d'un problème.
- **Données protégées** : si la liste des PC, le partage ou l'historique ne peuvent plus être relus (clé de chiffrement perdue, fichier abîmé), le fichier est mis de côté au lieu d'être effacé et l'application vous prévient, avec la marche à suivre pour tout récupérer.
- Le partage est plus robuste : une erreur imprévue pendant la publication ou la vérification des accès est signalée au lieu de fermer l'application.

## 1.5.3

- **Agent téléchargeable depuis l'application Windows** : *Réglages* → *Télécharger l'agent*. Installez-le sur cet ordinateur en un clic (invite administrateur), ou enregistrez-le pour un autre PC (x64 ou ARM64). L'application vérifie la signature du projet et l'empreinte du fichier.
- La fiche d'un PC signale quand son agent a une version de retard (par exemple pour obtenir les températures), avec un lien pour télécharger la nouvelle.

## 1.5.2

- **Le projet a déménagé** : il s'appelle désormais [slaynAW/Patronus](https://github.com/slaynAW/Patronus) sur GitHub. Liens et recherche de mises à jour pointent vers la nouvelle adresse (l'ancienne redirige vers la nouvelle).

## 1.5.1

- **Wake On LAN devient Patronus** : votre Patronus veille sur vos PC. Nouveau nom dans les applications, sur le téléphone et sur le PC Windows ; vos PC, réglages, historique et partages sont conservés.
- Sur Windows, l'exécutable s'appelle désormais `Patronus-Windows-….exe` pour les nouveaux téléchargements (le fichier déjà présent se met à jour sans changer de nom).

## 1.5.0

- **Températures du processeur et de la carte graphique** de chaque PC allumé, sur sa fiche et dans les listes, en orange dès 80 °C et en rouge dès 90 °C (avec l'agent 1.5.0, qui se propose tout seul aux PC Windows).
- Sous Windows, la température du processeur demande **LibreHardwareMonitor** lancé en administrateur sur le PC ; la carte graphique NVIDIA est lue sans rien installer.

## 1.4.1

- **Nouveau logo** : un commutateur relié à vos PC, dont un qui s'allume. Sur Android, l'icône suit aussi le thème du téléphone (icônes à thème).

## 1.4.0

- **Agent Windows à jour tout seul** : l'agent des PC (1.4.0) propose ses nouvelles versions à l'utilisateur du PC et s'installe après son accord, sans changer sa clé ni l'appairage. À installer une fois à la main depuis cette version.
- **Historique commun** au téléphone et au PC Windows (avec l'agent 1.4.0) : chaque démarrage, extinction ou mise en veille indique l'appareil qui l'a demandé (« par Pixel 8 »).
- **Historique dans la sauvegarde complète** : il suit lors d'un changement de téléphone ou de PC et s'ajoute à celui de l'appareil qui l'importe.
- Le tracé de latence des listes s'arrête au milieu de la ligne et ne passe plus sur le nom du PC.

## 1.3.1

- **Partage** : la connexion à GitHub aboutit désormais même si le téléphone coupe Internet à l'application pendant la validation du code (elle se termine dès le retour dans l'application).
- « Ouvrir GitHub » copie le code de connexion : il n'y a plus qu'à le coller sur la page de GitHub.

## 1.3.0

- **Partage de vos PC** avec les personnes de votre choix : vous décidez, PC par PC, qui peut les démarrer (ou aussi les éteindre). Invitation par QR code ou par lien, code de vérification à comparer.
- **Sécurisé de bout en bout** : chaque accès est chiffré pour le seul appareil de la personne autorisée et signé par vous ; personne d'autre ne peut le lire ni s'inviter, même avec l'application.
- Les PC reçus apparaissent dans vos listes, marqués « Partagé par … », et se mettent à jour tout seuls quand leur propriétaire les modifie ; retirer un accès les fait disparaître.
- La sauvegarde complète (protégée par mot de passe) contient votre clé de partage : changez de téléphone ou de PC sans réinviter tout le monde.

## 1.2.0

- **Nouveau style sombre**, identique sur Android et Windows : plan du réseau, fiche de chaque PC, synthèse en anneau.
- **Historique des démarrages et extinctions** sur 30 jours, dans la fiche de chaque PC (complet avec l'agent 1.2 ou plus, même quand l'application était fermée).
- **Latence en direct** façon électrocardiogramme dans la fiche d'un PC, et mini-tracés dans les listes.
- **Mises à jour intégrées** : les applications proposent les nouvelles versions et les installent en un clic, sans toucher à vos PC ni à l'historique.

## 1.1.0

- **Application Windows** : mêmes fonctions et même interface que l'application Android, en un seul fichier.

## 1.0.0

- Première version : démarrage des PC (Wake-on-LAN), état en temps réel, extinction, redémarrage et veille avec l'agent.
