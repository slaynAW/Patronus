# Nouveautés

Chaque section « ## X.Y.Z » est affichée dans les applications au moment de proposer la mise à jour
(titres « ### » et listes « - » ; `**gras**` accepté). À compléter avant de publier une version.

## 1.14.1

- **Plus de déconnexion de GitHub à tort** : quand GitHub freinait un moment des écritures trop rapprochées (partage, sauvegardes), l'application croyait la connexion révoquée et vous déconnectait. Elle garde désormais la connexion et réessaie quelques minutes plus tard ; seule une connexion vraiment expirée ou révoquée demande de vous reconnecter.
- **Partage** : « Afficher ma demande » est aussi proposé pour un partage déjà actif. Une personne dont la demande venait d'une version antérieure peut ainsi la renvoyer, et vous lui donnez tous les droits, y compris sur un PC en connexion chiffrée comme un NAS.
- **NAS Synology** (avec l'agent 1.12.1) : l'installation de l'agent s'arrêtait sur certains NAS avant d'afficher le QR code d'appairage. Elle va désormais jusqu'au bout.

## 1.14.0

- **Verrouillage facultatif** (Android) : dans *Réglages → Sécurité*, l'empreinte, le visage ou le code du téléphone peuvent être demandés avant les actions sensibles (éteindre un PC, gérer ses appareils autorisés, modifier les accès partagés, exporter la configuration, afficher une clé d'agent) et/ou à l'ouverture de l'application. Activé, il empêche une personne qui emprunte votre téléphone déverrouillé d'éteindre vos PC à votre place.
- **NAS Synology** (avec l'agent 1.12.0) : l'agent s'installe sur un NAS sous DSM 7, qui apparaît alors comme un PC : état, volumes, fiche, journal, mesures, extinction et redémarrage par l'outil de DSM, allumage par Wake-on-LAN. Le guide pas à pas est dans la documentation du projet (NAS-SYNOLOGY.md).
- **Correctif de sécurité** : l'application Windows est compilée avec Go 1.26.10, qui corrige une faille de la vérification des certificats.

## 1.13.0

- **Partage sans clé** (avec l'agent 1.11.0) : un PC en connexion chiffrée se partage désormais sans transmettre sa clé. Le PC enregistre lui-même l'appareil de la personne, avec ses seuls droits, grâce à une invitation à usage unique réservée à cet appareil ; tout se fait seul dès que le PC répond. Il suffit que votre téléphone ou votre ordinateur en soit administrateur (c'est le cas s'il a été appairé avec le QR code de « wol-agent pair »).
- **Nouveau droit « démarrer et consulter »** : la personne voit l'état du PC, son historique, ses mesures et sa fiche, sans pouvoir l'éteindre. « Démarrer et éteindre » est de nouveau possible pour les PC en connexion chiffrée.
- **Retrait appliqué par le PC** : retirer l'accès d'une personne retire aussi son appareil des PC partagés sans clé, dès que votre appareil les joint. Plus de clé à changer.
- **Pour la personne qui reçoit l'accès**, rien à faire : le PC apparaît d'abord en « démarrer », puis avec ses droits dès qu'il a enregistré son appareil (Android 10 ou plus). Si l'application indique « Demande modifiée en route », quittez ce partage et refaites la demande.
- Une personne dont l'application est plus ancienne reste en partage par clé : demandez-lui de mettre à jour son application, puis de vous renvoyer une demande.

### Aussi dans cette version

- **Connexion chiffrée avec vos PC** (avec l'agent 1.11.0) : échanges chiffrés (TLS 1.3), une identité propre à chaque téléphone ou ordinateur, et l'empreinte du PC vérifiée à chaque connexion. Les PC déjà appairés y passent d'eux-mêmes, sans rien rescanner (Android 10 ou plus).
- **QR code d'appairage sans clé secrète** et **appareils autorisés** : droits par appareil, retrait d'un téléphone perdu, invitation d'un autre appareil depuis la fiche du PC.
- Mettez à jour l'agent de vos PC (1.11.0) : les applications passent alors d'elles-mêmes en connexion chiffrée.

## 1.12.0

Version non publiée : tout son contenu est livré avec la 1.13.0.

- **Connexion chiffrée avec vos PC** (avec l'agent 1.11.0) : tous les échanges avec l'agent sont désormais chiffrés (TLS 1.3), et chaque téléphone ou ordinateur a sa propre identité, gardée dans le Keystore Android (ou chiffrée par Windows) et jamais exportée. Les PC déjà appairés y passent d'eux-mêmes, sans rien rescanner : leur fiche affiche « Chiffré et authentifié » et l'empreinte du PC, vérifiée à chaque connexion. Il faut Android 10 ou plus ; un téléphone sous Android 8 ou 9 continue comme avant.
- **QR code d'appairage sans clé secrète** : celui de « wol-agent pair » (et de l'installation de l'agent) ne contient plus qu'un code à usage unique, valable 10 minutes, que l'application n'envoie qu'au PC attendu, à l'enregistrement de la fiche.
- **Appareils autorisés** : chaque appareil a ses droits sur un PC (consulter, démarrer, contrôler, administrer). Celui qui scanne le QR code de « wol-agent pair » en est administrateur : depuis la fiche du PC, il voit les appareils autorisés, en retire un (téléphone perdu ou volé), change leurs droits, ou invite un autre téléphone ou ordinateur par un QR code à usage unique, sans passer par le PC. Un appareil passé de lui-même en connexion chiffrée a le droit « contrôler » : « wol-agent devices » sur le PC le change.
- **Partage** : un PC passé en connexion chiffrée se partage pour l'instant avec le droit « démarrer » ; « démarrer et éteindre » reviendra avec la prochaine version du partage. Les partages déjà en place continuent.
- Mettez à jour l'agent de vos PC (1.11.0) : les applications passent alors d'elles-mêmes en connexion chiffrée.

## 1.11.2

- **Correctifs de sécurité** : l'application Windows est compilée avec Go 1.26.9, qui corrige 26 failles de sa bibliothèque standard atteignables depuis l'application (connexions HTTPS à GitHub, vérification des certificats, analyse des adresses). Il s'agit surtout de blocages ou de plantages que pourraient provoquer un serveur ou des données malveillants.
- **Versions vérifiables** : chaque version officielle n'est signée qu'après une validation manuelle, et elle est accompagnée d'une attestation de provenance (preuve qu'elle a été compilée par GitHub à partir du code publié, vérifiable avec `gh attestation verify`) et de l'inventaire de ses composants. Voir SECURITY.md.

## 1.11.1

- **Température GPU sur le grand graphique** : une puce graphique intégrée au processeur (Intel UHD, Iris Xe, Radeon intégrée) a désormais sa courbe, sa valeur et sa moyenne. N'ayant pas de sonde à part, elle a la température du processeur : sa courbe est tracée en pointillés violets par-dessus celle du processeur.
- Une courbe n'est plus hachée quand la carte graphique ne donne pas sa température à chaque relevé (carte en veille…) : elle relie les relevés obtenus.

## 1.11.0

- **Températures en grand** : sur la fiche d'un PC équipé de l'agent, le grand graphique montre désormais la température du processeur (en bleu) et de la carte graphique (en violet) des 5 dernières minutes, en direct, avec leur moyenne et leur maximum. La latence reste affichée dans les informations du PC (et en grand pour un PC sans agent).
- **Fiche du PC** (avec l'agent 1.10.0) : processeur (cœurs, threads, fréquence), mémoire vive (barrettes, type, vitesse, emplacements), carte graphique (mémoire vidéo, pilote), carte mère et BIOS, modèle du PC et version du système. Elle est gardée sur l'appareil et reste visible PC éteint ; aucun numéro de série n'est lu.

## 1.10.0

- **Disques** (avec l'agent 1.9.0) : la fiche d'un PC montre l'espace libre de chaque lecteur et la santé de chaque SSD ou disque dur (état donné par Windows, température, usure, heures de fonctionnement, erreurs non corrigées), ainsi que les erreurs d'accès aux disques signalées par Windows ces 30 derniers jours. Un lecteur plein à 90 % ou plus, ou un disque à surveiller, est signalé dans la liste des PC.
- **Plantages et arrêts anormaux expliqués** : l'historique précise désormais la cause d'un arrêt inattendu — écran bleu (avec son code), arrêt forcé avec le bouton d'alimentation, coupure de courant ou blocage, erreur matérielle fatale. Elle est conservée dans les archives.

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
