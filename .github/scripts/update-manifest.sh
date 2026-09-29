#!/usr/bin/env bash
# Manifeste de mise à jour : dist/update.json (version, nouveautés, taille et empreinte de chaque
# fichier) et sa signature dist/update.json.sig (clé de signature de l'APK, RSA / SHA-256, base64).
# Les applications le lisent à l'adresse …/releases/latest/download/update.json et refusent tout
# manifeste dont la signature ne correspond pas au certificat qu'elles connaissent
# (Android : celui de l'application installée ; Windows : agent/update/release-cert.pem).
#
# La section « agent » décrit l'agent joint (numéro propre AGENT_VERSION, nouveautés de
# agent/NOUVEAUTES.md, binaires Windows) : les agents installés s'en servent pour se mettre à jour.
#
# Variables : VERSION, AGENT_VERSION, OFFICIAL (true/false), WOL_VERSION_CODE, KEYSTORE_BASE64,
# KEYSTORE_PASSWORD.
set -euo pipefail

base_version="${VERSION%%-*}"
# Section « ## X.Y.Z » de NOUVEAUTES.md, sans lignes vides au début.
notes=$(awk -v title="## $base_version" '$0 == title { found = 1; next } /^## / { if (found) exit } found' NOUVEAUTES.md | sed '/./,$!d')
if [[ -z "$notes" ]]; then
  if [[ "$OFFICIAL" == true ]]; then
    echo "::error title=Nouveautés::NOUVEAUTES.md ne contient pas de section « ## $base_version »."
    exit 1
  fi
  notes="Version de développement."
fi

# Nouveautés en tête des notes de la Release.
{ echo "## ✨ Nouveautés"; echo; echo "$notes"; echo; cat notes.md; } > notes.tmp && mv notes.tmp notes.md

describe() { # plateforme nom
  local path="dist/$2"
  [[ -f "$path" ]] || return 0
  jq -n --arg p "$1" --arg n "$2" --argjson s "$(stat -c %s "$path")" --arg h "$(sha256sum "$path" | cut -d' ' -f1)" \
    '{platform: $p, name: $n, size: $s, sha256: $h}'
}
files=$({
  describe android "Patronus-${VERSION}.apk"
  describe windows-amd64 "Patronus-Windows-${VERSION}-x64.exe"
  describe windows-arm64 "Patronus-Windows-${VERSION}-arm64.exe"
} | jq -s '.')

agent=null
agent_files=$({
  describe windows-amd64 wol-agent-windows-amd64.exe
  describe windows-arm64 wol-agent-windows-arm64.exe
} | jq -s '.')
if [[ -n "${AGENT_VERSION:-}" && "$agent_files" != "[]" ]]; then
  agent_notes=$(awk -v title="## ${AGENT_VERSION%%-*}" '$0 == title { found = 1; next } /^## / { if (found) exit } found' agent/NOUVEAUTES.md | sed '/./,$!d')
  if [[ -z "$agent_notes" && "$OFFICIAL" == true ]]; then
    echo "::error title=Nouveautés::agent/NOUVEAUTES.md ne contient pas de section « ## $AGENT_VERSION »."
    exit 1
  fi
  agent=$(jq -n --arg v "$AGENT_VERSION" --arg notes "$agent_notes" --argjson files "$agent_files" \
    '{version: $v, notes: $notes, files: $files}')
fi

jq -n --arg v "$VERSION" --argjson code "$WOL_VERSION_CODE" --arg d "$(date -u +%F)" --arg notes "$notes" --argjson files "$files" \
  --argjson agent "$agent" \
  '{format: 1, version: $v, code: $code, date: $d, notes: $notes, files: $files} + (if $agent == null then {} else {agent: $agent} end)' > update.json
cat update.json

if [[ -z "${KEYSTORE_BASE64:-}" || -z "${KEYSTORE_PASSWORD:-}" ]]; then
  if [[ "$OFFICIAL" == true ]]; then
    echo "::error title=Mises à jour::Secrets de signature absents : impossible de signer le manifeste de mise à jour."
    exit 1
  fi
  echo "::warning title=Mises à jour::Secrets de signature absents : pas de manifeste de mise à jour."
  exit 0
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
printf '%s' "$KEYSTORE_BASE64" | tr -d ' \t\r\n' | base64 -d > "$work/release.p12"
openssl pkcs12 -in "$work/release.p12" -nocerts -nodes -passin env:KEYSTORE_PASSWORD -out "$work/key.pem" 2>/dev/null
openssl pkcs12 -in "$work/release.p12" -clcerts -nokeys -passin env:KEYSTORE_PASSWORD 2>/dev/null | openssl x509 -out "$work/cert.pem"
openssl dgst -sha256 -sign "$work/key.pem" -out "$work/update.sig" update.json
openssl dgst -sha256 -verify <(openssl x509 -in "$work/cert.pem" -pubkey -noout) -signature "$work/update.sig" update.json

# Le certificat intégré à l'application Windows doit être celui de la clé de signature.
committed=agent/update/release-cert.pem # application Windows et agent
if ! cmp -s <(openssl x509 -in "$committed" -outform DER 2>/dev/null) <(openssl x509 -in "$work/cert.pem" -outform DER); then
  echo "::warning title=Mises à jour::$committed ne correspond pas à la clé de signature de l'APK : l'application Windows refuserait les mises à jour. Certificat (public) à y placer :"
  cat "$work/cert.pem"
  if [[ "$OFFICIAL" == true ]]; then
    exit 1
  fi
fi

# Joints seulement aux versions officielles (les applications ne lisent que la dernière d'entre elles).
if [[ "$OFFICIAL" == true ]]; then
  cp update.json dist/update.json
  base64 -w0 "$work/update.sig" > dist/update.json.sig
fi
