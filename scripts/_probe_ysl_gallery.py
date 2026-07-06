#!/usr/bin/env python3
import urllib.error
import urllib.request

BASE = "https://saint-laurent.dam.kering.com/m/{hash}/eCom-{sku}_{letter}.jpg?v=2"
hash_id = "35aadf21212ba61e"
sku = "633178AACYT3212"

def exists(url: str) -> bool:
    try:
        req = urllib.request.Request(url, method="HEAD", headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status == 200
    except Exception:
        return False

for i in range(15):
    letter = chr(ord("A") + i)
    url = BASE.format(hash=hash_id, sku=sku, letter=letter)
    print(letter, exists(url), url)
