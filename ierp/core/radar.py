"""
Human Capital & Reconnection Radar domain service for iERP.
Manages Dunbar relationship tiers and detects overdue relationship touchpoints
by cross-referencing contact records with journal event timelines.
Zero external dependencies (Python standard library only).
"""

from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional

from .db import db_session, get_db, init_db

# Default cadences (in days) per Dunbar tier
DEFAULT_CADENCE_BY_TIER = {
    1: 14,   # Tier 1: Inner Circle (family, closest friends) -> every 2 weeks
    2: 60,   # Tier 2: Core Network (collaborators, active mentors) -> every 2 months
    3: 180,  # Tier 3: Broad Network (acquaintances, colleagues) -> every 6 months
}


def update_contact_cadence(
    contact_id: int,
    tier: int,
    cadence_days: Optional[int] = None,
    db_path: Optional[Path] = None,
) -> bool:
    """Updates a contact's Dunbar tier (1, 2, or 3) and touch cadence in days."""
    init_db(db_path)
    clean_tier = max(1, min(3, tier))
    days = cadence_days if cadence_days and cadence_days > 0 else DEFAULT_CADENCE_BY_TIER.get(clean_tier, 60)

    with db_session(db_path) as cursor:
        cursor.execute("""
        UPDATE contacts
        SET tier = ?, cadence_days = ?
        WHERE id = ?
        """, (clean_tier, days, contact_id))
        return cursor.rowcount > 0


def compute_radar(
    tier: Optional[int] = None,
    overdue_only: bool = False,
    limit: int = 50,
    offset: int = 0,
    db_path: Optional[Path] = None,
) -> List[Dict[str, Any]]:
    """
    Computes days since last touch for contacts by joining events and event_contacts.
    Identifies relationships requiring proactive reachout.
    """
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()

    query = """
    SELECT c.id, c.name, c.org, c.email, c.phone, c.tier, c.cadence_days,
           MAX(e.start_date) as last_seen_date,
           COUNT(e.id) as total_interactions
    FROM contacts c
    LEFT JOIN event_contacts ec ON ec.contact_id = c.id
    LEFT JOIN events e ON e.id = ec.event_id
    WHERE 1=1
    """
    params: List[Any] = []

    if tier is not None:
        query += " AND c.tier = ?"
        params.append(tier)

    query += " GROUP BY c.id"

    rows = cursor.execute(query, params).fetchall()
    conn.close()

    now = datetime.now()
    results = []

    for r in rows:
        cid, name, org, email, phone, c_tier, cadence, last_seen, total_interactions = r
        c_tier = c_tier or 3
        cadence = cadence or DEFAULT_CADENCE_BY_TIER.get(c_tier, 180)

        if last_seen:
            try:
                # Handle YYYY-MM-DD or YYYY-MM-DD HH:MM:SS
                dt_str = last_seen[:10]
                last_dt = datetime.strptime(dt_str, "%Y-%m-%d")
                days_since = (now - last_dt).days
            except ValueError:
                days_since = 999
        else:
            days_since = 999  # Never met in logged events

        is_overdue = days_since > cadence
        days_overdue = max(0, days_since - cadence)

        if overdue_only and not is_overdue:
            continue

        results.append({
            "id": cid,
            "name": name,
            "org": org,
            "email": email,
            "phone": phone,
            "tier": c_tier,
            "cadence_days": cadence,
            "last_seen_date": last_seen,
            "days_since_last_touch": days_since,
            "is_overdue": is_overdue,
            "days_overdue": days_overdue,
            "total_interactions": total_interactions,
        })

    # Sort: Tier 1 first, then highest days_overdue, then highest days_since
    results.sort(key=lambda x: (x["tier"], -x["days_overdue"], -x["days_since_last_touch"]))
    return results[offset : offset + limit]


def get_radar_summary(db_path: Optional[Path] = None) -> Dict[str, Any]:
    """Provides high-level health overview of network cadences."""
    all_radar = compute_radar(overdue_only=False, limit=1000, db_path=db_path)
    tier_counts = {1: 0, 2: 0, 3: 0}
    tier_overdue = {1: 0, 2: 0, 3: 0}

    for item in all_radar:
        t = item["tier"]
        tier_counts[t] = tier_counts.get(t, 0) + 1
        if item["is_overdue"]:
            tier_overdue[t] = tier_overdue.get(t, 0) + 1

    return {
        "total_contacts": len(all_radar),
        "total_overdue": sum(tier_overdue.values()),
        "tier_counts": tier_counts,
        "tier_overdue": tier_overdue,
    }
