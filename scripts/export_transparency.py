#!/usr/bin/env python3
"""
Privacy-Safe Sovereign Transparency & Barbell Exporter.
Exports debt-free verification, barbell allocation percentages, and work telemetry
from iERP and portfolio-integration to nichsedge.github.io/data/transparency.json.

Zero private rupiah or dollar amounts are exported.

Usage:
    uv run scripts/export_transparency.py
    uv run scripts/export_transparency.py --check
"""

import argparse
import datetime
import json
import os
import sqlite3
import sys
from pathlib import Path
from typing import Any

IERP_ROOT = Path(__file__).resolve().parents[1]
PORTFOLIO_ROOT = IERP_ROOT.parent / "portfolio-integration"
WEBSITE_DATA = Path(
    os.environ.get(
        "PORTFOLIO_DATA",
        str(Path.home() / "Projects" / "nichsedge.github.io" / "data"),
    )
)


def get_ierp_db() -> Path | None:
    candidates = [
        IERP_ROOT / "ierp" / "events.db",
        IERP_ROOT / "events.db",
    ]
    return next((p for p in candidates if p.exists()), None)


def get_atracker_db() -> Path | None:
    candidates = [
        Path.home() / ".local" / "share" / "atracker" / "atracker.db",
        Path.home() / "Projects" / "atracker" / "tracker.db",
    ]
    return next((p for p in candidates if p.exists()), None)


def query_sovereign_metrics(db_path: Path) -> dict[str, Any]:
    con = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    cur = con.cursor()
    snap = cur.execute(
        "SELECT snapshot_date, liquid_cash, liabilities FROM networth_snapshots ORDER BY snapshot_date DESC, id DESC LIMIT 1"
    ).fetchone()
    
    snapshot_date = snap[0] if snap else datetime.date.today().isoformat()
    liquid_cash = snap[1] if snap else 0.0
    liabilities = snap[2] if snap else 0.0

    commitments = cur.execute(
        "SELECT amount, frequency FROM recurring_commitments WHERE status = 'active'"
    ).fetchall()
    monthly_burn = 0.0
    for amt, freq in commitments:
        if freq == "yearly":
            monthly_burn += amt / 12.0
        elif freq == "quarterly":
            monthly_burn += amt / 3.0
        elif freq == "weekly":
            monthly_burn += amt * (52.0 / 12.0)
        else:
            monthly_burn += amt
    con.close()

    runway_months = round(liquid_cash / monthly_burn, 1) if monthly_burn > 0 else (999.0 if liquid_cash > 0 else 0.0)
    tier = (
        f"Fortress Tier (>{int(runway_months)} Months Sovereign Runway)"
        if runway_months >= 24
        else (f"Sovereign Tier (>{int(runway_months)} Months Runway)" if runway_months >= 12 else "Lean Runway")
    )

    return {
        "snapshot_date": snapshot_date,
        "debt_free": liabilities == 0.0,
        "badge": "100% Debt-Free (Zero Liabilities)" if liabilities == 0.0 else "Active Liabilities",
        "runway_tier": tier,
        "runway_months": runway_months,
        "capital_preservation": "High (SBN & Yield-Bearing Anchors)",
    }


def query_work_telemetry() -> float:
    db = get_atracker_db()
    if not db:
        return 0.0
    try:
        con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
        cur = con.cursor()
        cur.execute(
            "SELECT COALESCE(SUM(duration_secs) / 3600.0, 0.0) FROM events WHERE is_idle = 0 AND timestamp >= (SELECT datetime(MAX(timestamp), '-30 days') FROM events)"
        )
        row = cur.fetchone()
        con.close()
        return round(float(row[0]), 1) if row else 0.0
    except Exception:
        return 0.0


def query_barbell_allocation() -> list[dict[str, Any]]:
    data_dir = PORTFOLIO_ROOT / "data"
    snapshots = sorted([f for f in data_dir.glob("*_snapshot.json") if not f.name.startswith("latest")])
    if not snapshots:
        return []
    
    try:
        snap_data = json.loads(snapshots[-1].read_text(encoding="utf-8"))
        by_class = snap_data.get("allocation", {}).get("by_asset_class", [])
        if by_class:
            result = []
            desc_map = {
                "Fixed Income": ("Sovereign Sukuk (ST013, ST014, PBS) & Bonds", "Capital Preservation & Steady Yield"),
                "Cash & Equivalents": ("High-Yield Digital Banking (Krom, Aladin) & Dry Powder", "Liquid Yield (5.0% - 7.5% p.a.)"),
                "Equities": ("IDX Blue-Chips & High-ROE Value Compounders (BBCA, INDF)", "Compounding & Value Growth"),
                "Crypto Barbell Satellite": ("Asymmetric Barbells (ETH, Hyperliquid L1s)", "High Volatility / Asymmetric Upside"),
                "Crypto": ("Asymmetric Barbells (ETH, Hyperliquid L1s)", "High Volatility / Asymmetric Upside"),
                "Gold": ("Physical & Digital Gold Reserve", "Macro Hedge"),
            }
            for item in by_class:
                cls_name = item.get("asset_class", "")
                pct = round(float(item.get("percentage", 0.0)), 1)
                default_desc = desc_map.get(cls_name, ("Diversified Asset", "Balanced Exposure"))
                result.append(
                    {
                        "asset_class": cls_name,
                        "percentage": pct,
                        "description": default_desc[0],
                        "risk_profile": default_desc[1],
                    }
                )
            return result
    except Exception as e:
        print(f"Warning reading snapshot: {e}", file=sys.stderr)
    return []


def main() -> int:
    parser = argparse.ArgumentParser(description="Export privacy-safe sovereign metrics to portfolio website")
    parser.add_argument("--check", action="store_true", help="Dry run without writing files")
    args = parser.parse_args()

    ierp_db = get_ierp_db()
    if not ierp_db:
        print("Error: ierp events.db not found", file=sys.stderr)
        return 1

    sovereign = query_sovereign_metrics(ierp_db)
    work_hours = query_work_telemetry()
    allocation = query_barbell_allocation()

    payload = {
        "updated_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "snapshot_date": sovereign["snapshot_date"],
        "sovereign_status": {
            "debt_free": sovereign["debt_free"],
            "badge": sovereign["badge"],
            "runway_tier": sovereign["runway_tier"],
            "runway_months": sovereign["runway_months"],
            "capital_preservation": sovereign["capital_preservation"],
        },
        "barbell_allocation": allocation,
        "telemetry": {
            "deep_work_hours_30d": work_hours,
            "builder_focus": "Distributed Systems, Data Quality, AI Agent Orchestration",
            "velocity_percentile": "Top 1% Independent Velocity",
        },
    }

    out_path = WEBSITE_DATA / "transparency.json"
    if args.check:
        print(f"[dry-run] Would write transparency payload to {out_path}:")
        print(json.dumps(payload, indent=2))
        return 0

    WEBSITE_DATA.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
    print(f"Successfully exported privacy-safe transparency metrics -> {out_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
