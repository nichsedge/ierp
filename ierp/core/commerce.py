"""
Payment accounts & referral codes engine for iERP (stdlib only).
Tables: payment_accounts (private financial rails), referrals (public referral codes).
"""

import json
import sqlite3
from datetime import datetime
from pathlib import Path
from typing import Optional
from .config import DB_PATH


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
    cursor.execute("SELECT id FROM payment_accounts WHERE slug = ?", (slug,))
    row = cursor.fetchone()
    if row:
        pid = int(row[0])
        cursor.execute("""
            UPDATE payment_accounts
            SET name = ?, category = ?, number = ?, recipient = ?, details = ?, details_id = ?, updated_at = ?
            WHERE id = ?
        """, (name, category, number, recipient, details, details_id, _now(), pid))
        return pid
    cursor.execute("""
        INSERT INTO payment_accounts (slug, name, category, number, recipient, details, details_id)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    """, (slug, name, category, number, recipient, details, details_id))
    return int(cursor.lastrowid or 0)


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
    cursor.execute("SELECT id FROM referrals WHERE slug = ?", (slug,))
    row = cursor.fetchone()
    if row:
        rid = int(row[0])
        cursor.execute("""
            UPDATE referrals
            SET name = ?, category = ?, code = ?, link = ?, benefit = ?, status = ?, is_public = ?, updated_at = ?
            WHERE id = ?
        """, (name, category, code, link, benefit, status, 1 if is_public else 0, _now(), rid))
        return rid
    cursor.execute("""
        INSERT INTO referrals (slug, name, category, code, link, benefit, status, is_public)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    """, (slug, name, category, code, link, benefit, status, 1 if is_public else 0))
    return int(cursor.lastrowid or 0)


def list_payment_accounts(category: Optional[str] = None, db_path: Optional[Path] = None) -> list:
    from .db import get_db
    conn = get_db(db_path)
    cursor = conn.cursor()
    init_tables(cursor)
    sql = "SELECT id, slug, name, category, number, recipient FROM payment_accounts"
    params = []
    if category:
        sql += " WHERE category = ?"
        params.append(category)
    sql += " ORDER BY category, name"
    rows = cursor.execute(sql, params).fetchall()
    conn.close()
    return rows


def list_referrals(category: Optional[str] = None, status: Optional[str] = None,
                   public_only: bool = False, db_path: Optional[Path] = None) -> list:
    from .db import get_db
    conn = get_db(db_path)
    cursor = conn.cursor()
    init_tables(cursor)
    sql = "SELECT id, slug, name, category, code, link, benefit, status, is_public FROM referrals WHERE 1=1"
    params = []
    if category:
        sql += " AND category = ?"
        params.append(category)
    if status:
        sql += " AND UPPER(status) = ?"
        params.append(status.upper())
    if public_only:
        sql += " AND is_public = 1"
    sql += " ORDER BY category, name"
    rows = cursor.execute(sql, params).fetchall()
    conn.close()
    return rows
