from curl_cffi import requests
import re

h = requests.get(
    "https://www.bottegaveneta.com/en-us/cassette-white-578004VMAY19007.html",
    impersonate="chrome120",
    timeout=45,
).text
m = re.findall(
    r"https://bottega-veneta\.dam\.kering\.com/m/[a-f0-9]+/Medium-[A-Z0-9]+_[A-Z]\.jpg(?:\?v=\d+)?",
    h,
)
print("m urls", len(set(m)))
for u in sorted(set(m))[:12]:
    print(u)
