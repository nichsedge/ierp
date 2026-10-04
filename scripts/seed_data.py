#!/usr/bin/env python3
"""Seed structured iERP data across domain modules. Uses CLI only."""
import subprocess, sys

def cmd(args):
    full = ["ierp"] + args
    print("Running:", " ".join(full))
    r = subprocess.run(full, capture_output=False, text=True)
    return r.returncode

c = 0
# Decisions: 5 more per quarter guide (2-8)
c |= cmd(["insert-decision", "--title", "Switch Primary Dev Workstation to PRoot Mobile",
           "--choice", "Migrate all active repos to Android PRoot Debian ARM64 (Xiaomi 14T Pro)",
           "--context", "Laptop WiFi unstable; mobile provides more consistent connectivity.",
           "--expected", "Consistent dev pipeline, less downtime, faster commits.",
           "--confidence", "8", "--review-date", "2026-11-15", "--project-id", "2"])

c |= cmd(["insert-decision", "--title", "Launch Bijak-Beli Price Transparency Index",
           "--choice", "Build web scraper + SQLite ingestion pipeline from Indomaret/Alfamart catalogs",
           "--expected", "Real-time price diffusion data for consumer staples.",
           "--confidence", "7", "--review-date", "2026-12-30", "--project-id", "2"])

c |= cmd(["insert-decision", "--title", "Adopt OpenRouter as Default LLM Provider",
           "--choice", "Route all agent tasks through openrouter/free tier first, escalate only on failure",
           "--expected", "Cost-sensitive operation; preserve high-value model budget.",
           "--confidence", "9", "--review-date", "2026-11-01"])

c |= cmd(["insert-decision", "--title", "Retire Old Digital Graveyard get-data Scripts",
           "--choice", "Delete deprecated scripts, rely solely on ierp export scripts",
           "--expected", "Single source of truth in ierp, no duplicate logic.",
           "--confidence", "9", "--review-date", "2027-01-01", "--project-id", "3"])

c |= cmd(["insert-decision", "--title", "Build Multi-Source Net Worth Snapshot Pipeline",
           "--choice", "Ingest monthly totals from portfolio-integration and sansfinance feeds into ierp finance",
           "--expected", "Runway calculations stay current without manual entry.",
           "--confidence", "8", "--review-date", "2026-11-30", "--project-id", "1"])

# Review: add monthly retrospective
c |= cmd(["insert-review", "--start", "2026-09-01", "--end", "2026-09-30",
           "--type", "monthly", "--wins", "Completed PRoot migration; synced media sources.",
           "--drains", "Laptop connectivity; budget tracking lag.",
           "--lessons", "Mobile-first dev improves reliability.",
           "--focus", "Price index pipeline + monthly finance ingestion",
           "--rating", "8", "--notes", "First month of structured retrospective cycle."])

# Finance: new snapshot today (2026-10-04) + commitment
c |= cmd(["insert-snapshot", "--date", "2026-10-04", "--liquid", "152000000",
           "--investments", "345000000", "--assets", "120000000",
           "--liabilities", "950000", "--currency", "IDR",
           "--notes", "October 2026 monthly snapshot including Xiaomi 14T Pro hard asset estimate."])

c |= cmd(["insert-commitment", "--name", "Mobile Data & Connectivity (Telkomsel/IndiHome Backup)",
           "--amount", "250000", "--frequency", "monthly", "--category", "cloud",
           "--currency", "IDR", "--notes", "Backup connectivity for mobile-first dev pipeline."])

# Maintenance: add document renewal
c |= cmd(["insert-maintenance", "--name", "KTP / Identity Document Renewal", "--due-date", "2027-06-15",
           "--category", "legal_id", "--interval", "365", "--cost", "50000",
           "--notes", "KTP renewal for Cimahi address; schedule 3 months ahead."])

# Radar / contact tier updates: set tiers for a sample of contacts
import sqlite3
with sqlite3.connect("ierp/events.db") as conn:
    cursor = conn.cursor()
    # Tier updates for existing contacts (sample inner circle and core network)
    for cid, tier, cadence in [
        (1, 2, 60),   # Adit -> core
        (2, 1, 14),   # Alvin -> inner circle
        (9, 2, 60),   # Daffa -> core (Tier 1 inner?) let's pick 2
        (5, 3, 180),  # Anggi -> broad
    ]:
        cursor.execute("UPDATE contacts SET tier = ?, cadence_days = ? WHERE id = ?", (tier, cadence, cid))
    conn.commit()
    # Verify
    cursor.execute("SELECT id, name, tier, cadence_days FROM contacts WHERE id IN (1,2,5,9)")
    print("Radar tier updates:", cursor.fetchall())

print(f"Seed completed with exit code {c}")
if c != 0:
    sys.exit(1)
