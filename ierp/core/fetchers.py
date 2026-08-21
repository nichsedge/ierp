"""
Fetchers for external media trackers (stdlib only: urllib + html.parser + XML).

Each fetch_<source> function returns a list of raw row dicts; normalization to
ierp's schema happens in sources.py. Profiles are configured in config.py.
"""

import json
import os
import re
import time
import urllib.parse
from pathlib import Path
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET
from html.parser import HTMLParser
from typing import Callable, Optional

from .sources import _NULLISH

BROWSER_HEADERS = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
                  "(KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
    "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
    "Accept-Language": "en-US,en;q=0.9",
}


# ---------------------------------------------------------------- helpers

def _http(url: str, method: str = "GET", payload: Optional[dict] = None,
          headers: Optional[dict] = None, timeout: int = 20,
          browser: bool = True) -> tuple:
    """Returns (status_code, parsed_json_or_text). Raises on network errors."""
    data = json.dumps(payload).encode("utf-8") if payload is not None else None
    base = BROWSER_HEADERS if browser else {}
    req = urllib.request.Request(url, data=data, method=method,
                                 headers={**base, **(headers or {})})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read() or b""
            status = resp.status
    except urllib.error.HTTPError as e:
        body = e.read() or b""
        status = e.code
    text = body.decode("utf-8", errors="replace")
    if "application/json" in str((headers or {}).get("Content-Type", "")) or \
       (text and text.lstrip()[:1] in "{["):
        try:
            return status, json.loads(text)
        except json.JSONDecodeError:
            pass
    return status, text


def _graphql(url: str, query: str, variables: Optional[dict] = None,
             headers: Optional[dict] = None, timeout: int = 20) -> dict:
    """POSTs a GraphQL query; returns parsed data or raises ValueError."""
    status, data = _http(url, method="POST", timeout=timeout, headers={
        "Content-Type": "application/json", **(headers or {}),
    }, payload={"query": query, "variables": variables or {}})
    if status != 200:
        raise ConnectionError(f"GraphQL endpoint {url} returned HTTP {status}")
    if isinstance(data, dict) and data.get("errors"):
        msgs = ", ".join(e.get("message", "?") for e in data["errors"])
        raise ValueError(f"GraphQL error: {msgs}")
    if not isinstance(data, dict) or "data" not in data:
        raise ValueError(f"Unexpected GraphQL response from {url}")
    return data["data"]


def retry(fetch_fn: Callable, max_retries: int = 3, delay: int = 2):
    """Retries a fetch function with linear backoff; re-raises on final failure."""
    for attempt in range(1, max_retries + 1):
        try:
            return fetch_fn()
        except Exception:
            if attempt == max_retries:
                raise
            time.sleep(delay * attempt)


def _clean_isbn(v) -> str:
    """Strips ISBN noise (quotes, '=\"...\"' CSV artifacts, X suffix is kept)."""
    if v is None:
        return ""
    s = str(v).strip()
    s = re.sub(r'^="?', "", s).replace('"', "")
    return s


def _num(v):
    """Numeric string -> float/int; else the cleaned string; None if nullish."""
    s = str(v or "").strip()
    if s.lower() in _NULLISH:
        return None
    try:
        f = float(s)
        return int(f) if f.is_integer() else f
    except ValueError:
        return s


def _iso(v) -> str:
    """Best-effort YYYY-MM-DD prefix of a date string."""
    s = str(v or "").strip()
    return s[:10] if len(s) >= 10 and s[4] == "-" and s[7] == "-" else s


def _load_env():
    """Populates os.environ from ~/.secrets or repo-root .env (idempotent)."""
    repo_root = Path(__file__).resolve().parent.parent.parent
    for candidate in (os.path.expanduser("~/.secrets"),
                      repo_root / ".env",
                      os.path.join(os.path.dirname(__file__), ".env")):
        if not os.path.exists(candidate):
            continue
        try:
            with open(candidate, "r", encoding="utf-8") as f:
                for line in f:
                    line = line.strip()
                    if not line or line.startswith("#") or "=" not in line:
                        continue
                    if line.startswith("export "):
                        line = line[7:]
                    k, v = line.split("=", 1)
                    os.environ.setdefault(k.strip(), v.strip().strip("'\""))
        except OSError:
            pass


