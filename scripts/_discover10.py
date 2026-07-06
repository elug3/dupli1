#!/usr/bin/env python3
from curl_cffi import requests

candidates = [
    "https://www.loewe.com/usa/en/women/bags/gate/mini-gate-dual-bag-in-soft-calfskin-and-jacquard/A650N46X13-8020.html",
    "https://www.loewe.com/usa/en/women/bags/gate/gate-bucket-bag-in-soft-grained-calfskin-and-raffia/A650Z56X02-2530.html",
    "https://www.loewe.com/eur/en/women/bags/gate/mini-gate-dual-bag-in-soft-calfskin-and-jacquard/A650N46X13-8020.html",
    "https://www.loewe.com/eur/en/women/bags/gate/mini-gate-dual-bag-in-soft-calfskin-and-jacquard/A650N46X13-2530.html",
    "https://www.loewe.com/int/en/women/bags/gate/gate-bucket-bag-in-soft-grained-calfskin-and-raffia/A650Z56X02-2530.html",
]
for u in candidates:
    r = requests.get(u, impersonate="chrome120", timeout=45)
    print(f"{r.status_code} ir={r.text.count('images_rd')} {u}")
