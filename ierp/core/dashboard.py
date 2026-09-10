"""
Zero-dependency web dashboard server and real-time OwnTracks GPS webhook receiver.
Features an interactive dark-mode dashboard with statistics, charts, search, contact management,
media consumption, vendors with favorite toggling, links, and commerce payment & referral tracking.
"""

import http.server
import json
import os
import socket
import urllib.parse
import webbrowser
from datetime import datetime
from typing import Dict, Any, List, Tuple, Optional

from .config import DB_PATH, MEDIA_DIR, TEMPLATES_DIR, C_BOLD, C_GREEN, C_CYAN, C_MAGENTA, C_YELLOW, C_RESET
from .db import get_db, init_db
from .decisions import list_decisions
from .finance import compute_runway, list_commitments, list_snapshots
from .geocoding import reverse_geocode
from .google_sync import sync_google_contacts
from .lifeops import get_maintenance_summary, list_maintenance
from .merging import auto_merge_contacts, merge_two_contacts
from .projects import list_projects
from .radar import compute_radar, get_radar_summary
from .reviews import list_retrospectives

def load_dashboard_html() -> str:
    """Loads the dashboard HTML template from the templates directory."""
    template_path = TEMPLATES_DIR / "dashboard.html"
    if template_path.exists():
        return template_path.read_text(encoding="utf-8")
    return "<!DOCTYPE html><html><body><h1>iERP Dashboard Template Not Found</h1></body></html>"


DASHBOARD_HTML = load_dashboard_html()


def ingest_location_ping(data: Dict[str, Any]) -> Dict[str, Any]:
    """Ingests live OwnTracks background GPS HTTP webhook pings."""
    init_db()
    msg_type = data.get("_type", "location")
    lat = data.get("lat")
    lon = data.get("lon")
    tst = data.get("tst")

    if lat is None or lon is None:
        return {"status": "ignored", "reason": "No coordinates provided"}

    date_iso = datetime.fromtimestamp(tst).strftime("%Y-%m-%d %H:%M:%S") if tst else datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    place_name = reverse_geocode(float(lat), float(lon))

    conn = get_db()
    cursor = conn.cursor()

    if msg_type == "transition":
        event_action = data.get("event", "enter")
        desc = data.get("desc") or "Geofence Region"
        title = f"Geofence {event_action.capitalize()}: {desc}"
        tags = ["location", "geofence", "owntracks", event_action]
        notes = f"OwnTracks Geofence Transition\nLat: {lat}, Lon: {lon}\nPlace: {place_name}"
    else:
        title = f"Location Ping: {place_name or 'Unknown Place'}"
        tags = ["location", "gps", "owntracks"]
        notes = f"Background GPS Ping\nAccuracy: {data.get('acc', 'N/A')}m, Battery: {data.get('batt', 'N/A')}%, Speed: {data.get('vel', 'N/A')}km/h"

    # Avoid duplicate pings within 10 minutes at identical location
    recent = cursor.execute("""
    SELECT id FROM events 
    WHERE place = ? AND start_date >= datetime(?, '-10 minutes')
    LIMIT 1
    """, (place_name, date_iso)).fetchone()

    if recent:
        conn.close()
        return {"status": "deduplicated", "event_id": recent[0], "place": place_name}

    cursor.execute("""
    INSERT INTO events (title, place, start_date, raw_date, tags, notes)
    VALUES (?, ?, ?, ?, ?, ?)
    """, (title, place_name, date_iso, date_iso, json.dumps(tags), notes))
    ev_id = cursor.lastrowid
    conn.commit()

    conn.close()
    return {"status": "success", "event_id": ev_id, "place": place_name, "date": date_iso}


def parse_pagination(query: Dict[str, List[str]], default_limit: int = 25, max_limit: int = 500) -> Tuple[int, int]:
    """Safely extracts limit and offset from query parameters."""
    try:
        limit = int(query.get("limit", [default_limit])[0])
        limit = max(1, min(limit, max_limit))
    except (ValueError, TypeError):
        limit = default_limit

    try:
        offset = int(query.get("offset", [0])[0])
        offset = max(0, offset)
    except (ValueError, TypeError):
        offset = 0

    return limit, offset


def parse_sort(query: Dict[str, List[str]], allowed_cols: Dict[str, str], default_col: str, default_dir: str = "DESC") -> Tuple[str, str]:
    """Whitelists and validates SQL column names and sort direction to prevent SQL injection."""
    raw_col = query.get("sort", [""])[0].strip().lower()
    raw_dir = query.get("dir", [query.get("order", [""])[0]])[0].strip().lower()

    sql_col = allowed_cols.get(raw_col, allowed_cols.get(default_col, default_col))
    sql_dir = "ASC" if raw_dir == "asc" else ("DESC" if raw_dir == "desc" else default_dir.upper())

    return sql_col, sql_dir


# Whitelist Mappings for Safe Dynamic ORDER BY
EVENT_SORT_COLS = {
    "id": "e.id",
    "title": "e.title",
    "place": "e.place",
    "start_date": "e.start_date",
    "end_date": "e.end_date",
    "created_at": "e.created_at",
}

