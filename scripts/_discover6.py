#!/usr/bin/env python3
from curl_cffi import requests
import re
import json

r = requests.get(
    "https://www.loewe.com/int/en/women/bags/gate",
    impersonate="chrome120",
    timeout=45,
)
# Look for embedded JSON product data
for pat in [
    r"__NEXT_DATA__.*?({.*?})</script>",
    r"window\.__STATE__\s*=\s*({.*?});",
    r"\"products\"\s*:\s*(\[.*?\])",
    r"images_rd/([A-Z0-9]+/[A-Z0-9.-]+/[A-Z0-9_]+\.jpg)",
    r"A650[A-Z0-9]{5,10}",
]:
    hits = re.findall(pat, r.text, re.S)
    print(pat[:40], "->", len(hits))
    if hits and len(hits) < 20:
        for h in hits[:5]:
            s = str(h)[:150]
            print(" ", s)

# sitemap
for sm in [
    "https://www.loewe.com/sitemap.xml",
    "https://www.loewe.com/sitemap_index.xml",
    "https://www.loewe.com/robots.txt",
]:
    try:
        rs = requests.get(sm, impersonate="chrome120", timeout=30)
        print(f"\n{sm}: {rs.status_code} len={len(rs.text)}")
        if "gate" in rs.text.lower():
            for line in rs.text.splitlines():
                if "gate" in line.lower() and "bags" in line.lower():
                    print(" ", line[:120])
                    break
    except Exception as exc:
        print(sm, exc)
