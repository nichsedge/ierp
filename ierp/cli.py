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

from ierp.core.audit import generate_life_audit
from ierp.core.commerce import (
    init_tables as init_commerce_tables,
    insert_payment_account,
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
from ierp.core.contacts import get_contact, insert_contact, list_contacts, update_contact
from ierp.core.dashboard import start_dashboard_server
from ierp.core.db import get_db, init_db
from ierp.core.decisions import (
    get_decision,
    insert_decision,
    list_decisions,
    review_decision,
)
from ierp.core.events import (
    delete_event,
    get_event,
    insert_event,
    list_events as query_events,
    search_events as query_search_events,
    update_event,
)
from ierp.core.finance import (
    compute_monthly_burn,
    compute_runway,
    insert_commitment,
    insert_snapshot,
    list_commitments,
    list_snapshots,
)
from ierp.core.gadgets import (
    delete_gadget,
    export_garden_gadgets,
    get_gadget,
    import_garden_gadgets,
    insert_gadget,
    list_gadgets,
    update_gadget,
)
from ierp.core.garden import export_garden_all
from ierp.core.google_sync import sync_google_contacts
from ierp.core.importers import import_crm_contacts, import_notion_export, import_timeline
from ierp.core.ingest import ingest_rows, serve_ingest
from ierp.core.lifeops import (
    complete_maintenance,
    get_maintenance,
    get_maintenance_summary,
    insert_maintenance,
    list_maintenance,
)
from ierp.core.linking import run_manual_link
from ierp.core.media import ingest_media_records, list_links, list_media, upsert_link
from ierp.core.merging import auto_merge_contacts, merge_two_contacts
from ierp.core.projects import (
    get_project,
    get_project_summary,
    insert_project,
    list_projects,
    update_project,
)
from ierp.core.radar import (
    compute_radar,
    get_radar_summary,
    update_contact_cadence,
)
from ierp.core.reviews import (
    get_retrospective,
    insert_retrospective,
    list_retrospectives,
)
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
            project_id=getattr(args, "project_id", None),
        )
        print(f"{C_GREEN}Successfully inserted event #{ev_id}: {args.title}{C_RESET}")
        if linked_names:
            print(f"{C_GREEN}Explicitly linked: {', '.join(linked_names)}{C_RESET}")
    except Exception as e:
        print(f"{C_RED}Failed to insert event: {e}{C_RESET}")


def handle_update_event(args: argparse.Namespace) -> None:
    try:
        ok, linked_names = update_event(
            event_id=args.id,
            title=args.title,
            place=args.place,
            start_date=args.start_date,
            end_date=args.end_date,
            tags=args.tags,
            url=args.url,
            notes=args.notes,
            contacts=args.contacts,
            project_id=getattr(args, "project_id", None),
        )
        if ok:
            print(f"{C_GREEN}Successfully updated event #{args.id}{C_RESET}")
            if linked_names:
                print(f"{C_GREEN}Explicitly linked: {', '.join(linked_names)}{C_RESET}")
        else:
            print(f"{C_RED}Event #{args.id} not found.{C_RESET}")
    except Exception as e:
        print(f"{C_RED}Failed to update event: {e}{C_RESET}")


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

    print(f"\n{C_BOLD}FTS5 Search results for '{args.query}' ({len(results)} found):{C_RESET}")
    print(f"{C_BOLD}{'ID':<5} | {'Date':<12} | {'Rank':<7} | {'Title':<28} | {'Place':<18} | {'Tags'}{C_RESET}")
    print("-" * 95)
    for ev in results:
        tags_str = ", ".join(ev["tags"])
        place = ev["place"] or ""
        start = (ev["start_date"] or "No Date")[:10]
        title = ev["title"] or ""
        rank_str = f"{ev.get('rank', 0.0):.3f}" if ev.get('rank') is not None else "-"
        print(f"{ev['id']:<5} | {start:<12} | {rank_str:<7} | {title[:28]:<28} | {place[:18]:<18} | {tags_str[:20]}")
        notes_snip = ev.get("notes_snippet")
        if notes_snip and "[MATCH]" in notes_snip:
            clean_snip = notes_snip.replace("[MATCH]", f"{C_CYAN}{C_BOLD}").replace("[/MATCH]", C_RESET)
            print(f"      └─ {C_BOLD}Note Match:{C_RESET} {clean_snip}")
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

def handle_insert_contact(args: argparse.Namespace) -> None:
    try:
        cid = insert_contact(
            name=args.name,
            org=args.org,
            client=args.client,
            location=args.location,
            email=args.email,
            phone=args.phone,
            notes=args.notes,
            tier=args.tier if args.tier is not None else 3,
            cadence_days=args.cadence,
        )
        print(f"{C_GREEN}Successfully inserted contact #{cid}: {args.name}{C_RESET}")
    except Exception as e:
        print(f"{C_RED}Failed to insert contact: {e}{C_RESET}")


def handle_update_contact(args: argparse.Namespace) -> None:
    try:
        ok = update_contact(
            contact_id=args.id,
            name=args.name,
            org=args.org,
            client=args.client,
            location=args.location,
            email=args.email,
            phone=args.phone,
            notes=args.notes,
            tier=args.tier,
            cadence_days=args.cadence,
        )
        if ok:
            print(f"{C_GREEN}Successfully updated contact #{args.id}{C_RESET}")
        else:
            print(f"{C_RED}Contact #{args.id} not found.{C_RESET}")
    except Exception as e:
        print(f"{C_RED}Failed to update contact: {e}{C_RESET}")


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
# Handlers: Gadgets & Hardware Assets
# -----------------------------------------------------------------------------

def handle_insert_gadget(args: argparse.Namespace) -> None:
    try:
        gid = insert_gadget(
            name=args.name,
            slug=args.slug,
            brand=args.brand,
            model=args.model,
            category=args.category,
            status=args.status,
            purchase_date=args.date,
            purchase_price=args.price,
            currency=args.currency,
            specs=args.specs,
            serial_number=args.serial,
            vendor_id=args.vendor_id,
            event_id=args.event_id,
            notes=args.notes,
            is_public=not args.private,
        )
        print(f"{C_GREEN}Successfully inserted gadget #{gid}: {args.name}{C_RESET}")
    except Exception as e:
        print(f"{C_RED}Failed to insert gadget: {e}{C_RESET}")


