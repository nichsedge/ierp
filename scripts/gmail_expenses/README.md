# Gmail Expense Sync via Cloud Pub/Sub & Turso Cloud

Real-time, zero-cost expense tracking pipeline connecting Gmail Push Notifications, Google Cloud Pub/Sub, and Turso Cloud (`email_expenses` table in `events.db`).

## 🏗️ Architecture

```
Gmail Inbox ──(push notification)──► Cloud Pub/Sub Topic (`gmail-expenses`)
                                              │
                                              ▼
                             Push Subscription (`gmail-expenses-push-sub`)
                                              │
                                              ▼
                             Cloud Run (`gmail-expenses-worker` in us-central1)
                                   (Scale-to-Zero, 100% Free Tier)
                                              │
                                              ▼
                                   Turso Cloud: `email_expenses` table
```

## 🔐 Zero-JSON Authentication (1-Click Desktop OAuth)

Tidak perlu download file `credentials.json` atau repot copy-paste JSON dari GCP Console. Cukup login sekali di browser menggunakan Google Application Default Credentials (ADC):

```bash
gcloud auth application-default login --scopes='https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/gmail.readonly'
```

Browser akan terbuka otomatis, pilih akun Google kamu (`muhammad.ichsanul19@gmail.com`), klik **Allow**. Semua token OAuth otomatis tersimpan di sistem.

## 🚀 Running the Pipeline

### 1. Aktifkan Gmail Watch
Daftarkan webhook inbox Gmail kamu ke Cloud Pub/Sub:
```bash
uv run ~/Projects/ierp/scripts/gmail_expenses/watch.py
```

### 2. Jalankan Listener Worker
Tarik notifikasi secara real-time dan simpan struk ke Turso:
```bash
uv run ~/Projects/ierp/scripts/gmail_expenses/worker.py
```

### 3. Test Mock Pipeline (Simulasi)
Untuk menguji coba tanpa menunggu email sungguhan:
```bash
uv run ~/Projects/ierp/scripts/gmail_expenses/test_pipeline.py
```
