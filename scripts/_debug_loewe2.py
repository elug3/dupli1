from curl_cffi import requests
import re

h = requests.get(
    "https://www.loewe.com/int/en/women/bags/puzzle/small-featherlight-puzzle-bag-in-nappa-lambskin/A510PLSX01-9190.html",
    impersonate="chrome120",
    timeout=45,
).text

# Try simpler pattern
for pat in [
    r"images_rd/[^\"\\]+?\.jpg",
    r"/images_rd/[A-Z0-9/._-]+\.jpg",
    r"A510PLSX01_9190_[A-Z0-9]+\.jpg",
]:
    found = re.findall(pat, h)
    print(pat, len(set(found)))
    for x in sorted(set(found))[:5]:
        print(" ", x[:90])
