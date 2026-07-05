#!/usr/bin/env python3
import urllib.request

TMPL = "https://www.celine.com/on/demandware.static/-/Sites-masterCatalog/default/images/large/{sku}_{n}_LIB_W_V2.jpg"
TMPL2 = "https://www.celine.com/on/demandware.static/-/Sites-masterCatalog/default/images/large/{sku}_{n}_SUM25_W.jpg"
SKUS = [
    "187363BF4.38NO", "187363BF4.10BL", "188373BF4.38NO", "194373BFN.04LU",
    "189613BF4.04LU", "191242BZ4.04LU", "188004BZS.27ED", "189003AFA.04LU",
    "188003BAG.38NO", "193953BF4.04LU", "187373BF4.04LU",
]


def count(sku, tmpl):
    return [n for n in range(1, 10) if ok(tmpl.format(sku=sku, n=n))]


def ok(url):
    try:
        req = urllib.request.Request(url, method="HEAD", headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=8) as resp:
            return resp.status == 200
    except Exception:
        return False


for sku in SKUS:
    h1 = count(sku, TMPL)
    h2 = count(sku, TMPL2)
    if h1 or h2:
        print(sku, "LIB", h1, "SUM", h2)
