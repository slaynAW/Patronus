#!/usr/bin/env python3
"""Recalcule indépendamment les vecteurs du stockage chiffré du hub (protocol/hub-store-vectors.json).

Seconde implémentation, écrite d'après docs/HUB.md (§9) et la description du fichier de vecteurs, et non
d'après le code Go (agent/internal/hub/store) : bibliothèque standard de Python seulement. AES-256
(FIPS 197) et GCM (NIST SP 800-38D) sont écrits ici, HKDF (RFC 5869) repose sur hmac. Une erreur dans
l'une des deux implémentations fait échouer la CI.

Usage : check-hub-store-vectors.py protocol/hub-store-vectors.json
"""
import hashlib
import hmac
import json
import struct
import sys

FILE_MAGIC = b"PHF1"
JOURNAL_MAGIC = b"PHJ1"
FILE_KEY_INFO = b"patronus-hub/1 file key"
NONCE_SIZE = 12
TAG_SIZE = 16
MAX_RECORD_BYTES = 1 << 20


# --- AES-256 (FIPS 197), chiffrement seul : c'est tout ce qu'utilise GCM. ---

def _xtime(a):
    a <<= 1
    return (a ^ 0x11B) if a & 0x100 else a


def _gmul(a, b):
    r = 0
    while b:
        if b & 1:
            r ^= a
        a = _xtime(a)
        b >>= 1
    return r


def _sbox():
    box = []
    for x in range(256):
        inv = 0 if x == 0 else next(y for y in range(1, 256) if _gmul(x, y) == 1)
        s = inv
        for shift in range(1, 5):
            s ^= ((inv << shift) | (inv >> (8 - shift))) & 0xFF
        box.append(s ^ 0x63)
    return box


SBOX = _sbox()
assert SBOX[0x00] == 0x63 and SBOX[0x53] == 0xED  # valeurs de la FIPS 197


def _expand_key(key):
    assert len(key) == 32
    nk, nr = 8, 14
    words = [list(key[4 * i:4 * i + 4]) for i in range(nk)]
    rcon = 1
    for i in range(nk, 4 * (nr + 1)):
        t = list(words[i - 1])
        if i % nk == 0:
            t = [SBOX[b] for b in t[1:] + t[:1]]
            t[0] ^= rcon
            rcon = _xtime(rcon)
        elif i % nk == 4:
            t = [SBOX[b] for b in t]
        words.append([a ^ b for a, b in zip(words[i - nk], t)])
    return [sum(words[4 * r:4 * r + 4], []) for r in range(nr + 1)]


def _encrypt_block(round_keys, block):
    s = [b ^ k for b, k in zip(block, round_keys[0])]
    for r in range(1, 15):
        s = [SBOX[b] for b in s]
        # Décalage des lignes : l'état est rangé colonne par colonne (octet 4c + l).
        s = [s[(4 * ((c + l) % 4)) + l] for c in range(4) for l in range(4)]
        if r != 14:
            mixed = []
            for c in range(4):
                a = s[4 * c:4 * c + 4]
                mixed += [
                    _gmul(a[0], 2) ^ _gmul(a[1], 3) ^ a[2] ^ a[3],
                    a[0] ^ _gmul(a[1], 2) ^ _gmul(a[2], 3) ^ a[3],
                    a[0] ^ a[1] ^ _gmul(a[2], 2) ^ _gmul(a[3], 3),
                    _gmul(a[0], 3) ^ a[1] ^ a[2] ^ _gmul(a[3], 2),
                ]
            s = mixed
        s = [b ^ k for b, k in zip(s, round_keys[r])]
    return bytes(s)


# Vecteur de la FIPS 197, annexe C.3 (AES-256).
assert _encrypt_block(_expand_key(bytes(range(32))), bytes.fromhex("00112233445566778899aabbccddeeff")).hex() == \
    "8ea2b7ca516745bfeafc49904b496089"


