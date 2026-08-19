# iERP (Individual Enterprise Resource Planning)

> **iERP** — Manage your life, journal, contacts, and timeline like an enterprise of one.

A zero-dependency, offline-first personal CRM, journal event log system, and timeline explorer with Google Contacts sync, Google Maps Timeline imports, and live GPS webhook ingestion.

---

## ⚡ Key Features

* **Zero External Dependencies**: Pure Python standard library only. Instant startup, zero supply-chain bloat.
* **Local SQLite with WAL Mode**: Safe concurrent reads/writes between CLI background syncs and web servers.
* **$0 Cloud Sync**:
  * **Google Contacts**: Incremental, tokenized sync using Google People API without monthly API costs.
  * **Google Maps Timeline**: Import raw location history JSON with reverse geocoding cache.
  * **Live GPS Webhooks**: Ingest live location updates from mobile GPS logger apps directly into SQLite.
* **Smart Contact Merge & Linking Engine**: Automatically associates event journal entries with contacts while preserving tags, aliases, and notes.
* **Web Dashboard**: Built-in responsive dashboard with interactive event filtering, contact inspection, and vendor management.

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
# Initialize database
uv run ierp init

# Sync Google Contacts ($0 API cost)
uv run ierp sync-contacts

# Auto-merge duplicate contacts
uv run ierp merge-contacts --auto

# Log a structured event
uv run ierp insert --title "Strategy Sync" --place "Jakarta" --start-date "2026-08-19" --tags "work,meeting"

# List recent events & contacts
uv run ierp list
uv run ierp contacts

# Launch Web Dashboard
uv run ierp dashboard
```

---

## 🛠️ Complete CLI Command Reference

| Command | Description |
| :--- | :--- |
| `uv run ierp init` | Initialize the SQLite database schema and indexes. |
| `uv run ierp sync-contacts` | Sync contacts from Google People API ($0 cost). |
| `uv run ierp merge-contacts --auto` | Auto-detect and merge matching duplicate contacts. |
| `uv run ierp merge-contacts --source-id <ID> --target-id <ID>` | Manually merge two contacts. |
| `uv run ierp import-timeline <file.json>` | Import Google Maps Semantic Location History JSON. |
| `uv run ierp import <dir>` | Import events from Notion export directory. |
| `uv run ierp import-crm <dir>` | Import CRM contacts from Notion export directory. |
| `uv run ierp insert --title "<Title>" ...` | Insert a structured journal event directly. |
| `uv run ierp list [--limit N]` | List recent events. |
| `uv run ierp show <ID>` | Show full event details and linked contacts. |
| `uv run ierp search "<query>"` | Search events by keyword. |
| `uv run ierp contacts [--source manual\|google\|merged\|all]` | List CRM contacts with filtering. |
| `uv run ierp show-contact <ID>` | Show contact details and linked events. |
| `uv run ierp vendors [--category <CAT>] [--favorite]` | List vendors and preferred service providers. |
| `uv run ierp show-vendor <ID>` | Show full vendor/seller details. |
| `uv run ierp insert-vendor --name "<Name>" ...` | Insert a vendor/seller record directly. |
| `uv run ierp link` | Run relationship discovery between events and contacts. |
| `uv run ierp dashboard [--port 8000]` | Start web dashboard server with live GPS webhook receiver. |
| `uv run ierp test` | Run automated test suite. |

---

## 🧪 Testing

Run the automated test suite with standard library `unittest`:
```bash
uv run ierp test
# or
uv run python -m unittest discover -s ierp/tests
```

---

## 🔒 Privacy & Local-First

All SQLite database records (`events.db`), media assets (`events_media/`), geocoding caches, and Google OAuth tokens remain strictly on your local machine and are ignored by git.
