#!/usr/bin/env python3
"""
CLI entrypoint for Personal Journal & CRM Event Log System.
Supports structured logging, Google Contacts/Timeline sync, merging, and interactive dashboard.
"""

import argparse
import json
import os
import sys
from pathlib import Path
from typing import Optional

# Ensure repository root is on sys.path
BASE_DIR = Path(__file__).resolve().parent
REPO_ROOT = BASE_DIR.parent
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))
if str(BASE_DIR) not in sys.path:
    sys.path.insert(0, str(BASE_DIR))

from ierp.core.config import (
    DB_PATH, MEDIA_DIR, C_RESET, C_BOLD, C_GREEN, C_CYAN, C_YELLOW, C_RED, C_MAGENTA
)
from ierp.core.db import get_db, init_db
from ierp.core.google_sync import sync_google_contacts
from ierp.core.importers import import_notion_export, import_crm_contacts, import_timeline, parse_date_to_iso
from ierp.core.linking import link_events_and_contacts, run_manual_link
from ierp.core.merging import merge_two_contacts, auto_merge_contacts
from ierp.core.dashboard import start_dashboard_server
from ierp.core.media import ingest_media_records, upsert_link, list_media, list_links
from ierp.core.commerce import (
    init_tables as init_commerce_tables,
    upsert_payment_account, upsert_referral,
    list_payment_accounts, list_referrals,
)
from ierp.core.ingest import ingest_rows, serve_ingest
from ierp.core.sync import sync_all
from ierp.core.config import MEDIA_PROFILES


def insert_event_direct(
    title: str,
    place: Optional[str] = None,
    start_date: Optional[str] = None,
    end_date: Optional[str] = None,
    tags: Optional[str] = None,
    url: Optional[str] = None,
    notes: Optional[str] = None
) -> None:
    """Inserts a structured event record directly into SQLite."""
    init_db()
    conn = get_db()
    cursor = conn.cursor()

    parsed_start, _ = parse_date_to_iso(start_date)
    parsed_end, _ = parse_date_to_iso(end_date)
    tag_list = [t.strip() for t in tags.split(",") if t.strip()] if tags else []

    try:
        cursor.execute("""
        INSERT INTO events (title, place, start_date, end_date, raw_date, tags, url, notes)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        """, (title, place, parsed_start, parsed_end, start_date, json.dumps(tag_list), url, notes))
        conn.commit()
        ev_id = cursor.lastrowid
        print(f"{C_GREEN}Successfully inserted event #{ev_id}: {title}{C_RESET}")
        link_events_and_contacts(conn)
    except Exception as e:
        print(f"{C_RED}Failed to insert event: {e}{C_RESET}")
    finally:
        conn.close()


def list_events(limit: int = 20) -> None:
    """Lists the most recent events."""
    if not DB_PATH.exists():
        print(f"{C_RED}Database does not exist.{C_RESET}")
        return

    conn = get_db()
    cursor = conn.cursor()
    cursor.execute("""
    SELECT id, title, place, start_date, tags FROM events 
    ORDER BY start_date DESC LIMIT ?
    """, (limit,))
    rows = cursor.fetchall()
    conn.close()

    if not rows:
        print("No events found.")
        return

    print(f"\n{C_BOLD}{'ID':<5} | {'Date':<19} | {'Title':<30} | {'Place':<20} | {'Tags':<20}{C_RESET}")
    print("-" * 105)
    for row in rows:
        eid, title, place, start, tags_json = row
        tags = ", ".join(json.loads(tags_json or "[]"))
        place = place or ""
        start = start or "No Date"
        print(f"{eid:<5} | {start[:19]:<19} | {title[:30]:<30} | {place[:20]:<20} | {tags[:20]:<20}")
    print()