_load_env()


# ---------------------------------------------------------------- fetchers

def fetch_hardcover(username: str, api_key: Optional[str] = None) -> list:
    """Fetches user books from the Hardcover.app GraphQL API."""
    token = api_key or os.environ.get("HARDCOVER_API_KEY")
    if not token:
        raise ValueError("HARDCOVER_API_KEY not found in environment or ~/.secrets.")
    print(f"Fetching Hardcover books for user: {username}...")
    query = f"""
    query GetUserBooks {{
      user_books(where: {{user: {{username: {{_eq: "{username}"}}}}}}) {{
        id rating status_id created_at updated_at
        user_book_reads {{ started_at finished_at }}
        edition {{
          title pages release_date isbn_10 isbn_13
          publisher {{ name }}
          book {{
            title description release_year rating
            contributions {{ author {{ name }} }}
          }}
        }}
      }}
    }}
    """
    data = retry(lambda: _graphql(
        "https://api.hardcover.app/v1/graphql", query,
        headers={"Authorization": token if token.startswith("Bearer ") else f"Bearer {token}",
                 "User-Agent": "ierp/1.0"},
    ))
    user_books = data.get("user_books", [])
    if not user_books:
        raise ValueError(f"No books retrieved from Hardcover for user: {username}")

    status_map = {1: "to-read", 2: "currently-reading", 3: "read"}
    records = []
    for item in user_books:
        edition = item.get("edition") or {}
        book = edition.get("book") or {}
        authors = [c.get("author", {}).get("name")
                   for c in (book.get("contributions") or [])
                   if c.get("author", {}).get("name")]
        reads = item.get("user_book_reads") or []
        finished = reads[-1].get("finished_at") if reads else None
        records.append({
            "Title": edition.get("title") or book.get("title") or "",
            "Author": ", ".join(authors),
            "My Rating": item.get("rating"),
            "Average Rating": book.get("rating"),
            "Pages": edition.get("pages"),
            "Year Published": str(book.get("release_year") or edition.get("release_date") or "")[:4],
            "Date Added": _iso(item.get("created_at", "")),
            "Date Read": _iso(finished or ""),
            "Bookshelves": status_map.get(item.get("status_id"), ""),
            "ISBN": _clean_isbn(edition.get("isbn_10")),
            "ISBN13": _clean_isbn(edition.get("isbn_13")),
        })
    print(f"Fetched {len(records)} books from Hardcover.")
    return records


def fetch_goodreads(user_id: str, shelf: str = "#ALL#") -> list:
    """Fetches user books from the Goodreads RSS feed (paginated)."""
    print(f"Fetching Goodreads data via RSS for user ID: {user_id}...")
    books = []
    page = 1
    while True:
        url = (f"https://www.goodreads.com/review/list_rss/{user_id}"
               f"?shelf={urllib.parse.quote(shelf)}&page={page}")

        def do_get():
            return _http(url, headers={"User-Agent": "Mozilla/5.0"}, timeout=15)

        status, text = retry(do_get)
        if status == 404:
            raise ValueError(f"Goodreads user ID '{user_id}' not found (HTTP 404).")
        if status != 200:
            raise ConnectionError(f"Goodreads RSS returned HTTP {status}")

        root = ET.fromstring(text.encode("utf-8"))
        channel = root.find("channel")
        items = channel.findall("item") if channel is not None else []
        if not items:
            break

        for item in items:
            def txt(tag):
                el = item.find(tag)
                return el.text.strip() if el is not None and el.text else ""

            rating = txt("user_rating")
            books.append({
                "Title": txt("title"),
                "Author": txt("author_name"),
                "Date Added": _iso(txt("user_date_added")),
                "Date Read": _iso(txt("user_read_at")),
                "My Rating": rating if rating not in ("", "0") else "",
                "Average Rating": txt("average_rating"),
                "Bookshelves": txt("user_shelves"),
                "ISBN": _clean_isbn(txt("isbn")),
                "ISBN13": _clean_isbn(txt("isbn13")),
                "Number of Pages": txt("book_medium") or txt(".//num_pages"),
                "Year Published": txt("book_published"),
                "My Review": txt("user_review"),
            })
        if len(items) < 100:
            break
        page += 1

    if not books:
        raise ValueError(f"No books retrieved from Goodreads for user ID: {user_id}.")
    print(f"Fetched {len(books)} books from Goodreads.")
    return books


