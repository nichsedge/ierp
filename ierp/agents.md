# Agent Guidelines for Personal Event Log & CRM (erp)

This directory contains a localized database and CLI system for tracking personal journal events and CRM contacts. Future agents interacting with this system should adhere to the guidelines below.

---

## 📂 System Architecture & Modules

The ERP is modularized under `erp/core/` while preserving single-command execution through `manage_events.py`:

- **CLI Entrypoint**: [manage_events.py](file:///home/al/Projects/creds/erp/manage_events.py)
- **Core Modules**:
  - `erp/core/config.py`: Central paths (`events.db`, `events_media`), constants, ANSI colors.
  - `erp/core/db.py`: SQLite `WAL` mode connection manager, schema migrations, and performance indexes.
  - `erp/core/merging.py`: Contact deduplication, note merging, and relationship preservation engine.
  - `erp/core/google_sync.py`: Google People API OAuth 2.0 client and incremental delta sync (`syncToken`).
  - `erp/core/geocoding.py`: OpenStreetMap Nominatim reverse-geocoder with rate-limiting and persistent disk caching.
  - `erp/core/importers.py`: Parsers for Notion CSV/Markdown exports & Google Maps Semantic Location History JSON.
  - `erp/core/dashboard.py`: Single-page interactive web dashboard, JSON REST API, and OwnTracks GPS webhook receiver.
- **Automated Tests**: [erp/tests/test_erp.py](file:///home/al/Projects/creds/erp/tests/test_erp.py)
- **Database**: `erp/events.db` (SQLite with `WAL` journaling mode, git-ignored)
- **Media Attachments**: `erp/events_media/` (Local folder, git-ignored)

---

## 🗄️ Database Schema & Concurrency

- SQLite operates in **Write-Ahead Logging (`WAL`)** mode with `busy_timeout = 5000ms` and `foreign_keys = ON`.
- Includes performance indexes on `events(start_date)`, `events(place)`, `contacts(name)`, `contacts(email)`, `contacts(google_id)`, `contacts(source)`, `event_contacts(event_id, contact_id)`, `vendors(name)`, `vendors(category)`, and `vendors(favorite)`.

---

## 🤖 Guidelines for AI Agents

### 1. Zero External Dependencies
- The system is built strictly using Python standard library modules (`urllib.request`, `http.server`, `sqlite3`, `json`, `socket`, `webbrowser`, `unittest`). Do not introduce external dependencies (`requests`, `pandas`, `pydantic`) without user confirmation.

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
- Parse the unstructured log in your chat memory (resolve dates to ISO `YYYY-MM-DD`, locations, tags, and notes), and then insert it directly by executing:
  ```bash
  python3 erp/manage_events.py insert \
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
  python3 erp/manage_events.py sync-contacts [--full] [--credentials path/to/credentials.json]
  ```
- **Merging Duplicate Contacts**:
  - Auto-merge duplicates across manual & Google synced contacts:
    ```bash
    python3 erp/manage_events.py merge-contacts --auto
    ```
  - Manually merge specific contacts:
    ```bash
    python3 erp/manage_events.py merge-contacts --source-id <id_to_remove> --target-id <id_to_keep>
    ```

### 7. Managing Vendors & Preferred Sellers
- Query favorite vendors or filter by category:
  ```bash
  python3 erp/manage_events.py vendors [--category "Motorbike Rental"] [--favorite]
  ```
- Insert a new vendor/service provider:
  ```bash
  python3 erp/manage_events.py insert-vendor \
    --name "Vendor Name" \
    --category "Motorbike Rental" \
    --location "Bali, Indonesia" \
    --phone "+62 851-..." \
    --favorite
  ```

### 8. Location Tracking & History ($0 API Cost)
- **Live Background Tracking (OwnTracks)**:
  - Start the dashboard: `python3 erp/manage_events.py dashboard`
  - In the OwnTracks mobile app, set Connection Mode to `HTTP` and Host/URL to `http://<LAN_OR_TAILSCALE_IP>:8000/api/webhook/location`.
  - Pings automatically reverse-geocode using OpenStreetMap Nominatim ($0) and log visits into `events` while auto-linking contacts.
- **Historical Google Maps Timeline Import**:
  - Ingest Google Maps Timeline JSON exports:
    ```bash
    python3 erp/manage_events.py import-timeline path/to/timeline.json
    ```

### 9. Running Tests & Web Dashboard
- To run the automated test suite:
  ```bash
  python3 erp/manage_events.py test
  ```
- To start the interactive web dashboard:
  ```bash
  python3 erp/manage_events.py dashboard [--port 8000] [--no-browser]
  ```

---

## 🛠️ CLI Usage Reference (with `uv` or direct Python)

You can run commands using **`uv run erp <command>`** or **`python3 erp/manage_events.py <command>`**:

- **Launch Web Dashboard**: `uv run erp dashboard [--port N] [--no-browser]`
- **Sync Google Contacts**: `uv run erp sync-contacts [--full]`
- **Auto-Merge Duplicate Contacts**: `uv run erp merge-contacts --auto`
- **Manual Contact Merge**: `uv run erp merge-contacts --source-id <id> --target-id <id>`
- **Run Automated Test Suite**: `uv run erp test`
- **Import Google Maps Timeline**: `uv run erp import-timeline <timeline_export.json>`
- **Import Notion Export**: `uv run erp import <notion_export_dir>`
- **List Events**: `uv run erp list [--limit N]`
- **Show Event Details**: `uv run erp show <event_id>`
- **List Contacts**: `uv run erp contacts [--source manual|google|merged|all]`
- **Show Contact Details**: `uv run erp show-contact <contact_id>`
- **List Vendors / Sellers**: `uv run erp vendors [--category <cat>] [--favorite]`
- **Show Vendor Details**: `uv run erp show-vendor <vendor_id>`
- **Insert Vendor / Seller**: `uv run erp insert-vendor --name "<name>" [--category "<cat>"] [--location "<loc>"] [--phone "<phone>"] [--favorite]`
- **Search Events**: `uv run erp search "<keyword>"`
- **Link Database**: `uv run erp link`



