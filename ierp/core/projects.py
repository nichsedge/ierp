"""
Projects & Strategic Initiatives domain service for iERP.
Provides CRUD operations, progress tracking, and event linkage for key life bets.
Zero external dependencies (Python standard library only).
"""

from datetime import datetime
from pathlib import Path
import re
from typing import Any, Dict, List, Optional

from .db import db_session, get_db, init_db


def generate_project_slug(title: str) -> str:
    """Generates a URL- and filesystem-safe slug from a project title."""
    s = title.lower().strip()
    s = re.sub(r"[^\w\s-]", "", s)
    s = re.sub(r"[\s_-]+", "-", s).strip("-")
    return s or f"project-{int(datetime.now().timestamp())}"


def insert_project(
    title: str,
    slug: Optional[str] = None,
    description: Optional[str] = None,
    status: str = "active",
    priority: str = "medium",
    start_date: Optional[str] = None,
    target_date: Optional[str] = None,
    db_path: Optional[Path] = None,
) -> int:
    """Inserts a new project / strategic initiative and returns its database ID."""
    init_db(db_path)
    clean_slug = slug or generate_project_slug(title)

    # Ensure unique slug
    conn = get_db(db_path)
    cursor = conn.cursor()
    existing = cursor.execute("SELECT id FROM projects WHERE slug = ?", (clean_slug,)).fetchone()
    if existing:
        clean_slug = f"{clean_slug}-{int(datetime.now().timestamp())}"

    cursor.execute("""
    INSERT INTO projects (slug, title, description, status, priority, start_date, target_date)
    VALUES (?, ?, ?, ?, ?, ?, ?)
    """, (clean_slug, title.strip(), description, status, priority, start_date, target_date))
    pid = cursor.lastrowid
    conn.commit()
    conn.close()
    return pid


def update_project(
    project_id: int,
    title: Optional[str] = None,
    description: Optional[str] = None,
    status: Optional[str] = None,
    priority: Optional[str] = None,
    start_date: Optional[str] = None,
    target_date: Optional[str] = None,
    db_path: Optional[Path] = None,
) -> bool:
    """Updates an existing project record."""
    init_db(db_path)
    fields = []
    values = []

    if title is not None:
        fields.append("title = ?")
        values.append(title.strip())
    if description is not None:
        fields.append("description = ?")
        values.append(description)
    if status is not None:
        fields.append("status = ?")
        values.append(status)
    if priority is not None:
        fields.append("priority = ?")
        values.append(priority)
    if start_date is not None:
        fields.append("start_date = ?")
        values.append(start_date)
    if target_date is not None:
        fields.append("target_date = ?")
        values.append(target_date)

    if not fields:
        return False

    fields.append("updated_at = datetime('now', 'localtime')")
    values.append(project_id)

    with db_session(db_path) as cursor:
        cursor.execute(f"UPDATE projects SET {', '.join(fields)} WHERE id = ?", values)
        return cursor.rowcount > 0


def get_project(identifier: Any, db_path: Optional[Path] = None) -> Optional[Dict[str, Any]]:
    """Retrieves a single project by ID (int) or slug (str)."""
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()

    if isinstance(identifier, int) or (isinstance(identifier, str) and identifier.isdigit()):
        row = cursor.execute("""
        SELECT id, slug, title, description, status, priority, start_date, target_date, created_at, updated_at
        FROM projects WHERE id = ?
        """, (int(identifier),)).fetchone()
    else:
        row = cursor.execute("""
        SELECT id, slug, title, description, status, priority, start_date, target_date, created_at, updated_at
        FROM projects WHERE slug = ?
        """, (str(identifier),)).fetchone()

    conn.close()
    if not row:
        return None

    return {
        "id": row[0],
        "slug": row[1],
        "title": row[2],
        "description": row[3],
        "status": row[4],
        "priority": row[5],
        "start_date": row[6],
        "target_date": row[7],
        "created_at": row[8],
        "updated_at": row[9],
    }


def list_projects(
    status: Optional[str] = None,
    priority: Optional[str] = None,
    limit: int = 50,
    offset: int = 0,
    db_path: Optional[Path] = None,
) -> List[Dict[str, Any]]:
    """Lists projects with optional status and priority filtering."""
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()

    query = """
    SELECT p.id, p.slug, p.title, p.description, p.status, p.priority, 
           p.start_date, p.target_date, p.created_at, p.updated_at,
           COUNT(DISTINCT e.id) as event_count,
           COUNT(DISTINCT d.id) as decision_count
    FROM projects p
    LEFT JOIN events e ON e.project_id = p.id
    LEFT JOIN decisions d ON d.project_id = p.id
    WHERE 1=1
    """
    params: List[Any] = []

    if status:
        query += " AND p.status = ?"
        params.append(status)
    if priority:
        query += " AND p.priority = ?"
        params.append(priority)

    query += " GROUP BY p.id ORDER BY p.updated_at DESC LIMIT ? OFFSET ?"
    params.extend([limit, offset])

    rows = cursor.execute(query, params).fetchall()
    conn.close()

    return [
        {
            "id": r[0],
            "slug": r[1],
            "title": r[2],
            "description": r[3],
            "status": r[4],
            "priority": r[5],
            "start_date": r[6],
            "target_date": r[7],
            "created_at": r[8],
            "updated_at": r[9],
            "event_count": r[10],
            "decision_count": r[11],
        }
        for r in rows
    ]


def delete_project(project_id: int, db_path: Optional[Path] = None) -> bool:
    """Deletes a project record by ID and unlinks its events."""
    init_db(db_path)
    with db_session(db_path) as cursor:
        cursor.execute("UPDATE events SET project_id = NULL WHERE project_id = ?", (project_id,))
        cursor.execute("UPDATE decisions SET project_id = NULL WHERE project_id = ?", (project_id,))
        cursor.execute("DELETE FROM projects WHERE id = ?", (project_id,))
        return cursor.rowcount > 0


def get_project_summary(project_id: int, db_path: Optional[Path] = None) -> Optional[Dict[str, Any]]:
    """Fetches full project details including linked events and decisions."""
    proj = get_project(project_id, db_path)
    if not proj:
        return None

    conn = get_db(db_path)
    cursor = conn.cursor()

    events = cursor.execute("""
    SELECT id, title, place, start_date, tags FROM events
    WHERE project_id = ? ORDER BY start_date DESC LIMIT 25
    """, (proj["id"],)).fetchall()

    decisions = cursor.execute("""
    SELECT id, title, choice, confidence, review_date, status FROM decisions
    WHERE project_id = ? ORDER BY review_date ASC LIMIT 25
    """, (proj["id"],)).fetchall()

    conn.close()

    proj["events"] = [
        {"id": e[0], "title": e[1], "place": e[2], "start_date": e[3], "tags": e[4]}
        for e in events
    ]
    proj["decisions"] = [
        {"id": d[0], "title": d[1], "choice": d[2], "confidence": d[3], "review_date": d[4], "status": d[5]}
        for d in decisions
    ]
    return proj
