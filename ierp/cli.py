#!/usr/bin/env python3
"""
CLI entrypoint for Personal Journal & CRM Event Log System (iERP).
Supports structured logging, Google Contacts/Timeline sync, merging, and interactive dashboard.
Uses idiomatic argparse subparser dispatching with zero external dependencies.
"""

import argparse
import json
import os
import sys
from pathlib import Path

# Ensure repository root is on sys.path
BASE_DIR = Path(__file__).resolve().parent
REPO_ROOT = BASE_DIR.parent
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))
if str(BASE_DIR) not in sys.path:
    sys.path.insert(0, str(BASE_DIR))

from ierp.core.commerce import (
    init_tables as init_commerce_tables,
    list_payment_accounts,
    list_referrals,
    upsert_payment_account,
    upsert_referral,
)
from ierp.core.config import (
    C_BOLD,
    C_CYAN,
    C_GREEN,
    C_MAGENTA,
    C_RED,
    C_RESET,
    C_YELLOW,
    DB_PATH,
    media_profiles,
)
from ierp.core.contacts import get_contact, list_contacts
from ierp.core.dashboard import start_dashboard_server
from ierp.core.db import get_db, init_db
from ierp.core.events import get_event, insert_event, list_events as query_events, search_events as query_search_events
from ierp.core.google_sync import sync_google_contacts
from ierp.core.importers import import_crm_contacts, import_notion_export, import_timeline
from ierp.core.ingest import ingest_rows, serve_ingest
from ierp.core.linking import run_manual_link
from ierp.core.media import ingest_media_records, list_links, list_media, upsert_link
from ierp.core.merging import auto_merge_contacts, merge_two_contacts
from ierp.core.receipts import compute_balance, get_receipt, list_receipts, upsert_receipt
from ierp.core.sync import sync_all
from ierp.core.vendors import get_vendor, insert_vendor, list_vendors


# -----------------------------------------------------------------------------
# Handlers: Database & Core Initialization
# -----------------------------------------------------------------------------

def handle_init(args: argparse.Namespace) -> None:
    init_db(verbose=True)


def handle_sync_contacts(args: argparse.Namespace) -> None:
    sync_google_contacts(custom_creds=args.credentials, full_resync=args.full)


def handle_merge_contacts(args: argparse.Namespace) -> None:
    if args.source_id and args.target_id:
        merge_two_contacts(args.source_id, args.target_id)
    else:
        auto_merge_contacts()


def handle_import_timeline(args: argparse.Namespace) -> None:
    import_timeline(args.file)


def handle_import(args: argparse.Namespace) -> None:
    import_notion_export(args.dir)


def handle_import_crm(args: argparse.Namespace) -> None:
    import_crm_contacts(args.dir)


def handle_link(args: argparse.Namespace) -> None:
    run_manual_link()


# -----------------------------------------------------------------------------
# Handlers: Events
# -----------------------------------------------------------------------------

def handle_insert_event(args: argparse.Namespace) -> None:
    try:
        ev_id, linked_names = insert_event(
            title=args.title,
            place=args.place,
            start_date=args.start_date,
            end_date=args.end_date,
            tags=args.tags,
            url=args.url,
            notes=args.notes,
            contacts=args.contacts,
        )
        print(f"{C_GREEN}Successfully inserted event #{ev_id}: {args.title}{C_RESET}")
        if linked_names:
            print(f"{C_GREEN}Explicitly linked: {', '.join(linked_names)}{C_RESET}")
    except Exception as e:
        print(f"{C_RED}Failed to insert event: {e}{C_RESET}")


def handle_list_events(args: argparse.Namespace) -> None:
    if not DB_PATH.exists():
        print(f"{C_RED}Database does not exist.{C_RESET}")
        return

    items, _ = query_events(limit=args.limit)
    if not items:
        print("No events found.")
        return

    print(f"\n{C_BOLD}{'ID':<5} | {'Date':<19} | {'Title':<30} | {'Place':<20} | {'Tags':<20}{C_RESET}")
    print("-" * 105)
    for ev in items:
        tags_str = ", ".join(ev["tags"])
        place = ev["place"] or ""
        start = ev["start_date"] or "No Date"
        title = ev["title"] or ""
        print(f"{ev['id']:<5} | {start[:19]:<19} | {title[:30]:<30} | {place[:20]:<20} | {tags_str[:20]:<20}")
    print()


