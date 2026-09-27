"""
Payment accounts & referral codes engine for iERP (stdlib only).
Tables: payment_accounts (private financial rails), referrals (public referral codes).
"""

import sqlite3
from contextlib import closing
from datetime import datetime
from pathlib import Path
from typing import Optional

from .db import get_db

_PAY_FIELDS = ("name", "category", "number", "recipient", "details", "details_id")
_REFERRAL_FIELDS = ("name", "category", "code", "link", "benefit", "status", "is_public")


def _now() -> str:
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def init_tables(cursor: sqlite3.Cursor) -> None:
    """Idempotent schema creation (called from db.init_db)."""
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS payment_accounts (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        slug TEXT UNIQUE,
        name TEXT NOT NULL,
        category TEXT,
        number TEXT,
        recipient TEXT,
        details TEXT,
        details_id TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS referrals (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        slug TEXT UNIQUE,
        name TEXT NOT NULL,
        category TEXT,
        code TEXT,
        link TEXT,
        benefit TEXT,
        status TEXT DEFAULT 'ACTIVE',
        is_public INTEGER DEFAULT 1,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_payment_accounts_category ON payment_accounts(category);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_referrals_category ON referrals(category);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_referrals_status ON referrals(status);")


def _coalesce_update(table: str, fields: tuple, row_id: int,
                     values: dict, cursor: sqlite3.Cursor) -> None:
    """Partial-record-safe UPDATE: only non-NULL values overwrite."""
    assignments = ", ".join(f"{f} = COALESCE(?, {f})" for f in fields)
    params = [values.get(f) for f in fields] + [_now(), row_id]
    cursor.execute(f"UPDATE {table} SET {assignments}, updated_at = ? WHERE id = ?", params)


def upsert_payment_account(
    cursor: sqlite3.Cursor,
    slug: str,
    name: str,
    category: Optional[str] = None,
    number: Optional[str] = None,
    recipient: Optional[str] = None,
    details: Optional[str] = None,
    details_id: Optional[str] = None,
) -> int:
    """Idempotent upsert keyed by slug. Returns payment_accounts.id."""
    values = dict(zip(_PAY_FIELDS, (name, category, number, recipient, details, details_id)))
    cursor.execute("SELECT id FROM payment_accounts WHERE slug = ?", (slug,))
    row = cursor.fetchone()
    if row:
        pid = int(row[0])
        _coalesce_update("payment_accounts", _PAY_FIELDS, pid, values, cursor)
        return pid
    cursor.execute("""
        INSERT INTO payment_accounts (slug, name, category, number, recipient, details, details_id)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    """, (slug, *values.values()))
    return int(cursor.lastrowid or 0)


def insert_payment_account(
    slug: str,
    name: str,
    category: Optional[str] = None,
    number: Optional[str] = None,
    recipient: Optional[str] = None,
    details: Optional[str] = None,
    details_id: Optional[str] = None,
    db_path: Optional[Path] = None,
) -> int:
    """Inserts or updates a payment account record in SQLite. Returns payment_accounts.id."""
    with closing(get_db(db_path)) as conn:
        cur = conn.cursor()
        pid = upsert_payment_account(
            cur,
            slug=slug,
            name=name,
            category=category,
            number=number,
            recipient=recipient,
            details=details,
            details_id=details_id,
        )
        conn.commit()
        return pid


def delete_payment_account(account_id: int, db_path: Optional[Path] = None) -> bool:
    """Deletes a payment account by ID. Returns True if row was deleted."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        cursor.execute("DELETE FROM payment_accounts WHERE id = ?", (account_id,))
        conn.commit()
        return cursor.rowcount > 0


def get_payment_account(identifier: str | int, db_path: Optional[Path] = None) -> Optional[dict]:
    """Retrieves a single payment account by ID or slug."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        if isinstance(identifier, int) or (isinstance(identifier, str) and identifier.isdigit()):
            row = cursor.execute("""
                SELECT id, slug, name, category, number, recipient, details, details_id, created_at, updated_at
                FROM payment_accounts WHERE id = ?
            """, (int(identifier),)).fetchone()
        else:
            row = cursor.execute("""
                SELECT id, slug, name, category, number, recipient, details, details_id, created_at, updated_at
                FROM payment_accounts WHERE slug = ?
            """, (str(identifier),)).fetchone()

        if not row:
            return None
        return {
            "id": row[0],
            "slug": row[1],
            "name": row[2],
            "category": row[3],
            "number": row[4],
            "recipient": row[5],
            "details": row[6],
            "details_id": row[7],
            "created_at": row[8],
            "updated_at": row[9],
        }


def upsert_referral(
    cursor: sqlite3.Cursor,
    slug: str,
    name: str,
    category: Optional[str] = None,
    code: Optional[str] = None,
    link: Optional[str] = None,
    benefit: Optional[str] = None,
    status: Optional[str] = None,
    is_public: bool = True,
) -> int:
    """Idempotent upsert keyed by slug. Returns referrals.id."""
    values = dict(zip(_REFERRAL_FIELDS, (name, category, code, link, benefit, status,
                                         1 if is_public else 0)))
    cursor.execute("SELECT id FROM referrals WHERE slug = ?", (slug,))
    row = cursor.fetchone()
    if row:
        rid = int(row[0])
        _coalesce_update("referrals", _REFERRAL_FIELDS, rid, values, cursor)
        return rid
    cursor.execute("""
        INSERT INTO referrals (slug, name, category, code, link, benefit, status, is_public)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    """, (slug, *values.values()))
    return int(cursor.lastrowid or 0)


def delete_referral(referral_id: int, db_path: Optional[Path] = None) -> bool:
    """Deletes a referral code by ID. Returns True if row was deleted."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        cursor.execute("DELETE FROM referrals WHERE id = ?", (referral_id,))
        conn.commit()
        return cursor.rowcount > 0


def get_referral(identifier: str | int, db_path: Optional[Path] = None) -> Optional[dict]:
    """Retrieves a single referral code by ID or slug."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        if isinstance(identifier, int) or (isinstance(identifier, str) and identifier.isdigit()):
            row = cursor.execute("""
                SELECT id, slug, name, category, code, link, benefit, status, is_public, created_at, updated_at
                FROM referrals WHERE id = ?
            """, (int(identifier),)).fetchone()
        else:
            row = cursor.execute("""
                SELECT id, slug, name, category, code, link, benefit, status, is_public, created_at, updated_at
                FROM referrals WHERE slug = ?
            """, (str(identifier),)).fetchone()

        if not row:
            return None
        return {
            "id": row[0],
            "slug": row[1],
            "name": row[2],
            "category": row[3],
            "code": row[4],
            "link": row[5],
            "benefit": row[6],
            "status": row[7],
            "is_public": bool(row[8]),
            "created_at": row[9],
            "updated_at": row[10],
        }


def _query(db_path: Optional[Path], sql: str, params: list) -> list:
    """Runs a read query on a short-lived connection."""
    with closing(get_db(db_path)) as conn:
        return conn.execute(sql, params).fetchall()


def list_payment_accounts(category: Optional[str] = None, db_path: Optional[Path] = None) -> list:
    sql = "SELECT id, slug, name, category, number, recipient FROM payment_accounts"
    params: list = []
    if category:
        sql += " WHERE category = ?"
        params.append(category)
    sql += " ORDER BY category, name"
    return _query(db_path, sql, params)


def list_referrals(category: Optional[str] = None, status: Optional[str] = None,
                   public_only: bool = False, db_path: Optional[Path] = None) -> list:
    sql = ("SELECT id, slug, name, category, code, link, benefit, status, is_public "
           "FROM referrals WHERE 1=1")
    params: list = []
    if category:
        sql += " AND category = ?"
        params.append(category)
    if status:
        sql += " AND UPPER(status) = ?"
        params.append(status.upper())
    if public_only:
        sql += " AND is_public = 1"
    sql += " ORDER BY category, name"
    return _query(db_path, sql, params)
