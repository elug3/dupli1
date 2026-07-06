#!/usr/bin/env python3
import re
from curl_cffi import requests

url = "https://www.bottegaveneta.com/en-us/cassette-white-578004VMAY19007.html"
html = requests.get(url, impersonate="chrome120", timeout=45).text
code = "578004VMAY19007"

# All gallery angles for this SKU
pat = re.compile(
    rf"https://bottega-veneta\.dam\.kering\.com/(?:m/[a-f0-9]+|asset/[a-f0-9-]+)/(?:Medium|Original-Ecom)/{re.escape(code)}_[A-Z]\.jpg(?:\?v=\d+)?",
    re.I,
)
found = sorted(set(pat.findall(html)))
print(f"code {code}: {len(found)}")
for u in found:
    print(u)

# Also try partial code match 578004
pat2 = re.compile(
    r"https://bottega-veneta\.dam\.kering\.com/(?:m/[a-f0-9]+|asset/[a-f0-9-]+)/(?:Medium|Original-Ecom)/578004[A-Z0-9]+_[A-Z]\.jpg(?:\?v=\d+)?",
    re.I,
)
found2 = sorted(set(pat2.findall(html)))
print(f"\n578004*: {len(found2)}")
for u in found2:
    print(u)
