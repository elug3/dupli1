#!/usr/bin/env python3
import urllib.request

SKUS = {
    "Triomphe": "187363BF4.38NO",
    "Classic Box": "189613BF4.04LU",
    "canvas": "191242BZ4.04LU",
}
TEMPLATES = [
    "https://www.celine.com/on/demandware.static/-/Sites-masterCatalog/default/dw9fb6b232/images/large/{sku}_{n}_LIB_W_V2.jpg",
    "https://www.celine.com/dw/image/v2/AAVP_PRD/on/demandware.static/-/Sites-masterCatalog/default/dw9fb6b232/images/large/{sku}_{n}_LIB_W_V2.jpg?sw=2000",
    "https://www.celine.com/on/demandware.static/-/Sites-masterCatalog/default/images/large/{sku}_{n}_LIB_W_V2.jpg",
]


def exists(url: str) -> bool:
    try:
        req = urllib.request.Request(url, method="HEAD", headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=12) as resp:
            return resp.status == 200
    except Exception:
        return False


for name, sku in SKUS.items():
    print(f"\n{name} {sku}")
    for tmpl in TEMPLATES:
        hits = [n for n in range(1, 10) if exists(tmpl.format(sku=sku, n=n))]
        if hits:
            print(" ", tmpl.split("/default/")[0][-20:], hits)
