"""
Sovereign Treasury & Runway domain service for iERP.
Manages net worth balance snapshots, recurring burn commitments, and calculates
real-world sovereignty runway (months of living expenses).
Zero external dependencies (Python standard library only).
"""

from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional

from .db import db_session, get_db, init_db


def insert_snapshot(
    snapshot_date: Optional[str] = None,
    liquid_cash: float = 0.0,
    investments: float = 0.0,
    hard_assets: float = 0.0,
    liabilities: float = 0.0,
    currency: str = "IDR",
    notes: Optional[str] = None,
    db_path: Optional[Path] = None,
) -> int:
    """Records a balance sheet / net worth snapshot."""
    init_db(db_path)
    date_str = snapshot_date or datetime.now().strftime("%Y-%m-%d")

    with db_session(db_path) as cursor:
        cursor.execute("""
        INSERT INTO networth_snapshots (
            snapshot_date, liquid_cash, investments, hard_assets, liabilities, currency, notes
        ) VALUES (?, ?, ?, ?, ?, ?, ?)
        """, (date_str, float(liquid_cash), float(investments), float(hard_assets), float(liabilities), currency, notes))
        return cursor.lastrowid


def list_snapshots(limit: int = 30, db_path: Optional[Path] = None) -> List[Dict[str, Any]]:
    """Lists net worth snapshots in chronological order."""
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()
    rows = cursor.execute("""
    SELECT id, snapshot_date, liquid_cash, investments, hard_assets, liabilities, currency, notes, created_at
    FROM networth_snapshots
    ORDER BY snapshot_date DESC, id DESC
    LIMIT ?
    """, (limit,)).fetchall()
    conn.close()

    results = []
    for r in rows:
        liquid = r[2] or 0.0
        invest = r[3] or 0.0
        assets = r[4] or 0.0
        liab = r[5] or 0.0
        net_worth = (liquid + invest + assets) - liab
        results.append({
            "id": r[0],
            "snapshot_date": r[1],
            "liquid_cash": liquid,
            "investments": invest,
            "hard_assets": assets,
            "liabilities": liab,
            "net_worth": net_worth,
            "currency": r[6],
            "notes": r[7],
            "created_at": r[8],
        })
    return results


def get_latest_snapshot(db_path: Optional[Path] = None) -> Optional[Dict[str, Any]]:
    """Retrieves the most recent net worth snapshot."""
    snaps = list_snapshots(limit=1, db_path=db_path)
    return snaps[0] if snaps else None


def insert_commitment(
    name: str,
    amount: float,
    category: str = "saas",
    currency: str = "IDR",
    frequency: str = "monthly",
    payment_account_id: Optional[int] = None,
    status: str = "active",
    renewal_date: Optional[str] = None,
    notes: Optional[str] = None,
    db_path: Optional[Path] = None,
) -> int:
    """Inserts a recurring financial commitment / fixed burn expense."""
    init_db(db_path)
    valid_freqs = {"monthly", "yearly", "quarterly", "weekly"}
    clean_freq = frequency.lower() if frequency.lower() in valid_freqs else "monthly"

    with db_session(db_path) as cursor:
        cursor.execute("""
        INSERT INTO recurring_commitments (
            name, category, amount, currency, frequency, payment_account_id, status, renewal_date, notes
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        """, (
            name.strip(),
            category.strip().lower(),
            float(amount),
            currency.upper(),
            clean_freq,
            payment_account_id,
            status,
            renewal_date,
            notes,
        ))
        return cursor.lastrowid


