#!/usr/bin/env python3
"""Image de conteneur du hub (docs/HUB.md, §3 et §10), au format OCI, sans démon ni image de base.

Une seule couche : le binaire statique patronus-hub de l'architecture (linux/amd64, linux/arm64), à la
racine, mode 0555, propriétaire 0:0 (le hub tourne sous l'utilisateur 65532 et ne peut pas se modifier).
Utilisateur 65532:65532, port 9771, volumes /data et /keys, commande « serve ». Rien d'autre : pas de
shell, pas de gestionnaire de paquets, pas de certificats (le hub ne contacte aucun service Internet).

Mêmes binaires et même date (SOURCE_DATE_EPOCH, celle du commit) : mêmes octets, donc même empreinte.

Usage : hub-image.py SORTIE VERSION RÉVISION amd64=BINAIRE arm64=BINAIRE
Écrit le dossier OCI SORTIE (oci-layout, index.json, blobs/sha256/…), dont l'index multi-architecture
porte le nom VERSION (skopeo : « oci:SORTIE:VERSION »), et affiche l'empreinte de cet index : celle de
l'image publiée, attestée par le job de publication.
"""

import gzip
import hashlib
import io
import json
import os
import re
import sys
import tarfile
import time

SOURCE = "https://github.com/slaynAW/Patronus"
PLATFORMS = {"amd64": {"architecture": "amd64", "os": "linux"}, "arm64": {"architecture": "arm64", "os": "linux", "variant": "v8"}}
OCI_INDEX = "application/vnd.oci.image.index.v1+json"
OCI_MANIFEST = "application/vnd.oci.image.manifest.v1+json"
OCI_CONFIG = "application/vnd.oci.image.config.v1+json"
OCI_LAYER = "application/vnd.oci.image.layer.v1.tar+gzip"


def canonical(obj):
    """JSON compact, clés triées : les mêmes données donnent toujours les mêmes octets."""
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


class Layout:
    def __init__(self, root):
        self.root = root
        os.makedirs(os.path.join(root, "blobs", "sha256"), exist_ok=True)

    def blob(self, data, media_type):
        digest = hashlib.sha256(data).hexdigest()
        path = os.path.join(self.root, "blobs", "sha256", digest)
        with open(path, "wb") as f:
            f.write(data)
        return {"mediaType": media_type, "digest": "sha256:" + digest, "size": len(data)}


def layer(binary, mtime):
    """Couche (tar, puis gzip) contenant seulement /patronus-hub ; renvoie (tar, tar.gz)."""
    with open(binary, "rb") as f:
        data = f.read()
    if not data.startswith(b"\x7fELF"):
        sys.exit(f"{binary} : ce n'est pas un binaire Linux (ELF)")
    raw = io.BytesIO()
    with tarfile.open(fileobj=raw, mode="w", format=tarfile.USTAR_FORMAT) as tar:
        info = tarfile.TarInfo("patronus-hub")
        info.size = len(data)
        info.mode = 0o555
        info.uid = info.gid = 0
        info.uname = info.gname = ""
        info.mtime = mtime
        tar.addfile(info, io.BytesIO(data))
    plain = raw.getvalue()
    packed = io.BytesIO()
    # Ni nom de fichier ni date dans l'en-tête gzip.
    with gzip.GzipFile(filename="", mode="wb", fileobj=packed, mtime=0, compresslevel=9) as gz:
        gz.write(plain)
    return plain, packed.getvalue()


def main():
    if len(sys.argv) < 5:
        sys.exit(__doc__)
    out, version, revision = sys.argv[1], sys.argv[2], sys.argv[3]
    binaries = dict(arg.split("=", 1) for arg in sys.argv[4:])
    if set(binaries) != set(PLATFORMS):
        sys.exit("architectures attendues : " + ", ".join(sorted(PLATFORMS)))
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?", version):
        sys.exit(f"version invalide : {version}")
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        sys.exit(f"révision invalide (empreinte de commit attendue) : {revision}")
    epoch = int(os.environ.get("SOURCE_DATE_EPOCH", "0"))
    created = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(epoch))
    labels = {
        "org.opencontainers.image.title": "patronus-hub",
        "org.opencontainers.image.description": "Hub Patronus sur le NAS : surveillance continue des PC, historique long, programmations (docs/HUB.md).",
        "org.opencontainers.image.source": SOURCE,
        "org.opencontainers.image.url": SOURCE + "/blob/main/docs/HUB.md",
        "org.opencontainers.image.version": version,
        "org.opencontainers.image.revision": revision,
        "org.opencontainers.image.created": created,
    }
    layout = Layout(out)
    manifests = []
    for arch in sorted(PLATFORMS):
        platform = PLATFORMS[arch]
        plain, packed = layer(binaries[arch], epoch)
        config = {
            "created": created,
            **platform,
            "config": {
                "User": "65532:65532",
                # « docker exec patronus-hub patronus-hub pair » (lien d'appairage suivant) trouve le binaire.
                "Env": ["PATH=/"],
                "Entrypoint": ["/patronus-hub"],
                "Cmd": ["serve"],
                "ExposedPorts": {"9771/tcp": {}},
                "Volumes": {"/data": {}, "/keys": {}},
                "WorkingDir": "/",
                "Labels": labels,
            },
            "rootfs": {"type": "layers", "diff_ids": ["sha256:" + hashlib.sha256(plain).hexdigest()]},
            "history": [{"created": created, "created_by": "patronus-hub (.github/scripts/hub-image.py)"}],
        }
        manifest = {
            "schemaVersion": 2,
            "mediaType": OCI_MANIFEST,
            "config": layout.blob(canonical(config), OCI_CONFIG),
            "layers": [layout.blob(packed, OCI_LAYER)],
            "annotations": labels,
        }
        desc = layout.blob(canonical(manifest), OCI_MANIFEST)
        desc["platform"] = platform
        manifests.append(desc)
    index = {"schemaVersion": 2, "mediaType": OCI_INDEX, "manifests": manifests, "annotations": labels}
    top = layout.blob(canonical(index), OCI_INDEX)
    top["annotations"] = {"org.opencontainers.image.ref.name": version}
    with open(os.path.join(out, "index.json"), "wb") as f:
        f.write(canonical({"schemaVersion": 2, "mediaType": OCI_INDEX, "manifests": [top]}))
    with open(os.path.join(out, "oci-layout"), "wb") as f:
        f.write(canonical({"imageLayoutVersion": "1.0.0"}))
    print(top["digest"])


if __name__ == "__main__":
    main()
