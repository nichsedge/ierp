# AGENTS.md — Guidelines for iERP Development

## ⚡ Core Philosophy & Architecture

* **Zero-CGO Pure Go**: Built with Go 1.24+ standard library and `modernc.org/sqlite` (`database/sql`). Compiles to a fast, standalone static binary with zero CGO dependencies.
* **Strict "No Backward Compatibility"**: Always target modern Go idiom, clean package boundaries, and strict type safety. Never create backward compatibility shims or legacy aliases.
* **Local SQLite with WAL Mode**: All database connections MUST use WAL (`PRAGMA journal_mode = WAL;`), foreign keys enabled (`PRAGMA foreign_keys = ON;`), and busy timeouts (`PRAGMA busy_timeout = 5000;`) to guarantee non-blocking concurrent reads and writes between background tasks and the embedded web dashboard.
* **Direct Binary Execution**:
  * Run the CLI directly: `ierp <command>` (e.g. `ierp runway`, `ierp radar`, `ierp audit`).
  * Run test suite: `go test -v ./...`.
  * Build binary: `go build -o bin/ierp ./cmd/ierp`.
  * Install to user path: `cp bin/ierp ~/.local/bin/ierp`.
* **Archived Python Implementation**: The original Python implementation is permanently archived in the `archive/python` branch (`git checkout archive/python`).

---

## 🌐 Ecosystem Role & Downstream Export Contracts

