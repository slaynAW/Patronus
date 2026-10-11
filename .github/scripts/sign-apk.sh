#!/usr/bin/env bash
# Signe un APK avec apksigner (outils du SDK Android) puis vérifie le résultat : signature v2 et v3
# valide, un seul signataire qui est bien la clé fournie, alignement d'origine conservé.
#
# Le job « Application Android » n'a jamais accès à la clé définitive : il produit un APK signé avec
# une clé de test. Seul le job « Publication officielle » (environnement GitHub « publication », qui
# attend ta validation) reçoit la clé et appelle ce script. Le job Android l'appelle aussi à chaque
# build avec une clé jetable, pour qu'un problème de signature ne se découvre pas le jour d'une version.
#
# Usage : sign-apk.sh <APK d'entrée> <APK signé> <magasin PKCS12> <alias>
# Mot de passe du magasin et de la clé : variable KEYSTORE_PASSWORD (jamais en argument, visible
# dans la liste des processus).
set -euo pipefail

in=$1 out=$2 keystore=$3 alias=$4
: "${KEYSTORE_PASSWORD:?variable KEYSTORE_PASSWORD absente}"

sdk=${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}
tools=$({ find "$sdk/build-tools" -mindepth 1 -maxdepth 1 -type d 2>/dev/null || true; } | sort -V | tail -n 1)
if [[ -z "$tools" || ! -x "$tools/apksigner" ]]; then
  echo "::error title=Signature::apksigner introuvable (build-tools du SDK Android)."
  exit 1
fi
echo "Outils : $tools"

# Mêmes schémas que la configuration Gradle de publication (minSdk 26 : pas de v1).
"$tools/apksigner" sign --ks "$keystore" --ks-type PKCS12 --ks-key-alias "$alias" \
  --ks-pass env:KEYSTORE_PASSWORD --key-pass env:KEYSTORE_PASSWORD \
  --v1-signing-enabled false --v2-signing-enabled true --v3-signing-enabled true \
  --out "$out" "$in"

report=$("$tools/apksigner" verify --verbose --print-certs "$out")
grep -E '^(Verified using v[23] |Number of signers|Signer #1 certificate (DN|SHA-256))' <<< "$report" || true
for line in 'Verified using v2 scheme (APK Signature Scheme v2): true' \
            'Verified using v3 scheme (APK Signature Scheme v3): true' \
            'Number of signers: 1'; do
  if ! grep -qxF "$line" <<< "$report"; then
    echo "::error title=Signature::Vérification de l'APK signé : « $line » attendu."
    exit 1
  fi
done

signer=$(sed -n 's/^Signer #1 certificate SHA-256 digest: //p' <<< "$report")
if ! expected=$(openssl pkcs12 -in "$keystore" -clcerts -nokeys -passin env:KEYSTORE_PASSWORD 2>/dev/null \
    | openssl x509 -outform DER | sha256sum | cut -d' ' -f1); then
  echo "::error title=Signature::Certificat illisible dans le magasin de clés (mot de passe ?)."
  exit 1
fi
if [[ -z "$signer" || "$signer" != "$expected" ]]; then
  echo "::error title=Signature::L'APK n'est pas signé par le certificat de la clé fournie."
  exit 1
fi

# Alignement : chaque fichier non compressé garde au moins celui de l'APK d'origine (pages de 16 Kio
# pour les bibliothèques natives, sans quoi elles ne se chargent pas sur les appareils à pages de 16 Kio).
python3 -I - "$in" "$out" <<'PY'
import struct, sys, zipfile

def data_offsets(path):
    offsets = {}
    with open(path, "rb") as f, zipfile.ZipFile(path) as z:
        for info in z.infolist():
            if info.compress_type != zipfile.ZIP_STORED:
                continue
            f.seek(info.header_offset)
            name_len, extra_len = struct.unpack("<HH", f.read(30)[26:30])
            offsets[info.filename] = info.header_offset + 30 + name_len + extra_len
    return offsets

def alignment(offset):
    return next((a for a in (16384, 4096, 4) if offset % a == 0), 1)

before, after = data_offsets(sys.argv[1]), data_offsets(sys.argv[2])
bad = [n for n, o in before.items() if n not in after or alignment(after[n]) < alignment(o)]
if bad:
    print("::error title=Signature::Alignement perdu après signature : " + ", ".join(sorted(bad)[:5]))
    sys.exit(1)
libs = sorted({alignment(o) for n, o in after.items() if n.endswith(".so")})
print(f"Alignement conservé ({len(after)} fichiers non compressés, bibliothèques natives : {libs or 'aucune'}).")
PY
echo "APK signé et vérifié (certificat SHA-256 $signer)."
