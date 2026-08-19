"""
Central configuration, paths, and constants for iERP.
Strictly zero external dependencies (Python standard library only).
"""

from pathlib import Path

# Base Paths
BASE_DIR = Path(__file__).resolve().parent.parent
DB_PATH = BASE_DIR / "events.db"
MEDIA_DIR = BASE_DIR / "events_media"
GOOGLE_CREDS_PATH = BASE_DIR / "google_credentials.json"
GOOGLE_TOKEN_PATH = BASE_DIR / "google_token.json"
GEO_CACHE_PATH = BASE_DIR / "geocode_cache.json"

# ANSI Terminal Color Tokens
C_RESET = "\033[0m"
C_BOLD = "\033[1m"
C_GREEN = "\033[32m"
C_CYAN = "\033[36m"
C_YELLOW = "\033[33m"
C_RED = "\033[31m"
C_MAGENTA = "\033[35m"

# Notion Month Name Mapping
MONTH_MAP = {
    "january": "01", "jan": "01",
    "february": "02", "feb": "02",
    "march": "03", "mar": "03",
    "april": "04", "apr": "04",
    "may": "05",
    "june": "06", "jun": "06",
    "july": "07", "jul": "07",
    "august": "08", "aug": "08",
    "september": "09", "sep": "09", "sept": "09",
    "october": "10", "oct": "10",
    "november": "11", "nov": "11",
    "december": "12", "dec": "12"
}
