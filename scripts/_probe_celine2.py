#!/usr/bin/env python3
from curl_cffi import requests
import re

urls = [
    "https://www.celine.com/en-gb/celine-shop-women/handbags/triomphe-canvas/classique-triomphe-bag-in-triomphe-canvas-and-calfskin-191242BZ4.04LU.html",
    "https://www.celine.com/en-int/celine-women/handbags/triomphe/classique-triomphe-bag-in-shiny-calfskin-187363BF4.10BL.html",
    "https://www.celine.com/en-gb/celine-shop-women/handbags/classic/classic-box-medium-bag-in-box-calfskin-189613BF4.04LU.html",
]
for u in urls:
    r = requests.get(u, impersonate="chrome120", timeout=45)
    imgs = sorted(set(re.findall(
        r"https://www\.celine\.com/on/demandware\.static/[^\"'\s<>]+?\.jpg[^\"'\s<>]*",
        r.text,
    )))
    print(u.split("/")[-1][:50], r.status_code, len(r.text), "imgs", len(imgs))
    for i in imgs[:5]:
        print(" ", i[:120])
