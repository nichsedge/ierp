"""
Zero-dependency web dashboard server and real-time OwnTracks GPS webhook receiver.
Features an interactive dark-mode dashboard with statistics, charts, search, and contact management.
"""

import http.server
import json
import os
import socket
import urllib.parse
import webbrowser
from datetime import datetime
from typing import Dict, Any

from .config import DB_PATH, MEDIA_DIR, C_BOLD, C_GREEN, C_CYAN, C_MAGENTA, C_YELLOW, C_RESET
from .db import get_db, init_db
from .geocoding import reverse_geocode
from .google_sync import sync_google_contacts
from .merging import auto_merge_contacts, merge_two_contacts

DASHBOARD_HTML = """<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Personal Journal & CRM ERP</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
<style>
:root {
  --bg-primary: #090d16;
  --bg-card: rgba(18, 24, 38, 0.7);
  --bg-card-hover: rgba(28, 36, 56, 0.85);
  --accent-cyan: #00f2fe;
  --accent-blue: #4facfe;
  --accent-purple: #8a2be2;
  --accent-pink: #f43f5e;
  --accent-green: #10b981;
  --text-main: #f3f4f6;
  --text-muted: #9ca3af;
  --card-border: rgba(255, 255, 255, 0.08);
  --glow-shadow: 0 8px 32px 0 rgba(0, 242, 254, 0.08);
}

* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: 'Outfit', sans-serif;
  background-color: var(--bg-primary);
  background-image: 
    radial-gradient(at 0% 0%, rgba(79, 172, 254, 0.12) 0px, transparent 50%),
    radial-gradient(at 100% 100%, rgba(138, 43, 226, 0.12) 0px, transparent 50%);
  background-attachment: fixed;
  color: var(--text-main);
  min-height: 100vh;
  padding: 30px 20px;
}

.container { max-width: 1280px; margin: 0 auto; }

header {
  display: flex; justify-content: space-between; align-items: center;
  margin-bottom: 30px; padding-bottom: 20px;
  border-bottom: 1px solid var(--card-border);
}

.logo-group { display: flex; align-items: center; gap: 14px; }
.logo-icon {
  width: 44px; height: 44px; border-radius: 12px;
  background: linear-gradient(135deg, var(--accent-blue), var(--accent-purple));
  display: flex; align-items: center; justify-content: center;
  font-size: 22px; font-weight: 700; color: #fff;
  box-shadow: 0 4px 15px rgba(79, 172, 254, 0.4);
}
h1 { font-size: 24px; font-weight: 700; letter-spacing: -0.5px; }

.status-badge {
  display: inline-flex; align-items: center; gap: 8px;
  padding: 6px 14px; border-radius: 20px;
  background: rgba(16, 185, 129, 0.1); border: 1px solid rgba(16, 185, 129, 0.3);
  color: var(--accent-green); font-size: 13px; font-weight: 500;
}
.pulse {
  width: 8px; height: 8px; border-radius: 50%;
  background: var(--accent-green);
  box-shadow: 0 0 8px var(--accent-green);
  animation: pulse 2s infinite;
}
@keyframes pulse { 0% { opacity: 0.4; } 50% { opacity: 1; } 100% { opacity: 0.4; } }

.nav-tabs { display: flex; gap: 10px; margin-bottom: 25px; }
.tab-btn {
  padding: 10px 20px; border-radius: 12px; border: 1px solid var(--card-border);
  background: var(--bg-card); color: var(--text-muted); font-size: 14px; font-weight: 600;
  cursor: pointer; transition: all 0.2s ease;
}
.tab-btn:hover { background: var(--bg-card-hover); color: var(--text-main); }
.tab-btn.active {
  background: linear-gradient(135deg, rgba(79, 172, 254, 0.2), rgba(138, 43, 226, 0.2));
  border-color: var(--accent-blue); color: var(--accent-cyan);
}

.stats-grid {
  display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px; margin-bottom: 30px;
}
.stat-card {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 16px; padding: 20px; backdrop-filter: blur(12px);
  position: relative; overflow: hidden;
}
.stat-card::before {
  content: ''; position: absolute; top: 0; left: 0; width: 100%; height: 3px;
  background: linear-gradient(90deg, var(--accent-cyan), var(--accent-purple));
}
.stat-label { font-size: 13px; color: var(--text-muted); text-transform: uppercase; font-weight: 600; margin-bottom: 6px; }
.stat-value { font-size: 32px; font-weight: 700; color: #fff; }
.stat-icon { position: absolute; right: 20px; top: 20px; font-size: 28px; opacity: 0.3; }

.charts-grid { display: grid; grid-template-columns: 2fr 1fr; gap: 20px; margin-bottom: 30px; }
@media (max-width: 900px) { .charts-grid { grid-template-columns: 1fr; } }

.panel {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 18px; padding: 24px; backdrop-filter: blur(12px);
}
.panel-title { font-size: 16px; font-weight: 600; margin-bottom: 20px; display: flex; align-items: center; justify-content: space-between; }

.bar-chart { display: flex; align-items: flex-end; gap: 12px; height: 180px; padding-top: 20px; }
.chart-bar-col { flex: 1; display: flex; flex-direction: column; align-items: center; gap: 8px; height: 100%; }
.bar-wrapper { width: 100%; height: 100%; display: flex; align-items: flex-end; justify-content: center; }
.bar-fill {
  width: 70%; max-width: 24px; border-radius: 6px 6px 0 0;
  background: linear-gradient(180deg, var(--accent-cyan), var(--accent-blue));
  transition: height 0.5s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
}
.bar-fill:hover::after {
  content: attr(data-count);
  position: absolute; top: -25px; left: 50%; transform: translateX(-50%);
  background: rgba(0,0,0,0.8); padding: 2px 6px; border-radius: 4px; font-size: 11px; color: #fff;
}
.bar-label { font-size: 11px; color: var(--text-muted); font-family: 'JetBrains Mono', monospace; }

.tag-cloud { display: flex; flex-wrap: wrap; gap: 8px; }
.tag-pill {
  padding: 6px 12px; border-radius: 20px; font-size: 12px; font-weight: 500;
  background: rgba(255, 255, 255, 0.05); border: 1px solid var(--card-border);
  color: var(--text-main); cursor: pointer; transition: all 0.2s;
  display: flex; align-items: center; gap: 6px;
}
.tag-pill:hover, .tag-pill.active { background: var(--accent-blue); color: #fff; border-color: var(--accent-blue); }
.tag-count { font-size: 10px; background: rgba(0, 0, 0, 0.3); padding: 1px 6px; border-radius: 10px; }

.search-box { margin-bottom: 20px; position: relative; }
.search-input {
  width: 100%; padding: 14px 20px; border-radius: 14px;
  background: var(--bg-card); border: 1px solid var(--card-border);
  color: #fff; font-size: 15px; outline: none; transition: border-color 0.2s;
}
.search-input:focus { border-color: var(--accent-blue); box-shadow: 0 0 15px rgba(79, 172, 254, 0.2); }

.events-grid { display: flex; flex-direction: column; gap: 12px; }
.event-card {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 14px; padding: 18px 22px; cursor: pointer;
  transition: all 0.2s ease;
}
.event-card:hover {
  background: var(--bg-card-hover);
  border-color: rgba(255, 255, 255, 0.2);
  transform: translateY(-2px);
}
.event-header { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 8px; }
.event-title { font-size: 17px; font-weight: 600; color: #fff; }
.event-date { font-size: 13px; color: var(--accent-cyan); font-family: 'JetBrains Mono', monospace; }
.event-meta { display: flex; gap: 15px; font-size: 13px; color: var(--text-muted); margin-bottom: 8px; }
.meta-item { display: flex; align-items: center; gap: 5px; }
.event-notes-preview { font-size: 14px; color: var(--text-muted); line-height: 1.5; white-space: pre-wrap; max-height: 4.5em; overflow: hidden; }

.contacts-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 16px; }
.contact-card {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 14px; padding: 18px; transition: transform 0.2s;
}
.contact-card:hover { transform: translateY(-2px); border-color: rgba(255, 255, 255, 0.2); }
.contact-name { font-size: 16px; font-weight: 600; margin-bottom: 4px; }
.contact-org { font-size: 13px; color: var(--accent-purple); margin-bottom: 8px; }
.contact-badge {
  display: inline-block; background: rgba(79, 172, 254, 0.1);
  color: var(--accent-blue); padding: 4px 10px; border-radius: 12px;
  font-size: 12px; font-weight: 500;
}

.modal-overlay {
  position: fixed; top: 0; left: 0; right: 0; bottom: 0;
  background: rgba(0, 0, 0, 0.7); backdrop-filter: blur(8px);
  display: none; align-items: center; justify-content: center;
  padding: 20px; z-index: 1000;
}
.modal-content {
  background: #111827; border: 1px solid var(--card-border);
  border-radius: 20px; max-width: 650px; width: 100%;
  max-height: 85vh; overflow-y: auto; padding: 30px;
  position: relative; color: var(--text-main);
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.5);
}
.close-btn {
  position: absolute; top: 20px; right: 20px;
  background: rgba(255, 255, 255, 0.1); border: none; color: #fff;
  width: 32px; height: 32px; border-radius: 50%; cursor: pointer;
  font-size: 18px; display: flex; align-items: center; justify-content: center;
}
.close-btn:hover { background: rgba(255, 255, 255, 0.2); }
.modal-body { margin-top: 20px; font-size: 15px; line-height: 1.6; white-space: pre-wrap; }
.media-img { max-width: 100%; border-radius: 12px; margin-top: 15px; border: 1px solid var(--card-border); }
</style>
</head>
<body>
<div class="container">
  <header>
    <div class="logo-group">
      <div class="logo-icon">E</div>
      <div>
        <h1>Journal & CRM Dashboard</h1>
        <p style="font-size: 13px; color: var(--text-muted); margin-top: 2px;">Personal Event Log System</p>
      </div>
    </div>
    <div class="status-badge">
      <div class="pulse"></div>
      Database Connected
    </div>
  </header>

  <div class="nav-tabs">
    <button class="tab-btn active" onclick="switchTab('overview', event)">Overview</button>
    <button class="tab-btn" onclick="switchTab('events', event)">Timeline Events</button>
    <button class="tab-btn" onclick="switchTab('contacts', event)">CRM Contacts</button>
  </div>

  <!-- OVERVIEW TAB -->
  <div id="tab-overview">
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-label">Total Events</div>
        <div class="stat-value" id="stat-events">-</div>
        <div class="stat-icon">📅</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Registered Contacts</div>
        <div class="stat-value" id="stat-contacts">-</div>
        <div class="stat-icon">👥</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Linked Interactions</div>
        <div class="stat-value" id="stat-links">-</div>
        <div class="stat-icon">🔗</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Media Files</div>
        <div class="stat-value" id="stat-media">-</div>
        <div class="stat-icon">🖼️</div>
      </div>
    </div>

    <div class="charts-grid">
      <div class="panel">
        <div class="panel-title">Monthly Event Activity</div>
        <div class="bar-chart" id="monthly-chart"></div>
      </div>
      <div class="panel">
        <div class="panel-title">Top Categories / Tags</div>
        <div class="tag-cloud" id="top-tags"></div>
      </div>
    </div>
  </div>

  <!-- TIMELINE EVENTS TAB -->
  <div id="tab-events" style="display:none;">
    <div class="search-box">
      <input type="text" id="event-search" class="search-input" placeholder="Search events by title, location, or notes..." oninput="debounceSearch()">
    </div>
    <div class="events-grid" id="events-list"></div>
  </div>

  <!-- CRM CONTACTS TAB -->
  <div id="tab-contacts" style="display:none;">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; flex-wrap: wrap; gap: 12px;">
      <div style="display: flex; gap: 8px;" id="contact-source-filters">
        <button class="tag-pill active" onclick="filterContactsBySource('', this)">All Contacts</button>
        <button class="tag-pill" onclick="filterContactsBySource('manual', this)">Manual (CRM)</button>
        <button class="tag-pill" onclick="filterContactsBySource('google', this)">Google Synced</button>
        <button class="tag-pill" onclick="filterContactsBySource('merged', this)">Merged</button>
      </div>
      <button class="tag-pill" style="background: linear-gradient(135deg, var(--accent-purple), var(--accent-pink)); color:#fff; border:none; cursor:pointer; font-weight:600; padding:8px 16px;" onclick="triggerAutoMerge()">⚡ Auto-Merge Duplicates</button>
    </div>
    <div class="search-box">
      <input type="text" id="contact-search" class="search-input" placeholder="Search contacts by name, organization, client, email, or phone..." oninput="debounceContactSearch()">
    </div>
    <div class="contacts-grid" id="contacts-list"></div>
  </div>
</div>

<!-- EVENT DETAIL MODAL -->
<div class="modal-overlay" id="event-modal">
  <div class="modal-content">
    <button class="close-btn" onclick="closeModal()">×</button>
    <h2 id="modal-title" style="margin-bottom: 8px;"></h2>
    <div id="modal-meta" style="color: var(--accent-cyan); font-size: 14px; margin-bottom: 16px;"></div>
    <div id="modal-tags" style="display:flex; gap:6px; margin-bottom:16px;"></div>
    <div id="modal-people" style="color: var(--accent-purple); font-size: 14px; margin-bottom: 16px;"></div>
    <hr style="border: 0; border-top: 1px solid var(--card-border);">
    <div class="modal-body" id="modal-body"></div>
    <div id="modal-media"></div>
  </div>
</div>

<script>
let currentTag = '';
let searchTimeout = null;
let currentContactSource = '';
let contactSearchTimeout = null;

async function loadStats() {
  const res = await fetch('/api/stats');
  const data = await res.json();
  
  document.getElementById('stat-events').innerText = data.total_events;
  document.getElementById('stat-contacts').innerText = data.total_contacts;
  document.getElementById('stat-links').innerText = data.total_links;
  document.getElementById('stat-media').innerText = data.total_media;

  const chartEl = document.getElementById('monthly-chart');
  chartEl.innerHTML = '';
  const maxCount = Math.max(...data.monthly_counts.map(m => m.count), 1);
  
  data.monthly_counts.slice().reverse().forEach(m => {
    const col = document.createElement('div');
    col.className = 'chart-bar-col';
    const heightPct = (m.count / maxCount) * 100;
    col.innerHTML = `
      <div class="bar-wrapper">
        <div class="bar-fill" style="height: ${heightPct}%" data-count="${m.count}"></div>
      </div>
      <div class="bar-label">${m.year_month}</div>
    `;
    chartEl.appendChild(col);
  });

  const tagsEl = document.getElementById('top-tags');
  tagsEl.innerHTML = '';
  data.top_tags.forEach(([tag, count]) => {
    const btn = document.createElement('div');
    btn.className = `tag-pill ${currentTag === tag ? 'active' : ''}`;
    btn.innerHTML = `${tag} <span class="tag-count">${count}</span>`;
    btn.onclick = () => filterByTag(tag);
    tagsEl.appendChild(btn);
  });
}

async function loadEvents(query = '', tag = '') {
  const params = new URLSearchParams({ q: query, tag: tag, limit: 50 });
  const res = await fetch('/api/events?' + params.toString());
  const events = await res.json();
  
  const listEl = document.getElementById('events-list');
  listEl.innerHTML = '';
  
  if (events.length === 0) {
    listEl.innerHTML = '<div style="color: var(--text-muted); text-align: center; padding: 40px;">No matching events found.</div>';
    return;
  }

  events.forEach(ev => {
    const card = document.createElement('div');
    card.className = 'event-card';
    card.onclick = () => showEventModal(ev.id);
    
    const tagsHtml = (ev.tags || []).map(t => `<span class="tag-pill" style="font-size:11px; padding:3px 8px;">${t}</span>`).join('');
    const peopleHtml = (ev.linked_contacts || []).map(c => `👤 ${c.name}`).join(', ');

    card.innerHTML = `
      <div class="event-header">
        <div class="event-title">${ev.title}</div>
        <div class="event-date">${ev.start_date || 'No Date'}</div>
      </div>
      <div class="event-meta">
        ${ev.place ? `<div class="meta-item">📍 ${ev.place}</div>` : ''}
        ${peopleHtml ? `<div class="meta-item" style="color:var(--accent-purple);">${peopleHtml}</div>` : ''}
      </div>
      <div style="display:flex; gap:6px; margin-bottom:10px;">${tagsHtml}</div>
      <div class="event-notes-preview">${ev.notes || ''}</div>
    `;
    listEl.appendChild(card);
  });
}

async function loadContacts(source = '', query = '') {
  currentContactSource = source;
  const params = new URLSearchParams();
  if (source) params.append('source', source);
  if (query) params.append('q', query);
  
  const res = await fetch('/api/contacts?' + params.toString());
  const contacts = await res.json();
  
  const listEl = document.getElementById('contacts-list');
  listEl.innerHTML = '';

  if (contacts.length === 0) {
    listEl.innerHTML = '<div style="color: var(--text-muted); text-align: center; padding: 40px; grid-column: 1 / -1;">No matching contacts found.</div>';
    return;
  }

  contacts.forEach(c => {
    const card = document.createElement('div');
    card.className = 'contact-card';
    
    let badgeColor = 'var(--accent-blue)';
    let badgeText = (c.source || 'manual').toUpperCase();
    if (c.source === 'google') { badgeColor = 'var(--accent-green)'; }
    if (c.source === 'merged') { badgeColor = 'var(--accent-pink)'; }

    card.innerHTML = `
      <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:8px;">
        <div class="contact-name">${c.name}</div>
        <span style="font-size:10px; font-weight:700; padding:2px 8px; border-radius:10px; background:rgba(255,255,255,0.06); color:${badgeColor}; border:1px solid ${badgeColor};">${badgeText}</span>
      </div>
      ${c.org ? `<div class="contact-org">🏢 ${c.org} ${c.client ? `(${c.client})` : ''}</div>` : ''}
      ${c.email ? `<div style="font-size:13px; color:var(--text-muted); margin-bottom:4px;">✉️ ${c.email}</div>` : ''}
      ${c.phone ? `<div style="font-size:13px; color:var(--text-muted); margin-bottom:4px;">📞 ${c.phone}</div>` : ''}
      ${c.location ? `<div style="font-size:13px; color:var(--text-muted); margin-bottom:8px;">📍 ${c.location}</div>` : ''}
      <div class="contact-badge">${c.event_count} Linked Events</div>
    `;
    listEl.appendChild(card);
  });
}

function filterContactsBySource(source, btn) {
  document.querySelectorAll('#contact-source-filters .tag-pill').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  const q = document.getElementById('contact-search') ? document.getElementById('contact-search').value : '';
  loadContacts(source, q);
}

function debounceContactSearch() {
  clearTimeout(contactSearchTimeout);
  contactSearchTimeout = setTimeout(() => {
    const q = document.getElementById('contact-search').value;
    loadContacts(currentContactSource, q);
  }, 250);
}

async function triggerAutoMerge() {
  if (!confirm('Scan and automatically merge duplicate contacts between manual CRM and Google Contacts?')) return;
  const res = await fetch('/api/contacts/merge', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({ auto: true }) });
  const data = await res.json();
  alert(`Auto-merge complete!\\nMerged ${data.merged_count} contact(s).`);
  loadContacts(currentContactSource);
  loadStats();
}

async function showEventModal(eventId) {
  const res = await fetch('/api/events/' + eventId);
  const ev = await res.json();
  
  document.getElementById('modal-title').innerText = ev.title;
  document.getElementById('modal-meta').innerText = `${ev.start_date || 'No Date'} ${ev.place ? ' • 📍 ' + ev.place : ''}`;
  document.getElementById('modal-tags').innerHTML = (ev.tags || []).map(t => `<span class="tag-pill">${t}</span>`).join('');
  document.getElementById('modal-people').innerText = (ev.contacts || []).map(c => `👤 ${c.name} (${c.org || 'Contact'})`).join(', ');
  document.getElementById('modal-body').innerText = ev.notes || 'No description notes available.';
  
  const mediaEl = document.getElementById('modal-media');
  mediaEl.innerHTML = '';
  if (ev.media && ev.media.length > 0) {
    ev.media.forEach(m => {
      const img = document.createElement('img');
      img.src = '/' + m.stored_path;
      img.className = 'media-img';
      mediaEl.appendChild(img);
    });
  }

  document.getElementById('event-modal').style.display = 'flex';
}

function closeModal() {
  document.getElementById('event-modal').style.display = 'none';
}

function switchTab(tabName, evt) {
  document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
  if (evt && evt.target) evt.target.classList.add('active');
  
  document.getElementById('tab-overview').style.display = tabName === 'overview' ? 'block' : 'none';
  document.getElementById('tab-events').style.display = tabName === 'events' ? 'block' : 'none';
  document.getElementById('tab-contacts').style.display = tabName === 'contacts' ? 'block' : 'none';

  if (tabName === 'events') loadEvents();
  if (tabName === 'contacts') loadContacts();
}

function filterByTag(tag) {
  currentTag = currentTag === tag ? '' : tag;
  const eventTabBtn = document.querySelectorAll('.tab-btn')[1];
  switchTab('events', { target: eventTabBtn });
  loadEvents('', currentTag);
}

function debounceSearch() {
  clearTimeout(searchTimeout);
  searchTimeout = setTimeout(() => {
    const q = document.getElementById('event-search').value;
    loadEvents(q, currentTag);
  }, 250);
}

loadStats();
loadEvents();
</script>
</body>
</html>
"""


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
        notes = f"OwnTracks Geofence Transition\\nLat: {lat}, Lon: {lon}\\nPlace: {place_name}"
    else:
        title = f"Location Ping: {place_name or 'Unknown Place'}"
        tags = ["location", "gps", "owntracks"]
        notes = f"Background GPS Ping\\nAccuracy: {data.get('acc', 'N/A')}m, Battery: {data.get('batt', 'N/A')}%, Speed: {data.get('vel', 'N/A')}km/h"

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
        self.wfile.write(json.dumps(data).encode("utf-8"))

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

            conn.close()

            res = {
                "total_events": total_events,
                "total_contacts": total_contacts,
                "total_media": total_media,
                "total_links": total_links,
                "top_tags": top_tags,
                "monthly_counts": monthly_counts
            }
            self.send_json(res)

        elif path == "/api/events":
            q = query.get("q", [""])[0].strip()
            tag_filter = query.get("tag", [""])[0].strip()
            limit = int(query.get("limit", [50])[0])

            conn = get_db()
            cursor = conn.cursor()

            sql = "SELECT id, title, place, start_date, end_date, raw_date, tags, notes FROM events WHERE 1=1"
            params = []

            if q:
                sql += " AND (title LIKE ? OR place LIKE ? OR notes LIKE ?)"
                like_q = f"%{q}%"
                params.extend([like_q, like_q, like_q])

            if tag_filter:
                sql += " AND tags LIKE ?"
                params.append(f"%{tag_filter}%")

            sql += " ORDER BY start_date DESC LIMIT ?"
            params.append(limit)

            rows = cursor.execute(sql, params).fetchall()

            events = []
            for r in rows:
                ev_id, title, place, start, end, raw_d, tags_json, notes = r
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
                    "linked_contacts": [{"id": cid, "name": cname} for cid, cname in linked_contacts]
                })

            conn.close()
            self.send_json(events)

        elif path.startswith("/api/events/"):
            try:
                ev_id = int(path.split("/")[-1])
            except ValueError:
                self.send_error(400, "Invalid event ID")
                return

            conn = get_db()
            cursor = conn.cursor()

            row = cursor.execute("""
                SELECT id, title, place, start_date, end_date, raw_date, tags, url, notes FROM events WHERE id = ?
            """, (ev_id,)).fetchone()

            if not row:
                conn.close()
                self.send_error(404, "Event not found")
                return

            eid, title, place, start, end, raw_d, tags_json, url, notes = row
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
                "contacts": [{"id": c[0], "name": c[1], "org": c[2]} for c in contacts],
                "media": [{"id": m[0], "original_filename": m[1], "stored_path": m[2]} for m in media]
            }
            self.send_json(res)

        elif path == "/api/contacts":
            source_filter = query.get("source", [""])[0].strip().lower()
            q = query.get("q", [""])[0].strip()

            conn = get_db()
            cursor = conn.cursor()

            sql = """
                SELECT c.id, c.name, c.org, c.client, c.location, c.notes, c.email, c.phone, c.source, c.google_id,
                       (SELECT COUNT(*) FROM event_contacts ec WHERE ec.contact_id = c.id) as cnt
                FROM contacts c
                WHERE 1=1
            """
            params = []
            if source_filter and source_filter != "all":
                sql += " AND LOWER(c.source) = ?"
                params.append(source_filter)
            if q:
                sql += " AND (c.name LIKE ? OR c.org LIKE ? OR c.client LIKE ? OR c.email LIKE ? OR c.phone LIKE ?)"
                like_q = f"%{q}%"
                params.extend([like_q, like_q, like_q, like_q, like_q])

            sql += " ORDER BY cnt DESC, c.name ASC"
            rows = cursor.execute(sql, params).fetchall()
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
                "event_count": r[10]
            } for r in rows]

            self.send_json(contacts)

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
