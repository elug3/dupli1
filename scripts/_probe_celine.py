#!/usr/bin/env python3
from curl_cffi import requests
import re

urls = [
    "https://www.celine.com/en/women/handbags/triomphe/triomphe-shoulder-bag-in-shiny-calfskin-188373BF4.38NO.html",
    "https://www.celine.com/en-int/women/handbags/triomphe/triomphe-shoulder-bag-in-shiny-calfskin-194373BFN.04LU.html",
    "https://www.celine.com/en-int/women/handbags/triomphe/triomphe-teen-bag-in-shiny-calfskin-194372BF4.38NO.html",
]
for u in urls:
    r = requests.get(u, impersonate="chrome120", timeout=45)
    print(u.split("/")[-1][:40], r.status_code, len(r.text))
    for pat in [r"celine\.dam\.kering", r"scene7", r"\.jpg", r"productImage"]:
        print(" ", pat, len(re.findall(pat, r.text, re.I)))
