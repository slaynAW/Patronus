# Agent sur un NAS Synology (DSM 7)

L'agent s'installe aussi sur un NAS Synology sous **DSM 7** (agent 1.12.0 ou plus). Le NAS apparaît alors dans les
applications comme un PC :

- **état** fiable, utilisation du processeur, température du processeur quand DSM la fournit au noyau ;
- **volumes** (espace libre) et disques (modèle, type) ; leur santé S.M.A.R.T. reste dans le Gestionnaire de stockage de DSM ;
- **fiche** : modèle du NAS (« Synology DS218+ »), version de DSM, processeur, mémoire ;
- **journal** des démarrages et arrêts sur 30 jours, **mesures** sur 90 jours ;
- **extinction** et **redémarrage** par l'outil de DSM, **allumage** par Wake-on-LAN. La veille n'existe pas sur un NAS.

DSM 6 n'est pas pris en charge (pas de systemd).

## 1. Préparer DSM

1. **Adresse IP fixe** : réservez l'adresse du NAS dans votre box (bail DHCP fixe), ou fixez-la dans *Panneau de
   configuration* → *Réseau* → *Interface réseau*.
2. **Wake-on-LAN** : *Panneau de configuration* → *Matériel et alimentation* → *Général* → cochez le **WOL du réseau
   local 1** (« Enable WOL on LAN 1 » en anglais ; et du 2 si le NAS en a deux et que vous l'utilisez).
3. **SSH**, le temps de l'installation : *Panneau de configuration* → *Terminal et SNMP* → **Activer le service SSH**.
4. **Pare-feu de DSM**, s'il est activé (*Panneau de configuration* → *Sécurité* → *Pare-feu*) : ajoutez une règle qui
   autorise le **port TCP 9770** depuis votre réseau local seulement (par exemple `192.168.1.0`, masque
   `255.255.255.0`), placée avant les règles de refus. N'ouvrez jamais ce port sur votre box.

## 2. Installer

Connectez-vous en SSH avec un compte administrateur (`ssh nom@192.168.1.x`), puis passez en root : `sudo -i`.

```sh
uname -m    # x86_64 : wol-agent-linux-amd64 ; aarch64 : wol-agent-linux-arm64 ; armv7l : wol-agent-linux-arm
```

Les commandes ci-dessous prennent `wol-agent-linux-amd64` (DS218+, DS220+, DS920+… : processeur Intel ou AMD) ;
remplacez le nom si `uname -m` en indique un autre.

```sh
mkdir -p /volume1/patronus-agent
chmod 700 /volume1/patronus-agent
ln -s /volume1/patronus-agent /etc/wol-agent
cd /volume1/patronus-agent
curl -fLO https://github.com/slaynAW/Patronus/releases/latest/download/wol-agent-linux-amd64
curl -fLO https://github.com/slaynAW/Patronus/releases/latest/download/SHA256SUMS.txt
grep ' wol-agent-linux-amd64$' SHA256SUMS.txt | sha256sum -c -
chmod 700 wol-agent-linux-amd64
./wol-agent-linux-amd64 install --name "NAS"
```

- **`/volume1/patronus-agent`** garde les données de l'agent (son identité, les appareils autorisés, le journal, les
  mesures) sur le volume : une mise à jour de DSM peut effacer `/etc`, pas vos volumes. Ce n'est pas un dossier
  partagé : il n'est pas visible sur le réseau (n'en faites pas un dossier partagé). Le lien `/etc/wol-agent` y mène.
- `sha256sum -c` doit répondre **OK** : le fichier est intact. Sa provenance se vérifie aussi depuis un PC
  ([SECURITY.md](../SECURITY.md#vérifier-une-version)).
- À la fin, un **QR code** s'affiche : sur le téléphone, *Ajouter un PC* → *Scanner le QR code*, puis *Enregistrer*.
  Application Windows : *Ajouter un PC* → *Coller le lien*, avec le lien `wolagent://…` affiché sous le QR code, ou
  invitez cet ordinateur depuis le téléphone (fiche du NAS → *Appareils autorisés*). Le code ne sert qu'une fois,
  pendant 10 minutes : `wol-agent pair` en affiche un autre.
- Si le QR code annonce une autre adresse que celle du NAS sur votre réseau (Docker, VPN…) :
  `wol-agent pair --ip 192.168.1.x`.

## 3. Garder l'agent après une mise à jour de DSM

Une mise à jour de DSM peut retirer le service. Une tâche au démarrage le remet en place, avec sa configuration :

*Panneau de configuration* → *Planificateur de tâches* → *Créer* → *Tâche déclenchée* → *Script défini par
l'utilisateur* :

- *Général* : nom « Patronus », utilisateur **root**, évènement **Démarrage** ;
- *Paramètres de tâche* → *Script défini par l'utilisateur* :

```sh
[ -e /etc/wol-agent ] || ln -s /volume1/patronus-agent /etc/wol-agent
/volume1/patronus-agent/wol-agent-linux-amd64 install --no-pair
```

`--no-pair` réinstalle sans créer de code d'appairage : rien de secret dans le résultat de la tâche. Une fois la tâche
créée, vous pouvez **désactiver SSH**.

## 4. Éteindre, redémarrer, allumer

- **Éteindre** et **Redémarrer** passent par l'outil de DSM (`synopoweroff`), comme le menu de DSM : les paquets et
  les volumes s'arrêtent proprement. DSM refuse pendant une opération critique (mise à jour, réparation d'un
  volume…) : le NAS reste alors allumé. *Forcer* est sans effet sur un NAS.
- **Allumer** : Wake-on-LAN, depuis l'application, comme un PC (étape 1.2). Après une coupure de courant, voyez la
  reprise automatique de l'alimentation dans le même panneau de DSM.
- Faites un premier essai avec **Redémarrer**, à un moment où personne n'utilise le NAS.

## 5. À savoir

- **Hibernation des disques** : l'agent écrit quelques octets chaque minute (mesures, signe de vie) ; tant qu'il
  tourne, les disques ne passent plus en hibernation.
- **Température** : celle du processeur si DSM charge son capteur (`coretemp` sur les processeurs Intel) ; sinon
  elle n'apparaît pas. DSM affiche l'état thermique dans *Panneau de configuration* → *Centre d'infos*.
- **Mises à jour** : pas de mise à jour automatique en dehors de Windows. Quand la fiche du NAS signale une nouvelle
  version, refaites les commandes `curl`, `sha256sum` et `chmod` de l'étape 2 dans `/volume1/patronus-agent`, puis
  `./wol-agent-linux-amd64 install --no-pair` (ou redémarrez le NAS).
- **Sécurité** : l'agent tourne en root (il faut l'être pour éteindre), dans un service systemd durci qui n'écrit que
  dans son dossier ; il n'accepte que les adresses du réseau local, et seulement les appareils appairés (TLS mutuel,
  [SECURITE.md](SECURITE.md)). Pour piloter le NAS hors de chez vous, passez par un VPN, jamais par une redirection
  de port.
- **Désinstaller** : `sudo wol-agent uninstall`, supprimez la tâche du planificateur, puis `rm /etc/wol-agent` et
  `rm -r /volume1/patronus-agent`.