def search_events(query: str) -> None:
    """Searches for events matching a query keyword."""
    if not DB_PATH.exists():
        print(f"{C_RED}Database does not exist.{C_RESET}")
        return

    conn = get_db()
    cursor = conn.cursor()
    like_query = f"%{query}%"
    cursor.execute("""
    SELECT id, title, place, start_date, tags FROM events 
    WHERE title LIKE ? OR place LIKE ? OR tags LIKE ? OR notes LIKE ?
    ORDER BY start_date DESC
    """, (like_query, like_query, like_query, like_query))
    rows = cursor.fetchall()
    conn.close()

    if not rows:
        print(f"No events matching '{query}' found.")
        return

    print(f"\n{C_BOLD}Search results for '{query}':{C_RESET}")
    print(f"{C_BOLD}{'ID':<5} | {'Date':<19} | {'Title':<30} | {'Place':<20} | {'Tags':<20}{C_RESET}")
    print("-" * 105)
    for row in rows:
        eid, title, place, start, tags_json = row
        tags = ", ".join(json.loads(tags_json or "[]"))
        place = place or ""
        start = start or "No Date"
        print(f"{eid:<5} | {start[:19]:<19} | {title[:30]:<30} | {place[:20]:<20} | {tags[:20]:<20}")
    print()


def show_event(event_id: int) -> None:
    """Displays full event details and linked CRM contacts."""
    if not DB_PATH.exists():
        print(f"{C_RED}Database does not exist.{C_RESET}")
        return

    conn = get_db()
    cursor = conn.cursor()
    row = cursor.execute("""
    SELECT title, place, start_date, end_date, raw_date, tags, url, notes 
    FROM events WHERE id = ?
    """, (event_id,)).fetchone()

    if not row:
        print(f"{C_RED}Event with ID {event_id} not found.{C_RESET}")
        conn.close()
        return

    title, place, start, end, raw_date, tags_json, url, notes = row
    tags = ", ".join(json.loads(tags_json or "[]"))
    start = start or "No Date"

    media_rows = cursor.execute("SELECT original_filename, stored_path FROM event_media WHERE event_id = ?", (event_id,)).fetchall()
    contact_rows = cursor.execute("""
    SELECT c.id, c.name, c.org FROM contacts c
    JOIN event_contacts ec ON c.id = ec.contact_id
    WHERE ec.event_id = ?
    """, (event_id,)).fetchall()
    conn.close()

    print(f"\n{C_BOLD}{C_MAGENTA}=== Event Details (ID: {event_id}) ==={C_RESET}")
    print(f"{C_BOLD}Title:{C_RESET}      {title}")
    if place:
        print(f"{C_BOLD}Place:{C_RESET}      {place}")
    print(f"{C_BOLD}Date:{C_RESET}       {start} {f'→ {end}' if end else ''} (Original: {raw_date or 'None'})")
    if tags:
        print(f"{C_BOLD}Tags:{C_RESET}       {tags}")
    if url:
        print(f"{C_BOLD}URL:{C_RESET}        {url}")

    if contact_rows:
        contacts_str = ", ".join(f"{name} (ID: {cid})" for cid, name, org in contact_rows)
        print(f"{C_BOLD}Linked People:{C_RESET} {contacts_str}")

    if media_rows:
        print(f"{C_BOLD}Attachments:{C_RESET}")
        for orig, stored in media_rows:
            print(f"  - {orig} ({stored})")

    print(f"\n{C_BOLD}--- Notes / Markdown Content ---{C_RESET}")
    if notes:
        print(notes)
    else:
        print(f"{C_YELLOW}(No notes content){C_RESET}")
    print(f"{C_BOLD}{C_MAGENTA}================================={C_RESET}\n")


def list_contacts(source_filter: Optional[str] = None) -> None:
    """Lists contacts with optional source filtering ('manual', 'google', 'merged', 'all')."""
    init_db()
    conn = get_db()
    cursor = conn.cursor()

    sql = "SELECT id, name, org, client, location, email, phone, source FROM contacts WHERE 1=1"
    params = []
    if source_filter and source_filter.lower() != "all":
        sql += " AND LOWER(source) = ?"
        params.append(source_filter.lower())

    sql += " ORDER BY name ASC"
    rows = cursor.execute(sql, params).fetchall()
    conn.close()

    if not rows:
        print(f"No contacts found{' with source ' + source_filter if source_filter else ''}.")
        return

    print(f"\n{C_BOLD}{'ID':<5} | {'Name':<22} | {'Source':<10} | {'Org/Title':<18} | {'Client':<12} | {'Email/Phone':<22}{C_RESET}")
    print("-" * 105)
    for row in rows:
        cid, name, org, client, location, email, phone, src = row
        src_label = (src or "manual").capitalize()
        contact_info = email or phone or ""
        print(f"{cid:<5} | {name[:22]:<22} | {src_label:<10} | {(org or '')[:18]:<18} | {(client or '')[:12]:<12} | {contact_info[:22]:<22}")
    print()