# --- GCM (NIST SP 800-38D), nonce de 96 bits. ---

def _gf_mult(x, y):
    r_poly = 0xE1 << 120
    z, v = 0, y
    for i in range(127, -1, -1):
        if (x >> i) & 1:
            z ^= v
        v = (v >> 1) ^ r_poly if v & 1 else v >> 1
    return z


def _ghash(h, data):
    y = 0
    for i in range(0, len(data), 16):
        y = _gf_mult(y ^ int.from_bytes(data[i:i + 16], "big"), h)
    return y


def _pad16(b):
    return b + bytes(-len(b) % 16)


def _gctr(round_keys, counter, data):
    out = bytearray()
    for i in range(0, len(data), 16):
        stream = _encrypt_block(round_keys, counter)
        out += bytes(a ^ b for a, b in zip(data[i:i + 16], stream))
        counter = counter[:12] + struct.pack(">I", (struct.unpack(">I", counter[12:])[0] + 1) & 0xFFFFFFFF)
    return bytes(out)


def _gcm_tag(round_keys, j0, aad, ciphertext):
    h = int.from_bytes(_encrypt_block(round_keys, bytes(16)), "big")
    s = _ghash(h, _pad16(aad) + _pad16(ciphertext) + struct.pack(">QQ", 8 * len(aad), 8 * len(ciphertext)))
    return (s ^ int.from_bytes(_encrypt_block(round_keys, j0), "big")).to_bytes(16, "big")


def gcm_seal(key, nonce, plaintext, aad):
    assert len(nonce) == NONCE_SIZE
    rk = _expand_key(key)
    j0 = nonce + b"\x00\x00\x00\x01"
    ct = _gctr(rk, nonce + b"\x00\x00\x00\x02", plaintext)
    return ct + _gcm_tag(rk, j0, aad, ct)


def gcm_open(key, nonce, sealed, aad):
    if len(sealed) < TAG_SIZE:
        return None
    rk = _expand_key(key)
    ct, tag = sealed[:-TAG_SIZE], sealed[-TAG_SIZE:]
    if not hmac.compare_digest(_gcm_tag(rk, nonce + b"\x00\x00\x00\x01", aad, ct), tag):
        return None
    return _gctr(rk, nonce + b"\x00\x00\x00\x02", ct)


# Vecteur 16 de l'article de McGrew et Viega sur GCM (AES-256, nonce de 96 bits, données authentifiées).
_K = bytes.fromhex("feffe9928665731c6d6a8f9467308308feffe9928665731c6d6a8f9467308308")
_P = bytes.fromhex("d9313225f88406e5a55909c5aff5269a86a7a9531534f7da2e4c303d8a318a721c3c0c95956809532fcf0e2449a6b525"
                   "b16aedf5aa0de657ba637b39")
_A = bytes.fromhex("feedfacedeadbeeffeedfacedeadbeefabaddad2")
_IV = bytes.fromhex("cafebabefacedbaddecaf888")
assert gcm_seal(_K, _IV, _P, _A).hex() == (
    "522dc1f099567d07f47f37a32a84427d643a8cdcbfe5c0c97598a2bd2555d1aa8cb08e48590dbb3da7b08b1056828838"
    "c5f61e6393ba7a0abcc9f662" "76fc6ece0f4e1768cddf8853bb2d551b")


# --- Format du hub ---

def hkdf_sha256(ikm, info, length=32):
    prk = hmac.new(bytes(32), ikm, hashlib.sha256).digest()  # sans sel : HashLen octets nuls (RFC 5869)
    okm, t, i = b"", b"", 1
    while len(okm) < length:
        t = hmac.new(prk, t + info + bytes([i]), hashlib.sha256).digest()
        okm += t
        i += 1
    return okm[:length]


def file_key(data_key, path):
    return hkdf_sha256(data_key, FILE_KEY_INFO + b"\x00" + path.encode())


