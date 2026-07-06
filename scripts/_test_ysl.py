from curl_cffi import requests
import re

u = "https://www.ysl.com/en-en/women/handbags/lou-camera-bag-in-quilted-grain-de-poudre-embossed-leather-6125421EL071000.html"
r = requests.get(u, impersonate="chrome120", timeout=45)
print(r.status_code, len(r.text))
imgs = re.findall(r"https://ysl\.dam\.kering\.com[^\"'\s<>]+", r.text)
print("kering", len(set(imgs)))
for i in sorted(set(imgs))[:10]:
    print(i[:120])
