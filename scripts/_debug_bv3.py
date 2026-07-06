#!/usr/bin/env python3
import re
from curl_cffi import requests

url = "https://www.bottegaveneta.com/en-us/cassette-white-578004VMAY19007.html"
html = requests.get(url, impersonate="chrome120", timeout=45).text

pat = re.compile(
    r"https://bottega-veneta\.dam\.kering\.com/(?:m/[a-f0-9]+|asset/[a-f0-9-]+)/(?:Medium|Original-Ecom)/[A-Z0-9]+_[A-Z]\.jpg(?:\?v=\d+)?",
    re.I,
)
all_urls = sorted(set(pat.findall(html)))
# Exclude campaign shots
product = [u for u in all_urls if "Adv_" not in u and "Campaign" not in u and "Attitude" not in u]
print(f"product gallery: {len(product)}")
for u in product:
    print(u)
