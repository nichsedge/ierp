import os
import sys
import json
import base64
import re
import time
import hashlib
import hmac
import urllib.request
import urllib.error
from datetime import datetime, timezone
from http.server import HTTPServer, BaseHTTPRequestHandler
from google.oauth2.credentials import Credentials
from google.auth.transport.requests import Request
from googleapiclient.discovery import build
import requests

PORT = int(os.environ.get("PORT", "8080"))

TURSO_URL = os.environ.get("TURSO_DATABASE_URL", "").replace("libsql://", "https://")
TURSO_TOKEN = os.environ.get("TURSO_AUTH_TOKEN", "")

CLIENT_ID = os.environ.get("GOOGLE_CLIENT_ID", "")
CLIENT_SECRET = os.environ.get("GOOGLE_CLIENT_SECRET", "")
REFRESH_TOKEN = os.environ.get("GOOGLE_REFRESH_TOKEN", "")

R2_ACCOUNT_ID = os.environ.get("R2_ACCOUNT_ID", "")
R2_ACCESS_KEY_ID = os.environ.get("R2_ACCESS_KEY_ID", "")
R2_SECRET_ACCESS_KEY = os.environ.get("R2_SECRET_ACCESS_KEY", "")
R2_BUCKET_NAME = os.environ.get("R2_BUCKET_NAME", "ichsanul-dev")

SYNC_AUTH_KEY = os.environ.get("SYNC_AUTH_KEY", "")

SCOPES = ["https://www.googleapis.com/auth/gmail.readonly"]

def get_gmail_service():
    if not (CLIENT_ID and CLIENT_SECRET and REFRESH_TOKEN):
        print("[ERROR] Missing Google OAuth credentials in environment!")
        return None
    creds = Credentials(
        token=None,
        refresh_token=REFRESH_TOKEN,
        token_uri="https://oauth2.googleapis.com/token",
        client_id=CLIENT_ID,
        client_secret=CLIENT_SECRET,
        scopes=SCOPES
    )
    creds.refresh(Request())
    return build("gmail", "v1", credentials=creds, cache_discovery=False)

def get_body(payload: dict) -> str:
    if "data" in payload.get("body", {}):
        return base64.urlsafe_b64decode(payload["body"]["data"]).decode("utf-8", errors="ignore")
    for part in payload.get("parts", []):
        text = get_body(part)
        if text:
            return text
    return ""

def build_sigv4_headers(method: str, canonical_uri: str, payload_bytes: bytes, content_type: str = ""):
    host = f"{R2_ACCOUNT_ID}.r2.cloudflarestorage.com"
    now = datetime.now(timezone.utc)
    amz_date = now.strftime('%Y%m%dT%H%M%SZ')
    date_stamp = now.strftime('%Y%m%d')
    payload_hash = hashlib.sha256(payload_bytes).hexdigest()

    headers = {
        "host": host,
        "x-amz-content-sha256": payload_hash,
        "x-amz-date": amz_date
    }
    if content_type:
        headers["content-type"] = content_type

    canonical_headers = "".join([f"{k}:{v}\n" for k, v in sorted(headers.items())])
    signed_headers = ";".join(sorted(headers.keys()))
    canonical_request = f"{method}\n{canonical_uri}\n\n{canonical_headers}\n{signed_headers}\n{payload_hash}"
    credential_scope = f"{date_stamp}/auto/s3/aws4_request"
    string_to_sign = f"AWS4-HMAC-SHA256\n{amz_date}\n{credential_scope}\n{hashlib.sha256(canonical_request.encode('utf-8')).hexdigest()}"

    def sign(key, msg):
        return hmac.new(key, msg.encode('utf-8'), hashlib.sha256).digest()

    k_date = sign(("AWS4" + R2_SECRET_ACCESS_KEY).encode('utf-8'), date_stamp)
    k_region = sign(k_date, "auto")
    k_service = sign(k_region, "s3")
    k_signing = sign(k_service, "aws4_request")
    signature = hmac.new(k_signing, string_to_sign.encode('utf-8'), hashlib.sha256).hexdigest()

    auth_header = f"AWS4-HMAC-SHA256 Credential={R2_ACCESS_KEY_ID}/{credential_scope}, SignedHeaders={signed_headers}, Signature={signature}"
    headers["Authorization"] = auth_header
    return headers

