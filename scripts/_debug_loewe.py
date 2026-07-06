from curl_cffi import requests
import re

url = "https://www.loewe.com/int/en/women/bags/puzzle/small-featherlight-puzzle-bag-in-nappa-lambskin/A510PLSX01-9190.html"
h = requests.get(url, impersonate="chrome120", timeout=45).text
print("len", len(h))
pat = re.compile(r"https://www\.loewe\.com/dw/image/v2/BBPC_PRD/on/demandware\.static/-/Sites-Loewe_master/default/[^\"'\s<>]+?/images_rd/[^\"'\s<>]+?\.jpg")
found = sorted(set(pat.findall(h)))
print("imgs", len(found))
for u in found[:12]:
    print(u[:110])
