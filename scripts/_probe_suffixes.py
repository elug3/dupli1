#!/usr/bin/env python3
"""Probe Kering DAM gallery suffixes for a product."""
import urllib.request

UA = "Mozilla/5.0"
base = "https://bottega-veneta.dam.kering.com/m/5fac54574ab958ee/Medium-578004VMAY19007_{}.jpg?v=5"
for letter in "ABCDEFGHI":
    url = base.format(letter)
    try:
        req = urllib.request.Request(url, headers={"User-Agent": UA})
        with urllib.request.urlopen(req, timeout=15) as r:
            print(letter, r.status, r.headers.get("Content-Length"))
    except Exception as e:
        print(letter, "FAIL", e)
