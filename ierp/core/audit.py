"""
Life Audit & Pulse Engine for iERP.
Scans the entire personal ERP database to identify missing snapshots,
overdue reviews, neglected relationships, expiring commitments, and gaps in life tracking.
Zero external dependencies (Python standard library only).
"""

from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional

from .db import get_db, init_db
from .decisions import list_decisions
from .finance import compute_runway, list_commitments, list_snapshots
from .lifeops import get_maintenance_summary, list_maintenance
from .projects import list_projects
from .radar import compute_radar, get_radar_summary
from .reviews import list_retrospectives


def generate_life_audit(db_path: Optional[Path] = None) -> Dict[str, Any]:
    """
    Performs a comprehensive audit of all life domains in iERP.
    Returns categorized findings, gaps, and actionable CLI commands.
    """
    init_db(db_path)
    now = datetime.now()
    today_str = now.strftime("%Y-%m-%d")
    findings: List[Dict[str, Any]] = []

    # 1. Sovereign Treasury & Runway Audit
    snapshots = list_snapshots(limit=1, db_path=db_path)
    if not snapshots:
        findings.append({
            "domain": "Treasury",
            "severity": "action_needed",
            "title": "No Net Worth Snapshot",
            "description": "You haven't recorded a balance sheet snapshot. iERP cannot calculate your liquid reserves or wealth trajectory.",
            "command": "uv run ierp insert-snapshot --liquid <amount> --investments <amount>",
        })
    else:
        latest_snap = snapshots[0]
        try:
            snap_dt = datetime.strptime(latest_snap["snapshot_date"][:10], "%Y-%m-%d")
            days_since_snap = (now - snap_dt).days
        except ValueError:
            days_since_snap = 999

        if days_since_snap > 30:
            findings.append({
                "domain": "Treasury",
                "severity": "warning",
                "title": "Outdated Net Worth Snapshot",
                "description": f"Latest snapshot is {days_since_snap} days old ({latest_snap['snapshot_date']}). Monthly balance checkup recommended.",
                "command": "uv run ierp insert-snapshot --liquid <amount>",
            })

    commitments = list_commitments(status="active", db_path=db_path)
    if not commitments:
        findings.append({
            "domain": "Treasury",
            "severity": "action_needed",
            "title": "No Recurring Commitments Logged",
            "description": "Zero recurring expenses logged. Fixed burn rate is Rp0, so runway cannot be accurately calculated.",
            "command": "uv run ierp insert-commitment --name <Rent/SaaS> --amount <amount> --frequency monthly",
        })
    else:
        runway = compute_runway(db_path=db_path)
        if runway["runway_months"] < 6 and not runway["is_infinite"]:
            findings.append({
                "domain": "Treasury",
                "severity": "warning",
                "title": "Lean Freedom Runway",
                "description": f"Current runway is {runway['runway_months']} months ({runway['status_label']}). Consider cutting recurring burn.",
                "command": "uv run ierp runway",
            })

    # 2. Decision Journal Audit
    due_decisions = list_decisions(status="pending", pending_review_only=True, db_path=db_path)
    if due_decisions:
        titles = [f"#{d['id']} {d['title']}" for d in due_decisions[:3]]
        findings.append({
            "domain": "Decisions",
            "severity": "action_needed",
            "title": f"{len(due_decisions)} Decision(s) Due for Review",
            "description": f"Decisions reached scheduled review date: {', '.join(titles)}. Calibrate your hypothesis with actual outcomes.",
            "command": f"uv run ierp review-decision {due_decisions[0]['id']} --outcome <results>",
        })

    all_decisions = list_decisions(limit=1, db_path=db_path)
    if not all_decisions:
        findings.append({
            "domain": "Decisions",
            "severity": "tip",
            "title": "Decision Journal Unused",
            "description": "Logging 1–2 key bets per quarter prevents revisionist memory and sharpens your decision calibration over years.",
            "command": "uv run ierp insert-decision --title <Bet> --choice <Chosen Option> --confidence <1-10> --review-date <YYYY-MM-DD>",
        })

    # 3. Human Capital & Reconnection Radar
    t1_overdue = compute_radar(tier=1, overdue_only=True, limit=5, db_path=db_path)
    if t1_overdue:
        names = [f"{c['name']} ({c['days_overdue']}d late)" for c in t1_overdue[:3]]
        findings.append({
            "domain": "Radar",
            "severity": "warning",
            "title": f"{len(t1_overdue)} Inner Circle (Tier 1) Contact(s) Overdue",
            "description": f"Close relationships needing touchpoints: {', '.join(names)}.",
            "command": "uv run ierp radar --tier 1 --overdue-only",
        })

    radar_summary = get_radar_summary(db_path=db_path)
    if radar_summary["tier_counts"].get(1, 0) == 0 and radar_summary["total_contacts"] > 0:
        findings.append({
            "domain": "Radar",
            "severity": "tip",
            "title": "No Contacts Assigned to Tier 1",
            "description": "All contacts are currently on default tiers. Designate your inner circle (family, closest friends) to activate radar alerts.",
            "command": "uv run ierp set-tier --contact-id <id> --tier 1 --cadence 14",
        })

    # 4. Life Ops & Maintenance Audit
    maint_summary = get_maintenance_summary(db_path=db_path)
    if maint_summary["total_overdue"] > 0:
        overdue_items = [f"#{m['id']} {m['name']} (due {m['due_date']})" for m in maint_summary["overdue_items"][:3]]
        findings.append({
            "domain": "Life Ops",
            "severity": "action_needed",
            "title": f"{maint_summary['total_overdue']} Maintenance / Expiration Item(s) Overdue",
            "description": f"Overdue items: {', '.join(overdue_items)}.",
            "command": f"uv run ierp complete-maintenance {maint_summary['overdue_items'][0]['id']}",
        })
    elif maint_summary["total_pending"] == 0:
        findings.append({
            "domain": "Life Ops",
            "severity": "tip",
            "title": "No Preventive Maintenance Scheduled",
            "description": "Keep life frictionless by scheduling vehicle oil changes, AC cleaning, and passport/domain expirations.",
            "command": "uv run ierp insert-maintenance --name <Task/Doc> --due-date <YYYY-MM-DD> --interval <days>",
        })

    # 5. Sprint Retrospectives
    retros = list_retrospectives(limit=1, db_path=db_path)
    if not retros:
        findings.append({
            "domain": "Retrospectives",
            "severity": "action_needed",
            "title": "No Retrospectives Logged",
            "description": "Double-loop learning requires weekly or monthly retrospectives to identify what drained your energy and what to focus on next.",
            "command": "uv run ierp insert-review --type weekly --start <YYYY-MM-DD> --end <YYYY-MM-DD> --wins <...> --focus <...>",
        })
    else:
        latest_retro = retros[0]
        try:
            end_dt = datetime.strptime(latest_retro["period_end"][:10], "%Y-%m-%d")
            days_since_retro = (now - end_dt).days
        except ValueError:
            days_since_retro = 999

        if days_since_retro > 14:
            findings.append({
                "domain": "Retrospectives",
                "severity": "warning",
                "title": f"Retrospective Due ({days_since_retro} days since last review)",
                "description": f"Last review covered up to {latest_retro['period_end']}. Time to reflect on the recent sprint.",
                "command": "uv run ierp insert-review --type weekly --start <YYYY-MM-DD> --end <YYYY-MM-DD>",
            })

    # 6. Strategic Projects & Unlinked Events
    projects = list_projects(status="active", db_path=db_path)
    if not projects:
        findings.append({
            "domain": "Projects",
            "severity": "tip",
            "title": "No Active Strategic Initiatives",
            "description": "Group your current quarterly bets and goals into projects so daily timeline events contribute to clear outcomes.",
            "command": "uv run ierp insert-project --title <Project Name> --priority high",
        })

    # Count recent events without project_id
    conn = get_db(db_path)
    cur = conn.cursor()
    unlinked_events = cur.execute("""
    SELECT COUNT(*) FROM events 
    WHERE project_id IS NULL AND start_date >= date('now', '-30 days')
    """).fetchone()[0]
    conn.close()

    if unlinked_events > 5:
        findings.append({
            "domain": "Projects",
            "severity": "tip",
            "title": f"{unlinked_events} Recent Events Unlinked to Projects",
            "description": "Linking daily events to projects clarifies which life bets consume the bulk of your time.",
            "command": "uv run ierp list --limit 10",
        })

    return {
        "generated_at": today_str,
        "total_findings": len(findings),
        "actions_needed": sum(1 for f in findings if f["severity"] == "action_needed"),
        "warnings": sum(1 for f in findings if f["severity"] == "warning"),
        "tips": sum(1 for f in findings if f["severity"] == "tip"),
        "findings": findings,
    }
