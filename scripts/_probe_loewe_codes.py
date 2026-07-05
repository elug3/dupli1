#!/usr/bin/env python3
import urllib.request

BASE = (
    "https://www.loewe.com/dw/image/v2/BBPC_PRD/on/demandware.static/"
    "-/Sites-Loewe_master/default/dwe6879fa4/images_rd/{code}/{code}-{color}/{code}_{color}_{angle}.jpg"
    "?sw=2000&q=90"
)
CODES = ["B411FLKX01", "B411FLKX02", "B411FLKX03", "A411FLKX01"]
COLORS = ["2530", "1100", "8020", "1000", "5120"]
ANGLES = ["1F", "1O", "1P", "1Q", "1R", "1S", "1T", "1U", "1V", "1W", "2O", "1A", "1B", "1C"]


def exists(url: str) -> bool:
    try:
        req = urllib.request.Request(url, method="HEAD", headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status == 200
    except Exception:
        return False


for code in CODES + ["A650Z56X02", "A650N46X13"]:
    for color in COLORS:
        imgs = []
        for angle in ANGLES:
            url = BASE.format(code=code, color=color, angle=angle)
            if exists(url):
                imgs.append(angle)
        if imgs:
            print(code, color, len(imgs), imgs)