def handle_list_gadgets(args: argparse.Namespace) -> None:
    rows, total = list_gadgets(
        category=args.category,
        status=args.status,
        brand=args.brand,
        q=args.q,
        limit=args.limit,
    )
    if not rows:
        print("No gadgets found.")
        return

    print(f"\n{C_BOLD}{'ID':<4} | {'Name':<26} | {'Brand':<10} | {'Category':<14} | {'Status':<8} | {'Acquired':<10} | Price{C_RESET}")
    print("-" * 92)
    for g in rows:
        price_str = f"{g['currency'] or 'IDR'} {g['purchase_price']:,.0f}" if g.get("purchase_price") else "-"
        date_str = (g.get("purchase_date") or "-")[:10]
        status_str = g.get("status") or "active"
        print(f"{g['id']:<4} | {g['name'][:26]:<26} | {(g['brand'] or '-'):<10} | {(g['category'] or '-'):<14} | {status_str:<8} | {date_str:<10} | {price_str}")
    print(f"\nTotal gadgets: {total}\n")


def handle_show_gadget(args: argparse.Namespace) -> None:
    g = get_gadget(args.id)
    if not g:
        print(f"{C_RED}Gadget '{args.id}' not found.{C_RESET}")
        return

    print(f"\n{C_BOLD}{C_GREEN}=== Gadget #{g['id']}: {g['name']} ==={C_RESET}")
    print(f"{C_BOLD}Slug:{C_RESET}          {g['slug']}")
    print(f"{C_BOLD}Brand:{C_RESET}         {g['brand'] or '-'}")
    print(f"{C_BOLD}Model:{C_RESET}         {g['model'] or '-'}")
    print(f"{C_BOLD}Category:{C_RESET}      {g['category'] or '-'}")
    print(f"{C_BOLD}Status:{C_RESET}        {g['status'] or 'active'}")
    print(f"{C_BOLD}Acquired:{C_RESET}      {g['purchase_date'] or '-'}")
    if g.get("purchase_price"):
        print(f"{C_BOLD}Price:{C_RESET}         {g['currency'] or 'IDR'} {g['purchase_price']:,.0f}")
    if g.get("specs_json"):
        print(f"{C_BOLD}Specs:{C_RESET}         {g['specs_json']}")
    if g.get("serial_number"):
        print(f"{C_BOLD}Serial No:{C_RESET}     {g['serial_number']}")
    if g.get("vendor_name"):
        print(f"{C_BOLD}Vendor:{C_RESET}        {g['vendor_name']} (#{g['vendor_id']})")
    if g.get("event_title"):
        print(f"{C_BOLD}Linked Event:{C_RESET}  {g['event_title']} (#{g['event_id']})")
    if g.get("notes"):
        print(f"\n{C_BOLD}--- Notes ---{C_RESET}")
        print(g["notes"])
    print(f"\n{C_BOLD}Public Garden:{C_RESET} {'Yes' if g.get('is_public', 1) else 'No'}")
    print(f"{C_BOLD}Created:{C_RESET}       {g['created_at']}")
    print(f"{C_BOLD}{C_GREEN}{'=' * 35}{C_RESET}\n")


def handle_import_gadgets(args: argparse.Namespace) -> None:
    garden_dir = Path(args.dir) if args.dir else None
    imported, skipped = import_garden_gadgets(garden_dir=garden_dir)
    print(f"{C_GREEN}Gadget import complete: {imported} imported, {skipped} updated/skipped.{C_RESET}")


def handle_export_gadgets(args: argparse.Namespace) -> None:
    garden_dir = Path(args.garden_dir) if args.garden_dir else None
    res = export_garden_gadgets(garden_dir=garden_dir, dry_run=args.dry_run)
    mode = " [dry-run]" if res["dry_run"] else ""
    print(f"{C_GREEN}Gadgets exported{mode} to {res['target_dir']}: {res['notes_written']} notes written, index updated.{C_RESET}")


def handle_garden_export(args: argparse.Namespace) -> None:
    garden_dir = Path(args.dir) if args.dir else None
    targets = [args.target] if args.target != "all" else ["projects", "decisions", "reviews", "gadgets"]
    res = export_garden_all(garden_root=garden_dir, targets=targets, dry_run=args.dry_run)
    mode = " [dry-run]" if res["dry_run"] else ""
    print(f"\n{C_BOLD}{C_GREEN}Digital Garden Export Complete{mode}!{C_RESET}")
    print(f"Garden Root: {res['garden_root']}")
    for dom, info in res["domains"].items():
        print(f"  - {dom.capitalize():<15}: {info.get('notes_written', 0)} notes -> {info.get('target_dir', '')}")
    print()


# -----------------------------------------------------------------------------
# Handlers: Cashflow & Financial Intelligence (Sans Finance SSOT)
# -----------------------------------------------------------------------------

def handle_cashflow(args: argparse.Namespace) -> None:
    from ierp.core.finance import get_sansfinance_cashflow, compute_sansfinance_summary
    cf = get_sansfinance_cashflow(limit=args.limit)
    summary = compute_sansfinance_summary()
    if not cf:
        print("No cashflow records found in Sans Finance snapshot.")
        return

    print(f"\n{C_BOLD}{C_GREEN}=== Monthly Cashflow (Sans Finance SSOT) ==={C_RESET}\n")
    print(f"{C_BOLD}{'Month':<10} | {'Income':<16} | {'Expenses':<16} | Net Cashflow{C_RESET}")
    print("-" * 62)
    for c in cf:
        inc = c.get("income", 0.0)
        exp = c.get("cost", 0.0)
        net = inc - exp
        net_color = C_GREEN if net >= 0 else C_RED
        print(f"{c['year_month']:<10} | Rp{inc:>13,.0f} | Rp{exp:>13,.0f} | {net_color}Rp{net:>13,.0f}{C_RESET}")
    print("-" * 62)
    print(f"Total Income:   Rp{summary['total_income']:,.0f}")
    print(f"Total Expenses: Rp{summary['total_costs']:,.0f}")
    print(f"Net Savings:    Rp{summary['net_cash']:,.0f}\n")


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


