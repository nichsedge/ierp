"""
Source normalization for media ingestion (stdlib only).
Maps raw rows from external trackers (Goodreads, Hardcover, Letterboxd,
AniList, MyDramaList) onto the ierp media schema. The mapping tables below
are the single place to touch when a source adds or renames columns.
"""

import math
import re
from typing import Optional

# get-data source key -> (ierp media_type, ierp source name)
SOURCE_MAP = {
    "hardcover": ("book", "hardcover"),
    "goodreads": ("book", "goodreads"),
    "letterboxd": ("film", "letterboxd"),
    "anilist_anime": ("anime", "anilist"),
    "anilist_manga": ("manga", "anilist"),
    "mydramalist": ("drama", "mydramalist"),
}

# When the same normalized title exists from multiple sources within a
# media_type, only rows from the first source listed here are kept.
# (Hardcover is the ONLY book source; goodreads fetcher is unused for sync.)
SOURCE_PREFERENCE = {
    "book": ("hardcover", "goodreads"),
}

# Candidate column names per record field (first non-empty match wins).
FIELD_CANDIDATES = {
    "title": ("Title", "Name", "series_title", "manga_title", "title"),
    "original_title": ("series_native_title", "native_title", "Original Title"),
    "year": ("Year", "year", "release_year", "series_season_year",
             "Year Published", "year_published"),
    "author": ("Author", "author"),
    "status": ("Reading Status", "status", "Progress", "my_status"),
    "rating": ("My Rating", "Rating", "rating", "Score", "score", "my_score"),
    "progress": ("Progress", "progress", "my_watched_episodes"),
    "review": ("My Review", "my_comments", "notes"),
}

# Candidate column names per date field.
DATE_CANDIDATES = {
    "started_at": ("Date Started", "started_at", "Start Date", "my_start_date"),
    "finished_at": ("Date Read", "finished_at", "Finish Date", "my_finish_date"),
    "date_logged": ("Date Added", "Date", "date", "updated_at", "created_at"),
}

# Columns excluded from the `extra` dict (mapped to real fields or redundant).
_EXTRA_EXCLUDED = {"Title", "Name", "series_title", "manga_title"}

# Values that mean "no data" in tracker exports.
_NULLISH = {"", "none", "null", "nan", "0000-00-00"}


def _clean(v):
    """None/NaN/nullish strings -> None; integral floats -> int; else passthrough."""
    if v is None:
        return None
    if isinstance(v, float) and math.isnan(v):
        return None
    s = str(v).strip()
    if s.lower() in _NULLISH:
        return None
    if isinstance(v, float) and v.is_integer():
        return int(v)
    return v


def _first(r: dict, candidates: tuple):
    for col in candidates:
        v = _clean(r.get(col))
        if v is not None:
            return v
    return None


def _iso_date(v) -> Optional[str]:
    """
    Best-effort ISO YYYY-MM-DD from common tracker date formats, stdlib only.
    Handles ISO datetimes, US M/D/YYYY, YYYY-MM-DD, and Unix timestamps.
    """
    v = _clean(v)
    if v is None:
        return None
    s = str(v).strip()

    # ISO datetime / date prefix
    if len(s) >= 10 and s[4] == "-" and s[7] == "-":
        return s[:10]

    # Unix timestamp (seconds or millis)
    if s.isdigit() and len(s) >= 10:
        from datetime import datetime, timezone
        ts = int(s[:13]) if len(s) > 10 else int(s)
        try:
            return datetime.fromtimestamp(ts, tz=timezone.utc).strftime("%Y-%m-%d")
        except (ValueError, OverflowError, OSError):
            return None

    # M/D/YYYY or D/M/YYYY (assume US order, as Letterboxd/Goodreads exports use it)
    parts = s.replace("-", "/").split("/")
    if len(parts) == 3:
        try:
            from datetime import date
            y, m, d = int(parts[2]), int(parts[0]), int(parts[1])
            return date(y, m, d).strftime("%Y-%m-%d")
        except ValueError:
            return None

    # Last resort: let datetime.parse try ISO-8601 variants
    from datetime import datetime
    for fmt in ("%Y-%m-%dT%H:%M:%S", "%Y/%m/%d", "%d %b %Y", "%b %d, %Y"):
        try:
            return datetime.strptime(s, fmt).strftime("%Y-%m-%d")
        except ValueError:
            continue
    return None


def normalize_row(source_key: str, r: dict) -> Optional[dict]:
    """Maps one raw row dict onto an ierp ingest record. Returns None to skip."""
    media_type, src = SOURCE_MAP.get(source_key, (source_key, source_key))
    title = _first(r, FIELD_CANDIDATES["title"])
    if not title or not str(title).strip():
        return None
    title = str(title).strip()

    review = _first(r, FIELD_CANDIDATES["review"])
    year_raw = _first(r, FIELD_CANDIDATES["year"])
    try:
        year = int(str(year_raw)[:4]) if year_raw else None
    except (ValueError, TypeError):
        year = None

    return {
        "media_type": media_type,
        "title": title,
        "original_title": _first(r, FIELD_CANDIDATES["original_title"]),
        "year": year,
        "author": _first(r, FIELD_CANDIDATES["author"]),
        "source": src,
        "extra": {k: v for k, v in r.items()
                  if _clean(v) is not None and k not in _EXTRA_EXCLUDED},
        "status": _first(r, FIELD_CANDIDATES["status"]),
        "rating": _first(r, FIELD_CANDIDATES["rating"]),
        "progress": _first(r, FIELD_CANDIDATES["progress"]),
        "started_at": _iso_date(_first(r, DATE_CANDIDATES["started_at"])),
        "finished_at": _iso_date(_first(r, DATE_CANDIDATES["finished_at"])),
        "date_logged": _iso_date(_first(r, DATE_CANDIDATES["date_logged"])),
        "review": str(review).strip() or None if review else None,
        "raw": {k: (str(v) if v is not None else None) for k, v in r.items()},
    }


def normalize_title(title: str) -> str:
    """Aggressive normalization for cross-source title matching (alphanumeric lowercase)."""
    return re.sub(r"[^a-z0-9]", "", (title or "").lower())


def normalize_rows(source_key: str, rows: list, existing_titles: Optional[set] = None) -> list:
    """
    Maps a list of raw row dicts onto ierp ingest records.
    If SOURCE_PREFERENCE defines higher-priority sources for this media_type and
    `existing_titles` (normalized titles already stored from those sources) is
    given, rows duplicating them are skipped — the preferred source wins.
    """
    media_type, src = SOURCE_MAP.get(source_key, (source_key, source_key))
    preferred = SOURCE_PREFERENCE.get(media_type, ())
    shadowed = existing_titles if (existing_titles is not None and src in preferred) else None
    higher = set(preferred[:preferred.index(src)]) if src in preferred else set()

    records = []
    for r in rows:
        if not isinstance(r, dict):
            continue
        rec = normalize_row(source_key, r)
        if not rec:
            continue
        if shadowed is not None and higher:
            # skip if any higher-priority source already has this title
            if any(normalize_title(rec["title"]) in s for s in [shadowed]):
                continue
        records.append(rec)
    return records
