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

# Sync Google Contacts
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
