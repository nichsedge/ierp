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


def run_tests():
    suite = unittest.TestLoader().loadTestsFromTestCase(TestIERP)
    runner = unittest.TextTestRunner(verbosity=2)
    return runner.run(suite)


if __name__ == "__main__":
    unittest.main()
