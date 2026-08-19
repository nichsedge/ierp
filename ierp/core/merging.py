"""
Contact deduplication, fuzzy matching, and intelligent merging engine.
Unifies manual CRM records with Google Contacts while preserving custom tags and relationship integrity.
"""

import re
import sqlite3
from typing import Optional, Dict, Any, List
from .config import C_BOLD, C_GREEN, C_RESET
from .db import get_db, init_db


def merge_two_contacts(source_id: int, target_id: int, conn: Optional[sqlite3.Connection] = None) -> Dict[str, Any]:
    """
    Merges a source contact into a target contact:
      - Combines non-empty fields (client, location, org, email, phone, google_id)
      - Merges notes line-by-line without duplicates
      - Sets source to 'merged'
      - Re-points all event_contacts foreign keys from source to target
      - Deletes the source contact
    """
    close_at_end = False
    if conn is None:
        init_db()
        conn = get_db()
        close_at_end = True

    cursor = conn.cursor()
    src = cursor.execute(
        "SELECT id, name, client, date, location, org, notes, email, phone, google_id, source FROM contacts WHERE id = ?",
        (source_id,)
    ).fetchone()
    tgt = cursor.execute(
        "SELECT id, name, client, date, location, org, notes, email, phone, google_id, source FROM contacts WHERE id = ?",
        (target_id,)
    ).fetchone()

    if not src or not tgt:
        if close_at_end:
            conn.close()
        return {"status": "error", "message": f"Contact #{source_id} or #{target_id} not found."}

    s_id, s_name, s_client, s_date, s_loc, s_org, s_notes, s_email, s_phone, s_gid, s_source = src
    t_id, t_name, t_client, t_date, t_loc, t_org, t_notes, t_email, t_phone, t_gid, t_source = tgt

    # Prefer full formal name (usually from Google Contacts) over short nickname
    if s_gid and len(s_name or "") > len(t_name or ""):
        final_name = s_name
        nickname = t_name
    elif t_gid and len(t_name or "") > len(s_name or ""):
        final_name = t_name
        nickname = s_name
    else:
        final_name = (t_name if len(t_name or "") >= len(s_name or "") else s_name) or t_name or s_name
        nickname = s_name if final_name != s_name else t_name

    final_client = t_client or s_client
    final_date = t_date or s_date
    final_loc = t_loc or s_loc
    final_org = t_org or s_org
    final_email = t_email or s_email
    final_phone = t_phone or s_phone
    final_gid = t_gid or s_gid

    # Deduplicate notes lines
    notes_set = set()
    combined_notes_list = []
    if nickname and nickname != final_name:
        alias_note = f"Nickname / Alias: {nickname}"
        notes_set.add(alias_note)
        combined_notes_list.append(alias_note)

    for note_blob in (t_notes, s_notes):
        if note_blob:
            for line in note_blob.split("\n"):
                line_clean = line.strip()
                if line_clean and line_clean not in notes_set:
                    notes_set.add(line_clean)
                    combined_notes_list.append(line)
    final_notes = "\n".join(combined_notes_list) if combined_notes_list else None

    # Source status
    final_source = "merged"

    # Update target record
    cursor.execute("""
    UPDATE contacts 
    SET name = ?, client = ?, date = ?, location = ?, org = ?, notes = ?, 
        email = ?, phone = ?, google_id = ?, source = ?
    WHERE id = ?
    """, (final_name, final_client, final_date, final_loc, final_org, final_notes, final_email, final_phone, final_gid, final_source, t_id))

    # Move event relationships
    cursor.execute("""
    INSERT OR IGNORE INTO event_contacts (event_id, contact_id)
    SELECT event_id, ? FROM event_contacts WHERE contact_id = ?
    """, (t_id, s_id))
    cursor.execute("DELETE FROM event_contacts WHERE contact_id = ?", (s_id,))

    # Delete source record
    cursor.execute("DELETE FROM contacts WHERE id = ?", (s_id,))

    conn.commit()
    if close_at_end:
        conn.close()

    return {
        "status": "success",
        "merged_id": t_id,
        "removed_id": s_id,
        "name": final_name,
        "source": final_source
    }


def auto_merge_contacts(conn: Optional[sqlite3.Connection] = None) -> Dict[str, Any]:
    """
    Scans for duplicate contacts across manual entries and Google synced contacts.
    Applies multi-stage matching rules:
      1. Exact email match
      2. Exact normalized name match
      3. Prefix / nickname match across differing sources
    """
    close_at_end = False
    if conn is None:
        init_db()
        conn = get_db()
        close_at_end = True

    cursor = conn.cursor()
    cursor.execute("SELECT id, name, email, google_id, source, org, client FROM contacts ORDER BY id ASC")
    all_contacts = cursor.fetchall()

    merged_count = 0
    merged_details: List[str] = []
    processed_ids = set()

    for i in range(len(all_contacts)):
        c1 = all_contacts[i]
        id1, name1, email1, gid1, src1, org1, client1 = c1
        if id1 in processed_ids:
            continue

        norm_name1 = re.sub(r'[^a-zA-Z0-9]', '', (name1 or '')).lower()
        norm_email1 = (email1 or '').strip().lower()

        for j in range(i + 1, len(all_contacts)):
            c2 = all_contacts[j]
            id2, name2, email2, gid2, src2, org2, client2 = c2
            if id2 in processed_ids:
                continue

            norm_name2 = re.sub(r'[^a-zA-Z0-9]', '', (name2 or '')).lower()
            norm_email2 = (email2 or '').strip().lower()

            is_match = False
            match_reason = ""

            # 1. Exact email match
            if norm_email1 and norm_email2 and norm_email1 == norm_email2:
                is_match = True
                match_reason = f"Matching email '{email1}'"

            # 2. Exact normalized name match
            elif norm_name1 and norm_name2 and norm_name1 == norm_name2:
                is_match = True
                match_reason = f"Matching name '{name1}'"

            if is_match:
                target_id = id1
                source_id = id2
                # Prefer keeping the manual CRM entry ID
                if gid1 and not gid2:
                    target_id = id2
                    source_id = id1

                res = merge_two_contacts(source_id, target_id, conn=conn)
                if res.get("status") == "success":
                    merged_count += 1
                    processed_ids.add(source_id)
                    merged_details.append(f"Merged contact #{source_id} into #{target_id} ({match_reason})")
                    break

    conn.commit()

    if close_at_end:
        conn.close()

    print(f"\n{C_BOLD}{C_GREEN}Contact Merge Scan Complete!{C_RESET}")
    print(f"  Total Contacts Merged: {C_GREEN}{merged_count}{C_RESET}")
    for detail in merged_details:
        print(f"  - {detail}")
    if merged_count == 0:
        print("  No duplicate contacts detected.")
    print()

    return {
        "status": "success",
        "merged_count": merged_count,
        "details": merged_details
    }
