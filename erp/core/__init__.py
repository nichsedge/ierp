"""
Personal ERP Core Package.
"""

from .config import DB_PATH, MEDIA_DIR, GOOGLE_CREDS_PATH, GOOGLE_TOKEN_PATH, GEO_CACHE_PATH
from .db import get_db, db_session, init_db
from .geocoding import reverse_geocode
from .merging import merge_two_contacts, auto_merge_contacts
from .linking import link_events_and_contacts, run_manual_link
from .google_sync import get_google_access_token, sync_google_contacts
from .importers import parse_date_to_iso, import_notion_export, import_crm_contacts, import_timeline
from .dashboard import start_dashboard_server, ingest_location_ping

__all__ = [
    "DB_PATH", "MEDIA_DIR", "GOOGLE_CREDS_PATH", "GOOGLE_TOKEN_PATH", "GEO_CACHE_PATH",
    "get_db", "db_session", "init_db",
    "reverse_geocode",
    "merge_two_contacts", "auto_merge_contacts",
    "link_events_and_contacts", "run_manual_link",
    "get_google_access_token", "sync_google_contacts",
    "parse_date_to_iso", "import_notion_export", "import_crm_contacts", "import_timeline",
    "start_dashboard_server", "ingest_location_ping"
]
