#!/usr/bin/env python3
from curl_cffi import requests
import re

r = requests.get(
    "https://www.loewe.com/int/en/women/bags/gate",
    impersonate="chrome120",
    timeout=45,
)
print("Sites", sorted(set(re.findall(r"Sites-[A-Za-z0-9_-]+", r.text)))[:10])
print("cgid", sorted(set(re.findall(r"cgid[\"']?\s*[:=]\s*[\"']([^\"']+)[\"']", r.text)))[:15])
print("A650", sorted(set(re.findall(r"A650[A-Z0-9]+", r.text)))[:10])

for sm in [
    "https://www.loewe.com/eur/sitemap_0-product.xml",
    "https://www.loewe.com/eur/sitemap_1-product.xml",
]:
    b = requests.get(sm, impersonate="chrome120", timeout=120).text
    for code in ["A650", "N46X13", "Z56X02", "Balloon", "balloon-bag", "gate-dual", "gate-bucket"]:
        c = b.count(code)
        if c:
            print(sm.split("/")[-1], code, c)
            idx = b.find(code)
            print(" ", b[idx - 60 : idx + 120].replace("\n", " ")[:180])
