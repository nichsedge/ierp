"""
Google People API client and OAuth 2.0 synchronizer ($0 cost).
Supports Application Default Credentials (ADC) and local loopback authentication with incremental delta sync.
"""

import http.server
import json
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import webbrowser
from datetime import datetime
from pathlib import Path
from typing import Optional, Dict, Any, Tuple

from .config import (
    GOOGLE_CREDS_PATH, GOOGLE_TOKEN_PATH,
    C_BOLD, C_GREEN, C_YELLOW, C_RED, C_CYAN, C_RESET
)
from .db import get_db, init_db
from .linking import link_events_and_contacts


def load_google_client_config(custom_path: Optional[str] = None) -> Tuple[Optional[str], Optional[str], Optional[Path]]:
    """Discovers client ID and client secret from known credential paths."""
    search_paths = []
    if custom_path:
        search_paths.append(Path(custom_path))
    search_paths.extend([
        GOOGLE_CREDS_PATH,
        Path(__file__).resolve().parent.parent / "credentials.json",
        Path(__file__).resolve().parent.parent / "client_secret.json",
        Path(__file__).resolve().parent.parent.parent.parent / "gcp" / "OAuth_cred_PeopleAPI.json",
    ])
    for p in Path(__file__).resolve().parent.parent.glob("client_secret*.json"):
        search_paths.append(p)
    for p in Path(__file__).resolve().parent.parent.glob("*OAuth*.json"):
        search_paths.append(p)

    for p in search_paths:
        if p.exists() and p.is_file():
            try:
                with open(p, "r", encoding="utf-8") as f:
                    data = json.load(f)
                cfg = data.get("installed") or data.get("web") or data
                client_id = cfg.get("client_id")
                client_secret = cfg.get("client_secret")
                if client_id and client_secret:
                    return client_id, client_secret, p
            except Exception:
                continue
    return None, None, None


def get_google_access_token(custom_creds_path: Optional[str] = None) -> Optional[str]:
    """Retrieves or refreshes a valid Google OAuth 2.0 access token ($0 API cost)."""
    # 1. First attempt: Check if Application Default Credentials (ADC) has contacts scope
    try:
        proc = subprocess.run(
            ["gcloud", "auth", "application-default", "print-access-token"],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=5
        )
        if proc.returncode == 0 and proc.stdout.strip():
            candidate_token = proc.stdout.strip()
            info_req = urllib.request.Request(f"https://oauth2.googleapis.com/tokeninfo?access_token={candidate_token}")
            with urllib.request.urlopen(info_req, timeout=5) as info_resp:
                info_data = json.loads(info_resp.read().decode("utf-8"))
            scopes = info_data.get("scope", "").split()
            if any("contacts" in s for s in scopes):
                print(f"{C_GREEN}Authenticated via gcloud Application Default Credentials (ADC)!{C_RESET}")
                return candidate_token
    except Exception:
        pass

    # 2. Second attempt: Check local OAuth client configuration
    client_id, client_secret, creds_file = load_google_client_config(custom_creds_path)
    if not client_id or not client_secret:
        print(f"\n{C_RED}{C_BOLD}Google OAuth credentials not configured!{C_RESET}")
        print(f"Please place your Google Cloud Desktop App OAuth JSON at:")
        print(f"  {C_CYAN}{GOOGLE_CREDS_PATH}{C_RESET}\n")
        return None

    tokens = {}
    if GOOGLE_TOKEN_PATH.exists():
        try:
            with open(GOOGLE_TOKEN_PATH, "r", encoding="utf-8") as f:
                tokens = json.load(f)
        except Exception:
            tokens = {}

    now = int(datetime.now().timestamp())
    # If access token is still valid
    if tokens.get("access_token") and tokens.get("expires_at", 0) > now + 60:
        return tokens["access_token"]

    # Try refreshing existing token
    if tokens.get("refresh_token"):
        try:
            req_data = urllib.parse.urlencode({
                "client_id": client_id,
                "client_secret": client_secret,
                "refresh_token": tokens["refresh_token"],
                "grant_type": "refresh_token"
            }).encode("utf-8")
            req = urllib.request.Request("https://oauth2.googleapis.com/token", data=req_data, method="POST")
            req.add_header("Content-Type", "application/x-www-form-urlencoded")
            with urllib.request.urlopen(req, timeout=15) as resp:
                refreshed = json.loads(resp.read().decode("utf-8"))
            tokens["access_token"] = refreshed["access_token"]
            tokens["expires_at"] = now + refreshed.get("expires_in", 3600)
            if "refresh_token" in refreshed:
                tokens["refresh_token"] = refreshed["refresh_token"]
            with open(GOOGLE_TOKEN_PATH, "w", encoding="utf-8") as f:
                json.dump(tokens, f, indent=2)
            return tokens["access_token"]
        except Exception as e:
            print(f"{C_YELLOW}Token refresh failed ({e}), starting new OAuth consent flow...{C_RESET}")

    # Loopback OAuth Flow
    port = 8765
    redirect_uri = f"http://localhost:{port}/callback"
    auth_code_holder = {"code": None, "error": None}

    class OAuthCallbackHandler(http.server.BaseHTTPRequestHandler):
        def log_message(self, format, *args):
            pass
        def do_GET(self):
            parsed = urllib.parse.urlparse(self.path)
            if parsed.path == "/callback":
                qs = urllib.parse.parse_qs(parsed.query)
                if "code" in qs:
                    auth_code_holder["code"] = qs["code"][0]
                    self.send_response(200)
                    self.send_header("Content-Type", "text/html; charset=utf-8")
                    self.end_headers()
                    self.wfile.write(b"<html><body style='font-family:sans-serif;padding:40px;text-align:center;background:#090d16;color:#f3f4f6'><h2>Google Contacts Authorization Successful!</h2><p>You can close this tab and return to the terminal.</p></body></html>")
                else:
                    auth_code_holder["error"] = qs.get("error", ["Unknown error"])[0]
                    self.send_response(400)
                    self.send_header("Content-Type", "text/html; charset=utf-8")
                    self.end_headers()
                    self.wfile.write(b"<html><body style='font-family:sans-serif;padding:40px;text-align:center;color:#ef4444'><h2>Authorization Failed</h2></body></html>")

    server = http.server.HTTPServer(("localhost", port), OAuthCallbackHandler)
    server.timeout = 120

    scope = "https://www.googleapis.com/auth/contacts.readonly"
    auth_params = urllib.parse.urlencode({
        "client_id": client_id,
        "redirect_uri": redirect_uri,
        "response_type": "code",
        "scope": scope,
        "access_type": "offline",
        "prompt": "consent"
    })
    auth_url = f"https://accounts.google.com/o/oauth2/v2/auth?{auth_params}"

    print(f"\n{C_BOLD}=== Google Contacts OAuth Authorization ==={C_RESET}")
    print(f"Opening browser for Google authorization:\n  {C_CYAN}{auth_url}{C_RESET}\n")
    print("Waiting for authorization callback in browser...")

    try:
        webbrowser.open(auth_url)
    except Exception:
        pass

    while not auth_code_holder["code"] and not auth_code_holder["error"]:
        server.handle_request()

    server.server_close()

    if not auth_code_holder["code"]:
        print(f"{C_RED}OAuth authorization failed: {auth_code_holder.get('error')}{C_RESET}")
        return None

    # Exchange authorization code for tokens
    token_params = urllib.parse.urlencode({
        "code": auth_code_holder["code"],
        "client_id": client_id,
        "client_secret": client_secret,
        "redirect_uri": redirect_uri,
        "grant_type": "authorization_code"
    }).encode("utf-8")

    token_req = urllib.request.Request("https://oauth2.googleapis.com/token", data=token_params, method="POST")
    token_req.add_header("Content-Type", "application/x-www-form-urlencoded")
    with urllib.request.urlopen(token_req, timeout=15) as resp:
        token_data = json.loads(resp.read().decode("utf-8"))

    token_data["expires_at"] = now + token_data.get("expires_in", 3600)
    with open(GOOGLE_TOKEN_PATH, "w", encoding="utf-8") as f:
        json.dump(token_data, f, indent=2)

    print(f"{C_GREEN}Authentication successful and token saved to {GOOGLE_TOKEN_PATH.name}.{C_RESET}")
    return token_data.get("access_token")