class _LetterboxdParser(HTMLParser):
    """Extracts rated films from Letterboxd ratings-page HTML."""

    def __init__(self):
        super().__init__()
        self.films: list = []
        self._stack: list = []
        self._current: Optional[dict] = None
        self._capture_class: Optional[str] = None
        self._buf: list = []

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        classes = (a.get("class") or "").split()
        self._stack.append((tag, classes))

        if "griditem" in classes:
            self._current = {"Name": "", "Year": None, "Letterboxd URI": "",
                             "Rating": None, "Date": None}
        elif self._current is not None:
            if "data-item-full-display-name" in a:
                full = a.get("data-item-full-display-name") or ""
                if full:
                    m = re.search(r"\((\d{4})\)", full)
                    if m:
                        self._current["Year"] = int(m.group(1))
                        self._current["Name"] = re.sub(r"\s*\(\d{4}\)", "", full).strip()
                    else:
                        self._current["Name"] = a.get("data-item-name") or full
                link = a.get("data-item-link", "")
                if link:
                    self._current["Letterboxd URI"] = f"https://letterboxd.com{link}"
            rated = next((c for c in classes if c.startswith("rated-")), None)
            if rated:
                try:
                    self._current["Rating"] = int(rated.split("-")[1]) / 2.0
                except ValueError:
                    pass

    def handle_endtag(self, tag):
        if not self._stack:
            return
        popped_tag, popped_classes = self._stack.pop()
        if popped_tag == tag and "griditem" in popped_classes and self._current:
            if self._current["Name"]:
                self.films.append(self._current)
            self._current = None

    def close(self):
        super().close()


def fetch_letterboxd(username: str, max_pages: int = 200) -> list:
    """Scrapes the user's rated films from Letterboxd (public ratings pages)."""
    print(f"Fetching Letterboxd ratings for user: {username}...")
    films: list = []
    page = 1
    while page <= max_pages:
        url = f"https://letterboxd.com/{username}/films/ratings/page/{page}/"
        status, html = retry(lambda: _http(url, timeout=15))
        if status == 404:
            raise ValueError(f"Letterboxd profile '{username}' not found (HTTP 404).")
        if status != 200:
            raise ConnectionError(f"Letterboxd returned HTTP {status}")

        parser = _LetterboxdParser()
        parser.feed(html)
        if not parser.films:
            break
        films.extend(parser.films)

        if not html or 'paginate-page' not in html:
            break
        page += 1
        time.sleep(0.3)

    if not films:
        raise ValueError(f"No rated films found for Letterboxd user: {username}.")
    print(f"Fetched {len(films)} rated films from Letterboxd.")
    return films


