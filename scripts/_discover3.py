#!/usr/bin/env python3
from curl_cffi import requests
import re

SEARCHES = [
    ("lou camera", "https://www.ysl.com/en-us/search?q=lou+camera+bag"),
    ("kate medium", "https://www.ysl.com/en-us/search?q=kate+medium"),
    ("loulou small", "https://www.ysl.com/en-us/search?q=loulou+small"),
    ("sac de jour nano", "https://www.ysl.com/en-us/search?q=sac+de+jour+nano"),
    ("niki medium", "https://www.ysl.com/en-us/search?q=niki+medium"),
    ("solferino small", "https://www.ysl.com/en-us/search?q=solferino+small"),
    ("cassandre envelope", "https://www.ysl.com/en-us/search?q=cassandre+envelope"),
]

for name, url in SEARCHES:
    r = requests.get(url, impersonate="chrome120", timeout=45)
    pr = sorted(set(re.findall(r"/en-us/pr/[a-z0-9-]+-\d+[A-Z0-9]+\.html", r.text, re.I)))
    full = [f"https://www.ysl.com{p}" for p in pr[:3]]
    print(f"\n{name}: {r.status_code} hits={len(pr)}")
    for u in full:
        print(" ", u)

# Loewe gate/balloon via search
for name, q in [("gate mini", "gate+mini"), ("balloon small", "balloon+small")]:
    url = f"https://www.loewe.com/int/en/search?q={q}"
    r = requests.get(url, impersonate="chrome120", timeout=45)
    paths = sorted(set(re.findall(r"/int/en/women/bags/[^\"'<>]+\.html", r.text)))
    print(f"\nloewe {name}: {r.status_code} paths={len(paths)}")
    for p in paths[:5]:
        print(" ", p)
