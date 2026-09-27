# Configurer les PC pour le Wake-on-LAN

Le Wake-on-LAN (WoL) permet à la **carte réseau** d'un PC éteint ou en veille de rester alimentée et d'écouter le réseau :
à la réception du « paquet magique » contenant son adresse MAC, elle allume le PC.

## Conditions indispensables

- **Câble Ethernet.** Le réveil par Wi-Fi (« WoWLAN ») n'existe que sur de rares portables et ne fonctionne pas depuis l'état éteint.
- **Alimentation branchée** (secteur, pas seulement batterie pour un portable ; multiprise allumée).
- **Même réseau local** que le téléphone (même box / même sous-réseau). Le réveil à distance est prévu plus tard (voir [AMELIORATIONS.md](AMELIORATIONS.md)).
- **Adresse IP fixe** conseillée : dans la box, réservez l'adresse IP du PC (bail DHCP statique) pour que l'état et l'agent la retrouvent toujours.

## 1. BIOS / UEFI

Au démarrage, appuyez sur `Suppr`, `F2` ou `F10` (selon la marque), puis cherchez (souvent dans *Power Management* / *APM* / *Advanced*) :

| Nom possible | Valeur |
|---|---|
| Wake on LAN, Power On By PCI-E / PCI, Resume by LAN, Remote Wake Up | **Enabled** |
| ErP Ready / EuP 2013 / Deep Sleep / Deep Power Off | **Disabled** (ces modes coupent la carte réseau) |
| AC Back / Restore on AC Power Loss | au choix (utile pour les serveurs) |

Sur certains PC de bureau (Dell, HP, Lenovo), l'option s'appelle *Wake on LAN / WLAN* avec les choix *LAN Only*.

## 2. Windows 10 / 11

1. **Gestionnaire de périphériques** (`Win + X`) → *Cartes réseau* → votre carte **Ethernet** (Intel, Realtek…) → *Propriétés* :
   - Onglet **Gestion de l'alimentation** : cocher *Autoriser ce périphérique à sortir l'ordinateur du mode veille* et *N'autoriser que le paquet magique…*.
   - Onglet **Avancé** : *Wake on Magic Packet* = **Activé** ; *Shutdown Wake-On-Lan* / *Wake from S5* = **Activé** (si présent) ;
     *Energy Efficient Ethernet* / *Green Ethernet* = **Désactivé** (recommandé).
2. **Désactiver le démarrage rapide** : Panneau de configuration → *Options d'alimentation* → *Choisir l'action des boutons d'alimentation* →
   *Modifier des paramètres actuellement non disponibles* → décocher **Activer le démarrage rapide**.
   > Le démarrage rapide met le PC dans un état hybride dans lequel beaucoup de cartes réseau ne se réveillent pas.
   > L'agent, lui, déclenche toujours un arrêt complet.
3. **Adresse MAC** : `ipconfig /all` → *Adresse physique* de la carte Ethernet (ex. `AA-BB-CC-DD-EE-FF`). L'agent l'affiche aussi.
4. **Profil réseau** : Paramètres → Réseau → Ethernet → **Réseau privé** (l'agent n'est autorisé par le pare-feu que sur les réseaux privés).

## 3. Linux

```bash
# Nom de la carte et adresse MAC
ip link
# État actuel du WoL (« g » = paquet magique activé)
sudo ethtool enp3s0 | grep Wake-on
# Activer (temporaire)
sudo ethtool -s enp3s0 wol g
# Activer de façon permanente avec NetworkManager
nmcli connection show
sudo nmcli connection modify "Connexion filaire 1" 802-3-ethernet.wake-on-lan magic
```

Avec `systemd-networkd`, ajoutez `WakeOnLan=magic` dans la section `[Link]` d'un fichier `.link`.

## 4. macOS

Réglages Système → *Batterie* / *Économiseur d'énergie* → **Réveiller pour l'accès au réseau** (Mac de bureau, en veille uniquement).

## 5. Vérifier

1. Mettez d'abord le PC **en veille**, puis appuyez sur **Démarrer** dans l'application : c'est le cas le plus simple.
2. Ensuite, éteignez-le complètement et recommencez.
3. Si rien ne se passe : les voyants de la prise réseau du PC doivent rester allumés quand le PC est éteint.
   S'ils sont éteints, la carte n'est pas alimentée (BIOS / ErP / démarrage rapide).

## Cas particuliers

- **Plusieurs réseaux / VLAN** : le broadcast ne traverse pas les routeurs. Renseignez dans *Options avancées* l'adresse de diffusion
  du réseau du PC (ex. `192.168.20.255`) si votre routeur autorise le « directed broadcast ».
- **Box qui filtre le broadcast Wi-Fi → Ethernet** : rare, mais certains répéteurs ou points d'accès en mode « isolation » bloquent le paquet.
  Désactivez l'isolation des clients Wi-Fi.
- **Mot de passe SecureOn** : quelques cartes (anciennes Intel / Broadcom) exigent un mot de passe de 6 octets ; renseignez-le dans *Options avancées*.