def handle_insert_pay(args: argparse.Namespace) -> None:
    init_db()
    slug = args.slug or args.name.lower().replace(" ", "-")
    pid = insert_payment_account(
        slug=slug,
        name=args.name,
        category=args.category,
        number=args.number,
        recipient=args.recipient,
        details=args.details,
        details_id=args.details_id,
    )
    print(f"{C_GREEN}Payment account #{pid} saved: {args.name} ({args.number}){C_RESET}")


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
# Handlers: Projects / Initiatives
# -----------------------------------------------------------------------------

def handle_insert_project(args: argparse.Namespace) -> None:
    init_db()
    pid = insert_project(
        title=args.title,
        slug=args.slug,
        description=args.description,
        status=args.status,
        priority=args.priority,
        start_date=args.start_date,
        target_date=args.target_date,
    )
    print(f"{C_GREEN}Project #{pid} created: {args.title} (Status: {args.status}, Priority: {args.priority}){C_RESET}")


def handle_list_projects(args: argparse.Namespace) -> None:
    projects = list_projects(status=args.status, priority=args.priority, limit=args.limit)
    if not projects:
        print("No projects found.")
        return

    print(f"\n{C_BOLD}{'ID':<4} | {'Slug':<18} | {'Title':<30} | {'Status':<10} | {'Priority':<8} | {'Events':<6} | Target{C_RESET}")
    print("-" * 105)
    for p in projects:
        target = p["target_date"] or "-"
        print(f"{p['id']:<4} | {p['slug'][:18]:<18} | {p['title'][:30]:<30} | {p['status'][:10]:<10} | {p['priority'][:8]:<8} | {p['event_count']:<6} | {target}")
    print()


def handle_show_project(args: argparse.Namespace) -> None:
    summary = get_project_summary(args.id)
    if not summary:
        print(f"{C_RED}Project '{args.id}' not found.{C_RESET}")
        return

    print(f"\n{C_BOLD}{C_MAGENTA}=== Project Details (ID: {summary['id']}) ==={C_RESET}")
    print(f"{C_BOLD}Title:{C_RESET}       {summary['title']} ({summary['slug']})")
    print(f"{C_BOLD}Status:{C_RESET}      {summary['status'].upper()} | Priority: {summary['priority'].upper()}")
    print(f"{C_BOLD}Timeline:{C_RESET}    {summary['start_date'] or 'N/A'} → {summary['target_date'] or 'Ongoing'}")
    if summary.get("description"):
        print(f"{C_BOLD}Description:{C_RESET} {summary['description']}")

    if summary.get("events"):
        print(f"\n{C_BOLD}Linked Events ({len(summary['events'])}):{C_RESET}")
        for e in summary["events"]:
            print(f"  - [{e['start_date'] or 'No date'}] #{e['id']} {e['title']} ({e['place'] or 'No place'})")

    if summary.get("decisions"):
        print(f"\n{C_BOLD}Linked Decisions ({len(summary['decisions'])}):{C_RESET}")
        for d in summary["decisions"]:
            print(f"  - #{d['id']} {d['title']} | Choice: {d['choice']} (Confidence: {d['confidence']}/10, Status: {d['status']})")
    print()


# -----------------------------------------------------------------------------
# Handlers: Decision Journal
# -----------------------------------------------------------------------------

def handle_insert_decision(args: argparse.Namespace) -> None:
    init_db()
    did = insert_decision(
        title=args.title,
        choice=args.choice,
        context=args.context,
        expected_outcome=args.expected,
        confidence=args.confidence,
        review_date=args.review_date,
        project_id=args.project_id,
    )
    print(f"{C_GREEN}Decision #{did} recorded: {args.title} (Confidence: {args.confidence}/10, Review: {args.review_date or 'None'}){C_RESET}")


def handle_list_decisions(args: argparse.Namespace) -> None:
    decisions = list_decisions(status=args.status, project_id=args.project_id, pending_review_only=args.pending_review, limit=args.limit)
    if not decisions:
        print("No decisions found.")
        return

    print(f"\n{C_BOLD}{'ID':<4} | {'Title':<30} | {'Status':<10} | {'Conf':<5} | {'Review Due':<12} | Project{C_RESET}")
    print("-" * 95)
    for d in decisions:
        review_due = d["review_date"] or "-"
        proj = d["project_title"] or "-"
        print(f"{d['id']:<4} | {d['title'][:30]:<30} | {d['status'][:10]:<10} | {d['confidence']:<5} | {review_due:<12} | {proj[:20]}")
    print()


def handle_show_decision(args: argparse.Namespace) -> None:
    d = get_decision(args.id)
    if not d:
        print(f"{C_RED}Decision #{args.id} not found.{C_RESET}")
        return

    print(f"\n{C_BOLD}{C_MAGENTA}=== Decision Details (ID: {d['id']}) ==={C_RESET}")
    print(f"{C_BOLD}Title:{C_RESET}            {d['title']}")
    print(f"{C_BOLD}Status:{C_RESET}           {d['status'].upper()} (Confidence: {d['confidence']}/10)")
    print(f"{C_BOLD}Project:{C_RESET}          {d['project_title'] or 'None'}")
    print(f"{C_BOLD}Review Date:{C_RESET}      {d['review_date'] or 'Unscheduled'}")
    if d.get("context"):
        print(f"\n{C_BOLD}Context & Mental State:{C_RESET}\n{d['context']}")
    print(f"\n{C_BOLD}Choice Made:{C_RESET}\n{d['choice']}")
    if d.get("expected_outcome"):
        print(f"\n{C_BOLD}Expected Outcome:{C_RESET}\n{d['expected_outcome']}")
    if d.get("actual_outcome"):
        print(f"\n{C_BOLD}Actual Outcome (Post-Review):{C_RESET}\n{d['actual_outcome']}")
    print()


def handle_review_decision(args: argparse.Namespace) -> None:
    ok = review_decision(args.id, actual_outcome=args.outcome, status=args.status)
    if ok:
        print(f"{C_GREEN}Decision #{args.id} successfully reviewed and marked '{args.status}'.{C_RESET}")
    else:
        print(f"{C_RED}Failed to review decision #{args.id}.{C_RESET}")


# -----------------------------------------------------------------------------
# Handlers: Sovereign Treasury & Runway
# -----------------------------------------------------------------------------

