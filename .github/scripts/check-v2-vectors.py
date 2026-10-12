#!/usr/bin/env python3
"""Recalcule indépendamment les vecteurs de test de wolagent/2 (protocol/v2-vectors.json).

Seconde implémentation, écrite d'après docs/PROTOCOLE-V2.md et non d'après le code Go
(agent/protocol2) : bibliothèque standard de Python, et OpenSSL en ligne de commande pour relire
les certificats. Une erreur dans l'une des deux implémentations fait échouer la CI.

Usage : check-v2-vectors.py protocol/v2-vectors.json
"""
import base64
import hashlib
import hmac
import json
import re
import subprocess
import sys
import urllib.parse

CROCKFORD = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
B64_CHARS = re.compile(r"[A-Za-z0-9_-]*")
# SubjectPublicKeyInfo P-256 standard : id-ecPublicKey, prime256v1, point non compressé (§2.2).
P256_SPKI_PREFIX = bytes.fromhex("3059" "3013" "06072a8648ce3d0201" "06082a8648ce3d030107" "034200" "04")
LINK_PREFIX = "wolagent://pair?"
ROLES = ["view", "wake", "control", "admin"]
# Tableau du §5 : rôle minimal de chaque commande ; pair et migrate sont réservées à l'appairage.
MIN_ROLE = {
    "status": "view", "history": "view", "metrics": "view", "specs": "view",
    "wakes": "wake",
    "shutdown": "control", "reboot": "control", "sleep": "control",
    "devices": "admin", "devices.revoke": "admin", "devices.role": "admin", "devices.rename": "admin",
    "invite": "admin", "invites": "admin", "invites.cancel": "admin",
}
PAIRING = ("pair", "migrate")
IPV4 = re.compile(r"((25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])")
IPV6 = re.compile(r"[0-9A-Fa-f:.]+(%[A-Za-z0-9_.\-]+)?")
LABEL = re.compile(r"[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?")
MAC = re.compile(r"[0-9A-Fa-f]{2}([:-])[0-9A-Fa-f]{2}(\1[0-9A-Fa-f]{2}){4}")

failures = []


def check(condition, message):
    if not condition:
        failures.append(message)


def b64_encode(raw):
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def b64_decode(text, size=None):
    """B64 sous sa seule écriture admise : alphabet URL, sans remplissage, bits de fin nuls."""
    if not B64_CHARS.fullmatch(text) or len(text) % 4 == 1:
        raise ValueError(text)
    raw = base64.urlsafe_b64decode(text + "=" * (-len(text) % 4))
    if b64_encode(raw) != text or (size is not None and len(raw) != size):
        raise ValueError(text)
    return raw


# --- Empreintes (§2) -----------------------------------------------------------------------------

def der_element(data, pos):
    """Élément DER à pos : (étiquette, début du contenu, fin)."""
    tag, length = data[pos], data[pos + 1]
    pos += 2
    if length & 0x80:
        count = length & 0x7F
        if not 1 <= count <= 3:
            raise ValueError("longueur DER")
        length = int.from_bytes(data[pos:pos + count], "big")
        pos += count
    if pos + length > len(data):
        raise ValueError("élément DER tronqué")
    return tag, pos, pos + length


def certificate_spki(der):
    """SubjectPublicKeyInfo d'un certificat X.509, tel qu'il y est écrit."""
    tag, start, end = der_element(der, 0)
    if tag != 0x30 or end != len(der):
        raise ValueError("Certificate")
    tag, pos, _ = der_element(der, start)  # tbsCertificate
    if tag != 0x30:
        raise ValueError("tbsCertificate")
    tag, _, after = der_element(der, pos)
    if tag == 0xA0:  # version
        pos = after
    for _ in range(5):  # serialNumber, signature, issuer, validity, subject
        pos = der_element(der, pos)[2]
    tag, _, end = der_element(der, pos)
    if tag != 0x30:
        raise ValueError("subjectPublicKeyInfo")
    return der[pos:end]


