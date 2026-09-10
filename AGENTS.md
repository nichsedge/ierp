# AGENTS.md — Guidelines for iERP Development

## ⚡ Core Philosophy & Architecture

* **Zero External Dependencies**: Pure Python 3.11+ standard library only (`sqlite3`, `argparse`, `json`, `datetime`, `http.server`, `urllib`). Do not introduce third-party libraries (`requests`, `flask`, `pydantic`, etc.) into production code.
* **Strict "No Backward Compatibility"**: Always target the latest Python standard features and clean, modern implementations. Never create backward compatibility shims or legacy aliases.
* **Local SQLite with WAL Mode**: All database connections MUST use WAL (`PRAGMA journal_mode = WAL;`) and busy timeouts (`PRAGMA busy_timeout = 5000;`) to guarantee non-blocking concurrent reads and writes between background tasks and the web dashboard.
* **Strict `uv run` Execution**:
  * Run the CLI directly: `uv run ierp <command>` (e.g. `uv run ierp runway`, `uv run ierp radar`).
  * Run test suite: `uv run ierp test` or `uv run pytest`.
  * Run standalone helper scripts directly: `uv run scripts/export_garden.py`.
  * **NEVER** use `uv run python <script.py>`.

---

## 🌐 Ecosystem Role & Downstream Export Contracts

iERP is the **Central Single Source of Truth (SSOT) for Structured Personal ERP Data** across the entire workstation ([`~/Projects/DATA_ARCHITECTURE.md`](file:///home/al/Projects/DATA_ARCHITECTURE.md)):

* **Downstream Export Pipelines**:
  * **Digital Graveyard Export**: `uv run scripts/export_garden.py` regenerates media consumption notes (`content/Read/`, `content/Watch/`), social/profile links (`content/Write/Links.md`), and hardware notes (`content/Personal/Gadgets/`) in `~/Projects/digital-graveyard`.
  * **Commerce Export**: `uv run scripts/export_commerce.py` regenerates `pay.json` (payment accounts) and `referrals.json` (affiliate codes) in `~/Projects/nichsedge.github.io/data/`.
  * **Cloudflare R2 Backup**: `python3 scripts/backup_r2.py` performs atomic SQLite backup and uploads `db/ierp_latest.sqlite` and timestamped snapshots to Cloudflare R2 (`ichsanul-dev`).
* **Upstream Intake**:
  * **Net Worth Snapshots**: High-level multi-asset valuation totals from `portfolio-integration` feed `ierp insert-snapshot`.
  * **Commitments & Burn**: Fixed monthly commitments from `sansfinance` feed `ierp insert-commitment` for runway calculations.
  * **Media Trackers**: Ingests books, films, anime, manga, and dramas via `uv run ierp sync`.
* **Strict Boundary**: Unstructured knowledge, freeform essays, and daily thoughts belong in `digital-graveyard`, NOT in `ierp`. Structured operational events, contacts, and life ops tasks belong in `ierp`.

---

## 🏛️ Domain Layer Organization

Code in `ierp/core/` is organized by bounded domain services:

| Module | Responsibility | Key Tables |
| :--- | :--- | :--- |
| `db.py` | Connection pooling, WAL setup, schema migrations, and indexing | All |
| `projects.py` | Strategic bets and initiatives lifecycle | `projects` |
| `decisions.py` | Judgment calibration, hypotheses, and reviews | `decisions` |
| `finance.py` | Net worth snapshots, recurring burn, and runway computation | `networth_snapshots`, `recurring_commitments` |
| `radar.py` | Dunbar relationship tiers and touchpoint cadence | `contacts` (tier/cadence), `events` |
| `lifeops.py` | Preventive maintenance, servicing, and document renewals | `maintenance_items` |
| `reviews.py` | Sprint retrospectives (weekly, monthly, quarterly) | `retrospectives` |
| `events.py` | Structured event logging, timeline search, and contact linking | `events`, `event_contacts` |
| `contacts.py` | CRM contact management and resolution | `contacts` |
| `receipts.py` | Event-level cashflows and balance calculations | `receipts` |
| `gadgets.py` | Hardware assets, specs, and digital garden sync | `gadgets` |
| `vendors.py` | Service providers, rental shops, and merchants | `vendors` |
| `commerce.py` | Payment accounts and affiliate referrals | `payment_accounts`, `referrals` |
| `media.py` | Content ingestion from tracking platforms | `media_items`, `media_logs` |
| `audit.py` | Life audit & pulse engine scanning for data gaps and overdue items | All |
| `garden.py` | Deep Digital Garden export (projects, decisions, retrospectives, gadgets) | All |
| `dashboard.py`| Zero-dependency web server and GPS webhook ingestion | HTTP handlers |

---

## ⚡ SQLite WAL & FTS5 Full-Text Search Engine

* **Local SQLite with WAL Mode**: All database connections MUST use WAL (`PRAGMA journal_mode = WAL;`) and busy timeouts (`PRAGMA busy_timeout = 5000;`) to guarantee non-blocking concurrent reads and writes between background tasks and the web dashboard.
* **FTS5 Full-Text Search Virtual Table**: The `events_fts` external content virtual table indexes timeline events and notes in real-time using automatic SQLite triggers (`events_ai`, `events_ad`, `events_au`) and provides BM25 relevance ranking (`bm25(events_fts)`).
* **Pure Standard Library Garden Generation**: All markdown notes and YAML frontmatter exports to the Digital Garden must remain 100% standard library (no `pyyaml` or external frontmatter libraries).

---

## 🔄 Proactive Documentation Sync Rule

Whenever CLI commands, database schemas, scripts, or architectural models are added, modified, or removed:
1. Update `README.md` with the new command or feature descriptions.
2. Update this file (`AGENTS.md`) with any relevant architectural guidelines.
3. Add corresponding test coverage in `ierp/tests/test_ierp.py`.
4. Ensure `uv run ierp test` passes 100% before concluding the task.
