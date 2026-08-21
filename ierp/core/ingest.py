"""
Ingestion bridge for external fetchers (stdlib only).

Fetchers (e.g. digital-graveyard's get-data) POST raw rows here; ierp owns all
normalization and storage. Two modes:

  1. Library:  ingest_rows(source_key, rows) -> dict
  2. HTTP:     `uv run ierp serve-ingest --port 8765` then POST
               /ingest/<source_key> with a JSON array of raw row objects.

The HTTP mode exists so a fetcher in another repo/venv never needs ierp
installed — it just POSTs JSON.
"""

import json
import sqlite3
from contextlib import closing
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Optional

from .db import get_db, init_db
from .media import ingest_media_records
from .sources import SOURCE_MAP, normalize_rows


def ingest_rows(source_key: str, rows: list, db_path: Optional[Path] = None) -> dict:
    """
    Normalizes raw rows for a source and bulk-ingests them in one transaction.
    Cross-source duplicates are skipped when a higher-priority source
    (SOURCE_PREFERENCE in sources.py) already has the same title.
    Returns {"items": n, "skipped": n}.
    """
    init_db(db_path)
    records = normalize_rows(source_key, rows, existing_titles=_stored_titles(db_path, source_key))
    result = ingest_media_records(records, db_path)
    result["skipped"] = len(rows) - len(records)
    return result


def _stored_titles(db_path: Optional[Path], source_key: str) -> set:
    """
    Normalized titles already stored from sources with HIGHER priority than
    source_key for its media_type. Used to shadow duplicate rows.
    """
    from .db import get_db
    from .sources import SOURCE_MAP, SOURCE_PREFERENCE, normalize_title

    media_type, src = SOURCE_MAP.get(source_key, (source_key, source_key))
    preferred = SOURCE_PREFERENCE.get(media_type, ())
    if src not in preferred:
        return set()
    higher = preferred[:preferred.index(src)]
    if not higher:
        return set()

    placeholders = ",".join("?" for _ in higher)
    with closing(get_db(db_path)) as conn:
        rows = conn.execute(
            f"SELECT title FROM media_items WHERE media_type = ? AND source IN ({placeholders})",
            (media_type, *higher),
        ).fetchall()
    return {normalize_title(t) for (t,) in rows}


def make_handler(db_path: Optional[Path] = None):
    """Builds a request handler bound to a specific DB path."""

    class IngestHandler(BaseHTTPRequestHandler):
        def _send(self, code: int, payload: dict) -> None:
            body = json.dumps(payload).encode("utf-8")
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_POST(self):
            if not self.path.startswith("/ingest/"):
                self._send(404, {"error": "POST to /ingest/<source_key>"})
                return
            source_key = self.path[len("/ingest/"):].strip("/")
            if source_key not in SOURCE_MAP:
                self._send(400, {"error": f"unknown source '{source_key}'",
                                 "known": sorted(SOURCE_MAP)})
                return
            try:
                length = int(self.headers.get("Content-Length", 0))
                if length > 64 * 1024 * 1024:
                    self._send(413, {"error": "payload too large"})
                    return
                rows = json.loads(self.rfile.read(length) or b"[]")
                if not isinstance(rows, list):
                    self._send(400, {"error": "body must be a JSON array of row objects"})
                    return
                result = ingest_rows(source_key, rows, db_path)
                self._send(200, result)
            except (json.JSONDecodeError, ValueError) as e:
                self._send(400, {"error": f"bad request: {e}"})
            except sqlite3.Error as e:
                self._send(500, {"error": f"database error: {e}"})

        def log_message(self, format, *args):  # noqa: A002 - stdlib signature
            pass

    return IngestHandler


def serve_ingest(port: int = 8765, db_path: Optional[Path] = None) -> None:
    """Starts the local ingestion HTTP server (Ctrl+C to stop)."""
    init_db(db_path)
    server = ThreadingHTTPServer(("127.0.0.1", port), make_handler(db_path))
    print(f"ierp ingest server listening on http://127.0.0.1:{port}")
    print(f"  POST /ingest/<source_key>   known sources: {', '.join(sorted(SOURCE_MAP))}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down.")
    finally:
        server.server_close()
