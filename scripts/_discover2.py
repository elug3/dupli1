#!/usr/bin/env python3
from curl_cffi import requests
import re

r = requests.get(
    "https://www.loewe.com/int/en/women/bags/gate",
    impersonate="chrome120",
    timeout=45,
)
print("gate page", len(r.text))
print("A650 ids", sorted(set(re.findall(r"A650[A-Z0-9]+-[0-9]+", r.text)))[:15])
print("gate paths", sorted(set(re.findall(r"/women/bags/gate/[a-z0-9-]+/[A-Z0-9.-]+\.html", r.text)))[:10])

r2 = requests.get(
    "https://www.loewe.com/int/en/women/bags/balloon",
    impersonate="chrome120",
    timeout=45,
)
print("\nballoon page", len(r2.text))
print("balloon ids", sorted(set(re.findall(r"A[0-9A-Z]+[A-Z0-9]*-[0-9]+", r2.text)))[:15])
print("balloon paths", sorted(set(re.findall(r"/women/bags/balloon/[a-z0-9-]+/[A-Z0-9.-]+\.html", r2.text)))[:10])

r3 = requests.get(
    "https://www.ysl.com/en-us/shop-women/handbags/all-handbags",
    impersonate="chrome120",
    timeout=45,
)
skus = sorted(set(re.findall(r"633[0-9A-Z]{10,20}", r3.text)))
print("\nysl skus", len(skus), skus[:10])
urls = sorted(set(re.findall(r"https://www\.ysl\.com/en-us/pr/[^\s\"'<>]+", r3.text)))
print("ysl pr urls", len(urls))
for u in urls[:8]:
    print(" ", u)