iERP is the **Central Single Source of Truth (SSOT) for Structured Personal ERP Data** across the entire workstation ([`~/Projects/DATA_ARCHITECTURE.md`](file:///home/al/Projects/DATA_ARCHITECTURE.md)):

* **Downstream Export Pipelines**:
  * **Digital Graveyard Deep Export**: `ierp export-garden` regenerates hardware notes (`content/Knowledge/Entities/Gadget/`), strategic initiatives (`content/Knowledge/Projects/`), decision journals (`content/Knowledge/Decisions/`), and retrospectives (`content/Write/Retrospectives/`) in `~/Projects/digital-graveyard`.
  * **Commerce Export**: `ierp export-commerce` regenerates `pay.json` (payment accounts) and `referrals.json` (affiliate codes) in `~/Projects/nichsedge.github.io/data/`.
  * **Cloudflare R2 Backup**: `scripts/backup_r2.py` performs atomic SQLite backup and uploads `db/ierp_latest.sqlite` and timestamped snapshots to Cloudflare R2 (`ichsanul-dev`).
* **Upstream Intake**:
  * **Net Worth Snapshots**: High-level multi-asset valuation totals feed `ierp insert-snapshot`.
  * **Commitments & Burn**: Fixed monthly commitments feed `ierp insert-commitment` for runway calculations.
* **Strict Boundary**: Unstructured knowledge, freeform essays, and daily thoughts belong in `digital-graveyard`, NOT in `ierp`. Structured operational events, contacts, and life ops tasks belong in `ierp`.

---

## 🏛️ Internal Package Boundaries

Code in `internal/` is organized by bounded domain services:

| Package | Responsibility | Key Tables |
| :--- | :--- | :--- |
| `internal/config` | Environment paths (`IERP_DB`, `IERP_MEDIA_DIR`) and ANSI terminal formatting | None |
| `internal/db` | WAL connection pooling, schema initialization, idempotent migrations, FTS5 triggers | All |
| `internal/events` | Structured timeline logging, contact linking, FTS5 search with BM25 & snippets | `events`, `event_contacts`, `events_fts` |
| `internal/contacts`| CRM contact management, insertion, name/ID resolution, tier assignment | `contacts` |
| `internal/radar` | Dunbar relationship tiers (14d/60d/180d) and daily reconnection engine | `contacts`, `events` |
| `internal/finance` | Net worth snapshots, recurring burn commitments, and sovereign runway computation | `networth_snapshots`, `recurring_commitments` |
| `internal/lifeops` | Preventive maintenance, due-date tracking, and recurring auto-rescheduling | `maintenance_items` |
| `internal/projects`| Strategic initiative tracking and event linking | `projects` |
| `internal/decisions`| Decision journal, confidence calibration, and retrospective review tracking | `decisions` |
| `internal/reviews` | Sprint retrospectives (weekly, monthly, quarterly) | `retrospectives` |
| `internal/gadgets` | Hardware assets and physical asset registry | `gadgets` |
| `internal/vendors` | Service merchants, vendors, and favorite toggling | `vendors` |
| `internal/commerce`| Payment destination accounts and affiliate referral codes | `payment_accounts`, `referrals` |
| `internal/garden` | Deep Digital Garden export (projects, decisions, retrospectives, gadgets) | All |
| `internal/importers`| Timeline semantic JSON and natural language date parser | `events` |
| `internal/media` | Media items, logs, idempotent upserts, and profile links | `media_items`, `links` |
| `internal/dashboard`| Embedded HTTP dashboard server (`embed.FS`) & GPS webhook receiver | HTTP Handlers |
| `internal/audit` | Life audit & pulse engine scanning across all life domains | All |
| `cmd/ierp` | Standalone CLI entrypoint with subcommands | CLI Dispatcher |

---

## ⚡ SQLite WAL & FTS5 Full-Text Search Engine

* **Local SQLite with WAL Mode**: All database connections MUST use WAL (`PRAGMA journal_mode = WAL;`) and busy timeouts (`PRAGMA busy_timeout = 5000;`) to guarantee non-blocking concurrent reads and writes between background tasks and the web dashboard.
* **Cadence & Tier SSOT**: Dunbar relationship tier cadences are centrally defined in `contacts.DefaultCadenceByTier` (Tier 0: 0d Untracked, Tier 1: 14d, Tier 2: 60d, Tier 3: 180d) and reused across CRM and Radar domains.
* **Atomic Life Operations**: Life ops and maintenance auto-rescheduling must execute within a single database transaction (`tx.Begin()`), validating date formats upfront before any mutation.
* **FTS5 Full-Text Search Virtual Table**: The `events_fts` external content virtual table indexes timeline events and notes in real-time using automatic SQLite triggers (`events_ai`, `events_ad`, `events_au`) and provides BM25 relevance ranking (`bm25(events_fts)`).
* **Pure Standard Library Garden Generation**: All markdown notes and YAML frontmatter exports to the Digital Garden must remain standard library string formatting without external template or frontmatter dependencies.

---

## 🎨 Dashboard Architecture & Embedded Assets

* **Single-Binary Zero-Build Deployment**: The dashboard uses Go 1.16+ `embed.FS` to vendor all templates and static assets (`Petite-Vue`, CSS, JS) directly inside the compiled binary.
* **Offline-First & Tailscale Access**: Fully offline-capable without external CDN dependencies. Accessible across Tailscale devices.
* **Separation of Concerns**:
  - `internal/dashboard/assets/templates/dashboard.html`: Declarative markup using Petite-Vue directives.
  - `internal/dashboard/assets/static/css/dashboard.css`: Design system, glassmorphism cards, responsive layouts.
  - `internal/dashboard/assets/static/js/app.js`: Reactive Petite-Vue store, tab controllers, API clients.
  - `internal/dashboard/dashboard.go`: Go stdlib `net/http` concurrent server with REST endpoints and GPS webhook ingestion.

---

## 🔄 Proactive Documentation Sync Rule

Whenever CLI commands, database schemas, scripts, or architectural models are added, modified, or removed:
1. Update `README.md` with the new command or feature descriptions.
2. Update this file (`AGENTS.md`) with any relevant architectural guidelines.
3. Add corresponding test coverage in Go `internal/*_test.go`.
4. Ensure `go test -v ./...` passes 100% before concluding the task.
