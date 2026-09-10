"""
Life Ops & Preventive Maintenance domain service for iERP.
Tracks recurring servicing cycles (vehicles, hardware, health checks) and
critical document expirations (passports, licenses, domains, warranties).
Zero external dependencies (Python standard library only).
"""

from datetime import datetime, timedelta
from pathlib import Path
from typing import Any, Dict, List, Optional

from .db import db_session, get_db, init_db


def insert_maintenance(
    name: str,
    due_date: str,
    category: str = "general",
    interval_days: Optional[int] = None,
    cost: float = 0.0,
    notes: Optional[str] = None,
    gadget_id: Optional[int] = None,
    db_path: Optional[Path] = None,
) -> int:
    """Schedules a maintenance task or critical document expiration."""
    init_db(db_path)
    with db_session(db_path) as cursor:
        cursor.execute("""
        INSERT INTO maintenance_items (
            name, category, due_date, interval_days, cost, notes, gadget_id, status
        ) VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')
        """, (
            name.strip(),
            category.strip().lower(),
            due_date.strip(),
            interval_days,
            float(cost),
            notes,
            gadget_id,
        ))
        return cursor.lastrowid


def complete_maintenance(
    item_id: int,
    cost: Optional[float] = None,
    completion_date: Optional[str] = None,
    db_path: Optional[Path] = None,
) -> Dict[str, Any]:
    """
    Marks a maintenance task completed. If interval_days is configured,
    automatically creates the next scheduled occurrence.
    """
    init_db(db_path)
    now_str = completion_date or datetime.now().strftime("%Y-%m-%d")
    item = get_maintenance(item_id, db_path)
    if not item:
        return {"success": False, "error": "Item not found"}

    next_id = None
    next_due = None

    with db_session(db_path) as cursor:
        update_cost = cost if cost is not None else item["cost"]
        cursor.execute("""
        UPDATE maintenance_items
        SET status = 'completed', cost = ?, updated_at = datetime('now', 'localtime')
        WHERE id = ?
        """, (update_cost, item_id))

        # Auto-reschedule if recurring
        interval = item["interval_days"]
        if interval and interval > 0:
            try:
                base_dt = datetime.strptime(now_str[:10], "%Y-%m-%d")
                next_dt = base_dt + timedelta(days=interval)
                next_due = next_dt.strftime("%Y-%m-%d")

                cursor.execute("""
                INSERT INTO maintenance_items (
                    name, category, due_date, interval_days, cost, notes, gadget_id, status
                ) VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')
                """, (
                    item["name"],
                    item["category"],
                    next_due,
                    interval,
                    item["cost"],
                    item["notes"],
                    item["gadget_id"],
                ))
                next_id = cursor.lastrowid
            except ValueError:
                pass

    return {
        "success": True,
        "completed_id": item_id,
        "next_item_id": next_id,
        "next_due_date": next_due,
    }


def get_maintenance(item_id: int, db_path: Optional[Path] = None) -> Optional[Dict[str, Any]]:
    """Retrieves a single maintenance task by ID."""
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()
    row = cursor.execute("""
    SELECT m.id, m.name, m.category, m.due_date, m.interval_days, m.status, 
           m.cost, m.notes, m.gadget_id, m.created_at, m.updated_at,
           g.name as gadget_name
    FROM maintenance_items m
    LEFT JOIN gadgets g ON g.id = m.gadget_id
    WHERE m.id = ?
    """, (item_id,)).fetchone()
    conn.close()

    if not row:
        return None

    today_str = datetime.now().strftime("%Y-%m-%d")
    is_overdue = row[5] == "pending" and row[3] < today_str

    return {
        "id": row[0],
        "name": row[1],
        "category": row[2],
        "due_date": row[3],
        "interval_days": row[4],
        "status": row[5],
        "cost": row[6],
        "notes": row[7],
        "gadget_id": row[8],
        "created_at": row[9],
        "updated_at": row[10],
        "gadget_name": row[11],
        "is_overdue": is_overdue,
    }


def list_maintenance(
    status: Optional[str] = "pending",
    category: Optional[str] = None,
    due_within_days: Optional[int] = None,
    limit: int = 50,
    offset: int = 0,
    db_path: Optional[Path] = None,
) -> List[Dict[str, Any]]:
    """Lists maintenance tasks with optional filtering."""
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()

    query = """
    SELECT m.id, m.name, m.category, m.due_date, m.interval_days, m.status, 
           m.cost, m.notes, m.gadget_id, m.created_at, m.updated_at,
           g.name as gadget_name
    FROM maintenance_items m
    LEFT JOIN gadgets g ON g.id = m.gadget_id
    WHERE 1=1
    """
    params: List[Any] = []

    if status:
        query += " AND m.status = ?"
        params.append(status)
    if category:
        query += " AND m.category = ?"
        params.append(category.lower())
    if due_within_days is not None:
        target_dt = datetime.now() + timedelta(days=due_within_days)
        target_str = target_dt.strftime("%Y-%m-%d")
        query += " AND m.due_date <= ?"
        params.append(target_str)

    query += " ORDER BY m.due_date ASC, m.id ASC LIMIT ? OFFSET ?"
    params.extend([limit, offset])

    rows = cursor.execute(query, params).fetchall()
    conn.close()

    today_str = datetime.now().strftime("%Y-%m-%d")
    results = []
    for r in rows:
        due = r[3]
        is_overdue = r[5] == "pending" and due < today_str
        results.append({
            "id": r[0],
            "name": r[1],
            "category": r[2],
            "due_date": due,
            "interval_days": r[4],
            "status": r[5],
            "cost": r[6],
            "notes": r[7],
            "gadget_id": r[8],
            "created_at": r[9],
            "updated_at": r[10],
            "gadget_name": r[11],
            "is_overdue": is_overdue,
        })
    return results


def delete_maintenance(item_id: int, db_path: Optional[Path] = None) -> bool:
    """Deletes a maintenance record by ID."""
    init_db(db_path)
    with db_session(db_path) as cursor:
        cursor.execute("DELETE FROM maintenance_items WHERE id = ?", (item_id,))
        return cursor.rowcount > 0


def get_maintenance_summary(db_path: Optional[Path] = None) -> Dict[str, Any]:
    """Returns overview of pending, overdue, and upcoming maintenance tasks."""
    pending = list_maintenance(status="pending", limit=500, db_path=db_path)
    today_str = datetime.now().strftime("%Y-%m-%d")

    overdue = [item for item in pending if item["due_date"] < today_str]
    due_next_30d = [
        item for item in pending
        if today_str <= item["due_date"] <= (datetime.now() + timedelta(days=30)).strftime("%Y-%m-%d")
    ]

    return {
        "total_pending": len(pending),
        "total_overdue": len(overdue),
        "due_next_30d": len(due_next_30d),
        "overdue_items": overdue[:5],
    }
