# Pistes d'amélioration

Propositions à valider ensemble. L'architecture actuelle a été pensée pour les accueillir sans refonte.

## Priorité haute (fort intérêt au quotidien)

| # | Amélioration | Intérêt | Effort |
|---|---|---|---|
| 1 | **Accès à distance via VPN** (WireGuard, Tailscale, VPN de la box) | Voir l'état et éteindre hors de la maison. L'application détecte déjà un VPN actif et sait joindre les PC par IP ; l'agent accepte déjà la plage Tailscale `100.64.0.0/10`. | Faible (doc + réglages) |
| 2 | **Réveil à distance via un relais** sur le réseau local (Raspberry Pi, NAS, ou un PC toujours allumé avec l'agent) | Le paquet magique ne traverse pas Internet : un relais authentifié (même protocole signé, nouvelle commande `wake <mac>`) l'émet localement. | Moyen |
| 3 | **Widget d'écran d'accueil** (Glance) avec voyant + bouton par PC | Démarrer un PC sans ouvrir l'application. | Moyen |
| 4 | **Tuile de réglages rapides** et **raccourcis d'application** (appui long sur l'icône) | Un geste pour allumer le PC principal. | Faible |
| 5 | **Verrouillage biométrique** (empreinte / visage) optionnel avant une extinction | Évite une extinction par quelqu'un qui emprunte le téléphone. | Faible |

## Priorité moyenne

| # | Amélioration | Intérêt |
|---|---|---|
| 6 | **Découverte automatique** des PC du réseau (mDNS / annonce de l'agent) | Plus besoin de saisir l'IP ; l'agent pourrait s'annoncer (`_wolagent._tcp`). Compatible avec le sélecteur Android 17 (pas d'autorisation nécessaire). |
| 7 | **Notifications** : « PC allumé », « le PC ne s'est pas réveillé », surveillance en arrière-plan optionnelle (WorkManager) | Suivi sans garder l'application ouverte. |
| 8 | **Groupes et actions groupées** (« tout allumer », « tout éteindre le soir ») | Pratique avec plusieurs PC. |
| 9 | **Planification** (allumer à 8 h en semaine, éteindre à 23 h) | Automatisation ; côté téléphone (alarme) ou côté agent. |
| 10 | **Extinction différée** avec compte à rebours et **annulation** (le protocole prend déjà en charge `delay`) | Laisser le temps de sauvegarder. |
| 11 | **Historique** des allumages / extinctions et temps d'allumage | Statistiques, diagnostic. |
| 12 | **Diagnostic Wake-on-LAN intégré** (assistant pas à pas : BIOS, carte réseau, démarrage rapide, test de réception du paquet par l'agent) | Réduit fortement les difficultés de configuration. |

## Priorité basse / confort

| # | Amélioration |
|---|---|
| 13 | Icônes / couleurs personnalisées par PC, tri et recherche |
| 14 | Traduction anglaise (les textes sont déjà externalisés dans `strings.xml`) |
| 15 | Actions supplémentaires de l'agent : verrouiller la session, hibernation, lancer un script |
| 16 | Sauvegarde automatique chiffrée (dossier choisi par l'utilisateur) |
| 17 | Publication via F-Droid ou Google Play (mises à jour automatiques) |
| 18 | Chiffrement du canal avec l'agent (TLS avec épinglage de certificat ou Noise) — utile surtout avec un relais |
| 19 | Application Wear OS (montre) |
| 20 | Tests d'interface automatisés (Compose UI tests sur émulateur en CI) |

## Recommandation

Commencer par **1 (VPN)**, **4 (tuile / raccourcis)**, **5 (biométrie)** et **10 (extinction différée annulable)** : peu d'effort,
gain immédiat. Puis **2 (relais)** si le réveil hors de la maison est important, et **12 (assistant de diagnostic)** pour
fiabiliser la mise en place sur de nouveaux PC.
