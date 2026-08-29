"""
Vendors & Preferred Sellers domain service for iERP (Individual Enterprise Resource Planning).
Provides commercial vendor management, favorite toggling, categorization, and search.
"""

import sqlite3
from contextlib import closing
from pathlib import Path
from typing import Any

from .db import get_db, init_db


def insert_vendor(
    name: str,
    category: str | None = None,
    location: str | None = None,
    phone: str | None = None,
    email: str | None = None,
    url: str | None = None,
    notes: str | None = None,
    favorite: bool = False,
    source: str = "manual",
    db_path: Path | None = None,
) -> int:
    """Inserts a structured vendor record directly into SQLite. Returns vendors.id."""
    init_db(db_path)
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        cursor.execute(
            """
            INSERT INTO vendors (name, category, location, phone, email, url, notes, favorite, source)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (name, category, location, phone, email, url, notes, 1 if favorite else 0, source),
        )
        conn.commit()
        return int(cursor.lastrowid or 0)


def list_vendors(
    category_filter: str | None = None,
    favorite_only: bool = False,
    q: str | None = None,
    limit: int = 50,
    offset: int = 0,
    sort_col: str = "v.favorite",
    sort_dir: str = "DESC",
    db_path: Path | None = None,
) -> tuple[list[dict[str, Any]], int, list[str]]:
    """
    Lists vendors with optional category/favorite filtering, search, and pagination.
    Returns (vendors_list, total_count, available_categories).
    """
    where_clauses = ["1=1"]
    params: list[Any] = []

    if q:
        where_clauses.append(
            "(v.name LIKE ? OR v.category LIKE ? OR v.location LIKE ? OR v.notes LIKE ? OR v.phone LIKE ? OR v.email LIKE ?)"
        )
        like_q = f"%{q}%"
        params.extend([like_q, like_q, like_q, like_q, like_q, like_q])

    if category_filter:
        where_clauses.append("LOWER(v.category) = ?")
        params.append(category_filter.lower())

    if favorite_only:
        where_clauses.append("v.favorite = 1")

    where_sql = " AND ".join(where_clauses)
    direction = "ASC" if str(sort_dir).upper() == "ASC" else "DESC"

    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        total = cursor.execute(f"SELECT COUNT(*) FROM vendors v WHERE {where_sql}", params).fetchone()[0]

        fetch_sql = f"""
            SELECT v.id, v.name, v.category, v.location, v.phone, v.email, v.url, v.notes, v.favorite, v.source, v.created_at
            FROM vendors v
            WHERE {where_sql}
            ORDER BY {sort_col} {direction}, v.name ASC
            LIMIT ? OFFSET ?
        """
        rows = cursor.execute(fetch_sql, [*params, limit, offset]).fetchall()

        cats_raw = cursor.execute(
            "SELECT DISTINCT category FROM vendors WHERE category IS NOT NULL AND category != '' ORDER BY category"
        ).fetchall()
        categories = [c[0] for c in cats_raw if c[0]]

        vendors = [{
            "id": r[0],
            "name": r[1],
            "category": r[2],
            "location": r[3],
            "phone": r[4],
            "email": r[5],
            "url": r[6],
            "notes": r[7],
            "favorite": bool(r[8]),
            "source": r[9],
            "created_at": r[10],
        } for r in rows]

    return vendors, total, categories


def get_vendor(vendor_id: int, db_path: Path | None = None) -> dict[str, Any] | None:
    """Displays full vendor details."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        row = cursor.execute(
            """
            SELECT name, category, location, phone, email, url, notes, favorite, source, created_at
            FROM vendors WHERE id = ?
            """,
            (vendor_id,),
        ).fetchone()

        if not row:
            return None

        name, category, location, phone, email, url, notes, favorite, source, created_at = row
        return {
            "id": vendor_id,
            "name": name,
            "category": category,
            "location": location,
            "phone": phone,
            "email": email,
            "url": url,
            "notes": notes,
            "favorite": bool(favorite),
            "source": source or "manual",
            "created_at": created_at,
        }


def toggle_vendor_favorite(
    vendor_id: int,
    favorite: bool | None = None,
    db_path: Path | None = None,
) -> bool | None:
    """Toggles or sets the favorite flag for a vendor. Returns new boolean state, or None if vendor not found."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        if favorite is not None:
            fav_val = 1 if favorite else 0
            cursor.execute("UPDATE vendors SET favorite = ? WHERE id = ?", (fav_val, vendor_id))
        else:
            cursor.execute(
                "UPDATE vendors SET favorite = CASE WHEN favorite = 1 THEN 0 ELSE 1 END WHERE id = ?",
                (vendor_id,),
            )
        conn.commit()

        row = cursor.execute("SELECT favorite FROM vendors WHERE id = ?", (vendor_id,)).fetchone()
        return bool(row[0]) if row else None


def delete_vendor(vendor_id: int, db_path: Path | None = None) -> bool:
    """Deletes a vendor by ID. Returns True if row was deleted."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        cursor.execute("DELETE FROM vendors WHERE id = ?", (vendor_id,))
        conn.commit()
        return cursor.rowcount > 0