def handle_search_events(args: argparse.Namespace) -> None:
    if not DB_PATH.exists():
        print(f"{C_RED}Database does not exist.{C_RESET}")
        return

    results = query_search_events(args.query)
    if not results:
        print(f"No events matching '{args.query}' found.")
        return

    print(f"\n{C_BOLD}Search results for '{args.query}':{C_RESET}")
    print(f"{C_BOLD}{'ID':<5} | {'Date':<19} | {'Title':<30} | {'Place':<20} | {'Tags':<20}{C_RESET}")
    print("-" * 105)
    for ev in results:
        tags_str = ", ".join(ev["tags"])
        place = ev["place"] or ""
        start = ev["start_date"] or "No Date"
        title = ev["title"] or ""
        print(f"{ev['id']:<5} | {start[:19]:<19} | {title[:30]:<30} | {place[:20]:<20} | {tags_str[:20]:<20}")
    print()


def handle_show_event(args: argparse.Namespace) -> None:
    if not DB_PATH.exists():
        print(f"{C_RED}Database does not exist.{C_RESET}")
        return

    ev = get_event(args.id)
    if not ev:
        print(f"{C_RED}Event with ID {args.id} not found.{C_RESET}")
        return

    tags_str = ", ".join(ev["tags"])
    start = ev["start_date"] or "No Date"
    end = ev["end_date"]
    raw_date = ev["raw_date"]

    print(f"\n{C_BOLD}{C_MAGENTA}=== Event Details (ID: {args.id}) ==={C_RESET}")
    print(f"{C_BOLD}Title:{C_RESET}      {ev['title']}")
    if ev.get("place"):
        print(f"{C_BOLD}Place:{C_RESET}      {ev['place']}")
    print(f"{C_BOLD}Date:{C_RESET}       {start} {f'→ {end}' if end else ''} (Original: {raw_date or 'None'})")
    if tags_str:
        print(f"{C_BOLD}Tags:{C_RESET}       {tags_str}")
    if ev.get("url"):
        print(f"{C_BOLD}URL:{C_RESET}        {ev['url']}")

    if ev.get("contacts"):
        contacts_str = ", ".join(f"{c['name']} (ID: {c['id']})" for c in ev["contacts"])
        print(f"{C_BOLD}Linked People:{C_RESET} {contacts_str}")

    if ev.get("media"):
        print(f"{C_BOLD}Attachments:{C_RESET}")
        for m in ev["media"]:
            print(f"  - {m['original_filename']} ({m['stored_path']})")

    print(f"\n{C_BOLD}--- Notes / Markdown Content ---{C_RESET}")
    if ev.get("notes"):
        print(ev["notes"])
    else:
        print(f"{C_YELLOW}(No notes content){C_RESET}")
    print(f"{C_BOLD}{C_MAGENTA}================================={C_RESET}\n")


# -----------------------------------------------------------------------------
# Handlers: Contacts
# -----------------------------------------------------------------------------

def handle_list_contacts(args: argparse.Namespace) -> None:
    init_db()
    items, _ = list_contacts(source_filter=args.source, limit=500)
    if not items:
        print(f"No contacts found{' with source ' + args.source if args.source else ''}.")
        return

    print(f"\n{C_BOLD}{'ID':<5} | {'Name':<22} | {'Source':<10} | {'Org/Title':<18} | {'Client':<12} | {'Email/Phone':<22}{C_RESET}")
    print("-" * 105)
    for c in items:
        src_label = (c["source"] or "manual").capitalize()
        contact_info = c["email"] or c["phone"] or ""
        print(f"{c['id']:<5} | {c['name'][:22]:<22} | {src_label:<10} | {(c['org'] or '')[:18]:<18} | {(c['client'] or '')[:12]:<12} | {contact_info[:22]:<22}")
    print()


