"""
Sprint Retrospectives & Double-Loop Learning domain service for iERP.
Synthesizes weekly, monthly, and quarterly reflection cycles to evaluate wins,
energy drains, operational lessons, and upcoming strategic focus.
Zero external dependencies (Python standard library only).
"""

from pathlib import Path
from typing import Any, Dict, List, Optional

from .db import db_session, get_db, init_db


def insert_retrospective(
    period_start: str,
    period_end: str,
    period_type: str = "monthly",
    wins: Optional[str] = None,
    drains_burnout: Optional[str] = None,
    lessons: Optional[str] = None,
    focus_next: Optional[str] = None,
    rating: int = 7,
    notes: Optional[str] = None,
    db_path: Optional[Path] = None,
) -> int:
    """Inserts a retrospective synthesis log."""
    init_db(db_path)
    clamped_rating = max(1, min(10, rating))

    with db_session(db_path) as cursor:
        cursor.execute("""
        INSERT INTO retrospectives (
            period_start, period_end, period_type, wins, drains_burnout, lessons, focus_next, rating, notes
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        """, (
            period_start.strip(),
            period_end.strip(),
            period_type.strip().lower(),
            wins.strip() if wins else None,
            drains_burnout.strip() if drains_burnout else None,
            lessons.strip() if lessons else None,
            focus_next.strip() if focus_next else None,
            clamped_rating,
            notes,
        ))
        return cursor.lastrowid


def list_retrospectives(
    period_type: Optional[str] = None,
    limit: int = 20,
    offset: int = 0,
    db_path: Optional[Path] = None,
) -> List[Dict[str, Any]]:
    """Lists retrospectives ordered by period start date descending."""
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()

    query = """
    SELECT id, period_start, period_end, period_type, wins, drains_burnout, 
           lessons, focus_next, rating, notes, created_at, updated_at
    FROM retrospectives
    WHERE 1=1
    """
    params: List[Any] = []

    if period_type:
        query += " AND period_type = ?"
        params.append(period_type.lower())

    query += " ORDER BY period_start DESC, id DESC LIMIT ? OFFSET ?"
    params.extend([limit, offset])

    rows = cursor.execute(query, params).fetchall()
    conn.close()

    return [
        {
            "id": r[0],
            "period_start": r[1],
            "period_end": r[2],
            "period_type": r[3],
            "wins": r[4],
            "drains_burnout": r[5],
            "lessons": r[6],
            "focus_next": r[7],
            "rating": r[8],
            "notes": r[9],
            "created_at": r[10],
            "updated_at": r[11],
        }
        for r in rows
    ]


def get_retrospective(retro_id: int, db_path: Optional[Path] = None) -> Optional[Dict[str, Any]]:
    """Retrieves a single retrospective by ID."""
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()
    row = cursor.execute("""
    SELECT id, period_start, period_end, period_type, wins, drains_burnout, 
           lessons, focus_next, rating, notes, created_at, updated_at
    FROM retrospectives WHERE id = ?
    """, (retro_id,)).fetchone()
    conn.close()

    if not row:
        return None

    return {
        "id": row[0],
        "period_start": row[1],
        "period_end": row[2],
        "period_type": row[3],
        "wins": row[4],
        "drains_burnout": row[5],
        "lessons": row[6],
        "focus_next": row[7],
        "rating": row[8],
        "notes": row[9],
        "created_at": row[10],
        "updated_at": row[11],
    }


def delete_retrospective(retro_id: int, db_path: Optional[Path] = None) -> bool:
    """Deletes a retrospective entry by ID."""
    init_db(db_path)
    with db_session(db_path) as cursor:
        cursor.execute("DELETE FROM retrospectives WHERE id = ?", (retro_id,))
        return cursor.rowcount > 0
