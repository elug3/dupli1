#!/usr/bin/env python3
from curl_cffi import requests
import re

base = "https://www.loewe.com/on/demandware.store/Sites-LW_WW_storefront/en"
for path in [
    f"{base}/Search-Show?cgid=w_gate",
    f"{base}/Search-UpdateGrid?cgid=w_gate&start=0&sz=48",
    f"{base}/Search-Show?cgid=w_balloon",
    f"{base}/Search-UpdateGrid?cgid=w_balloon&start=0&sz=48",
]:
    r = requests.get(path, impersonate="chrome120", timeout=45)
    print(f"\n{path.split('?')[1]} -> {r.status_code} len={len(r.text)}")
    urls = sorted(set(re.findall(r"/int/en/women/bags/[^\"'<>]+\.html", r.text)))
    print(" urls", len(urls))
    for u in urls[:10]:
        print(" ", u)
    skus = sorted(set(re.findall(r"[AB][0-9A-Z]{8,12}-[0-9]{4}", r.text)))
    print(" skus", skus[:10])