def handle_show_contact(args: argparse.Namespace) -> None:
    init_db()
    c = get_contact(args.id)
    if not c:
        print(f"{C_RED}Contact with ID {args.id} not found.{C_RESET}")
        return

    source_display = (c["source"] or "manual").capitalize()
    if c.get("is_google_linked") and c["source"] != "google":
        source_display = f"{source_display} (Google Synced)"

    print(f"\n{C_BOLD}{C_CYAN}=== Contact Details (ID: {args.id}) ==={C_RESET}")
    print(f"{C_BOLD}Name:{C_RESET}       {c['name']}")
    print(f"{C_BOLD}Source:{C_RESET}     {source_display}")
    if c.get("email"):
        print(f"{C_BOLD}Email:{C_RESET}      {c['email']}")
    if c.get("phone"):
        print(f"{C_BOLD}Phone:{C_RESET}      {c['phone']}")
    if c.get("org"):
        print(f"{C_BOLD}Org/Role:{C_RESET}   {c['org']}")
    if c.get("client"):
        print(f"{C_BOLD}Client:{C_RESET}     {c['client']}")
    if c.get("location"):
        print(f"{C_BOLD}Location:{C_RESET}   {c['location']}")
    if c.get("date"):
        print(f"{C_BOLD}Added Date:{C_RESET} {c['date']}")

    if c.get("events"):
        print(f"\n{C_BOLD}Linked Events ({len(c['events'])}):{C_RESET}")
        for ev in c["events"]:
            estart = ev["start_date"] or "No Date"
            print(f"  - [{ev['id']}] {estart[:10]} | {ev['title']}")
    else:
        print(f"\n{C_YELLOW}No linked events found for this contact.{C_RESET}")

    if c.get("notes"):
        print(f"\n{C_BOLD}--- Notes ---{C_RESET}")
        print(c["notes"])
    print(f"{C_BOLD}{C_CYAN}=========================================={C_RESET}\n")


# -----------------------------------------------------------------------------
# Handlers: Vendors
# -----------------------------------------------------------------------------

def handle_list_vendors(args: argparse.Namespace) -> None:
    init_db()
    items, _, _ = list_vendors(category_filter=args.category, favorite_only=args.favorite, limit=500)
    if not items:
        print(f"No vendors found{' in category ' + args.category if args.category else ''}.")
        return

    print(f"\n{C_BOLD}{'ID':<5} | {'Name':<24} | {'Category':<18} | {'Location':<18} | {'Phone':<20} | {'Fav':<5}{C_RESET}")
    print("-" * 100)
    for v in items:
        fav_star = "★ Yes" if v["favorite"] else "No"
        print(f"{v['id']:<5} | {v['name'][:24]:<24} | {(v['category'] or '')[:18]:<18} | {(v['location'] or '')[:18]:<18} | {(v['phone'] or '')[:20]:<20} | {fav_star:<5}")
    print()


def handle_show_vendor(args: argparse.Namespace) -> None:
    init_db()
    v = get_vendor(args.id)
    if not v:
        print(f"{C_RED}Vendor with ID {args.id} not found.{C_RESET}")
        return

    fav_display = f"{C_YELLOW}★ Favorite{C_RESET}" if v["favorite"] else "Standard"

    print(f"\n{C_BOLD}{C_GREEN}=== Vendor Details (ID: {args.id}) ==={C_RESET}")
    print(f"{C_BOLD}Name:{C_RESET}       {v['name']}")
    print(f"{C_BOLD}Status:{C_RESET}     {fav_display}")
    if v.get("category"):
        print(f"{C_BOLD}Category:{C_RESET}   {v['category']}")
    if v.get("location"):
        print(f"{C_BOLD}Location:{C_RESET}   {v['location']}")
    if v.get("phone"):
        print(f"{C_BOLD}Phone:{C_RESET}      {v['phone']}")
    if v.get("email"):
        print(f"{C_BOLD}Email:{C_RESET}      {v['email']}")
    if v.get("url"):
        print(f"{C_BOLD}URL:{C_RESET}        {v['url']}")
    if v.get("source"):
        print(f"{C_BOLD}Source:{C_RESET}     {(v['source'] or 'manual').capitalize()}")
    if v.get("created_at"):
        print(f"{C_BOLD}Added Date:{C_RESET} {v['created_at']}")

    if v.get("notes"):
        print(f"\n{C_BOLD}--- Notes ---{C_RESET}")
        print(v["notes"])
    print(f"{C_BOLD}{C_GREEN}=========================================={C_RESET}\n")


def handle_insert_vendor(args: argparse.Namespace) -> None:
    try:
        vid = insert_vendor(
            name=args.name,
            category=args.category,
            location=args.location,
            phone=args.phone,
            email=args.email,
            url=args.url,
            notes=args.notes,
            favorite=args.favorite,
        )
        print(f"{C_GREEN}Successfully inserted vendor #{vid}: {args.name}{C_RESET}")
    except Exception as e:
        print(f"{C_RED}Failed to insert vendor: {e}{C_RESET}")


