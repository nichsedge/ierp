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

# Media tracker profiles (used by `ierp sync`).
# Values are read from environment variables, loaded from ~/.secrets or ierp/.env
# (see fetchers.load_environment). Format: IERP_<SOURCE>__<FIELD>, e.g.
#   IERP_GOODREADS__USER_ID=74584614
#   IERP_HARDCOVER__USERNAME=nichsedge
#   HARDCOVER_API_KEY=...   (token; may include the "Bearer " prefix)
_MEDIA_ENV_PREFIX = "IERP_"


def _media_profile(source: str, fields: dict) -> dict:
    """fields: {arg_name: env_suffix}. Env value wins over the default."""
    import os
    out = {}
    for arg, suffix in fields.items():
        val = os.environ.get(f"{_MEDIA_ENV_PREFIX}{source.upper().replace('-', '_')}__{suffix}")
        out[arg] = val if val else fields[arg]
    return out


def media_profiles() -> dict:
    """Resolves media tracker profiles from env with hardcoded fallbacks."""
    return {
        "hardcover": _media_profile("hardcover", {"username": "nichsedge"}),
        "goodreads": _media_profile("goodreads", {"user_id": "74584614"}),
        "letterboxd": _media_profile("letterboxd", {"username": "PenyulTekowel"}),
        "anilist_anime": _media_profile("anilist_anime", {"username": "laataiasu"}),
        "anilist_manga": _media_profile("anilist_manga", {"username": "laataiasu"}),
        "mydramalist": _media_profile("mydramalist", {"username": "Chanculus"}),
    }

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