def handle_insert_snapshot(args: argparse.Namespace) -> None:
    init_db()
    sid = insert_snapshot(
        snapshot_date=args.date,
        liquid_cash=args.liquid,
        investments=args.investments,
        hard_assets=args.assets,
        liabilities=args.liabilities,
        currency=args.currency,
        notes=args.notes,
    )
    net_worth = (args.liquid + args.investments + args.assets) - args.liabilities
    print(f"{C_GREEN}Net worth snapshot #{sid} recorded for {args.date or 'today'}: Net Worth = {args.currency} {net_worth:,.2f}{C_RESET}")


def handle_list_snapshots(args: argparse.Namespace) -> None:
    snapshots = list_snapshots(limit=args.limit)
    if not snapshots:
        print("No net worth snapshots found.")
        return

    print(f"\n{C_BOLD}{'ID':<4} | {'Date':<10} | {'Liquid Cash':<16} | {'Investments':<16} | {'Hard Assets':<16} | {'Liabilities':<14} | Net Worth{C_RESET}")
    print("-" * 115)
    for s in snapshots:
        curr = s["currency"]
        print(f"{s['id']:<4} | {s['snapshot_date']:<10} | {curr} {s['liquid_cash']:>10,.0f} | {curr} {s['investments']:>10,.0f} | {curr} {s['hard_assets']:>10,.0f} | {curr} {s['liabilities']:>8,.0f} | {curr} {s['net_worth']:>11,.0f}")
    print()


def handle_insert_commitment(args: argparse.Namespace) -> None:
    init_db()
    cid = insert_commitment(
        name=args.name,
        amount=args.amount,
        category=args.category,
        currency=args.currency,
        frequency=args.frequency,
        payment_account_id=args.account_id,
        renewal_date=args.renewal_date,
        notes=args.notes,
    )
    print(f"{C_GREEN}Recurring commitment #{cid} saved: {args.name} ({args.currency} {args.amount:,.2f} / {args.frequency}){C_RESET}")


def handle_list_commitments(args: argparse.Namespace) -> None:
    commitments = list_commitments(status=args.status, category=args.category)
    if not commitments:
        print("No recurring commitments found.")
        return

    print(f"\n{C_BOLD}{'ID':<4} | {'Name':<24} | {'Category':<14} | {'Frequency':<10} | {'Monthly Equiv':<18} | Status{C_RESET}")
    print("-" * 95)
    for c in commitments:
        curr = c["currency"]
        print(f"{c['id']:<4} | {c['name'][:24]:<24} | {c['category'][:14]:<14} | {c['frequency']:<10} | {curr} {c['monthly_amount']:>12,.0f} | {c['status']}")
    print()


def handle_runway(args: argparse.Namespace) -> None:
    data = compute_runway()
    print(f"\n{C_BOLD}{C_CYAN}=== Sovereign Runway & Treasury Status ==={C_RESET}")
    print(f"{C_BOLD}Liquid Capital Reserves:{C_RESET} IDR {data['liquid_cash']:,.2f} (from latest snapshot: {data['latest_snapshot_date'] or 'None'})")
    print(f"{C_BOLD}Net Worth Position:{C_RESET}       IDR {data['net_worth']:,.2f}")
    print(f"{C_BOLD}Monthly Recurring Burn:{C_RESET}   IDR {data['monthly_burn']:,.2f} across {data['commitments_count']} active commitments")
    
    if data["is_infinite"]:
        runway_str = f"{C_GREEN}{C_BOLD}INFINITE{C_RESET} (Zero recurring burn with positive reserves)"
    else:
        color = C_GREEN if data["runway_months"] >= 12 else (C_YELLOW if data["runway_months"] >= 6 else C_RED)
        runway_str = f"{color}{C_BOLD}{data['runway_months']} Months{C_RESET} [{data['status_label']}]"
    
    print(f"{C_BOLD}Runway to Zero:{C_RESET}           {runway_str}")

    if data["burn_by_category"]:
        print(f"\n{C_BOLD}Monthly Burn Breakdown:{C_RESET}")
        for cat, amt in sorted(data["burn_by_category"].items(), key=lambda x: -x[1]):
            pct = (amt / data["monthly_burn"] * 100) if data["monthly_burn"] > 0 else 0
            print(f"  - {cat:<16}: IDR {amt:>12,.0f} ({pct:>4.1f}%)")
    print()


# -----------------------------------------------------------------------------
# Handlers: Human Capital Radar
# -----------------------------------------------------------------------------

def handle_radar(args: argparse.Namespace) -> None:
    items = compute_radar(tier=args.tier, overdue_only=args.overdue_only, limit=args.limit)
    if not items:
        print("No contacts matching radar criteria.")
        return

    print(f"\n{C_BOLD}{'ID':<4} | {'Name':<24} | {'Tier':<5} | {'Cadence':<8} | {'Last Seen':<12} | {'Days Ago':<9} | Status{C_RESET}")
    print("-" * 95)
    for c in items:
        last = c["last_seen_date"][:10] if c["last_seen_date"] else "Never"
        days_str = str(c["days_since_last_touch"]) if c["days_since_last_touch"] < 999 else "Never"
        if c["is_overdue"]:
            status = f"{C_RED}{c['days_overdue']}d overdue{C_RESET}"
        else:
            status = f"{C_GREEN}Active{C_RESET}"
        print(f"{c['id']:<4} | {c['name'][:24]:<24} | Tier {c['tier']} | {c['cadence_days']}d     | {last:<12} | {days_str:<9} | {status}")
    print()


def handle_set_tier(args: argparse.Namespace) -> None:
    ok = update_contact_cadence(args.contact_id, tier=args.tier, cadence_days=args.cadence)
    if ok:
        print(f"{C_GREEN}Contact #{args.contact_id} updated: Tier {args.tier} (Cadence: {args.cadence or 'default'} days){C_RESET}")
    else:
        print(f"{C_RED}Failed to update contact #{args.contact_id}.{C_RESET}")


# -----------------------------------------------------------------------------
# Handlers: Life Ops & Maintenance
# -----------------------------------------------------------------------------

