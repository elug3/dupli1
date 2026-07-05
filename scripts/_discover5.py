#!/usr/bin/env python3
from curl_cffi import requests
import re

candidates = [
    "https://www.loewe.com/int/en/women/bags/gate/mini-gate-dual-bag-in-soft-calfskin-and-jacquard/A650N46X13-2530.html",
    "https://www.loewe.com/int/en/women/bags/gate/mini-gate-dual-bag-in-soft-grained-calfskin/A650N46X13-2530.html",
    "https://www.loewe.com/int/en/women/bags/gate/mini-gate-dual-bag-in-soft-calfskin/A650N46X13-2530.html",
    "https://www.loewe.com/apc/en/women/bags/gate/mini-gate-dual-bag-in-soft-calfskin/A650N46X13-2530.html",
    "https://www.loewe.com/int/en/women/bags/balloon/small-balloon-bag-in-classic-calfskin/A411FC2X01-2530.html",
    "https://www.loewe.com/int/en/women/bags/balloon/small-balloon-bag-in-classic-calfskin/B411FC2X01-2530.html",
    "https://www.loewe.com/int/en/women/bags/balloon/small-balloon-bag-in-classic-calfskin/A411FC2X01-1100.html",
]
for u in candidates:
    r = requests.get(u, impersonate="chrome120", timeout=45)
    ir = r.text.count("images_rd")
    code = re.search(r"A650[A-Z0-9]+|A411[A-Z0-9]+|B411[A-Z0-9]+", u)
    print(f"{r.status_code} ir={ir} {u}")

# YSL loulou small
r = requests.get(
    "https://www.ysl.com/en-us/search?q=loulou+small+in+quilted",
    impersonate="chrome120",
    timeout=45,
)
for p in sorted(set(re.findall(r"/en-us/pr/loulou-small[^\"']+\.html", r.text, re.I)))[:8]:
    print("loulou", "https://www.ysl.com" + p)

# sac de jour nano specifically
r2 = requests.get(
    "https://www.ysl.com/en-us/search?q=sac+de+jour+nano+grained",
    impersonate="chrome120",
    timeout=45,
)
for p in sorted(set(re.findall(r"/en-us/pr/sac-de-jour[^\"']+nano[^\"']+\.html", r2.text, re.I)))[:8]:
    print("sdj", "https://www.ysl.com" + p)

# cassandre envelope clutch
r3 = requests.get(
    "https://www.ysl.com/en-us/search?q=cassandre+envelope+clutch",
    impersonate="chrome120",
    timeout=45,
)
for p in sorted(set(re.findall(r"/en-us/pr/cassandre-envelope[^\"']+clutch[^\"']+\.html", r3.text, re.I)))[:8]:
    print("cass", "https://www.ysl.com" + p)