def openssl_spki(der):
    """SubjectPublicKeyInfo selon OpenSSL, ou None si OpenSSL ne lit pas le certificat."""
    result = subprocess.run(["openssl", "x509", "-inform", "DER", "-noout", "-pubkey"],
                            input=der, capture_output=True, check=False)
    if result.returncode != 0:
        return None
    lines = result.stdout.decode().splitlines()
    return base64.b64decode("".join(line for line in lines if line and not line.startswith("-----")))


def short_pin(pin):
    bits = "".join(f"{byte:08b}" for byte in pin)[:60]
    chars = "".join(CROCKFORD[int(bits[i:i + 5], 2)] for i in range(0, 60, 5))
    return "-".join(chars[i:i + 4] for i in range(0, 12, 4))


def check_pins(pins):
    by_pin = {}
    for case in pins["valid"]:
        der = b64_decode(case["certificate"])
        spki = certificate_spki(der)
        check(len(spki) == 91 and spki.startswith(P256_SPKI_PREFIX), f"empreintes : {case['name']} : pas une clé P-256")
        check(openssl_spki(der) == spki, f"empreintes : {case['name']} : OpenSSL lit une autre clé")
        pin = hashlib.sha256(spki).digest()
        check(b64_encode(pin) == case["pin"], f"empreintes : {case['name']} : {b64_encode(pin)}")
        check(short_pin(pin) == case["short"], f"empreintes : {case['name']} : courte {short_pin(pin)}")
        by_pin.setdefault(case["pin"], set()).add(case["certificate"])
    check(any(len(certs) > 1 for certs in by_pin.values()), "empreintes : pas deux certificats d'une même clé")
    for case in pins["invalid"]:
        der = b64_decode(case["certificate"])
        try:
            spki = certificate_spki(der)
        except (ValueError, IndexError):
            check(case["error"] == "certificate", f"empreintes : {case['name']} : illisible")
            check(openssl_spki(der) is None, f"empreintes : {case['name']} : lu par OpenSSL")
            continue
        check(case["error"] == "key_type", f"empreintes : {case['name']} : certificat lisible")
        check(openssl_spki(der) == spki, f"empreintes : {case['name']} : OpenSSL lit une autre clé")
        check(not (len(spki) == 91 and spki.startswith(P256_SPKI_PREFIX)), f"empreintes : {case['name']} : clé P-256")


# --- Codes d'appairage (§7.2) --------------------------------------------------------------------

def normalize_code(text):
    chars = []
    for char in text:
        if char in " -":
            continue
        if not char.isascii():
            return None
        char = {"O": "0", "I": "1", "L": "1"}.get(char.upper(), char.upper())
        if char not in CROCKFORD:
            return None
        chars.append(char)
    return "".join(chars) if len(chars) == 8 else None


def check_codes(codes):
    for case in codes["encode"]:
        bits = f"{int(case['bytes'], 16):040b}"
        code = "".join(CROCKFORD[int(bits[i:i + 5], 2)] for i in range(0, 40, 5))
        check(code == case["code"], f"codes : {case['bytes']} donne {code}")
    for case in codes["normalize"]:
        code = normalize_code(case["input"])
        check(code == case["code"], f"codes : {case['input']!r} donne {code}")
        check(code is not None and f"{code[:4]}-{code[4:]}" == case["display"], f"codes : affichage de {code}")
    for case in codes["invalid"]:
        check(normalize_code(case["input"]) is None, f"codes : {case['name']} accepté")


# --- Liens d'appairage (§7.1) --------------------------------------------------------------------

class LinkError(Exception):
    pass


def form_decode(value):
    """Décodage application/x-www-form-urlencoded strict : % suivi de deux chiffres hexadécimaux,
    « + » pour une espace, et un résultat en UTF-8 valide."""
    raw, out, i = value.encode(), bytearray(), 0
    while i < len(raw):
        if raw[i] == ord("%"):
            pair = raw[i + 1:i + 3]
            if len(pair) != 2 or any(c not in b"0123456789abcdefABCDEF" for c in pair):
                raise LinkError("format")
            out.append(int(pair, 16))
            i += 3
        else:
            out.append(0x20 if raw[i] == ord("+") else raw[i])
            i += 1
    try:
        return out.decode("utf-8")
    except UnicodeDecodeError:
        raise LinkError("format") from None


