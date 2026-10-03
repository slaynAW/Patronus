# Nouveautés de l'agent

L'agent a son propre numéro de version : la première section « ## X.Y.Z » ci-dessous est celle de l'agent
compilé par la CI. **Ajoutez une section (numéro supérieur) à chaque modification de l'agent** : c'est ce
numéro que les agents installés comparent au leur pour proposer la mise à jour. Le texte de la section est
affiché dans la fenêtre qui la propose (listes « - » ; `**gras**` accepté).

## 1.8.0

- **Enregistrement continu des mesures** : températures et utilisation du processeur et de la carte graphique relevées toutes les 10 secondes et résumées chaque minute (moyenne et maximum), gardées 90 jours sur le PC, même quand aucune application n'est ouverte. Les applications 1.8.0 les affichent en graphiques et les archivent chiffrées sur GitHub. `wol-agent status` indique les jours enregistrés ; `"metrics": false` dans la configuration le désactive.

## 1.7.0

- **Utilisation du processeur et de la carte graphique en %**, affichée par les applications 1.7.0. Sous Windows, elle est mesurée exactement comme dans le Gestionnaire des tâches (compteurs de performance de Windows), pour toutes les cartes graphiques, y compris les puces intégrées (Intel UHD…), sans rien installer. `wol-agent status` l'affiche aussi.

## 1.6.1

- **Température GPU des PC à graphique intégré** (Intel UHD Graphics 770, Iris Xe, Radeon des processeurs AMD) : ces puces font partie du processeur et n'ont pas de sonde lisible à part (le Gestionnaire des tâches n'en affiche pas non plus). L'agent transmet la température de la puce, celle du processeur, en le précisant.

## 1.6.0

- **Température de la carte graphique pour toutes les marques** : NVIDIA, AMD et Intel Arc sont lues comme dans le Gestionnaire des tâches, sans rien installer (seules les cartes NVIDIA l'étaient).
- **Température du processeur retrouvée avec LibreHardwareMonitor 0.9.5 ou plus**, qui ne passe plus que par son serveur web : l'agent le trouve même sur une autre adresse ou un autre port, et indique précisément ce qu'il manque (serveur web à activer, mot de passe, pilote PawnIO). `wol-agent status` détaille ce qu'il trouve.
- **Réseau « Public » détecté** : sur un réseau classé Public par Windows, le pare-feu bloque l'agent et les applications voient le PC éteint. L'agent le signale et propose de classer le réseau en Privé (avec votre accord) ; `wol-agent status` et l'installation l'indiquent aussi.

## 1.5.3

- **Rapport de diagnostic** : `wol-agent diagnostic` crée un rapport chiffré par un mot de passe (état du service, configuration sans la clé, journal des démarrages et arrêts, journal technique), à transmettre pour analyser un problème.

## 1.5.2

- Mises à jour recherchées à la nouvelle adresse du projet, [slaynAW/Patronus](https://github.com/slaynAW/Patronus).

## 1.5.1

- **Nouveau nom : Patronus**. Le service et la règle de pare-feu s'appellent désormais « Patronus - Agent » ; rien ne change dans le fonctionnement, la clé ni l'appairage. La commande reste `wol-agent`.

## 1.5.0

- **Températures** du processeur et de la carte graphique, affichées par les applications (1.5.0). Carte NVIDIA lue directement par son pilote ; processeur lu grâce à **LibreHardwareMonitor**, à lancer en administrateur sur le PC (option « Run On Windows Startup »).
- `wol-agent status` affiche les températures lues.

## 1.4.0

- **Mises à jour guidées** (Windows) : l'agent recherche chaque jour une nouvelle version et la propose à l'utilisateur connecté ; elle s'installe après son accord, sans changer la clé ni l'appairage.
- **Journal commun** : l'agent note aussi les démarrages demandés depuis les applications et le nom de l'appareil à l'origine de chaque demande ; téléphone et PC affichent le même historique.

## 1.2.0

- Journal des démarrages, extinctions et mises en veille sur 30 jours, lu par les applications.

## 1.0.0

- Première version : extinction, redémarrage et mise en veille à distance, commandes authentifiées.
