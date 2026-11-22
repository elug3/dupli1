#!/usr/bin/env python3
"""Load the product reference data (brands, base colors, colors, sizes,
editions) from scripts/seed/reference-data.json through the admin API.

The product service no longer seeds these at startup — only OS and the
sizes a category sells, which follow from code. Run this once on a new environment,
or again after editing the JSON: it only creates what is missing and never
changes or deletes an existing row, so staff edits stay.

    BASE=http://localhost:8080 scripts/seed-reference-data.py
    scripts/seed-reference-data.py --base-url https://api.example --email ops@x --dry-run

The password comes from --password or DUPLI1_ADMIN_PASSWORD; without either
it is asked for (hidden). Only DUPLI1_ENV=dev falls back to "password", the
local seeded owner. Like the services, an unset DUPLI1_ENV means prod
(shared/pkg/runenv). The account needs product.master.write.

    DUPLI1_ENV=dev BASE=http://localhost:8080 scripts/seed-reference-data.py
"""

from __future__ import annotations

import argparse
import getpass
import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path

DATA = Path(__file__).resolve().parent / "seed" / "reference-data.json"
CATALOG = "/api/v1/products/catalog"

# Order matters: a color points at its base color.
KINDS = [
    ("base_colors", "base-colors"),
    ("colors", "colors"),
    ("sizes", "sizes"),
    ("editions", "editions"),
    ("brands", "brands"),
]


class API:
    def __init__(self, base: str):
        self.base = base.rstrip("/")
        self.token = ""

    def call(self, method: str, path: str, body=None):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(self.base + path, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if self.token:
            req.add_header("Authorization", f"Bearer {self.token}")
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                raw = resp.read()
                return resp.status, json.loads(raw) if raw else None
        except urllib.error.HTTPError as err:
            raw = err.read()
            try:
                return err.code, json.loads(raw)
            except ValueError:
                return err.code, raw.decode(errors="replace")

    def login(self, email: str, password: str) -> None:
        status, body = self.call("POST", "/api/v1/auth/login",
                                 {"email": email, "password": password, "client": "manage"})
        if status != 200:
            sys.exit(f"login {email}: {status} {body}")
        status, body = self.call("POST", "/api/v1/auth/refresh",
                                 {"refresh_token": body["refresh_token"]})
        if status != 200:
            sys.exit(f"refresh: {status} {body}")
        self.token = body["token"]


def run_mode() -> str:
    """DUPLI1_ENV as the services read it: blank is prod."""
    mode = os.environ.get("DUPLI1_ENV", "").strip().lower() or "prod"
    if mode not in ("dev", "prod"):
        sys.exit(f'DUPLI1_ENV must be "dev" or "prod", got "{mode}"')
    return mode


def admin_password(email: str) -> str:
    if run_mode() == "dev":
        return "password"  # the local seeded owner (docker-compose.yml)
    if not sys.stdin.isatty():
        sys.exit("no password: pass --password or DUPLI1_ADMIN_PASSWORD "
                 "(or DUPLI1_ENV=dev for the local seeded owner)")
    return getpass.getpass(f"password for {email}: ")


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--base-url", default=os.environ.get("BASE", "http://localhost:8080"))
    p.add_argument("--email", default=os.environ.get("DUPLI1_ADMIN_EMAIL", "admin@dupli1.com"))
    p.add_argument("--password", default=os.environ.get("DUPLI1_ADMIN_PASSWORD", ""))
    p.add_argument("--data", type=Path, default=DATA)
    p.add_argument("--dry-run", action="store_true", help="report what would be created, write nothing")
    args = p.parse_args()
    if not args.password:
        args.password = admin_password(args.email)

    data = json.loads(args.data.read_text(encoding="utf-8"))
    api = API(args.base_url)
    api.login(args.email, args.password)

    failed = 0
    for key, path in KINDS:
        status, existing = api.call("GET", f"{CATALOG}/{path}")
        if status != 200:
            sys.exit(f"list {path}: {status} {existing}")
        have = {row["code"] for row in existing or []}
        created = skipped = 0
        for row in data.get(key, []):
            if row["code"] in have:
                skipped += 1
                continue
            if args.dry_run:
                print(f"  would create {key[:-1]} {row['code']} {row['name']}")
                created += 1
                continue
            status, body = api.call("POST", f"{CATALOG}/{path}", row)
            if status in (200, 201):
                created += 1
            elif status == 409:  # created by someone else since the list
                skipped += 1
            else:
                print(f"  ! {key[:-1]} {row['code']}: {status} {body}", file=sys.stderr)
                failed += 1
        verb = "would create" if args.dry_run else "created"
        print(f"{key}: {verb} {created}, already there {skipped}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