def r2_put_object(key: str, data: bytes, content_type="application/x-sqlite3") -> bool:
    canonical_uri = f"/{R2_BUCKET_NAME}/{key}"
    headers = build_sigv4_headers("PUT", canonical_uri, data, content_type)
    url = f"https://{headers['host']}{canonical_uri}"
    req = urllib.request.Request(url, data=data, headers=headers, method="PUT")
    try:
        with urllib.request.urlopen(req, timeout=30) as res:
            return res.status in (200, 201)
    except Exception as e:
        print(f"❌ R2 PUT error ({key}): {e}")
        return False

def r2_get_object(key: str) -> bytes | None:
    canonical_uri = f"/{R2_BUCKET_NAME}/{key}"
    headers = build_sigv4_headers("GET", canonical_uri, b"")
    url = f"https://{headers['host']}{canonical_uri}"
    req = urllib.request.Request(url, headers=headers, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=30) as res:
            return res.read()
    except Exception as e:
        print(f"❌ R2 GET error ({key}): {e}")
        return None

RULES_CACHE: list[dict] = []
RULES_CACHE_TIME: float = 0.0

def get_merchant_rules() -> list[dict]:
    global RULES_CACHE, RULES_CACHE_TIME
    now = time.time()
    if RULES_CACHE and (now - RULES_CACHE_TIME) < 300:
        return RULES_CACHE
    if not (TURSO_URL and TURSO_TOKEN):
        return []
    try:
        pipeline_url = f"{TURSO_URL}/v2/pipeline"
        headers = {"Authorization": f"Bearer {TURSO_TOKEN}", "Content-Type": "application/json"}
        sql = "SELECT pattern, clean_merchant, category FROM merchant_rules ORDER BY length(pattern) DESC;"
        payload = {"requests": [{"type": "execute", "stmt": {"sql": sql}}]}
        res = requests.post(pipeline_url, json=payload, headers=headers, timeout=10)
        data = res.json()
        exec_res = data.get("results", [{}])[0].get("response", {}).get("result", {})
        rules = []
        for row in exec_res.get("rows", []):
            pat = row[0].get("value", "").lower() if row[0] else ""
            clean = row[1].get("value") if row[1] and row[1].get("value") else None
            cat = row[2].get("value", "Misc") if row[2] else "Misc"
            if pat:
                rules.append({"pattern": pat, "clean_merchant": clean, "category": cat})
        RULES_CACHE = rules
        RULES_CACHE_TIME = now
        return rules
    except Exception as e:
        print(f"⚠️ Error fetching merchant rules from Turso: {e}")
        return RULES_CACHE

def apply_merchant_rule(merchant: str, text: str = "") -> tuple[str, str]:
    rules = get_merchant_rules()
    haystack = f"{merchant} {text}".lower()
    for r in rules:
        if r["pattern"] in haystack:
            clean = r["clean_merchant"] or merchant
            return clean, r["category"]
    return merchant, "Misc"

def add_merchant_rule(pattern: str, clean_merchant: str | None, category: str, notes: str = "") -> bool:
    global RULES_CACHE, RULES_CACHE_TIME
    if not (TURSO_URL and TURSO_TOKEN):
        return False
    pipeline_url = f"{TURSO_URL}/v2/pipeline"
    headers = {"Authorization": f"Bearer {TURSO_TOKEN}", "Content-Type": "application/json"}
    sql = """
    INSERT INTO merchant_rules (pattern, clean_merchant, category, notes)
    VALUES (?, ?, ?, ?)
    ON CONFLICT(pattern) DO UPDATE SET
        clean_merchant=excluded.clean_merchant,
        category=excluded.category,
        notes=excluded.notes;
    """
    args = [
        {"type": "text", "value": pattern.lower()},
        {"type": "text", "value": clean_merchant} if clean_merchant else {"type": "null"},
        {"type": "text", "value": category},
        {"type": "text", "value": notes}
    ]
    payload = {"requests": [{"type": "execute", "stmt": {"sql": sql, "args": args}}]}
    try:
        res = requests.post(pipeline_url, json=payload, headers=headers, timeout=10)
        RULES_CACHE = []
        RULES_CACHE_TIME = 0.0
        return res.status_code == 200
    except Exception as e:
        print(f"❌ Error adding merchant rule: {e}")
        return False