def list_commitments(
    status: Optional[str] = "active",
    category: Optional[str] = None,
    db_path: Optional[Path] = None,
) -> List[Dict[str, Any]]:
    """Lists recurring financial commitments with optional filtering."""
    init_db(db_path)
    conn = get_db(db_path)
    cursor = conn.cursor()

    query = """
    SELECT c.id, c.name, c.category, c.amount, c.currency, c.frequency, 
           c.payment_account_id, c.status, c.renewal_date, c.notes,
           p.name as account_name
    FROM recurring_commitments c
    LEFT JOIN payment_accounts p ON p.id = c.payment_account_id
    WHERE 1=1
    """
    params: List[Any] = []

    if status:
        query += " AND c.status = ?"
        params.append(status)
    if category:
        query += " AND c.category = ?"
        params.append(category.lower())

    query += " ORDER BY c.amount DESC"
    rows = cursor.execute(query, params).fetchall()
    conn.close()

    results = []
    for r in rows:
        amount = r[3] or 0.0
        freq = r[5] or "monthly"
        # Calculate monthly normalized amount
        if freq == "yearly":
            monthly_norm = amount / 12.0
        elif freq == "quarterly":
            monthly_norm = amount / 3.0
        elif freq == "weekly":
            monthly_norm = amount * (52.0 / 12.0)
        else:
            monthly_norm = amount

        results.append({
            "id": r[0],
            "name": r[1],
            "category": r[2],
            "amount": amount,
            "monthly_amount": monthly_norm,
            "currency": r[4],
            "frequency": freq,
            "payment_account_id": r[6],
            "status": r[7],
            "renewal_date": r[8],
            "notes": r[9],
            "account_name": r[10],
        })
    return results


def delete_commitment(commitment_id: int, db_path: Optional[Path] = None) -> bool:
    """Deletes a recurring commitment record by ID."""
    init_db(db_path)
    with db_session(db_path) as cursor:
        cursor.execute("DELETE FROM recurring_commitments WHERE id = ?", (commitment_id,))
        return cursor.rowcount > 0


def compute_monthly_burn(status: str = "active", db_path: Optional[Path] = None) -> Dict[str, Any]:
    """Computes total monthly recurring burn rate segmented by category."""
    commitments = list_commitments(status=status, db_path=db_path)
    total_monthly = 0.0
    by_category: Dict[str, float] = {}

    for c in commitments:
        m_amt = c["monthly_amount"]
        total_monthly += m_amt
        cat = c["category"]
        by_category[cat] = by_category.get(cat, 0.0) + m_amt

    return {
        "total_monthly_burn": total_monthly,
        "commitments_count": len(commitments),
        "by_category": by_category,
    }


def compute_runway(db_path: Optional[Path] = None) -> Dict[str, Any]:
    """
    Calculates sovereignty runway in months:
    Runway = Liquid Reserves / Total Monthly Recurring Burn
    """
    snapshot = get_latest_snapshot(db_path)
    burn = compute_monthly_burn(status="active", db_path=db_path)

    monthly_burn = burn["total_monthly_burn"]
    liquid_cash = snapshot["liquid_cash"] if snapshot else 0.0
    net_worth = snapshot["net_worth"] if snapshot else 0.0

    if monthly_burn <= 0:
        runway_months = float("inf") if liquid_cash > 0 else 0.0
        status_label = "INFINITE" if liquid_cash > 0 else "ZERO_RESERVE"
    else:
        runway_months = round(liquid_cash / monthly_burn, 1)
        if runway_months >= 24:
            status_label = "FORTRESS (>24m)"
        elif runway_months >= 12:
            status_label = "SOVEREIGN (>12m)"
        elif runway_months >= 6:
            status_label = "STABLE (6-12m)"
        elif runway_months >= 3:
            status_label = "LEAN (3-6m)"
        else:
            status_label = "CRITICAL (<3m)"

    return {
        "liquid_cash": liquid_cash,
        "net_worth": net_worth,
        "monthly_burn": monthly_burn,
        "runway_months": runway_months if runway_months != float("inf") else 999.0,
        "is_infinite": monthly_burn <= 0 and liquid_cash > 0,
        "status_label": status_label,
        "burn_by_category": burn["by_category"],
        "latest_snapshot_date": snapshot["snapshot_date"] if snapshot else None,
        "commitments_count": burn["commitments_count"],
    }


