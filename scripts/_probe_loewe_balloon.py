#!/usr/bin/env python3
from curl_cffi import requests
import re
import urllib.request

b = requests.get(
    "https://www.loewe.com/eur/sitemap_0-product.xml",
    impersonate="chrome120",
    timeout=120,
).text + requests.get(
    "https://www.loewe.com/eur/sitemap_1-product.xml",
    impersonate="chrome120",
    timeout=120,
).text

for pat in ["balloon-bag", "balloon bag", "/bags/balloon/", "Balloon"]:
    hits = re.findall(rf"<loc>([^<]*{pat.replace(' ', '[^<]*')}[^<]*)</loc>", b, re.I)
    print(pat, len(hits))
    for h in hits[:10]:
        print(" ", h)

# probe likely balloon codes from sitemap
codes = sorted(set(re.findall(r"/([ABCH][0-9]{3}[A-Z0-9]{5,10})-[0-9]{4}\.html", b)))
balloonish = [c for c in codes if "BAL" in c.upper() or c.startswith("B4") or "BLN" in c]
print("codes sample", len(codes), balloonish[:20])

BASE = (
    "https://www.loewe.com/dw/image/v2/BBPC_PRD/on/demandware.static/"
    "-/Sites-Loewe_master/default/dwe6879fa4/images_rd/{code}/{code}-{color}/{code}_{color}_{angle}.jpg"
    "?sw=2000&q=90"
)
HASHES = ["dwe6879fa4", "dw2c6dee81", "dw35934894"]
CANDIDATES = [
    "B411FC2X01", "A411FC2X01", "B411BLNX01", "A411BLNX01", "B411FC2X02",
    "C411FC2X01", "A411FC2X02", "B411FC1X01", "A650Z56X02",
]
COLORS = ["2530", "1100", "8020", "1000"]
ANGLES = ["1F", "1O", "1P", "1Q", "1R", "1S", "1T", "1U", "1W"]


def exists(url: str) -> bool:
    try:
        req = urllib.request.Request(url, method="HEAD", headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status == 200
    except Exception:
        return False


for code in CANDIDATES:
    for color in COLORS:
        url = BASE.format(code=code, color=color, angle="1F")
        if exists(url):
            print("HIT", code, color, url)
            imgs = []
            for angle in ANGLES:
                u = BASE.format(code=code, color=color, angle=angle)
                if exists(u):
                    imgs.append(u)
            print("  count", len(imgs))
