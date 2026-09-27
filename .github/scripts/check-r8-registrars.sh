#!/usr/bin/env bash
# Vérifie que R8 n'a pas supprimé les constructeurs des composants ML Kit / Firebase
# (« ComponentRegistrar ») : ils sont instanciés par réflexion à partir du manifeste.
# S'ils disparaissent, le lecteur de QR code de Google plante dans l'APK de publication.
#
# Usage : check-r8-registrars.sh [--strict]   (--strict : échec si un problème est trouvé)
set -uo pipefail

strict=false
[ "${1:-}" = "--strict" ] && strict=true

usage=$(find app/build/outputs/mapping -name usage.txt 2>/dev/null | head -1)
manifest=$(find app/build/intermediates -path '*merged_manifest*' -path '*elease*' -name AndroidManifest.xml 2>/dev/null | head -1)
echo "Rapport R8 (code supprimé) : ${usage:-introuvable}"
echo "Manifeste fusionné         : ${manifest:-introuvable}"
if [ -z "$usage" ] || [ -z "$manifest" ]; then
  echo "::warning title=R8::Fichiers de diagnostic introuvables, contrôle ignoré."
  exit 0
fi

registrars=$(grep -o 'com\.google\.firebase\.components:[A-Za-z0-9_.$]*' "$manifest" | cut -d: -f2 | sort -u)
if [ -z "$registrars" ]; then
  echo "Aucun composant déclaré dans le manifeste."
  exit 0
fi

problems=0
for class in $registrars; do
  removed=$(awk -v c="$class:" '$0 == c { p = 1; next } p && /^[^ \t]/ { p = 0 } p' "$usage")
  if grep -qx "$class" "$usage"; then
    echo "✗ $class : classe entière supprimée"
    problems=$((problems + 1))
  elif echo "$removed" | grep -q '<init>'; then
    echo "✗ $class : constructeur supprimé"
    echo "$removed" | sed 's/^/      /'
    problems=$((problems + 1))
  else
    echo "✓ $class"
  fi
done

if [ "$problems" -gt 0 ]; then
  msg="$problems composant(s) ML Kit/Firebase cassé(s) par R8 : le scan de QR code planterait."
  if $strict; then echo "::error title=R8::$msg"; exit 1; fi
  echo "::warning title=R8::$msg"
fi
exit 0
