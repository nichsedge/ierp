# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "google-api-python-client>=2.150.0",
#     "google-auth>=2.36.0",
#     "requests>=2.32.0",
# ]
# ///
"""
Scans Gmail for recent financial expense transactions,
parses merchant, amount, category, and payment method,
and synchronizes them into Turso Cloud (email_expenses table).
"""

import os
import sys
import base64
import re
from datetime import datetime, timezone
from pathlib import Path
from google.oauth2.credentials import Credentials
from google.auth.transport.requests import Request
from googleapiclient.discovery import build
import requests

SCRIPT_DIR = Path(__file__).resolve().parent
TOKEN_FILE = SCRIPT_DIR / ".gmail_token.json"
SCOPES = ["https://www.googleapis.com/auth/gmail.readonly"]

TURSO_URL = os.environ.get("TURSO_DATABASE_URL", "").replace("libsql://", "https://")
TURSO_TOKEN = os.environ.get("TURSO_AUTH_TOKEN", "")

RULES_CACHE: list[dict] = []

def get_merchant_rules() -> list[dict]:
    global RULES_CACHE
    if RULES_CACHE:
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
        return rules
    except Exception as e:
        print(f"⚠️ Error fetching merchant rules from Turso: {e}")
        return RULES_CACHE

def apply_merchant_rule(merchant: str, text: str = "") -> tuple[str, str]:
    rules = get_merchant_rules()
    m_lower = (merchant or "").lower()
    if m_lower and m_lower != "unknown":
        for r in rules:
            if r["pattern"] in m_lower:
                clean = r["clean_merchant"] or merchant
                return clean, r["category"]
    haystack = f"{merchant} {text}".lower()
    for r in rules:
        if r["pattern"] in haystack:
            clean = r["clean_merchant"] or merchant
            return clean, r["category"]
    return merchant, "Misc"

def get_body(payload: dict) -> str:
    """Extracts raw html or plain text from multipart Gmail message payload."""
    if "data" in payload.get("body", {}):
        return base64.urlsafe_b64decode(payload["body"]["data"]).decode("utf-8", errors="ignore")
    for part in payload.get("parts", []):
        text = get_body(part)
        if text:
            return text
    return ""

def parse_financial_transaction(msg_id: str, headers: dict, raw_body: str, snippet: str) -> dict | None:
    """
    Detects if an email is a financial expense and extracts structured metadata:
    - Amount (float)
    - Currency (IDR)
    - Merchant name
    - Category
    - Payment Method
    """
    subject = headers.get("subject", "")
    sender = headers.get("from", "")
    date_str = headers.get("date", "")
    
    clean_text = re.sub(r"<[^>]+>", "\n", raw_body)
    lines = [l.strip() for l in clean_text.split("\n") if l.strip()]
    full_text = " ".join(lines)
    
    # 1. Anti-patterns (Ignore promos, notifications, statements, refunds, limit changes)
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

    # Specific Handler: Krom Bank QRIS
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

    # Specific Handler: Kredivo
    elif "kredivo" in sender.lower() or "kredivo" in subject.lower():
        payment_method = "Kredivo"
        # Prune promotional & security footer to prevent false positives (e.g. 'Shop at Shopee' footer ad)
        body_lines = []
        for line in lines:
            if any(cut in line.lower() for cut in ["coba layanan favorit", "hati-hati dengan berbagai modus", "butuh bantuan?"]):
                break
            body_lines.append(line)

        # 1. Amount extraction
        for idx, line in enumerate(body_lines):
            if any(k in line.lower() for k in ["transaksi berhasil!", "total yang perlu dibayar", "bayar dalam 1 bulan", "total belanja", "rp bayar:"]):
                for target in [line, body_lines[idx+1] if idx+1 < len(body_lines) else ""]:
                    m = re.search(r"Rp\s*([0-9.,]+)", target)
                    if m:
                        val = float(m.group(1).replace(".", "").replace(",", "."))
                        if val > 0 and amount == 0.0:
                            amount = val
        if amount == 0.0:
            m = re.search(r"Rp\s*([0-9.,]+)", snippet + " " + " ".join(body_lines))
            if m:
                amount = float(m.group(1).replace(".", "").replace(",", "."))

        # 2. Merchant extraction: Check 'Nama Merchant'
        for idx, line in enumerate(body_lines):
            line_clean = line.strip().lower()
            if line_clean in ("nama merchant", "merchant:") and idx + 1 < len(body_lines):
                cand = body_lines[idx+1].strip()
                if cand and not any(k in cand.lower() for k in ["tanggal", "id transaksi", "detail", "total"]):
                    merchant = cand
                    break
            elif line_clean.startswith("nama merchant:"):
                merchant = line.split(":", 1)[1].strip()
                break
            elif line_clean.startswith("merchant:"):
                merchant = line.split(":", 1)[1].strip()
                break

        # 3. Check 'Tipe Transaksi' (e.g. 'Belanja di Shopee')
        if merchant == "Unknown":
            for idx, line in enumerate(body_lines):
                if line.strip().lower() == "tipe transaksi" and idx + 1 < len(body_lines):
                    tipe = body_lines[idx+1].strip()
                    if "belanja di " in tipe.lower():
                        merchant = re.sub(r"(?i)belanja\s+di\s+", "", tipe).strip()
                        break

        # 4. Check biller / utility services
        content_upper = (" ".join(body_lines) + " " + subject).upper()
        if merchant == "Unknown":
            if any(k in content_upper for k in ["PLN", "TOKEN", "LISTRIK"]):
                merchant = "PLN Listrik"
                category = "Utility"
            elif any(k in content_upper for k in ["PULSA", "PAKET DATA"]):
                merchant = "Pulsa"
                category = "Utility"

        # 5. Check body_lines (excluding promotional footer) for marketplace keywords
        if merchant == "Unknown":
            body_joined = " ".join(body_lines).lower()
            if "tokopedia" in body_joined:
                merchant = "Tokopedia"
                category = "Shopping"
            elif "shopee" in snippet.lower() or "shopee" in subject.lower():
                merchant = "Shopee"
                category = "Shopping"
            else:
                merchant = "Kredivo Merchant"

    # Specific Handler: GOERS / Ticketing
    elif "goersapp.com" in sender.lower() and ("tiket" in subject.lower() or "berhasil" in subject.lower()):
        payment_method = "QRIS / Online"
        category = "Entertainment"
        if "taman mini" in subject.lower() or "tmii" in subject.lower():
            merchant = "Taman Mini Indonesia Indah (TMII)"
        else:
            merchant = "GOERS Ticketing"
        # Total line search
        for idx, line in enumerate(lines):
            if line.strip().lower() == "total" and idx + 1 < len(lines):
                m = re.search(r"(?:IDR|Rp)\s*([0-9.,]+)", lines[idx+1])
                if m:
                    amount = float(m.group(1).replace(".", "").replace(",", "."))
                    break

    # Generic Fallback Parser for GoPay, ShopeePay, BCA, Grab, Gojek
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
            "received_at": date_str,
            "amount": amount,
            "currency": "IDR",
            "merchant": merchant,
            "category": category,
            "payment_method": payment_method,
            "raw_snippet": snippet[:500]
        }
    return None

