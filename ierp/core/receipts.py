"""
Receipts / receivables engine for iERP.
Tracks monetary transactions (income, costs, expected payments) against journal events.
Zero external dependencies (stdlib only).
"""

import sqlite3
from contextlib import closing
from datetime import datetime
from pathlib import Path
from typing import Optional

from .db import get_db

_VALID_TYPES = ("income", "cost", "expected")
_VALID_STATUSES = ("paid", "partial", "unpaid")


def _now() -> str:
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def upsert_receipt(
    cursor: sqlite3.Cursor,
    event_id: int,
    amount: float,
    type: str,
    status: str = "paid",
    notes: Optional[str] = None,
    receipt_id: Optional[int] = None,
) -> int:
    """Inserts or updates a receipt. Returns receipts.id."""
    # Validate type and status
    if type not in _VALID_TYPES:
        raise ValueError(f"type must be one of {_VALID_TYPES}, got '{type}'")
    if status not in _VALID_STATUSES:
        raise ValueError(f"status must be one of {_VALID_STATUSES}, got '{status}'")

    if receipt_id is not None:
        cursor.execute(
            """
            UPDATE receipts
            SET event_id = ?, amount = ?, type = ?, status = ?, notes = ?, updated_at = datetime('now', 'localtime')
            WHERE id = ?
            """,
            (event_id, amount, type, status, notes, receipt_id),
        )
        return receipt_id

    cursor.execute(
        """
        INSERT INTO receipts (event_id, amount, type, status, notes)
        VALUES (?, ?, ?, ?, ?)
        """,
        (event_id, amount, type, status, notes),
    )
    return int(cursor.lastrowid or 0)


def list_receipts(
    event_id: Optional[int] = None,
    type: Optional[str] = None,
    status: Optional[str] = None,
    db_path: Optional[Path] = None,
) -> list:
    """Query receipts with optional filters. Returns list of dicts."""
    sql = (
        "SELECT r.id, r.event_id, r.amount, r.type, r.status, r.notes, "
        "r.created_at, r.updated_at, e.title AS event_title, e.start_date AS event_date "
        "FROM receipts r LEFT JOIN events e ON r.event_id = e.id WHERE 1=1"
    )
    params: list = []
    if event_id is not None:
        sql += " AND r.event_id = ?"
        params.append(event_id)
    if type is not None:
        sql += " AND r.type = ?"
        params.append(type)
    if status is not None:
        sql += " AND r.status = ?"
        params.append(status)
    sql += " ORDER BY r.created_at DESC"

    with closing(get_db(db_path)) as conn:
        conn.row_factory = sqlite3.Row
        rows = conn.execute(sql, params).fetchall()
    return [dict(row) for row in rows]


def get_receipt(receipt_id: int, db_path: Optional[Path] = None) -> Optional[dict]:
    """Fetch a single receipt by ID with its linked event title."""
    with closing(get_db(db_path)) as conn:
        conn.row_factory = sqlite3.Row
        row = conn.execute(
            """
            SELECT r.id, r.event_id, r.amount, r.type, r.status, r.notes,
                   r.created_at, r.updated_at, e.title AS event_title, e.start_date AS event_date, e.tags AS event_tags
            FROM receipts r
            LEFT JOIN events e ON r.event_id = e.id
            WHERE r.id = ?
            """,
            (receipt_id,),
        ).fetchone()
    return dict(row) if row else None


def delete_receipt(receipt_id: int, db_path: Optional[Path] = None) -> bool:
    """Delete a receipt by ID. Returns True if a row was deleted."""
    with closing(get_db(db_path)) as conn:
        cur = conn.execute("DELETE FROM receipts WHERE id = ?", (receipt_id,))
        conn.commit()
        return cur.rowcount > 0


def compute_balance(event_id: Optional[int] = None, db_path: Optional[Path] = None) -> dict:
    """
    Computes net financial position from receipts.

    Returns:
        {
            "total_income":     sum of all income receipts (paid + partial),
            "total_costs":      sum of all cost receipts (paid + partial),
            "total_expected":   sum of all expected receipts,
            "net_cash":         total_income - total_costs,
            "net_position":     (total_income + total_expected) - total_costs,
            "outstanding":      sum of per-event outstanding receivables,
        }
    """
    with closing(get_db(db_path)) as conn:
        if event_id is not None:
            rows = conn.execute(
                "SELECT event_id, type, status, amount FROM receipts WHERE event_id = ?",
                (event_id,)
            ).fetchall()
        else:
            rows = conn.execute(
                "SELECT event_id, type, status, amount FROM receipts"
            ).fetchall()

    total_income = 0.0
    total_costs = 0.0
    total_expected = 0.0
    event_income_map: dict[int, float] = {}
    event_expected_map: dict[int, float] = {}

    for eid, rtype, rstatus, amount in rows:
        if rtype == "income" and rstatus in ("paid", "partial"):
            total_income += amount
            if eid is not None:
                event_income_map[eid] = event_income_map.get(eid, 0.0) + amount
        elif rtype == "cost" and rstatus in ("paid", "partial"):
            total_costs += amount
        elif rtype == "expected":
            total_expected += amount
            if eid is not None:
                event_expected_map[eid] = event_expected_map.get(eid, 0.0) + amount

    if event_id is not None:
        outstanding = max(0.0, total_expected - total_income)
    else:
        # Sum outstanding per-event to avoid cross-event income cancellation
        outstanding = 0.0
        for eid, exp in event_expected_map.items():
            inc = event_income_map.get(eid, 0.0)
            if exp > inc:
                outstanding += (exp - inc)
        null_exp = sum(amount for eid, rtype, rstatus, amount in rows if eid is None and rtype == "expected")
        outstanding += null_exp

    return {
        "total_income": round(total_income, 2),
        "total_costs": round(total_costs, 2),
        "total_expected": round(total_expected, 2),
        "net_cash": round(total_income - total_costs, 2),
        "net_position": round((total_income + total_expected) - total_costs, 2),
        "outstanding": round(outstanding, 2),
    }
