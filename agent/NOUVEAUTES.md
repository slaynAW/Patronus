# Nouveautés de l'agent

L'agent a son propre numéro de version : la première section « ## X.Y.Z » ci-dessous est celle de l'agent
compilé par la CI. **Ajoutez une section (numéro supérieur) à chaque modification de l'agent** : c'est ce
numéro que les agents installés comparent au leur pour proposer la mise à jour. Le texte de la section est
affiché dans la fenêtre qui la propose (listes « - » ; `**gras**` accepté).

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
