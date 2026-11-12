# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "google-api-python-client>=2.150.0",
#     "google-auth-oauthlib>=1.2.0",
#     "google-auth-httplib2>=0.2.0",
#     "requests>=2.32.0",
# ]
# ///
"""
Gmail Watch Activator.
Registers Gmail push notifications to the Cloud Pub/Sub topic using your project's OAuth Client ID.
No JSON files needed: reads GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET from environment.
"""

import os
import sys
import json
from pathlib import Path
from google.auth.transport.requests import Request
from google.oauth2.credentials import Credentials
from google_auth_oauthlib.flow import InstalledAppFlow
from googleapiclient.discovery import build

SCOPES = [
    "https://www.googleapis.com/auth/gmail.readonly",
]

PROJECT_ID = os.environ.get("GCP_PROJECT", "ichsanul-dev")
TOPIC_NAME = f"projects/{PROJECT_ID}/topics/gmail-expenses"

SCRIPT_DIR = Path(__file__).resolve().parent
TOKEN_FILE = SCRIPT_DIR / ".gmail_token.json"

def get_gmail_service():
    creds = None
    if TOKEN_FILE.exists():
        try:
            creds = Credentials.from_authorized_user_file(str(TOKEN_FILE), SCOPES)
        except Exception:
            creds = None

    if not creds or not creds.valid:
        refreshed = False
        if creds and creds.expired and creds.refresh_token:
            try:
                creds.refresh(Request())
                refreshed = True
            except Exception as err:
                print(f"⚠️ Refresh token gagal/kadaluarsa ({err}). Membuka browser untuk re-autentikasi...")
                creds = None

        if not refreshed:
            client_id = os.environ.get("GOOGLE_CLIENT_ID", "").strip()
            client_secret = os.environ.get("GOOGLE_CLIENT_SECRET", "").strip()

            if not client_id or not client_secret:
                if TOKEN_FILE.exists():
                    try:
                        with open(TOKEN_FILE, "r", encoding="utf-8") as f:
                            saved = json.load(f)
                            client_id = client_id or saved.get("client_id", "").strip()
                            client_secret = client_secret or saved.get("client_secret", "").strip()
                    except Exception:
                        pass

            if not client_id or not client_secret:
                print("❌ Missing GOOGLE_CLIENT_ID or GOOGLE_CLIENT_SECRET in environment / .gmail_token.json.")
                sys.exit(1)

            client_config = {
                "installed": {
                    "client_id": client_id,
                    "client_secret": client_secret,
                    "auth_uri": "https://accounts.google.com/o/oauth2/auth",
                    "token_uri": "https://oauth2.googleapis.com/token",
                    "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
                    "redirect_uris": ["http://localhost"]
                }
            }

            print("🌐 Membuka browser untuk login OAuth...")
            flow = InstalledAppFlow.from_client_config(client_config, SCOPES)
            creds = flow.run_local_server(port=0)

        # Save credentials for reuse
        with open(TOKEN_FILE, "w", encoding="utf-8") as f:
            f.write(creds.to_json())

    return build("gmail", "v1", credentials=creds), creds

def main():
    print(f"🔗 Setting up Gmail watch on topic: {TOPIC_NAME}...")
    service, creds = get_gmail_service()

    request_body = {
        "topicName": TOPIC_NAME,
        "labelIds": ["INBOX"]
    }

    try:
        response = service.users().watch(userId="me", body=request_body).execute()
        print("\n✅ Gmail watch BERHASIL diaktifkan!")
        print(f"  • History ID : {response.get('historyId')}")
        print(f"  • Expiration : {response.get('expiration')} (aktif ~7 hari)")
        print("\nSekarang setiap email baru masuk di inbox Gmail, sinyalnya akan dipush ke Pub/Sub!")

        if creds and creds.refresh_token:
            print(f"\n🔑 Refresh Token: {creds.refresh_token[:15]}... (lengkap tersimpan di {TOKEN_FILE})")
            print("🔄 Memperbarui Cloud Run service 'gmail-expenses-worker'...")
            import subprocess
            try:
                cmd = [
                    "gcloud", "run", "services", "update", "gmail-expenses-worker",
                    "--region=us-central1",
                    f"--update-env-vars=GOOGLE_REFRESH_TOKEN={creds.refresh_token}"
                ]
                res = subprocess.run(cmd, capture_output=True, text=True)
                if res.returncode == 0:
                    print("✅ Cloud Run berhasil diupdate dengan Refresh Token baru!")
                else:
                    print(f"⚠️ Gagal update Cloud Run: {res.stderr.strip()}")
            except Exception as e:
                print(f"⚠️ Error updating Cloud Run: {e}")
    except Exception as err:
        print(f"❌ Error saat memanggil users.watch(): {err}")
        sys.exit(1)

if __name__ == "__main__":
    main()
