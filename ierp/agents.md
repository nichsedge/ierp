# Agent Guidelines for iERP (Individual Enterprise Resource Planning)

This directory contains a localized database and CLI system for managing personal journal events, CRM contacts, and timeline tracking. Future agents interacting with this system should adhere to the guidelines below.

---

## 📂 System Architecture & Modules

The system is modularized under `ierp/core/` with single-command execution through `ierp.cli`:

- **CLI Entrypoint**: `ierp.cli` (invoked via `uv run ierp <command>`)
- **Core Modules**:
  - `ierp/core/config.py`: Central paths (`events.db`, `events_media`), constants, ANSI colors.
  - `ierp/core/db.py`: SQLite `WAL` mode connection manager, schema migrations, and performance indexes.
  - `ierp/core/merging.py`: Contact deduplication, note merging, and relationship preservation engine.
  - `ierp/core/google_sync.py`: Google People API OAuth 2.0 client and incremental delta sync (`syncToken`).
  - `ierp/core/geocoding.py`: OpenStreetMap Nominatim reverse-geocoder with rate-limiting and persistent disk caching.
  - `ierp/core/importers.py`: Parsers for Notion CSV/Markdown exports & Google Maps Semantic Location History JSON.
  - `ierp/core/dashboard.py`: Single-page interactive web dashboard, JSON REST API, and OwnTracks GPS webhook receiver.