def query_turso_inbox() -> list[dict]:
    if not (TURSO_URL and TURSO_TOKEN):
        return []
    pipeline_url = f"{TURSO_URL}/v2/pipeline"
    headers = {"Authorization": f"Bearer {TURSO_TOKEN}", "Content-Type": "application/json"}
    sql = """
    SELECT id, source, message_id, merchant, amount, currency, category, payment_method, received_at, type
    FROM email_expenses
    WHERE synced_at IS NULL
    ORDER BY received_at DESC
    LIMIT 100;
    """
    payload = {"requests": [{"type": "execute", "stmt": {"sql": sql}}]}
    try:
        res = requests.post(pipeline_url, json=payload, headers=headers, timeout=10)
        data = res.json()
        exec_res = data.get("results", [{}])[0].get("response", {}).get("result", {})
        cols = [c["name"] for c in exec_res.get("cols", [])]
        items = []
        for row in exec_res.get("rows", []):
            item = {}
            for idx, cell in enumerate(row):
                item[cols[idx]] = cell.get("value")
            item["type"] = item.get("type") or "EXPENSE"
            # Apply dynamic merchant rules in case they were updated after ingestion
            clean_m, detected_cat = apply_merchant_rule(item.get("merchant", ""), "")
            if clean_m and clean_m != item.get("merchant"):
                item["merchant"] = clean_m
            if item.get("category") in ("Misc", "General", None) and detected_cat != "Misc":
                item["category"] = detected_cat
            items.append(item)
        return items
    except Exception as e:
        print(f"❌ Turso query error: {e}")
        return []

def acknowledge_turso_inbox(ids: list[str]) -> int:
    if not (TURSO_URL and TURSO_TOKEN) or not ids:
        return 0
    pipeline_url = f"{TURSO_URL}/v2/pipeline"
    headers = {"Authorization": f"Bearer {TURSO_TOKEN}", "Content-Type": "application/json"}
    placeholders = ", ".join(["?"] * len(ids))
    sql = f"UPDATE email_expenses SET synced_at = CURRENT_TIMESTAMP WHERE id IN ({placeholders});"
    args = [{"type": "text", "value": str(i)} for i in ids]
    payload = {"requests": [{"type": "execute", "stmt": {"sql": sql, "args": args}}]}
    try:
        res = requests.post(pipeline_url, json=payload, headers=headers, timeout=10)
        data = res.json()
        return data.get("results", [{}])[0].get("response", {}).get("result", {}).get("affected_row_count", 0)
    except Exception as e:
        print(f"❌ Turso ack error: {e}")
        return 0

