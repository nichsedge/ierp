#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["pyyaml>=6.0.0"]
# ///
"""
Exports structured data from the ierp SQLite database into Digital Graveyard
markdown notes. This is the read path: garden content is generated FROM the DB.

Usage:
    python3 export_garden.py            # regenerate media + links notes
    python3 export_garden.py --check    # dry-run: report counts only

Environment overrides:
    IERP_DB          path to ierp events.db      (default: repo-relative)
    GARDEN_CONTENT   garden content root         (default: ~/Projects/digital-graveyard/content)
"""

import argparse
import json
import os
import re
import sqlite3
import sys
from datetime import datetime
from pathlib import Path

import yaml

IERP_DB = os.environ.get("IERP_DB", str(Path(__file__).resolve().parent.parent / "ierp" / "events.db"))
GARDEN_CONTENT = os.environ.get("GARDEN_CONTENT", os.path.expanduser("~/Projects/digital-graveyard/content"))

# media_type -> (content subdir, tags)
MEDIA_TARGETS = {
    "book": ("Read/Hardcover", ["book"]),
    "film": ("Watch/Letterboxd", ["film"]),
    "anime": ("Watch/Anime", ["anime", "film"]),
    "manga": ("Read/Manga", ["manga", "book"]),
    "drama": ("Watch/Drama", ["film", "drama"]),
}

LINKS_NOTE = "Write/Links.md"

# Frontmatter keys excluded from the markdown meta bullet list.
_FM_META_KEYS = {"title", "date", "tags", "publish_external"}


def sanitize_filename(text: str) -> str:
    """Filesystem-safe note filename, preserving readability."""
    no_punc = re.sub(r"[^\w\s\-]", " ", str(text))
    return " ".join(no_punc.split()).strip() or "Untitled"


def yaml_str(v) -> str:
    """Formats a scalar as Obsidian-safe YAML."""
    if v is None:
        return "null"
    s = str(v).replace("\\", "\\\\").replace('"', '\\"')
    if any(c in s for c in ":#[],{}'") or s.lower() in ("true", "false", "null", "yes", "no"):
        return f'"{s}"'
    return s


def render_frontmatter(fm: dict) -> str:
    lines = ["---"]
    for k, v in fm.items():
        if k == "tags":
            lines.append(f"tags: [{', '.join(v)}]" if v else "tags: []")
        elif isinstance(v, bool):
            lines.append(f"{k}: {'true' if v else 'false'}")
        else:
            lines.append(f"{k}: {yaml_str(v)}")
    lines.append("---")
    return "\n".join(lines)


def write_note(rel_path: str, fm: dict, body: str, dry_run: bool = False) -> None:
    path = Path(GARDEN_CONTENT) / rel_path
    if dry_run:
        print(f"  [dry-run] would write {rel_path}")
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(render_frontmatter(fm) + "\n\n" + body.rstrip() + "\n", encoding="utf-8")


def build_media_note(mtype: str, title: str, source: str, data: dict) -> tuple:
    """Returns (frontmatter dict, markdown body) for one media item."""
    _, tags = MEDIA_TARGETS[mtype]
    date = (data.get("date_logged") or data.get("finished_at")
            or data.get("started_at") or "2016-01-01")

    fm = {"title": title, "date": date, "tags": tags, "publish_external": False}
    for key in ("author", "year", "original_title", "status", "rating",
                "progress", "started_at", "finished_at", "source"):
        val = data.get(key) or (source if key == "source" else None)
        if val not in (None, ""):
            fm[key] = val

    lines = [f"# {title}", ""]
    meta = [(k.replace("_", " ").capitalize(), v) for k, v in fm.items() if k not in _FM_META_KEYS]
    if meta:
        lines += [f"- **{k}:** {v}" for k, v in meta]
    review = data.get("review")
    if review:
        lines += ["", "## Review", "", str(review)]
    return fm, "\n".join(lines)


def export_media(conn: sqlite3.Connection, dry_run: bool = False) -> dict:
    cur = conn.cursor()
    rows = cur.execute(
        "SELECT media_type, title, source, data_json FROM media_items"
    ).fetchall()

    counts: dict = {}
    expected: dict = {}  # subdir -> set of filenames the DB says should exist
    for mtype, title, source, data_json in rows:
        if mtype not in MEDIA_TARGETS:
            continue  # unknown media_type: skip rather than crash on new sources

        try:
            data = json.loads(data_json or "{}")
        except (json.JSONDecodeError, TypeError):
            data = {}
        data.setdefault("source", source)

        fm, body = build_media_note(mtype, title, source, data)
        subdir, _ = MEDIA_TARGETS[mtype]
        filename = f"{sanitize_filename(title)}.md"
        rel = str(Path(subdir) / filename)
        expected.setdefault(subdir, set()).add(filename)
        write_note(rel, fm, body, dry_run)
        counts[mtype] = counts.get(mtype, 0) + 1

    pruned = prune_stale_notes(expected, dry_run)
    counts["_pruned"] = pruned
    return counts


def prune_stale_notes(expected: dict, dry_run: bool = False) -> int:
    """
    Deletes .md files in managed media dirs that no longer correspond to any
    DB row (e.g. after source switches or title changes). Only touches the
    MEDIA_TARGETS directories — never other garden content.
    """
    pruned = 0
    for subdir, filenames in expected.items():
        dir_path = Path(GARDEN_CONTENT) / subdir
        if not dir_path.is_dir():
            continue
        for f in sorted(dir_path.glob("*.md")):
            if f.name not in filenames:
                if dry_run:
                    print(f"  [dry-run] would prune {subdir}/{f.name}")
                else:
                    f.unlink()
                pruned += 1
    return pruned


def export_links(conn: sqlite3.Connection, dry_run: bool = False) -> int:
    rows = conn.execute(
        "SELECT label, url, category, is_public FROM links ORDER BY category, label"
    ).fetchall()

    by_cat: dict = {}
    for label, url, cat, pub in rows:
        by_cat.setdefault(cat or "Other", []).append((label, url, pub))

    body_lines = ["# Links", ""]
    for cat in sorted(by_cat):
        body_lines += [f"## {cat}", ""]
        for label, url, pub in by_cat[cat]:
            body_lines.append(f"* **{label}:** {url}" + ("" if pub else " *(private)*"))
        body_lines.append("")

    fm = {
        "title": "Links",
        "date": datetime.now().strftime("%Y-%m-%d"),
        "tags": ["note"],
        "publish_external": True,
    }
    write_note(LINKS_NOTE, fm, "\n".join(body_lines), dry_run)
    return len(rows)


def main() -> int:
    parser = argparse.ArgumentParser(description="Export ierp DB -> Digital Graveyard notes")
    parser.add_argument("--check", action="store_true", help="Dry run")
    args = parser.parse_args()

    db = Path(IERP_DB)
    if not db.exists():
        print(f"ierp DB not found at {db}", file=sys.stderr)
        return 1

    conn = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    try:
        counts = export_media(conn, args.check)
        n_links = export_links(conn, args.check)
    finally:
        conn.close()

    mode = " [dry-run]" if args.check else ""
    print(f"\nExported from ierp{mode}:")
    for mtype, n in sorted(counts.items()):
        if mtype == "_pruned":
            print(f"  - pruned : {n:4d} stale notes")
        else:
            print(f"  - {mtype:8}: {n:4d} notes")
    print(f"  - links  : {n_links} entries -> {LINKS_NOTE}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
