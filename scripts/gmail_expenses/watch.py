# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "google-api-python-client>=2.150.0",
#     "google-auth-oauthlib>=1.2.0",
#     "google-auth-httplib2>=0.2.0",
# ]
# ///
"""
Gmail Watch Activator.
Calls gmail.users().watch() to forward new inbox messages to the Cloud Pub/Sub topic.
Requires an OAuth 2.0 client credentials file (credentials.json) from Google Cloud Console.
"""

import os
import sys
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
TOKEN_FILE = SCRIPT_DIR / "token.json"
CREDS_FILE = SCRIPT_DIR / "credentials.json"

def get_gmail_service():
    creds = None
    if TOKEN_FILE.exists():
        creds = Credentials.from_authorized_user_file(str(TOKEN_FILE), SCOPES)

    if not creds or not creds.valid:
        if creds and creds.expired and creds.refresh_token:
            creds.refresh(Request())
        else:
            if not CREDS_FILE.exists():
                print(f"❌ Error: {CREDS_FILE} not found!")
                print("\nTo generate credentials.json:")
                print("1. Go to Google Cloud Console > APIs & Services > Credentials")
                print("2. Click Create Credentials > OAuth client ID > Application type: Desktop app")
                print(f"3. Download JSON and save as: {CREDS_FILE}")
                sys.exit(1)

            flow = InstalledAppFlow.from_client_secrets_file(str(CREDS_FILE), SCOPES)
            creds = flow.run_local_server(port=0)

        with open(TOKEN_FILE, "w") as token:
            token.write(creds.to_json())

    return build("gmail", "v1", credentials=creds)

def main():
    print(f"🔗 Setting up Gmail watch on topic: {TOPIC_NAME}...")
    service = get_gmail_service()
    
    # Watch request body
    request_body = {
        "topicName": TOPIC_NAME,
        "labelIds": ["INBOX"]
    }
    
    response = service.users().watch(userId="me", body=request_body).execute()
    print("✅ Gmail watch successfully activated!")
    print(f"  • History ID : {response.get('historyId')}")
    print(f"  • Expiration : {response.get('expiration')} (valid ~7 days, renew periodically)")

if __name__ == "__main__":
    main()