def parse_financial_transaction(msg_id: str, headers: dict, raw_body: str, snippet: str) -> dict | None:
    subject = headers.get("subject", "")
    sender = headers.get("from", "")
    date_str = headers.get("date", "")
    
    clean_text = re.sub(r"<[^>]+>", "\n", raw_body)
    lines = [l.strip() for l in clean_text.split("\n") if l.strip()]
    full_text = " ".join(lines)
    
    ignore_patterns = [
        "statement of account", "laporan bulanan", "limit paylater", "selamat! limit",
        "dibatalkan", "udah cair", "deposito", "hore, kamu dapat", "sale", "promo",
        "new app linked", "terima kasih telah mendaftar"
    ]
    if any(p in subject.lower() or p in snippet.lower() for p in ignore_patterns):
        return None

    amount = 0.0
    merchant = "Unknown"
    payment_method = "Unknown"
    category = "Misc"
    tx_type = "EXPENSE"

    # Bareksa SBN Kupon / Imbal Hasil (Income)
    if "bareksa.com" in sender.lower() and ("kupon" in subject.lower() or "imbal hasil" in subject.lower()):
        tx_type = "INCOME"
        category = "Investments"
        payment_method = "Bareksa"

        seri_match = re.search(r"(?:seri|kupon)\s+([A-Z0-9]+)", subject + " " + full_text, re.IGNORECASE)
        seri = seri_match.group(1).upper() if seri_match else "SBN"
        merchant = f"Kupon SBN {seri}"

        m = re.search(r"(?:imbal hasil|nominal)\s*(?:rp|idr)?\s*([0-9.,]+)", full_text, re.IGNORECASE)
        if not m:
            m = re.search(r"Rp\s*([0-9.,]+)", snippet + " " + full_text)
        if m:
            amount = float(m.group(1).replace(".", "").replace(",", "."))

        if "cimb niaga" in full_text.lower():
            payment_method = "CIMB Niaga"
        elif "bca" in full_text.lower():
            payment_method = "BCA"
        elif "mandiri" in full_text.lower():
            payment_method = "Mandiri"

    # Krom Bank QRIS
    elif "krom.id" in sender.lower() and "qris" in subject.lower():
        payment_method = "QRIS (Krom)"
        for line in lines:
            if line.startswith("Merchant:"):
                merchant = line.replace("Merchant:", "").strip().split(",")[0].strip()
            elif line.startswith("Jumlah:"):
                m = re.search(r"Rp\s*([0-9.,]+)", line)
                if m:
                    amount = float(m.group(1).replace(".", "").replace(",", "."))
        if any(f in merchant.lower() for f in ["resto", "cafe", "makan", "warung", "jambak", "kopi", "esb"]):
            category = "Food"

    # Kredivo
    elif "kredivo" in sender.lower() or "kredivo" in subject.lower():
        payment_method = "Kredivo"
        m = re.search(r"Rp\s*([0-9.,]+)", snippet + " " + full_text)
        if m:
            amount = float(m.group(1).replace(".", "").replace(",", "."))
        if "shopee" in full_text.lower() or "shopee" in snippet.lower():
            merchant = "Shopee"
            category = "Shopping"
        elif "tokopedia" in full_text.lower():
            merchant = "Tokopedia"
            category = "Shopping"
        else:
            merchant = "Kredivo Merchant"

    # GOERS / Ticketing
    elif "goersapp.com" in sender.lower() and ("tiket" in subject.lower() or "berhasil" in subject.lower()):
        payment_method = "QRIS / Online"
        category = "Entertainment"
        if "taman mini" in subject.lower() or "tmii" in subject.lower():
            merchant = "Taman Mini Indonesia Indah (TMII)"
        else:
            merchant = "GOERS Ticketing"
        for idx, line in enumerate(lines):
            if line.strip().lower() == "total" and idx + 1 < len(lines):
                m = re.search(r"(?:IDR|Rp)\s*([0-9.,]+)", lines[idx+1])
                if m:
                    amount = float(m.group(1).replace(".", "").replace(",", "."))
                    break

    # Generic Fallback
    elif any(k in (sender + " " + subject).lower() for k in ["pembayaran berhasil", "transaksi berhasil", "payment receipt", "gopay", "shopeepay"]):
        m = re.search(r"(?:rp|idr)\.?\s*([0-9]{1,3}(?:[.,][0-9]{3})*(?:[.,][0-9]{2})?)", snippet + " " + full_text, re.IGNORECASE)
        if m:
            raw_val = m.group(1).replace(".", "").replace(",", ".")
            try:
                amount = float(raw_val)
            except ValueError:
                amount = 0.0
        for brand in ["GoPay", "ShopeePay", "Shopee", "Tokopedia", "Grab", "Gojek", "BCA", "Mandiri", "Jenius", "Bank Jago", "Steam", "Spotify", "Netflix"]:
            if brand.lower() in (sender + " " + subject).lower():
                merchant = brand
                payment_method = brand
                break
        combined = (subject + " " + snippet).lower()
        if any(f in combined for f in ["kopi", "makan", "resto", "food", "cafe"]):
            category = "Food"
        elif any(f in combined for f in ["ride", "car", "transit", "tol", "bensin", "parkir", "gojek", "grab"]):
            category = "Transport"
        elif any(f in combined for f in ["steam", "spotify", "netflix", "youtube", "game"]):
            category = "Entertainment"
        elif any(f in combined for f in ["tokopedia", "shopee", "blibli"]):
            category = "Shopping"

    if amount > 0:
        clean_m, detected_cat = apply_merchant_rule(merchant, f"{subject} {snippet}")
        if clean_m and clean_m != "Unknown":
            merchant = clean_m
        if detected_cat != "Misc":
            category = detected_cat

        return {
            "id": f"gmail-{msg_id}",
            "source": "gmail",
            "message_id": msg_id,
            "sender": sender,
            "subject": subject,
            "received_at": date_str or datetime.now(timezone.utc).isoformat(),
            "amount": amount,
            "currency": "IDR",
            "merchant": merchant,
            "category": category,
            "payment_method": payment_method,
            "raw_snippet": snippet[:500],
            "type": tx_type
        }
    return None