CONTACT_SORT_COLS = {
    "id": "c.id",
    "name": "c.name",
    "org": "c.org",
    "client": "c.client",
    "location": "c.location",
    "email": "c.email",
    "phone": "c.phone",
    "source": "c.source",
    "date": "c.date",
    "created_at": "c.created_at",
    "event_count": "cnt",
    "cnt": "cnt",
}

MEDIA_SORT_COLS = {
    "id": "m.id",
    "media_type": "m.media_type",
    "type": "m.media_type",
    "title": "m.title",
    "source": "m.source",
    "status": "status",
    "rating": "rating",
    "date": "date_val",
    "date_logged": "date_val",
    "created_at": "m.created_at",
}

VENDOR_SORT_COLS = {
    "id": "v.id",
    "name": "v.name",
    "category": "v.category",
    "location": "v.location",
    "favorite": "v.favorite",
    "source": "v.source",
    "created_at": "v.created_at",
}

LINK_SORT_COLS = {
    "id": "l.id",
    "label": "l.label",
    "url": "l.url",
    "category": "l.category",
    "is_public": "l.is_public",
    "created_at": "l.created_at",
}

PAYMENT_SORT_COLS = {
    "id": "p.id",
    "name": "p.name",
    "slug": "p.slug",
    "category": "p.category",
    "number": "p.number",
    "recipient": "p.recipient",
    "created_at": "p.created_at",
}

REFERRAL_SORT_COLS = {
    "id": "r.id",
    "name": "r.name",
    "slug": "r.slug",
    "category": "r.category",
    "code": "r.code",
    "status": "r.status",
    "is_public": "r.is_public",
    "created_at": "r.created_at",
}

RECEIPT_SORT_COLS = {
    "id": "r.id",
    "event_id": "r.event_id",
    "amount": "r.amount",
    "type": "r.type",
    "status": "r.status",
    "created_at": "r.created_at",
    "updated_at": "r.updated_at",
}


class DashboardRequestHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        pass

    def send_json(self, data, status=200):
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization")
        self.end_headers()
        self.wfile.write(json.dumps(data, ensure_ascii=False, default=str).encode("utf-8"))

    def do_OPTIONS(self):
        self.send_response(200)
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization")
        self.end_headers()

    def do_POST(self):
        parsed_url = urllib.parse.urlparse(self.path)
        path = parsed_url.path

        content_length = int(self.headers.get("Content-Length", 0))
        raw_body = self.rfile.read(content_length).decode("utf-8") if content_length > 0 else "{}"

        try:
            body_data = json.loads(raw_body)
        except Exception:
            body_data = {}

        if path in ("/api/webhook/location", "/api/webhook/owntracks", "/api/webhook"):
            if isinstance(body_data, list):
                results = [ingest_location_ping(item) for item in body_data]
                self.send_json(results)
            else:
                res = ingest_location_ping(body_data)
                self.send_json([res])

        elif path == "/api/sync/contacts":
            full_resync = body_data.get("full", False)
            res = sync_google_contacts(full_resync=full_resync)
            self.send_json(res)

        elif path == "/api/contacts/merge":
            if body_data.get("auto", False):
                res = auto_merge_contacts()
                self.send_json(res)
            elif "source_id" in body_data and "target_id" in body_data:
                res = merge_two_contacts(int(body_data["source_id"]), int(body_data["target_id"]))
                self.send_json(res)
            else:
                res = auto_merge_contacts()
                self.send_json(res)

        elif path in ("/api/vendors/favorite", "/api/vendors/toggle-favorite"):
            vendor_id = int(body_data.get("id", 0))
            if not vendor_id:
                self.send_error(400, "Missing vendor ID")
                return

            conn = get_db()
            cursor = conn.cursor()

            if "favorite" in body_data:
                fav_val = 1 if body_data["favorite"] else 0
                cursor.execute("UPDATE vendors SET favorite = ? WHERE id = ?", (fav_val, vendor_id))
            else:
                cursor.execute("UPDATE vendors SET favorite = CASE WHEN favorite = 1 THEN 0 ELSE 1 END WHERE id = ?", (vendor_id,))
            conn.commit()

            row = cursor.execute("SELECT favorite FROM vendors WHERE id = ?", (vendor_id,)).fetchone()
            conn.close()

            if row:
                self.send_json({"status": "success", "id": vendor_id, "favorite": bool(row[0])})
            else:
                self.send_error(404, "Vendor not found")

        else:
            self.send_error(404, "Endpoint not found")

    def do_GET(self):
        parsed_url = urllib.parse.urlparse(self.path)
        path = parsed_url.path
        query = urllib.parse.parse_qs(parsed_url.query)

        if path == "/":
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.end_headers()
            self.wfile.write(DASHBOARD_HTML.encode("utf-8"))

        elif path == "/api/stats":
            conn = get_db()
            cursor = conn.cursor()
            total_events = cursor.execute("SELECT COUNT(*) FROM events").fetchone()[0]
            total_contacts = cursor.execute("SELECT COUNT(*) FROM contacts").fetchone()[0]
            total_media = cursor.execute("SELECT COUNT(*) FROM event_media").fetchone()[0]
            total_links = cursor.execute("SELECT COUNT(*) FROM event_contacts").fetchone()[0]

            tags_raw = cursor.execute("SELECT tags FROM events WHERE tags IS NOT NULL").fetchall()
            all_tags = {}
            for (t_json,) in tags_raw:
                try:
                    tags = json.loads(t_json) if t_json else []
                    for tag in tags:
                        all_tags[tag] = all_tags.get(tag, 0) + 1
                except Exception:
                    pass
            top_tags = sorted(all_tags.items(), key=lambda x: x[1], reverse=True)[:15]

            months_raw = cursor.execute("""
                SELECT strftime('%Y-%m', start_date) as ym, COUNT(*) 
                FROM events 
                WHERE start_date IS NOT NULL AND start_date != ''
                GROUP BY ym ORDER BY ym DESC LIMIT 12
            """).fetchall()
            monthly_counts = [{"year_month": ym, "count": cnt} for ym, cnt in months_raw if ym]

            daily_raw = cursor.execute("""
                SELECT substr(start_date, 1, 10) as dt, COUNT(*)
                FROM events
                WHERE start_date IS NOT NULL AND start_date != '' AND start_date >= date('now', '-365 days')
                GROUP BY dt ORDER BY dt ASC
            """).fetchall()
            daily_counts = {dt: cnt for dt, cnt in daily_raw if dt}

            places_raw = cursor.execute("""
                SELECT place, COUNT(*)
                FROM events
                WHERE place IS NOT NULL AND place != ''
                GROUP BY place ORDER BY COUNT(*) DESC LIMIT 12
            """).fetchall()
            top_places = [[p, cnt] for p, cnt in places_raw if p]

            media_raw = cursor.execute("""
                SELECT media_type, COUNT(*)
                FROM media_items
                WHERE media_type IS NOT NULL AND media_type != ''
                GROUP BY media_type ORDER BY COUNT(*) DESC
            """).fetchall()
            media_summary = {mt: cnt for mt, cnt in media_raw if mt}

            # Monthly cashflow
            cf_raw = cursor.execute("""
                SELECT strftime('%Y-%m', created_at) as ym, type, SUM(amount)
                FROM receipts
                WHERE created_at IS NOT NULL
                GROUP BY ym, type ORDER BY ym DESC LIMIT 12
            """).fetchall()
            cashflow_map: dict[str, dict[str, float]] = {}
            for ym, rtype, total_amt in cf_raw:
                if not ym:
                    continue
                if ym not in cashflow_map:
                    cashflow_map[ym] = {"year_month": ym, "income": 0.0, "cost": 0.0, "expected": 0.0}
                if rtype in ("income", "cost", "expected"):
                    cashflow_map[ym][rtype] = float(total_amt or 0.0)
            cashflow = list(cashflow_map.values())

            conn.close()

            res = {
                "total_events": total_events,
                "total_contacts": total_contacts,
                "total_media": total_media,
                "total_links": total_links,
                "top_tags": top_tags,
                "monthly_counts": monthly_counts,
                "daily_counts": daily_counts,
                "top_places": top_places,
                "media_summary": media_summary,
                "cashflow": cashflow,
            }
            # Add financial summary
            from ierp.core.receipts import compute_balance
            try:
                bal = compute_balance()
                res["finance"] = bal
            except Exception:
                pass
            self.send_json(res)

        elif path == "/api/events":
            q = query.get("q", [""])[0].strip()
            tag_filter = query.get("tag", [""])[0].strip()
            from_date = query.get("from", query.get("from_date", query.get("date_from", [""])))[0].strip()
            to_date = query.get("to", query.get("to_date", query.get("date_to", [""])))[0].strip()

            limit, offset = parse_pagination(query, default_limit=25)
            sort_col, sort_dir = parse_sort(query, EVENT_SORT_COLS, default_col="start_date", default_dir="DESC")

            conn = get_db()
            cursor = conn.cursor()

            where_clauses = ["1=1"]
            params = []

            if q:
                where_clauses.append("(e.title LIKE ? OR e.place LIKE ? OR e.notes LIKE ?)")
                like_q = f"%{q}%"
                params.extend([like_q, like_q, like_q])

            if tag_filter:
                where_clauses.append("e.tags LIKE ?")
                params.append(f"%{tag_filter}%")

            if from_date:
                where_clauses.append("(e.start_date >= ? OR (e.start_date IS NULL AND e.raw_date >= ?))")
                params.extend([from_date, from_date])

            if to_date:
                to_bound = to_date + " 23:59:59" if len(to_date) == 10 else to_date
                where_clauses.append("(e.start_date <= ? OR (e.start_date IS NULL AND e.raw_date <= ?))")
                params.extend([to_bound, to_bound])

            where_sql = " AND ".join(where_clauses)

            total = cursor.execute(f"SELECT COUNT(*) FROM events e WHERE {where_sql}", params).fetchone()[0]

            fetch_sql = f"""
                SELECT e.id, e.title, e.place, e.start_date, e.end_date, e.raw_date, e.tags, e.notes, e.created_at
                FROM events e
                WHERE {where_sql}
                ORDER BY {sort_col} {sort_dir}, e.id DESC
                LIMIT ? OFFSET ?
            """
            rows = cursor.execute(fetch_sql, [*params, limit, offset]).fetchall()

            events = []
            for r in rows:
                ev_id, title, place, start, end, raw_d, tags_json, notes, created_at = r
                try:
                    tags = json.loads(tags_json) if tags_json else []
                except Exception:
                    tags = []

                linked_contacts = cursor.execute("""
                    SELECT c.id, c.name FROM contacts c
                    JOIN event_contacts ec ON c.id = ec.contact_id
                    WHERE ec.event_id = ?
                """, (ev_id,)).fetchall()

                events.append({
                    "id": ev_id,
                    "title": title,
                    "place": place,
                    "start_date": start,
                    "end_date": end,
                    "raw_date": raw_d,
                    "tags": tags,
                    "notes": notes,
                    "created_at": created_at,
                    "linked_contacts": [{"id": cid, "name": cname} for cid, cname in linked_contacts]
                })

            conn.close()

            res = {
                "items": events,
                "total": total,
                "limit": limit,
                "offset": offset
            }
            self.send_json(res)

        elif path.startswith("/api/events/"):
            try:
                ev_id = int(path.split("/")[-1])
            except ValueError:
                self.send_error(400, "Invalid event ID")
                return

            conn = get_db()
            cursor = conn.cursor()

            row = cursor.execute("""
                SELECT id, title, place, start_date, end_date, raw_date, tags, url, notes, created_at FROM events WHERE id = ?
            """, (ev_id,)).fetchone()

            if not row:
                conn.close()
                self.send_error(404, "Event not found")
                return

            eid, title, place, start, end, raw_d, tags_json, url, notes, created_at = row
            try:
                tags = json.loads(tags_json) if tags_json else []
            except Exception:
                tags = []

            contacts = cursor.execute("""
                SELECT c.id, c.name, c.org FROM contacts c
                JOIN event_contacts ec ON c.id = ec.contact_id
                WHERE ec.event_id = ?
            """, (eid,)).fetchall()

            media = cursor.execute("""
                SELECT id, original_filename, stored_path FROM event_media WHERE event_id = ?
            """, (eid,)).fetchall()

            conn.close()

            res = {
                "id": eid,
                "title": title,
                "place": place,
                "start_date": start,
                "end_date": end,
                "raw_date": raw_d,
                "tags": tags,
                "url": url,
                "notes": notes,
                "created_at": created_at,
                "contacts": [{"id": c[0], "name": c[1], "org": c[2]} for c in contacts],
                "media": [{"id": m[0], "original_filename": m[1], "stored_path": m[2]} for m in media]
            }
            self.send_json(res)

        elif path == "/api/contacts":
            source_filter = query.get("source", [""])[0].strip().lower()
            q = query.get("q", [""])[0].strip()

            limit, offset = parse_pagination(query, default_limit=24)
            sort_col, sort_dir = parse_sort(query, CONTACT_SORT_COLS, default_col="event_count", default_dir="DESC")

            conn = get_db()
            cursor = conn.cursor()

            where_clauses = ["1=1"]
            params = []

            if source_filter and source_filter != "all":
                where_clauses.append("LOWER(c.source) = ?")
                params.append(source_filter)

            if q:
                where_clauses.append("(c.name LIKE ? OR c.org LIKE ? OR c.client LIKE ? OR c.email LIKE ? OR c.phone LIKE ? OR c.notes LIKE ? OR c.location LIKE ?)")
                like_q = f"%{q}%"
                params.extend([like_q, like_q, like_q, like_q, like_q, like_q, like_q])

            where_sql = " AND ".join(where_clauses)

            total = cursor.execute(f"SELECT COUNT(*) FROM contacts c WHERE {where_sql}", params).fetchone()[0]

            fetch_sql = f"""
                SELECT c.id, c.name, c.org, c.client, c.location, c.notes, c.email, c.phone, c.source, c.google_id, c.created_at,
                       (SELECT COUNT(*) FROM event_contacts ec WHERE ec.contact_id = c.id) as cnt
                FROM contacts c
                WHERE {where_sql}
                ORDER BY {sort_col} {sort_dir}, c.name ASC
                LIMIT ? OFFSET ?
            """
            rows = cursor.execute(fetch_sql, [*params, limit, offset]).fetchall()
            conn.close()

            contacts = [{
                "id": r[0],
                "name": r[1],
                "org": r[2],
                "client": r[3],
                "location": r[4],
                "notes": r[5],
                "email": r[6],
                "phone": r[7],
                "source": r[8] or ("google" if r[9] else "manual"),
                "is_google_linked": bool(r[9]),
                "created_at": r[10],
                "event_count": r[11]
            } for r in rows]

            res = {
                "items": contacts,
                "total": total,
                "limit": limit,
                "offset": offset
            }
            self.send_json(res)

        elif path == "/api/media":
            media_type_filter = query.get("type", query.get("media_type", [""]))[0].strip().lower()
            q = query.get("q", [""])[0].strip()
            source_filter = query.get("source", [""])[0].strip()
            status_filter = query.get("status", [""])[0].strip().lower()

            limit, offset = parse_pagination(query, default_limit=24)
            sort_col, sort_dir = parse_sort(query, MEDIA_SORT_COLS, default_col="id", default_dir="DESC")

            conn = get_db()
            cursor = conn.cursor()

            where_clauses = ["1=1"]
            params = []

            if media_type_filter and media_type_filter != "all":
                where_clauses.append("LOWER(m.media_type) = ?")
                params.append(media_type_filter)

            if source_filter:
                where_clauses.append("LOWER(m.source) = ?")
                params.append(source_filter.lower())

            if q:
                where_clauses.append("(m.title LIKE ? OR m.source LIKE ? OR m.data_json LIKE ? OR l.review LIKE ?)")
                like_q = f"%{q}%"
                params.extend([like_q, like_q, like_q, like_q])

            if status_filter:
                where_clauses.append("(LOWER(l.status) LIKE ? OR LOWER(json_extract(m.data_json, '$.status')) LIKE ? OR LOWER(json_extract(m.data_json, '$.my_status')) LIKE ? OR LOWER(json_extract(m.data_json, '$.Bookshelves')) LIKE ?)")
                like_st = f"%{status_filter}%"
                params.extend([like_st, like_st, like_st, like_st])

            where_sql = " AND ".join(where_clauses)

            count_sql = f"""
                SELECT COUNT(DISTINCT m.id)
                FROM media_items m
                LEFT JOIN media_logs l ON m.id = l.media_item_id
                WHERE {where_sql}
            """
            total = cursor.execute(count_sql, params).fetchone()[0]

            fetch_sql = f"""
                SELECT 
                    m.id, 
                    m.media_type, 
                    m.title, 
                    m.source, 
                    m.created_at,
                    m.updated_at,
                    COALESCE(l.status, json_extract(m.data_json, '$.status'), json_extract(m.data_json, '$.my_status'), json_extract(m.data_json, '$.Bookshelves')) as status,
                    COALESCE(l.rating, json_extract(m.data_json, '$.rating'), json_extract(m.data_json, '$.Rating'), json_extract(m.data_json, '$.Score'), json_extract(m.data_json, '$.my_score'), json_extract(m.data_json, '$.Average Rating')) as rating,
                    COALESCE(l.date_logged, l.finished_at, json_extract(m.data_json, '$.date_logged'), json_extract(m.data_json, '$.Date Added'), json_extract(m.data_json, '$.year'), m.created_at) as date_val,
                    COALESCE(json_extract(m.data_json, '$.author'), json_extract(m.data_json, '$.Author')) as author
                FROM media_items m
                LEFT JOIN media_logs l ON m.id = l.media_item_id
                WHERE {where_sql}
                GROUP BY m.id
                ORDER BY {sort_col} {sort_dir}, m.id DESC
                LIMIT ? OFFSET ?
            """
            rows = cursor.execute(fetch_sql, [*params, limit, offset]).fetchall()

            types_raw = cursor.execute("SELECT DISTINCT media_type FROM media_items WHERE media_type IS NOT NULL ORDER BY media_type").fetchall()
            types = [t[0] for t in types_raw if t[0]]

            conn.close()

            items = [{
                "id": r[0],
                "media_type": r[1],
                "title": r[2],
                "source": r[3],
                "created_at": r[4],
                "updated_at": r[5],
                "status": r[6],
                "rating": r[7],
                "date": r[8],
                "author": r[9]
            } for r in rows]

            res = {
                "items": items,
                "total": total,
                "limit": limit,
                "offset": offset,
                "types": types
            }
            self.send_json(res)

        elif path == "/api/vendors":
            q = query.get("q", [""])[0].strip()
            category_filter = query.get("category", [""])[0].strip()
            fav_filter = query.get("favorite", [""])[0].strip().lower()

            limit, offset = parse_pagination(query, default_limit=24)
            sort_col, sort_dir = parse_sort(query, VENDOR_SORT_COLS, default_col="favorite", default_dir="DESC")

            conn = get_db()
            cursor = conn.cursor()

            where_clauses = ["1=1"]
            params = []

            if q:
                where_clauses.append("(v.name LIKE ? OR v.category LIKE ? OR v.location LIKE ? OR v.notes LIKE ? OR v.phone LIKE ? OR v.email LIKE ?)")
                like_q = f"%{q}%"
                params.extend([like_q, like_q, like_q, like_q, like_q, like_q])

            if category_filter:
                where_clauses.append("v.category = ?")
                params.append(category_filter)

            if fav_filter in ("1", "true"):
                where_clauses.append("v.favorite = 1")
            elif fav_filter in ("0", "false"):
                where_clauses.append("v.favorite = 0")

            where_sql = " AND ".join(where_clauses)

            total = cursor.execute(f"SELECT COUNT(*) FROM vendors v WHERE {where_sql}", params).fetchone()[0]

            fetch_sql = f"""
                SELECT v.id, v.name, v.category, v.location, v.phone, v.email, v.url, v.notes, v.favorite, v.source, v.created_at
                FROM vendors v
                WHERE {where_sql}
                ORDER BY {sort_col} {sort_dir}, v.name ASC
                LIMIT ? OFFSET ?
            """
            rows = cursor.execute(fetch_sql, [*params, limit, offset]).fetchall()

            cats_raw = cursor.execute("SELECT DISTINCT category FROM vendors WHERE category IS NOT NULL AND category != '' ORDER BY category").fetchall()
            categories = [c[0] for c in cats_raw if c[0]]

            conn.close()

            vendors = [{
                "id": r[0],
                "name": r[1],
                "category": r[2],
                "location": r[3],
                "phone": r[4],
                "email": r[5],
                "url": r[6],
                "notes": r[7],
                "favorite": bool(r[8]),
                "source": r[9],
                "created_at": r[10]
            } for r in rows]

            res = {
                "items": vendors,
                "total": total,
                "limit": limit,
                "offset": offset,
                "categories": categories
            }
            self.send_json(res)

        elif path == "/api/links":
            q = query.get("q", [""])[0].strip()
            category_filter = query.get("category", [""])[0].strip()
            public_only = query.get("public_only", query.get("is_public", [""]))[0].strip()

            limit, offset = parse_pagination(query, default_limit=24)
            sort_col, sort_dir = parse_sort(query, LINK_SORT_COLS, default_col="category", default_dir="ASC")

            conn = get_db()
            cursor = conn.cursor()

            where_clauses = ["1=1"]
            params = []

            if q:
                where_clauses.append("(l.label LIKE ? OR l.url LIKE ? OR l.category LIKE ? OR l.notes LIKE ?)")
                like_q = f"%{q}%"
                params.extend([like_q, like_q, like_q, like_q])

            if category_filter:
                where_clauses.append("l.category = ?")
                params.append(category_filter)

            if public_only in ("1", "true"):
                where_clauses.append("l.is_public = 1")
            elif public_only in ("0", "false"):
                where_clauses.append("l.is_public = 0")

            where_sql = " AND ".join(where_clauses)

            total = cursor.execute(f"SELECT COUNT(*) FROM links l WHERE {where_sql}", params).fetchone()[0]

            fetch_sql = f"""
                SELECT l.id, l.label, l.url, l.category, l.is_public, l.notes, l.created_at
                FROM links l
                WHERE {where_sql}
                ORDER BY {sort_col} {sort_dir}, l.label ASC
                LIMIT ? OFFSET ?
            """
            rows = cursor.execute(fetch_sql, [*params, limit, offset]).fetchall()

            cats_raw = cursor.execute("SELECT DISTINCT category FROM links WHERE category IS NOT NULL AND category != '' ORDER BY category").fetchall()
            categories = [c[0] for c in cats_raw if c[0]]

            conn.close()

            links = [{
                "id": r[0],
                "label": r[1],
                "url": r[2],
                "category": r[3],
                "is_public": bool(r[4]),
                "notes": r[5],
                "created_at": r[6]
            } for r in rows]

            res = {
                "items": links,
                "total": total,
                "limit": limit,
                "offset": offset,
                "categories": categories
            }
            self.send_json(res)

        elif path in ("/api/payment-accounts", "/api/payments"):
            q = query.get("q", [""])[0].strip()
            category_filter = query.get("category", [""])[0].strip()

            limit, offset = parse_pagination(query, default_limit=24)
            sort_col, sort_dir = parse_sort(query, PAYMENT_SORT_COLS, default_col="category", default_dir="ASC")

            conn = get_db()
            cursor = conn.cursor()

            where_clauses = ["1=1"]
            params = []

            if q:
                where_clauses.append("(p.name LIKE ? OR p.category LIKE ? OR p.number LIKE ? OR p.recipient LIKE ? OR p.details LIKE ? OR p.slug LIKE ?)")
                like_q = f"%{q}%"
                params.extend([like_q, like_q, like_q, like_q, like_q, like_q])

            if category_filter:
                where_clauses.append("p.category = ?")
                params.append(category_filter)

            where_sql = " AND ".join(where_clauses)

            total = cursor.execute(f"SELECT COUNT(*) FROM payment_accounts p WHERE {where_sql}", params).fetchone()[0]

            fetch_sql = f"""
                SELECT p.id, p.slug, p.name, p.category, p.number, p.recipient, p.details, p.details_id, p.created_at, p.updated_at
                FROM payment_accounts p
                WHERE {where_sql}
                ORDER BY {sort_col} {sort_dir}, p.name ASC
                LIMIT ? OFFSET ?
            """
            rows = cursor.execute(fetch_sql, [*params, limit, offset]).fetchall()

            cats_raw = cursor.execute("SELECT DISTINCT category FROM payment_accounts WHERE category IS NOT NULL AND category != '' ORDER BY category").fetchall()
            categories = [c[0] for c in cats_raw if c[0]]

            conn.close()

            payments = [{
                "id": r[0],
                "slug": r[1],
                "name": r[2],
                "category": r[3],
                "number": r[4],
                "recipient": r[5],
                "details": r[6],
                "details_id": r[7],
                "created_at": r[8],
                "updated_at": r[9]
            } for r in rows]

            res = {
                "items": payments,
                "total": total,
                "limit": limit,
                "offset": offset,
                "categories": categories
            }
            self.send_json(res)

        elif path == "/api/referrals":
            q = query.get("q", [""])[0].strip()
            category_filter = query.get("category", [""])[0].strip()
            status_filter = query.get("status", [""])[0].strip().upper()
            public_only = query.get("public_only", query.get("is_public", [""]))[0].strip()

            limit, offset = parse_pagination(query, default_limit=24)
            sort_col, sort_dir = parse_sort(query, REFERRAL_SORT_COLS, default_col="category", default_dir="ASC")

            conn = get_db()
            cursor = conn.cursor()

            where_clauses = ["1=1"]
            params = []

            if q:
                where_clauses.append("(r.name LIKE ? OR r.category LIKE ? OR r.code LIKE ? OR r.link LIKE ? OR r.benefit LIKE ? OR r.slug LIKE ?)")
                like_q = f"%{q}%"
                params.extend([like_q, like_q, like_q, like_q, like_q, like_q])

            if category_filter:
                where_clauses.append("r.category = ?")
                params.append(category_filter)

            if status_filter:
                where_clauses.append("UPPER(r.status) = ?")
                params.append(status_filter)

            if public_only in ("1", "true"):
                where_clauses.append("r.is_public = 1")
            elif public_only in ("0", "false"):
                where_clauses.append("r.is_public = 0")

            where_sql = " AND ".join(where_clauses)

            total = cursor.execute(f"SELECT COUNT(*) FROM referrals r WHERE {where_sql}", params).fetchone()[0]

            fetch_sql = f"""
                SELECT r.id, r.slug, r.name, r.category, r.code, r.link, r.benefit, r.status, r.is_public, r.created_at, r.updated_at
                FROM referrals r
                WHERE {where_sql}
                ORDER BY {sort_col} {sort_dir}, r.name ASC
                LIMIT ? OFFSET ?
            """
            rows = cursor.execute(fetch_sql, [*params, limit, offset]).fetchall()

            cats_raw = cursor.execute("SELECT DISTINCT category FROM referrals WHERE category IS NOT NULL AND category != '' ORDER BY category").fetchall()
            categories = [c[0] for c in cats_raw if c[0]]

            conn.close()

            referrals = [{
                "id": r[0],
                "slug": r[1],
                "name": r[2],
                "category": r[3],
                "code": r[4],
                "link": r[5],
                "benefit": r[6],
                "status": r[7],
                "is_public": bool(r[8]),
                "created_at": r[9],
                "updated_at": r[10]
            } for r in rows]

            res = {
                "items": referrals,
                "total": total,
                "limit": limit,
                "offset": offset,
                "categories": categories
            }
            self.send_json(res)

        elif path == "/api/commerce":
            # Combined commerce endpoint
            limit, offset = parse_pagination(query, default_limit=24)
            conn = get_db()
            cursor = conn.cursor()

            p_rows = cursor.execute("""
                SELECT id, slug, name, category, number, recipient, details, details_id, created_at, updated_at
                FROM payment_accounts ORDER BY category, name
            """).fetchall()

            r_rows = cursor.execute("""
                SELECT id, slug, name, category, code, link, benefit, status, is_public, created_at, updated_at
                FROM referrals ORDER BY category, name
            """).fetchall()
            conn.close()

            payments = [{
                "id": r[0], "slug": r[1], "name": r[2], "category": r[3], "number": r[4],
                "recipient": r[5], "details": r[6], "details_id": r[7], "created_at": r[8], "updated_at": r[9]
            } for r in p_rows]

            referrals = [{
                "id": r[0], "slug": r[1], "name": r[2], "category": r[3], "code": r[4],
                "link": r[5], "benefit": r[6], "status": r[7], "is_public": bool(r[8]), "created_at": r[9], "updated_at": r[10]
            } for r in r_rows]

            self.send_json({
                "payment_accounts": {"items": payments, "total": len(payments)},
                "referrals": {"items": referrals, "total": len(referrals)}
            })

        elif path == "/api/receipts":
            type_filter = query.get("type", [""])[0].strip()
            status_filter = query.get("status", [""])[0].strip()
            event_filter = query.get("event_id", [""])[0].strip()
            q = query.get("q", [""])[0].strip()
            limit, offset = parse_pagination(query, default_limit=50)
            sort_col, sort_dir = parse_sort(query, RECEIPT_SORT_COLS, default_col="created_at", default_dir="DESC")

            conn = get_db()
            cursor = conn.cursor()
            where_clauses = ["1=1"]
            params = []

            if type_filter and type_filter != "all":
                where_clauses.append("r.type = ?")
                params.append(type_filter)
            if status_filter and status_filter != "all":
                where_clauses.append("r.status = ?")
                params.append(status_filter)
            if event_filter:
                try:
                    eid = int(event_filter)
                    where_clauses.append("r.event_id = ?")
                    params.append(eid)
                except ValueError:
                    pass
            if q:
                where_clauses.append("(e.title LIKE ? OR e.notes LIKE ? OR r.notes LIKE ?)")
                like_q = f"%{q}%"
                params.extend([like_q, like_q, like_q])

            where_sql = " AND ".join(where_clauses)
            total = cursor.execute(f"SELECT COUNT(*) FROM receipts r LEFT JOIN events e ON r.event_id = e.id WHERE {where_sql}", params).fetchone()[0]

            fetch_sql = f"""
                SELECT r.id, r.event_id, r.amount, r.type, r.status, r.notes, r.created_at, r.updated_at,
                       e.title, e.start_date, e.tags
                FROM receipts r
                LEFT JOIN events e ON r.event_id = e.id
                WHERE {where_sql}
                ORDER BY {sort_col} {sort_dir}, r.id DESC
                LIMIT ? OFFSET ?
            """
            rows = cursor.execute(fetch_sql, [*params, limit, offset]).fetchall()

            types_raw = cursor.execute("SELECT DISTINCT type FROM receipts WHERE type IS NOT NULL ORDER BY type").fetchall()
            types = [t[0] for t in types_raw if t[0]]

            statuses_raw = cursor.execute("SELECT DISTINCT status FROM receipts WHERE status IS NOT NULL ORDER BY status").fetchall()
            statuses = [s[0] for s in statuses_raw if s[0]]

            conn.close()

            items = [{
                "id": r[0],
                "event_id": r[1],
                "amount": r[2],
                "type": r[3],
                "status": r[4],
                "notes": r[5],
                "created_at": r[6],
                "updated_at": r[7],
                "event_title": r[8],
                "event_date": r[9],
                "event_tags": json.loads(r[10]) if r[10] else []
            } for r in rows]

            self.send_json({
                "items": items,
                "total": total,
                "limit": limit,
                "offset": offset,
                "types": types,
                "statuses": statuses
            })

        elif path == "/api/projects":
            status_f = query.get("status", [""])[0].strip() or None
            priority_f = query.get("priority", [""])[0].strip() or None
            limit, offset = parse_pagination(query, default_limit=50)
            items = list_projects(status=status_f, priority=priority_f, limit=limit, offset=offset)
            self.send_json({"items": items, "total": len(items)})

        elif path == "/api/decisions":
            status_f = query.get("status", [""])[0].strip() or None
            pending_only = query.get("pending_review", ["false"])[0].lower() in ("true", "1")
            limit, offset = parse_pagination(query, default_limit=50)
            items = list_decisions(status=status_f, pending_review_only=pending_only, limit=limit, offset=offset)
            self.send_json({"items": items, "total": len(items)})

        elif path == "/api/runway":
            data = compute_runway()
            self.send_json(data)

        elif path == "/api/radar":
            tier_val = query.get("tier", [""])[0].strip()
            tier_int = int(tier_val) if tier_val.isdigit() else None
            overdue_only = query.get("overdue_only", ["false"])[0].lower() in ("true", "1")
            limit, offset = parse_pagination(query, default_limit=50)
            items = compute_radar(tier=tier_int, overdue_only=overdue_only, limit=limit, offset=offset)
            summary = get_radar_summary()
            self.send_json({"items": items, "summary": summary, "total": len(items)})

        elif path == "/api/maintenance":
            status_f = query.get("status", ["pending"])[0].strip() or None
            category_f = query.get("category", [""])[0].strip() or None
            limit, offset = parse_pagination(query, default_limit=50)
            items = list_maintenance(status=status_f, category=category_f, limit=limit, offset=offset)
            summary = get_maintenance_summary()
            self.send_json({"items": items, "summary": summary, "total": len(items)})

        elif path == "/api/reviews":
            type_f = query.get("type", [""])[0].strip() or None
            limit, offset = parse_pagination(query, default_limit=20)
            items = list_retrospectives(period_type=type_f, limit=limit, offset=offset)
            self.send_json({"items": items, "total": len(items)})

        elif path.startswith("/events_media/"):
            filename = os.path.basename(path)
            media_path = MEDIA_DIR / filename
            if media_path.exists() and media_path.is_file():
                self.send_response(200)
                if filename.endswith(".png"):
                    self.send_header("Content-Type", "image/png")
                elif filename.endswith(".jpg") or filename.endswith(".jpeg"):
                    self.send_header("Content-Type", "image/jpeg")
                else:
                    self.send_header("Content-Type", "application/octet-stream")
                self.end_headers()
                with open(media_path, "rb") as f:
                    self.wfile.write(f.read())
            else:
                self.send_error(404, "Media file not found")
        else:
            self.send_error(404, "Not Found")


