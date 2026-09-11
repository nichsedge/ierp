#!/usr/bin/env python3
"""Sync latest portfolio snapshot & burn commitments into iERP events.db.

Reads latest_snapshot.json and updates iERP networth_snapshots and commitments,
conforming to ~/Projects/DATA_ARCHITECTURE.md.
"""

import json
from pathlib import Path

from ierp.core.finance import (
    compute_runway,
    get_latest_snapshot,
    insert_commitment,
    insert_snapshot,
    list_commitments,
)

PORTFOLIO_DATA = Path.home() / "Projects" / "portfolio-integration" / "data"


def sync_snapshot() -> dict:
    snapshot_files = sorted(PORTFOLIO_DATA.glob("*_snapshot.json"))
    snapshot_files = [f for f in snapshot_files if not f.name.startswith("latest")]

    latest_file = PORTFOLIO_DATA / "latest_snapshot.json"
    if not latest_file.exists() and snapshot_files:
        latest_file = snapshot_files[-1]

    if not latest_file.exists():
        print(f"⚠️ No snapshot file found in {PORTFOLIO_DATA}")
        return {}

    data = json.loads(latest_file.read_text(encoding="utf-8"))
    metadata = data.get("metadata", {})
    totals = data.get("totals", {})
    allocation = data.get("allocation", {}).get("by_category", [])

    date_str = metadata.get("date")

    # Liquid cash (bank accounts + stablecoins)
    liquid_cash = sum(
        c.get("value_idr", 0.0)
        for c in allocation
        if c.get("category") in ["Bank Account", "Stablecoin"]
    )
    if liquid_cash == 0.0:
        # Fallback to liquid_cash_accounts list if present
        liquid_cash = sum(
            a.get("value_idr", 0.0) for a in data.get("liquid_cash_accounts", [])
        )

    # Investments (stocks, SBN, crypto, P2P)
    investments = sum(
        c.get("value_idr", 0.0)
        for c in allocation
        if c.get("category") not in ["Bank Account", "Stablecoin"]
    )

    liabilities = float(totals.get("total_liabilities_idr", 0.0))

    # Check if this date already exists in iERP to maintain idempotency
    existing = get_latest_snapshot()
    if not existing or existing["snapshot_date"] != date_str:
        insert_snapshot(
            snapshot_date=date_str,
            liquid_cash=liquid_cash,
            investments=investments,
            hard_assets=0.0,
            liabilities=liabilities,
            currency="IDR",
            notes="Automated SSOT sync from portfolio-integration",
        )
        print(f"✅ Ingested snapshot for {date_str} (Liquid: Rp {liquid_cash:,.0f}, Invest: Rp {investments:,.0f})")
    else:
        print(f"ℹ️ Snapshot for {date_str} already recorded in iERP")

    # Seed baseline commitments if empty (based on SansFinance 90-day empirical burn)
    active_commitments = list_commitments(status="active")
    if not active_commitments:
        print("🌱 Seeding initial baseline commitments from SansFinance empirical burn...")
        insert_commitment(
            name="Living Expenses (Food, Transit, Household)",
            amount=3000000.0,
            category="lifestyle",
            frequency="monthly",
            notes="Baseline living expenses derived from SansFinance 90-day burn",
        )
        insert_commitment(
            name="Cloud, Hosting & Domain Infrastructure",
            amount=500000.0,
            category="cloud",
            frequency="monthly",
            notes="Cloudflare, Vercel, domains, API tokens",
        )
        insert_commitment(
            name="Emergency & Health Ops Buffer",
            amount=350000.0,
            category="insurance",
            frequency="monthly",
            notes="Medical reserve and life ops maintenance buffer",
        )
        print("✅ Seeded 3 baseline recurring commitments (Total burn: Rp 3,850,000/mo)")

    runway = compute_runway()
    print("\n🏛️ Sovereign Treasury & Runway Status:")
    print(f"• Liquid Reserves: Rp {runway['liquid_cash']:,.0f}")
    print(f"• Monthly Burn:    Rp {runway['monthly_burn']:,.0f} across {runway['commitments_count']} commitments")
    print(f"• Zero-Income Runway: {runway['runway_months']} Months [{runway['status_label']}]")
    return runway


if __name__ == "__main__":
    sync_snapshot()
