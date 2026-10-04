# iERP (Individual Enterprise Resource Planning)

> **iERP** — Manage your life, journal, contacts, runway, and timeline like an enterprise of one.

A zero-CGO, offline-first personal operating system, CRM, journal event log system, and life navigation engine written in modern Go (`github.com/nichsedge/ierp`) with embedded web dashboard, strategic initiatives, decision journaling, sovereign runway calculations, relationship radar, and preventive life ops.

---

## ⚡ Key Features

* **Zero-CGO Pure Go**: Built with Go 1.24+ standard library and pure Go SQLite (`modernc.org/sqlite`). Instant startup, portable single binary, zero external runtime dependencies.
* **Local SQLite with WAL Mode**: Safe concurrent reads/writes between CLI background syncs and web servers with foreign keys and 5000ms busy timeout.
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
* **Embedded Web Dashboard**: Single-binary dashboard with embedded Petite-Vue templates and assets (`embed.FS`). Tailscale & mobile-optimized with standalone PWA support, interactive 365-day activity heatmaps, event filtering, project initiatives, decision journal, relationship radar, and sovereignty runway.

---

## 🚀 Quickstart

### Installation & Build

```bash
# Clone repository
git clone https://github.com/nichsedge/ierp.git
cd ierp

# Build standalone binary
go build -o bin/ierp ./cmd/ierp

# Install to local path
cp bin/ierp ~/.local/bin/ierp
```

### CLI Usage

```bash
# Initialize database schema, columns, and FTS5 triggers
ierp init-db

# View sovereign treasury runway
ierp runway

# Check relationship radar for overdue contacts
ierp radar --overdue-only

# Pick single highest-priority reconnection today
ierp daily-reconnection

# Log a strategic project / initiative
ierp insert-project --title "Digital Sovereignty Infra" --priority high

# Log a structured event linked to the project
ierp insert --title "Deploy SQLite WAL" --place "Jakarta" --start "2026-10-04" --project-id 1

# Search events with FTS5 BM25 ranking
ierp search "Dinner"

# Record a choice in the Decision Journal
ierp insert-decision --title "Accept Retainer Contract" --choice "Accept" --confidence 8 --review-date "2026-12-01"

# Record a net worth snapshot and monthly recurring commitment
ierp insert-snapshot --liquid 100000000 --investments 250000000 --assets 40000000
ierp insert-commitment --name "Apartment Rent" --amount 6000000 --frequency monthly --category housing

# Schedule a life ops maintenance task
ierp insert-maintenance --name "Motorbike Servicing" --due-date "2026-11-01" --interval 90 --category vehicle

# Run life audit engine
ierp audit

# Export to Digital Graveyard and Portfolio Commerce
ierp export-garden
ierp export-commerce

# Launch Embedded Web Dashboard (zero build pipeline, embedded Petite-Vue)
ierp dashboard --port 8921
```

---

## 🛠️ Complete CLI Command Reference

| Command | Description |
| :--- | :--- |
| `ierp init-db` | Initialize SQLite schema, indexes, and FTS5 triggers. |
| `ierp runway` | Display sovereign runway in months, net worth, and monthly burn rate. |
| `ierp insert-snapshot` | Record a net worth balance snapshot (`--liquid`, `--investments`, `--assets`, `--liabilities`). |
| `ierp snapshots` | List historical net worth balance snapshots. |
| `ierp insert-commitment` | Record a recurring burn commitment (`--name`, `--amount`, `--frequency`, `--category`). |
| `ierp commitments` | List recurring financial commitments and monthly normalized burn. |
| `ierp insert-project` | Create a strategic initiative (`--title`, `--slug`, `--priority`, `--target`). |
| `ierp projects` | List projects. |
| `ierp insert-decision` | Record a choice in the decision journal (`--title`, `--choice`, `--confidence`, `--review-date`). |
| `ierp decisions` | List logged decisions with review status. |
| `ierp review-decision <ID>` | Conduct a retrospective review on a decision (`--outcome`). |
| `ierp radar` | Display relationship reconnection radar (`--overdue-only`, `--tier`). |
| `ierp daily-reconnection` | Show top-priority relationship needing reachout today. |
| `ierp set-tier` | Set Dunbar tier (0-3) and touch cadence for a contact (`--contact-id`, `--tier`, `--cadence`). |
| `ierp insert-maintenance` | Schedule a maintenance task or document expiration (`--name`, `--due-date`, `--interval`). |
| `ierp maintenance` | List maintenance tasks with overdue highlighting (`--overdue-only`). |
| `ierp complete-maintenance <ID>` | Mark task complete (auto-reschedules if interval configured). |
| `ierp insert-review` | Log a sprint retrospective (`--start`, `--end`, `--type`, `--wins`, `--drains`, `--lessons`, `--focus`). |
| `ierp reviews` | List sprint retrospectives. |
| `ierp audit` | Scan entire ERP for missing data, overdue items, and actionable next steps. |
| `ierp insert` | Insert a structured journal event (`--title`, `--place`, `--start`, `--tags`, `--contacts`). |
| `ierp list` | List recent events with filters (`--limit`, `--tag`, `--q`). |
| `ierp search "<query>"` | Fast SQLite FTS5 full-text search with BM25 relevance ranking and note snippets. |
| `ierp export-garden` | Export projects, decisions, reviews, and gadgets to Digital Garden markdown notes. |
| `ierp export-commerce` | Export payment accounts and referral links to `pay.json` and `referrals.json`. |
| `ierp sync portfolio` | Ingest multi-asset portfolio snapshot & update sovereign runway. |
| `ierp sync gh-projects` | Ingest GitHub repositories via GraphQL API into SQLite `events.db`. |
| `ierp export gh-projects` | Export all repositories to `nichsedge.github.io/data/github_repos_all.json`. |
| `ierp r2 [status\|push\|pull\|auto]` | Bidirectional Cloudflare R2 synchronization with pure Go AWS SigV4 & WAL checkpointing. |
| `ierp dashboard` | Launch zero-dependency embedded web dashboard server (`--port`, `--host`). |

---

## 🧪 Testing

```bash
# Run all Go package unit and integration tests
go test -v ./...
```