def valid_host(host):
    if not host or len(host) > 253 or host != host.strip():
        return False
    if IPV4.fullmatch(host):
        return True
    if ":" in host:
        return bool(IPV6.fullmatch(host)) and 2 <= host.count(":") <= 7
    if all(c in "0123456789." for c in host):
        return False
    labels = (host[:-1] if host.endswith(".") else host).split(".")
    return all(LABEL.fullmatch(label) for label in labels)


def clean_name(name, limit):
    name = "".join(c for c in name if not (ord(c) <= 0x1F or 0x7F <= ord(c) <= 0x9F)).strip(" ")
    kept, units = [], 0
    for char in name:
        units += 2 if ord(char) > 0xFFFF else 1
        if units > limit:
            break
        kept.append(char)
    return "".join(kept).rstrip(" ")


def parse_link(text):
    link = text.strip(" \t\r\n")
    head = "".join(chr(ord(c) + 32) if "A" <= c <= "Z" else c for c in link[:len(LINK_PREFIX)])
    if len(link.encode()) > 2048 or head != LINK_PREFIX:
        raise LinkError("format")
    params = {}
    for part in link[len(LINK_PREFIX):].split("&"):
        if not part:
            continue
        name, equals, value = part.partition("=")
        if not equals or not name:
            raise LinkError("format")
        value = form_decode(value)
        if name in params:
            raise LinkError("duplicate")
        params[name] = value
    if params.get("v") != "2":
        raise LinkError("v")
    host = params.get("h", "")
    if not valid_host(host):
        raise LinkError("h")
    try:
        agent = b64_decode(params.get("a", ""), 32)
    except ValueError:
        raise LinkError("a") from None
    code = normalize_code(params.get("c", ""))
    if code is None:
        raise LinkError("c")
    port = 9770
    if "p" in params:
        if not re.fullmatch(r"[0-9]{1,5}", params["p"]) or not 1 <= int(params["p"]) <= 65535:
            raise LinkError("p")
        port = int(params["p"])
    mac = ""
    if "m" in params:
        mac = params["m"].upper().replace("-", ":")
        if not MAC.fullmatch(params["m"]) or mac in ("00:00:00:00:00:00", "FF:FF:FF:FF:FF:FF"):
            raise LinkError("m")
    role = params.get("r", "")
    if "r" in params and role not in ROLES:
        raise LinkError("r")
    expires = 0
    if "e" in params:
        if not re.fullmatch(r"[0-9]{1,12}", params["e"]) or not 1 <= int(params["e"]) <= 253402300799:
            raise LinkError("e")
        expires = int(params["e"])
    return {
        "name": clean_name(params.get("n", ""), 64) or clean_name(host, 64), "host": host, "port": port,
        "mac": mac, "agent": b64_encode(agent), "code": code, "role": role, "expires": expires,
    }


def write_link(fields):
    def escape(value):
        return urllib.parse.quote_plus(value, safe=":")
    params = ["v=2"]
    if fields["name"]:
        params.append("n=" + escape(fields["name"]))
    params.append("h=" + escape(fields["host"]))
    if fields["port"]:
        params.append(f"p={fields['port']}")
    if fields["mac"]:
        params.append("m=" + escape(fields["mac"]))
    params += ["a=" + fields["agent"], "c=" + escape(fields["code"])]
    if fields["role"]:
        params.append("r=" + fields["role"])
    if fields["expires"]:
        params.append(f"e={fields['expires']}")
    return LINK_PREFIX + "&".join(params)


