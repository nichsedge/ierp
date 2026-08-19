"""
Importers for Notion CSV/Markdown directories and Google Maps Timeline Semantic Location History.
"""

import csv
import json
import os
import re
import shutil
import sqlite3
import uuid
from datetime import datetime
from pathlib import Path
from typing import Optional, Tuple, List, Dict, Any

from .config import MEDIA_DIR, MONTH_MAP, C_BOLD, C_GREEN, C_YELLOW, C_RED, C_RESET
from .db import get_db, init_db
from .geocoding import reverse_geocode
from .linking import link_events_and_contacts


def parse_date_to_iso(date_str: Optional[str]) -> Tuple[Optional[str], Optional[str]]:
    """Converts natural/Notion date strings into ISO 8601 YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS."""
    if not date_str:
        return None, None
    clean = date_str.strip()
    if not clean or clean.lower() == "none":
        return None, None

    if "->" in clean or "→" in clean:
        parts = re.split(r"\s*(?:->|→)\s*", clean)
        start_raw = parts[0].strip()
        end_raw = parts[1].strip() if len(parts) > 1 else None
        return _normalize_single_date(start_raw), _normalize_single_date(end_raw)
    else:
        return _normalize_single_date(clean), None


def _normalize_single_date(s: Optional[str]) -> Optional[str]:
    if not s:
        return None
    s = s.strip()

    if re.match(r"^\d{4}-\d{2}-\d{2}(?:[T\s]\d{2}:\d{2}(?::\d{2})?)?", s):
        return s.replace(" ", "T") if " " in s and len(s) > 10 else s

    m = re.match(r"^([A-Za-z]+)\s+(\d{1,2}),?\s*(\d{4})(?:\s+(\d{1,2}:\d{2}(?::\d{2})?))?", s)
    if m:
        month_name = m.group(1).lower()
        month_num = MONTH_MAP.get(month_name)
        if month_num:
            day = int(m.group(2))
            year = m.group(3)
            time_part = m.group(4)
            if time_part:
                return f"{year}-{month_num}-{day:02d}T{time_part}"
            return f"{year}-{month_num}-{day:02d}"

    m2 = re.match(r"^(\d{1,2})\s+([A-Za-z]+)\s+(\d{4})(?:\s+(\d{1,2}:\d{2}(?::\d{2})?))?", s)
    if m2:
        day = int(m2.group(1))
        month_name = m2.group(2).lower()
        month_num = MONTH_MAP.get(month_name)
        if month_num:
            year = m2.group(3)
            time_part = m2.group(4)
            if time_part:
                return f"{year}-{month_num}-{day:02d}T{time_part}"
            return f"{year}-{month_num}-{day:02d}"

    return s


def import_notion_export(export_dir: str) -> None:
    """Parses Notion export CSVs and markdown files into SQLite."""
    init_db()
    dir_path = Path(export_dir)
    if not dir_path.exists():
        print(f"{C_RED}Export directory not found: {export_dir}{C_RESET}")
        return

    csv_files = list(dir_path.glob("*.csv"))
    if not csv_files:
        print(f"{C_RED}No CSV index found in Notion export directory.{C_RESET}")
        return

    csv_path = csv_files[0]
    conn = get_db()
    cursor = conn.cursor()

    count = 0
    with open(csv_path, "r", encoding="utf-8-sig") as f:
        reader = csv.DictReader(f)
        for row in reader:
            title = row.get("Name") or row.get("Title") or "Untitled Event"
            place = row.get("Place") or row.get("Location") or None
            raw_date = row.get("Date") or None
            start_date, end_date = parse_date_to_iso(raw_date)

            tags_raw = row.get("Tags") or row.get("Tag") or ""
            tags = [t.strip() for t in tags_raw.split(",") if t.strip()] if tags_raw else []
            url = row.get("URL") or None

            md_files = list(dir_path.glob(f"{title[:20]}*.md"))
            notes = ""
            if md_files:
                try:
                    with open(md_files[0], "r", encoding="utf-8") as mdf:
                        notes = mdf.read()
                except Exception:
                    pass

            cursor.execute("""
            INSERT INTO events (title, place, start_date, end_date, raw_date, tags, url, notes)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """, (title, place, start_date, end_date, raw_date, json.dumps(tags), url, notes))
            count += 1

    conn.commit()
    print(f"{C_GREEN}Successfully imported {count} events from Notion.{C_RESET}")
    link_events_and_contacts(conn)
    conn.close()


