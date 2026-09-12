"""
Digital Garden export domain service for iERP (Individual Enterprise Resource Planning).
Provides bidirectional knowledge synthesis exporting Projects, Decisions (PDRs),
Retrospectives, and Gadgets into Obsidian-compatible Markdown notes with catalog indexes.
Strictly zero external dependencies (pure Python 3.11+ standard library).
"""

import json
import os
import re
from datetime import datetime
from pathlib import Path
from typing import Any

from .config import DB_PATH
from .decisions import list_decisions
from .gadgets import export_garden_gadgets
from .projects import get_project_summary, list_projects
from .reviews import list_retrospectives


def resolve_garden_root(custom_path: Path | str | None = None) -> Path:
    """
    Resolves the root directory of the Digital Garden content.
    Checks custom path, environment variable GARDEN_CONTENT, and standard directory layouts.
    """
    if custom_path is not None:
        return Path(custom_path).expanduser().resolve()

    env_dir = os.environ.get("GARDEN_CONTENT")
    if env_dir:
        return Path(env_dir).expanduser().resolve()

    candidates = [
        Path.home() / "Projects" / "digital-graveyard" / "content",
        Path.home() / "Projects" / "digital-garden" / "content",
    ]
    for c in candidates:
        if c.is_dir():
            return c

    # Default fallback
    return Path.home() / "Projects" / "digital-graveyard" / "content"


def slugify(text: str) -> str:
    """Generates a URL and filesystem-friendly slug from text."""
    cleaned = re.sub(r"[^\w\s-]", "", str(text).lower()).strip()
    slug = re.sub(r"[\s_-]+", "-", cleaned)
    return slug or "untitled"


def format_yaml_scalar(val: Any) -> str:
    """Formats a scalar value into valid YAML matching digital garden standards."""
    if val is None:
        return "null"
    if isinstance(val, bool):
        return "true" if val else "false"
    if isinstance(val, (int, float)):
        return str(val)

    s = str(val).strip()
    if re.match(r"^\d{4}-\d{2}-\d{2}$", s):
        return s
    # Quote string and escape double quotes and backslashes
    escaped = s.replace("\\", "\\\\").replace('"', '\\"')
    return f'"{escaped}"'


def render_frontmatter(fm: dict[str, Any]) -> str:
    """Renders a python dictionary as a YAML frontmatter block."""
    lines = ["---"]
    primary_keys = ["title", "date", "tags", "publish_external", "status"]

    for k in primary_keys:
        if k in fm:
            v = fm[k]
            if k == "title":
                escaped = str(v).replace("\\", "\\\\").replace('"', '\\"')
                lines.append(f'title: "{escaped}"')
            elif k == "date":
                lines.append(f"date: {str(v)[:10]}")
            elif k == "tags":
                if isinstance(v, list):
                    lines.append(f"tags: [{', '.join(v)}]")
                else:
                    lines.append(f"tags: [{v}]")
            elif k == "publish_external":
                lines.append(f"publish_external: {'true' if v else 'false'}")
            elif k == "status":
                lines.append(f"status: {format_yaml_scalar(v)}")

    for k, v in fm.items():
        if k not in primary_keys:
            if isinstance(v, list):
                lines.append(f"{k}: [{', '.join(format_yaml_scalar(item) for item in v)}]")
            else:
                lines.append(f"{k}: {format_yaml_scalar(v)}")

    lines.append("---")
    return "\n".join(lines)


def write_garden_note(file_path: Path, frontmatter: dict[str, Any], body: str, dry_run: bool = False) -> None:
    """Writes a markdown note with rendered frontmatter to file_path."""
    if dry_run:
        return
    file_path.parent.mkdir(parents=True, exist_ok=True)
    content = render_frontmatter(frontmatter) + "\n\n" + body.strip() + "\n"
    file_path.write_text(content, encoding="utf-8")


# -----------------------------------------------------------------------------
# Domain Exporters: Projects, Decisions, Retrospectives
# -----------------------------------------------------------------------------