def show_contact(contact_id: int) -> None:
    """Displays contact details and all linked events."""
    init_db()
    conn = get_db()
    cursor = conn.cursor()

    row = cursor.execute("""
    SELECT name, client, date, location, org, notes, email, phone, google_id, source FROM contacts 
    WHERE id = ?
    """, (contact_id,)).fetchone()

    if not row:
        print(f"{C_RED}Contact with ID {contact_id} not found.{C_RESET}")
        conn.close()
        return

    name, client, date_val, location, org, notes, email, phone, google_id, source = row
    event_rows = cursor.execute("""
    SELECT e.id, e.title, e.start_date FROM events e
    JOIN event_contacts ec ON e.id = ec.event_id
    WHERE ec.contact_id = ?
    ORDER BY e.start_date DESC
    """, (contact_id,)).fetchall()
    conn.close()

    source_display = (source or "manual").capitalize()
    if google_id and source != "google":
        source_display = f"{source_display} (Google Synced)"

    print(f"\n{C_BOLD}{C_CYAN}=== Contact Details (ID: {contact_id}) ==={C_RESET}")
    print(f"{C_BOLD}Name:{C_RESET}       {name}")
    print(f"{C_BOLD}Source:{C_RESET}     {source_display}")
    if email:
        print(f"{C_BOLD}Email:{C_RESET}      {email}")
    if phone:
        print(f"{C_BOLD}Phone:{C_RESET}      {phone}")
    if org:
        print(f"{C_BOLD}Org/Role:{C_RESET}   {org}")
    if client:
        print(f"{C_BOLD}Client:{C_RESET}     {client}")
    if location:
        print(f"{C_BOLD}Location:{C_RESET}   {location}")
    if date_val:
        print(f"{C_BOLD}Added Date:{C_RESET} {date_val}")

    if event_rows:
        print(f"\n{C_BOLD}Linked Events ({len(event_rows)}):{C_RESET}")
        for eid, etitle, estart in event_rows:
            estart = estart or "No Date"
            print(f"  - [{eid}] {estart[:10]} | {etitle}")
    else:
        print(f"\n{C_YELLOW}No linked events found for this contact.{C_RESET}")

    if notes:
        print(f"\n{C_BOLD}--- Notes ---{C_RESET}")
        print(notes)
    print(f"{C_BOLD}{C_CYAN}=========================================={C_RESET}\n")


def insert_vendor_direct(
    name: str,
    category: Optional[str] = None,
    location: Optional[str] = None,
    phone: Optional[str] = None,
    email: Optional[str] = None,
    url: Optional[str] = None,
    notes: Optional[str] = None,
    favorite: bool = False
) -> None:
    """Inserts a structured vendor record directly into SQLite."""
    init_db()
    conn = get_db()
    cursor = conn.cursor()

    try:
        cursor.execute("""
        INSERT INTO vendors (name, category, location, phone, email, url, notes, favorite)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        """, (name, category, location, phone, email, url, notes, 1 if favorite else 0))
        conn.commit()
        vid = cursor.lastrowid
        print(f"{C_GREEN}Successfully inserted vendor #{vid}: {name}{C_RESET}")
    except Exception as e:
        print(f"{C_RED}Failed to insert vendor: {e}{C_RESET}")
    finally:
        conn.close()


def list_vendors(category_filter: Optional[str] = None, favorite_only: bool = False) -> None:
    """Lists vendors with optional category and favorite filtering."""
    init_db()
    conn = get_db()
    cursor = conn.cursor()

    sql = "SELECT id, name, category, location, phone, favorite FROM vendors WHERE 1=1"
    params = []
    if category_filter:
        sql += " AND LOWER(category) = ?"
        params.append(category_filter.lower())
    if favorite_only:
        sql += " AND favorite = 1"

    sql += " ORDER BY favorite DESC, name ASC"
    rows = cursor.execute(sql, params).fetchall()
    conn.close()

    if not rows:
        print(f"No vendors found{' in category ' + category_filter if category_filter else ''}.")
        return

    print(f"\n{C_BOLD}{'ID':<5} | {'Name':<24} | {'Category':<18} | {'Location':<18} | {'Phone':<20} | {'Fav':<5}{C_RESET}")
    print("-" * 100)
    for row in rows:
        vid, name, cat, loc, phone, fav = row
        fav_star = "★ Yes" if fav else "No"
        print(f"{vid:<5} | {name[:24]:<24} | {(cat or '')[:18]:<18} | {(loc or '')[:18]:<18} | {(phone or '')[:20]:<20} | {fav_star:<5}")
    print()


