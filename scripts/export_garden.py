#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["pyyaml>=6.0.0"]
# ///
"""
Exports structured data from the ierp SQLite database into Digital Graveyard
markdown notes. This is the read path: garden content is generated FROM the DB.

Usage:
    uv run export_from_ierp.py            # regenerate media + links notes
    uv run export_from_ierp.py --check    # dry-run: report counts only
"""

import argparse
import os
import sqlite3
import sys
from pathlib import Path
from pathlib import Path
from datetime import datetime

import yaml

IERP_DB = os.environ.get("IERP_DB", str(Path(__file__).resolve().parent.parent / "ierp" / "events.db"))
GARDEN_CONTENT = os.environ.get("GARDEN_CONTENT", os.path.expanduser("~/Projects/digital-graveyard/content"))
CONTENT_ROOT = GARDEN_CONTENT

# source_key -> (content subdir, tags, frontmatter title prefix)
MEDIA_TARGETS = {
    "book": ("Read/Goodreads", ["book"]),
    "film": ("Watch/Letterboxd", ["film"]),
    "anime": ("Watch/Anime", ["anime", "film"]),
    "manga": ("Read/Manga", ["manga", "book"]),
    "drama": ("Watch/Drama", ["film", "drama"]),
}

LINKS_NOTE = "Write/Links.md"


def sanitize_filename(text):
    import re
    no_punc = re.sub(r"[^\w\s\-]", " ", str(text))
    return " ".join(no_punc.split()).strip() or "Untitled"


def yaml_str(v):
    if v is None:
        return "null"
    s = str(v).replace("\\", "\\\\").replace('"', '\\"')
    if any(c in s for c in ":#[],{}'") or s.lower() in ("true", "false", "null", "yes", "no"):
        return f'"{s}"'
    return s


def write_note(rel_path, frontmatter, body, dry_run=False):
    path = os.path.join(CONTENT_ROOT, rel_path)
    if dry_run:
        print(f"  [dry-run] would write {rel_path}")
        return
    os.makedirs(os.path.dirname(path), exist_ok=True)
    lines = ["---"]
    for k, v in frontmatter.items():
        if k in ("tags",):
            lines.append(f"{k}: [{', '.join(v)}]" if v else f"{k}: []")
        elif isinstance(v, bool):
            lines.append(f"{k}: {'true' if v else 'false'}")
        else:
            lines.append(f"{k}: {yaml_str(v)}")
    lines.append("---")
    lines.append("")
    lines.append(body.rstrip() + "\n")
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))


def export_media(conn, dry_run=False):
    cur = conn.cursor()
    rows = cur.execute("""
        SELECT i.media_type, i.title, i.original_title, i.year, i.author, i.country,
               i.url, i.extra_json, l.status, l.rating, l.progress, l.started_at,
               l.finished_at, l.date_logged, l.review, l.raw_json, l.source
        FROM media_items i JOIN media_logs l ON l.media_item_id = i.id
    """).fetchall()

    counts = {}
    for (mtype, title, orig, year, author, country, url, extra_json,
         status, rating, progress, started, finished, dlog, review, raw_json, source) in rows:
        target = MEDIA_TARGETS.get(mtype)
        if not target:
            continue
        subdir, tags = target
        extra = {}
        try:
            extra = {k: v for k, v in __import__("json").loads(extra_json or "{}").items()}
        except Exception:
            pass

        date = dlog or finished or started or "2016-01-01"
        fm = {
            "title": title,
            "date": date,
            "tags": tags,
            "publish_external": False,
        }
        for key, val in (
            ("author", author), ("year", year), ("country", country),
            ("status", status), ("rating", rating), ("progress", progress),
            ("started", started), ("finished", finished), ("url", url),
            ("source", source),
        ):
            if val not in (None, ""):
                fm[key] = val

        lines = [f"# {title}", ""]
        meta = [(k.capitalize(), v) for k, v in fm.items()
                if k not in ("title", "date", "tags", "publish_external")]
        if meta:
            lines += [f"- **{k}:** {v}" for k, v in meta]
        if review:
            lines += ["", "## Review", "", review]

        rel = os.path.join(subdir, f"{sanitize_filename(title)}.md")
        write_note(rel, fm, "\n".join(lines), dry_run)
        counts[mtype] = counts.get(mtype, 0) + 1
    return counts


def export_links(conn, dry_run=False):
    cur = conn.cursor()
    rows = cur.execute(
        "SELECT label, url, category, is_public FROM links ORDER BY category, label"
    ).fetchall()
    by_cat = {}
    for label, url, cat, pub in rows:
        by_cat.setdefault(cat or "Other", []).append((label, url, pub))

    body_lines = ["# Links", ""]
    for cat in sorted(by_cat):
        body_lines.append(f"## {cat}")
        body_lines.append("")
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


def main():
    parser = argparse.ArgumentParser(description="Export ierp DB -> Digital Graveyard notes")
    parser.add_argument("--check", action="store_true", help="Dry run")
    args = parser.parse_args()

    if not os.path.exists(IERP_DB):
        print(f"ierp DB not found at {IERP_DB}")
        sys.exit(1)

    # Read-only connection
    conn = sqlite3.connect(f"file:{IERP_DB}?mode=ro", uri=True)
    try:
        counts = export_media(conn, args.check)
        n_links = export_links(conn, args.check)
    finally:
        conn.close()

    mode = " [dry-run]" if args.check else ""
    print(f"\nExported from ierp{mode}:")
    for mtype, n in sorted(counts.items()):
        print(f"  - {mtype:8}: {n:4d} notes")
    print(f"  - links  : {n_links} entries -> {LINKS_NOTE}")


if __name__ == "__main__":
    main()