def export_garden_projects(
    garden_root: Path | str | None = None,
    dry_run: bool = False,
    db_path: Path | None = None,
) -> dict[str, Any]:
    """
    Exports strategic projects and initiatives to Knowledge/Projects/.
    Generates linked events timeline, decisions list, and index.md catalog.
    """
    root = resolve_garden_root(garden_root)
    target_dir = root / "Knowledge" / "Projects"
    if not dry_run:
        target_dir.mkdir(parents=True, exist_ok=True)

    projects = list_projects(db_path=db_path)
    written_notes = 0
    catalog_rows = []

    for p in projects:
        pid = p["id"]
        slug = p.get("slug") or slugify(p["title"])
        details = get_project_summary(pid, db_path=db_path)
        if not details:
            continue

        title = details["title"]
        status = details.get("status") or "active"
        priority = details.get("priority") or "medium"
        start_date = (details.get("start_date") or details.get("created_at") or datetime.now().strftime("%Y-%m-%d"))[:10]
        target_date = details.get("target_date") or "Unspecified"
        description = details.get("description") or "Strategic initiative container."

        note_file = target_dir / f"{slug}.md"

        fm = {
            "title": title,
            "date": start_date,
            "tags": ["project", "initiative", "ierp"],
            "publish_external": True,
            "status": "seedling",
            "project_status": status,
            "priority": priority,
            "target_date": target_date,
        }

        body_parts = [
            f"# {title}",
            "",
            f"- **Status:** `{status}`",
            f"- **Priority:** `{priority}`",
            f"- **Target Date:** `{target_date}`",
            f"- **Start Date:** `{start_date}`",
            "",
            "## Strategic Overview",
            description,
            "",
        ]

        # Linked Decisions
        decisions = details.get("decisions", [])
        if decisions:
            body_parts.append("## Strategic Decisions")
            for d in decisions:
                d_slug = slugify(d['title'])
                body_parts.append(
                    f"- [[Knowledge/Decisions/{d_slug}|{d['title']}]] — **Choice:** `{d['choice']}` "
                    f"(Confidence: {d.get('confidence', 7)}/10, Status: `{d.get('status', 'pending')}`)"
                )
            body_parts.append("")

        # Linked Events Timeline
        events = details.get("events", [])
        if events:
            body_parts.append("## Timeline & Milestones")
            for ev in events:
                ev_date = (ev.get("start_date") or ev.get("created_at") or "")[:10]
                place_str = f" ({ev['place']})" if ev.get("place") else ""
                body_parts.append(f"- `{ev_date}`: **{ev['title']}**{place_str}")
                if ev.get("notes"):
                    first_line = ev["notes"].strip().split("\n")[0]
                    body_parts.append(f"  - {first_line[:120]}")
            body_parts.append("")

        write_garden_note(note_file, fm, "\n".join(body_parts), dry_run=dry_run)
        written_notes += 1
        catalog_rows.append((title, slug, status, priority, target_date))

    # Generate Knowledge/Projects/index.md
    index_file = target_dir / "index.md"
    index_fm = {
        "title": "Projects",
        "date": datetime.now().strftime("%Y-%m-%d"),
        "tags": ["moc", "projects"],
        "publish_external": True,
    }
    index_body = [
        "# Projects & Strategic Initiatives",
        "",
        "Strategic bets and long-term milestones synchronized from the iERP engine.",
        "",
        "| Project | Status | Priority | Target Date |",
        "| :--- | :--- | :--- | :--- |",
    ]
    for title, slug, status, priority, target_date in sorted(catalog_rows, key=lambda x: x[0]):
        index_body.append(f"| [[{title}]] | `{status}` | `{priority}` | {target_date} |")

    index_body.append("")
    index_body.append("### Directory")
    index_body.append("")
    for title, slug, _, _, _ in sorted(catalog_rows, key=lambda x: x[0]):
        index_body.append(f"- [[{title}]]")

    write_garden_note(index_file, index_fm, "\n".join(index_body), dry_run=dry_run)

    return {
        "domain": "projects",
        "notes_written": written_notes,
        "target_dir": str(target_dir),
        "index_updated": True,
    }


