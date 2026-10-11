#!/usr/bin/env bash
# Inventaire des composants (SBOM CycloneDX) d'un module Go du dépôt : modules tiers embarqués, tous
# systèmes confondus, et bibliothèque standard de Go (version qui compile les binaires publiés).
# Nos propres modules y portent le numéro de version publié plutôt qu'une pseudo-version tirée de git.
#
# Usage : sbom-go.sh <dossier du module> <fichier de sortie> <chemin de module>=<version>...
# Exige cyclonedx-gomod dans le PATH (version épinglée par le workflow).
set -euo pipefail

dir=$1 out=$2
shift 2
versions=$(for pair in "$@"; do jq -n --arg k "${pair%%=*}" --arg v "${pair#*=}" '{($k): $v}'; done | jq -s 'add // {}')

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
(cd "$dir" && cyclonedx-gomod mod -json -std -short-purls -output "$tmp" .)

jq --argjson m "$versions" '
  def ours:
    if type == "string" and test("^pkg:golang/[^@]+@") then
      capture("^pkg:golang/(?<path>[^@]+)@(?<version>[^?]*)(?<rest>.*)$") as $p
      | if $m[$p.path] != null then "pkg:golang/\($p.path)@\($m[$p.path])\($p.rest)" else . end
    else . end;
  walk(if type == "object" then
      (if has("bom-ref") then .["bom-ref"] |= ours else . end)
      | (if has("purl") then .purl |= ours else . end)
      | (if has("ref") then .ref |= ours else . end)
      | (if has("dependsOn") then .dependsOn |= map(ours) else . end)
      | (if has("version") and (.name | type) == "string" and $m[.name] != null then .version = $m[.name] else . end)
    else . end)' "$tmp" > "$out"

# Garde-fous : le composant principal est bien le nôtre, nos modules portent le numéro publié et la
# bibliothèque standard est présente.
jq -e --argjson m "$versions" '
  ($m[.metadata.component.name] != null)
  and ([.metadata.component, (.components // [])[] | select($m[.name] != null and .version != $m[.name])] | length == 0)
  and ([.components[]? | select(.name == "std")] | length == 1)' "$out" > /dev/null || {
  echo "::error title=Inventaire::Inventaire des composants incomplet ou incohérent : $out"
  exit 1
}
jq -r '"\(.metadata.component.name) \(.metadata.component.version) : " + ([.components[] | "\(.name) \(.version)"] | join(", "))' "$out"
