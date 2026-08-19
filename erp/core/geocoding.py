"""
Reverse-geocoding service utilizing OpenStreetMap Nominatim with local persistent caching.
Enforces OSM usage policies (User-Agent header, rate-limiting).
"""

import json
import time
import urllib.parse
import urllib.request
from typing import Optional
from .config import GEO_CACHE_PATH, C_YELLOW, C_RESET

_LAST_LOOKUP_TIME = 0.0


def _load_geo_cache() -> dict:
    if GEO_CACHE_PATH.exists():
        try:
            with open(GEO_CACHE_PATH, "r", encoding="utf-8") as f:
                return json.load(f)
        except Exception:
            return {}
    return {}


def _save_geo_cache(cache: dict) -> None:
    try:
        with open(GEO_CACHE_PATH, "w", encoding="utf-8") as f:
            json.dump(cache, f, indent=2, ensure_ascii=False)
    except Exception:
        pass


def reverse_geocode(lat: float, lon: float) -> Optional[str]:
    """
    Converts latitude/longitude into a clean place/city name using OpenStreetMap Nominatim ($0 cost).
    Results are cached to disk in geocode_cache.json.
    """
    global _LAST_LOOKUP_TIME
    cache_key = f"{round(lat, 4)},{round(lon, 4)}"
    cache = _load_geo_cache()

    if cache_key in cache:
        return cache[cache_key]

    # Enforce Nominatim 1-second rate limit between remote API calls
    elapsed = time.time() - _LAST_LOOKUP_TIME
    if elapsed < 1.0:
        time.sleep(1.0 - elapsed)

    url = f"https://nominatim.openstreetmap.org/reverse?lat={lat}&lon={lon}&format=json&zoom=16&addressdetails=1"
    req = urllib.request.Request(url)
    req.add_header("User-Agent", "PersonalERP-EventLog/2.0 (muhammad.ichsanul19@gmail.com)")
    req.add_header("Accept", "application/json")

    place_name = None
    try:
        _LAST_LOOKUP_TIME = time.time()
        with urllib.request.urlopen(req, timeout=8) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            addr = data.get("address", {})
            venue = addr.get("building") or addr.get("amenity") or addr.get("shop") or addr.get("office") or addr.get("tourism")
            area = addr.get("suburb") or addr.get("neighbourhood") or addr.get("quarter")
            city = addr.get("city") or addr.get("town") or addr.get("regency") or addr.get("municipality") or addr.get("county")

            if venue and city:
                place_name = f"{venue}, {city}"
            elif venue and area:
                place_name = f"{venue}, {area}"
            elif area and city:
                place_name = f"{area}, {city}"
            elif data.get("name"):
                place_name = f"{data['name']}, {city}" if city else data["name"]
            elif data.get("display_name"):
                parts = [p.strip() for p in data["display_name"].split(",")]
                place_name = ", ".join(parts[:3])
            else:
                place_name = city or f"GPS ({round(lat, 4)}, {round(lon, 4)})"
    except Exception as e:
        print(f"{C_YELLOW}Geocoding warning ({e}), falling back to coordinates.{C_RESET}")
        place_name = f"GPS ({round(lat, 4)}, {round(lon, 4)})"

    if place_name:
        cache[cache_key] = place_name
        _save_geo_cache(cache)

    return place_name
