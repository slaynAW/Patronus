# Mesures et archives

Patronus garde l'historique des **températures** et de l'**utilisation** du processeur et de la carte
graphique de vos PC, minute par minute, ainsi que le **journal des démarrages et arrêts**, et les range
**chiffrés** sur votre compte GitHub pour les consulter plus tard.

## Enregistrement continu (agent 1.8.0)

L'agent de chaque PC relève les capteurs toutes les 10 secondes (toutes les 5 secondes quand une
application regarde le PC) et enregistre **une ligne par minute** : moyenne et maximum de la
température du processeur et de la carte graphique, et de leur utilisation. Ces lignes sont gardées
**90 jours** sur le PC (un fichier par jour dans `metrics/`, à côté de la configuration de l'agent,
environ 2 à 3 Mo au total), même quand aucune application n'est ouverte.

- `wol-agent status` indique les jours enregistrés.
- `"metrics": false` dans `config.json` désactive l'enregistrement (l'état reste lu à la demande).
- Les applications lisent ces mesures par la commande authentifiée `metrics` (voir
  [PROTOCOLE.md](PROTOCOLE.md)), réservée, comme l'état, aux détenteurs de la clé de l'agent.

## Archives sur GitHub

*Réglages* → *Sauvegarde automatique* → *Archives des mesures* (application Windows ou téléphone). Il faut
que la sauvegarde automatique soit activée (son **mot de passe** chiffre les archives) et que GitHub y soit
connecté (*Sur GitHub* : même compte, droit « gist » seulement). Les archives utilisent cette connexion
des sauvegardes, pas directement celle du partage ; toucher *Archives des mesures* la connecte au besoin
(compte du partage repris, ou connexion par code), puis propose d'activer l'archivage.

- **Toutes les 30 minutes** tant que l'application est ouverte, elle rattrape ce que les agents des PC
  joignables ont enregistré depuis le dernier archivage, avec leur journal des démarrages et arrêts (y
  compris les commandes et l'appareil à l'origine), et l'ajoute aux archives. *Archiver maintenant*
  le fait tout de suite.
- Comme les agents gardent 90 jours, rien n'est perdu si l'application reste fermée quelques semaines.
- **Un Gist secret par mois** (« Patronus – archives chiffrées 2026-10 ») : un fichier de mesures par jour
  (`mesures-2026-10-03.txt`) avec tous les PC, et le journal du mois (`journal-2026-10.txt`). Plusieurs
  appareils peuvent archiver : leurs ajouts sont fusionnés.

### Protections

- **Rien n'est lisible sans le mot de passe des sauvegardes** : chaque fichier est compressé puis chiffré
  en AES-256-GCM avec une clé dérivée du mot de passe par PBKDF2-HMAC-SHA256 (600 000 itérations, sel
  propre à chaque mois). Le nom de chaque fichier est authentifié : un fichier ne peut être ni modifié ni
  remplacé par un autre sans que ce soit détecté.
- **En clair, seules les dates** apparaissent (noms des fichiers). Les noms des PC, leurs adresses MAC,
  les mesures et le journal sont chiffrés.
- Le Gist est secret (non listé) ; il n'est visible que de votre compte et de qui en aurait le lien,
  qui ne pourrait de toute façon rien en lire.
- Le mot de passe et le jeton GitHub restent sur l'appareil, chiffrés (DPAPI sous Windows, Keystore sur
  Android).
- *Changer le mot de passe* (sauvegarde automatique) rechiffre **tous les mois d'archives** par le
  nouveau mot de passe, sans rien perdre : chaque Gist est recopié, vérifié, puis l'ancien est supprimé
  avec son historique ([SAUVEGARDE.md](SAUVEGARDE.md#changer-le-mot-de-passe)).
- En revanche, désactiver puis réactiver la sauvegarde automatique avec un autre mot de passe, ou
  changer de compte GitHub, ne rechiffre rien : les mois suivants vont dans de nouveaux Gists (le mois en
  cours est réarchivé avec tout ce que les agents gardent encore) et les anciens restent lisibles avec
  l'ancien mot de passe seulement.

Format détaillé : `desktop/internal/archive` (Windows) et `core/archive` (Android), vecteurs de test
communs dans `protocol/archive-vectors.json`.

## Consulter

Fiche d'un PC → **Mesures** :

- périodes **24 h, 7, 30 ou 90 jours**, ou un **mois archivé** ;
- deux graphiques, **températures** et **utilisation** (CPU en bleu, GPU en violet ; trait plein :
  moyenne, trait fin : maximum), coupés quand le PC était éteint ; moyenne et maximum de la période ;
  survol (Windows) ou toucher (téléphone) pour les valeurs d'un instant ;
- le **journal** de la période (démarrages, arrêts, veilles, commandes).

Les mesures des 90 derniers jours sont lues directement sur le PC s'il est allumé ; au-delà, ou PC
éteint, dans les archives GitHub.

## Désactiver

*Archives des mesures* → *Arrêter* : plus aucun envoi. Les archives existantes restent dans vos Gists ;
supprimez-les sur gist.github.com si besoin. Désactiver la sauvegarde automatique ou déconnecter GitHub
arrête aussi les archives.