def show_vendor(vendor_id: int) -> None:
    """Displays vendor details."""
    init_db()
    conn = get_db()
    cursor = conn.cursor()

    row = cursor.execute("""
    SELECT name, category, location, phone, email, url, notes, favorite, source, created_at FROM vendors 
    WHERE id = ?
    """, (vendor_id,)).fetchone()
    conn.close()

    if not row:
        print(f"{C_RED}Vendor with ID {vendor_id} not found.{C_RESET}")
        return

    name, category, location, phone, email, url, notes, favorite, source, created_at = row
    fav_display = f"{C_YELLOW}★ Favorite{C_RESET}" if favorite else "Standard"

    print(f"\n{C_BOLD}{C_GREEN}=== Vendor Details (ID: {vendor_id}) ==={C_RESET}")
    print(f"{C_BOLD}Name:{C_RESET}       {name}")
    print(f"{C_BOLD}Status:{C_RESET}     {fav_display}")
    if category:
        print(f"{C_BOLD}Category:{C_RESET}   {category}")
    if location:
        print(f"{C_BOLD}Location:{C_RESET}   {location}")
    if phone:
        print(f"{C_BOLD}Phone:{C_RESET}      {phone}")
    if email:
        print(f"{C_BOLD}Email:{C_RESET}      {email}")
    if url:
        print(f"{C_BOLD}URL:{C_RESET}        {url}")
    if source:
        print(f"{C_BOLD}Source:{C_RESET}     {(source or 'manual').capitalize()}")
    if created_at:
        print(f"{C_BOLD}Added Date:{C_RESET} {created_at}")

    if notes:
        print(f"\n{C_BOLD}--- Notes ---{C_RESET}")
        print(notes)
    print(f"{C_BOLD}{C_GREEN}=========================================={C_RESET}\n")


def run_tests() -> None:
    """Executes the automated regression test suite."""
    from ierp.tests.test_ierp import run_tests as execute_suite
    print(f"\n{C_BOLD}{C_CYAN}=== Running iERP Automated Test Suite ==={C_RESET}\n")
    result = execute_suite()
    if result.wasSuccessful():
        print(f"\n{C_GREEN}{C_BOLD}All tests passed successfully!{C_RESET}\n")
    else:
        print(f"\n{C_RED}{C_BOLD}Some tests failed.{C_RESET}\n")
        sys.exit(1)


