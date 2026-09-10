# iERP (Individual Enterprise Resource Planning)

> **iERP** — Manage your life, journal, contacts, runway, and timeline like an enterprise of one.

A zero-dependency, offline-first personal operating system, CRM, journal event log system, and life navigation engine with Google Contacts sync, Google Maps Timeline imports, live GPS webhook ingestion, strategic initiatives, decision journaling, sovereign runway calculations, relationship radar, and preventive life ops.

---

## ⚡ Key Features

* **Zero External Dependencies**: Pure Python 3.11+ standard library only. Instant startup, zero supply-chain bloat.
* **Local SQLite with WAL Mode**: Safe concurrent reads/writes between CLI background syncs and web servers.
* **Sovereignty Runway & Treasury**:
  * Track net worth balance snapshots (liquid cash, investments, hard assets, liabilities).
  * Monitor fixed recurring burn commitments across housing, cloud, SaaS, and lifestyle.
  * Real-time automated **Runway in Months** calculation: $\text{Runway} = \frac{\text{Liquid Reserves}}{\text{Monthly Burn}}$.
* **Strategy & Decision Journal**:
  * **Projects & Initiatives**: High-level containers for strategic bets that directly link to daily journal events.
  * **Decision Journal**: Record context, options, hypotheses, and confidence scores (1–10) with scheduled retrospective review dates to calibrate judgment.
* **Human Capital & Relationship Radar**:
  * Dunbar tiers (Tier 1: Inner Circle 14d, Tier 2: Core 60d, Tier 3: Broad 180d).
  * Automated overdue reachout alerts calculated from journal event timelines.