# -----------------------------------------------------------------------------
# Handlers: Receipts & Balances
# -----------------------------------------------------------------------------

def handle_insert_receipt(args: argparse.Namespace) -> None:
    init_db()
    ev = get_event(args.event_id)
    if not ev:
        print(f"{C_RED}Event ID {args.event_id} not found.{C_RESET}")
        return

    try:
        conn = get_db()
        cur = conn.cursor()
        rid = upsert_receipt(
            cur,
            event_id=args.event_id,
            amount=args.amount,
            type=args.type,
            status=args.status,
            notes=args.notes,
            receipt_id=args.id,
        )
        conn.commit()
        conn.close()
        action_verb = "updated" if args.id else "inserted"
        print(f"{C_GREEN}Receipt #{rid} {action_verb}: Rp{args.amount:,.0f} ({args.type}/{args.status}) for event #{args.event_id} — {ev['title']}{C_RESET}")
    except ValueError as e:
        print(f"{C_RED}{e}{C_RESET}")


def handle_list_receipts(args: argparse.Namespace) -> None:
    rows = list_receipts(event_id=args.event_id, type=args.type, status=args.status)
    if not rows:
        print("No receipts found.")
        return
    rows = rows[:args.limit]
    print(f"\n{C_BOLD}{'ID':<5} | {'Event':<5} | {'Amount':<14} | {'Type':<9} | {'Status':<8} | {'Date':<12} | Event Title{C_RESET}")
    print("-" * 105)
    for r in rows:
        ev_title = (r.get("event_title") or "")[:24]
        date = (r.get("event_date") or "")[:12]
        print(f"{r['id']:<5} | {r['event_id']:<5} | Rp{r['amount']:>11,.0f} | {r['type']:<9} | {r['status']:<8} | {date:<12} | {ev_title}")
    print()


def handle_show_receipt(args: argparse.Namespace) -> None:
    r = get_receipt(args.id)
    if not r:
        print(f"{C_RED}Receipt with ID {args.id} not found.{C_RESET}")
        return
    print(f"\n{C_BOLD}{C_GREEN}=== Receipt #{r['id']} ==={C_RESET}")
    print(f"{C_BOLD}Event ID:{C_RESET}  {r['event_id']}")
    if r.get("event_title"):
        print(f"{C_BOLD}Event Title:{C_RESET} {r['event_title']}")
    if r.get("event_date"):
        print(f"{C_BOLD}Event Date:{C_RESET}  {r['event_date'][:10]}")
    print(f"{C_BOLD}Amount:{C_RESET}    Rp{r['amount']:,.0f}")
    print(f"{C_BOLD}Type:{C_RESET}      {r['type']}")
    print(f"{C_BOLD}Status:{C_RESET}   {r['status']}")
    if r.get("notes"):
        print(f"\n{C_BOLD}--- Notes ---{C_RESET}")
        print(r["notes"])
    print(f"\n{C_BOLD}Created:{C_RESET}  {r['created_at']}")
    if r.get("updated_at"):
        print(f"{C_BOLD}Updated:{C_RESET}  {r['updated_at']}")
    print(f"{C_BOLD}{C_GREEN}========================{C_RESET}\n")


def handle_balance(args: argparse.Namespace) -> None:
    bal = compute_balance(event_id=args.event_id)
    title = f"=== Financial Position (Event #{args.event_id}) ===" if args.event_id else "=== Financial Position ==="
    print(f"\n{C_BOLD}{C_GREEN}{title}{C_RESET}\n")
    print(f"  {C_BOLD}Total Income:{C_RESET}    Rp{bal['total_income']:,.0f}")
    print(f"  {C_BOLD}Total Costs:{C_RESET}     Rp{bal['total_costs']:,.0f}")
    print(f"  {C_BOLD}Total Expected:{C_RESET}  Rp{bal['total_expected']:,.0f}")
    print(f"  {C_BOLD}Outstanding:{C_RESET}     Rp{bal['outstanding']:,.0f}")
    print(f"\n  {C_BOLD}Net Cash:{C_RESET}       Rp{bal['net_cash']:,.0f}")
    print(f"  {C_BOLD}Net Position:{C_RESET}    Rp{bal['net_position']:,.0f}")
    print(f"\n{C_BOLD}{C_GREEN}{'=' * len(title)}{C_RESET}\n")


# -----------------------------------------------------------------------------
# Handlers: Media, Links & Commerce
# -----------------------------------------------------------------------------

