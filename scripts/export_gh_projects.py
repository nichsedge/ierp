#!/usr/bin/env python3
"""Export GitHub projects/repositories from iERP SQLite database to portfolio site data.

Outputs JSON matching the schema expected by nichsedge.github.io/app/projects/projects-client.tsx:
    ~/Projects/nichsedge.github.io/data/github_repos_all.json

Usage:
    uv run scripts/export_gh_projects.py            # Writes github_repos_all.json
    uv run scripts/export_gh_projects.py --check    # Dry run: report counts only

Environment overrides:
    IERP_DB         path to ierp events.db   (default: repo-relative)
    PORTFOLIO_DATA  portfolio data dir       (default: ~/Projects/nichsedge.github.io/data)
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
from pathlib import Path
import sqlite3
import sys
from typing import Any, Dict, List

IERP_DB = os.environ.get(
    "IERP_DB",
    str(Path(__file__).resolve().parent.parent / "ierp" / "events.db"),
)
PORTFOLIO_DATA = os.environ.get(
    "PORTFOLIO_DATA",
    os.path.expanduser("~/Projects/nichsedge.github.io/data"),
)


def export_repos(conn: sqlite3.Connection) -> List[Dict[str, Any]]:
    conn.row_factory = sqlite3.Row
    cursor = conn.execute("""
        SELECT
            repo_id, name, full_name, owner_login, owner_url, html_url,
            homepage, description, topics, language, private, fork,
            archived, template, disabled, created_at, updated_at, pushed_at,
            default_branch, default_branch_oid, stargazers_count, watchers_count,
            forks_count, open_issues_count, open_prs_count, license_spdx, license_name
        FROM github_repositories
        ORDER BY pushed_at DESC, updated_at DESC
    """)
    rows = cursor.fetchall()

    results = []
    for r in rows:
        topics_val = r["topics"]
        try:
            topics = json.loads(topics_val) if topics_val else []
        except (json.JSONDecodeError, TypeError):
            topics = []

        results.append({
            "id": r["repo_id"],
            "name": r["name"],
            "full_name": r["full_name"],
            "owner_login": r["owner_login"],
            "owner_url": r["owner_url"],
            "html_url": r["html_url"],
            "homepage": r["homepage"],
            "description": r["description"],
            "topics": topics,
            "language": r["language"],
            "private": bool(r["private"]),
            "fork": bool(r["fork"]),
            "archived": bool(r["archived"]),
            "template": bool(r["template"]),
            "disabled": bool(r["disabled"]),
            "created_at": r["created_at"],
            "updated_at": r["updated_at"],
            "pushed_at": r["pushed_at"],
            "default_branch": r["default_branch"],
            "default_branch_oid": r["default_branch_oid"],
            "stargazers_count": r["stargazers_count"],
            "watchers_count": r["watchers_count"],
            "forks_count": r["forks_count"],
            "open_issues_count": r["open_issues_count"],
            "open_prs_count": r["open_prs_count"],
            "license_spdx": r["license_spdx"],
            "license_name": r["license_name"],
        })
    return results


def main() -> int:
    parser = argparse.ArgumentParser(description="Export GitHub repositories from iERP to portfolio site")
    parser.add_argument("--check", action="store_true", help="Dry run: report counts only")
    parser.add_argument(
        "--output",
        type=Path,
        default=None,
        help="Optional custom output path (default: $PORTFOLIO_DATA/github_repos_all.json)",
    )
    args = parser.parse_args()

    db = Path(IERP_DB)
    if not db.exists():
        print(f"ierp DB not found at {db}", file=sys.stderr)
        return 1

    conn = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    try:
        repos = export_repos(conn)
    finally:
        conn.close()

    payload = {
        "generated_at_utc": dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "source": "ierp (events.db)",
        "affiliations": ["OWNER", "COLLABORATOR", "ORGANIZATION_MEMBER"],
        "count": len(repos),
        "repos": repos,
    }

    if args.output:
        dest_file = args.output
    else:
        dest_dir = Path(PORTFOLIO_DATA)
        if not dest_dir.exists():
            print(f"Target portfolio data directory not found at {dest_dir}. Skipping export.")
            return 0
        dest_file = dest_dir / "github_repos_all.json"

    if args.check:
        print(f"[dry-run] would write {len(repos)} repositories -> {dest_file}")
        return 0

    dest_file.parent.mkdir(parents=True, exist_ok=True)
    dest_file.write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"Wrote {len(repos)} repositories -> {dest_file}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
