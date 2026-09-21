#!/usr/bin/env python3
"""
Exports payment accounts & referrals from the ierp SQLite database back to
the portfolio's data/ JSON files (identical schema, zero app changes).

Usage:
    python3 export_commerce.py            # writes pay.json + referrals.json
    python3 export_commerce.py --check    # dry run

Environment overrides:
    IERP_DB         path to ierp events.db   (default: repo-relative)
    PORTFOLIO_DATA  portfolio data dir       (default: ~/Projects/nichsedge.github.io/data)
"""

import argparse
import json
import os
import sqlite3
import sys
from pathlib import Path

IERP_DB = os.environ.get("IERP_DB", str(Path(__file__).resolve().parent.parent / "ierp" / "events.db"))
PORTFOLIO_DATA = os.environ.get(
    "PORTFOLIO_DATA",
    os.path.expanduser("~/Projects/nichsedge.github.io/data"),
)


def export_pay(conn: sqlite3.Connection) -> list:
    rows = conn.execute(
        "SELECT slug, name, category, number, recipient, details, details_id "
        "FROM payment_accounts ORDER BY id"
    ).fetchall()
    keys = ("id", "name", "category", "number", "recipient", "details", "details_id")
    return [dict(zip(keys, row)) for row in rows]


def export_referrals(conn: sqlite3.Connection) -> list:
    rows = conn.execute(
        "SELECT slug, name, category, code, link, benefit, status "
        "FROM referrals WHERE is_public = 1 ORDER BY id"
    ).fetchall()
    keys = ("id", "name", "category", "code", "link", "benefit", "status")
    return [dict(zip(keys, row)) for row in rows]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true", help="Dry run: report counts only")
    args = parser.parse_args()

    db = Path(IERP_DB)
    if not db.exists():
        print(f"ierp DB not found at {db}", file=sys.stderr)
        return 1

    conn = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    try:
        targets = {
            "pay.json": export_pay(conn),
            "referrals.json": export_referrals(conn),
        }
    finally:
        conn.close()

    data_dir = Path(PORTFOLIO_DATA)
    if not data_dir.exists():
        print(f"Target portfolio data directory not found at {data_dir}. Skipping commerce export.")
        return 0

    for fname, data in targets.items():
        path = data_dir / fname
        if args.check:
            print(f"[dry-run] would write {len(data)} records -> {path}")
            continue
        path.write_text(json.dumps(data, indent=1, ensure_ascii=False) + "\n", encoding="utf-8")
        print(f"Wrote {len(data)} records -> {path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