def handle_ingest_media(args: argparse.Namespace) -> None:
    init_db()
    if args.file:
        with open(args.file, "r", encoding="utf-8") as f:
            records = json.load(f)
    else:
        records = json.load(sys.stdin)
    result = ingest_media_records(records)
    print(f"{C_GREEN}Ingested {result['items']} media items.{C_RESET}")


def handle_ingest_rows(args: argparse.Namespace) -> None:
    if args.file:
        with open(args.file, "r", encoding="utf-8") as f:
            rows = json.load(f)
    else:
        rows = json.load(sys.stdin)
    result = ingest_rows(args.source, rows)
    print(f"{C_GREEN}Ingested {result['items']} media items ({result['skipped']} skipped) from {args.source}.{C_RESET}")


def handle_serve_ingest(args: argparse.Namespace) -> None:
    serve_ingest(port=args.port)


def handle_sync(args: argparse.Namespace) -> None:
    from ierp.core.fetchers import _load_env
    _load_env()
    profiles = media_profiles()
    if args.list:
        print("Configured media sources (env override: IERP_<SOURCE>__<FIELD>):")
        for key, profile in profiles.items():
            print(f"  - {key:15} {profile}")
        return

    results = sync_all(profiles, sources=args.sources)
    failed = 0
    for r in results:
        if r["ok"]:
            print(f"{C_GREEN}✅ {r['source']:15} {r['rows']:4d} rows ingested{C_RESET}")
        else:
            failed += 1
            print(f"{C_RED}❌ {r['source']:15} {r['error']}{C_RESET}")
    if failed:
        sys.exit(1)


def handle_media(args: argparse.Namespace) -> None:
    rows = list_media(media_type=args.type, limit=args.limit)
    if not rows:
        print("No media found.")
        return

    print(f"\n{C_BOLD}{'ID':<6} | {'Type':<8} | {'Title':<34} | {'Status':<12} | {'Rating':<6} | {'Date':<10}{C_RESET}")
    print("-" * 90)
    for mid, mtype, title, rating, dlog in rows:
        date = (dlog or "")[:10]
        rating_s = f"{rating:g}" if rating is not None else ""
        print(f"{mid:<6} | {(mtype or ''):<8} | {title[:34]:<34} | {'':<12} | {rating_s:<6} | {date:<10}")
    print()


def handle_insert_link(args: argparse.Namespace) -> None:
    init_db()
    lid = upsert_link(args.label, args.url, category=args.category, is_public=not args.private, notes=args.notes)
    print(f"{C_GREEN}Link #{lid} saved: {args.label}{C_RESET}")


def handle_links(args: argparse.Namespace) -> None:
    rows = list_links(category=args.category, public_only=args.public_only)
    if not rows:
        print("No links found.")
        return

    print(f"\n{C_BOLD}{'ID':<5} | {'Label':<28} | {'Category':<12} | {'Public':<6} | URL{C_RESET}")
    print("-" * 100)
    for lid, label, url, cat, pub in rows:
        print(f"{lid:<5} | {label[:28]:<28} | {(cat or '')[:12]:<12} | {'yes' if pub else 'no':<6} | {url}")
    print()


def handle_import_pay(args: argparse.Namespace) -> None:
    init_db()
    with open(args.file, "r", encoding="utf-8") as f:
        items = json.load(f)
    conn = get_db()
    cur = conn.cursor()
    init_commerce_tables(cur)
    for it in items:
        upsert_payment_account(
            cur,
            slug=it.get("id") or it.get("name"),
            name=it["name"],
            category=it.get("category"),
            number=it.get("number"),
            recipient=it.get("recipient"),
            details=it.get("details"),
            details_id=it.get("details_id"),
        )
    conn.commit()
    conn.close()
    print(f"{C_GREEN}Imported {len(items)} payment accounts.{C_RESET}")


def handle_import_referrals(args: argparse.Namespace) -> None:
    init_db()
    with open(args.file, "r", encoding="utf-8") as f:
        items = json.load(f)
    conn = get_db()
    cur = conn.cursor()
    init_commerce_tables(cur)
    for it in items:
        upsert_referral(
            cur,
            slug=it.get("id") or it.get("name"),
            name=it["name"],
            category=it.get("category"),
            code=it.get("code"),
            link=it.get("link"),
            benefit=it.get("benefit"),
            status=it.get("status"),
        )
    conn.commit()
    conn.close()
    print(f"{C_GREEN}Imported {len(items)} referrals.{C_RESET}")