def check_links(links):
    for case in links["valid"]:
        try:
            fields = parse_link(case["link"])
        except LinkError as error:
            check(False, f"liens : {case['name']} : refusé ({error})")
            continue
        check(fields == case["expect"], f"liens : {case['name']} : {fields}")
        check((write_link(fields) == case["link"]) == case["canonical"], f"liens : {case['name']} : écriture")
        check(parse_link(write_link(fields)) == fields, f"liens : {case['name']} : relecture")
    for case in links["invalid"]:
        try:
            parse_link(case["link"])
            check(False, f"liens : {case['name']} : accepté")
        except LinkError as error:
            check(str(error) == case["field"], f"liens : {case['name']} : {error}, attendu {case['field']}")
    for case in links["expiry"]:
        expires = parse_link(case["link"])["expires"]
        check((expires != 0 and case["now"] >= expires) == case["expired"], f"liens : expiration à {case['now']}")


# --- Preuves de migration (§9) -------------------------------------------------------------------

def check_migration(vectors):
    for case in vectors:
        key = b64_decode(case["key"])
        fields = [b64_decode(case[name], 32) for name in ("agent", "device", "nc", "na")]
        for label, expected in (("agent", case["agentProof"]), ("device", case["deviceProof"])):
            message = f"wolagent/2 migrate {label}".encode() + b"\x00" + b"".join(fields)
            proof = b64_encode(hmac.new(key, message, hashlib.sha256).digest())
            check(proof == expected, f"migration : {case['name']} : preuve {label} {proof}")


# --- Rôles (§5) et trames (§4.1) -----------------------------------------------------------------

def authorize(registered, role, cmd):
    if cmd in PAIRING:
        return "already_paired" if registered else "ok"
    if not registered:
        return "unknown_device"
    if cmd not in MIN_ROLE:
        return "unsupported"
    return "ok" if ROLES.index(role) >= ROLES.index(MIN_ROLE[cmd]) else "insufficient_role"


def check_roles(roles):
    check(roles["order"] == ROLES, f"rôles : ordre {roles['order']}")
    commands = {row["cmd"] for row in roles["authorize"]}
    check(commands >= set(MIN_ROLE) | set(PAIRING), "rôles : commandes manquantes")
    for row in roles["authorize"]:
        cmd = row["cmd"]
        kind = "pairing" if cmd in PAIRING else MIN_ROLE.get(cmd, "unknown")
        check(row["minRole"] == kind, f"rôles : {cmd!r} : {row['minRole']}")
        check(row["unregistered"] == authorize(False, None, cmd), f"rôles : {cmd!r} : identité inconnue")
        for role in ROLES:
            check(row[role] == authorize(True, role, cmd), f"rôles : {cmd!r} : {role}")


def read_stream(data, limit):
    payloads, pos = [], 0
    while pos < len(data):
        if len(data) - pos < 4:
            return payloads, "truncated"
        length = int.from_bytes(data[pos:pos + 4], "big")
        pos += 4
        if length == 0:
            return payloads, "empty"
        if length > limit:
            return payloads, "too_large"
        if len(data) - pos < length:
            return payloads, "truncated"
        payloads.append(data[pos:pos + length].decode())
        pos += length
    return payloads, "eof"


def check_frames(frames):
    check(frames["maxRequest"] == 16 * 1024 and frames["maxResponse"] == 4 * 1024 * 1024, "trames : limites")
    for case in frames["streams"]:
        payloads, end = read_stream(bytes.fromhex(case["stream"]), case["limit"])
        check((payloads, end) == (case["payloads"], case["end"]), f"trames : {case['name']} : {payloads} {end}")


def main():
    with open(sys.argv[1], encoding="utf-8") as f:
        vectors = json.load(f)
    check(vectors["protocol"] == "wolagent/2", "protocole")
    sections = [("pins", check_pins), ("codes", check_codes), ("links", check_links),
                ("migration", check_migration), ("roles", check_roles), ("frames", check_frames)]
    for name, check_section in sections:
        before = len(failures)
        try:
            check_section(vectors[name])
        except Exception as error:  # vecteur mal formé : le signaler avec sa section
            failures.append(f"{name} : {type(error).__name__} : {error}")
        print(f"{name} : {'OK' if len(failures) == before else 'ÉCHEC'}")
    for failure in failures:
        print(f"::error title=Vecteurs wolagent/2::{failure}")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
