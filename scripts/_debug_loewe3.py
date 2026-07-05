from curl_cffi import requests
import re

h = requests.get(
    "https://www.loewe.com/int/en/women/bags/puzzle/small-featherlight-puzzle-bag-in-nappa-lambskin/A510PLSX01-9190.html",
    impersonate="chrome120",
    timeout=45,
).text

# Full URLs in JSON may be escaped
pat = re.compile(
    r"https:\\/\\/www\.loewe\.com\\/dw\\/image\\/v2\\/BBPC_PRD\\/on\\/demandware\.static\\/"
    r"-\\/Sites-Loewe_master\\/default\\/([a-z0-9]+)\\/images_rd\\/"
    r"(A510PLSX01\\/A510PLSX01-9190\\/A510PLSX01_9190_[A-Z0-9]+\\.jpg)",
    re.I,
)
found = pat.findall(h)
print("escaped full", len(found))
by_hash = {}
for hash_id, path in found:
    path = path.replace("\\/", "/")
    by_hash.setdefault(path.rsplit("_", 1)[-1], hash_id)

# Unescaped
pat2 = re.compile(
    r"https://www\.loewe\.com/dw/image/v2/BBPC_PRD/on/demandware\.static/"
    r"-/Sites-Loewe_master/default/([a-z0-9]+)/images_rd/"
    r"(A510PLSX01/A510PLSX01-9190/A510PLSX01_9190_[A-Z0-9]+\.jpg)",
    re.I,
)
found2 = pat2.findall(h)
print("unescaped full", len(set(found2)))

# Build URLs from relative paths + hash lookup in html
rel_pat = re.compile(r"(A510PLSX01/A510PLSX01-9190/A510PLSX01_9190_[A-Z0-9]+\.jpg)")
rels = sorted(set(rel_pat.findall(h)))
print("rel angles", len(rels))
for rel in rels[:15]:
    # find hash before this path in html snippet
    idx = h.find(rel.replace("/", "\\/"))
    if idx < 0:
        idx = h.find(rel)
    snippet = h[max(0, idx - 120): idx + len(rel)]
    hm = re.search(r"default/([a-z0-9]{10})/images_rd", snippet)
    hash_id = hm.group(1) if hm else "UNKNOWN"
    url = (
        f"https://www.loewe.com/dw/image/v2/BBPC_PRD/on/demandware.static/"
        f"-/Sites-Loewe_master/default/{hash_id}/images_rd/{rel}?sw=2000&q=90"
    )
    print(url[:130])
