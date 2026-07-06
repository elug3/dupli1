#!/usr/bin/env python3
"""Debug extract product gallery URLs from a PDP."""
import re
from curl_cffi import requests

url = "https://www.bottegaveneta.com/en-us/cassette-white-578004VMAY19007.html"
html = requests.get(url, impersonate="chrome120", timeout=45).text

# Product gallery: Medium-{SKU}_{letter}.jpg
pat = re.compile(
    r"https://bottega-veneta\.dam\.kering\.com/m/[a-f0-9]+/Medium-[A-Z0-9]+_[A-Z]\.jpg(?:\?v=\d+)?",
    re.I,
)
found = sorted(set(pat.findall(html)))
print(f"found {len(found)}")
for u in found:
    print(u)
