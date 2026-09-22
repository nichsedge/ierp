"""
Decision Journal domain service for iERP.
Calibrates long-term human judgment by recording context, choices, confidence,
and scheduled retrospective reviews.
Zero external dependencies (Python standard library only).
"""

from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional

from .db import db_session, get_db, init_db


def insert_decision(
    title: str,
    choice: str,
    context: Optional[str] = None,
    expected_outcome: Optional[str] = None,
    confidence: int = 7,
    review_date: Optional[str] = None,
    project_id: Optional[int] = None,
    db_path: Optional[Path] = None,
) -> int:
    """Records a major decision in the decision journal."""
    init_db(db_path)
    clamped_confidence = max(1, min(10, confidence))

    with db_session(db_path) as cursor:
        cursor.execute("""
        INSERT INTO decisions (
            title, context, choice, expected_outcome, confidence, review_date, project_id, status
        ) VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')
        """, (
            title.strip(),
            context.strip() if context else None,
            choice.strip(),
            expected_outcome.strip() if expected_outcome else None,
            clamped_confidence,
            review_date,
            project_id,
        ))
        return cursor.lastrowid


def review_decision(
    decision_id: int,
    actual_outcome: str,
    status: str = "reviewed",
    db_path: Optional[Path] = None,
) -> bool:
    """Conducts a retrospective review on a logged decision."""
    init_db(db_path)
    with db_session(db_path) as cursor:
        cursor.execute("""
        UPDATE decisions
        SET actual_outcome = ?, status = ?, updated_at = datetime('now', 'localtime')
        WHERE id = ?
        """, (actual_outcome.strip(), status, decision_id))
        return cursor.rowcount > 0


def get_decision(decision_id: int, db_path: Optional[Path] = None) -> Optional[Dict[str, Any]]:
    """Retrieves a single decision record by ID."""
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()
    row = cursor.execute("""
    SELECT d.id, d.title, d.context, d.choice, d.expected_outcome, d.confidence, 
           d.review_date, d.actual_outcome, d.status, d.project_id, d.created_at, d.updated_at,
           p.title as project_title, p.slug as project_slug
    FROM decisions d
    LEFT JOIN projects p ON p.id = d.project_id
    WHERE d.id = ?
    """, (decision_id,)).fetchone()
    conn.close()

    if not row:
        return None

    return {
        "id": row[0],
        "title": row[1],
        "context": row[2],
        "choice": row[3],
        "expected_outcome": row[4],
        "confidence": row[5],
        "review_date": row[6],
        "actual_outcome": row[7],
        "status": row[8],
        "project_id": row[9],
        "created_at": row[10],
        "updated_at": row[11],
        "project_title": row[12],
        "project_slug": row[13],
    }


def list_decisions(
    status: Optional[str] = None,
    project_id: Optional[int] = None,
    pending_review_only: bool = False,
    limit: int = 50,
    offset: int = 0,
    db_path: Optional[Path] = None,
) -> List[Dict[str, Any]]:
    """Lists decisions with optional filters."""
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()

    query = """
    SELECT d.id, d.title, d.context, d.choice, d.expected_outcome, d.confidence, 
           d.review_date, d.actual_outcome, d.status, d.project_id, d.created_at, d.updated_at,
           p.title as project_title, p.slug as project_slug
    FROM decisions d
    LEFT JOIN projects p ON p.id = d.project_id
    WHERE 1=1
    """
    params: List[Any] = []

    if status:
        query += " AND d.status = ?"
        params.append(status)
    if project_id is not None:
        query += " AND d.project_id = ?"
        params.append(project_id)
    if pending_review_only:
        query += " AND d.status = 'pending' AND d.review_date IS NOT NULL AND d.review_date <= date('now')"

    query += " ORDER BY CASE WHEN d.status = 'pending' THEN 0 ELSE 1 END, d.review_date ASC, d.created_at DESC LIMIT ? OFFSET ?"
    params.extend([limit, offset])

    rows = cursor.execute(query, params).fetchall()
    conn.close()

    return [
        {
            "id": r[0],
            "title": r[1],
            "context": r[2],
            "choice": r[3],
            "expected_outcome": r[4],
            "confidence": r[5],
            "review_date": r[6],
            "actual_outcome": r[7],
            "status": r[8],
            "project_id": r[9],
            "created_at": r[10],
            "updated_at": r[11],
            "project_title": r[12],
            "project_slug": r[13],
        }
        for r in rows
    ]


def delete_decision(decision_id: int, db_path: Optional[Path] = None) -> bool:
    """Deletes a decision record by ID."""
    init_db(db_path)
    with db_session(db_path) as cursor:
        cursor.execute("DELETE FROM decisions WHERE id = ?", (decision_id,))
        return cursor.rowcount > 0


def get_decision_alerts(window_days: int = 7, db_path: Optional[Path] = None) -> Dict[str, List[Dict[str, Any]]]:
    """
    Returns decisions requiring retrospective attention:
    - 'overdue': review_date <= date('now') and status == 'pending'
    - 'upcoming': review_date > date('now') and review_date <= date('now', f'+{window_days} days') and status == 'pending'
    """
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()

    query = """
    SELECT d.id, d.title, d.choice, d.expected_outcome, d.confidence, d.review_date, d.status,
           p.title as project_title,
           CAST(ROUND(julianday(d.review_date) - julianday(date('now'))) AS INTEGER) as days_until_review
    FROM decisions d
    LEFT JOIN projects p ON p.id = d.project_id
    WHERE d.status = 'pending' AND d.review_date IS NOT NULL
    ORDER BY d.review_date ASC
    """
    rows = cursor.execute(query).fetchall()
    conn.close()

    overdue = []
    upcoming = []

    for r in rows:
        did, title, choice, exp, conf, r_date, status, p_title, days_left = r
        item = {
            "id": did,
            "title": title,
            "choice": choice,
            "expected_outcome": exp,
            "confidence": conf,
            "review_date": r_date,
            "status": status,
            "project_title": p_title,
            "days_until_review": days_left if days_left is not None else 0,
        }
        if days_left is not None and days_left <= 0:
            item["days_overdue"] = abs(days_left)
            overdue.append(item)
        elif days_left is not None and days_left <= window_days:
            upcoming.append(item)

    return {"overdue": overdue, "upcoming": upcoming}

