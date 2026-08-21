"""
Media sync orchestrator: fetch from trackers -> normalize -> ingest into ierp.
One call per source; used by the `sync` CLI command.
"""

from typing import Optional
from pathlib import Path

from . import fetchers
from .ingest import ingest_rows
from .sources import SOURCE_MAP

# get-data source key -> (fetcher, kwargs)
FETCHERS = {
    "hardcover": (fetchers.fetch_hardcover, ("username",)),
    "goodreads": (fetchers.fetch_goodreads, ("user_id",)),
    "letterboxd": (fetchers.fetch_letterboxd, ("username",)),
    "anilist_anime": (fetchers.fetch_anilist, ("username",)),
    "anilist_manga": (fetchers.fetch_anilist, ("username",)),
    "mydramalist": (fetchers.fetch_mydramalist, ("username",)),
}


def sync_source(source_key: str, profiles: dict, db_path: Optional[Path] = None) -> dict:
    """
    Fetches and ingests one source. Returns
    {"source": key, "ok": bool, "rows": n, "error": str|None}.
    """
    if source_key not in FETCHERS:
        return {"source": source_key, "ok": False, "rows": 0,
                "error": f"unknown source (known: {sorted(FETCHERS)})"}

    fn, arg_names = FETCHERS[source_key]
    profile = profiles.get(source_key, {})
    kwargs = {name: profile[name] for name in arg_names if name in profile}

    # anilist fetcher needs media_type
    if source_key.startswith("anilist"):
        kwargs["media_type"] = "ANIME" if source_key == "anilist_anime" else "MANGA"

    try:
        rows = fn(**kwargs)
    except Exception as e:
        return {"source": source_key, "ok": False, "rows": 0, "error": str(e)}

    result = ingest_rows(source_key, rows, db_path)
    return {"source": source_key, "ok": True, "rows": result["items"], "error": None}


def sync_all(profiles: dict, sources: Optional[list] = None,
             db_path: Optional[Path] = None) -> list:
    """Syncs all (or selected) sources; one source failing doesn't stop the rest."""
    keys = sources or list(FETCHERS)
    return [sync_source(k, profiles, db_path) for k in keys]