def sync_google_contacts(custom_creds: Optional[str] = None, full_resync: bool = False) -> Dict[str, Any]:
    """
    Synchronizes contacts from Google People API into SQLite:
      - Uses syncToken for incremental delta changes ($0 API cost)
      - Automatically sets source to 'google' or 'merged'
      - Preserves manual notes and client relations
      - Triggers auto-linking with journal events
    """
    init_db()
    access_token = get_google_access_token(custom_creds)
    if not access_token:
        return {"status": "error", "message": "Failed to obtain Google access token"}

    conn = get_db()
    cursor = conn.cursor()

    sync_token = None
    if not full_resync:
        row = cursor.execute("SELECT value FROM sync_state WHERE key = 'google_contacts_sync_token'").fetchone()
        if row:
            sync_token = row[0]

    base_url = "https://people.googleapis.com/v1/people/me/connections"
    person_fields = "names,emailAddresses,phoneNumbers,organizations,addresses,biographies,metadata"

    print(f"\n{C_BOLD}{C_CYAN}=== Syncing Google Contacts ==={C_RESET}")
    if sync_token:
        print(f"  Performing incremental delta sync...")
    else:
        print(f"  Performing full contact synchronization...")

    next_page_token = None
    next_sync_token = None
    total_fetched = 0
    added_count = 0
    updated_count = 0

    while True:
        params = {
            "personFields": person_fields,
            "pageSize": 200,
        }
        if sync_token:
            params["syncToken"] = sync_token
        else:
            params["requestSyncToken"] = "true"

        if next_page_token:
            params["pageToken"] = next_page_token

        query_str = urllib.parse.urlencode(params)
        req = urllib.request.Request(f"{base_url}?{query_str}")
        req.add_header("Authorization", f"Bearer {access_token}")
        req.add_header("Accept", "application/json")

        try:
            with urllib.request.urlopen(req, timeout=20) as resp:
                data = json.loads(resp.read().decode("utf-8"))
        except urllib.error.HTTPError as e:
            if e.code == 410 or "syncToken" in str(e):
                print(f"{C_YELLOW}Sync token expired. Restarting full sync...{C_RESET}")
                cursor.execute("DELETE FROM sync_state WHERE key = 'google_contacts_sync_token'")
                conn.commit()
                conn.close()
                return sync_google_contacts(custom_creds, full_resync=True)
            print(f"{C_RED}Google People API error: {e}{C_RESET}")
            conn.close()
            return {"status": "error", "message": str(e)}
        except Exception as e:
            print(f"{C_RED}Sync request failed: {e}{C_RESET}")
            conn.close()
            return {"status": "error", "message": str(e)}

        connections = data.get("connections", [])
        next_sync_token = data.get("nextSyncToken") or next_sync_token
        next_page_token = data.get("nextPageToken")

        for person in connections:
            total_fetched += 1
            resource_name = person.get("resourceName")

            names = person.get("names", [])
            name = None
            if names:
                raw_name = names[0].get("displayName") or f"{names[0].get('givenName', '')} {names[0].get('familyName', '')}".strip()
                name = raw_name.title() if raw_name else None
            if not name:
                continue

            emails = [em.get("value") for em in person.get("emailAddresses", []) if em.get("value")]
            email_primary = emails[0] if emails else None

            phones = [ph.get("value") for ph in person.get("phoneNumbers", []) if ph.get("value")]
            phone_primary = phones[0] if phones else None

            orgs = person.get("organizations", [])
            org_name = None
            job_title = None
            if orgs:
                org_name = orgs[0].get("name")
                job_title = orgs[0].get("title")
            org_display = f"{org_name} ({job_title})" if org_name and job_title else (org_name or job_title)

            addresses = person.get("addresses", [])
            location_val = None
            if addresses:
                location_val = addresses[0].get("formattedValue") or addresses[0].get("city")

            bios = person.get("biographies", [])
            bio_text = bios[0].get("value") if bios else ""

            note_lines = []
            if job_title and not org_name:
                note_lines.append(f"Title: {job_title}")
            if emails:
                note_lines.append(f"Emails: {', '.join(emails)}")
            if phones:
                note_lines.append(f"Phones: {', '.join(phones)}")
            if bio_text:
                note_lines.append(f"Bio: {bio_text}")
            notes = "\n".join(note_lines) if note_lines else None

            now_iso = datetime.now().strftime("%Y-%m-%d")

            # Check if contact already exists
            existing = None
            if resource_name:
                existing = cursor.execute(
                    "SELECT id, name, org, location, notes, email, phone, client, source FROM contacts WHERE google_id = ?",
                    (resource_name,)
                ).fetchone()
            if not existing and email_primary:
                existing = cursor.execute(
                    "SELECT id, name, org, location, notes, email, phone, client, source FROM contacts WHERE email = ?",
                    (email_primary,)
                ).fetchone()
            if not existing:
                existing = cursor.execute(
                    "SELECT id, name, org, location, notes, email, phone, client, source FROM contacts WHERE LOWER(name) = LOWER(?)",
                    (name.strip(),)
                ).fetchone()

            if existing:
                cid, orig_name, orig_org, orig_loc, orig_notes, orig_email, orig_phone, orig_client, orig_source = existing
                new_org = org_display or orig_org
                new_loc = location_val or orig_loc
                combined_notes = orig_notes or ""
                if notes and notes not in combined_notes:
                    combined_notes = f"{combined_notes}\n{notes}".strip() if combined_notes else notes
                new_source = "merged" if (orig_client or orig_source == "manual" or orig_source == "merged") else "google"

                cursor.execute("""
                UPDATE contacts 
                SET name = ?, org = ?, location = ?, email = COALESCE(?, email), phone = COALESCE(?, phone), 
                    notes = ?, google_id = ?, source = ?
                WHERE id = ?
                """, (name, new_org, new_loc, email_primary, phone_primary, combined_notes, resource_name, new_source, cid))
                updated_count += 1
            else:
                cursor.execute("""
                INSERT INTO contacts (name, client, date, location, org, notes, email, phone, google_id, source)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'google')
                """, (name, None, now_iso, location_val, org_display, notes, email_primary, phone_primary, resource_name))
                added_count += 1

        if not next_page_token:
            break

    if next_sync_token:
        cursor.execute("""
        INSERT INTO sync_state (key, value, updated_at) 
        VALUES ('google_contacts_sync_token', ?, datetime('now', 'localtime'))
        ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
        """, (next_sync_token,))

    conn.commit()
    print(f"\n{C_GREEN}Google Contacts sync complete!{C_RESET}")
    print(f"  Total Processed: {total_fetched}")
    print(f"  New Contacts:    {C_GREEN}{added_count}{C_RESET}")
    print(f"  Updated Contacts:{C_YELLOW}{updated_count}{C_RESET}")

    link_events_and_contacts(conn)
    conn.close()
    return {
        "status": "success",
        "total_processed": total_fetched,
        "added": added_count,
        "updated": updated_count
    }