def fetch_anilist(username: str, media_type: str = "ANIME") -> list:
    """Fetches anime or manga list entries from the AniList GraphQL API."""
    print(f"Fetching AniList {media_type} for user: {username}...")
    query = """
    query ($userName: String, $type: MediaType) {
      MediaListCollection(userName: $userName, type: $type) {
        lists {
          entries {
            id status score(format: POINT_10_DECIMAL) progress repeat notes
            startedAt { year month day }
            completedAt { year month day }
            media {
              id idMal
              title { romaji english native }
              format episodes chapters volumes
              seasonYear startDate { year }
            }
          }
        }
      }
    }
    """
    data = retry(lambda: _graphql(
        "https://graphql.anilist.co", query,
        variables={"userName": username, "type": media_type.upper()},
    ))
    lists = (data.get("MediaListCollection") or {}).get("lists", [])

    status_map = {
        "COMPLETED": "Completed",
        "CURRENT": "Watching" if media_type == "ANIME" else "Reading",
        "PLANNING": "Plan to Watch" if media_type == "ANIME" else "Plan to Read",
        "DROPPED": "Dropped",
        "PAUSED": "On-Hold",
        "REPEATING": "Rewatching" if media_type == "ANIME" else "Rereading",
    }

    def _ymd(node):
        if not node or not (node.get("year") and node.get("month") and node.get("day")):
            return ""
        return f"{node['year']:04d}-{node['month']:02d}-{node['day']:02d}"

    records = []
    for lst in lists:
        for entry in lst.get("entries", []):
            media = entry.get("media") or {}
            t = media.get("title") or {}
            records.append({
                "series_animedb_id": media.get("idMal") or media.get("id"),
                "series_title": t.get("romaji") or t.get("english") or t.get("native") or "",
                "series_type": media.get("format") or "",
                "series_episodes": media.get("episodes") if media_type == "ANIME" else media.get("chapters"),
                "series_native_title": t.get("native") or "",
                "series_season_year": media.get("seasonYear") or (media.get("startDate") or {}).get("year"),
                "my_id": entry.get("id"),
                "my_watched_episodes": entry.get("progress"),
                "my_start_date": _ymd(entry.get("startedAt")),
                "my_finish_date": _ymd(entry.get("completedAt")),
                "my_score": entry.get("score"),
                "my_status": status_map.get(entry.get("status", ""), (entry.get("status") or "").title()),
                "my_times_watched": entry.get("repeat", 0),
                "my_comments": entry.get("notes"),
                "my_priority": "LOW",
                "manga_title": t.get("romaji") or t.get("english") or t.get("native") or "",
                "title": t.get("romaji") or t.get("english") or t.get("native") or "",
            })
    print(f"Fetched {len(records)} AniList {media_type} entries.")
    return records


class _MDLTableParser(HTMLParser):
    """Extracts rows from MyDramaList 'mdl-style-table' tables."""

    _LANGS = r"\s*(?:Korean|Japanese|Chinese|Taiwanese|Thai|Hong Kong)\s*(?:Movie|Drama|Special)$"

    def __init__(self):
        super().__init__()
        self.rows: list = []
        self._in_table = False
        self._in_row = False
        self._in_cell = False
        self._current_row: list = []
        self._current_cell: list = []

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        classes = (a.get("class") or "").split()
        if tag == "table" and "mdl-style-table" in classes:
            self._in_table = True
        elif self._in_table and tag == "tr":
            self._in_row = True
            self._current_row = []
        elif self._in_table and self._in_row and tag == "td":
            self._in_cell = True
            self._current_cell = []

    def handle_data(self, data):
        if self._in_cell:
            self._current_cell.append(data)

    def handle_endtag(self, tag):
        if self._in_table and self._in_row and tag == "td" and self._in_cell:
            self._in_cell = False
            self._current_row.append("".join(self._current_cell))
        elif self._in_table and self._in_row and tag == "tr":
            self._in_row = False
            if self._current_row:
                self.rows.append(self._current_row)
        elif self._in_table and tag == "table":
            self._in_table = False
            self._in_row = False

    def get_rows(self) -> list:
        out = []
        for cells in self.rows:
            if len(cells) < 6:
                continue
            texts = ["".join(c).strip() for c in cells]
            title = re.sub(self._LANGS, "", texts[0], flags=re.IGNORECASE).strip()
            out.append({
                "Title": title,
                "Country": texts[1],
                "Year": _num(texts[2]),
                "Type": texts[3],
                "Score": _num(texts[4]),
                "Progress": texts[5],
            })
        return out


def fetch_mydramalist(username: str) -> list:
    """Scrapes the user's public MyDramaList page."""
    print(f"Fetching MyDramaList for user: {username}...")
    url = f"https://mydramalist.com/dramalist/{username}"
    status, html = retry(lambda: _http(url, timeout=20))
    if status == 404:
        raise ValueError(f"MyDramaList user '{username}' not found (HTTP 404).")
    if status != 200:
        raise ConnectionError(f"MyDramaList returned HTTP {status}")

    parser = _MDLTableParser()
    parser.feed(html)
    rows = parser.get_rows()
    if not rows:
        raise ValueError(f"No dramalist tables found for MyDramaList user '{username}'.")
    print(f"Fetched {len(rows)} MyDramaList entries.")
    return rows
