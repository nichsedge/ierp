# Gmail Expense Sync via Cloud Pub/Sub & Turso Cloud

Real-time, zero-cost expense tracking pipeline connecting Gmail Push Notifications, Google Cloud Pub/Sub, and Turso Cloud (`email_expenses` table in `events.db`).

## 🏗️ Architecture

```
Gmail Inbox ──(push notification)──► Cloud Pub/Sub Topic (`gmail-expenses`)
                                              │
                                              ▼
                                 Pub/Sub Subscription (`gmail-expenses-sub`)
                                              │
                                              ▼
                                       `worker.py` (Local/Cloud Run)
                                              │
                                              ▼
                                   Turso Cloud: `email_expenses` table
```

## 🗄️ Database Schema

Data is isolated in its own dedicated table in Turso Cloud (`events.db`), preventing interference with existing ERP models:

```sql
CREATE TABLE IF NOT EXISTS email_expenses (
    id TEXT PRIMARY KEY,
    source TEXT DEFAULT 'gmail',
    message_id TEXT UNIQUE,
    sender TEXT,
    subject TEXT,
    received_at TEXT,
    amount REAL,
    currency TEXT DEFAULT 'IDR',
    merchant TEXT,
    category TEXT,
    payment_method TEXT,
    raw_snippet TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

## 🚀 Running the Pipeline

### 1. Test Pipeline End-to-End (Simulation)
Publish a test event to Cloud Pub/Sub:
```bash
uv run ~/Projects/ierp/scripts/gmail_expenses/test_pipeline.py
```

### 2. Run the Worker Listener
Start the worker to pull messages from Pub/Sub, parse receipts, and write to Turso:
```bash
uv run ~/Projects/ierp/scripts/gmail_expenses/worker.py
```

### 3. Connect Real Gmail Inbox
1. Download OAuth 2.0 Client credentials (`credentials.json`) from GCP Console (APIs & Services > Credentials > Desktop App).
2. Save to `~/Projects/ierp/scripts/gmail_expenses/credentials.json`.
3. Activate watch:
```bash
uv run ~/Projects/ierp/scripts/gmail_expenses/watch.py
```
*(Note: Gmail watch expires after 7 days; schedule `watch.py` weekly via cron).*
