#!/usr/bin/env python3
from curl_cffi import requests
import re

r = requests.get(
    "https://www.ysl.com/en-us/search?q=cassandre+envelope+clutch+grain",
    impersonate="chrome120",
    timeout=45,
)
for p in sorted(set(re.findall(r"/en-us/pr/cassandre-envelope[^\"']+clutch[^\"']+\.html", r.text, re.I)))[:5]:
    print("https://www.ysl.com" + p)
