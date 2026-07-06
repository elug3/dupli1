#!/usr/bin/env python3
from curl_cffi import requests
import re

queries = [
    "loulou+small+bag+quilted",
    "gate+mini+dual",
    "small+balloon+bag",
]
for q in queries:
    if "gate" in q or "balloon" in q:
        url = f"https://www.loewe.com/int/en/search?lang=en_US&q={q}"
    else:
        url = f"https://www.ysl.com/en-us/search?q={q}"
    r = requests.get(url, impersonate="chrome120", timeout=45)
    if "loewe" in url:
        hits = sorted(set(re.findall(r"loewe\.com[^\"'<>]+\.html", r.text)))
    else:
        hits = sorted(set(re.findall(r"/en-us/pr/[a-z0-9-]+-\d+[A-Z0-9]+\.html", r.text, re.I)))
    print(f"\n{q}: {len(hits)}")
    for h in hits[:6]:
        if not h.startswith("http"):
            h = "https://www.ysl.com" + h
        print(" ", h[:120])

# try direct loewe gate URLs from common patterns
candidates = [
    "https://www.loewe.com/int/en/women/bags/gate/mini-gate-dual-bag-in-soft-grained-calfskin/A650N46X01-1100.html",
    "https://www.loewe.com/int/en/women/bags/gate/mini-gate-dual-bag-in-soft-calfskin/A650N46X13-8020.html",
    "https://www.loewe.com/int/en/women/bags/gate/mini-gate-bag-in-soft-grained-calfskin/A650N46X01-1100.html",
    "https://www.loewe.com/int/en/women/bags/balloon/small-balloon-bag-in-classic-calfskin/A411FC2X01-1100.html",
    "https://www.loewe.com/int/en/women/bags/balloon/small-balloon-bag-in-classic-calfskin/A411FC2X01-2530.html",
    "https://www.loewe.com/int/en/women/bags/balloon/small-balloon-bag-in-classic-calfskin/A411FC2X01-1100",
]
print("\n--- direct loewe probes ---")
for u in candidates:
    r = requests.get(u, impersonate="chrome120", timeout=45)
    ir = r.text.count("images_rd")
    print(f"{r.status_code} images_rd={ir} {u.split('/')[-1]}")