def save_to_turso(record: dict) -> bool:
    if not (TURSO_URL and TURSO_TOKEN):
        print("[WARN] TURSO_DATABASE_URL or TURSO_AUTH_TOKEN not set.")
        return False

    pipeline_url = f"{TURSO_URL}/v2/pipeline"
    headers = {
        "Authorization": f"Bearer {TURSO_TOKEN}",
        "Content-Type": "application/json"
    }

    sql = """
    INSERT INTO email_expenses (
        id, source, message_id, sender, subject, received_at,
        amount, currency, merchant, category, payment_method, raw_snippet, type
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(message_id) DO UPDATE SET
        amount=excluded.amount,
        merchant=excluded.merchant,
        category=excluded.category,
        payment_method=excluded.payment_method,
        type=excluded.type;
    """

    args = [
        {"type": "text", "value": str(record["id"])},
        {"type": "text", "value": str(record.get("source", "gmail"))},
        {"type": "text", "value": str(record["message_id"])},
        {"type": "text", "value": str(record.get("sender", ""))},
        {"type": "text", "value": str(record.get("subject", ""))},
        {"type": "text", "value": str(record.get("received_at", datetime.now(timezone.utc).isoformat()))},
        {"type": "float", "value": float(record.get("amount", 0.0))},
        {"type": "text", "value": str(record.get("currency", "IDR"))},
        {"type": "text", "value": str(record.get("merchant", "Unknown"))},
        {"type": "text", "value": str(record.get("category", "Misc"))},
        {"type": "text", "value": str(record.get("payment_method", "Unknown"))},
        {"type": "text", "value": str(record.get("raw_snippet", "")[:500])},
        {"type": "text", "value": str(record.get("type", "EXPENSE"))}
    ]

    payload = {"requests": [{"type": "execute", "stmt": {"sql": sql, "args": args}}]}
    try:
        res = requests.post(pipeline_url, json=payload, headers=headers, timeout=10)
        ok = "results" in res.json() and res.json()["results"][0].get("type") == "ok"
        if ok:
            print(f"✅ Committed to Turso: {record.get('merchant')} - {record.get('currency')} {record.get('amount')}")
        else:
            print(f"❌ Turso error: {res.json()}")
        return ok
    except Exception as e:
        print(f"❌ Turso error: {e}")
        return False