def export_garden_decisions(
    garden_root: Path | str | None = None,
    dry_run: bool = False,
    db_path: Path | None = None,
) -> dict[str, Any]:
    """
    Exports Decision Journal entries to Knowledge/Decisions/ as Personal Decision Records (PDRs).
    Generates decision hypotheses, retrospective reviews, and index.md catalog.
    """
    root = resolve_garden_root(garden_root)
    target_dir = root / "Knowledge" / "Decisions"
    if not dry_run:
        target_dir.mkdir(parents=True, exist_ok=True)

    decisions = list_decisions(db_path=db_path)
    written_notes = 0
    catalog_rows = []

    for d in decisions:
        title = d["title"]
        slug = slugify(title)
        choice = d.get("choice") or "Unspecified"
        status = d.get("status") or "pending"
        confidence = d.get("confidence", 7)
        review_date = d.get("review_date") or "Unscheduled"
        date_str = (d.get("created_at") or datetime.now().strftime("%Y-%m-%d"))[:10]
        context = d.get("context") or "No context recorded."
        expected = d.get("expected_outcome") or "No hypothesis logged."
        actual = d.get("actual_outcome")
        proj_title = d.get("project_title")

        note_file = target_dir / f"{slug}.md"

        fm = {
            "title": title,
            "date": date_str,
            "tags": ["decision", "pdr", "judgment"],
            "publish_external": True,
            "status": "seedling",
            "decision_status": status,
            "confidence": confidence,
            "review_date": review_date,
        }

        body_parts = [
            f"# {title}",
            "",
            f"- **Status:** `{status}`",
            f"- **Chosen Alternative:** `{choice}`",
            f"- **Confidence Level:** `{confidence}/10`",
            f"- **Review Date:** `{review_date}`",
        ]
        if proj_title:
            p_slug = slugify(proj_title)
            body_parts.append(f"- **Linked Project:** [[Knowledge/Projects/{p_slug}|{proj_title}]]")

        body_parts.extend([
            "",
            "## Context & Framing",
            context,
            "",
            "## The Choice Made",
            f"> {choice}",
            "",
            "## Hypotheses & Expected Outcomes",
            expected,
            "",
            "## Retrospective Review & Calibration",
        ])

        if actual:
            body_parts.append(actual)
        else:
            body_parts.append(f"*Pending retrospective review scheduled for `{review_date}`.*")
        body_parts.append("")

        write_garden_note(note_file, fm, "\n".join(body_parts), dry_run=dry_run)
        written_notes += 1
        catalog_rows.append((title, slug, choice, confidence, status, review_date))

    # Generate Knowledge/Decisions/index.md
    index_file = target_dir / "index.md"
    index_fm = {
        "title": "Decision Journal",
        "date": datetime.now().strftime("%Y-%m-%d"),
        "tags": ["moc", "decisions", "pdr"],
        "publish_external": True,
    }
    index_body = [
        "# Decision Journal (Personal Decision Records)",
        "",
        "Logged bets, judgments, and retrospective calibration reviews from iERP.",
        "",
        "| Decision | Choice | Confidence | Status | Review Date |",
        "| :--- | :--- | :--- | :--- | :--- |",
    ]
    for title, slug, choice, confidence, status, review_date in sorted(catalog_rows, key=lambda x: x[0]):
        index_body.append(f"| [[{title}]] | `{choice}` | `{confidence}/10` | `{status}` | {review_date} |")

    index_body.append("")
    index_body.append("### Directory")
    index_body.append("")
    for title, slug, _, _, _, _ in sorted(catalog_rows, key=lambda x: x[0]):
        index_body.append(f"- [[{title}]]")

    write_garden_note(index_file, index_fm, "\n".join(index_body), dry_run=dry_run)

    return {
        "domain": "decisions",
        "notes_written": written_notes,
        "target_dir": str(target_dir),
        "index_updated": True,
    }


