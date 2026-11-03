# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "google-cloud-pubsub>=2.30.0",
#     "google-api-python-client>=2.150.0",
#     "google-auth>=2.36.0",
#     "google-auth-httplib2>=0.2.0",
#     "requests>=2.32.0",
# ]
# ///
"""
Gmail Expense Tracker - Pub/Sub Worker Listener.
Uses Application Default Credentials (ADC) - No credentials.json required!
"""

import os
import sys
import json
import base64
import re
from datetime import datetime, timezone
from pathlib import Path
from google.cloud import pubsub_v1
import google.auth
from google.auth.transport.requests import Request
from googleapiclient.discovery import build
import requests

PROJECT_ID = os.environ.get("GCP_PROJECT", "ichsanul-dev")
SUBSCRIPTION_NAME = "gmail-expenses-sub"
SUBSCRIPTION_PATH = f"projects/{PROJECT_ID}/subscriptions/{SUBSCRIPTION_NAME}"

TURSO_URL = os.environ.get("TURSO_DATABASE_URL", "").replace("libsql://", "https://")
TURSO_TOKEN = os.environ.get("TURSO_AUTH_TOKEN", "")

SCOPES = [
    "https://www.googleapis.com/auth/gmail.readonly",
    "https://www.googleapis.com/auth/cloud-platform",
]

_GMAIL_SERVICE = None

def get_gmail_service():
    global _GMAIL_SERVICE
    if _GMAIL_SERVICE is None:
        try:
            creds, _ = google.auth.default(scopes=SCOPES)
            if not creds.valid:
                creds.refresh(Request())
            _GMAIL_SERVICE = build("gmail", "v1", credentials=creds)
        except Exception as e:
            print(f"[WARN] Could not initialize Gmail service: {e}")
    return _GMAIL_SERVICE

