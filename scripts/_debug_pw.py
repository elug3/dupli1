from playwright.sync_api import sync_playwright
import re, time

url = "https://www.bottegaveneta.com/en-us/cassette-white-578004VMAY19007.html"
with sync_playwright() as p:
    page = p.chromium.launch(headless=True).new_page()
    page.goto(url, wait_until="domcontentloaded", timeout=60000)
    time.sleep(3)
    html = page.content()
    print("len", len(html))
    pat = re.compile(r"bottega-veneta\.dam\.kering\.com[^\"'\s<>]+Medium[^\"'\s<>]+")
    found = pat.findall(html)
    print("matches", len(found))
    for u in sorted(set(found))[:15]:
        print(u[:100])