def export_garden_reviews(
    garden_root: Path | str | None = None,
    dry_run: bool = False,
    db_path: Path | None = None,
) -> dict[str, Any]:
    """
    Exports Sprint Retrospectives to Write/Retrospectives/.
    Generates wins, energy drains/burnout, lessons, and next focus summaries.
    """
    root = resolve_garden_root(garden_root)
    target_dir = root / "Write" / "Retrospectives"
    if not dry_run:
        target_dir.mkdir(parents=True, exist_ok=True)

    reviews = list_retrospectives(limit=200, db_path=db_path)
    written_notes = 0
    catalog_rows = []

    for r in reviews:
        p_start = r.get("period_start") or "Unknown"
        p_end = r.get("period_end") or "Unknown"
        p_type = r.get("period_type") or "weekly"
        rating = r.get("rating", 7)
        wins = r.get("wins") or "None recorded."
        drains = r.get("drains_burnout") or "None recorded."
        lessons = r.get("lessons") or "None recorded."
        focus = r.get("focus_next") or "None recorded."
        notes = r.get("notes")

        note_title = f"{p_type.capitalize()} Retrospective ({p_start} to {p_end})"
        file_slug = f"{p_start}_{p_end}_{p_type}"
        note_file = target_dir / f"{file_slug}.md"

        fm = {
            "title": note_title,
            "date": p_end,
            "tags": ["retrospective", "review", "lifeops"],
            "publish_external": True,
            "period_type": p_type,
            "rating": rating,
        }

        body_parts = [
            f"# {note_title}",
            "",
            f"- **Period:** `{p_start}` → `{p_end}`",
            f"- **Sprint Type:** `{p_type.capitalize()}`",
            f"- **Energy & Progress Rating:** `{rating}/10`",
            "",
            "## 🏆 Wins & Progress",
            wins,
            "",
            "## 🔋 Energy Drains & Burnout Signals",
            drains,
            "",
            "## 💡 Lessons Learned & Mental Models",
            lessons,
            "",
            "## 🎯 Next Sprint Focus",
            focus,
        ]

        if notes:
            body_parts.extend(["", "## Additional Notes", notes])
        body_parts.append("")

        write_garden_note(note_file, fm, "\n".join(body_parts), dry_run=dry_run)
        written_notes += 1
        catalog_rows.append((note_title, file_slug, p_start, p_end, p_type, rating, focus))

    # Generate Write/Retrospectives/index.md
    index_file = target_dir / "index.md"
    index_fm = {
        "title": "Retrospectives & Sprint Reviews",
        "date": datetime.now().strftime("%Y-%m-%d"),
        "tags": ["moc", "retrospective"],
        "publish_external": True,
    }
    index_body = [
        "# Sprint Retrospectives",
        "",
        "Periodic sprint retrospectives synthesized from the iERP Life Ops engine.",
        "",
        "| Sprint | Type | Rating | Next Sprint Focus |",
        "| :--- | :--- | :--- | :--- |",
    ]
    for title, slug, p_start, p_end, p_type, rating, focus in sorted(catalog_rows, key=lambda x: x[2], reverse=True):
        clean_focus = focus.strip().split("\n")[0][:60]
        index_body.append(f"| [[{slug}|{p_start} to {p_end}]] | `{p_type}` | `{rating}/10` | {clean_focus} |")

    index_body.append("")
    index_body.append("### Directory")
    index_body.append("")
    for title, slug, _, _, _, _, _ in sorted(catalog_rows, key=lambda x: x[2], reverse=True):
        index_body.append(f"- [[{slug}|{title}]]")

    write_garden_note(index_file, index_fm, "\n".join(index_body), dry_run=dry_run)

    return {
        "domain": "reviews",
        "notes_written": written_notes,
        "target_dir": str(target_dir),
        "index_updated": True,
    }


def export_garden_all(
    garden_root: Path | str | None = None,
    targets: list[str] | None = None,
    dry_run: bool = False,
    db_path: Path | None = None,
) -> dict[str, Any]:
    """
    Unified export coordinator across all iERP domains into the Digital Garden.
    Targets can include: 'all', 'projects', 'decisions', 'reviews', 'gadgets'.
    """
    root = resolve_garden_root(garden_root)
    active_targets = set(targets or ["all"])
    run_all = "all" in active_targets

    results: dict[str, Any] = {
        "garden_root": str(root),
        "dry_run": dry_run,
        "domains": {},
    }

    if run_all or "projects" in active_targets:
        results["domains"]["projects"] = export_garden_projects(
            garden_root=root, dry_run=dry_run, db_path=db_path
        )

    if run_all or "decisions" in active_targets:
        results["domains"]["decisions"] = export_garden_decisions(
            garden_root=root, dry_run=dry_run, db_path=db_path
        )

    if run_all or "reviews" in active_targets or "retrospectives" in active_targets:
        results["domains"]["reviews"] = export_garden_reviews(
            garden_root=root, dry_run=dry_run, db_path=db_path
        )

    if run_all or "gadgets" in active_targets:
        gadget_dir = root / "Knowledge" / "Entities" / "Gadget"
        results["domains"]["gadgets"] = export_garden_gadgets(
            garden_dir=gadget_dir, dry_run=dry_run, db_path=db_path
        )

    return results