* **Life Ops & Preventive Maintenance**:
  * Track servicing cycles (vehicles, hardware, health checks) with automatic recurring rescheduling upon completion.
  * Monitor document and warranty expirations (passports, driver's licenses, domains, contracts).
* **Sprint Retrospectives**:
  * Periodic weekly, monthly, and quarterly synthesis (wins, energy drains/burnout, lessons, next sprint focus).
* **SQLite FTS5 Full-Text Search**:
  * Sub-millisecond indexed search across timeline events, locations, tags, and notes.
  * BM25 relevance scoring (`bm25(events_fts)`) and highlighted contextual snippet extraction.
* **Deep Digital Garden Sync**:
  * Bidirectional knowledge synthesis generating Obsidian-compatible Markdown notes for **Strategic Projects**, **Decision Journal (PDRs)**, **Sprint Retrospectives**, and **Gadgets**.
  * Auto-generated catalog indexes with Markdown tables and wikilinks (`[[Note Name]]`).
* **$0 Cloud Sync**:
  * **Google Contacts**: Incremental, tokenized sync using Google People API without monthly API costs.
  * **Google Maps Timeline**: Import raw location history JSON with reverse geocoding cache.
  * **Live GPS Webhooks**: Ingest live location updates from mobile GPS logger apps directly into SQLite.
* **Receipts & Receivables Financial Engine**: Track income, costs, and expected payments against journal projects with real-time balance calculations.
* **Web Dashboard**: Responsive dark-mode dashboard with interactive event filtering, project kanban, decision log, relationship radar, runway gauge, vendor management, and financial ledger.

---

## 🚀 Quickstart

### Installation
Clone the repository and run using [uv](https://docs.astral.sh/uv/):
```bash
git clone https://github.com/nichsedge/ierp.git
cd ierp
uv sync
```

### CLI Usage
```bash
# Initialize database schema and indexes
uv run ierp init

# View sovereign treasury runway
uv run ierp runway

# Check relationship radar for overdue contacts
uv run ierp radar --overdue-only

# Log a strategic project / initiative
uv run ierp insert-project --title "Digital Sovereignty Infra" --priority high

# Log a structured event linked to the project
uv run ierp insert --title "Deploy SQLite WAL" --place "Jakarta" --start-date "2026-08-19" --project-id 1

# Record a choice in the Decision Journal
uv run ierp insert-decision --title "Accept Retainer Contract" --choice "Accept" --confidence 8 --review-date "2026-12-01"

# Record a net worth snapshot and monthly recurring commitment
uv run ierp insert-snapshot --liquid 100000000 --investments 250000000 --assets 40000000
uv run ierp insert-commitment --name "Apartment Rent" --amount 6000000 --frequency monthly --category housing

# Schedule a life ops maintenance task
uv run ierp insert-maintenance --name "Motorbike Servicing" --due-date "2026-10-01" --interval 90 --category vehicle

# Launch Web Dashboard
uv run ierp dashboard
```

---

## 🛠️ Complete CLI Command Reference

| Command | Description |
| :--- | :--- |
| `uv run ierp init` | Initialize the SQLite database schema, columns, and indexes. |
| `uv run ierp runway` | Display sovereign runway in months, net worth, and monthly burn rate. |
| `uv run ierp insert-snapshot` | Record a net worth balance snapshot (`--liquid`, `--investments`, `--assets`, `--liabilities`). |
| `uv run ierp snapshots` | List historical net worth balance snapshots. |
| `uv run ierp insert-commitment` | Record a recurring burn commitment (`--name`, `--amount`, `--frequency`, `--category`). |
| `uv run ierp commitments` | List recurring financial commitments and monthly normalized burn. |
| `uv run ierp insert-project` | Create a strategic initiative (`--title`, `--slug`, `--status`, `--priority`, `--target-date`). |
| `uv run ierp projects` | List projects with linked event and decision counts. |
| `uv run ierp show-project <ID\|slug>` | Show full project details with linked events and decisions. |
| `uv run ierp insert-decision` | Record a choice in the decision journal (`--title`, `--choice`, `--confidence`, `--review-date`). |
| `uv run ierp decisions` | List logged decisions with review status filters (`--pending-review`). |
| `uv run ierp show-decision <ID>` | Show full decision context, choice, hypotheses, and post-review outcome. |
| `uv run ierp review-decision <ID>` | Conduct a retrospective review on a decision (`--outcome`, `--status`). |
| `uv run ierp radar` | Display relationship reconnection radar with overdue touchpoint alerts. |
| `uv run ierp set-tier` | Set Dunbar tier (1, 2, 3) and touch cadence in days for a contact. |
| `uv run ierp insert-maintenance` | Schedule a maintenance task or document expiration (`--name`, `--due-date`, `--interval`). |
| `uv run ierp maintenance` | List maintenance tasks with overdue highlighting (`--due-within <days>`). |
| `uv run ierp complete-maintenance <ID>` | Mark maintenance task complete (auto-schedules next occurrence if interval set). |
| `uv run ierp insert-review` | Log a sprint retrospective (`--start`, `--end`, `--type`, `--wins`, `--drains`, `--lessons`, `--focus`). |
| `uv run ierp reviews` | List sprint retrospectives. |
| `uv run ierp show-review <ID>` | Show full sprint retrospective review details. |
| `uv run ierp audit` | Scan entire ERP for missing data, overdue items, and actionable next steps (`--json`). |
| `uv run ierp insert --title "<Title>" ...` | Insert a structured journal event directly (`--project-id`, `--contact`). |
| `uv run ierp list [--limit N]` | List recent events. |
| `uv run ierp show <ID>` | Show full event details and linked contacts. |
| `uv run ierp search "<query>"` | Fast SQLite FTS5 full-text search with BM25 relevance ranking and note snippets. |
| `uv run ierp garden-export` | Export iERP projects, decisions, reviews, and gadgets to Digital Garden markdown notes. |
| `uv run ierp contacts [--source ...]` | List CRM contacts with filtering (`--tier`). |
| `uv run ierp show-contact <ID>` | Show contact details, Dunbar tier, and linked events. |
| `uv run ierp sync-contacts` | Sync contacts from Google People API ($0 cost). |
| `uv run ierp merge-contacts --auto` | Auto-detect and merge matching duplicate contacts. |
| `uv run ierp vendors [--favorite]` | List vendors and preferred service providers. |
| `uv run ierp gadgets` | List hardware assets/gadgets with specs and linkages. |
| `uv run ierp insert-receipt` | Record or update monetary receipt/receivable against an event. |
| `uv run ierp balance` | Show net cash, net position, and outstanding balance summary. |
| `uv run ierp pay` | List payment accounts and banking nodes. |
| `uv run ierp referrals` | List referral codes and affiliate links. |
| `uv run ierp dashboard [--port 8000]` | Start web dashboard server with live GPS webhook receiver. |
| `uv run ierp test` | Run automated unit and integration test suite. |

---

## 🧪 Testing

Run the automated test suite:
```bash
uv run ierp test
# or
uv run pytest
```

---

## 🔒 Privacy & Local-First

All SQLite database records (`events.db`), media assets (`events_media/`), geocoding caches, and Google OAuth tokens remain strictly on your local machine and are ignored by git.

---

## 💾 Backups & Disaster Recovery

iERP includes automated atomic SQLite online backups and zero-dependency Cloudflare R2 cloud synchronization:

```bash
# Perform an immediate atomic backup to local and Cloudflare R2:
python3 scripts/backup_r2.py
```

- **Local backups**: Overwrites a single rolling backup in `~/Projects/ierp/backups/events_backup_latest.sqlite` (with `events_backup_previous.sqlite` rotation).
- **Cloudflare R2**: Overwrites `db/ierp_latest.sqlite` via AWS SigV4 signed requests in your private bucket (`ichsanul-dev`). Zero historical append, zero storage creep, 100% free tier safe forever.
- **Automation**: Executed automatically during the daily ecosystem sync pipeline (`~/Projects/_scheduled_jobs/sync_ecosystem.py`).

