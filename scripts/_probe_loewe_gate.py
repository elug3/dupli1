#!/usr/bin/env python3
import urllib.error
import urllib.request

BASE = (
    "https://www.loewe.com/dw/image/v2/BBPC_PRD/on/demandware.static/"
    "-/Sites-Loewe_master/default/{hash}/images_rd/{code}/{code}-{color}/{code}_{color}_{angle}.jpg"
    "?sw=2000&q=90"
)
# reuse known hashes from puzzle/flamenco/amazona
HASHES = [
    "dwe6879fa4", "dw2c6dee81", "dw35934894", "dwbf587b16", "dwa8eda735",
    "dwbd27cb59", "dw9f9d612e", "dw25d15202", "dw65902c83", "dw1657387f",
]
CODE = "A650N46X13"
COLORS = ["2530", "1100", "8020", "1000", "2150"]
ANGLES = ["1F", "1O", "1P", "1Q", "1R", "1S", "1T", "1U", "1V", "1W", "2O", "1A"]


def exists(url: str) -> bool:
    try:
        req = urllib.request.Request(url, method="HEAD", headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status == 200
    except Exception:
        return False


found = []
for h in HASHES:
    for color in COLORS:
        for angle in ANGLES:
            url = BASE.format(hash=h, code=CODE, color=color, angle=angle)
            if exists(url):
                print("FOUND", url)
                found.append(url)
                if len(found) >= 9:
                    break
        if len(found) >= 9:
            break
    if len(found) >= 9:
        break
print("total", len(found))
