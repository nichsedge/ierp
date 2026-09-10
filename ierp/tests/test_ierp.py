"""
Automated unit and integration test suite for iERP.
Runs with standard library unittest (zero external dependencies).
"""

import json
import sqlite3
import tempfile
import unittest
from pathlib import Path
import sys

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))

from ierp.core.audit import generate_life_audit
from ierp.core.config import MONTH_MAP
from ierp.core.dashboard import load_dashboard_html
from ierp.core.db import get_db, init_db
from ierp.core.importers import parse_date_to_iso
from ierp.core.linking import link_events_and_contacts
from ierp.core.merging import merge_two_contacts, auto_merge_contacts
from ierp.core.receipts import upsert_receipt, list_receipts, get_receipt, compute_balance, delete_receipt
from ierp.core.events import insert_event, list_events, get_event, search_events, delete_event
from ierp.core.contacts import insert_contact, list_contacts, get_contact, resolve_contact, delete_contact
from ierp.core.vendors import insert_vendor, list_vendors, get_vendor, toggle_vendor_favorite, delete_vendor
from ierp.core.media import upsert_media_item, ingest_media_records, list_media, upsert_link, list_links
from ierp.core.sources import normalize_row, normalize_rows, _iso_date, normalize_title
from ierp.core.commerce import init_tables as init_commerce_tables, upsert_payment_account, upsert_referral, list_payment_accounts, list_referrals
from ierp.core.decisions import delete_decision, get_decision, insert_decision, list_decisions, review_decision
from ierp.core.finance import compute_monthly_burn, compute_runway, insert_commitment, insert_snapshot, list_commitments, list_snapshots
from ierp.core.gadgets import (
    delete_gadget,
    export_garden_gadgets,
    get_gadget,
    import_garden_gadgets,
    insert_gadget,
    list_gadgets,
    update_gadget,
)
from ierp.core.lifeops import complete_maintenance, get_maintenance, get_maintenance_summary, insert_maintenance, list_maintenance
from ierp.core.projects import delete_project, get_project, get_project_summary, insert_project, list_projects, update_project
from ierp.core.radar import compute_radar, get_radar_summary, update_contact_cadence
from ierp.core.reviews import delete_retrospective, get_retrospective, insert_retrospective, list_retrospectives
from ierp.core.garden import (
    export_garden_all,
    export_garden_decisions,
    export_garden_projects,
    export_garden_reviews,
)
from ierp.cli import build_parser