class RequestHandler(BaseHTTPRequestHandler):
    def send_json(self, data: dict, status: int = 200):
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Sync-Token")
        self.send_header("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, OPTIONS")
        self.end_headers()
        self.wfile.write(json.dumps(data, indent=2).encode("utf-8"))

    def is_auth(self) -> bool:
        if not SYNC_AUTH_KEY:
            return True
        auth = self.headers.get("Authorization", "")
        token = auth[7:].strip() if auth.startswith("Bearer ") else self.headers.get("X-Sync-Token", "")
        return token == SYNC_AUTH_KEY

    def do_OPTIONS(self):
        self.send_response(204)
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Sync-Token")
        self.send_header("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, OPTIONS")
        self.end_headers()

    def do_HEAD(self):
        if self.path == "/api/backup":
            if not self.is_auth():
                self.send_response(401)
                self.end_headers()
                return
            data = r2_get_object("db/sans_finance_latest.sqlite")
            if not data:
                self.send_response(404)
                self.end_headers()
                return
            self.send_response(200)
            self.send_header("Content-Type", "application/x-sqlite3")
            self.send_header("Content-Length", str(len(data)))
            self.send_header("Access-Control-Allow-Origin", "*")
            self.end_headers()
            return
        self.send_response(200)
        self.end_headers()

    def do_GET(self):
        # 1. Staging Inbox for Sans Finance
        if self.path == "/api/inbox":
            if not self.is_auth():
                return self.send_json({"error": "Unauthorized"}, 401)
            items = query_turso_inbox()
            return self.send_json({"success": True, "count": len(items), "items": items})

        # 2. Backup download (Restore)
        if self.path == "/api/backup":
            if not self.is_auth():
                return self.send_json({"error": "Unauthorized"}, 401)
            data = r2_get_object("db/sans_finance_latest.sqlite")
            if not data:
                return self.send_json({"error": "No backup found in R2"}, 404)
            self.send_response(200)
            self.send_header("Content-Type", "application/x-sqlite3")
            self.send_header("Content-Disposition", 'attachment; filename="sans_finance_latest.sqlite"')
            self.send_header("Content-Length", str(len(data)))
            self.send_header("Access-Control-Allow-Origin", "*")
            self.end_headers()
            self.wfile.write(data)
            return

        # 3. Canonical Categories
        if self.path == "/api/categories":
            return self.send_json({
                "categories": [
                    {"id": 1, "name": "Food", "icon": "restaurant"},
                    {"id": 2, "name": "Utility", "icon": "🏠"},
                    {"id": 3, "name": "Shopping", "icon": "shopping_bag"},
                    {"id": 4, "name": "Transport", "icon": "commute"},
                    {"id": 5, "name": "Subscriptions", "icon": "language"},
                    {"id": 6, "name": "Entertainment", "icon": "🍾"},
                    {"id": 8, "name": "Misc", "icon": "🔧"}
                ]
            })

        # 4. Merchant Rules
        if self.path == "/api/rules":
            if not self.is_auth():
                return self.send_json({"error": "Unauthorized"}, 401)
            rules = get_merchant_rules()
            return self.send_json({"success": True, "count": len(rules), "rules": rules})

        # 5. Gmail Renewal
        if self.path == "/renew-watch":
            service = get_gmail_service()
            if not service:
                return self.send_json({"error": "Failed to init Gmail service"}, 500)
            try:
                topic_name = f"projects/{os.environ.get('GCP_PROJECT', 'ichsanul-dev')}/topics/gmail-expenses"
                res = service.users().watch(userId="me", body={"topicName": topic_name, "labelIds": ["INBOX"]}).execute()
                print(f"[RENEWAL] Watch renewed successfully! HistoryId: {res.get('historyId')}")
                return self.send_json({"status": "renewed", "historyId": res.get("historyId"), "expiration": res.get("expiration")})
            except Exception as e:
                return self.send_json({"error": str(e)}, 500)

        # 5. Default Health Check
        self.send_json({"status": "ok", "service": "gmail-expenses-cloud-run"})

    def do_POST(self):
        content_length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(content_length)

        # 1. Sans Finance Inbox Ack
        if self.path == "/api/inbox/ack":
            if not self.is_auth():
                return self.send_json({"error": "Unauthorized"}, 401)
            try:
                payload = json.loads(body.decode("utf-8"))
                ids = payload.get("ids", [])
                updated = acknowledge_turso_inbox(ids)
                return self.send_json({"success": True, "acknowledged_count": updated, "ids": ids})
            except Exception as e:
                return self.send_json({"error": str(e)}, 400)

        # 2. Add/Update Merchant Rule
        if self.path == "/api/rules":
            if not self.is_auth():
                return self.send_json({"error": "Unauthorized"}, 401)
            try:
                payload = json.loads(body.decode("utf-8"))
                pat = payload.get("pattern", "").strip().lower()
                clean = payload.get("clean_merchant")
                cat = payload.get("category", "Misc").strip()
                notes = payload.get("notes", "")
                if not pat:
                    return self.send_json({"error": "pattern is required"}, 400)
                ok = add_merchant_rule(pat, clean, cat, notes)
                return self.send_json({"success": ok, "pattern": pat, "clean_merchant": clean, "category": cat})
            except Exception as e:
                return self.send_json({"error": str(e)}, 400)

        # 3. PubSub Push Webhook
        try:
            envelope = json.loads(body.decode("utf-8"))
            pubsub_message = envelope.get("message", {})

            if "data" in pubsub_message:
                raw_payload = base64.b64decode(pubsub_message["data"]).decode("utf-8")
                data = json.loads(raw_payload)
                print(f"📩 Pub/Sub Push received: {data}")

                if "emailAddress" in data and "historyId" in data:
                    service = get_gmail_service()
                    if service:
                        try:
                            history = service.users().history().list(
                                userId="me", startHistoryId=data["historyId"], historyTypes=["messageAdded"]
                            ).execute()
                            messages = []
                            for record_hist in history.get("history", []):
                                for msg_added in record_hist.get("messagesAdded", []):
                                    messages.append(msg_added["message"])

                            if not messages:
                                recent = service.users().messages().list(userId="me", maxResults=5).execute()
                                messages = recent.get("messages", [])

                            print(f"📬 Processing {len(messages)} new message(s).")
                            for msg in messages:
                                full_msg = service.users().messages().get(userId="me", id=msg["id"], format="full").execute()
                                payload = full_msg.get("payload", {})
                                headers = {h["name"].lower(): h["value"] for h in payload.get("headers", [])}
                                raw_msg_body = get_body(payload)
                                snippet = full_msg.get("snippet", "")

                                tx = parse_financial_transaction(msg["id"], headers, raw_msg_body, snippet)
                                if tx:
                                    save_to_turso(tx)
                                else:
                                    print(f"⏩ Non-financial email skipped: {headers.get('subject')}")
                        except Exception as e:
                            print(f"⚠️ Error querying Gmail API: {e}")

            return self.send_json({"status": "acknowledged"})
        except Exception as e:
            print(f"❌ Error handling push: {e}")
            self.send_response(500)
            self.end_headers()

    def do_PUT(self):
        # Backup upload from Sans Finance
        if self.path == "/api/backup":
            if not self.is_auth():
                return self.send_json({"error": "Unauthorized"}, 401)
            content_length = int(self.headers.get("Content-Length", 0))
            if content_length < 1024:
                return self.send_json({"error": "File too small or empty"}, 400)
            db_bytes = self.rfile.read(content_length)

            now_stamp = datetime.now(timezone.utc).strftime("%Y%m%d%H%M%S")
            archive_key = f"db/archive/sans_finance_{now_stamp}.sqlite"
            latest_key = "db/sans_finance_latest.sqlite"

            ok1 = r2_put_object(latest_key, db_bytes)
            ok2 = r2_put_object(archive_key, db_bytes)
            if ok1:
                return self.send_json({
                    "success": True,
                    "message": f"Backup saved to {latest_key} and {archive_key}",
                    "bytes": len(db_bytes)
                })
            return self.send_json({"error": "Failed to upload to R2"}, 500)

        self.send_json({"error": "Not found"}, 404)

def run():
    print(f"🚀 Cloud Run Gmail Expense Service listening on port {PORT}...")
    server = HTTPServer(("0.0.0.0", PORT), RequestHandler)
    server.serve_forever()

if __name__ == "__main__":
    run()