def handle_insert_maintenance(args: argparse.Namespace) -> None:
    init_db()
    mid = insert_maintenance(
        name=args.name,
        due_date=args.due_date,
        category=args.category,
        interval_days=args.interval,
        cost=args.cost,
        notes=args.notes,
        gadget_id=args.gadget_id,
    )
    print(f"{C_GREEN}Maintenance task #{mid} scheduled: {args.name} (Due: {args.due_date}, Category: {args.category}){C_RESET}")


def handle_list_maintenance(args: argparse.Namespace) -> None:
    items = list_maintenance(status=args.status, category=args.category, due_within_days=args.due_within, limit=args.limit)
    if not items:
        print("No maintenance items found.")
        return

    print(f"\n{C_BOLD}{'ID':<4} | {'Name':<28} | {'Category':<14} | {'Due Date':<11} | {'Interval':<9} | Status{C_RESET}")
    print("-" * 90)
    for m in items:
        status = f"{C_RED}OVERDUE{C_RESET}" if m["is_overdue"] else (f"{C_GREEN}Completed{C_RESET}" if m["status"] == "completed" else "Pending")
        interval_str = f"{m['interval_days']}d" if m["interval_days"] else "-"
        print(f"{m['id']:<4} | {m['name'][:28]:<28} | {m['category'][:14]:<14} | {m['due_date']:<11} | {interval_str:<9} | {status}")
    print()


def handle_complete_maintenance(args: argparse.Namespace) -> None:
    res = complete_maintenance(args.id, cost=args.cost, completion_date=args.date)
    if res["success"]:
        print(f"{C_GREEN}Maintenance #{args.id} marked completed.{C_RESET}")
        if res.get("next_item_id"):
            print(f"{C_CYAN}Auto-rescheduled next occurrence #{res['next_item_id']} due on {res['next_due_date']}.{C_RESET}")
    else:
        print(f"{C_RED}Failed to complete maintenance #{args.id}: {res.get('error')}{C_RESET}")


# -----------------------------------------------------------------------------
# Handlers: Sprint Retrospectives
# -----------------------------------------------------------------------------

def handle_insert_review(args: argparse.Namespace) -> None:
    init_db()
    rid = insert_retrospective(
        period_start=args.start,
        period_end=args.end,
        period_type=args.type,
        wins=args.wins,
        drains_burnout=args.drains,
        lessons=args.lessons,
        focus_next=args.focus,
        rating=args.rating,
        notes=args.notes,
    )
    print(f"{C_GREEN}Retrospective #{rid} ({args.type}) recorded for {args.start} → {args.end} (Rating: {args.rating}/10){C_RESET}")


def handle_list_reviews(args: argparse.Namespace) -> None:
    reviews = list_retrospectives(period_type=args.type, limit=args.limit)
    if not reviews:
        print("No retrospectives found.")
        return

    print(f"\n{C_BOLD}{'ID':<4} | {'Type':<10} | {'Period':<23} | {'Rating':<7} | Primary Focus Next{C_RESET}")
    print("-" * 90)
    for r in reviews:
        period = f"{r['period_start']} → {r['period_end']}"
        focus = (r["focus_next"] or "-")[:40]
        print(f"{r['id']:<4} | {r['period_type']:<10} | {period:<23} | {r['rating']:<7} | {focus}")
    print()


def handle_show_review(args: argparse.Namespace) -> None:
    r = get_retrospective(args.id)
    if not r:
        print(f"{C_RED}Retrospective #{args.id} not found.{C_RESET}")
        return

    print(f"\n{C_BOLD}{C_MAGENTA}=== Retrospective Review #{r['id']} ({r['period_type'].upper()}) ==={C_RESET}")
    print(f"{C_BOLD}Period:{C_RESET}       {r['period_start']} → {r['period_end']} (Score: {r['rating']}/10)")
    if r.get("wins"):
        print(f"\n{C_GREEN}{C_BOLD}Top Wins & Accomplishments:{C_RESET}\n{r['wins']}")
    if r.get("drains_burnout"):
        print(f"\n{C_RED}{C_BOLD}Energy Drains & Bottlenecks:{C_RESET}\n{r['drains_burnout']}")
    if r.get("lessons"):
        print(f"\n{C_YELLOW}{C_BOLD}Key Lessons Learned:{C_RESET}\n{r['lessons']}")
    if r.get("focus_next"):
        print(f"\n{C_CYAN}{C_BOLD}Focus Themes for Next Sprint:{C_RESET}\n{r['focus_next']}")
    if r.get("notes"):
        print(f"\n{C_BOLD}Additional Notes:{C_RESET}\n{r['notes']}")
    print()


