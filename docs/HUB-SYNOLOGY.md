# Hub sur un NAS Synology (DSM 7.2, Container Manager)

Le **hub** ([HUB.md](HUB.md)) tourne en continu sur le NAS, dans un conteneur. Il surveille les PC qui lui sont
confiés et les appareils de la maison, même quand votre téléphone ou votre ordinateur est éteint :

- **historique long** : journal et mesures de chaque PC gardés un an par défaut (de 3 mois à 3 ans), au-delà des 30 et
  90 jours de l'agent ;
- **« Pendant votre absence »** : PC qui ne répond plus ou de nouveau, arrêt inattendu, disque à surveiller, mesure
  critique… signalés à la prochaine ouverture de l'application ;
- **programmations** : démarrer un PC à heure fixe, l'éteindre le soir s'il n'est plus utilisé ;
- **démarrage depuis le NAS** : quand votre téléphone n'est pas sur le réseau de la maison (VPN), le hub envoie le
  paquet magique depuis le réseau local.

Le hub n'est **administrateur d'aucun PC** et ne relaie aucune commande : il peut lire l'état d'un PC et le démarrer,
mais pas l'éteindre (sauf l'extinction programmée que vous activez PC par PC), ni gérer ses appareils autorisés.

**Il faut** : DSM 7.2 ou plus avec le paquet **Container Manager** (proposé par le *Centre de paquets* sur les modèles
compatibles, comme le DS218+, le DS220+ ou le DS920+ ; l'image existe pour les processeurs Intel ou AMD et ARM 64 bits) ;
les applications Patronus 2.0 ; l'agent 1.11.0 ou plus sur les PC à confier au hub. L'agent du NAS ([NAS-SYNOLOGY.md](NAS-SYNOLOGY.md)), s'il est installé, reste tel quel : le hub
utilise le port 9771, l'agent le 9770.

## 1. Préparer DSM

1. **Adresse IP fixe** pour le NAS (bail DHCP réservé dans la box, ou *Panneau de configuration* → *Réseau*).
2. **Container Manager** : *Centre de paquets* → *Container Manager* → *Installer*. Il crée le dossier partagé
   `docker`.
3. **SSH**, le temps de l'installation : *Panneau de configuration* → *Terminal et SNMP* → **Activer le service SSH**.
4. **Pare-feu de DSM**, s'il est activé (*Panneau de configuration* → *Sécurité* → *Pare-feu*) : ajoutez une règle qui
   autorise le **port TCP 9771** depuis votre réseau local seulement (par exemple `192.168.1.0`, masque
   `255.255.255.0`), avant les règles de refus. N'ouvrez **jamais** ce port sur votre box : hors de chez vous, passez
   par un VPN.

## 2. Créer les deux dossiers du hub

En SSH, avec un compte administrateur (`ssh nom@192.168.1.x`) :

```sh
sudo mkdir -p /volume1/docker/patronus-hub/data /volume1/patronus-hub-keys
sudo chown 65532:65532 /volume1/docker/patronus-hub/data /volume1/patronus-hub-keys
sudo chmod 700 /volume1/docker/patronus-hub/data /volume1/patronus-hub-keys
```

- **`/volume1/docker/patronus-hub/data`** : l'historique, les PC confiés et les appareils autorisés, **chiffrés**. Ce
  dossier peut être sauvegardé (Hyper Backup) : sans la clé, son contenu est illisible.
- **`/volume1/patronus-hub-keys`** : l'identité du hub et la clé des données. Hors de tout dossier partagé : ni
  visible sur le réseau, ni sauvegardé avec le reste. Après le premier démarrage, **copiez-le une fois** sur une clé
  USB rangée à part : clé perdue, historique du hub perdu (les PC gardent leurs 30 jours de journal et 90 jours de
  mesures, et vos archives GitHub restent).
- L'utilisateur **65532** est celui du conteneur : ni root, ni un compte de DSM. Le hub ne peut écrire que dans ces
  deux dossiers.

## 3. Créer le projet

*Container Manager* → *Projet* → *Créer* :

- **Nom du projet** : `patronus-hub` ;
- **Chemin** : `/volume1/docker/patronus-hub` ;
- **Source** : *Créer un fichier docker-compose.yml*, puis collez :

```yaml
services:
  patronus-hub:
    image: ghcr.io/slaynaw/patronus-hub:latest
    container_name: patronus-hub
    network_mode: host            # paquets magiques en diffusion, adresses réelles des applications
    user: "65532:65532"           # ni root, ni compte DSM
    read_only: true
    cap_drop: [ALL]
    security_opt: [no-new-privileges:true]
    mem_limit: 128m
    pids_limit: 64
    environment:
      TZ: Europe/Paris            # heure du journal du conteneur
    volumes:
      - /volume1/docker/patronus-hub/data:/data
      - /volume1/patronus-hub-keys:/keys
    restart: unless-stopped
```

*Suivant* (ignorez le portail web : le hub n'a pas de page web), puis *Terminé* : l'image est téléchargée et le hub
démarre.

- **Nom du hub** affiché dans les applications (« Hub NAS » par défaut) : ajoutez la ligne
  `command: ["serve", "--name", "NAS du salon"]` sous `container_name`.
- **Réseau de l'hôte** : seule exception au cloisonnement, nécessaire au paquet magique (une diffusion ne sort pas
  d'un réseau Docker) et pour que le hub voie l'adresse réelle des applications.

## 4. Appairer une première application

*Container Manager* → *Conteneur* → **patronus-hub** → *Journal* : au premier démarrage, le hub y écrit un lien
d'appairage administrateur, à usage unique, valable 10 minutes :

```
Appairage d'une application au hub (rôle admin), une seule fois, jusqu'à 21:42 :
  lien : patronus://hub?v=1&…
  ou : adresse 192.168.1.20, port 9771, code H3RT-6WQA
  empreinte du hub : 4D1Z-7KQ2-9MXP
Nouveau code : sudo docker exec patronus-hub patronus-hub pair
```

1. **Application Windows** : *Réglages* → *Hub sur le NAS* → *Appairer un hub* → collez le lien.
   **Téléphone** : *Réglages* → *Appairer un hub* → *Coller le lien* (copiez-le depuis DSM ouvert dans le navigateur
   du téléphone). Le plus simple pour les appareils suivants : depuis le premier, fiche du hub → *Inviter un
   appareil*, puis scanner le QR code avec l'autre.
2. Comparez l'**empreinte du hub** affichée par l'application avec celle du journal : l'application la vérifie à
   chaque connexion.
3. Lien expiré ou déjà utilisé : `sudo docker exec patronus-hub patronus-hub pair` (en SSH) en affiche un nouveau
   (`--role view|wake|admin` pour un autre rôle qu'administrateur).

Si le lien annonce une autre adresse que celle du NAS sur votre réseau (VPN, plusieurs cartes réseau), ajoutez
`command: ["serve", "--ip", "192.168.1.20"]` au projet, puis *Action* → *Construire*, et `--ip 192.168.1.20` à la
commande `pair`. Avec un nom en plus, une seule ligne : `command: ["serve", "--name", "NAS du salon", "--ip",
"192.168.1.20"]`.

## 5. Confier les PC au hub

- **Un PC avec l'agent** : dans l'application, fiche du PC → ligne **Hub** → *Confier au hub*, PC allumé, depuis un
  appareil administrateur du PC et du hub. Le PC crée une invitation réservée à l'empreinte du hub, au rôle *Démarrer* ;
  le hub s'en sert pour s'appairer au PC. Le NAS lui-même se confie de la même façon si son agent est installé.
- **Un appareil sans agent** (box, imprimante, caméra…) : fiche du hub → *Ajouter un appareil*, avec une vérification
  par connexion à un port ou par page web. Le hub vérifie seulement qu'il répond.
- La fiche du hub liste ce qui lui est confié et l'état qu'il en voit ; *Retirer du hub* efface ce qu'il en gardait
  et retire le hub des appareils autorisés du PC.

## 6. Programmer un PC

Fiche du hub → bouton **horloge** d'un PC confié :

- **Le démarrer** : jours et heure (le hub connaît l'adresse MAC du PC) ;
- **L'éteindre s'il est inactif** : seulement si le processeur est resté sous 10 % pendant le dernier quart d'heure et
  qu'aucun appareil ne lui a rien demandé depuis une heure ; sinon le hub réessaie toutes les 15 minutes pendant
  2 heures. Extinction normale, jamais forcée. L'application donne pour cela au hub le rôle *Contrôler* sur ce PC, et
  le lui retire quand vous désactivez l'extinction.

Les heures suivent le **fuseau horaire du hub** (fiche du hub → *Programmations* ; Europe/Paris par défaut),
changements d'heure compris.

## 7. Mettre à jour

Le hub ne se met pas à jour seul. Quand une nouvelle version paraît : *Container Manager* → *Image* →
`ghcr.io/slaynaw/patronus-hub` → *Mettre à jour*, puis *Projet* → **patronus-hub** → *Action* → *Construire* (le
conteneur est recréé, les dossiers sont gardés). En SSH (selon la version de Container Manager, `docker-compose` ou
`docker compose`) :

```sh
cd /volume1/docker/patronus-hub && sudo docker-compose pull && sudo docker-compose up -d
```

`sudo docker exec patronus-hub patronus-hub version` affiche la version en marche.

## 8. Vérifier l'image (facultatif)

L'image est construite par la CI du dépôt à partir du même commit que les applications, et ne contient que le binaire
`patronus-hub` (aussi publié seul dans chaque version). Sa provenance se vérifie depuis un PC, avec
[GitHub CLI](https://cli.github.com) :

```sh
gh attestation verify oci://ghcr.io/slaynaw/patronus-hub:2.0.0 --repo slaynAW/Patronus
```

Pour n'installer que l'image vérifiée, épinglez-la par son empreinte dans le projet :
`image: ghcr.io/slaynaw/patronus-hub:2.0.0@sha256:…` (empreinte donnée par la commande ci-dessus).

## 9. À savoir

- **Sécurité** : le hub tourne sous l'utilisateur 65532, sans aucune capacité, système de fichiers en lecture seule,
  sans accès au socket Docker. Il n'accepte que les adresses du réseau local et les appareils appairés (TLS mutuel,
  [PROTOCOLE-HUB.md](PROTOCOLE-HUB.md)), et ne contacte aucun service sur Internet.
- **Restaurer une sauvegarde** du dossier de données (Hyper Backup) rétablit tout ce qu'il contenait à cette date,
  appareils autorisés compris : un téléphone retiré depuis serait de nouveau autorisé. Après une restauration, ouvrez
  la fiche du hub dans une application et retirez de ses appareils autorisés ceux qui ne devraient plus y être.
- **Hibernation des disques** : le hub écrit peu (journal d'un PC allumé toutes les 10 minutes s'il a changé,
  mesures une fois par jour, évènements quand ils arrivent ; rien pour un PC éteint), mais chaque écriture réveille
  un disque dur en hibernation.
- **Changer de NAS** : recopiez les deux dossiers (même propriétaire 65532, mode 700), puis recréez le projet. Le hub
  garde son identité, ses PC et son historique ; les PC n'ont rien à faire. Si le nouveau NAS a une autre adresse IP,
  ouvrez dans chaque application un lien de `patronus-hub pair` : elle retrouve le hub à sa nouvelle adresse.
- **Désinstaller** : d'abord, dans l'application, *Retirer du hub* chaque PC (le hub est ainsi retiré de leurs
  appareils autorisés ; sinon, sur chaque PC : `wol-agent devices revoke <empreinte du hub>`), puis *Oublier ce hub*.
  Ensuite *Projet* → **patronus-hub** → *Arrêter* → *Supprimer*, et en SSH
  `sudo rm -r /volume1/docker/patronus-hub /volume1/patronus-hub-keys`.