def main():
    parser = argparse.ArgumentParser(description="Personal Journal & CRM Event Log System CLI")
    subparsers = parser.add_subparsers(dest="command", help="Command to run")

    subparsers.add_parser("init", help="Initialize the SQLite database schema")

    sync_contacts_parser = subparsers.add_parser("sync-contacts", help="Sync contacts from Google Contacts ($0 API cost)")
    sync_contacts_parser.add_argument("--credentials", help="Path to Google Cloud OAuth credentials JSON")
    sync_contacts_parser.add_argument("--full", action="store_true", help="Force full sync (ignore incremental syncToken)")

    merge_contacts_parser = subparsers.add_parser("merge-contacts", help="Merge duplicate contacts")
    merge_contacts_parser.add_argument("--auto", action="store_true", help="Auto-detect and merge matching duplicate contacts")
    merge_contacts_parser.add_argument("--source-id", type=int, help="Source contact ID to merge and delete")
    merge_contacts_parser.add_argument("--target-id", type=int, help="Target contact ID to merge into and keep")

    import_timeline_parser = subparsers.add_parser("import-timeline", help="Import Google Maps Timeline JSON export")
    import_timeline_parser.add_argument("file", help="Path to Google Maps Timeline JSON")

    import_parser = subparsers.add_parser("import", help="Import events from Notion export directory")
    import_parser.add_argument("dir", help="Path to Notion export directory")

    import_crm_parser = subparsers.add_parser("import-crm", help="Import CRM contacts from Notion export directory")
    import_crm_parser.add_argument("dir", help="Path to CRM export directory")

    subparsers.add_parser("link", help="Manually run linking between events and contacts")

    insert_parser = subparsers.add_parser("insert", help="Insert a structured event directly")
    insert_parser.add_argument("--title", required=True, help="Event title")
    insert_parser.add_argument("--place", help="Event place/venue")
    insert_parser.add_argument("--start-date", help="Start date (YYYY-MM-DD)")
    insert_parser.add_argument("--end-date", help="End date")
    insert_parser.add_argument("--tags", help="Comma separated tags")
    insert_parser.add_argument("--url", help="Event URL")
    insert_parser.add_argument("--notes", help="Event notes/body content")

    list_parser = subparsers.add_parser("list", help="List recent events")
    list_parser.add_argument("--limit", type=int, default=20, help="Number of items to show")

    contacts_parser = subparsers.add_parser("contacts", help="List contacts")
    contacts_parser.add_argument("--source", choices=["manual", "google", "merged", "all"], default="all", help="Filter by contact source")

    vendors_parser = subparsers.add_parser("vendors", help="List vendors/sellers")
    vendors_parser.add_argument("--category", help="Filter by vendor category")
    vendors_parser.add_argument("--favorite", action="store_true", help="Show favorite vendors only")

    show_vendor_parser = subparsers.add_parser("show-vendor", help="Show full vendor details")
    show_vendor_parser.add_argument("id", type=int, help="Vendor database ID")

    insert_vendor_parser = subparsers.add_parser("insert-vendor", help="Insert a vendor/seller record directly")
    insert_vendor_parser.add_argument("--name", required=True, help="Vendor/business name")
    insert_vendor_parser.add_argument("--category", help="Category (e.g. Motorbike Rental, Accommodation, Cafe)")
    insert_vendor_parser.add_argument("--location", help="Vendor location/city")
    insert_vendor_parser.add_argument("--phone", help="Contact phone number")
    insert_vendor_parser.add_argument("--email", help="Contact email")
    insert_vendor_parser.add_argument("--url", help="Website URL")
    insert_vendor_parser.add_argument("--notes", help="Notes or description")
    insert_vendor_parser.add_argument("--favorite", action="store_true", help="Mark as favorite vendor")

    search_parser = subparsers.add_parser("search", help="Search events by keyword")
    search_parser.add_argument("query", help="Keyword query to search for")

    show_parser = subparsers.add_parser("show", help="Show full event details")
    show_parser.add_argument("id", type=int, help="Event database ID")

    show_contact_parser = subparsers.add_parser("show-contact", help="Show contact details and linked events")
    show_contact_parser.add_argument("id", type=int, help="Contact database ID")

    dashboard_parser = subparsers.add_parser("dashboard", help="Start web dashboard server with live GPS webhook receiver")
    dashboard_parser.add_argument("--port", type=int, default=8000, help="Port to run web server on (default: 8000)")
    dashboard_parser.add_argument("--no-browser", action="store_true", help="Do not open browser automatically")

    subparsers.add_parser("test", help="Run automated test suite")

    media_ingest_parser = subparsers.add_parser("ingest-media", help="Ingest normalized media records from JSON (stdin or file) - used by get-data")
    media_ingest_parser.add_argument("file", nargs="?", help="Path to JSON array of media records (default: stdin)")

    ingest_rows_parser = subparsers.add_parser("ingest-rows", help="Ingest RAW source rows from JSON (file or stdin); ierp does the normalization")
    ingest_rows_parser.add_argument("source", help="Source key (hardcover, goodreads, letterboxd, anilist_anime, anilist_manga, mydramalist)")
    ingest_rows_parser.add_argument("file", nargs="?", help="Path to JSON array of raw row objects (default: stdin)")

    serve_ingest_parser = subparsers.add_parser("serve-ingest", help="Start local HTTP ingestion server (POST /ingest/<source_key> with raw rows JSON)")
    serve_ingest_parser.add_argument("--port", type=int, default=8765)

    sync_parser = subparsers.add_parser("sync", help="Fetch media from trackers and ingest (all sources or --source)")
    sync_parser.add_argument("--source", action="append", dest="sources",
                             help="Sync only this source (repeatable: --source goodreads --source letterboxd)")
    sync_parser.add_argument("--list", action="store_true", help="List configured sources")

    media_list_parser = subparsers.add_parser("media", help="List recent media logs")
    media_list_parser.add_argument("--type", help="Filter by media_type (book, film, anime, manga, drama)")
    media_list_parser.add_argument("--limit", type=int, default=20)

    insert_link_parser = subparsers.add_parser("insert-link", help="Insert/upsert a link record")
    insert_link_parser.add_argument("--label", required=True)
    insert_link_parser.add_argument("--url", required=True)
    insert_link_parser.add_argument("--category", help="Category (e.g. social, profile, reference)")
    insert_link_parser.add_argument("--private", action="store_true", help="Exclude from public garden export")
    insert_link_parser.add_argument("--notes", help="Notes")

    links_parser = subparsers.add_parser("links", help="List links")
    links_parser.add_argument("--category", help="Filter by category")
    links_parser.add_argument("--public-only", action="store_true")

    import_pay_parser = subparsers.add_parser("import-pay", help="Import payment accounts from portfolio pay.json")
    import_pay_parser.add_argument("file", help="Path to pay.json")

    import_referrals_parser = subparsers.add_parser("import-referrals", help="Import referrals from portfolio referrals.json")
    import_referrals_parser.add_argument("file", help="Path to referrals.json")

    pay_parser = subparsers.add_parser("pay", help="List payment accounts")
    pay_parser.add_argument("--category", help="Filter by category")

    referrals_parser = subparsers.add_parser("referrals", help="List referral codes")
    referrals_parser.add_argument("--category", help="Filter by category")
    referrals_parser.add_argument("--status", help="Filter by status (ACTIVE, OFFLINE, ...)")
    referrals_parser.add_argument("--public-only", action="store_true")

    args = parser.parse_args()

    if args.command == "init":
        init_db(verbose=True)
    elif args.command == "sync-contacts":
        sync_google_contacts(custom_creds=args.credentials, full_resync=args.full)
    elif args.command == "merge-contacts":
        if args.source_id and args.target_id:
            merge_two_contacts(args.source_id, args.target_id)
        else:
            auto_merge_contacts()
    elif args.command == "import-timeline":
        import_timeline(args.file)
    elif args.command == "import":
        import_notion_export(args.dir)
    elif args.command == "import-crm":
        import_crm_contacts(args.dir)
    elif args.command == "link":
        run_manual_link()
    elif args.command == "insert":
        insert_event_direct(args.title, args.place, args.start_date, args.end_date, args.tags, args.url, args.notes)
    elif args.command == "list":
        list_events(args.limit)
    elif args.command == "contacts":
        list_contacts(source_filter=args.source)
    elif args.command == "vendors":
        list_vendors(category_filter=args.category, favorite_only=args.favorite)
    elif args.command == "show-vendor":
        show_vendor(args.id)
    elif args.command == "insert-vendor":
        insert_vendor_direct(
            args.name,
            category=args.category,
            location=args.location,
            phone=args.phone,
            email=args.email,
            url=args.url,
            notes=args.notes,
            favorite=args.favorite
        )
    elif args.command == "search":
        search_events(args.query)
    elif args.command == "show":
        show_event(args.id)
    elif args.command == "show-contact":
        show_contact(args.id)
    elif args.command == "dashboard":
        start_dashboard_server(port=args.port, open_browser=not args.no_browser)
    elif args.command == "test":
        run_tests()
    elif args.command == "ingest-media":
        init_db()
        if args.file:
            with open(args.file, "r", encoding="utf-8") as f:
                records = json.load(f)
        else:
            records = json.load(sys.stdin)
        result = ingest_media_records(records)
        print(f"{C_GREEN}Ingested {result['items']} media items / {result['logs']} logs.{C_RESET}")
    elif args.command == "ingest-rows":
        if args.file:
            with open(args.file, "r", encoding="utf-8") as f:
                rows = json.load(f)
        else:
            rows = json.load(sys.stdin)
        result = ingest_rows(args.source, rows)
        print(f"{C_GREEN}Ingested {result['items']} media items / {result['logs']} logs "
              f"({result['skipped']} skipped) from {args.source}.{C_RESET}")
    elif args.command == "serve-ingest":
        serve_ingest(port=args.port)
    elif args.command == "sync":
        if args.list:
            print("Configured media sources:")
            for key, profile in MEDIA_PROFILES.items():
                print(f"  - {key:15} {profile}")
            return
        results = sync_all(MEDIA_PROFILES, sources=args.sources)
        failed = 0
        for r in results:
            if r["ok"]:
                print(f"{C_GREEN}✅ {r['source']:15} {r['rows']:4d} rows ingested{C_RESET}")
            else:
                failed += 1
                print(f"{C_RED}❌ {r['source']:15} {r['error']}{C_RESET}")
        if failed:
            sys.exit(1)
    elif args.command == "media":
        rows = list_media(media_type=args.type, limit=args.limit)
        if not rows:
            print("No media found.")
        else:
            print(f"\n{C_BOLD}{'ID':<6} | {'Type':<8} | {'Title':<34} | {'Status':<12} | {'Rating':<6} | {'Date':<10}{C_RESET}")
            print("-" * 90)
            for mid, mtype, title, status, rating, dlog, fin in rows:
                date = (dlog or fin or "")[:10]
                rating_s = f"{rating:g}" if rating is not None else ""
                print(f"{mid:<6} | {(mtype or ''):<8} | {title[:34]:<34} | {(status or '')[:12]:<12} | {rating_s:<6} | {date:<10}")
            print()
    elif args.command == "insert-link":
        init_db()
        lid = upsert_link(args.label, args.url, category=args.category,
                          is_public=not args.private, notes=args.notes)
        print(f"{C_GREEN}Link #{lid} saved: {args.label}{C_RESET}")
    elif args.command == "links":
        rows = list_links(category=args.category, public_only=args.public_only)
        if not rows:
            print("No links found.")
        else:
            print(f"\n{C_BOLD}{'ID':<5} | {'Label':<28} | {'Category':<12} | {'Public':<6} | URL{C_RESET}")
            print("-" * 100)
            for lid, label, url, cat, pub in rows:
                print(f"{lid:<5} | {label[:28]:<28} | {(cat or '')[:12]:<12} | {'yes' if pub else 'no':<6} | {url}")
            print()
    elif args.command == "import-pay":
        init_db()
        with open(args.file, "r", encoding="utf-8") as f:
            items = json.load(f)
        conn = get_db()
        cur = conn.cursor()
        init_commerce_tables(cur)
        for it in items:
            upsert_payment_account(cur, slug=it.get("id") or it.get("name"), name=it["name"],
                                   category=it.get("category"), number=it.get("number"),
                                   recipient=it.get("recipient"), details=it.get("details"),
                                   details_id=it.get("details_id"))
        conn.commit(); conn.close()
        print(f"{C_GREEN}Imported {len(items)} payment accounts.{C_RESET}")
    elif args.command == "import-referrals":
        init_db()
        with open(args.file, "r", encoding="utf-8") as f:
            items = json.load(f)
        conn = get_db()
        cur = conn.cursor()
        init_commerce_tables(cur)
        for it in items:
            upsert_referral(cur, slug=it.get("id") or it.get("name"), name=it["name"],
                            category=it.get("category"), code=it.get("code"),
                            link=it.get("link"), benefit=it.get("benefit"),
                            status=it.get("status"))
        conn.commit(); conn.close()
        print(f"{C_GREEN}Imported {len(items)} referrals.{C_RESET}")
    elif args.command == "pay":
        rows = list_payment_accounts(category=args.category)
        if not rows:
            print("No payment accounts found.")
        else:
            print(f"\n{C_BOLD}{'ID':<4} | {'Slug':<12} | {'Name':<32} | {'Category':<20} | {'Number':<18} | Recipient{C_RESET}")
            print("-" * 110)
            for pid, slug, name, cat, number, recipient in rows:
                print(f"{pid:<4} | {(slug or '')[:12]:<12} | {name[:32]:<32} | {(cat or '')[:20]:<20} | {(number or '')[:18]:<18} | {recipient or ''}")
            print()
    elif args.command == "referrals":
        rows = list_referrals(category=args.category, status=args.status, public_only=args.public_only)
        if not rows:
            print("No referrals found.")
        else:
            print(f"\n{C_BOLD}{'ID':<4} | {'Name':<18} | {'Category':<22} | {'Status':<8} | {'Code':<22} | Link{C_RESET}")
            print("-" * 110)
            for rid, slug, name, cat, code, link, benefit, status, pub in rows:
                print(f"{rid:<4} | {name[:18]:<18} | {(cat or '')[:22]:<22} | {(status or '')[:8]:<8} | {(code or '')[:22]:<22} | {link or ''}")
            print()
    else:
        parser.print_help()


if __name__ == "__main__":
    main()