def get_local_ip() -> str:
    """Detects local network IP for mobile GPS tracker configuration."""
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect(("8.8.8.8", 80))
            return s.getsockname()[0]
    except Exception:
        return "127.0.0.1"


def find_available_port(preferred_port: int = 8000) -> int:
    """Discovers an available TCP port starting from preferred_port."""
    port = preferred_port
    while port < preferred_port + 100:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
            try:
                s.bind(("0.0.0.0", port))
                return port
            except OSError:
                port += 1
    return preferred_port


def start_dashboard_server(port: int = 8000, open_browser: bool = True) -> None:
    """Launches the zero-dependency web dashboard server and OwnTracks receiver."""
    init_db()
    actual_port = find_available_port(port)
    if actual_port != port:
        print(f"{C_YELLOW}Port {port} is in use. Using next available port: {actual_port}{C_RESET}")

    server_address = ("", actual_port)
    try:
        httpd = http.server.HTTPServer(server_address, DashboardRequestHandler)
    except OSError:
        server_address = ("", 0)
        httpd = http.server.HTTPServer(server_address, DashboardRequestHandler)
        actual_port = httpd.server_port

    lan_ip = get_local_ip()
    local_url = f"http://localhost:{actual_port}"
    webhook_url = f"http://{lan_ip}:{actual_port}/api/webhook/location"

    print(f"\n{C_GREEN}{C_BOLD}=== Personal Journal & CRM Web Dashboard ==={C_RESET}")
    print(f"  {C_CYAN}Dashboard URL:{C_RESET}       {local_url}")
    print(f"  {C_CYAN}LAN Access URL:{C_RESET}      http://{lan_ip}:{actual_port}")
    print(f"  {C_MAGENTA}OwnTracks Webhook:{C_RESET}   {webhook_url}\n")
    print(f"  Press {C_YELLOW}Ctrl+C{C_RESET} to stop the server.\n")

    if open_browser:
        try:
            webbrowser.open(local_url)
        except Exception:
            pass

    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print(f"\n{C_YELLOW}Dashboard server stopped.{C_RESET}")
        httpd.server_close()