class TestIERP(unittest.TestCase):

    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.db_path = Path(self.temp_dir.name) / "test_events.db"
        init_db(self.db_path)
        self.conn = get_db(self.db_path)

    def tearDown(self):
        self.conn.close()
        self.temp_dir.cleanup()

    def test_schema_and_indexes(self):
        """Verifies database tables, columns, and indexes are properly created."""
        cursor = self.conn.cursor()

        # Check tables
        tables = [r[0] for r in cursor.execute("SELECT name FROM sqlite_master WHERE type='table'").fetchall()]
        for expected in [
            "events", "contacts", "event_media", "event_contacts", "sync_state", "vendors", 
            "media_items", "links", "payment_accounts", "referrals", "gadgets", "projects",
            "decisions", "networth_snapshots", "recurring_commitments", "maintenance_items", "retrospectives"
        ]:
            self.assertIn(expected, tables)

        # Check contacts columns
        cols = [r[1] for r in cursor.execute("PRAGMA table_info(contacts)").fetchall()]
        for col in ["id", "name", "client", "date", "location", "org", "notes", "email", "phone", "google_id", "source", "tier", "cadence_days"]:
            self.assertIn(col, cols)

        # Check events columns
        e_cols = [r[1] for r in cursor.execute("PRAGMA table_info(events)").fetchall()]
        self.assertIn("project_id", e_cols)

        # Check vendors columns
        v_cols = [r[1] for r in cursor.execute("PRAGMA table_info(vendors)").fetchall()]
        for col in ["id", "name", "category", "location", "phone", "email", "url", "notes", "favorite", "source", "created_at"]:
            self.assertIn(col, v_cols)

        # Check indexes
        indexes = [r[1] for r in cursor.execute("PRAGMA index_list(contacts)").fetchall()]
        self.assertTrue(len(indexes) > 0)
        v_indexes = [r[1] for r in cursor.execute("PRAGMA index_list(vendors)").fetchall()]
        self.assertTrue(len(v_indexes) > 0)

    def test_date_parser(self):
        """Verifies natural language and Notion date strings parse into ISO 8601."""
        self.assertEqual(parse_date_to_iso("2026-08-14"), ("2026-08-14", None))
        self.assertEqual(parse_date_to_iso("August 14, 2026"), ("2026-08-14", None))
        self.assertEqual(parse_date_to_iso("14 August 2026"), ("2026-08-14", None))
        self.assertEqual(parse_date_to_iso("August 14, 2026 -> August 16, 2026"), ("2026-08-14", "2026-08-16"))

    def test_domain_events_crud_and_search(self):
        """Verifies events domain service: insert, list, get, search, and delete."""
        # 1. Insert contact to link
        c_id = insert_contact(name="Budi Pratama", org="TechCorp", db_path=self.db_path)

        # 2. Insert event
        ev_id, linked = insert_event(
            title="Q3 Strategy Workshop",
            place="Bandung",
            start_date="2026-09-01",
            tags="workshop,strategy",
            notes="Quarterly planning session with Budi Pratama",
            contacts=["Budi Pratama"],
            db_path=self.db_path,
        )
        self.assertGreater(ev_id, 0)
        self.assertIn("Budi Pratama", linked)

        # 3. Get event
        ev = get_event(ev_id, db_path=self.db_path)
        self.assertIsNotNone(ev)
        self.assertEqual(ev["title"], "Q3 Strategy Workshop")
        self.assertEqual(ev["place"], "Bandung")
        self.assertEqual(ev["tags"], ["workshop", "strategy"])
        self.assertEqual(len(ev["contacts"]), 1)
        self.assertEqual(ev["contacts"][0]["id"], c_id)

        # 4. Search event
        search_res = search_events("Workshop", db_path=self.db_path)
        self.assertEqual(len(search_res), 1)
        self.assertEqual(search_res[0]["id"], ev_id)

        # 5. List events
        items, total = list_events(tag="strategy", db_path=self.db_path)
        self.assertEqual(total, 1)
        self.assertEqual(items[0]["id"], ev_id)

        # 6. Delete event
        self.assertTrue(delete_event(ev_id, db_path=self.db_path))
        self.assertIsNone(get_event(ev_id, db_path=self.db_path))

    def test_domain_contacts_crud_and_resolve(self):
        """Verifies contacts domain service: insert, resolve, list, get, and delete."""
        c_id = insert_contact(
            name="Sarah Connor",
            org="Cyberdyne",
            email="sarah@resistance.org",
            phone="+1234567890",
            notes="Key person",
            source="manual",
            db_path=self.db_path,
        )
        self.assertGreater(c_id, 0)

        # Resolve contact
        res_id, res_name = resolve_contact(self.conn, "Sarah Connor")
        self.assertEqual(res_id, c_id)
        self.assertEqual(res_name, "Sarah Connor")

        # Resolve by ID
        res_id2, res_name2 = resolve_contact(self.conn, str(c_id))
        self.assertEqual(res_id2, c_id)

        # Get contact
        detail = get_contact(c_id, db_path=self.db_path)
        self.assertIsNotNone(detail)
        self.assertEqual(detail["email"], "sarah@resistance.org")
        self.assertEqual(detail["org"], "Cyberdyne")

        # List contacts
        contacts_list, total = list_contacts(q="resistance", db_path=self.db_path)
        self.assertEqual(total, 1)
        self.assertEqual(contacts_list[0]["id"], c_id)

        # Delete contact
        self.assertTrue(delete_contact(c_id, db_path=self.db_path))
        self.assertIsNone(get_contact(c_id, db_path=self.db_path))

    def test_domain_vendors_crud_and_favorites(self):
        """Verifies vendors domain service: insert, list, toggle favorite, and delete."""
        vid = insert_vendor(
            name="Canggu Moto Rental",
            category="Motorbike Rental",
            location="Canggu, Bali",
            phone="+628111222333",
            favorite=False,
            db_path=self.db_path,
        )
        self.assertGreater(vid, 0)

        v = get_vendor(vid, db_path=self.db_path)
        self.assertIsNotNone(v)
        self.assertEqual(v["name"], "Canggu Moto Rental")
        self.assertFalse(v["favorite"])

        # Toggle favorite to True
        new_fav = toggle_vendor_favorite(vid, db_path=self.db_path)
        self.assertTrue(new_fav)
        v_updated = get_vendor(vid, db_path=self.db_path)
        self.assertTrue(v_updated["favorite"])

        # List vendors
        v_list, total, cats = list_vendors(favorite_only=True, db_path=self.db_path)
        self.assertEqual(total, 1)
        self.assertIn("Motorbike Rental", cats)

        # Delete vendor
        self.assertTrue(delete_vendor(vid, db_path=self.db_path))
        self.assertIsNone(get_vendor(vid, db_path=self.db_path))

    def test_contact_merging(self):
        """Verifies merging manual CRM entries with Google Contacts preserves notes, relations, and sets merged source."""
        cursor = self.conn.cursor()

        # 1. Insert manual contact
        cursor.execute("""
        INSERT INTO contacts (name, client, org, notes, source)
        VALUES ('Adit', 'NewsPage', 'Accenture', 'Manual tag: project lead', 'manual')
        """)
        c_manual = cursor.lastrowid

        # 2. Insert Google synced contact
        cursor.execute("""
        INSERT INTO contacts (name, email, phone, google_id, notes, source)
        VALUES ('Adit', 'adit@example.com', '+62812345678', 'people/c12345', 'Title: Consultant', 'google')
        """)
        c_google = cursor.lastrowid

        # 3. Insert event linked to manual contact
        cursor.execute("""
        INSERT INTO events (title, start_date, notes) VALUES ('Meeting with Adit', '2026-08-14', 'Discuss roadmap')
        """)
        ev_id = cursor.lastrowid
        cursor.execute("INSERT INTO event_contacts (event_id, contact_id) VALUES (?, ?)", (ev_id, c_manual))
        self.conn.commit()

        # 4. Auto-merge
        res = auto_merge_contacts(self.conn)
        self.assertEqual(res["merged_count"], 1)

        # 5. Verify merged record
        merged = cursor.execute("SELECT id, name, client, org, email, phone, google_id, source, notes FROM contacts WHERE id = ?", (c_manual,)).fetchone()
        self.assertIsNotNone(merged)
        self.assertEqual(merged[2], "NewsPage")
        self.assertEqual(merged[4], "adit@example.com")
        self.assertEqual(merged[7], "merged")
        self.assertIn("Manual tag: project lead", merged[8])
        self.assertIn("Title: Consultant", merged[8])

        # 6. Verify duplicate deleted
        deleted = cursor.execute("SELECT id FROM contacts WHERE id = ?", (c_google,)).fetchone()
        self.assertIsNone(deleted)

        # 7. Verify junction table relation preserved
        rel = cursor.execute("SELECT event_id, contact_id FROM event_contacts WHERE event_id = ?", (ev_id,)).fetchone()
        self.assertEqual(rel, (ev_id, c_manual))

    def test_event_contact_linking(self):
        """Verifies relationship discovery between event notes and contact names."""
        cursor = self.conn.cursor()
        cursor.execute("INSERT INTO contacts (name, org, source) VALUES ('Monica', 'Accenture', 'manual')")
        c_id = cursor.lastrowid

        cursor.execute("INSERT INTO events (title, place, notes) VALUES ('Dinner with Monica and team', 'Jakarta', 'Great conversation')")
        e_id = cursor.lastrowid
        self.conn.commit()

        link_count = link_events_and_contacts(self.conn)
        self.assertEqual(link_count, 1)

        linked = cursor.execute("SELECT contact_id FROM event_contacts WHERE event_id = ?", (e_id,)).fetchone()
        self.assertEqual(linked[0], c_id)

    def test_receipts_crud_and_balance(self):
        """Verifies receipt insertion, updating, deletion, listing, retrieval, filtering, and balance computation."""
        cursor = self.conn.cursor()
        cursor.execute("INSERT INTO events (title, start_date, tags) VALUES ('Website Project', '2026-08-26', '[\"project\"]')")
        ev_id = cursor.lastrowid
        cursor.execute("INSERT INTO events (title, start_date, tags) VALUES ('Consulting Retainer', '2026-08-27', '[\"consulting\"]')")
        ev_id_2 = cursor.lastrowid
        self.conn.commit()

        # Insert receipts
        dp_id = upsert_receipt(cursor, event_id=ev_id, amount=1500000, type="income", status="paid", notes="DP received")
        cost_id = upsert_receipt(cursor, event_id=ev_id, amount=233100, type="cost", status="paid", notes="Domain purchase")
        exp_id = upsert_receipt(cursor, event_id=ev_id, amount=5000000, type="expected", status="unpaid", notes="Expected full payment")
        upsert_receipt(cursor, event_id=ev_id_2, amount=3000000, type="income", status="paid", notes="Direct retainer")
        self.conn.commit()

        # Test listing with type filter
        income_rows = list_receipts(type="income", db_path=self.db_path)
        self.assertEqual(len(income_rows), 2)

        cost_rows = list_receipts(type="cost", db_path=self.db_path)
        self.assertEqual(len(cost_rows), 1)
        self.assertEqual(cost_rows[0]["amount"], 233100)

        # Test status filter
        unpaid_rows = list_receipts(status="unpaid", db_path=self.db_path)
        self.assertEqual(len(unpaid_rows), 1)
        self.assertEqual(unpaid_rows[0]["type"], "expected")

        # Test get_receipt
        detail = get_receipt(exp_id, db_path=self.db_path)
        self.assertIsNotNone(detail)
        self.assertEqual(detail["amount"], 5000000)
        self.assertEqual(detail["type"], "expected")
        self.assertEqual(detail["status"], "unpaid")

        # Test update receipt via upsert_receipt
        updated_id = upsert_receipt(cursor, event_id=ev_id, amount=5000000, type="expected", status="paid", notes="Settled full payment", receipt_id=exp_id)
        self.assertEqual(updated_id, exp_id)
        self.conn.commit()
        detail_updated = get_receipt(exp_id, db_path=self.db_path)
        self.assertEqual(detail_updated["status"], "paid")

        # Restore back to unpaid for balance tests
        upsert_receipt(cursor, event_id=ev_id, amount=5000000, type="expected", status="unpaid", notes="Expected full payment", receipt_id=exp_id)
        self.conn.commit()

        # Test balance computation
        bal_global = compute_balance(db_path=self.db_path)
        self.assertEqual(bal_global["total_income"], 4500000)
        self.assertEqual(bal_global["total_costs"], 233100)
        self.assertEqual(bal_global["total_expected"], 5000000)
        self.assertEqual(bal_global["outstanding"], 3500000)

        # Test delete_receipt
        self.assertTrue(delete_receipt(cost_id, db_path=self.db_path))
        self.assertIsNone(get_receipt(cost_id, db_path=self.db_path))

    def test_media_upserts_and_json_merging(self):
        """Verifies media engine idempotent upserts, key-wise JSON merging, and links management."""
        cursor = self.conn.cursor()

        # 1. First upsert
        rec1 = {
            "media_type": "book",
            "title": "Clean Code",
            "source": "hardcover",
            "author": "Robert C. Martin",
            "rating": 4.5,
        }
        item_id = upsert_media_item(cursor, rec1)
        self.assertGreater(item_id, 0)
        self.conn.commit()

        # 2. Second upsert with new non-null field (review and date_logged)
        rec2 = {
            "media_type": "book",
            "title": "Clean Code",
            "source": "hardcover",
            "review": "A timeless classic for developers",
            "date_logged": "2026-08-20",
        }
        item_id_2 = upsert_media_item(cursor, rec2)
        self.assertEqual(item_id, item_id_2)
        self.conn.commit()

        # 3. Verify key-wise merge preserved author and added review
        media_list = list_media(media_type="book", db_path=self.db_path)
        self.assertEqual(len(media_list), 1)
        mid, mtype, title, rating, dlog = media_list[0]
        self.assertEqual(title, "Clean Code")
        self.assertEqual(rating, 4.5)
        self.assertEqual(dlog, "2026-08-20")

        # 4. Ingest bulk media records
        bulk_res = ingest_media_records([
            {"media_type": "film", "title": "Inception", "source": "letterboxd", "rating": 5.0},
            {"media_type": "film", "title": "Interstellar", "source": "letterboxd", "rating": 5.0},
        ], db_path=self.db_path)
        self.assertEqual(bulk_res["items"], 2)

        films = list_media(media_type="film", db_path=self.db_path)
        self.assertEqual(len(films), 2)

        # 5. Links upsert
        lid = upsert_link("GitHub", "https://github.com/nichsedge", category="social", is_public=True, db_path=self.db_path)
        self.assertGreater(lid, 0)
        links = list_links(category="social", public_only=True, db_path=self.db_path)
        self.assertEqual(len(links), 1)
        self.assertEqual(links[0][1], "GitHub")

    def test_sources_normalization(self):
        """Verifies source tracker normalization, date conversion, and title deduplication."""
        # Test _iso_date formats
        self.assertEqual(_iso_date("2026-08-29T12:00:00Z"), "2026-08-29")
        self.assertEqual(_iso_date("08/29/2026"), "2026-08-29")
        self.assertEqual(_iso_date("29 Aug 2026"), "2026-08-29")
        self.assertEqual(_iso_date(None), None)
        self.assertEqual(_iso_date("nan"), None)

        # Test normalize_title
        self.assertEqual(normalize_title("Attack on Titan: The Final Season"), "attackontitanthefinalseason")

        # Test normalize_row for AniList
        raw_anilist = {
            "series_title": "Frieren: Beyond Journey's End",
            "series_season_year": 2023,
            "my_score": 10,
            "my_status": "Completed",
            "my_finish_date": "2024-03-22",
        }
        rec = normalize_row("anilist_anime", raw_anilist)
        self.assertIsNotNone(rec)
        self.assertEqual(rec["media_type"], "anime")
        self.assertEqual(rec["title"], "Frieren: Beyond Journey's End")
        self.assertEqual(rec["rating"], 10)
        self.assertEqual(rec["finished_at"], "2024-03-22")

        # Test normalize_rows with source preference
        raw_rows = [
            {"Title": "Dune", "Author": "Frank Herbert", "My Rating": 5},
            {"Title": "Hyperion", "Author": "Dan Simmons", "My Rating": 5},
        ]
        norm_rows = normalize_rows("hardcover", raw_rows)
        self.assertEqual(len(norm_rows), 2)

    def test_commerce_upserts_and_queries(self):
        """Verifies payment account and referral code management in the commerce engine."""
        cursor = self.conn.cursor()
        init_commerce_tables(cursor)

        # Upsert payment accounts
        upsert_payment_account(
            cursor,
            slug="bca",
            name="Bank Central Asia",
            category="Bank Transfer",
            number="1234567890",
            recipient="Ichsan",
        )
        self.conn.commit()

        accounts = list_payment_accounts(category="Bank Transfer", db_path=self.db_path)
        self.assertEqual(len(accounts), 1)
        self.assertEqual(accounts[0][1], "bca")
        self.assertEqual(accounts[0][4], "1234567890")

        # Upsert referral
        upsert_referral(
            cursor,
            slug="digitalocean",
            name="DigitalOcean",
            category="Cloud",
            code="DO100",
            link="https://m.do.co/c/example",
            benefit="$200 free credit",
            status="ACTIVE",
        )
        self.conn.commit()

        refs = list_referrals(status="ACTIVE", public_only=True, db_path=self.db_path)
        self.assertEqual(len(refs), 1)
        self.assertEqual(refs[0][1], "digitalocean")
        self.assertEqual(refs[0][4], "DO100")

    def test_cli_parser_dispatch(self):
        """Verifies CLI argument parser properly registers all subcommands and their handlers."""
        parser = build_parser()

        # Test command parsing and handler registration
        args_init = parser.parse_args(["init"])
        self.assertTrue(hasattr(args_init, "func"))

        args_list = parser.parse_args(["list", "--limit", "10"])
        self.assertEqual(args_list.limit, 10)
        self.assertTrue(hasattr(args_list, "func"))

        args_balance = parser.parse_args(["balance", "--event-id", "42"])
        self.assertEqual(args_balance.event_id, 42)
        self.assertTrue(hasattr(args_balance, "func"))

        args_vendor = parser.parse_args(["vendors", "--favorite"])
        self.assertTrue(args_vendor.favorite)
        self.assertTrue(hasattr(args_vendor, "func"))

        args_contacts = parser.parse_args(["contacts", "--source", "merged"])
        self.assertEqual(args_contacts.source, "merged")
        self.assertTrue(hasattr(args_contacts, "func"))

        args_gadget = parser.parse_args(["gadgets", "--category", "Smartphone"])
        self.assertEqual(args_gadget.category, "Smartphone")
        self.assertTrue(hasattr(args_gadget, "func"))

        args_ins_gadget = parser.parse_args(["insert-gadget", "--name", "Pixel 9", "--price", "15000000"])
        self.assertEqual(args_ins_gadget.name, "Pixel 9")
        self.assertEqual(args_ins_gadget.price, 15000000.0)
        self.assertTrue(hasattr(args_ins_gadget, "func"))

        args_proj = parser.parse_args(["insert-project", "--title", "Sovereign OS", "--priority", "high"])
        self.assertEqual(args_proj.title, "Sovereign OS")
        self.assertEqual(args_proj.priority, "high")
        self.assertTrue(hasattr(args_proj, "func"))

        args_dec = parser.parse_args(["insert-decision", "--title", "Relocate", "--choice", "Bali", "--confidence", "9"])
        self.assertEqual(args_dec.choice, "Bali")
        self.assertEqual(args_dec.confidence, 9)
        self.assertTrue(hasattr(args_dec, "func"))

        args_snap = parser.parse_args(["insert-snapshot", "--liquid", "50000000", "--investments", "150000000"])
        self.assertEqual(args_snap.liquid, 50000000.0)
        self.assertEqual(args_snap.investments, 150000000.0)
        self.assertTrue(hasattr(args_snap, "func"))

        args_com = parser.parse_args(["insert-commitment", "--name", "Rent", "--amount", "5000000", "--category", "housing"])
        self.assertEqual(args_com.name, "Rent")
        self.assertEqual(args_com.amount, 5000000.0)
        self.assertTrue(hasattr(args_com, "func"))

        args_runway = parser.parse_args(["runway"])
        self.assertTrue(hasattr(args_runway, "func"))

        args_radar = parser.parse_args(["radar", "--tier", "1", "--overdue-only"])
        self.assertEqual(args_radar.tier, 1)
        self.assertTrue(args_radar.overdue_only)
        self.assertTrue(hasattr(args_radar, "func"))

        args_maint = parser.parse_args(["insert-maintenance", "--name", "Bike Oil", "--due-date", "2026-10-01", "--interval", "90"])
        self.assertEqual(args_maint.name, "Bike Oil")
        self.assertEqual(args_maint.interval, 90)
        self.assertTrue(hasattr(args_maint, "func"))

        args_rev = parser.parse_args(["insert-review", "--start", "2026-08-01", "--end", "2026-08-31", "--rating", "8"])
        self.assertEqual(args_rev.start, "2026-08-01")
        self.assertEqual(args_rev.rating, 8)
        self.assertTrue(hasattr(args_rev, "func"))

        args_audit = parser.parse_args(["audit", "--json"])
        self.assertTrue(args_audit.json)
        self.assertTrue(hasattr(args_audit, "func"))

    def test_gadgets_crud_and_sync(self):
        """Verifies gadget domain service: insert, get, list, update, delete, and garden export/import."""
        # 1. Insert vendor to link
        vid = insert_vendor(name="Eraspace", category="Electronics", db_path=self.db_path)

        # 2. Insert gadget
        gid = insert_gadget(
            name="Xiaomi 14T Pro",
            brand="Xiaomi",
            category="Smartphone",
            status="active",
            purchase_date="2026-03-22",
            purchase_price=11000000.0,
            specs={"ram": "12GB", "storage": "256GB"},
            vendor_id=vid,
            notes="Daily driver phone",
            db_path=self.db_path,
        )
        self.assertGreater(gid, 0)

        # 3. Get gadget
        g = get_gadget(gid, db_path=self.db_path)
        self.assertIsNotNone(g)
        self.assertEqual(g["name"], "Xiaomi 14T Pro")
        self.assertEqual(g["vendor_name"], "Eraspace")
        self.assertEqual(g["slug"], "xiaomi-14t-pro")

        # 4. Update gadget
        updated = update_gadget(gid, status="backup", notes="Now a backup device", db_path=self.db_path)
        self.assertTrue(updated)
        g2 = get_gadget("xiaomi-14t-pro", db_path=self.db_path)
        self.assertEqual(g2["status"], "backup")
        self.assertEqual(g2["notes"], "Now a backup device")

        # 5. List gadgets
        rows, total = list_gadgets(category="Smartphone", db_path=self.db_path)
        self.assertEqual(total, 1)
        self.assertEqual(rows[0]["name"], "Xiaomi 14T Pro")

        # 6. Test export to garden directory
        garden_dir = Path(self.temp_dir.name) / "garden_gadgets"
        res = export_garden_gadgets(garden_dir=garden_dir, db_path=self.db_path)
        self.assertEqual(res["notes_written"], 1)
        note_file = garden_dir / "Xiaomi 14T Pro.md"
        self.assertTrue(note_file.exists())
        note_text = note_file.read_text(encoding="utf-8")
        self.assertIn('title: "Xiaomi 14T Pro"', note_text)
        self.assertIn("Now a backup device", note_text)
        self.assertIn("12GB", note_text)

        index_file = garden_dir / "index.md"
        self.assertTrue(index_file.exists())
        index_text = index_file.read_text(encoding="utf-8")
        self.assertIn("[[Xiaomi 14T Pro]]", index_text)

        # 7. Test import from garden directory
        imported, skipped = import_garden_gadgets(garden_dir=garden_dir, db_path=self.db_path)
        self.assertEqual(imported, 0)
        self.assertEqual(skipped, 1)  # Existing matched by slug

        # 8. Delete gadget
        deleted = delete_gadget(gid, db_path=self.db_path)
        self.assertTrue(deleted)
        self.assertIsNone(get_gadget(gid, db_path=self.db_path))

    def test_dashboard_endpoints(self):
        """Verifies dashboard HTTP server API endpoints with filtering, sorting, and pagination."""
        import http.server
        import threading
        import urllib.request
        from ierp.core.dashboard import DashboardRequestHandler
        import ierp.core.config as config

        # Seed test data
        cursor = self.conn.cursor()
        cursor.execute("INSERT INTO events (title, place, start_date, tags, notes) VALUES ('Tech Summit', 'Jakarta', '2026-08-10', '[\"tech\", \"conference\"]', 'Keynote talk')")
        cursor.execute("INSERT INTO events (title, place, start_date, tags, notes) VALUES ('Bali Meetup', 'Bali', '2026-08-20', '[\"community\"]', 'Casual meetup')")
        ev_id = cursor.lastrowid
        cursor.execute("INSERT INTO vendors (name, category, location, favorite) VALUES ('Bali Moto', 'Rental', 'Bali', 1)")
        cursor.execute("INSERT INTO vendors (name, category, location, favorite) VALUES ('Java Moto', 'Rental', 'Jakarta', 0)")
        cursor.execute("INSERT INTO links (label, url, category, is_public) VALUES ('Portfolio', 'https://example.com', 'profile', 1)")
        cursor.execute("INSERT INTO payment_accounts (slug, name, category, number, recipient) VALUES ('bca', 'BCA', 'Bank', '12345678', 'Ichsan')")
        cursor.execute("INSERT INTO referrals (slug, name, category, code, link, status) VALUES ('ref1', 'Cloud', 'Hosting', 'SAVE50', 'https://ref.com', 'ACTIVE')")
        cursor.execute("INSERT INTO media_items (media_type, title, source, data_json) VALUES ('book', 'Sample Book', 'hardcover', '{\"author\": \"Author A\", \"rating\": 4.5}')")
        cursor.execute("INSERT INTO receipts (event_id, amount, type, status, notes) VALUES (?, 1000000, 'income', 'paid', 'Sponsor payment')", (ev_id,))
        self.conn.commit()

        # Temporarily point DB_PATH to test DB
        old_db_path = config.DB_PATH
        config.DB_PATH = self.db_path
        server = None
        server_thread = None
        try:
            server = http.server.HTTPServer(("127.0.0.1", 0), DashboardRequestHandler)
            port = server.server_port
            server_thread = threading.Thread(target=server.serve_forever, daemon=True)
            server_thread.start()

            base_url = f"http://127.0.0.1:{port}"

            # 1. Test GET /
            with urllib.request.urlopen(f"{base_url}/") as res:
                self.assertEqual(res.status, 200)
                html = res.read().decode("utf-8")
                self.assertIn("Journal & CRM ERP", html)
                self.assertIn("Media Logs", html)
                self.assertIn("Vendors", html)
                self.assertIn("Commerce", html)
                self.assertIn("Receipts", html)

            # 2. Test GET /api/stats
            with urllib.request.urlopen(f"{base_url}/api/stats") as res:
                self.assertEqual(res.status, 200)
                data = json.loads(res.read().decode("utf-8"))
                self.assertGreaterEqual(data["total_events"], 2)
                self.assertIn("finance", data)
                self.assertEqual(data["finance"]["total_income"], 1000000)
                self.assertIn("daily_counts", data)
                self.assertIn("2026-08-10", data["daily_counts"])
                self.assertIn("top_places", data)
                self.assertIn("media_summary", data)
                self.assertEqual(data["media_summary"]["book"], 1)
                self.assertIn("cashflow", data)

            # 3. Test GET /api/events with filtering and sorting
            with urllib.request.urlopen(f"{base_url}/api/events?q=Tech&from=2026-08-01&to=2026-08-15&sort=start_date&dir=asc&limit=5&offset=0") as res:
                self.assertEqual(res.status, 200)
                data = json.loads(res.read().decode("utf-8"))
                self.assertEqual(data["total"], 1)
                self.assertEqual(data["items"][0]["title"], "Tech Summit")

            # 4. Test GET /api/vendors and POST /api/vendors/favorite
            with urllib.request.urlopen(f"{base_url}/api/vendors?category=Rental&sort=name&dir=asc&limit=5&offset=0") as res:
                self.assertEqual(res.status, 200)
                data = json.loads(res.read().decode("utf-8"))
                self.assertEqual(data["total"], 2)
                vendor_id = data["items"][0]["id"]

            req = urllib.request.Request(
                f"{base_url}/api/vendors/favorite",
                data=json.dumps({"id": vendor_id}).encode("utf-8"),
                headers={"Content-Type": "application/json"}
            )
            with urllib.request.urlopen(req) as res:
                self.assertEqual(res.status, 200)
                fav_res = json.loads(res.read().decode("utf-8"))
                self.assertEqual(fav_res["status"], "success")

            # 5. Test GET /api/media
            with urllib.request.urlopen(f"{base_url}/api/media?type=book&limit=5&offset=0") as res:
                self.assertEqual(res.status, 200)
                data = json.loads(res.read().decode("utf-8"))
                self.assertEqual(data["total"], 1)
                self.assertEqual(data["items"][0]["title"], "Sample Book")

            # 6. Test GET /api/links
            with urllib.request.urlopen(f"{base_url}/api/links?category=profile&limit=5&offset=0") as res:
                self.assertEqual(res.status, 200)
                data = json.loads(res.read().decode("utf-8"))
                self.assertEqual(data["total"], 1)

            # 7. Test GET /api/payment-accounts and /api/referrals
            with urllib.request.urlopen(f"{base_url}/api/payment-accounts?limit=5&offset=0") as res:
                self.assertEqual(res.status, 200)
                data = json.loads(res.read().decode("utf-8"))
                self.assertEqual(data["total"], 1)

            with urllib.request.urlopen(f"{base_url}/api/referrals?status=active&limit=5&offset=0") as res:
                self.assertEqual(res.status, 200)
                data = json.loads(res.read().decode("utf-8"))
                self.assertEqual(data["total"], 1)

            # 8. Test GET /api/receipts
            with urllib.request.urlopen(f"{base_url}/api/receipts?type=income&status=paid&limit=5&offset=0") as res:
                self.assertEqual(res.status, 200)
                data = json.loads(res.read().decode("utf-8"))
                self.assertEqual(data["total"], 1)
                self.assertEqual(data["items"][0]["amount"], 1000000)
                self.assertEqual(data["items"][0]["type"], "income")

            # 9. Test GET /api/projects, /api/decisions, /api/runway, /api/radar, /api/maintenance, /api/reviews
            with urllib.request.urlopen(f"{base_url}/api/projects") as res:
                self.assertEqual(res.status, 200)
                p_data = json.loads(res.read().decode("utf-8"))
                self.assertIn("items", p_data)

            with urllib.request.urlopen(f"{base_url}/api/decisions") as res:
                self.assertEqual(res.status, 200)
                d_data = json.loads(res.read().decode("utf-8"))
                self.assertIn("items", d_data)

            with urllib.request.urlopen(f"{base_url}/api/runway") as res:
                self.assertEqual(res.status, 200)
                rw_data = json.loads(res.read().decode("utf-8"))
                self.assertIn("runway_months", rw_data)

            with urllib.request.urlopen(f"{base_url}/api/radar") as res:
                self.assertEqual(res.status, 200)
                rd_data = json.loads(res.read().decode("utf-8"))
                self.assertIn("summary", rd_data)

            with urllib.request.urlopen(f"{base_url}/api/maintenance") as res:
                self.assertEqual(res.status, 200)
                m_data = json.loads(res.read().decode("utf-8"))
                self.assertIn("summary", m_data)

            with urllib.request.urlopen(f"{base_url}/api/reviews") as res:
                self.assertEqual(res.status, 200)
                rv_data = json.loads(res.read().decode("utf-8"))
                self.assertIn("items", rv_data)

        finally:
            config.DB_PATH = old_db_path
            if server:
                server.shutdown()
                server.server_close()

    def test_projects_domain_crud_and_events_link(self):
        """Verifies projects domain: insert, update, get, list, summary, and linking to events."""
        pid = insert_project(
            title="Digital Sovereignty Infra",
            description="Building offline-first personal data lake",
            status="active",
            priority="high",
            start_date="2026-08-01",
            target_date="2026-12-31",
            db_path=self.db_path,
        )
        self.assertIsInstance(pid, int)

        proj = get_project(pid, db_path=self.db_path)
        self.assertIsNotNone(proj)
        self.assertEqual(proj["title"], "Digital Sovereignty Infra")
        self.assertEqual(proj["priority"], "high")

        # Insert event linked to project
        eid, _ = insert_event(
            title="Deploy SQLite WAL cluster",
            start_date="2026-08-15",
            project_id=pid,
            db_path=self.db_path,
        )

        # Insert decision linked to project
        did = insert_decision(
            title="Choose storage architecture",
            choice="SQLite WAL mode",
            project_id=pid,
            db_path=self.db_path,
        )

        summary = get_project_summary(pid, db_path=self.db_path)
        self.assertEqual(len(summary["events"]), 1)
        self.assertEqual(summary["events"][0]["title"], "Deploy SQLite WAL cluster")
        self.assertEqual(len(summary["decisions"]), 1)
        self.assertEqual(summary["decisions"][0]["choice"], "SQLite WAL mode")

        # Update and list
        update_project(pid, status="completed", db_path=self.db_path)
        projs = list_projects(status="completed", db_path=self.db_path)
        self.assertEqual(len(projs), 1)

        delete_project(pid, db_path=self.db_path)
        self.assertIsNone(get_project(pid, db_path=self.db_path))

    def test_decisions_domain_crud_and_review(self):
        """Verifies decision journal: logging decisions and completing retrospective review."""
        did = insert_decision(
            title="Pivot to Fractional Data Engineering",
            choice="Accept retainer contract",
            context="Current job has high meetings to output ratio",
            expected_outcome="Double free time and increase autonomy",
            confidence=8,
            review_date="2026-11-01",
            db_path=self.db_path,
        )
        self.assertIsInstance(did, int)

        d = get_decision(did, db_path=self.db_path)
        self.assertEqual(d["confidence"], 8)
        self.assertEqual(d["status"], "pending")

        # Review decision
        ok = review_decision(
            did,
            actual_outcome="Free time increased by 60%, revenue up 25%",
            status="reviewed",
            db_path=self.db_path,
        )
        self.assertTrue(ok)

        d_reviewed = get_decision(did, db_path=self.db_path)
        self.assertEqual(d_reviewed["status"], "reviewed")
        self.assertIn("Free time increased", d_reviewed["actual_outcome"])

        delete_decision(did, db_path=self.db_path)
        self.assertIsNone(get_decision(did, db_path=self.db_path))

    def test_finance_snapshots_commitments_and_runway(self):
        """Verifies net worth snapshots, recurring burn commitments, and runway calculation."""
        # Insert net worth snapshot
        sid = insert_snapshot(
            snapshot_date="2026-08-30",
            liquid_cash=120000000.0,    # 120M IDR liquid
            investments=300000000.0,    # 300M IDR investments
            hard_assets=50000000.0,     # 50M IDR assets
            liabilities=0.0,
            db_path=self.db_path,
        )
        self.assertIsInstance(sid, int)

        snaps = list_snapshots(db_path=self.db_path)
        self.assertEqual(len(snaps), 1)
        self.assertEqual(snaps[0]["net_worth"], 470000000.0)

        # Insert commitments
        cid1 = insert_commitment(
            name="Apartment Rent",
            amount=8000000.0,
            frequency="monthly",
            category="housing",
            db_path=self.db_path,
        )
        cid2 = insert_commitment(
            name="Cloud Servers",
            amount=2000000.0,
            frequency="monthly",
            category="cloud",
            db_path=self.db_path,
        )
        self.assertIsInstance(cid1, int)
        self.assertIsInstance(cid2, int)

        burn = compute_monthly_burn(db_path=self.db_path)
        self.assertEqual(burn["total_monthly_burn"], 10000000.0)
        self.assertEqual(burn["commitments_count"], 2)

        # Calculate runway: 120M / 10M = 12.0 months
        runway = compute_runway(db_path=self.db_path)
        self.assertEqual(runway["runway_months"], 12.0)
        self.assertIn("SOVEREIGN", runway["status_label"])

    def test_radar_and_contact_cadence(self):
        """Verifies contact cadence and reconnection radar overdue detection."""
        cid = insert_contact(
            name="Budi Santoso",
            tier=1,
            cadence_days=14,
            db_path=self.db_path,
        )
        self.assertIsInstance(cid, int)

        # Without any event interaction, contact is flagged overdue
        radar = compute_radar(overdue_only=True, db_path=self.db_path)
        self.assertTrue(any(c["id"] == cid for c in radar))

        # Log an event with this contact today
        insert_event(
            title="Catch up coffee with Budi",
            start_date="2026-09-07",
            contacts=[cid],
            db_path=self.db_path,
        )

        # Re-compute radar: should no longer be overdue
        radar_after = compute_radar(overdue_only=True, db_path=self.db_path)
        self.assertFalse(any(c["id"] == cid for c in radar_after))

        summary = get_radar_summary(db_path=self.db_path)
        self.assertGreaterEqual(summary["total_contacts"], 1)

    def test_maintenance_lifecycle_and_auto_reschedule(self):
        """Verifies maintenance task completion and automatic recurring rescheduling."""
        mid = insert_maintenance(
            name="Motorbike Oil Change",
            due_date="2026-09-01",
            interval_days=60,
            cost=150000.0,
            category="vehicle",
            db_path=self.db_path,
        )
        self.assertIsInstance(mid, int)

        item = get_maintenance(mid, db_path=self.db_path)
        self.assertEqual(item["status"], "pending")
        self.assertTrue(item["is_overdue"])

        # Complete task
        res = complete_maintenance(mid, cost=160000.0, completion_date="2026-09-07", db_path=self.db_path)
        self.assertTrue(res["success"])
        self.assertIsNotNone(res["next_item_id"])
        self.assertEqual(res["next_due_date"], "2026-11-06")

        # Verify old item completed and new item scheduled
        completed_item = get_maintenance(mid, db_path=self.db_path)
        self.assertEqual(completed_item["status"], "completed")
        self.assertEqual(completed_item["cost"], 160000.0)

        next_item = get_maintenance(res["next_item_id"], db_path=self.db_path)
        self.assertEqual(next_item["status"], "pending")
        self.assertEqual(next_item["due_date"], "2026-11-06")

    def test_retrospectives_crud(self):
        """Verifies sprint retrospective logs: insert, list, get, and delete."""
        rid = insert_retrospective(
            period_start="2026-08-01",
            period_end="2026-08-31",
            period_type="monthly",
            wins="Shipped iERP v2 with Sovereign Life OS",
            drains_burnout="Too much time debugging brittle CSS",
            lessons="Simplicity and local SQLite WAL mode beat distributed databases",
            focus_next="Launch Decision Journal and calibrate judgment",
            rating=9,
            db_path=self.db_path,
        )
        self.assertIsInstance(rid, int)

        retro = get_retrospective(rid, db_path=self.db_path)
        self.assertEqual(retro["rating"], 9)
        self.assertIn("Shipped iERP v2", retro["wins"])

        retros = list_retrospectives(period_type="monthly", db_path=self.db_path)
        self.assertEqual(len(retros), 1)

        delete_retrospective(rid, db_path=self.db_path)
        self.assertIsNone(get_retrospective(rid, db_path=self.db_path))

    def test_audit_engine(self):
        """Verifies life audit engine identifies missing data, overdue items, and recommendations."""
        # 1. On empty DB, audit should flag missing snapshot, commitments, and reviews
        audit_res = generate_life_audit(db_path=self.db_path)
        self.assertGreaterEqual(audit_res["actions_needed"], 2)
        self.assertGreater(audit_res["total_findings"], 0)

        # Check domain presence in findings
        domains = {f["domain"] for f in audit_res["findings"]}
        self.assertIn("Treasury", domains)
        self.assertIn("Decisions", domains)
        self.assertIn("Life Ops", domains)
        self.assertIn("Retrospectives", domains)

        # 2. Add snapshot and commitment, verify Treasury actions clear
        insert_snapshot(liquid_cash=50000000.0, investments=100000000.0, db_path=self.db_path)
        insert_commitment(name="Rent", amount=5000000.0, category="housing", db_path=self.db_path)

        audit_res_updated = generate_life_audit(db_path=self.db_path)
        treasury_actions = [
            f for f in audit_res_updated["findings"]
            if f["domain"] == "Treasury" and f["severity"] == "action_needed"
        ]
        self.assertEqual(len(treasury_actions), 0)

    def test_fts5_events_indexing_and_bm25_search(self):
        """Verifies SQLite FTS5 virtual table, sync triggers, BM25 ranking, and query fallback."""
        cursor = self.conn.cursor()

        # 1. Verify events_fts virtual table exists
        tables = [r[0] for r in cursor.execute("SELECT name FROM sqlite_master WHERE type='table'").fetchall()]
        self.assertIn("events_fts", tables)

        # 2. Insert events and verify they are searchable via FTS5
        eid1, _ = insert_event(
            title="Deploy SQLite WAL Mode",
            place="Jakarta",
            start_date="2026-08-01",
            tags=["sqlite", "tech"],
            notes="Configured Write-Ahead Logging for high concurrency.",
            db_path=self.db_path,
        )
        eid2, _ = insert_event(
            title="Weekend in Bandung",
            place="Bandung",
            start_date="2026-08-05",
            tags=["travel"],
            notes="Relaxing trip with friends.",
            db_path=self.db_path,
        )
        eid3, _ = insert_event(
            title="FTS5 Full Text Search Architecture",
            place="Jakarta",
            start_date="2026-08-10",
            tags=["sqlite", "search"],
            notes="Implemented SQLite FTS5 with BM25 ranking for sub-millisecond timeline queries.",
            db_path=self.db_path,
        )

        # 3. FTS5 Search for 'SQLite'
        results = search_events("SQLite", db_path=self.db_path)
        self.assertEqual(len(results), 2)
        found_ids = {r["id"] for r in results}
        self.assertIn(eid1, found_ids)
        self.assertIn(eid3, found_ids)
        self.assertNotIn(eid2, found_ids)
        # Check snippet presence
        self.assertTrue(any("SQLite" in r.get("notes_snippet", "") or "SQLite" in r.get("title_snippet", "") for r in results))

        # 4. Search via list_events with q parameter
        events_list, total_count = list_events(q="Bandung", db_path=self.db_path)
        self.assertEqual(total_count, 1)
        self.assertEqual(events_list[0]["id"], eid2)

        # 5. Verify update trigger syncs events_fts
        cursor.execute("UPDATE events SET title = 'Visit Yogyakarta', place = 'Yogyakarta', notes = 'Exploring Prambanan and Malioboro' WHERE id = ?", (eid2,))
        self.conn.commit()

        # Searching old term 'Bandung' should return 0, 'Yogyakarta' should return 1
        self.assertEqual(len(search_events("Bandung", db_path=self.db_path)), 0)
        yogya_res = search_events("Yogyakarta", db_path=self.db_path)
        self.assertEqual(len(yogya_res), 1)
        self.assertEqual(yogya_res[0]["id"], eid2)

        # 6. Verify delete trigger syncs events_fts
        delete_event(eid1, db_path=self.db_path)
        self.assertEqual(len(search_events("WAL", db_path=self.db_path)), 0)

        # 7. Query with special characters should not crash (graceful fallback)
        res_symbols = search_events(":::???***", db_path=self.db_path)
        self.assertIsInstance(res_symbols, list)

    def test_garden_export_projects_decisions_retrospectives(self):
        """Verifies exporting Projects, Decisions (PDRs), and Retrospectives to Digital Garden."""
        with tempfile.TemporaryDirectory() as garden_tmp:
            garden_dir = Path(garden_tmp)

            # 1. Insert Project with linked event
            pid = insert_project(
                slug="sovereign-infra",
                title="Sovereign Infra",
                description="Self-hosted zero-dependency personal ERP architecture.",
                priority="high",
                start_date="2026-08-01",
                target_date="2026-12-31",
                db_path=self.db_path,
            )
            insert_event(
                title="Deploy WAL Engine",
                place="Jakarta",
                start_date="2026-08-02",
                project_id=pid,
                notes="Enabled non-blocking concurrent SQLite writes.",
                db_path=self.db_path,
            )

            # 2. Insert Decision linked to Project
            did = insert_decision(
                title="Adopt SQLite Over Postgres",
                choice="Pure SQLite WAL",
                context="Evaluating storage layer for offline-first personal operating system.",
                expected_outcome="Sub-millisecond queries, zero daemon management, zero supply chain bloat.",
                confidence=9,
                review_date="2026-11-01",
                project_id=pid,
                db_path=self.db_path,
            )
            review_decision(
                did,
                actual_outcome="Exceeded expectations with zero-cost and instant startup.",
                status="validated",
                db_path=self.db_path,
            )

            # 3. Insert Retrospective
            insert_retrospective(
                period_start="2026-08-01",
                period_end="2026-08-31",
                period_type="monthly",
                wins="Shipped FTS5 search and Digital Garden deep sync.",
                drains_burnout="Manual repetitive documentation sync.",
                lessons="Automation via agent workflows eliminates friction.",
                focus_next="Expand mobile GPS ingestion and banking importers.",
                rating=10,
                db_path=self.db_path,
            )

            # 4. Dry run test (should write 0 files)
            dry_res = export_garden_all(garden_root=garden_dir, dry_run=True, db_path=self.db_path)
            self.assertTrue(dry_res["dry_run"])
            self.assertFalse((garden_dir / "Knowledge" / "Projects" / "sovereign-infra.md").exists())

            # 5. Full export test
            export_res = export_garden_all(garden_root=garden_dir, dry_run=False, db_path=self.db_path)
            self.assertEqual(export_res["domains"]["projects"]["notes_written"], 1)
            self.assertEqual(export_res["domains"]["decisions"]["notes_written"], 1)
            self.assertEqual(export_res["domains"]["reviews"]["notes_written"], 1)

            # Verify Project note and index
            proj_file = garden_dir / "Knowledge" / "Projects" / "sovereign-infra.md"
            self.assertTrue(proj_file.exists())
            proj_content = proj_file.read_text(encoding="utf-8")
            self.assertIn("title: \"Sovereign Infra\"", proj_content)
            self.assertIn("publish_external: true", proj_content)
            self.assertIn("Deploy WAL Engine", proj_content)
            self.assertTrue((garden_dir / "Knowledge" / "Projects" / "index.md").exists())

            # Verify Decision (PDR) note and index
            dec_file = garden_dir / "Knowledge" / "Decisions" / "adopt-sqlite-over-postgres.md"
            self.assertTrue(dec_file.exists())
            dec_content = dec_file.read_text(encoding="utf-8")
            self.assertIn("confidence: 9", dec_content)
            self.assertIn("Pure SQLite WAL", dec_content)
            self.assertIn("Exceeded expectations", dec_content)
            self.assertIn("[[Knowledge/Projects/sovereign-infra|Sovereign Infra]]", dec_content)
            self.assertTrue((garden_dir / "Knowledge" / "Decisions" / "index.md").exists())

            # Verify Retrospective note and index
            retro_file = garden_dir / "Write" / "Retrospectives" / "2026-08-01_2026-08-31_monthly.md"
            self.assertTrue(retro_file.exists())
            retro_content = retro_file.read_text(encoding="utf-8")
            self.assertIn("Shipped FTS5 search", retro_content)
            self.assertIn("rating: 10", retro_content)
            self.assertTrue((garden_dir / "Write" / "Retrospectives" / "index.md").exists())


def run_tests():
    suite = unittest.TestLoader().loadTestsFromTestCase(TestIERP)
    runner = unittest.TextTestRunner(verbosity=2)
    return runner.run(suite)


if __name__ == "__main__":
    unittest.main()
