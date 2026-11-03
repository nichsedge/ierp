import os
import sys
import json
import base64
import re
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
    category = "General"

    # Krom Bank QRIS
    if "krom.id" in sender.lower() and "qris" in subject.lower():
        payment_method = "QRIS (Krom)"
        for line in lines:
            if line.startswith("Merchant:"):
                merchant = line.replace("Merchant:", "").strip().split(",")[0].strip()
            elif line.startswith("Jumlah:"):
                m = re.search(r"Rp\s*([0-9.,]+)", line)
                if m:
                    amount = float(m.group(1).replace(".", "").replace(",", "."))
        if any(f in merchant.lower() for f in ["resto", "cafe", "makan", "warung", "jambak", "kopi", "esb"]):
            category = "Food & Beverage"

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

    if amount > 0:
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
            "raw_snippet": snippet[:500]
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
        amount, currency, merchant, category, payment_method, raw_snippet
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(message_id) DO UPDATE SET
        amount=excluded.amount,
        merchant=excluded.merchant,
        category=excluded.category,
        payment_method=excluded.payment_method;
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
        {"type": "text", "value": str(record.get("category", "General"))},
        {"type": "text", "value": str(record.get("payment_method", "Unknown"))},
        {"type": "text", "value": str(record.get("raw_snippet", "")[:500])}
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
    def do_GET(self):
        if self.path == "/renew-watch":
            service = get_gmail_service()
            if not service:
                self.send_response(500)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps({"error": "Failed to init Gmail service"}).encode("utf-8"))
                return

            try:
                topic_name = f"projects/{os.environ.get('GCP_PROJECT', 'ichsanul-dev')}/topics/gmail-expenses"
                request_body = {
                    "topicName": topic_name,
                    "labelIds": ["INBOX"]
                }
                res = service.users().watch(userId="me", body=request_body).execute()
                print(f"[RENEWAL] Watch renewed successfully! HistoryId: {res.get('historyId')}, Expiration: {res.get('expiration')}")
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps({
                    "status": "renewed",
                    "historyId": res.get("historyId"),
                    "expiration": res.get("expiration")
                }).encode("utf-8"))
            except Exception as e:
                print(f"[RENEWAL ERROR] {e}")
                self.send_response(500)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps({"error": str(e)}).encode("utf-8"))
            return

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps({"status": "ok", "service": "gmail-expenses-cloud-run"}).encode("utf-8"))

    def do_POST(self):
        content_length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(content_length)

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

                            print(f"📬 Processing {len(messages)} new message(s) from history.")
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

            # Return 200 OK so Pub/Sub marks the push delivered
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"status": "acknowledged"}).encode("utf-8"))
        except Exception as e:
            print(f"❌ Error handling push: {e}")
            self.send_response(500)
            self.end_headers()

def run():
    print(f"🚀 Cloud Run Gmail Expense Service listening on port {PORT}...")
    server = HTTPServer(("0.0.0.0", PORT), RequestHandler)
    server.serve_forever()

if __name__ == "__main__":
    run()