def handle_pay(args: argparse.Namespace) -> None:
    rows = list_payment_accounts(category=args.category)
    if not rows:
        print("No payment accounts found.")
        return

    print(f"\n{C_BOLD}{'ID':<4} | {'Slug':<12} | {'Name':<32} | {'Category':<20} | {'Number':<18} | Recipient{C_RESET}")
    print("-" * 110)
    for pid, slug, name, cat, number, recipient in rows:
        print(f"{pid:<4} | {(slug or '')[:12]:<12} | {name[:32]:<32} | {(cat or '')[:20]:<20} | {(number or '')[:18]:<18} | {recipient or ''}")
    print()


def handle_referrals(args: argparse.Namespace) -> None:
    rows = list_referrals(category=args.category, status=args.status, public_only=args.public_only)
    if not rows:
        print("No referrals found.")
        return

    print(f"\n{C_BOLD}{'ID':<4} | {'Name':<18} | {'Category':<22} | {'Status':<8} | {'Code':<22} | Link{C_RESET}")
    print("-" * 110)
    for rid, slug, name, cat, code, link, benefit, status, pub in rows:
        print(f"{rid:<4} | {name[:18]:<18} | {(cat or '')[:22]:<22} | {(status or '')[:8]:<8} | {(code or '')[:22]:<22} | {link or ''}")
    print()


# -----------------------------------------------------------------------------
# Handlers: Dashboard & Tests
# -----------------------------------------------------------------------------

def handle_dashboard(args: argparse.Namespace) -> None:
    start_dashboard_server(port=args.port, open_browser=not args.no_browser)


def handle_test(args: argparse.Namespace) -> None:
    from ierp.tests.test_ierp import run_tests as execute_suite
    print(f"\n{C_BOLD}{C_CYAN}=== Running iERP Automated Test Suite ==={C_RESET}\n")
    result = execute_suite()
    if result.wasSuccessful():
        print(f"\n{C_GREEN}{C_BOLD}All tests passed successfully!{C_RESET}\n")
    else:
        print(f"\n{C_RED}{C_BOLD}Some tests failed.{C_RESET}\n")
        sys.exit(1)


# -----------------------------------------------------------------------------
# CLI Parser Setup & Dispatch
# -----------------------------------------------------------------------------