def get_sansfinance_db_path() -> Optional[Path]:
    """Locates the local Sans Finance SQLite database or snapshot (read-only SSOT)."""
    import os

    env_path = os.getenv("SANSFINANCE_DB_PATH")
    if env_path:
        p = Path(env_path)
        if p.exists() and p.stat().st_size > 0:
            return p

    candidates = [
        Path.home() / "Projects" / "portfolio-integration" / "data" / "sans_finance_latest.sqlite",
        Path.home() / "Projects" / "sansfinance" / "sans_finance_db_snapshot.sqlite",
    ]
    for c in candidates:
        if c.exists() and c.stat().st_size > 0:
            return c
    return None


def get_sansfinance_cashflow(limit: int = 12, db_file: Optional[Path] = None) -> List[Dict[str, Any]]:
    """
    Reads monthly cashflow aggregates (Income vs Expense) directly from
    the Sans Finance SQLite database snapshot (Single Source of Truth).
    Returns list of monthly dictionaries compatible with dashboard cashflow charts.
    """
    import sqlite3
    from contextlib import closing

    target_db = db_file or get_sansfinance_db_path()
    if not target_db or not target_db.exists() or target_db.stat().st_size == 0:
        return []

    query = """
    SELECT 
        strftime('%Y-%m', date/1000, 'unixepoch', 'localtime') as ym,
        type,
        SUM(amount) / 100.0 as total_amt
    FROM expenses
    WHERE date IS NOT NULL
    GROUP BY ym, type
    ORDER BY ym DESC
    """
    cashflow_map: Dict[str, Dict[str, Any]] = {}

    try:
        # Connect in read-only URI mode to prevent any locks or mutation
        with closing(sqlite3.connect(f"file:{target_db}?mode=ro", uri=True)) as conn:
            cursor = conn.cursor()
            rows = cursor.execute(query).fetchall()
            for ym, rtype, total_amt in rows:
                if not ym:
                    continue
                if ym not in cashflow_map:
                    cashflow_map[ym] = {"year_month": ym, "income": 0.0, "cost": 0.0, "expected": 0.0}
                amt = float(total_amt or 0.0)
                rtype_upper = (rtype or "").upper()
                if rtype_upper == "INCOME":
                    cashflow_map[ym]["income"] += amt
                elif rtype_upper == "EXPENSE":
                    cashflow_map[ym]["cost"] += amt
    except Exception:
        return []

    return list(cashflow_map.values())[:limit]


def compute_sansfinance_summary(db_file: Optional[Path] = None) -> Dict[str, Any]:
    """Computes overall cashflow totals from the Sans Finance SSOT database snapshot."""
    import sqlite3
    from contextlib import closing

    target_db = db_file or get_sansfinance_db_path()
    if not target_db or not target_db.exists() or target_db.stat().st_size == 0:
        return {
            "total_income": 0.0,
            "total_costs": 0.0,
            "net_cash": 0.0,
            "net_position": 0.0,
            "outstanding": 0.0,
            "source": "none",
        }

    try:
        with closing(sqlite3.connect(f"file:{target_db}?mode=ro", uri=True)) as conn:
            cursor = conn.cursor()
            income_row = cursor.execute(
                "SELECT COALESCE(SUM(amount), 0) / 100.0 FROM expenses WHERE UPPER(type) = 'INCOME'"
            ).fetchone()
            cost_row = cursor.execute(
                "SELECT COALESCE(SUM(amount), 0) / 100.0 FROM expenses WHERE UPPER(type) = 'EXPENSE'"
            ).fetchone()
            total_income = float(income_row[0]) if income_row else 0.0
            total_costs = float(cost_row[0]) if cost_row else 0.0
            net_cash = total_income - total_costs
            return {
                "total_income": round(total_income, 2),
                "total_costs": round(total_costs, 2),
                "net_cash": round(net_cash, 2),
                "net_position": round(net_cash, 2),
                "outstanding": 0.0,
                "source": "sansfinance",
            }
    except Exception:
        return {
            "total_income": 0.0,
            "total_costs": 0.0,
            "net_cash": 0.0,
            "net_position": 0.0,
            "outstanding": 0.0,
            "source": "none",
        }
