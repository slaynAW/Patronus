#!/usr/bin/env bash
# Notes de la Release (notes.md) et sommes de contrôle (dist/SHA256SUMS.txt), pour une version
# officielle comme pour la pré-version de développement. update-manifest.sh ajoute ensuite les
# nouveautés en tête des notes.
#
# Variables : VERSION, AGENT_VERSION, OFFICIAL (true/false).
set -euo pipefail

(cd dist && sha256sum -- * > SHA256SUMS.txt)
blob="${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/blob/${GITHUB_SHA}"
{
  echo "## 📱 Installer l'application"
  echo "Depuis le téléphone, téléchargez **Patronus-${VERSION}.apk** ci-dessous puis ouvrez-le"
  echo "(autorisez l'installation depuis le navigateur si Android le demande)."
  echo
  if [[ "$OFFICIAL" != true ]]; then
    echo "> ⚠️ Version de développement : APK signé avec une **clé de test**, pas avec la clé des versions officielles."
    echo "> Il ne s'installe pas par-dessus l'application officielle : il faut d'abord la désinstaller (exportez votre configuration avant)."
    echo "> Voir [docs/SIGNATURE.md](${blob}/docs/SIGNATURE.md)."
    echo
  fi
  echo "## 🖥️ Application Windows (mêmes fonctions que l'application Android)"
  echo "Téléchargez **Patronus-Windows-${VERSION}-x64.exe** (ou \`-arm64.exe\` pour un PC ARM) et lancez-le :"
  echo "aucune installation, un seul fichier. Windows 10/11 (composant WebView2, déjà présent)."
  echo "Au premier lancement, SmartScreen peut afficher « Windows a protégé votre ordinateur » :"
  echo "*Informations complémentaires* → *Exécuter quand même*."
  echo
  echo "## 💻 Agent PC ${AGENT_VERSION} (facultatif, pour éteindre à distance)"
  echo "- **Windows** : \`wol-agent-windows-amd64.exe\` → double-clic → accepter l'invite administrateur → scanner le QR code."
  echo "- **Linux** : \`chmod +x wol-agent-linux-amd64 && sudo ./wol-agent-linux-amd64 install\`"
  echo "- **macOS** : \`chmod +x wol-agent-darwin-arm64 && sudo ./wol-agent-darwin-arm64 install\`"
  echo
  echo "## 🔏 Vérifier les fichiers"
  echo "- Sommes de contrôle : \`SHA256SUMS.txt\`. Commit : ${GITHUB_SHA}."
  if [[ "$OFFICIAL" == true ]]; then
    echo "- Provenance (compilé par la CI de ce dépôt, depuis ce commit) : \`gh attestation verify <fichier> --repo ${GITHUB_REPOSITORY}\`."
  fi
  echo "- Inventaire des composants (SBOM CycloneDX) : fichiers \`.cdx.json\`."
  echo "- Détails : [SECURITY.md](${blob}/SECURITY.md)."
} > notes.md
ls -l dist
