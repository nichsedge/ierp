#!/usr/bin/env python3
"""Sync GitHub projects and repositories into iERP SQLite database (events.db).

- Extracts repositories for the authenticated GitHub user via `gh api graphql`.
- Affiliations: OWNER, COLLABORATOR, ORGANIZATION_MEMBER.
- Upserts all repository metadata into `github_repositories` table in `events.db`.
- Optionally archives historical JSON/CSV datasets to `--archive-dir`.

Usage:
    uv run scripts/sync_github_repos.py
    uv run scripts/sync_github_repos.py --archive
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import json
import os
from pathlib import Path
import sqlite3
import subprocess
from typing import Any, Dict, List, Optional

IERP_ROOT = Path(__file__).resolve().parent.parent
DEFAULT_DB = IERP_ROOT / "ierp" / "events.db"
DEFAULT_ARCHIVE_DIR = Path.home() / "Projects" / ".data" / "github-projects-dataset"

QUERY = r'''
query($after:String){
  viewer {
    login
    repositories(
      first: 30,
      after: $after,
      affiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER],
      orderBy: {field: UPDATED_AT, direction: DESC}
    ) {
      pageInfo { hasNextPage endCursor }
      nodes {
        id
        name
        nameWithOwner
        url
        description
        homepageUrl
        isPrivate
        isFork
        isArchived
        isTemplate
        isDisabled
        createdAt
        updatedAt
        pushedAt
        primaryLanguage { name }
        defaultBranchRef {
          name
          target {
            __typename
            ... on Commit { oid }
          }
        }
        stargazerCount
        forkCount
        watchers { totalCount }
        issues(states: OPEN) { totalCount }
        pullRequests(states: OPEN) { totalCount }
        repositoryTopics(first: 50) {
          nodes { topic { name } }
        }
        licenseInfo { spdxId name }
        owner {
          __typename
          login
          url
        }
      }
    }
  }
}
'''


def run_cmd(cmd: List[str]) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, check=False, text=True, capture_output=True)


def gh_graphql(after: Optional[str]) -> Dict[str, Any]:
    cmd = ["gh", "api", "graphql", "-f", f"query={QUERY}", "-F", f"after={(after if after is not None else 'null')}"]
    p = run_cmd(cmd)
    if p.returncode != 0:
        raise SystemExit(f"gh api graphql failed (code={p.returncode})\nSTDOUT:\n{p.stdout}\nSTDERR:\n{p.stderr}")
    try:
        return json.loads(p.stdout)
    except json.JSONDecodeError as e:
        raise SystemExit(f"Failed to parse JSON from gh output: {e}\nOutput head:\n{p.stdout[:2000]}")


def flatten_repo(n: Dict[str, Any]) -> Dict[str, Any]:
    topics = []
    for t in ((n.get("repositoryTopics") or {}).get("nodes") or []):
        name = ((t or {}).get("topic") or {}).get("name")
        if name:
            topics.append(name)

    primary_language = (n.get("primaryLanguage") or {}).get("name")

    dbr = n.get("defaultBranchRef") or {}
    default_branch = dbr.get("name")
    default_branch_oid = None
    target = dbr.get("target") or {}
    if target.get("__typename") == "Commit":
        default_branch_oid = target.get("oid")

    license_info = n.get("licenseInfo") or {}
    owner = n.get("owner") or {}

    return {
        "repo_id": n.get("id"),
        "name": n.get("name"),
        "full_name": n.get("nameWithOwner"),
        "owner_login": owner.get("login"),
        "owner_url": owner.get("url"),
        "html_url": n.get("url"),
        "homepage": n.get("homepageUrl"),
        "description": n.get("description"),
        "topics": json.dumps(topics, ensure_ascii=False),
        "language": primary_language,
        "private": 1 if n.get("isPrivate") else 0,
        "fork": 1 if n.get("isFork") else 0,
        "archived": 1 if n.get("isArchived") else 0,
        "template": 1 if n.get("isTemplate") else 0,
        "disabled": 1 if n.get("isDisabled") else 0,
        "created_at": n.get("createdAt"),
        "updated_at": n.get("updatedAt"),
        "pushed_at": n.get("pushedAt"),
        "default_branch": default_branch,
        "default_branch_oid": default_branch_oid,
        "stargazers_count": n.get("stargazerCount") or 0,
        "watchers_count": (n.get("watchers") or {}).get("totalCount") or 0,
        "forks_count": n.get("forkCount") or 0,
        "open_issues_count": (n.get("issues") or {}).get("totalCount") or 0,
        "open_prs_count": (n.get("pullRequests") or {}).get("totalCount") or 0,
        "license_spdx": license_info.get("spdxId"),
        "license_name": license_info.get("name"),
    }


def upsert_repositories(db_path: Path, repos: List[Dict[str, Any]]) -> int:
    conn = sqlite3.connect(str(db_path), timeout=10.0)
    conn.execute("PRAGMA journal_mode = WAL;")
    conn.execute("PRAGMA busy_timeout = 5000;")

    sql = """
    INSERT INTO github_repositories (
        repo_id, name, full_name, owner_login, owner_url, html_url,
        homepage, description, topics, language, private, fork,
        archived, template, disabled, created_at, updated_at, pushed_at,
        default_branch, default_branch_oid, stargazers_count, watchers_count,
        forks_count, open_issues_count, open_prs_count, license_spdx, license_name,
        synced_at
    ) VALUES (
        :repo_id, :name, :full_name, :owner_login, :owner_url, :html_url,
        :homepage, :description, :topics, :language, :private, :fork,
        :archived, :template, :disabled, :created_at, :updated_at, :pushed_at,
        :default_branch, :default_branch_oid, :stargazers_count, :watchers_count,
        :forks_count, :open_issues_count, :open_prs_count, :license_spdx, :license_name,
        datetime('now', 'localtime')
    )
    ON CONFLICT(repo_id) DO UPDATE SET
        name = excluded.name,
        full_name = excluded.full_name,
        owner_login = excluded.owner_login,
        owner_url = excluded.owner_url,
        html_url = excluded.html_url,
        homepage = excluded.homepage,
        description = excluded.description,
        topics = excluded.topics,
        language = excluded.language,
        private = excluded.private,
        fork = excluded.fork,
        archived = excluded.archived,
        template = excluded.template,
        disabled = excluded.disabled,
        created_at = excluded.created_at,
        updated_at = excluded.updated_at,
        pushed_at = excluded.pushed_at,
        default_branch = excluded.default_branch,
        default_branch_oid = excluded.default_branch_oid,
        stargazers_count = excluded.stargazers_count,
        watchers_count = excluded.watchers_count,
        forks_count = excluded.forks_count,
        open_issues_count = excluded.open_issues_count,
        open_prs_count = excluded.open_prs_count,
        license_spdx = excluded.license_spdx,
        license_name = excluded.license_name,
        synced_at = datetime('now', 'localtime');
    """

    cursor = conn.cursor()
    cursor.executemany(sql, repos)
    conn.commit()
    count = cursor.rowcount
    conn.close()
    return count


def archive_dataset(archive_dir: Path, repos: List[Dict[str, Any]]) -> None:
    archive_dir.mkdir(parents=True, exist_ok=True)
    today = dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%d")
    out_dir = archive_dir / today
    latest_dir = archive_dir / "latest"
    out_dir.mkdir(parents=True, exist_ok=True)
    latest_dir.mkdir(parents=True, exist_ok=True)

    # Format nodes for archive output
    formatted_rows = []
    for r in repos:
        row = dict(r)
        row["id"] = row.pop("repo_id")
        row["topics"] = json.loads(row["topics"])
        row["private"] = bool(row["private"])
        row["fork"] = bool(row["fork"])
        row["archived"] = bool(row["archived"])
        row["template"] = bool(row["template"])
        row["disabled"] = bool(row["disabled"])
        formatted_rows.append(row)

    meta = {
        "generated_at_utc": dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "source": "ierp (gh api graphql)",
        "affiliations": ["OWNER", "COLLABORATOR", "ORGANIZATION_MEMBER"],
        "count": len(formatted_rows),
    }

    payload = {**meta, "repos": formatted_rows}
    json_path = out_dir / "github_repos_all.json"
    json_path.write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    (latest_dir / "github_repos_all.json").write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

    print(f"Archived dataset to {out_dir} & {latest_dir}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Sync GitHub repos into iERP events.db")
    parser.add_argument("--db", type=Path, default=Path(os.environ.get("IERP_DB", DEFAULT_DB)), help="Path to events.db")
    parser.add_argument("--archive", action="store_true", default=True, help="Also archive dataset to .data/ (default: True)")
    parser.add_argument("--no-archive", action="store_false", dest="archive")
    parser.add_argument("--archive-dir", type=Path, default=DEFAULT_ARCHIVE_DIR, help="Base directory for historical datasets")
    args = parser.parse_args()

    if not args.db.exists():
        raise SystemExit(f"events.db not found at {args.db}")

    print(f"Querying GitHub GraphQL API...")
    all_nodes: List[Dict[str, Any]] = []
    after: Optional[str] = None

    for _ in range(200):
        data = gh_graphql(after)
        if "errors" in data:
            raise SystemExit(f"GraphQL returned errors: {json.dumps(data['errors'], indent=2)}")
        repos_data = data["data"]["viewer"]["repositories"]
        nodes = repos_data.get("nodes") or []
        all_nodes.extend(nodes)

        if not repos_data["pageInfo"]["hasNextPage"]:
            break
        after = repos_data["pageInfo"]["endCursor"]
    else:
        raise SystemExit("Pagination exceeded 200 pages; aborting")

    flattened = [flatten_repo(n) for n in all_nodes]
    print(f"Fetched {len(flattened)} repositories from GitHub.")

    upsert_repositories(args.db, flattened)
    print(f"Upserted {len(flattened)} repositories into {args.db} (github_repositories table).")

    if args.archive:
        archive_dataset(args.archive_dir, flattened)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
