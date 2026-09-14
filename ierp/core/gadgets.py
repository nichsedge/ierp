"""
Gadgets & Hardware Assets domain service for iERP (Individual Enterprise Resource Planning).
Provides asset management, specs tracking, lifecycle states, relational links,
import from digital garden markdown, and downstream markdown synchronization.
"""

import json
import re
import sqlite3
from contextlib import closing
from datetime import datetime
from pathlib import Path
from typing import Any

from .config import DB_PATH
from .db import get_db, init_db


def generate_slug(name: str) -> str:
    """Generates a clean URL/filesystem friendly slug from a gadget name."""
    s = re.sub(r"[^\w\s\-]", "", name.lower()).strip()
    s = re.sub(r"[\s_]+", "-", s)
    return s or "gadget"


def infer_brand_and_category(name: str) -> tuple[str, str]:
    """Infers brand and device category from common naming patterns."""
    n = name.strip()
    brand = "Unknown"
    category = "Gadget"

    known_brands = [
        "Xiaomi", "Redmi", "Samsung", "SAMSUNG", "Huawei",
        "Lenovo", "Asus", "Alcatel", "Apple", "Sony",
        "Google", "Flexi", "OnePlus", "Oppo", "Vivo",
    ]
    for b in known_brands:
        if n.lower().startswith(b.lower()):
            brand = b.capitalize()
            if brand == "Redmi":
                brand = "Xiaomi"
            break

    lower_n = n.lower()
    if any(w in lower_n for w in ["band", "watch", "smartband", "smartwatch"]):
        category = "Wearable"
    elif any(w in lower_n for w in ["gt-e", "flexi", "keystore", "feature"]):
        category = "Feature Phone"
    elif any(w in lower_n for w in ["phone", "pro", "mini", "note", "zenfone", "redmi", "a390", "alcatel"]):
        category = "Smartphone"
    elif any(w in lower_n for w in ["laptop", "thinkpad", "macbook"]):
        category = "Laptop"
    elif any(w in lower_n for w in ["tab", "pad", "ipad"]):
        category = "Tablet"

    return brand, category


