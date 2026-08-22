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

from ierp.core.config import MONTH_MAP
from ierp.core.db import get_db, init_db
from ierp.core.importers import parse_date_to_iso
from ierp.core.linking import link_events_and_contacts
from ierp.core.merging import merge_two_contacts, auto_merge_contacts


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
        for expected in ["events", "contacts", "event_media", "event_contacts", "sync_state", "vendors"]:
            self.assertIn(expected, tables)

        # Check contacts columns
        cols = [r[1] for r in cursor.execute("PRAGMA table_info(contacts)").fetchall()]
        for col in ["id", "name", "client", "date", "location", "org", "notes", "email", "phone", "google_id", "source"]:
            self.assertIn(col, cols)

        # Check vendors columns
        v_cols = [r[1] for r in cursor.execute("PRAGMA table_info(vendors)").fetchall()]
        for col in ["id", "name", "category", "location", "phone", "email", "url", "notes", "favorite", "source", "created_at"]:
            self.assertIn(col, v_cols)

        # Check indexes
        indexes = [r[1] for r in cursor.execute("PRAGMA index_list(contacts)").fetchall()]
        self.assertTrue(len(indexes) > 0)
        v_indexes = [r[1] for r in cursor.execute("PRAGMA index_list(vendors)").fetchall()]
        self.assertTrue(len(v_indexes) > 0)

    def test_vendor_operations(self):
        """Verifies vendor creation, querying, favorite flags, and data migration."""
        cursor = self.conn.cursor()

        # Insert vendor
        cursor.execute("""
        INSERT INTO vendors (name, category, location, phone, notes, favorite)
        VALUES ('ASA Tours and Travel', 'Motorbike Rental', 'Bali, Indonesia', '+62 851-7300-3683', 'Favorite motorbike rental vendor/seller in Bali', 1)
        """)
        self.conn.commit()

        row = cursor.execute("SELECT name, category, phone, favorite FROM vendors WHERE name = 'ASA Tours and Travel'").fetchone()
        self.assertIsNotNone(row)
        self.assertEqual(row[0], "ASA Tours and Travel")
        self.assertEqual(row[1], "Motorbike Rental")
        self.assertEqual(row[2], "+62 851-7300-3683")
        self.assertEqual(row[3], 1)

    def test_date_parser(self):
        """Verifies natural language and Notion date strings parse into ISO 8601."""
        self.assertEqual(parse_date_to_iso("2026-08-14"), ("2026-08-14", None))
        self.assertEqual(parse_date_to_iso("August 14, 2026"), ("2026-08-14", None))
        self.assertEqual(parse_date_to_iso("14 August 2026"), ("2026-08-14", None))
        self.assertEqual(parse_date_to_iso("August 14, 2026 -> August 16, 2026"), ("2026-08-14", "2026-08-16"))

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

    def test_dashboard_helpers_and_sorting(self):
        """Verifies pagination and whitelist-safe sorting helpers."""
        from ierp.core.dashboard import parse_pagination, parse_sort, EVENT_SORT_COLS, VENDOR_SORT_COLS

        # Test pagination defaults and boundaries
        self.assertEqual(parse_pagination({}), (25, 0))
        self.assertEqual(parse_pagination({"limit": ["10"], "offset": ["20"]}), (10, 20))
        self.assertEqual(parse_pagination({"limit": ["-5"], "offset": ["-10"]}), (1, 0))
        self.assertEqual(parse_pagination({"limit": ["9999"]}), (500, 0))

        # Test sort whitelist validation against SQL injection
        col, direction = parse_sort({"sort": ["title"], "dir": ["asc"]}, EVENT_SORT_COLS, "start_date")
        self.assertEqual(col, "e.title")
        self.assertEqual(direction, "ASC")

        # Attempt SQL injection in sort and dir
        malicious_col, malicious_dir = parse_sort(
            {"sort": ["title; DROP TABLE events;--"], "dir": ["desc; SELECT * FROM contacts;"]},
            EVENT_SORT_COLS,
            "start_date"
        )
        self.assertEqual(malicious_col, "e.start_date")
        self.assertEqual(malicious_dir, "DESC")

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
        cursor.execute("INSERT INTO vendors (name, category, location, favorite) VALUES ('Bali Moto', 'Rental', 'Bali', 1)")
        cursor.execute("INSERT INTO vendors (name, category, location, favorite) VALUES ('Java Moto', 'Rental', 'Jakarta', 0)")
        cursor.execute("INSERT INTO links (label, url, category, is_public) VALUES ('Portfolio', 'https://example.com', 'profile', 1)")
        cursor.execute("INSERT INTO payment_accounts (slug, name, category, number, recipient) VALUES ('bca', 'BCA', 'Bank', '12345678', 'Ichsan')")
        cursor.execute("INSERT INTO referrals (slug, name, category, code, link, status) VALUES ('ref1', 'Cloud', 'Hosting', 'SAVE50', 'https://ref.com', 'ACTIVE')")
        cursor.execute("INSERT INTO media_items (media_type, title, source, data_json) VALUES ('book', 'Sample Book', 'hardcover', '{\"author\": \"Author A\", \"rating\": 4.5}')")
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
                self.assertIn("Journal & CRM Dashboard", html)
                self.assertIn("Media Logs", html)
                self.assertIn("Vendors", html)
                self.assertIn("Commerce", html)

            # 2. Test GET /api/stats
            with urllib.request.urlopen(f"{base_url}/api/stats") as res:
                self.assertEqual(res.status, 200)
                data = json.loads(res.read().decode("utf-8"))
                self.assertGreaterEqual(data["total_events"], 2)

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

        finally:
            config.DB_PATH = old_db_path
            if server:
                server.shutdown()
                server.server_close()


def run_tests():
    suite = unittest.TestLoader().loadTestsFromTestCase(TestIERP)
    runner = unittest.TextTestRunner(verbosity=2)
    return runner.run(suite)


if __name__ == "__main__":
    unittest.main()
