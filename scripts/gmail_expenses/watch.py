# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "google-api-python-client>=2.150.0",
#     "google-auth>=2.36.0",
#     "google-auth-httplib2>=0.2.0",
# ]
# ///
"""
Gmail Watch Activator using Google Application Default Credentials (ADC).
No credentials.json required! Just authenticate once via:
    gcloud auth application-default login --scopes='https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/gmail.readonly'
"""

import os
import sys
import google.auth
from google.auth.transport.requests import Request
from googleapiclient.discovery import build
from googleapiclient.errors import HttpError

SCOPES = [
    "https://www.googleapis.com/auth/gmail.readonly",
    "https://www.googleapis.com/auth/cloud-platform",
]

PROJECT_ID = os.environ.get("GCP_PROJECT", "ichsanul-dev")
TOPIC_NAME = f"projects/{PROJECT_ID}/topics/gmail-expenses"

def get_gmail_service():
    try:
        creds, _ = google.auth.default(scopes=SCOPES)
        if not creds.valid:
            creds.refresh(Request())
        return build("gmail", "v1", credentials=creds)
    except Exception as e:
        print(f"❌ Failed to load Google credentials: {e}")
        print("\n🔑 Please run this in your terminal to log in via browser:")
        print("  gcloud auth application-default login --scopes='https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/gmail.readonly'\n")
        sys.exit(1)

def main():
    print(f"🔗 Setting up Gmail watch on topic: {TOPIC_NAME}...")
    service = get_gmail_service()

    request_body = {
        "topicName": TOPIC_NAME,
        "labelIds": ["INBOX"]
    }

    try:
        response = service.users().watch(userId="me", body=request_body).execute()
        print("✅ Gmail watch successfully activated!")
        print(f"  • History ID : {response.get('historyId')}")
        print(f"  • Expiration : {response.get('expiration')} (valid ~7 days)")
    except HttpError as err:
        if err.resp.status == 403:
            print("❌ Permission Denied (403): Your login session does not have the Gmail scope.")
            print("\nRun this command to re-login with Gmail permissions:")
            print("  gcloud auth application-default login --scopes='https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/gmail.readonly'\n")
        else:
            print(f"❌ Gmail API Error: {err}")
        sys.exit(1)

if __name__ == "__main__":
    main()
