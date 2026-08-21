#!/usr/bin/env python3
"""
Exports payment accounts & referrals from the ierp SQLite database back to
the portfolio's data/ JSON files (identical schema, zero app changes).

Usage:
    python3 export_commerce.py            # writes pay.json + referrals.json
    python3 export_commerce.py --check    # dry run
"""

import argparse
import json
import os
import sqlite3

IERP_DB = os.environ.get("IERP_DB", os.path.expanduser("~/Projects/ierp/ierp/events.db"))
PORTFOLIO_DATA = os.environ.get(
    "PORTFOLIO_DATA",
    os.path.expanduser("~/Projects/nichsedge.github.io/data"),
)


def export_pay(conn):
    rows = conn.execute(
        "SELECT slug, name, category, number, recipient, details, details_id "
        "FROM payment_accounts ORDER BY id"
    ).fetchall()
    return [
        {
            "id": slug, "name": name, "category": category,
            "number": number, "recipient": recipient,
            "details": details, "details_id": details_id,
        }
        for slug, name, category, number, recipient, details, details_id in rows
    ]


def export_referrals(conn):
    rows = conn.execute(
        "SELECT slug, name, category, code, link, benefit, status "
        "FROM referrals WHERE is_public = 1 ORDER BY id"
    ).fetchall()
    return [
        {
            "id": slug, "name": name, "category": category,
            "code": code, "link": link, "benefit": benefit, "status": status,
        }
        for slug, name, category, code, link, benefit, status in rows
    ]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()

    conn = sqlite3.connect(f"file:{IERP_DB}?mode=ro", uri=True)
    try:
        pay = export_pay(conn)
        referrals = export_referrals(conn)
    finally:
        conn.close()

    targets = {
        "pay.json": pay,
        "referrals.json": referrals,
    }
    for fname, data in targets.items():
        path = os.path.join(PORTFOLIO_DATA, fname)
        payload = json.dumps(data, indent=1, ensure_ascii=False) + "\n"
        if args.check:
            print(f"[dry-run] would write {len(data)} records -> {path}")
            continue
        with open(path, "w", encoding="utf-8") as f:
            f.write(payload)
        print(f"Wrote {len(data)} records -> {path}")


if __name__ == "__main__":
    main()
