#!/usr/bin/env python3
from curl_cffi import requests
import re


def probe(name, url):
    try:
        r = requests.get(url, impersonate="chrome120", timeout=45)
        ysl = len(re.findall(r"saint-laurent\.dam\.kering", r.text))
        celine = len(re.findall(r"celine\.dam\.kering", r.text))
        bv_m = len(re.findall(r"bottega-veneta\.dam\.kering\.com/m/", r.text))
        images_rd = len(re.findall("images_rd", r.text))
        print(
            f"{name}: {r.status_code} len={len(r.text)} "
            f"images_rd={images_rd} ysl={ysl} celine={celine} bv_m={bv_m}"
        )
        if ysl:
            for u in re.findall(
                r"https://saint-laurent\.dam\.kering\.com/[^\s\"'<>]+?\.jpg[^\s\"'<>]*",
                r.text,
            )[:5]:
                print(" ", u[:130])
        if celine:
            for u in re.findall(
                r"https://celine\.dam\.kering\.com/[^\s\"'<>]+?\.jpg[^\s\"'<>]*",
                r.text,
            )[:5]:
                print(" ", u[:130])
        if bv_m:
            for u in re.findall(
                r"https://bottega-veneta\.dam\.kering\.com/m/[^\s\"'<>]+?\.jpg[^\s\"'<>]*",
                r.text,
            )[:5]:
                print(" ", u[:130])
        if images_rd:
            for u in re.findall(
                r"default/[a-z0-9]{10}/images_rd/[^\s\"'<>]+?\.jpg",
                r.text,
            )[:3]:
                print(" ", u[:130])
    except Exception as exc:
        print(f"{name}: ERR {exc}")


if __name__ == "__main__":
    probe(
        "loewe gate",
        "https://www.loewe.com/int/en/women/bags/gate/mini-gate-dual-bag-in-soft-calfskin-and-jacquard/A650N46X13-8020.html",
    )
    probe(
        "loewe balloon",
        "https://www.loewe.com/int/en/women/bags/balloon/small-balloon-bag-in-nappa-calfskin/A411FC2X01-1100.html",
    )
    probe("bv hop", "https://www.bottegaveneta.com/en-us/hop-small-black-806452V2H211443.html")
    probe(
        "bv cassette",
        "https://www.bottegaveneta.com/en-us/cassette-white-578004VMAY19007.html",
    )
    probe(
        "ysl niki",
        "https://www.ysl.com/en-us/pr/niki-medium-in-grained-lambskin-633178AACYT3212.html",
    )
    probe(
        "ysl lou",
        "https://www.ysl.com/en-us/shop-women/handbags/all-handbags",
    )
    probe(
        "celine triomphe",
        "https://www.celine.com/en/women/handbags/triomphe/triomphe-shoulder-bag-in-shiny-calfskin-188373BF4.38NO.html",
    )