def insert_gadget(
    name: str,
    slug: str | None = None,
    brand: str | None = None,
    model: str | None = None,
    category: str | None = None,
    status: str = "active",
    purchase_date: str | None = None,
    purchase_price: float | None = None,
    currency: str = "IDR",
    specs: dict[str, Any] | str | None = None,
    serial_number: str | None = None,
    vendor_id: int | None = None,
    event_id: int | None = None,
    notes: str | None = None,
    is_public: bool = True,
    db_path: Path | None = None,
) -> int:
    """Inserts a structured gadget record directly into SQLite. Returns gadgets.id."""
    init_db(db_path)
    base_slug = slug or generate_slug(name)

    if specs and isinstance(specs, dict):
        specs_json = json.dumps(specs, ensure_ascii=False)
    elif specs and isinstance(specs, str):
        specs_json = specs.strip()
    else:
        specs_json = None

    if not brand or not category:
        inf_brand, inf_cat = infer_brand_and_category(name)
        brand = brand or inf_brand
        category = category or inf_cat

    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()

        final_slug = base_slug
        counter = 2
        while True:
            cursor.execute("SELECT id FROM gadgets WHERE slug = ?", (final_slug,))
            if not cursor.fetchone():
                break
            final_slug = f"{base_slug}-{counter}"
            counter += 1

        cursor.execute(
            """
            INSERT INTO gadgets (
                slug, name, brand, model, category, status,
                purchase_date, purchase_price, currency, specs_json,
                serial_number, vendor_id, event_id,
                notes, is_public
            )
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                final_slug,
                name.strip(),
                brand.strip() if brand else None,
                model.strip() if model else None,
                category.strip() if category else None,
                status.strip() if status else "active",
                purchase_date.strip() if purchase_date else None,
                purchase_price,
                currency.strip() if currency else "IDR",
                specs_json,
                serial_number.strip() if serial_number else None,
                vendor_id,
                event_id,
                notes.strip() if notes else None,
                1 if is_public else 0,
            ),
        )
        conn.commit()
        return int(cursor.lastrowid or 0)


def update_gadget(
    gadget_id: int,
    name: str | None = None,
    brand: str | None = None,
    model: str | None = None,
    category: str | None = None,
    status: str | None = None,
    purchase_date: str | None = None,
    purchase_price: float | None = None,
    currency: str | None = None,
    specs: dict[str, Any] | str | None = None,
    serial_number: str | None = None,
    vendor_id: int | None = None,
    event_id: int | None = None,
    notes: str | None = None,
    is_public: bool | None = None,
    db_path: Path | None = None,
) -> bool:
    """Updates an existing gadget record. Returns True if updated."""
    init_db(db_path)
    fields = []
    params: list[Any] = []

    if name is not None:
        fields.append("name = ?")
        params.append(name.strip())
    if brand is not None:
        fields.append("brand = ?")
        params.append(brand.strip())
    if model is not None:
        fields.append("model = ?")
        params.append(model.strip())
    if category is not None:
        fields.append("category = ?")
        params.append(category.strip())
    if status is not None:
        fields.append("status = ?")
        params.append(status.strip())
    if purchase_date is not None:
        fields.append("purchase_date = ?")
        params.append(purchase_date.strip() if purchase_date else None)
    if purchase_price is not None:
        fields.append("purchase_price = ?")
        params.append(purchase_price)
    if currency is not None:
        fields.append("currency = ?")
        params.append(currency.strip())
    if specs is not None:
        fields.append("specs_json = ?")
        if isinstance(specs, dict):
            params.append(json.dumps(specs, ensure_ascii=False))
        else:
            params.append(str(specs).strip())
    if serial_number is not None:
        fields.append("serial_number = ?")
        params.append(serial_number.strip() if serial_number else None)
    if vendor_id is not None:
        fields.append("vendor_id = ?")
        params.append(vendor_id)
    if event_id is not None:
        fields.append("event_id = ?")
        params.append(event_id)
    if notes is not None:
        fields.append("notes = ?")
        params.append(notes.strip() if notes else None)
    if is_public is not None:
        fields.append("is_public = ?")
        params.append(1 if is_public else 0)

    if not fields:
        return False

    fields.append("updated_at = datetime('now', 'localtime')")
    params.append(gadget_id)

    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        query = f"UPDATE gadgets SET {', '.join(fields)} WHERE id = ?"
        cursor.execute(query, params)
        conn.commit()
        return cursor.rowcount > 0


def get_gadget(gadget_id_or_slug: int | str, db_path: Path | None = None) -> dict[str, Any] | None:
    """Fetches a single gadget with joined event and vendor details."""
    init_db(db_path)
    with closing(get_db(db_path)) as conn:
        conn.row_factory = sqlite3.Row
        cursor = conn.cursor()

        if isinstance(gadget_id_or_slug, int) or (isinstance(gadget_id_or_slug, str) and gadget_id_or_slug.isdigit()):
            cursor.execute(
                """
                SELECT g.*, v.name as vendor_name, e.title as event_title
                FROM gadgets g
                LEFT JOIN vendors v ON g.vendor_id = v.id
                LEFT JOIN events e ON g.event_id = e.id
                WHERE g.id = ?
                """,
                (int(gadget_id_or_slug),),
            )
        else:
            cursor.execute(
                """
                SELECT g.*, v.name as vendor_name, e.title as event_title
                FROM gadgets g
                LEFT JOIN vendors v ON g.vendor_id = v.id
                LEFT JOIN events e ON g.event_id = e.id
                WHERE g.slug = ?
                """,
                (str(gadget_id_or_slug),),
            )

        row = cursor.fetchone()
        return dict(row) if row else None


def list_gadgets(
    category: str | None = None,
    status: str | None = None,
    brand: str | None = None,
    q: str | None = None,
    limit: int = 50,
    offset: int = 0,
    sort_col: str = "g.purchase_date",
    sort_dir: str = "DESC",
    db_path: Path | None = None,
) -> tuple[list[dict[str, Any]], int]:
    """Lists gadgets with filtering, search, and pagination."""
    init_db(db_path)
    where_clauses = ["1=1"]
    params: list[Any] = []

    if category:
        where_clauses.append("g.category = ?")
        params.append(category)
    if status:
        where_clauses.append("g.status = ?")
        params.append(status)
    if brand:
        where_clauses.append("g.brand = ?")
        params.append(brand)
    if q:
        where_clauses.append("(g.name LIKE ? OR g.brand LIKE ? OR g.model LIKE ? OR g.notes LIKE ?)")
        wildcard = f"%{q}%"
        params.extend([wildcard, wildcard, wildcard, wildcard])

    where_sql = " AND ".join(where_clauses)
    valid_cols = {"g.id", "g.name", "g.brand", "g.category", "g.status", "g.purchase_date", "g.created_at"}
    if sort_col not in valid_cols:
        sort_col = "g.purchase_date"
    sort_dir = "ASC" if sort_dir.upper() == "ASC" else "DESC"

    with closing(get_db(db_path)) as conn:
        conn.row_factory = sqlite3.Row
        cursor = conn.cursor()

        count_query = f"SELECT COUNT(*) FROM gadgets g WHERE {where_sql}"
        cursor.execute(count_query, params)
        total_count = cursor.fetchone()[0]

        query = f"""
        SELECT g.*, v.name as vendor_name, e.title as event_title
        FROM gadgets g
        LEFT JOIN vendors v ON g.vendor_id = v.id
        LEFT JOIN events e ON g.event_id = e.id
        WHERE {where_sql}
        ORDER BY {sort_col} {sort_dir} NULLS LAST
        LIMIT ? OFFSET ?
        """
        cursor.execute(query, params + [limit, offset])
        rows = [dict(r) for r in cursor.fetchall()]

        return rows, total_count


def delete_gadget(gadget_id: int, db_path: Path | None = None) -> bool:
    """Deletes a gadget record by ID."""
    init_db(db_path)
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        cursor.execute("DELETE FROM gadgets WHERE id = ?", (gadget_id,))
        conn.commit()
        return cursor.rowcount > 0


def _parse_frontmatter_and_body(text: str) -> tuple[dict[str, Any], str]:
    """Parses frontmatter and markdown body using standard library only."""
    fm: dict[str, Any] = {}
    body = text
    if text.startswith("---"):
        parts = text.split("---", 2)
        if len(parts) >= 3:
            raw_fm = parts[1]
            body = parts[2]
            for line in raw_fm.splitlines():
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                if ":" in line:
                    key, val = line.split(":", 1)
                    key = key.strip()
                    val = val.strip()
                    if val.startswith('"') and val.endswith('"'):
                        val = val[1:-1]
                    elif val.startswith("'") and val.endswith("'"):
                        val = val[1:-1]
                    elif val.startswith("[") and val.endswith("]"):
                        items = [x.strip().strip('"\'') for x in val[1:-1].split(",") if x.strip()]
                        val = items
                    elif val.lower() == "true":
                        val = True
                    elif val.lower() == "false":
                        val = False
                    fm[key] = val
    return fm, body.strip()


def import_garden_gadgets(garden_dir: Path | str | None = None, db_path: Path | None = None) -> tuple[int, int]:
    """
    Imports gadget markdown notes from the Digital Garden into the iERP SQLite database.
    Idempotently matches on name or slug.
    """
    if garden_dir is None:
        possible = [
            Path.home() / "Projects" / "digital-garden" / "content" / "Knowledge" / "Entities" / "Gadget",
            Path.home() / "Projects" / "digital-graveyard" / "content" / "Knowledge" / "Entities" / "Gadget",
        ]
        for p in possible:
            if p.is_dir():
                garden_dir = p
                break

    if not garden_dir or not Path(garden_dir).is_dir():
        return 0, 0

    target_dir = Path(garden_dir)
    imported = 0
    skipped = 0

    init_db(db_path)
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()

        for md_file in sorted(target_dir.glob("*.md")):
            if md_file.name.lower() == "index.md":
                continue

            content = md_file.read_text(encoding="utf-8")
            fm, body = _parse_frontmatter_and_body(content)

            name = fm.get("title") or md_file.stem
            date = fm.get("date")
            purchase_date = str(date) if date else None

            # Clean body from markdown header matching name
            cleaned_body_lines = []
            for line in body.splitlines():
                if line.strip() == f"# {name}":
                    continue
                if line.strip():
                    cleaned_body_lines.append(line.strip())
            notes_str = "\n".join(cleaned_body_lines) if cleaned_body_lines else None

            # Specs extraction if body contains specs patterns e.g. "Second, 12 / 256"
            specs_data: dict[str, Any] = {}
            if notes_str and "12 / 256" in notes_str:
                specs_data["ram"] = "12GB"
                specs_data["storage"] = "256GB"
                if "second" in notes_str.lower():
                    specs_data["condition"] = "second"

            slug = generate_slug(name)
            brand, category = infer_brand_and_category(name)

            cursor.execute("SELECT id FROM gadgets WHERE slug = ? OR name = ?", (slug, name))
            existing = cursor.fetchone()

            if existing:
                gid = existing[0]
                update_gadget(
                    gadget_id=gid,
                    name=name,
                    brand=brand,
                    category=category,
                    purchase_date=purchase_date,
                    specs=specs_data if specs_data else None,
                    notes=notes_str,
                    db_path=db_path,
                )
                skipped += 1
            else:
                insert_gadget(
                    name=name,
                    slug=slug,
                    brand=brand,
                    category=category,
                    purchase_date=purchase_date,
                    specs=specs_data if specs_data else None,
                    notes=notes_str,
                    db_path=db_path,
                )
                imported += 1

    return imported, skipped


def export_garden_gadgets(
    garden_dir: Path | str | None = None,
    dry_run: bool = False,
    db_path: Path | None = None,
) -> dict[str, Any]:
    """
    Exports all gadgets from iERP SQLite database to the Digital Garden.
    Generates structured markdown note for each gadget and an updated index.md catalog.
    """
    if garden_dir is None:
        possible = [
            Path.home() / "Projects" / "digital-garden" / "content" / "Knowledge" / "Entities" / "Gadget",
            Path.home() / "Projects" / "digital-graveyard" / "content" / "Knowledge" / "Entities" / "Gadget",
        ]
        for p in possible:
            if p.is_dir():
                garden_dir = p
                break
        if garden_dir is None:
            garden_dir = Path.home() / "Projects" / "digital-garden" / "content" / "Knowledge" / "Entities" / "Gadget"

    target_dir = Path(garden_dir)
    if not dry_run:
        target_dir.mkdir(parents=True, exist_ok=True)

    rows, total = list_gadgets(limit=500, sort_col="g.name", sort_dir="ASC", db_path=db_path)

    written_notes = 0
    catalog_entries = []

    for g in rows:
        if not g.get("is_public", 1):
            continue

        name = g["name"]
        slug = g["slug"]
        brand = g["brand"] or "Unknown"
        cat = g["category"] or "Gadget"
        status = g["status"] or "active"
        p_date = g["purchase_date"] or datetime.now().strftime("%Y-%m-%d")
        specs_json = g.get("specs_json")

        specs_dict = {}
        if specs_json:
            try:
                specs_dict = json.loads(specs_json)
            except Exception:
                pass

        # Build note content
        fm_lines = [
            "---",
            f'title: "{name}"',
            f"date: {p_date}",
            "tags: [gadget]",
            "publish_external: true",
            "status: seedling",
            f'brand: "{brand}"',
            f'category: "{cat}"',
            f'device_status: "{status}"',
        ]
        fm_lines.append("---")

        body_lines = [f"# {name}", ""]
        meta_items = [
            ("Brand", brand),
            ("Category", cat),
            ("Status", status.capitalize()),
            ("Acquired", p_date),
        ]
        if g.get("purchase_price"):
            curr = g.get("currency") or "IDR"
            meta_items.append(("Purchase Price", f"{curr} {g['purchase_price']:,.0f}"))
        if g.get("vendor_name"):
            meta_items.append(("Vendor", g["vendor_name"]))
        if g.get("event_title"):
            meta_items.append(("Event", f"[[{g['event_title']}]]"))

        for k, v in specs_dict.items():
            meta_items.append((k.replace("_", " ").capitalize(), str(v)))

        for label, val in meta_items:
            body_lines.append(f"- **{label}:** {val}")

        if g.get("notes"):
            body_lines.append("")
            body_lines.append("## Notes")
            body_lines.append("")
            body_lines.append(g["notes"].strip())

        full_content = "\n".join(fm_lines) + "\n\n" + "\n".join(body_lines) + "\n"

        file_path = target_dir / f"{name}.md"
        if not dry_run:
            file_path.write_text(full_content, encoding="utf-8")
        written_notes += 1

        catalog_entries.append({
            "name": name,
            "brand": brand,
            "category": cat,
            "status": status,
            "date": p_date,
        })

    # Build index.md
    index_fm = [
        "---",
        'title: "Gadget"',
        f"date: {datetime.now().strftime('%Y-%m-%d')}",
        "tags: [gadget]",
        "publish_external: true",
        "---",
    ]
    index_body = [
        "## Gadget",
        "",
        "Structured hardware devices and inventory catalogue synchronized from the iERP warehouse.",
        "",
        "| Device | Category | Brand | Status | Acquired |",
        "| :--- | :--- | :--- | :--- | :--- |",
    ]
    for e in catalog_entries:
        index_body.append(
            f"| [[{e['name']}]] | {e['category']} | {e['brand']} | `{e['status']}` | {e['date']} |"
        )
    index_body.append("")
    index_body.append("### Directory")
    index_body.append("")
    for e in catalog_entries:
        index_body.append(f"- [[{e['name']}]]")
    index_body.append("")

    full_index = "\n".join(index_fm) + "\n\n" + "\n".join(index_body)
    index_path = target_dir / "index.md"
    if not dry_run:
        index_path.write_text(full_index, encoding="utf-8")

    return {
        "notes_written": written_notes,
        "index_updated": True,
        "target_dir": str(target_dir),
        "dry_run": dry_run,
    }