def import_crm_contacts(crm_dir: str) -> None:
    """Imports Notion CRM contacts CSV into SQLite."""
    init_db()
    dir_path = Path(crm_dir)
    if not dir_path.exists():
        print(f"{C_RED}CRM directory not found: {crm_dir}{C_RESET}")
        return

    csv_files = list(dir_path.glob("*.csv"))
    if not csv_files:
        print(f"{C_RED}No CSV found in CRM directory.{C_RESET}")
        return

    conn = get_db()
    cursor = conn.cursor()
    count = 0

    with open(csv_files[0], "r", encoding="utf-8-sig") as f:
        reader = csv.DictReader(f)
        for row in reader:
            name = row.get("Name") or row.get("Person")
            if not name:
                continue
            name = name.strip().title()
            client = row.get("Client")
            raw_date = row.get("Date")
            date_iso, _ = parse_date_to_iso(raw_date)
            location = row.get("Location")
            org = row.get("Org")
            notes = row.get("Notes") or ""

            cursor.execute("""
            INSERT INTO contacts (name, client, date, location, org, notes, source)
            VALUES (?, ?, ?, ?, ?, ?, 'manual')
            """, (name.strip(), client, date_iso, location, org, notes))
            count += 1

    conn.commit()
    print(f"{C_GREEN}Successfully imported {count} CRM contacts.{C_RESET}")
    link_events_and_contacts(conn)
    conn.close()


def import_timeline(json_file_path: str) -> Dict[str, Any]:
    """Imports Google Maps Semantic Location History / Timeline JSON exports into SQLite."""
    init_db()
    file_path = Path(json_file_path)
    if not file_path.exists():
        print(f"{C_RED}Timeline file not found: {file_path}{C_RESET}")
        return {"status": "error", "message": "File not found"}

    try:
        with open(file_path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except Exception as e:
        print(f"{C_RED}Failed to parse timeline JSON: {e}{C_RESET}")
        return {"status": "error", "message": str(e)}

    conn = get_db()
    cursor = conn.cursor()

    items = data.get("timelineObjects") or data.get("semanticSegments") or (data if isinstance(data, list) else [])
    imported_count = 0

    for item in items:
        place_visit = item.get("placeVisit")
        activity_segment = item.get("activitySegment")

        if place_visit:
            loc = place_visit.get("location", {})
            place_name = loc.get("name") or loc.get("address")
            lat_e7 = loc.get("latitudeE7")
            lng_e7 = loc.get("longitudeE7")

            if not place_name and lat_e7 and lng_e7:
                place_name = reverse_geocode(lat_e7 / 1e7, lng_e7 / 1e7)

            duration = place_visit.get("duration", {})
            start_ts = duration.get("startTimestamp")
            end_ts = duration.get("endTimestamp")

            title = f"Visit: {place_name or 'Location'}"
            notes = f"Google Maps Timeline Place Visit\nConfidence: {place_visit.get('confidence', 'Unknown')}"

            cursor.execute("""
            INSERT INTO events (title, place, start_date, end_date, raw_date, tags, notes)
            VALUES (?, ?, ?, ?, ?, ?, ?)
            """, (title, place_name, start_ts, end_ts, start_ts, json.dumps(["timeline", "maps", "visit"]), notes))
            imported_count += 1

        elif activity_segment:
            act_type = activity_segment.get("activityType", "Travel")
            duration = activity_segment.get("duration", {})
            start_ts = duration.get("startTimestamp")
            end_ts = duration.get("endTimestamp")
            dist_meters = activity_segment.get("distance", 0)

            title = f"Activity: {act_type.replace('_', ' ').title()}"
            notes = f"Distance: {round(dist_meters/1000, 2)} km" if dist_meters else ""

            cursor.execute("""
            INSERT INTO events (title, place, start_date, end_date, raw_date, tags, notes)
            VALUES (?, ?, ?, ?, ?, ?, ?)
            """, (title, None, start_ts, end_ts, start_ts, json.dumps(["timeline", "maps", "activity", act_type.lower()]), notes))
            imported_count += 1

    conn.commit()
    print(f"\n{C_GREEN}Google Maps Timeline import complete!{C_RESET}")
    print(f"  Total Timeline Objects Imported: {C_GREEN}{imported_count}{C_RESET}\n")

    link_events_and_contacts(conn)
    conn.close()
    return {"status": "success", "imported_count": imported_count}