- **Automated Tests**: [ierp/tests/test_ierp.py](file:///home/al/Projects/ierp/ierp/tests/test_ierp.py)
- **Database**: `ierp/events.db` (SQLite with `WAL` journaling mode, git-ignored)
- **Media Attachments**: `ierp/events_media/` (Local folder, git-ignored)

---

## 🗄️ Database Schema & Concurrency

- SQLite operates in **Write-Ahead Logging (`WAL`)** mode with `busy_timeout = 5000ms` and `foreign_keys = ON`.
- Includes performance indexes on `events(start_date)`, `events(place)`, `contacts(name)`, `contacts(email)`, `contacts(google_id)`, `contacts(source)`, `event_contacts(event_id, contact_id)`, `vendors(name)`, `vendors(category)`, and `vendors(favorite)`.

---

## 🤖 Guidelines for AI Agents

### 1. Zero External Dependencies
- The system is built strictly using Python standard library modules (`urllib.request`, `http.server`, `sqlite3`, `json`, `socket`, `webbrowser`, `unittest`). Do not introduce external dependencies (`requests`, `pandas`, `pydantic`).

### 2. Standardized ISO 8601 Dates
- All dates (`start_date`, `end_date`, and `contacts.date`) must be formatted strictly in ISO 8601 (`YYYY-MM-DD` or `YYYY-MM-DDTHH:MM:SS` / `YYYY-MM-DD HH:MM:SS`).
- Do not insert raw text date strings like `"August 4, 2025"`. Convert them to `YYYY-MM-DD` before inserting.

### 3. Strict Entity Separation (People vs Vendors/Businesses)
- **Contacts (`contacts`)** are strictly for human person-contacts and colleagues to preserve Google Contacts sync integrity and person deduplication.
- **Vendors & Sellers (`vendors`)** are dedicated for commercial businesses, rental providers, preferred shops, and favorite services (e.g. motorbike rentals, hotels, clinics). Do NOT insert business entities into `contacts`.

### 4. Strict Location Data Integrity
- Do not infer or hallucinate locations for contacts or vendors when location is unknown or unspecified.
- Store `NULL` / `None` if the user has not explicitly provided a location. Do not guess city or work locations based on company or event association.

### 5. Ingesting New Unstructured Entries
- Parse the unstructured log (resolve dates to ISO `YYYY-MM-DD`, locations, tags, and notes), and then insert it directly:
  ```bash
  uv run ierp insert \
    --title "Short Title" \
    --place "Location" \
    --start-date "YYYY-MM-DD" \
    --end-date "YYYY-MM-DD" \
    --tags "tag1, tag2" \
    --notes "Full text description of what happened."
  ```

### 6. Google Contacts Sync & Contact Sources ($0 API Cost)
- **Contact Sources**:
  - `manual`: Created manually in the local CRM or imported from Notion.
  - `google`: Synced from Google Contacts (Google People API).
  - `merged`: A unified contact containing both manual CRM notes/client associations and Google sync metadata.
- **Sync Command**:
  ```bash
  uv run ierp sync-contacts [--full] [--credentials path/to/credentials.json]
  ```
- **Merging Duplicate Contacts**:
  - Auto-merge duplicates across manual & Google synced contacts:
    ```bash
    uv run ierp merge-contacts --auto
    ```
  - Manually merge specific contacts:
    ```bash
    uv run ierp merge-contacts --source-id <id_to_remove> --target-id <id_to_keep>
    ```

### 7. Managing Vendors & Preferred Sellers
- Query favorite vendors or filter by category:
  ```bash
  uv run ierp vendors [--category "Motorbike Rental"] [--favorite]
  ```
- Insert a new vendor/service provider:
  ```bash
  uv run ierp insert-vendor \
    --name "Vendor Name" \
    --category "Motorbike Rental" \
    --location "Bali, Indonesia" \
    --phone "+62 851-..." \
    --favorite
  ```

### 8. Location Tracking & History ($0 API Cost)
- **Live Background Tracking (OwnTracks)**:
  - Start the dashboard: `uv run ierp dashboard`
  - In the OwnTracks mobile app, set Connection Mode to `HTTP` and Host/URL to `http://<LAN_OR_TAILSCALE_IP>:8000/api/webhook/location`.
  - Pings automatically reverse-geocode using OpenStreetMap Nominatim ($0) and log visits into `events` while auto-linking contacts.
- **Historical Google Maps Timeline Import**:
  - Ingest Google Maps Timeline JSON exports:
    ```bash
    uv run ierp import-timeline path/to/timeline.json
    ```

### 9. Running Tests & Web Dashboard
- To run the automated test suite:
  ```bash
  uv run ierp test
  ```
- To start the interactive web dashboard:
  ```bash
  uv run ierp dashboard [--port 8000] [--no-browser]
  ```

---

## 🛠️ CLI Usage Reference

Run commands using **`uv run ierp <command>`**:

- **Launch Web Dashboard**: `uv run ierp dashboard [--port N] [--no-browser]`
- **Sync Google Contacts**: `uv run ierp sync-contacts [--full]`
- **Auto-Merge Duplicate Contacts**: `uv run ierp merge-contacts --auto`
- **Manual Contact Merge**: `uv run ierp merge-contacts --source-id <id> --target-id <id>`
- **Run Automated Test Suite**: `uv run ierp test`
- **Import Google Maps Timeline**: `uv run ierp import-timeline <timeline_export.json>`
- **Import Notion Export**: `uv run ierp import <notion_export_dir>`
- **Import Notion CRM**: `uv run ierp import-crm <crm_export_dir>`
- **List Events**: `uv run ierp list [--limit N]`
- **Show Event Details**: `uv run ierp show <event_id>`
- **List Contacts**: `uv run ierp contacts [--source manual|google|merged|all]`
- **Show Contact Details**: `uv run ierp show-contact <contact_id>`
- **List Vendors / Sellers**: `uv run ierp vendors [--category <cat>] [--favorite]`
- **Show Vendor Details**: `uv run ierp show-vendor <vendor_id>`
- **Insert Vendor / Seller**: `uv run ierp insert-vendor --name "<name>" [--category "<cat>"] [--location "<loc>"] [--phone "<phone>"] [--favorite]`
- **Search Events**: `uv run ierp search "<keyword>"`
- **Link Database**: `uv run ierp link`