def seal_file(data_key, path, nonce, plaintext):
    return FILE_MAGIC + nonce + gcm_seal(file_key(data_key, path), nonce, plaintext, FILE_MAGIC + path.encode())


def open_file(data_key, path, data):
    if len(data) < len(FILE_MAGIC) + NONCE_SIZE + TAG_SIZE or data[:4] != FILE_MAGIC:
        return None
    nonce = data[4:4 + NONCE_SIZE]
    return gcm_open(file_key(data_key, path), nonce, data[4 + NONCE_SIZE:], FILE_MAGIC + path.encode())


def record_aad(path, index):
    return JOURNAL_MAGIC + struct.pack(">Q", index) + path.encode()


def seal_record(data_key, path, index, nonce, plaintext):
    body = nonce + gcm_seal(file_key(data_key, path), nonce, plaintext, record_aad(path, index))
    return struct.pack(">I", len(body)) + body


def open_journal(data_key, path, data):
    """Renvoie (enregistrements, fin de la partie complète), ou None si le journal est illisible."""
    if len(data) < 4:
        return ([], 0) if JOURNAL_MAGIC.startswith(data) else None
    if data[:4] != JOURNAL_MAGIC:
        return None
    key, pos, records = file_key(data_key, path), 4, []
    while True:
        if len(data) - pos < 4:
            return records, pos
        (n,) = struct.unpack(">I", data[pos:pos + 4])
        if n < NONCE_SIZE + TAG_SIZE or n > NONCE_SIZE + MAX_RECORD_BYTES + TAG_SIZE:
            return None
        if len(data) - pos - 4 < n:
            return records, pos
        body = data[pos + 4:pos + 4 + n]
        plain = gcm_open(key, body[:NONCE_SIZE], body[NONCE_SIZE:], record_aad(path, len(records)))
        if plain is None:
            return None
        records.append(plain)
        pos += 4 + n


def main(path):
    with open(path, encoding="utf-8") as f:
        v = json.load(f)
    data_key = bytes.fromhex(v["key"])
    errors = []

    def check(cond, what):
        if not cond:
            errors.append(what)

    for fk in v["fileKeys"]:
        check(file_key(data_key, fk["path"]).hex() == fk["key"], "clé du fichier " + fk["path"])
    for f in v["files"]:
        plain = f["plaintext"].encode()
        sealed = seal_file(data_key, f["path"], bytes.fromhex(f["nonce"]), plain)
        check(sealed.hex() == f["sealed"], "fichier : " + f["name"])
        check(open_file(data_key, f["path"], sealed) == plain, "relecture du fichier : " + f["name"])
    for j in v["journals"]:
        sealed = JOURNAL_MAGIC
        for i, r in enumerate(j["records"]):
            sealed += seal_record(data_key, j["path"], i, bytes.fromhex(r["nonce"]), r["plaintext"].encode())
        check(sealed.hex() == j["sealed"], "journal : " + j["name"])
        tail = bytes.fromhex(j.get("tail", ""))
        opened = open_journal(data_key, j["path"], sealed + tail)
        check(opened is not None and opened[1] == len(sealed)
              and [r.decode() for r in opened[0]] == [r["plaintext"] for r in j["records"]],
              "relecture du journal : " + j["name"])
    for x in v["invalid"]:
        data = bytes.fromhex(x["data"])
        if x["kind"] == "file":
            check(open_file(data_key, x["path"], data) is None, "accepté : " + x["name"])
        else:
            check(open_journal(data_key, x["path"], data) is None, "accepté : " + x["name"])

    total = len(v["fileKeys"]) + len(v["files"]) + len(v["journals"]) + len(v["invalid"])
    if errors:
        print("\n".join("ÉCHEC : " + e for e in errors))
        sys.exit(1)
    print(f"{total} vecteurs du stockage du hub vérifiés")


if __name__ == "__main__":
    main(sys.argv[1])