def save_to_turso(record: dict) -> bool:
    """Inserts or updates expense record in Turso."""
    if not TURSO_URL or not TURSO_TOKEN:
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
        return "results" in res.json() and res.json()["results"][0].get("type") == "ok"
    except Exception as e:
        print(f"Turso save error: {e}")
        return False

def main():
    if not TOKEN_FILE.exists():
        print(f"Error: {TOKEN_FILE} not found. Run watch.py first to authenticate.")
        sys.exit(1)

    creds = Credentials.from_authorized_user_file(str(TOKEN_FILE), SCOPES)
    if creds and creds.expired and creds.refresh_token:
        creds.refresh(Request())
    service = build("gmail", "v1", credentials=creds)

    print("🔍 Scanning Gmail inbox for expenses in November 2026...")
    # Filter: newer_than:30d matching transactions
    query = "newer_than:30d (receipt OR invoice OR pembayaran OR transaksi OR transfer OR gopay OR shopeepay OR bca OR jago OR jenius OR tokopedia OR shopee OR grab OR gojek OR krom OR kredivo OR goers)"
    res = service.users().messages().list(userId="me", q=query, maxResults=30).execute()
    messages = res.get("messages", [])

    print(f"📥 Found {len(messages)} candidate emails. Analyzing contents...\n")
    found_expenses = []

    for m in messages:
        msg = service.users().messages().get(userId="me", id=m["id"], format="full").execute()
        payload = msg.get("payload", {})
        headers = {h["name"].lower(): h["value"] for h in payload.get("headers", [])}
        raw_body = get_body(payload)
        snippet = msg.get("snippet", "")

        tx = parse_financial_transaction(m["id"], headers, raw_body, snippet)
        if tx:
            found_expenses.append(tx)
            saved = save_to_turso(tx)
            status = "💾 Saved to Turso" if saved else "⚠️ Turso skipped"
            print(f"💸 [TRANSACTION DETECTED] {status}")
            print(f"   • Merchant       : {tx['merchant']}")
            print(f"   • Amount         : {tx['currency']} {tx['amount']:,.2f}")
            print(f"   • Category       : {tx['category']}")
            print(f"   • Payment Method : {tx['payment_method']}")
            print(f"   • Date           : {tx['received_at']}")
            print(f"   • Subject        : {tx['subject']}")
            print("-" * 60)

    print(f"\n🎉 Selesai! Berhasil mengekstrak {len(found_expenses)} transaksi pengeluaran bulan ini ke Turso Cloud.")

if __name__ == "__main__":
    main()
