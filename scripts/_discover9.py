#!/usr/bin/env python3
from curl_cffi import requests
import re

b = requests.get(
    "https://www.loewe.com/eur/sitemap_0-product.xml",
    impersonate="chrome120",
    timeout=120,
).text
urls = re.findall(r"<loc>([^<]+)</loc>", b)
for kw in ["balloon", "gate", "A650", "A411"]:
    hits = [u for u in urls if kw.lower() in u.lower()]
    print(kw, len(hits))
    for u in hits[:8]:
        print(" ", u)
