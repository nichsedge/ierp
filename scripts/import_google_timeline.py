#!/usr/bin/env python3
import json
import re
import math
import time
import urllib.request
import urllib.parse
import subprocess
import os

def haversine(lat1, lon1, lat2, lon2):
    R = 6371000  # meters
    phi1, phi2 = math.radians(lat1), math.radians(lat2)
    delta_phi = math.radians(lat2 - lat1)
    delta_lambda = math.radians(lon2 - lon1)
    a = math.sin(delta_phi/2)**2 + math.cos(phi1)*math.cos(phi2)*math.sin(delta_lambda/2)**2
    return R * 2 * math.atan2(math.sqrt(a), math.sqrt(1 - a))

def clean_str(s):
    if not s:
        return ''
    return s.replace("'", "''").strip()

def main():
    merged_path = '/home/al/Downloads/Timeline_merged.json'
    if not os.path.exists(merged_path):
        print(f"File {merged_path} not found!")
        return

    print(f"Loading {merged_path}...")
    with open(merged_path, 'r', encoding='utf-8') as f:
        data = json.load(f)

    visits = [s for s in data.get('semanticSegments', []) if 'visit' in s]
    print(f"Total visits to process: {len(visits)}")

    # 1. Load existing places from places_cache
    existing_places = []
    try:
        out = subprocess.check_output(['turso', 'db', 'shell', 'ierp', 'SELECT place, latitude, longitude, city FROM places_cache;'], text=True)
        for line in out.strip().split('\n')[2:]:
            parts = line.split()
            if len(parts) >= 3:
                try:
                    lat = float(parts[-3])
                    lon = float(parts[-2])
                    city = parts[-1]
                    pname = ' '.join(parts[:-3])
                    existing_places.append({'place': pname, 'lat': lat, 'lon': lon, 'city': city})
                except:
                    pass
    except Exception as e:
        print("Warning: could not fetch existing places:", e)

    print(f"Loaded {len(existing_places)} existing places from places_cache.")

    # 2. Cluster visits within 150m
    clusters = []
    for v in visits:
        top = v['visit'].get('topCandidate', {})
        loc_str = top.get('placeLocation', {}).get('latLng', '')
        if not loc_str:
            continue
        pts = loc_str.split(',')
        lat = float(re.sub(r'[^\d.-]', '', pts[0]))
        lon = float(re.sub(r'[^\d.-]', '', pts[1]))
        st = v.get('startTime', '')
        et = v.get('endTime', '')
        sem = top.get('semanticType', 'UNKNOWN')
        pid = top.get('placeId', '')

        matched = None
        for c in clusters:
            if haversine(lat, lon, c['lat'], c['lon']) < 150:
                matched = c
                break

        visit_record = {'st': st, 'et': et, 'sem': sem, 'pid': pid}
        if matched:
            matched['visits'].append(visit_record)
        else:
            clusters.append({
                'lat': lat,
                'lon': lon,
                'visits': [visit_record],
                'place': None,
                'city': None,
                'display_name': None
            })

    print(f"Formed {len(clusters)} geographic clusters.")

    # 3. Known cluster overrides
    overrides = [
        (-6.2040, 106.8061, "Kost Bendungan Hilir", "Bendungan Hilir, Tanah Abang, Jakarta Pusat", "Jakarta Pusat"),
        (-6.2026, 106.8011, "Dipo Tower, Slipi", "Dipo Tower, Jl. Gatot Subroto, Slipi, Jakarta Pusat", "Jakarta Pusat"),
        (-6.2073, 106.7861, "Kemandoran, Palmerah", "Jl. Kemandoran, Grogol Utara, Kebayoran Lama, Jakarta Selatan", "Jakarta Selatan"),
        (-6.9750, 110.4266, "Gambiran, Semarang Tengah", "Gang Gambiran, Kranggan, Semarang Tengah", "Semarang"),
        (-6.9860, 110.4132, "Pandanaran, Simpang Lima", "Jl. Pandanaran, Simpang Lima, Semarang", "Semarang"),
        (-8.7107, 115.1705, "Legian, Kuta", "Jl. Lebak Bene, Legian, Kuta, Bali", "Bali"),
        (-6.8663, 107.5345, "Cipageran, Cimahi", "Cipageran, Cimahi Utara, Kota Cimahi", "Cimahi"),
    ]

    # 4. Resolve place names for each cluster
    new_places_to_cache = []
    for i, c in enumerate(clusters):
        # Check overrides
        matched_override = False
        for olat, olon, oplace, odisp, ocity in overrides:
            if haversine(c['lat'], c['lon'], olat, olon) < 200:
                c['place'] = oplace
                c['display_name'] = odisp
                c['city'] = ocity
                matched_override = True
                break
        if matched_override:
            new_places_to_cache.append(c)
            continue

        # Check existing places_cache
        closest = None
        min_dist = float('inf')
        for ex in existing_places:
            d = haversine(c['lat'], c['lon'], ex['lat'], ex['lon'])
            if d < min_dist:
                min_dist = d
                closest = ex
        if min_dist <= 250:
            c['place'] = closest['place']
            c['city'] = closest['city']
            c['display_name'] = closest['place']
            continue

        # Reverse geocode via Nominatim
        print(f"[{i+1}/{len(clusters)}] Reverse geocoding ({c['lat']:.4f}, {c['lon']:.4f})...")
        try:
            url = f"https://nominatim.openstreetmap.org/reverse?lat={c['lat']}&lon={c['lon']}&format=json&zoom=18"
            req = urllib.request.Request(url, headers={'User-Agent': 'ierp-dashboard-local/1.0'})
            res = urllib.request.urlopen(req, timeout=5)
            d = json.loads(res.read().decode())
            addr = d.get('address', {})
            name = d.get('name') or addr.get('amenity') or addr.get('shop') or addr.get('building') or addr.get('road') or addr.get('suburb')
            if not name:
                name = f"Location ({c['lat']:.3f}, {c['lon']:.3f})"
            city = addr.get('city') or addr.get('town') or addr.get('county') or addr.get('state_district') or 'Indonesia'
            if 'Jakarta' in city or 'Daerah Khusus' in city:
                city = addr.get('city_district') or addr.get('suburb') or 'Jakarta'
                if not city.startswith('Jakarta'):
                    city = f"Jakarta ({city})"
            elif 'Bandung' in city or 'Kota Bandung' in city:
                city = 'Bandung'
            elif 'Semarang' in city or 'Kota Semarang' in city:
                city = 'Semarang'
            
            c['place'] = name
            c['city'] = city
            c['display_name'] = d.get('display_name', name)[:100]
            print(f"  -> {name} ({city})")
        except Exception as e:
            print(f"  -> Reverse geocode fallback: {e}")
            c['place'] = f"Location ({c['lat']:.4f}, {c['lon']:.4f})"
            c['city'] = "Indonesia"
            c['display_name'] = c['place']
        
        new_places_to_cache.append(c)
        time.sleep(1.0)

    # 5. Insert new places into places_cache in Turso
    print(f"\nInserting {len(new_places_to_cache)} places into places_cache...")
    stmts = []
    for c in new_places_to_cache:
        p_esc = clean_str(c['place'])
        d_esc = clean_str(c['display_name'] or c['place'])
        c_esc = clean_str(c['city'] or 'Indonesia')
        stmts.append(f"INSERT OR REPLACE INTO places_cache (place, display_name, latitude, longitude, city) VALUES ('{p_esc}', '{d_esc}', {c['lat']}, {c['lon']}, '{c_esc}');")

    # Execute in batches of 25
    for batch_i in range(0, len(stmts), 25):
        batch = ' '.join(stmts[batch_i:batch_i+25])
        subprocess.run(['turso', 'db', 'shell', 'ierp', batch], capture_output=True)

    # 6. Insert visits into events table in Turso
    print("\nInserting visits into events table...")
    # Fetch existing event start_dates to avoid duplicates
    existing_dates = set()
    try:
        out = subprocess.check_output(['turso', 'db', 'shell', 'ierp', 'SELECT start_date FROM events WHERE start_date IS NOT NULL;'], text=True)
        for line in out.strip().split('\n')[2:]:
            d_val = line.strip()
            if d_val:
                existing_dates.add(d_val[:19])
    except Exception as e:
        print("Could not query existing dates:", e)

    event_stmts = []
    imported_count = 0
    for c in clusters:
        place = c.get('place')
        city = c.get('city') or 'Indonesia'
        for v in c['visits']:
            st = v.get('st', '')
            et = v.get('et', '')
            st_clean = st[:19] if len(st) >= 19 else st
            et_clean = et[:19] if len(et) >= 19 else et

            if st_clean in existing_dates:
                continue

            sem = v.get('sem', 'UNKNOWN')
            if sem == 'HOME':
                title = f"Stay at {place}"
            elif sem == 'WORK':
                title = f"Work Session at {place}"
            else:
                title = f"Visit to {place}"

            tags = ["timeline", "visit", city.lower().replace(' ', '-')]
            tags_json = json.dumps(tags)
            notes = f"Google Timeline import (Semantic: {sem}, PlaceID: {v.get('pid', '')})"

            t_esc = clean_str(title)
            p_esc = clean_str(place)
            n_esc = clean_str(notes)
            tags_esc = clean_str(tags_json)

            event_stmts.append(f"INSERT INTO events (title, place, start_date, end_date, raw_date, tags, notes) VALUES ('{t_esc}', '{p_esc}', '{st_clean}', '{et_clean}', '{st_clean}', '{tags_esc}', '{n_esc}');")
            existing_dates.add(st_clean)
            imported_count += 1

    print(f"Generated {len(event_stmts)} new timeline events to insert.")
    for batch_i in range(0, len(event_stmts), 25):
        batch = ' '.join(event_stmts[batch_i:batch_i+25])
        subprocess.run(['turso', 'db', 'shell', 'ierp', batch], capture_output=True)

    print(f"\n✅ Finished! Successfully imported {imported_count} new timeline events and cached places into Turso Cloud.")

if __name__ == '__main__':
    main()