def handle_audit(args: argparse.Namespace) -> None:
    audit_data = generate_life_audit()
    if getattr(args, "json", False):
        print(json.dumps(audit_data, indent=2))
        return

    print(f"\n{C_BOLD}{C_MAGENTA}=== iERP Life Audit & Pulse ({audit_data['generated_at']}) ==={C_RESET}")
    print(
        f"Summary: {C_RED}{audit_data['actions_needed']} action(s) needed{C_RESET}, "
        f"{C_YELLOW}{audit_data['warnings']} warning(s){C_RESET}, "
        f"{C_CYAN}{audit_data['tips']} tip(s){C_RESET}\n"
    )

    if not audit_data["findings"]:
        print(f"{C_GREEN}✨ All systems operational. Your personal ERP is fully up-to-date!{C_RESET}\n")
        return

    for idx, f in enumerate(audit_data["findings"], 1):
        sev = f["severity"]
        if sev == "action_needed":
            badge = f"{C_RED}{C_BOLD}[ACTION NEEDED]{C_RESET}"
        elif sev == "warning":
            badge = f"{C_YELLOW}{C_BOLD}[WARNING]{C_RESET}"
        else:
            badge = f"{C_CYAN}{C_BOLD}[TIP]{C_RESET}"

        domain = f"{C_BOLD}[{f['domain']}]{C_RESET}"
        print(f"{idx}. {badge} {domain} {C_BOLD}{f['title']}{C_RESET}")
        print(f"   {f['description']}")
        print(f"   {C_GREEN}👉 Action: {f['command']}{C_RESET}\n")


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
    p_insert.add_argument("--project-id", type=int, help="Linked project / strategic initiative ID")
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

    # update-event
    p_update = subparsers.add_parser("update-event", help="Update an existing event record")
    p_update.add_argument("id", type=int, help="Event database ID to update")
    p_update.add_argument("--title", help="New event title")
    p_update.add_argument("--place", help="New event location/place name")
    p_update.add_argument("--start-date", help="New event start date (ISO or natural string)")
    p_update.add_argument("--end-date", help="New event end date (ISO or natural string)")
    p_update.add_argument("--tags", help="Comma-separated tags or categories")
    p_update.add_argument("--url", help="Event URL")
    p_update.add_argument("--notes", help="Event notes/body content")
    p_update.add_argument("--contact", action="append", dest="contacts", help="Explicitly link this contact (name or ID; repeatable)")
    p_update.add_argument("--project-id", type=int, help="Linked project / strategic initiative ID")
    p_update.set_defaults(func=handle_update_event)

    # contacts
    p_contacts = subparsers.add_parser("contacts", help="List contacts")
    p_contacts.add_argument("--source", choices=["manual", "google", "merged", "all"], default="all", help="Filter by contact source")
    p_contacts.add_argument("--tier", type=int, choices=[1, 2, 3], help="Filter by Dunbar relationship tier (1, 2, 3)")
    p_contacts.set_defaults(func=handle_list_contacts)

    p_show_contact = subparsers.add_parser("show-contact", help="Show contact details and linked events")
    p_show_contact.add_argument("id", type=int, help="Contact database ID")
    p_show_contact.set_defaults(func=handle_show_contact)

    p_insert_contact = subparsers.add_parser("insert-contact", help="Insert a contact record directly")
    p_insert_contact.add_argument("--name", required=True, help="Contact full name")
    p_insert_contact.add_argument("--org", help="Organization / company / university")
    p_insert_contact.add_argument("--client", help="Client, team, or department")
    p_insert_contact.add_argument("--location", help="Location or university alma mater")
    p_insert_contact.add_argument("--email", help="Contact email address")
    p_insert_contact.add_argument("--phone", help="Contact phone number")
    p_insert_contact.add_argument("--notes", help="Contact notes or background")
    p_insert_contact.add_argument("--tier", type=int, choices=[1, 2, 3], default=3, help="Dunbar relationship tier (1, 2, 3)")
    p_insert_contact.add_argument("--cadence", type=int, help="Touchpoint cadence in days")
    p_insert_contact.set_defaults(func=handle_insert_contact)

    p_update_contact = subparsers.add_parser("update-contact", help="Update an existing contact record")
    p_update_contact.add_argument("id", type=int, help="Contact database ID to update")
    p_update_contact.add_argument("--name", help="New contact full name")
    p_update_contact.add_argument("--org", help="New organization / company / university")
    p_update_contact.add_argument("--client", help="New client, team, or department")
    p_update_contact.add_argument("--location", help="New location or alma mater")
    p_update_contact.add_argument("--email", help="New contact email address")
    p_update_contact.add_argument("--phone", help="New contact phone number")
    p_update_contact.add_argument("--notes", help="New contact notes or background")
    p_update_contact.add_argument("--tier", type=int, choices=[1, 2, 3], help="New Dunbar relationship tier (1, 2, 3)")
    p_update_contact.add_argument("--cadence", type=int, help="New touchpoint cadence in days")
    p_update_contact.set_defaults(func=handle_update_contact)

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

    # gadgets
    p_insert_gadget = subparsers.add_parser("insert-gadget", help="Insert a structured gadget/hardware asset")
    p_insert_gadget.add_argument("--name", required=True, help="Gadget/device name")
    p_insert_gadget.add_argument("--slug", help="Custom unique slug")
    p_insert_gadget.add_argument("--brand", help="Manufacturer brand (e.g. Xiaomi, Apple, Samsung)")
    p_insert_gadget.add_argument("--model", help="Specific model designation")
    p_insert_gadget.add_argument("--category", help="Device category (e.g. Smartphone, Wearable, Laptop)")
    p_insert_gadget.add_argument("--status", choices=["active", "backup", "retired", "sold", "broken"], default="active", help="Lifecycle status")
    p_insert_gadget.add_argument("--date", help="Purchase or acquisition date (YYYY-MM-DD)")
    p_insert_gadget.add_argument("--price", type=float, help="Acquisition price")
    p_insert_gadget.add_argument("--currency", default="IDR", help="Currency code (default: IDR)")
    p_insert_gadget.add_argument("--specs", help="Specs description or JSON")
    p_insert_gadget.add_argument("--serial", help="Serial number or IMEI")
    p_insert_gadget.add_argument("--vendor-id", type=int, help="Linked vendor ID")
    p_insert_gadget.add_argument("--event-id", type=int, help="Linked event ID")
    p_insert_gadget.add_argument("--notes", help="Notes or qualitative description")
    p_insert_gadget.add_argument("--private", action="store_true", help="Exclude from public digital garden export")
    p_insert_gadget.set_defaults(func=handle_insert_gadget)

    p_gadgets = subparsers.add_parser("gadgets", help="List gadgets with optional filters")
    p_gadgets.add_argument("--category", help="Filter by category")
    p_gadgets.add_argument("--status", help="Filter by status")
    p_gadgets.add_argument("--brand", help="Filter by brand")
    p_gadgets.add_argument("--q", help="Keyword search query")
    p_gadgets.add_argument("--limit", type=int, default=50, help="Max results to display")
    p_gadgets.set_defaults(func=handle_list_gadgets)

    p_show_gadget = subparsers.add_parser("show-gadget", help="Show full gadget details and specs")
    p_show_gadget.add_argument("id", help="Gadget ID or slug")
    p_show_gadget.set_defaults(func=handle_show_gadget)

    p_import_gadgets = subparsers.add_parser("import-gadgets", help="Import gadget markdown notes from digital garden into SQLite")
    p_import_gadgets.add_argument("dir", nargs="?", help="Path to digital garden Gadget folder")
    p_import_gadgets.set_defaults(func=handle_import_gadgets)

    p_export_gadgets = subparsers.add_parser("export-gadgets", help="Export gadgets from SQLite to digital garden markdown notes")
    p_export_gadgets.add_argument("--garden-dir", help="Path to digital garden Gadget folder")
    p_export_gadgets.add_argument("--dry-run", action="store_true", help="Dry run without writing files")
    p_export_gadgets.set_defaults(func=handle_export_gadgets)

    p_garden_export = subparsers.add_parser(
        "garden-export",
        aliases=["export-garden"],
        help="Export iERP entities (projects, decisions, reviews, gadgets) to Digital Garden Markdown notes",
    )
    p_garden_export.add_argument(
        "--target",
        choices=["all", "projects", "decisions", "reviews", "gadgets"],
        default="all",
        help="Entity domain to export (default: all)",
    )
    p_garden_export.add_argument("--dir", help="Custom Digital Garden content root directory")
    p_garden_export.add_argument("--dry-run", action="store_true", help="Simulate export without writing files")
    p_garden_export.set_defaults(func=handle_garden_export)

    # Cashflow (Sans Finance SSOT)
    p_cashflow = subparsers.add_parser("cashflow", help="Display monthly cashflow aggregates from Sans Finance SSOT")
    p_cashflow.add_argument("--limit", type=int, default=12, help="Number of months to show")
    p_cashflow.set_defaults(func=handle_cashflow)

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
    p_insert_pay = subparsers.add_parser("insert-pay", help="Insert/upsert a payment account")
    p_insert_pay.add_argument("--slug", help="Unique slug/ID (e.g. cimb)")
    p_insert_pay.add_argument("--name", required=True, help="Account/Bank Name (e.g. Bank CIMB Niaga)")
    p_insert_pay.add_argument("--category", default="Bank Node", help="Category (e.g. Bank Node, Digital Bank Node, E-Wallet Node)")
    p_insert_pay.add_argument("--number", required=True, help="Account number")
    p_insert_pay.add_argument("--recipient", default="MUHAMMAD ICHSANUL AMAL", help="Account recipient name")
    p_insert_pay.add_argument("--details", help="English transfer details description")
    p_insert_pay.add_argument("--details-id", help="Indonesian transfer details description")
    p_insert_pay.set_defaults(func=handle_insert_pay)

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

    # projects
    p_insert_project = subparsers.add_parser("insert-project", help="Insert a project / strategic initiative")
    p_insert_project.add_argument("--title", required=True, help="Project title")
    p_insert_project.add_argument("--slug", help="Unique URL-friendly slug")
    p_insert_project.add_argument("--description", help="Project overview or objective")
    p_insert_project.add_argument("--status", choices=["active", "paused", "completed", "archived"], default="active", help="Project status")
    p_insert_project.add_argument("--priority", choices=["high", "medium", "low"], default="medium", help="Strategic priority")
    p_insert_project.add_argument("--start-date", help="Start date (YYYY-MM-DD)")
    p_insert_project.add_argument("--target-date", help="Target completion date (YYYY-MM-DD)")
    p_insert_project.set_defaults(func=handle_insert_project)

    p_projects = subparsers.add_parser("projects", help="List projects and strategic initiatives")
    p_projects.add_argument("--status", help="Filter by status")
    p_projects.add_argument("--priority", help="Filter by priority")
    p_projects.add_argument("--limit", type=int, default=50)
    p_projects.set_defaults(func=handle_list_projects)

    p_show_project = subparsers.add_parser("show-project", help="Show full project details and linked events")
    p_show_project.add_argument("id", help="Project ID or slug")
    p_show_project.set_defaults(func=handle_show_project)

    # decisions
    p_insert_decision = subparsers.add_parser("insert-decision", help="Record a choice in the decision journal")
    p_insert_decision.add_argument("--title", required=True, help="Decision title / question")
    p_insert_decision.add_argument("--choice", required=True, help="The option chosen")
    p_insert_decision.add_argument("--context", help="Context, options considered, mental state")
    p_insert_decision.add_argument("--expected", help="Expected outcome / hypothesis")
    p_insert_decision.add_argument("--confidence", type=int, default=7, help="Confidence rating (1-10)")
    p_insert_decision.add_argument("--review-date", help="Scheduled review date (YYYY-MM-DD)")
    p_insert_decision.add_argument("--project-id", type=int, help="Linked project ID")
    p_insert_decision.set_defaults(func=handle_insert_decision)

    p_decisions = subparsers.add_parser("decisions", help="List logged decisions")
    p_decisions.add_argument("--status", choices=["pending", "reviewed", "abandoned"], help="Filter by status")
    p_decisions.add_argument("--project-id", type=int, help="Filter by project ID")
    p_decisions.add_argument("--pending-review", action="store_true", help="Show decisions due for review")
    p_decisions.add_argument("--limit", type=int, default=50)
    p_decisions.set_defaults(func=handle_list_decisions)

    p_show_decision = subparsers.add_parser("show-decision", help="Show full decision details")
    p_show_decision.add_argument("id", type=int, help="Decision ID")
    p_show_decision.set_defaults(func=handle_show_decision)

    p_review_decision = subparsers.add_parser("review-decision", help="Complete a retrospective review of a decision")
    p_review_decision.add_argument("id", type=int, help="Decision ID")
    p_review_decision.add_argument("--outcome", required=True, help="Actual outcome observed")
    p_review_decision.add_argument("--status", choices=["reviewed", "abandoned"], default="reviewed", help="Updated status")
    p_review_decision.set_defaults(func=handle_review_decision)

    # finance & runway
    p_insert_snapshot = subparsers.add_parser("insert-snapshot", help="Record a balance sheet / net worth snapshot")
    p_insert_snapshot.add_argument("--date", help="Snapshot date (YYYY-MM-DD, default today)")
    p_insert_snapshot.add_argument("--liquid", type=float, default=0.0, help="Liquid cash reserves")
    p_insert_snapshot.add_argument("--investments", type=float, default=0.0, help="Liquid or illiquid investments")
    p_insert_snapshot.add_argument("--assets", type=float, default=0.0, help="Hard asset value (gadgets, etc.)")
    p_insert_snapshot.add_argument("--liabilities", type=float, default=0.0, help="Total debts and liabilities")
    p_insert_snapshot.add_argument("--currency", default="IDR", help="Currency (default: IDR)")
    p_insert_snapshot.add_argument("--notes", help="Notes or qualitative context")
    p_insert_snapshot.set_defaults(func=handle_insert_snapshot)

    p_snapshots = subparsers.add_parser("snapshots", help="List net worth balance snapshots")
    p_snapshots.add_argument("--limit", type=int, default=30)
    p_snapshots.set_defaults(func=handle_list_snapshots)

    p_insert_commitment = subparsers.add_parser("insert-commitment", help="Record a recurring financial commitment / burn")
    p_insert_commitment.add_argument("--name", required=True, help="Commitment name (e.g. Rent, Notion Plus)")
    p_insert_commitment.add_argument("--amount", type=float, required=True, help="Payment amount")
    p_insert_commitment.add_argument("--category", default="saas", help="Category (housing, saas, insurance, cloud, lifestyle)")
    p_insert_commitment.add_argument("--frequency", choices=["monthly", "yearly", "quarterly", "weekly"], default="monthly")
    p_insert_commitment.add_argument("--currency", default="IDR")
    p_insert_commitment.add_argument("--account-id", type=int, help="Linked payment account ID")
    p_insert_commitment.add_argument("--renewal-date", help="Next renewal / charge date (YYYY-MM-DD)")
    p_insert_commitment.add_argument("--notes", help="Notes")
    p_insert_commitment.set_defaults(func=handle_insert_commitment)

    p_commitments = subparsers.add_parser("commitments", help="List recurring financial commitments")
    p_commitments.add_argument("--category", help="Filter by category")
    p_commitments.add_argument("--status", choices=["active", "cancelled"], default="active")
    p_commitments.set_defaults(func=handle_list_commitments)

    p_runway = subparsers.add_parser("runway", help="Display sovereign runway in months and treasury status")
    p_runway.set_defaults(func=handle_runway)

    # radar
    p_radar = subparsers.add_parser("radar", help="Human capital reconnection radar (detects neglected relationships)")
    p_radar.add_argument("--tier", type=int, choices=[1, 2, 3], help="Filter by Dunbar tier")
    p_radar.add_argument("--overdue-only", action="store_true", help="Show only overdue contacts")
    p_radar.add_argument("--limit", type=int, default=50)
    p_radar.set_defaults(func=handle_radar)

    p_set_tier = subparsers.add_parser("set-tier", help="Set Dunbar relationship tier and contact cadence")
    p_set_tier.add_argument("--contact-id", type=int, required=True, help="Contact ID")
    p_set_tier.add_argument("--tier", type=int, choices=[1, 2, 3], required=True, help="Tier 1 (Inner), 2 (Core), 3 (Broad)")
    p_set_tier.add_argument("--cadence", type=int, help="Touch cadence in days (default: 14 for T1, 60 for T2, 180 for T3)")
    p_set_tier.set_defaults(func=handle_set_tier)

    # life ops
    p_insert_maintenance = subparsers.add_parser("insert-maintenance", help="Schedule maintenance task or document expiration")
    p_insert_maintenance.add_argument("--name", required=True, help="Item / task name (e.g. Passport Renewal, Bike Service)")
    p_insert_maintenance.add_argument("--due-date", required=True, help="Due date (YYYY-MM-DD)")
    p_insert_maintenance.add_argument("--category", default="general", help="Category (vehicle, home, legal_id, gadget, health)")
    p_insert_maintenance.add_argument("--interval", type=int, help="Recurring interval in days (for auto-rescheduling)")
    p_insert_maintenance.add_argument("--cost", type=float, default=0.0, help="Estimated cost")
    p_insert_maintenance.add_argument("--gadget-id", type=int, help="Linked gadget ID")
    p_insert_maintenance.add_argument("--notes", help="Notes or documentation URL")
    p_insert_maintenance.set_defaults(func=handle_insert_maintenance)

    p_maintenance = subparsers.add_parser("maintenance", help="List maintenance items and upcoming expirations")
    p_maintenance.add_argument("--status", choices=["pending", "completed"], default="pending")
    p_maintenance.add_argument("--category", help="Filter by category")
    p_maintenance.add_argument("--due-within", type=int, help="Show items due within N days")
    p_maintenance.add_argument("--limit", type=int, default=50)
    p_maintenance.set_defaults(func=handle_list_maintenance)

    p_complete_maintenance = subparsers.add_parser("complete-maintenance", help="Mark a maintenance item as completed")
    p_complete_maintenance.add_argument("id", type=int, help="Maintenance item ID")
    p_complete_maintenance.add_argument("--cost", type=float, help="Actual cost incurred")
    p_complete_maintenance.add_argument("--date", help="Completion date (default: today)")
    p_complete_maintenance.set_defaults(func=handle_complete_maintenance)

    # retrospectives
    p_insert_review = subparsers.add_parser("insert-review", help="Log a sprint retrospective review")
    p_insert_review.add_argument("--start", required=True, help="Period start date (YYYY-MM-DD)")
    p_insert_review.add_argument("--end", required=True, help="Period end date (YYYY-MM-DD)")
    p_insert_review.add_argument("--type", choices=["weekly", "monthly", "quarterly"], default="monthly")
    p_insert_review.add_argument("--wins", help="Key wins and accomplishments")
    p_insert_review.add_argument("--drains", help="Energy drains, burnout, bottlenecks")
    p_insert_review.add_argument("--lessons", help="Operational or mental model lessons")
    p_insert_review.add_argument("--focus", help="Focus themes and key bets for next sprint")
    p_insert_review.add_argument("--rating", type=int, default=7, help="Sprint satisfaction rating (1-10)")
    p_insert_review.add_argument("--notes", help="Additional notes")
    p_insert_review.set_defaults(func=handle_insert_review)

    p_reviews = subparsers.add_parser("reviews", help="List sprint retrospectives")
    p_reviews.add_argument("--type", choices=["weekly", "monthly", "quarterly"])
    p_reviews.add_argument("--limit", type=int, default=20)
    p_reviews.set_defaults(func=handle_list_reviews)

    p_show_review = subparsers.add_parser("show-review", help="Show full retrospective review")
    p_show_review.add_argument("id", type=int, help="Retrospective ID")
    p_show_review.set_defaults(func=handle_show_review)

    # life audit
    p_audit = subparsers.add_parser("audit", help="Run comprehensive life audit for missing data, overdue reviews, and life gaps")
    p_audit.add_argument("--json", action="store_true", help="Output audit report as JSON")
    p_audit.set_defaults(func=handle_audit)

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