def save_to_turso(record: dict) -> bool:
    """Inserts an expense record into the dedicated email_expenses table in Turso."""
    if not TURSO_URL or not TURSO_TOKEN:
        print("[WARN] TURSO_DATABASE_URL or TURSO_AUTH_TOKEN not set. Skipping Turso save.")
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
        category=excluded.category;
    """

    now_iso = datetime.now(timezone.utc).isoformat()
    args = [
        {"type": "text", "value": str(record["id"])},
        {"type": "text", "value": str(record.get("source", "gmail"))},
        {"type": "text", "value": str(record["message_id"])},
        {"type": "text", "value": str(record.get("sender", ""))},
        {"type": "text", "value": str(record.get("subject", ""))},
        {"type": "text", "value": str(record.get("received_at", now_iso))},
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
        res_data = res.json()
        if "results" in res_data and res_data["results"][0].get("type") == "ok":
            print(f"✅ Saved to Turso (email_expenses): {record.get('merchant')} - {record.get('currency')} {record.get('amount')}")
            return True
        else:
            print(f"❌ Turso error: {res_data}")
            return False
    except Exception as e:
        print(f"❌ Failed to reach Turso: {e}")
        return False

def parse_expense_snippet(text: str, subject: str = "", sender: str = "") -> dict:
    """Extracts amount and merchant heuristic from receipt text/subject."""
    amount = 0.0
    currency = "IDR"
    
    amount_match = re.search(r"(?:rp|idr)\.?\s*([0-9]{1,3}(?:[.,][0-9]{3})*(?:[.,][0-9]{2})?)", text + " " + subject, re.IGNORECASE)
    if amount_match:
        raw_val = amount_match.group(1).replace(".", "").replace(",", ".")
        try:
            amount = float(raw_val)
        except ValueError:
            amount = 0.0

    # Merchant heuristic
    merchant = "Unknown"
    for brand in ["GoPay", "ShopeePay", "Shopee", "Tokopedia", "Grab", "Gojek", "BCA", "Mandiri", "Jenius", "Bank Jago", "Steam", "Spotify", "Netflix", "Google Play"]:
        if brand.lower() in (sender + " " + subject + " " + text).lower():
            merchant = brand
            break

    # Category heuristic
    category = "General"
    combined = (subject + " " + text).lower()
    if any(k in combined for k in ["kopi", "makan", "resto", "food", "kitchen", "cafe"]):
        category = "Food & Beverage"
    elif any(k in combined for k in ["ride", "car", "transit", "tol", "bensin", "parkir"]):
        category = "Transportation"
    elif any(k in combined for k in ["game", "steam", "spotify", "netflix", "youtube"]):
        category = "Entertainment"
    elif any(k in combined for k in ["tokopedia", "shopee", "blibli"]):
        category = "Shopping"

    return {
        "amount": amount,
        "currency": currency,
        "merchant": merchant,
        "category": category,
        "payment_method": merchant if merchant in ["GoPay", "ShopeePay", "BCA", "Jenius", "Bank Jago"] else "Electronic",
    }

def process_message(message: pubsub_v1.subscriber.message.Message):
    """Callback invoked by Pub/Sub subscriber when a message arrives."""
    try:
        raw_data = message.data.decode("utf-8")
        print(f"\n📩 [Pub/Sub Event Received] ID: {message.message_id}")
        data = json.loads(raw_data)

        if "mock_expense" in data:
            item = data["mock_expense"]
            parsed = parse_expense_snippet(item.get("snippet", ""), item.get("subject", ""), item.get("sender", ""))
            record = {
                "id": f"mock-{message.message_id}",
                "source": "mock_pubsub",
                "message_id": f"msg-{message.message_id}",
                "sender": item.get("sender", "service@gopay.co.id"),
                "subject": item.get("subject", "Pembayaran Berhasil"),
                "received_at": datetime.now(timezone.utc).isoformat(),
                "amount": item.get("amount", parsed["amount"]),
                "currency": item.get("currency", parsed["currency"]),
                "merchant": item.get("merchant", parsed["merchant"]),
                "category": item.get("category", parsed["category"]),
                "payment_method": item.get("payment_method", parsed["payment_method"]),
                "raw_snippet": item.get("snippet", "")
            }
            save_to_turso(record)
        elif "emailAddress" in data and "historyId" in data:
            print(f"🔔 Gmail Watch Notification for {data['emailAddress']} (historyId: {data['historyId']})")
            
            # Fetch message details if Gmail API service is authenticated
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
                    
                    print(f"📬 Found {len(messages)} new message(s) in inbox history.")
                    for msg in messages:
                        full_msg = service.users().messages().get(userId="me", id=msg["id"], format="snippet").execute()
                        headers = {h["name"].lower(): h["value"] for h in full_msg.get("payload", {}).get("headers", [])}
                        subject = headers.get("subject", "")
                        sender = headers.get("from", "")
                        snippet = full_msg.get("snippet", "")
                        
                        parsed = parse_expense_snippet(snippet, subject, sender)
                        record = {
                            "id": f"gmail-{msg['id']}",
                            "source": "gmail",
                            "message_id": msg["id"],
                            "sender": sender,
                            "subject": subject,
                            "received_at": datetime.now(timezone.utc).isoformat(),
                            "amount": parsed["amount"],
                            "currency": parsed["currency"],
                            "merchant": parsed["merchant"],
                            "category": parsed["category"],
                            "payment_method": parsed["payment_method"],
                            "raw_snippet": snippet
                        }
                        save_to_turso(record)
                except Exception as gerr:
                    print(f"⚠️ Error fetching Gmail history: {gerr}")
            else:
                record = {
                    "id": f"gmail-{data['historyId']}",
                    "source": "gmail_push",
                    "message_id": f"history-{data['historyId']}",
                    "sender": data["emailAddress"],
                    "subject": f"Gmail Inbox Update (History: {data['historyId']})",
                    "received_at": datetime.now(timezone.utc).isoformat(),
                    "amount": 0.0,
                    "currency": "IDR",
                    "merchant": "Gmail Push",
                    "category": "Notification",
                    "payment_method": "Pub/Sub",
                    "raw_snippet": json.dumps(data)
                }
                save_to_turso(record)
        else:
            print(f"ℹ️ Unhandled payload format: {data}")

        message.ack()
        print(f"✔️ Message {message.message_id} ACKed successfully.")
    except Exception as err:
        print(f"❌ Error processing message: {err}")
        message.nack()

def main():
    print(f"🚀 Starting Gmail Expense Pub/Sub Listener (ADC Mode)...")
    print(f"📌 Subscription: {SUBSCRIPTION_PATH}")
    print(f"🗄️ Turso Database: {TURSO_URL or 'Local Mock'}")

    subscriber = pubsub_v1.SubscriberClient()
    streaming_pull_future = subscriber.subscribe(SUBSCRIPTION_PATH, callback=process_message)
    print("👂 Listening for messages... (Press Ctrl+C to stop)")

    with subscriber:
        try:
            streaming_pull_future.result()
        except KeyboardInterrupt:
            print("\nStopping listener...")
            streaming_pull_future.cancel()
            streaming_pull_future.result()
        except Exception as e:
            print(f"Subscriber error: {e}")
            streaming_pull_future.cancel()

if __name__ == "__main__":
    main()