def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Personal Journal & CRM Event Log System CLI")
    subparsers = parser.add_subparsers(dest="command", help="Command to run")

    # init
    p_init = subparsers.add_parser("init", help="Initialize the SQLite database schema")
    p_init.set_defaults(func=handle_init)

    # sync-contacts
    p_sync_contacts = subparsers.add_parser("sync-contacts", help="Sync contacts from Google Contacts ($0 API cost)")
    p_sync_contacts.add_argument("--credentials", help="Path to Google Cloud OAuth credentials JSON")
    p_sync_contacts.add_argument("--full", action="store_true", help="Force full sync (ignore incremental syncToken)")
    p_sync_contacts.set_defaults(func=handle_sync_contacts)

    # merge-contacts
    p_merge = subparsers.add_parser("merge-contacts", help="Merge duplicate contacts")
    p_merge.add_argument("--auto", action="store_true", help="Auto-detect and merge matching duplicate contacts")
    p_merge.add_argument("--source-id", type=int, help="Source contact ID to merge and delete")
    p_merge.add_argument("--target-id", type=int, help="Target contact ID to merge into and keep")
    p_merge.set_defaults(func=handle_merge_contacts)

    # imports & links
    p_timeline = subparsers.add_parser("import-timeline", help="Import Google Maps Timeline JSON export")
    p_timeline.add_argument("file", help="Path to Google Maps Timeline JSON")
    p_timeline.set_defaults(func=handle_import_timeline)

    p_import = subparsers.add_parser("import", help="Import events from Notion export directory")
    p_import.add_argument("dir", help="Path to Notion export directory")
    p_import.set_defaults(func=handle_import)

    p_import_crm = subparsers.add_parser("import-crm", help="Import CRM contacts from Notion export directory")
    p_import_crm.add_argument("dir", help="Path to CRM export directory")
    p_import_crm.set_defaults(func=handle_import_crm)

    p_link = subparsers.add_parser("link", help="Manually run linking between events and contacts")
    p_link.set_defaults(func=handle_link)

    # events
    p_insert = subparsers.add_parser("insert", help="Insert a structured event directly")
    p_insert.add_argument("--title", required=True, help="Event title")
    p_insert.add_argument("--place", help="Event place/venue")
    p_insert.add_argument("--start-date", help="Start date (YYYY-MM-DD)")
    p_insert.add_argument("--end-date", help="End date")
    p_insert.add_argument("--tags", help="Comma separated tags")
    p_insert.add_argument("--url", help="Event URL")
    p_insert.add_argument("--notes", help="Event notes/body content")
    p_insert.add_argument("--contact", action="append", dest="contacts", help="Explicitly link this contact (name or ID; repeatable)")
    p_insert.set_defaults(func=handle_insert_event)

    p_list = subparsers.add_parser("list", help="List recent events")
    p_list.add_argument("--limit", type=int, default=20, help="Number of items to show")
    p_list.set_defaults(func=handle_list_events)

    p_search = subparsers.add_parser("search", help="Search events by keyword")
    p_search.add_argument("query", help="Keyword query to search for")
    p_search.set_defaults(func=handle_search_events)

    p_show = subparsers.add_parser("show", help="Show full event details")
    p_show.add_argument("id", type=int, help="Event database ID")
    p_show.set_defaults(func=handle_show_event)

    # contacts
    p_contacts = subparsers.add_parser("contacts", help="List contacts")
    p_contacts.add_argument("--source", choices=["manual", "google", "merged", "all"], default="all", help="Filter by contact source")
    p_contacts.set_defaults(func=handle_list_contacts)

    p_show_contact = subparsers.add_parser("show-contact", help="Show contact details and linked events")
    p_show_contact.add_argument("id", type=int, help="Contact database ID")
    p_show_contact.set_defaults(func=handle_show_contact)

    # vendors
    p_vendors = subparsers.add_parser("vendors", help="List vendors/sellers")
    p_vendors.add_argument("--category", help="Filter by vendor category")
    p_vendors.add_argument("--favorite", action="store_true", help="Show favorite vendors only")
    p_vendors.set_defaults(func=handle_list_vendors)

    p_show_vendor = subparsers.add_parser("show-vendor", help="Show full vendor details")
    p_show_vendor.add_argument("id", type=int, help="Vendor database ID")
    p_show_vendor.set_defaults(func=handle_show_vendor)

    p_insert_vendor = subparsers.add_parser("insert-vendor", help="Insert a vendor/seller record directly")
    p_insert_vendor.add_argument("--name", required=True, help="Vendor/business name")
    p_insert_vendor.add_argument("--category", help="Category (e.g. Motorbike Rental, Accommodation, Cafe)")
    p_insert_vendor.add_argument("--location", help="Vendor location/city")
    p_insert_vendor.add_argument("--phone", help="Contact phone number")
    p_insert_vendor.add_argument("--email", help="Contact email")
    p_insert_vendor.add_argument("--url", help="Website URL")
    p_insert_vendor.add_argument("--notes", help="Notes or description")
    p_insert_vendor.add_argument("--favorite", action="store_true", help="Mark as favorite vendor")
    p_insert_vendor.set_defaults(func=handle_insert_vendor)

    # receipts
    p_insert_receipt = subparsers.add_parser("insert-receipt", help="Record or update a monetary receipt against an event")
    p_insert_receipt.add_argument("--id", type=int, help="Receipt ID (if updating existing receipt)")
    p_insert_receipt.add_argument("--event-id", type=int, required=True, help="Event ID to link this receipt to")
    p_insert_receipt.add_argument("--amount", type=float, required=True, help="Amount in Rupiah")
    p_insert_receipt.add_argument("--type", choices=["income", "cost", "expected"], required=True, help="Transaction type")
    p_insert_receipt.add_argument("--status", choices=["paid", "partial", "unpaid"], default="paid", help="Payment status")
    p_insert_receipt.add_argument("--notes", help="Notes / description")
    p_insert_receipt.set_defaults(func=handle_insert_receipt)

    p_receipts = subparsers.add_parser("receipts", help="List receipts with optional filters")
    p_receipts.add_argument("--event-id", type=int, help="Filter by event ID")
    p_receipts.add_argument("--type", choices=["income", "cost", "expected"], help="Filter by type")
    p_receipts.add_argument("--status", choices=["paid", "partial", "unpaid"], help="Filter by status")
    p_receipts.add_argument("--limit", type=int, default=50)
    p_receipts.set_defaults(func=handle_list_receipts)

    p_show_receipt = subparsers.add_parser("show-receipt", help="Show full receipt details")
    p_show_receipt.add_argument("id", type=int, help="Receipt ID")
    p_show_receipt.set_defaults(func=handle_show_receipt)

    p_balance = subparsers.add_parser("balance", help="Show net financial position from receipts")
    p_balance.add_argument("--event-id", type=int, help="Optional event ID to filter balance")
    p_balance.set_defaults(func=handle_balance)

    # media & sync
    p_ingest_media = subparsers.add_parser("ingest-media", help="Ingest normalized media records from JSON")
    p_ingest_media.add_argument("file", nargs="?", help="Path to JSON array of media records (default: stdin)")
    p_ingest_media.set_defaults(func=handle_ingest_media)

    p_ingest_rows = subparsers.add_parser("ingest-rows", help="Ingest RAW source rows from JSON")
    p_ingest_rows.add_argument("source", help="Source key (hardcover, goodreads, letterboxd, anilist_anime, anilist_manga, mydramalist)")
    p_ingest_rows.add_argument("file", nargs="?", help="Path to JSON array of raw row objects (default: stdin)")
    p_ingest_rows.set_defaults(func=handle_ingest_rows)

    p_serve_ingest = subparsers.add_parser("serve-ingest", help="Start local HTTP ingestion server")
    p_serve_ingest.add_argument("--port", type=int, default=8765)
    p_serve_ingest.set_defaults(func=handle_serve_ingest)

    p_sync = subparsers.add_parser("sync", help="Fetch media from trackers and ingest")
    p_sync.add_argument("--source", action="append", dest="sources", help="Sync only this source (repeatable)")
    p_sync.add_argument("--list", action="store_true", help="List configured sources")
    p_sync.set_defaults(func=handle_sync)

    p_media = subparsers.add_parser("media", help="List recent media logs")
    p_media.add_argument("--type", help="Filter by media_type")
    p_media.add_argument("--limit", type=int, default=20)
    p_media.set_defaults(func=handle_media)

    p_insert_link = subparsers.add_parser("insert-link", help="Insert/upsert a link record")
    p_insert_link.add_argument("--label", required=True)
    p_insert_link.add_argument("--url", required=True)
    p_insert_link.add_argument("--category", help="Category (e.g. social, profile, reference)")
    p_insert_link.add_argument("--private", action="store_true", help="Exclude from public garden export")
    p_insert_link.add_argument("--notes", help="Notes")
    p_insert_link.set_defaults(func=handle_insert_link)

    p_links = subparsers.add_parser("links", help="List links")
    p_links.add_argument("--category", help="Filter by category")
    p_links.add_argument("--public-only", action="store_true")
    p_links.set_defaults(func=handle_links)

    # commerce
    p_import_pay = subparsers.add_parser("import-pay", help="Import payment accounts from pay.json")
    p_import_pay.add_argument("file", help="Path to pay.json")
    p_import_pay.set_defaults(func=handle_import_pay)

    p_import_ref = subparsers.add_parser("import-referrals", help="Import referrals from referrals.json")
    p_import_ref.add_argument("file", help="Path to referrals.json")
    p_import_ref.set_defaults(func=handle_import_referrals)

    p_pay = subparsers.add_parser("pay", help="List payment accounts")
    p_pay.add_argument("--category", help="Filter by category")
    p_pay.set_defaults(func=handle_pay)

    p_ref = subparsers.add_parser("referrals", help="List referral codes")
    p_ref.add_argument("--category", help="Filter by category")
    p_ref.add_argument("--status", help="Filter by status")
    p_ref.add_argument("--public-only", action="store_true")
    p_ref.set_defaults(func=handle_referrals)

    # dashboard & test
    p_dash = subparsers.add_parser("dashboard", help="Start web dashboard server with live GPS webhook receiver")
    p_dash.add_argument("--port", type=int, default=8000, help="Port to run web server on (default: 8000)")
    p_dash.add_argument("--no-browser", action="store_true", help="Do not open browser automatically")
    p_dash.set_defaults(func=handle_dashboard)

    p_test = subparsers.add_parser("test", help="Run automated test suite")
    p_test.set_defaults(func=handle_test)

    return parser


def main() -> None:
    parser = build_parser()
    args = parser.parse_args()

    if hasattr(args, "func"):
        args.func(args)
    else:
        parser.print_help()


if __name__ == "__main__":
    main()
