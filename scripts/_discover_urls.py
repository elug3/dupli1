#!/usr/bin/env python3
from curl_cffi import requests
import re


def fetch(url):
    r = requests.get(url, impersonate="chrome120", timeout=45)
    return r.status_code, r.text


def loewe_products(url):
    code, html = fetch(url)
    links = sorted(set(re.findall(r'href="(/[^"]+/bags/[^"]+\.html)"', html)))
    print(f"\n{url} -> {code} links={len(links)}")
    for link in links[:15]:
        print(" ", link)


def ysl_products(url):
    code, html = fetch(url)
    links = sorted(set(re.findall(r'href="(/en-us/pr/[^"]+\.html)"', html)))
    print(f"\n{url} -> {code} pr links={len(links)}")
    for link in links[:10]:
        print(" ", link)


if __name__ == "__main__":
    loewe_products("https://www.loewe.com/usa/en/women/bags/gate")
    loewe_products("https://www.loewe.com/usa/en/women/bags/balloon")
    for u in [
        "https://www.loewe.com/usa/en/women/bags/gate/mini-gate-dual-bag-in-soft-calfskin-and-jacquard/A650N46X13-8020.html",
        "https://www.loewe.com/int/en/women/bags/gate/mini-gate-dual-bag-in-soft-calfskin-and-jacquard/A650N46X13-8020.html",
    ]:
        code, html = fetch(u)
        print(f"\n{u.split('/')[-1]}: {code} images_rd={html.count('images_rd')}")

    ysl_products("https://www.ysl.com/en-us/shop-women/handbags/all-handbags")
