#!/usr/bin/env python3
from curl_cffi import requests
import re

for sm in [
    "https://www.loewe.com/eur/sitemap_0-product.xml",
    "https://www.loewe.com/eur/sitemap_1-product.xml",
]:
    body = requests.get(sm, impersonate="chrome120", timeout=120).text
    print(sm, "len", len(body))
    gate = [u for u in re.findall(r"<loc>([^<]+)</loc>", body) if "/gate/" in u.lower()]
    balloon = [u for u in re.findall(r"<loc>([^<]+)</loc>", body) if "/balloon/" in u.lower()]
    print(" gate", len(gate), "balloon", len(balloon))
    for u in gate[:5]:
        print(" ", u)
    for u in balloon[:5]:
        print(" ", u)
