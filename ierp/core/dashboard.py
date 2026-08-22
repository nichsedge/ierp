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
<link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700&family=JetBrains+Mono:wght@400;500;600&display=swap" rel="stylesheet">
<style>
:root {
  --bg-primary: #090d16;
  --bg-card: rgba(18, 24, 38, 0.75);
  --bg-card-hover: rgba(28, 36, 56, 0.9);
  --accent-cyan: #00f2fe;
  --accent-blue: #4facfe;
  --accent-purple: #8a2be2;
  --accent-pink: #f43f5e;
  --accent-green: #10b981;
  --accent-amber: #f59e0b;
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
  padding: 24px 20px;
}

.container { max-width: 1320px; margin: 0 auto; }

header {
  display: flex; justify-content: space-between; align-items: center;
  margin-bottom: 24px; padding-bottom: 18px;
  border-bottom: 1px solid var(--card-border);
  flex-wrap: wrap; gap: 16px;
}

.logo-group { display: flex; align-items: center; gap: 14px; }
.logo-icon {
  width: 44px; height: 44px; border-radius: 12px;
  background: linear-gradient(135deg, var(--accent-blue), var(--accent-purple));
  display: flex; align-items: center; justify-content: center;
  font-size: 22px; font-weight: 700; color: #fff;
  box-shadow: 0 4px 15px rgba(79, 172, 254, 0.4);
}
h1 { font-size: 23px; font-weight: 700; letter-spacing: -0.5px; }

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

.nav-tabs { display: flex; gap: 8px; margin-bottom: 22px; flex-wrap: wrap; }
.tab-btn {
  padding: 9px 16px; border-radius: 12px; border: 1px solid var(--card-border);
  background: var(--bg-card); color: var(--text-muted); font-size: 13.5px; font-weight: 600;
  cursor: pointer; transition: all 0.2s ease; display: inline-flex; align-items: center; gap: 6px;
}
.tab-btn:hover { background: var(--bg-card-hover); color: var(--text-main); }
.tab-btn.active {
  background: linear-gradient(135deg, rgba(79, 172, 254, 0.2), rgba(138, 43, 226, 0.2));
  border-color: var(--accent-blue); color: var(--accent-cyan);
}

.stats-grid {
  display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px; margin-bottom: 25px;
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
.stat-label { font-size: 12px; color: var(--text-muted); text-transform: uppercase; font-weight: 600; margin-bottom: 6px; }
.stat-value { font-size: 30px; font-weight: 700; color: #fff; }
.stat-icon { position: absolute; right: 20px; top: 20px; font-size: 26px; opacity: 0.35; }

.charts-grid { display: grid; grid-template-columns: 2fr 1fr; gap: 20px; margin-bottom: 30px; }
@media (max-width: 900px) { .charts-grid { grid-template-columns: 1fr; } }

.panel {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 18px; padding: 24px; backdrop-filter: blur(12px);
}
.panel-title { font-size: 16px; font-weight: 600; margin-bottom: 20px; display: flex; align-items: center; justify-content: space-between; }

.bar-chart { display: flex; align-items: flex-end; gap: 10px; height: 180px; padding-top: 20px; }
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
  background: rgba(0,0,0,0.85); padding: 2px 6px; border-radius: 4px; font-size: 11px; color: #fff;
  white-space: nowrap;
}
.bar-label { font-size: 11px; color: var(--text-muted); font-family: 'JetBrains Mono', monospace; }

.tag-cloud { display: flex; flex-wrap: wrap; gap: 8px; }
.tag-pill {
  padding: 6px 12px; border-radius: 20px; font-size: 12px; font-weight: 500;
  background: rgba(255, 255, 255, 0.05); border: 1px solid var(--card-border);
  color: var(--text-main); cursor: pointer; transition: all 0.2s;
  display: flex; align-items: center; gap: 6px; user-select: none;
}
.tag-pill:hover, .tag-pill.active { background: var(--accent-blue); color: #fff; border-color: var(--accent-blue); }
.tag-count { font-size: 10px; background: rgba(0, 0, 0, 0.35); padding: 1px 6px; border-radius: 10px; }

/* Filter & Controls Toolbar */
.toolbar-section {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 16px; padding: 16px 20px; margin-bottom: 20px;
  display: flex; flex-direction: column; gap: 14px;
}
.filter-row {
  display: flex; align-items: center; justify-content: space-between;
  flex-wrap: wrap; gap: 12px;
}
.filter-group { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.search-input-wrap { flex: 1; min-width: 260px; position: relative; }
.search-input {
  width: 100%; padding: 10px 16px; border-radius: 10px;
  background: rgba(0, 0, 0, 0.35); border: 1px solid var(--card-border);
  color: #fff; font-size: 14px; outline: none; transition: all 0.2s;
}
.search-input:focus { border-color: var(--accent-blue); box-shadow: 0 0 12px rgba(79, 172, 254, 0.2); }

.date-input {
  background: rgba(0, 0, 0, 0.35); border: 1px solid var(--card-border);
  color: var(--text-main); padding: 8px 12px; border-radius: 10px;
  font-size: 13px; font-family: 'JetBrains Mono', monospace; outline: none;
}
.date-input:focus { border-color: var(--accent-blue); }

.control-select {
  background: rgba(0, 0, 0, 0.35); border: 1px solid var(--card-border);
  color: var(--text-main); padding: 8px 12px; border-radius: 10px;
  font-size: 13px; outline: none; cursor: pointer;
}
.control-select option { background: #111827; color: #fff; }

.count-badge {
  font-size: 12.5px; font-family: 'JetBrains Mono', monospace;
  color: var(--accent-cyan); background: rgba(0, 242, 254, 0.08);
  border: 1px solid rgba(0, 242, 254, 0.25); padding: 4px 10px; border-radius: 12px;
}

.sort-bar {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
  font-size: 12.5px; color: var(--text-muted);
}
.sort-btn {
  background: rgba(255, 255, 255, 0.04); border: 1px solid var(--card-border);
  color: var(--text-muted); padding: 5px 10px; border-radius: 8px;
  font-size: 12px; cursor: pointer; transition: all 0.2s; display: inline-flex; align-items: center; gap: 4px;
}
.sort-btn:hover { background: rgba(255, 255, 255, 0.08); color: var(--text-main); }
.sort-btn.active {
  background: rgba(79, 172, 254, 0.15); border-color: var(--accent-blue);
  color: var(--accent-cyan); font-weight: 600;
}

/* Grids & Cards */
.events-grid { display: flex; flex-direction: column; gap: 12px; }
.event-card {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 14px; padding: 18px 22px; cursor: pointer;
  transition: all 0.2s ease;
}
.event-card:hover {
  background: var(--bg-card-hover); border-color: rgba(255, 255, 255, 0.2);
  transform: translateY(-2px);
}
.event-header { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 8px; }
.event-title { font-size: 16.5px; font-weight: 600; color: #fff; }
.event-date { font-size: 12.5px; color: var(--accent-cyan); font-family: 'JetBrains Mono', monospace; }
.event-meta { display: flex; gap: 15px; font-size: 13px; color: var(--text-muted); margin-bottom: 8px; flex-wrap: wrap; }
.meta-item { display: flex; align-items: center; gap: 5px; }
.event-notes-preview { font-size: 13.5px; color: var(--text-muted); line-height: 1.5; white-space: pre-wrap; max-height: 4.5em; overflow: hidden; }

.contacts-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.contact-card {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 14px; padding: 18px; transition: transform 0.2s, border-color 0.2s;
}
.contact-card:hover { transform: translateY(-2px); border-color: rgba(255, 255, 255, 0.2); }
.contact-name { font-size: 16px; font-weight: 600; margin-bottom: 4px; }
.contact-org { font-size: 13px; color: var(--accent-purple); margin-bottom: 8px; }
.contact-badge {
  display: inline-block; background: rgba(79, 172, 254, 0.1);
  color: var(--accent-blue); padding: 4px 10px; border-radius: 12px;
  font-size: 12px; font-weight: 500;
}

/* Media Cards */
.media-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 16px; }
.media-card {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 14px; padding: 18px; transition: transform 0.2s, border-color 0.2s;
  display: flex; flex-direction: column; justify-content: space-between; gap: 12px;
}
.media-card:hover { transform: translateY(-2px); border-color: rgba(255, 255, 255, 0.2); }
.media-title { font-size: 15.5px; font-weight: 600; color: #fff; line-height: 1.35; }
.media-type-badge {
  font-size: 11px; font-weight: 600; text-transform: uppercase;
  padding: 3px 8px; border-radius: 8px; background: rgba(138, 43, 226, 0.15);
  color: var(--accent-purple); border: 1px solid rgba(138, 43, 226, 0.3);
}
.media-rating {
  color: var(--accent-amber); font-weight: 600; font-size: 13px;
  display: inline-flex; align-items: center; gap: 4px;
}

/* Vendors Cards */
.vendors-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 16px; }
.vendor-card {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 14px; padding: 18px; transition: all 0.2s; position: relative;
}
.vendor-card:hover { transform: translateY(-2px); border-color: rgba(255, 255, 255, 0.2); }
.vendor-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 8px; }
.vendor-name { font-size: 16.5px; font-weight: 600; color: #fff; }
.fav-btn {
  background: none; border: none; font-size: 20px; cursor: pointer;
  color: rgba(255, 255, 255, 0.2); transition: transform 0.15s, color 0.15s; padding: 0 4px;
}
.fav-btn:hover { transform: scale(1.25); }
.fav-btn.is-fav { color: var(--accent-amber); text-shadow: 0 0 10px rgba(245, 158, 11, 0.5); }

/* Links Cards */
.links-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.link-card {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 14px; padding: 16px; transition: all 0.2s;
  display: flex; flex-direction: column; justify-content: space-between; gap: 10px;
}
.link-card:hover { transform: translateY(-2px); border-color: rgba(255, 255, 255, 0.2); }
.link-url {
  font-size: 12.5px; color: var(--accent-blue); word-break: break-all;
  text-decoration: none; display: inline-flex; align-items: center; gap: 4px;
}
.link-url:hover { text-decoration: underline; color: var(--accent-cyan); }

/* Commerce / Subtabs */
.commerce-subtabs { display: flex; gap: 8px; margin-bottom: 16px; }
.subtab-btn {
  padding: 7px 14px; border-radius: 10px; border: 1px solid var(--card-border);
  background: rgba(255, 255, 255, 0.04); color: var(--text-muted); font-size: 13px;
  font-weight: 600; cursor: pointer; transition: all 0.2s;
}
.subtab-btn.active {
  background: rgba(79, 172, 254, 0.15); border-color: var(--accent-blue); color: var(--accent-cyan);
}
.commerce-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 16px; }
.commerce-card {
  background: var(--bg-card); border: 1px solid var(--card-border);
  border-radius: 14px; padding: 18px; transition: all 0.2s;
}
.commerce-card:hover { transform: translateY(-2px); border-color: rgba(255, 255, 255, 0.2); }
.copy-box {
  background: rgba(0, 0, 0, 0.35); border: 1px solid var(--card-border);
  border-radius: 8px; padding: 6px 10px; font-family: 'JetBrains Mono', monospace;
  font-size: 12.5px; color: var(--accent-cyan); display: flex; justify-content: space-between;
  align-items: center; margin: 8px 0;
}
.copy-btn {
  background: rgba(255, 255, 255, 0.08); border: 1px solid var(--card-border);
  color: var(--text-main); font-size: 11px; padding: 2px 8px; border-radius: 6px;
  cursor: pointer; transition: all 0.15s;
}
.copy-btn:hover { background: var(--accent-blue); color: #fff; }
.copy-btn.copied { background: var(--accent-green); color: #fff; }

/* Scroll & Loading States */
.scroll-sentinel { width: 100%; height: 20px; margin-top: 10px; }
.load-more-wrap { display: flex; justify-content: center; padding: 20px 0; }
.load-more-btn {
  padding: 10px 24px; border-radius: 12px; border: 1px solid var(--card-border);
  background: var(--bg-card); color: var(--accent-cyan); font-weight: 600; font-size: 14px;
  cursor: pointer; transition: all 0.2s;
}
.load-more-btn:hover { background: var(--bg-card-hover); border-color: var(--accent-blue); }
.loading-indicator {
  text-align: center; padding: 20px; color: var(--text-muted); font-size: 13.5px;
  display: flex; align-items: center; justify-content: center; gap: 8px;
}
.end-indicator { text-align: center; padding: 25px 0; color: var(--text-muted); font-size: 12.5px; font-style: italic; }

/* Modal */
.modal-overlay {
  position: fixed; top: 0; left: 0; right: 0; bottom: 0;
  background: rgba(0, 0, 0, 0.75); backdrop-filter: blur(8px);
  display: none; align-items: center; justify-content: center;
  padding: 20px; z-index: 1000;
}
.modal-content {
  background: #111827; border: 1px solid var(--card-border);
  border-radius: 20px; max-width: 650px; width: 100%;
  max-height: 85vh; overflow-y: auto; padding: 28px;
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
.modal-body { margin-top: 18px; font-size: 14.5px; line-height: 1.6; white-space: pre-wrap; }
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
        <p style="font-size: 13px; color: var(--text-muted); margin-top: 2px;">Personal Event Log & Knowledge ERP System</p>
      </div>
    </div>
    <div class="status-badge">
      <div class="pulse"></div>
      Database Connected
    </div>
  </header>

  <div class="nav-tabs">
    <button class="tab-btn active" onclick="switchTab('overview', event)">📊 Overview</button>
    <button class="tab-btn" onclick="switchTab('events', event)">📅 Timeline Events</button>
    <button class="tab-btn" onclick="switchTab('contacts', event)">👥 CRM Contacts</button>
    <button class="tab-btn" onclick="switchTab('media', event)">🎬 Media Logs</button>
    <button class="tab-btn" onclick="switchTab('vendors', event)">⭐ Vendors</button>
    <button class="tab-btn" onclick="switchTab('links', event)">🔗 Links</button>
    <button class="tab-btn" onclick="switchTab('commerce', event)">💳 Commerce</button>
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
    <div class="toolbar-section">
      <div class="filter-row">
        <div class="search-input-wrap">
          <input type="text" id="event-search" class="search-input" placeholder="Search events by title, place, notes..." oninput="debounceTrigger('events')">
        </div>
        <div class="filter-group">
          <label style="font-size:12.5px; color:var(--text-muted);">From:</label>
          <input type="date" id="event-from" class="date-input" onchange="resetAndFetch('events')">
          <label style="font-size:12.5px; color:var(--text-muted);">To:</label>
          <input type="date" id="event-to" class="date-input" onchange="resetAndFetch('events')">
          <button class="sort-btn" onclick="clearEventFilters()">Clear</button>
        </div>
      </div>
      <div class="filter-row">
        <div class="sort-bar" id="events-sort-bar">
          <span>Sort by:</span>
          <button class="sort-btn active" data-col="start_date" onclick="setSort('events', 'start_date')">Date <span class="sort-dir">▼</span></button>
          <button class="sort-btn" data-col="title" onclick="setSort('events', 'title')">Title <span class="sort-dir"></span></button>
          <button class="sort-btn" data-col="place" onclick="setSort('events', 'place')">Place <span class="sort-dir"></span></button>
          <button class="sort-btn" data-col="created_at" onclick="setSort('events', 'created_at')">Created <span class="sort-dir"></span></button>
        </div>
        <div class="filter-group">
          <span class="count-badge" id="events-count">Loading...</span>
          <select id="events-limit" class="control-select" onchange="changeLimit('events')">
            <option value="10">10 / page</option>
            <option value="25" selected>25 / page</option>
            <option value="50">50 / page</option>
            <option value="100">100 / page</option>
          </select>
        </div>
      </div>
    </div>
    <div class="events-grid" id="events-list"></div>
    <div id="events-sentinel" class="scroll-sentinel"></div>
    <div id="events-footer" class="load-more-wrap"></div>
  </div>

  <!-- CRM CONTACTS TAB -->
  <div id="tab-contacts" style="display:none;">
    <div class="toolbar-section">
      <div class="filter-row">
        <div class="filter-group" id="contact-source-filters">
          <button class="tag-pill active" onclick="filterContactsSource('', this)">All Contacts</button>
          <button class="tag-pill" onclick="filterContactsSource('manual', this)">Manual (CRM)</button>
          <button class="tag-pill" onclick="filterContactsSource('google', this)">Google Synced</button>
          <button class="tag-pill" onclick="filterContactsSource('merged', this)">Merged</button>
        </div>
        <button class="tag-pill" style="background: linear-gradient(135deg, var(--accent-purple), var(--accent-pink)); color:#fff; border:none; cursor:pointer; font-weight:600; padding:8px 16px;" onclick="triggerAutoMerge()">⚡ Auto-Merge Duplicates</button>
      </div>
      <div class="filter-row">
        <div class="search-input-wrap">
          <input type="text" id="contact-search" class="search-input" placeholder="Search contacts by name, email, phone, org, client, notes..." oninput="debounceTrigger('contacts')">
        </div>
      </div>
      <div class="filter-row">
        <div class="sort-bar" id="contacts-sort-bar">
          <span>Sort by:</span>
          <button class="sort-btn active" data-col="event_count" onclick="setSort('contacts', 'event_count')">Linked Events <span class="sort-dir">▼</span></button>
          <button class="sort-btn" data-col="name" onclick="setSort('contacts', 'name')">Name <span class="sort-dir"></span></button>
          <button class="sort-btn" data-col="org" onclick="setSort('contacts', 'org')">Organization <span class="sort-dir"></span></button>
          <button class="sort-btn" data-col="location" onclick="setSort('contacts', 'location')">Location <span class="sort-dir"></span></button>
          <button class="sort-btn" data-col="source" onclick="setSort('contacts', 'source')">Source <span class="sort-dir"></span></button>
        </div>
        <div class="filter-group">
          <span class="count-badge" id="contacts-count">Loading...</span>
          <select id="contacts-limit" class="control-select" onchange="changeLimit('contacts')">
            <option value="12">12 / page</option>
            <option value="24" selected>24 / page</option>
            <option value="48">48 / page</option>
            <option value="96">96 / page</option>
          </select>
        </div>
      </div>
    </div>
    <div class="contacts-grid" id="contacts-list"></div>
    <div id="contacts-sentinel" class="scroll-sentinel"></div>
    <div id="contacts-footer" class="load-more-wrap"></div>
  </div>

  <!-- MEDIA CONSUMPTION TAB -->
  <div id="tab-media" style="display:none;">
    <div class="toolbar-section">
      <div class="filter-row">
        <div class="filter-group" id="media-type-filters">
          <button class="tag-pill active" onclick="filterMediaType('', this)">All Media</button>
          <button class="tag-pill" onclick="filterMediaType('book', this)">📚 Books</button>
          <button class="tag-pill" onclick="filterMediaType('film', this)">🎬 Films</button>
          <button class="tag-pill" onclick="filterMediaType('anime', this)">⛩️ Anime</button>
          <button class="tag-pill" onclick="filterMediaType('manga', this)">📖 Manga</button>
          <button class="tag-pill" onclick="filterMediaType('drama', this)">🎭 Dramas</button>
        </div>
      </div>
      <div class="filter-row">
        <div class="search-input-wrap">
          <input type="text" id="media-search" class="search-input" placeholder="Search media by title, author, source, notes..." oninput="debounceTrigger('media')">
        </div>
      </div>
      <div class="filter-row">
        <div class="sort-bar" id="media-sort-bar">
          <span>Sort by:</span>
          <button class="sort-btn active" data-col="id" onclick="setSort('media', 'id')">Latest <span class="sort-dir">▼</span></button>
          <button class="sort-btn" data-col="title" onclick="setSort('media', 'title')">Title <span class="sort-dir"></span></button>
          <button class="sort-btn" data-col="rating" onclick="setSort('media', 'rating')">Rating <span class="sort-dir"></span></button>
          <button class="sort-btn" data-col="date" onclick="setSort('media', 'date')">Date Logged <span class="sort-dir"></span></button>
          <button class="sort-btn" data-col="media_type" onclick="setSort('media', 'media_type')">Type <span class="sort-dir"></span></button>
        </div>
        <div class="filter-group">
          <span class="count-badge" id="media-count">Loading...</span>
          <select id="media-limit" class="control-select" onchange="changeLimit('media')">
            <option value="12">12 / page</option>
            <option value="24" selected>24 / page</option>
            <option value="48">48 / page</option>
            <option value="96">96 / page</option>
          </select>
        </div>
      </div>
    </div>
    <div class="media-grid" id="media-list"></div>
    <div id="media-sentinel" class="scroll-sentinel"></div>
    <div id="media-footer" class="load-more-wrap"></div>
  </div>

  <!-- VENDORS TAB -->
  <div id="tab-vendors" style="display:none;">
    <div class="toolbar-section">
      <div class="filter-row">
        <div class="filter-group">
          <button class="tag-pill" id="vendor-fav-toggle" onclick="toggleVendorFavFilter(this)">★ Favorites Only</button>
          <select id="vendor-category-filter" class="control-select" onchange="resetAndFetch('vendors')">
            <option value="">All Categories</option>
          </select>
        </div>
        <div class="search-input-wrap">
          <input type="text" id="vendor-search" class="search-input" placeholder="Search vendors by name, category, location, notes, phone..." oninput="debounceTrigger('vendors')">
        </div>
      </div>
      <div class="filter-row">
        <div class="sort-bar" id="vendors-sort-bar">
          <span>Sort by:</span>
          <button class="sort-btn active" data-col="favorite" onclick="setSort('vendors', 'favorite')">Favorites First <span class="sort-dir">▼</span></button>
          <button class="sort-btn" data-col="name" onclick="setSort('vendors', 'name')">Name <span class="sort-dir"></span></button>
          <button class="sort-btn" data-col="category" onclick="setSort('vendors', 'category')">Category <span class="sort-dir"></span></button>
          <button class="sort-btn" data-col="location" onclick="setSort('vendors', 'location')">Location <span class="sort-dir"></span></button>
        </div>
        <div class="filter-group">
          <span class="count-badge" id="vendors-count">Loading...</span>
          <select id="vendors-limit" class="control-select" onchange="changeLimit('vendors')">
            <option value="12">12 / page</option>
            <option value="24" selected>24 / page</option>
            <option value="48">48 / page</option>
          </select>
        </div>
      </div>
    </div>
    <div class="vendors-grid" id="vendors-list"></div>
    <div id="vendors-sentinel" class="scroll-sentinel"></div>
    <div id="vendors-footer" class="load-more-wrap"></div>
  </div>

  <!-- LINKS TAB -->
  <div id="tab-links" style="display:none;">
    <div class="toolbar-section">
      <div class="filter-row">
        <div class="filter-group" id="links-category-filters">
          <button class="tag-pill active" onclick="filterLinksCategory('', this)">All Links</button>
        </div>
        <div class="search-input-wrap">
          <input type="text" id="links-search" class="search-input" placeholder="Search links by label, URL, category, notes..." oninput="debounceTrigger('links')">
        </div>
      </div>
      <div class="filter-row">
        <div class="sort-bar" id="links-sort-bar">
          <span>Sort by:</span>
          <button class="sort-btn active" data-col="category" onclick="setSort('links', 'category')">Category <span class="sort-dir">▲</span></button>
          <button class="sort-btn" data-col="label" onclick="setSort('links', 'label')">Label <span class="sort-dir"></span></button>
          <button class="sort-btn" data-col="created_at" onclick="setSort('links', 'created_at')">Created <span class="sort-dir"></span></button>
        </div>
        <div class="filter-group">
          <span class="count-badge" id="links-count">Loading...</span>
          <select id="links-limit" class="control-select" onchange="changeLimit('links')">
            <option value="12">12 / page</option>
            <option value="24" selected>24 / page</option>
            <option value="48">48 / page</option>
          </select>
        </div>
      </div>
    </div>
    <div class="links-grid" id="links-list"></div>
    <div id="links-sentinel" class="scroll-sentinel"></div>
    <div id="links-footer" class="load-more-wrap"></div>
  </div>

  <!-- COMMERCE TAB (PAYMENT ACCOUNTS + REFERRALS) -->
  <div id="tab-commerce" style="display:none;">
    <div class="commerce-subtabs">
      <button class="subtab-btn active" id="subtab-payments-btn" onclick="switchCommerceSubtab('payments')">💳 Payment Accounts</button>
      <button class="subtab-btn" id="subtab-referrals-btn" onclick="switchCommerceSubtab('referrals')">🎁 Referral Codes</button>
    </div>

    <!-- Payments Sub-view -->
    <div id="view-payments">
      <div class="toolbar-section">
        <div class="filter-row">
          <div class="search-input-wrap">
            <input type="text" id="payments-search" class="search-input" placeholder="Search payment rails by bank name, recipient, number, details..." oninput="debounceTrigger('payments')">
          </div>
          <div class="filter-group">
            <span class="count-badge" id="payments-count">Loading...</span>
            <select id="payments-limit" class="control-select" onchange="changeLimit('payments')">
              <option value="12">12 / page</option>
              <option value="24" selected>24 / page</option>
              <option value="48">48 / page</option>
            </select>
          </div>
        </div>
      </div>
      <div class="commerce-grid" id="payments-list"></div>
      <div id="payments-sentinel" class="scroll-sentinel"></div>
      <div id="payments-footer" class="load-more-wrap"></div>
    </div>

    <!-- Referrals Sub-view -->
    <div id="view-referrals" style="display:none;">
      <div class="toolbar-section">
        <div class="filter-row">
          <div class="search-input-wrap">
            <input type="text" id="referrals-search" class="search-input" placeholder="Search referral programs, codes, links, benefit perks..." oninput="debounceTrigger('referrals')">
          </div>
          <div class="filter-group">
            <span class="count-badge" id="referrals-count">Loading...</span>
            <select id="referrals-limit" class="control-select" onchange="changeLimit('referrals')">
              <option value="12">12 / page</option>
              <option value="24" selected>24 / page</option>
              <option value="48">48 / page</option>
            </select>
          </div>
        </div>
      </div>
      <div class="commerce-grid" id="referrals-list"></div>
      <div id="referrals-sentinel" class="scroll-sentinel"></div>
      <div id="referrals-footer" class="load-more-wrap"></div>
    </div>
  </div>
</div>

<!-- EVENT DETAIL MODAL -->
<div class="modal-overlay" id="event-modal">
  <div class="modal-content">
    <button class="close-btn" onclick="closeModal()">×</button>
    <h2 id="modal-title" style="margin-bottom: 8px;"></h2>
    <div id="modal-meta" style="color: var(--accent-cyan); font-size: 14px; margin-bottom: 16px;"></div>
    <div id="modal-tags" style="display:flex; gap:6px; margin-bottom:16px; flex-wrap:wrap;"></div>
    <div id="modal-people" style="color: var(--accent-purple); font-size: 14px; margin-bottom: 16px;"></div>
    <hr style="border: 0; border-top: 1px solid var(--card-border);">
    <div class="modal-body" id="modal-body"></div>
    <div id="modal-media"></div>
  </div>
</div>

<script>
// Tab and pagination state store
const tabStates = {
  events: {
    endpoint: '/api/events',
    listElId: 'events-list',
    countElId: 'events-count',
    footerElId: 'events-footer',
    sentinelId: 'events-sentinel',
    limit: 25,
    offset: 0,
    total: 0,
    items: [],
    loading: false,
    hasMore: true,
    sort: 'start_date',
    dir: 'desc',
    currentTag: ''
  },
  contacts: {
    endpoint: '/api/contacts',
    listElId: 'contacts-list',
    countElId: 'contacts-count',
    footerElId: 'contacts-footer',
    sentinelId: 'contacts-sentinel',
    limit: 24,
    offset: 0,
    total: 0,
    items: [],
    loading: false,
    hasMore: true,
    sort: 'event_count',
    dir: 'desc',
    source: ''
  },
  media: {
    endpoint: '/api/media',
    listElId: 'media-list',
    countElId: 'media-count',
    footerElId: 'media-footer',
    sentinelId: 'media-sentinel',
    limit: 24,
    offset: 0,
    total: 0,
    items: [],
    loading: false,
    hasMore: true,
    sort: 'id',
    dir: 'desc',
    mediaType: ''
  },
  vendors: {
    endpoint: '/api/vendors',
    listElId: 'vendors-list',
    countElId: 'vendors-count',
    footerElId: 'vendors-footer',
    sentinelId: 'vendors-sentinel',
    limit: 24,
    offset: 0,
    total: 0,
    items: [],
    loading: false,
    hasMore: true,
    sort: 'favorite',
    dir: 'desc',
    favoriteOnly: false
  },
  links: {
    endpoint: '/api/links',
    listElId: 'links-list',
    countElId: 'links-count',
    footerElId: 'links-footer',
    sentinelId: 'links-sentinel',
    limit: 24,
    offset: 0,
    total: 0,
    items: [],
    loading: false,
    hasMore: true,
    sort: 'category',
    dir: 'asc',
    category: ''
  },
  payments: {
    endpoint: '/api/payment-accounts',
    listElId: 'payments-list',
    countElId: 'payments-count',
    footerElId: 'payments-footer',
    sentinelId: 'payments-sentinel',
    limit: 24,
    offset: 0,
    total: 0,
    items: [],
    loading: false,
    hasMore: true,
    sort: 'category',
    dir: 'asc'
  },
  referrals: {
    endpoint: '/api/referrals',
    listElId: 'referrals-list',
    countElId: 'referrals-count',
    footerElId: 'referrals-footer',
    sentinelId: 'referrals-sentinel',
    limit: 24,
    offset: 0,
    total: 0,
    items: [],
    loading: false,
    hasMore: true,
    sort: 'category',
    dir: 'asc'
  }
};

const debounceTimers = {};
const observers = {};

// Overview Stats Loader
async function loadStats() {
  try {
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
      btn.className = `tag-pill ${tabStates.events.currentTag === tag ? 'active' : ''}`;
      btn.innerHTML = `${escapeHtml(tag)} <span class="tag-count">${count}</span>`;
      btn.onclick = () => filterByTag(tag);
      tagsEl.appendChild(btn);
    });
  } catch (err) {
    console.error('Failed to load stats:', err);
  }
}

// Utility: HTML Escaper
function escapeHtml(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

// Reusable Query Builder for each Tab
function buildQueryParams(tabKey) {
  const state = tabStates[tabKey];
  const params = new URLSearchParams();
  params.set('limit', state.limit);
  params.set('offset', state.offset);
  if (state.sort) params.set('sort', state.sort);
  if (state.dir) params.set('dir', state.dir);

  if (tabKey === 'events') {
    const q = document.getElementById('event-search')?.value.trim();
    const fromDate = document.getElementById('event-from')?.value;
    const toDate = document.getElementById('event-to')?.value;
    if (q) params.set('q', q);
    if (state.currentTag) params.set('tag', state.currentTag);
    if (fromDate) params.set('from', fromDate);
    if (toDate) params.set('to', toDate);
  } else if (tabKey === 'contacts') {
    const q = document.getElementById('contact-search')?.value.trim();
    if (q) params.set('q', q);
    if (state.source) params.set('source', state.source);
  } else if (tabKey === 'media') {
    const q = document.getElementById('media-search')?.value.trim();
    if (q) params.set('q', q);
    if (state.mediaType) params.set('type', state.mediaType);
  } else if (tabKey === 'vendors') {
    const q = document.getElementById('vendor-search')?.value.trim();
    const cat = document.getElementById('vendor-category-filter')?.value;
    if (q) params.set('q', q);
    if (cat) params.set('category', cat);
    if (state.favoriteOnly) params.set('favorite', '1');
  } else if (tabKey === 'links') {
    const q = document.getElementById('links-search')?.value.trim();
    if (q) params.set('q', q);
    if (state.category) params.set('category', state.category);
  } else if (tabKey === 'payments') {
    const q = document.getElementById('payments-search')?.value.trim();
    if (q) params.set('q', q);
  } else if (tabKey === 'referrals') {
    const q = document.getElementById('referrals-search')?.value.trim();
    if (q) params.set('q', q);
  }

  return params;
}

// Fetch items for a tab (supports pagination append)
async function fetchTabItems(tabKey, isAppend = false) {
  const state = tabStates[tabKey];
  if (state.loading) return;
  if (isAppend && !state.hasMore) return;

  state.loading = true;
  if (!isAppend) {
    state.offset = 0;
    state.items = [];
    state.hasMore = true;
    const listEl = document.getElementById(state.listElId);
    if (listEl) listEl.innerHTML = '<div class="loading-indicator">⏳ Loading data...</div>';
  }

  const footerEl = document.getElementById(state.footerElId);
  if (footerEl && isAppend) {
    footerEl.innerHTML = '<div class="loading-indicator">⏳ Loading more items...</div>';
  }

  try {
    const params = buildQueryParams(tabKey);
    const res = await fetch(`${state.endpoint}?${params.toString()}`);
    const data = await res.json();

    const items = Array.isArray(data) ? data : (data.items || []);
    const total = (typeof data.total === 'number') ? data.total : (isAppend ? state.items.length + items.length : items.length);

    state.total = total;
    if (isAppend) {
      state.items.push(...items);
    } else {
      state.items = items;
    }

    state.offset = state.items.length;
    state.hasMore = state.items.length < state.total && items.length > 0;

    // Populate dynamic categories for filter selects
    if (!isAppend) {
      if (tabKey === 'vendors' && data.categories) {
        populateCategoryFilter('vendor-category-filter', data.categories);
      } else if (tabKey === 'links' && data.categories) {
        populateLinksCategoryPills(data.categories);
      }
    }

    renderTabItems(tabKey);
    updateCountBadge(tabKey);
    renderFooter(tabKey);
  } catch (err) {
    console.error(`Failed to fetch ${tabKey}:`, err);
    const listEl = document.getElementById(state.listElId);
    if (listEl && !isAppend) {
      listEl.innerHTML = `<div style="color:var(--accent-pink); text-align:center; padding:30px;">Error loading data.</div>`;
    }
  } finally {
    state.loading = false;
  }
}

function updateCountBadge(tabKey) {
  const state = tabStates[tabKey];
  const countEl = document.getElementById(state.countElId);
  if (countEl) {
    countEl.innerText = `Showing ${state.items.length} of ${state.total}`;
  }
}

function renderFooter(tabKey) {
  const state = tabStates[tabKey];
  const footerEl = document.getElementById(state.footerElId);
  if (!footerEl) return;

  if (state.hasMore) {
    footerEl.innerHTML = `<button class="load-more-btn" onclick="fetchTabItems('${tabKey}', true)">Load More (${state.total - state.items.length} remaining)</button>`;
  } else if (state.total > 0) {
    footerEl.innerHTML = `<div class="end-indicator">✓ All ${state.total} items loaded</div>`;
  } else {
    footerEl.innerHTML = '';
  }
}

// Master Renderer Router
function renderTabItems(tabKey) {
  const state = tabStates[tabKey];
  const listEl = document.getElementById(state.listElId);
  if (!listEl) return;

  if (state.items.length === 0) {
    listEl.innerHTML = `<div style="color: var(--text-muted); text-align: center; padding: 40px; grid-column: 1 / -1;">No matching items found.</div>`;
    return;
  }

  if (tabKey === 'events') renderEventsList(listEl, state.items);
  else if (tabKey === 'contacts') renderContactsGrid(listEl, state.items);
  else if (tabKey === 'media') renderMediaGrid(listEl, state.items);
  else if (tabKey === 'vendors') renderVendorsGrid(listEl, state.items);
  else if (tabKey === 'links') renderLinksGrid(listEl, state.items);
  else if (tabKey === 'payments') renderPaymentsGrid(listEl, state.items);
  else if (tabKey === 'referrals') renderReferralsGrid(listEl, state.items);
}

// Renderers
function renderEventsList(container, items) {
  container.innerHTML = '';
  items.forEach(ev => {
    const card = document.createElement('div');
    card.className = 'event-card';
    card.onclick = () => showEventModal(ev.id);
    
    const tagsHtml = (ev.tags || []).map(t => `<span class="tag-pill" style="font-size:11px; padding:3px 8px;">${escapeHtml(t)}</span>`).join('');
    const peopleHtml = (ev.linked_contacts || []).map(c => `👤 ${escapeHtml(c.name)}`).join(', ');

    card.innerHTML = `
      <div class="event-header">
        <div class="event-title">${escapeHtml(ev.title)}</div>
        <div class="event-date">${escapeHtml(ev.start_date || ev.raw_date || 'No Date')}</div>
      </div>
      <div class="event-meta">
        ${ev.place ? `<div class="meta-item">📍 ${escapeHtml(ev.place)}</div>` : ''}
        ${peopleHtml ? `<div class="meta-item" style="color:var(--accent-purple);">${peopleHtml}</div>` : ''}
      </div>
      <div style="display:flex; gap:6px; margin-bottom:10px; flex-wrap:wrap;">${tagsHtml}</div>
      <div class="event-notes-preview">${escapeHtml(ev.notes || '')}</div>
    `;
    container.appendChild(card);
  });
}

function renderContactsGrid(container, items) {
  container.innerHTML = '';
  items.forEach(c => {
    const card = document.createElement('div');
    card.className = 'contact-card';
    
    let badgeColor = 'var(--accent-blue)';
    let badgeText = (c.source || 'manual').toUpperCase();
    if (c.source === 'google') { badgeColor = 'var(--accent-green)'; }
    if (c.source === 'merged') { badgeColor = 'var(--accent-pink)'; }

    card.innerHTML = `
      <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:8px;">
        <div class="contact-name">${escapeHtml(c.name)}</div>
        <span style="font-size:10px; font-weight:700; padding:2px 8px; border-radius:10px; background:rgba(255,255,255,0.06); color:${badgeColor}; border:1px solid ${badgeColor};">${badgeText}</span>
      </div>
      ${c.org ? `<div class="contact-org">🏢 ${escapeHtml(c.org)} ${c.client ? `(${escapeHtml(c.client)})` : ''}</div>` : ''}
      ${c.email ? `<div style="font-size:13px; color:var(--text-muted); margin-bottom:4px;">✉️ ${escapeHtml(c.email)}</div>` : ''}
      ${c.phone ? `<div style="font-size:13px; color:var(--text-muted); margin-bottom:4px;">📞 ${escapeHtml(c.phone)}</div>` : ''}
      ${c.location ? `<div style="font-size:13px; color:var(--text-muted); margin-bottom:8px;">📍 ${escapeHtml(c.location)}</div>` : ''}
      <div class="contact-badge">${c.event_count || 0} Linked Events</div>
    `;
    container.appendChild(card);
  });
}

function renderMediaGrid(container, items) {
  container.innerHTML = '';
  items.forEach(m => {
    const card = document.createElement('div');
    card.className = 'media-card';

    const typeIcons = { book: '📚', film: '🎬', anime: '⛩️', manga: '📖', drama: '🎭' };
    const icon = typeIcons[m.media_type] || '📁';
    const ratingHtml = m.rating !== null && m.rating !== undefined 
      ? `<span class="media-rating">⭐ ${typeof m.rating === 'number' ? m.rating.toFixed(1) : m.rating}</span>`
      : '';
    const statusText = m.status ? `<span style="font-size:11px; padding:2px 8px; border-radius:6px; background:rgba(255,255,255,0.06); color:var(--accent-cyan); text-transform:capitalize;">${escapeHtml(m.status)}</span>` : '';

    card.innerHTML = `
      <div>
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px;">
          <span class="media-type-badge">${icon} ${escapeHtml(m.media_type)}</span>
          <span style="font-size:11px; color:var(--text-muted);">${escapeHtml(m.source || '')}</span>
        </div>
        <div class="media-title">${escapeHtml(m.title)}</div>
        ${m.author ? `<div style="font-size:13px; color:var(--text-muted); margin-top:4px;">By ${escapeHtml(m.author)}</div>` : ''}
      </div>
      <div style="display:flex; justify-content:space-between; align-items:center; border-top:1px solid var(--card-border); padding-top:10px; margin-top:6px;">
        <div style="display:flex; gap:8px; align-items:center;">
          ${statusText}
          ${ratingHtml}
        </div>
        <span style="font-size:12px; color:var(--text-muted); font-family:'JetBrains Mono',monospace;">${escapeHtml(m.date || '')}</span>
      </div>
    `;
    container.appendChild(card);
  });
}

function renderVendorsGrid(container, items) {
  container.innerHTML = '';
  items.forEach(v => {
    const card = document.createElement('div');
    card.className = 'vendor-card';
    const isFav = Boolean(v.favorite);

    card.innerHTML = `
      <div class="vendor-header">
        <div>
          <div class="vendor-name">${escapeHtml(v.name)}</div>
          <span style="display:inline-block; font-size:11px; color:var(--accent-purple); background:rgba(138,43,226,0.1); border:1px solid rgba(138,43,226,0.25); padding:2px 8px; border-radius:8px; margin-top:4px;">${escapeHtml(v.category || 'General')}</span>
        </div>
        <button class="fav-btn ${isFav ? 'is-fav' : ''}" title="${isFav ? 'Remove Favorite' : 'Mark Favorite'}" onclick="toggleVendorFavorite(${v.id}, event)">★</button>
      </div>
      ${v.location ? `<div style="font-size:13px; color:var(--text-muted); margin-bottom:4px;">📍 ${escapeHtml(v.location)}</div>` : ''}
      ${v.phone ? `<div style="font-size:13px; color:var(--accent-cyan); margin-bottom:4px;">📞 ${escapeHtml(v.phone)}</div>` : ''}
      ${v.email ? `<div style="font-size:13px; color:var(--text-muted); margin-bottom:4px;">✉️ ${escapeHtml(v.email)}</div>` : ''}
      ${v.notes ? `<div style="font-size:13.5px; color:var(--text-muted); margin-top:8px; line-height:1.4;">${escapeHtml(v.notes)}</div>` : ''}
    `;
    container.appendChild(card);
  });
}

function renderLinksGrid(container, items) {
  container.innerHTML = '';
  items.forEach(l => {
    const card = document.createElement('div');
    card.className = 'link-card';

    card.innerHTML = `
      <div>
        <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:6px;">
          <strong style="font-size:15px; color:#fff;">${escapeHtml(l.label)}</strong>
          <span style="font-size:10.5px; padding:2px 6px; border-radius:6px; background:rgba(255,255,255,0.06); color:var(--accent-purple); text-transform:uppercase;">${escapeHtml(l.category || 'Link')}</span>
        </div>
        <a href="${escapeHtml(l.url)}" target="_blank" rel="noopener noreferrer" class="link-url">
          ${escapeHtml(l.url)} ↗
        </a>
      </div>
      ${l.notes ? `<div style="font-size:13px; color:var(--text-muted);">${escapeHtml(l.notes)}</div>` : ''}
    `;
    container.appendChild(card);
  });
}

function renderPaymentsGrid(container, items) {
  container.innerHTML = '';
  items.forEach(p => {
    const card = document.createElement('div');
    card.className = 'commerce-card';

    card.innerHTML = `
      <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:8px;">
        <strong style="font-size:16px; color:#fff;">${escapeHtml(p.name)}</strong>
        <span style="font-size:11px; padding:2px 8px; border-radius:8px; background:rgba(79,172,254,0.12); color:var(--accent-blue);">${escapeHtml(p.category || 'Bank')}</span>
      </div>
      <div style="font-size:13px; color:var(--text-muted); margin-bottom:6px;">Recipient: <strong style="color:var(--text-main);">${escapeHtml(p.recipient || 'Personal')}</strong></div>
      ${p.number ? `
        <div class="copy-box">
          <span>${escapeHtml(p.number)}</span>
          <button class="copy-btn" onclick="copyText('${escapeHtml(p.number)}', this)">Copy</button>
        </div>
      ` : ''}
      ${p.details ? `<div style="font-size:13px; color:var(--text-muted); margin-top:6px;">${escapeHtml(p.details)}</div>` : ''}
    `;
    container.appendChild(card);
  });
}

function renderReferralsGrid(container, items) {
  container.innerHTML = '';
  items.forEach(r => {
    const card = document.createElement('div');
    card.className = 'commerce-card';
    const statusColor = (r.status === 'ACTIVE' || r.status === 'ONLINE') ? 'var(--accent-green)' : 'var(--accent-amber)';

    card.innerHTML = `
      <div style="display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:8px;">
        <strong style="font-size:16px; color:#fff;">${escapeHtml(r.name)}</strong>
        <span style="font-size:10px; font-weight:700; padding:2px 8px; border-radius:8px; background:rgba(255,255,255,0.06); color:${statusColor}; border:1px solid ${statusColor};">${escapeHtml(r.status || 'ACTIVE')}</span>
      </div>
      <div style="font-size:12px; color:var(--accent-purple); margin-bottom:8px;">${escapeHtml(r.category || 'Program')}</div>
      ${r.benefit ? `<div style="font-size:13px; color:var(--text-main); margin-bottom:10px;">🎁 ${escapeHtml(r.benefit)}</div>` : ''}
      ${r.code ? `
        <div class="copy-box">
          <span>Code: ${escapeHtml(r.code)}</span>
          <button class="copy-btn" onclick="copyText('${escapeHtml(r.code)}', this)">Copy</button>
        </div>
      ` : ''}
      ${r.link ? `
        <div style="margin-top:8px;">
          <a href="${escapeHtml(r.link)}" target="_blank" rel="noopener noreferrer" class="link-url">Open Referral Link ↗</a>
        </div>
      ` : ''}
    `;
    container.appendChild(card);
  });
}

// Interactive Actions
async function toggleVendorFavorite(vendorId, event) {
  if (event) event.stopPropagation();
  try {
    const res = await fetch('/api/vendors/favorite', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: vendorId })
    });
    const data = await res.json();
    if (data.status === 'success') {
      const state = tabStates.vendors;
      const vendor = state.items.find(v => v.id === vendorId);
      if (vendor) {
        vendor.favorite = data.favorite;
        renderVendorsGrid(document.getElementById(state.listElId), state.items);
      }
    }
  } catch (err) {
    console.error('Failed to toggle vendor favorite:', err);
  }
}

function copyText(text, btn) {
  navigator.clipboard.writeText(text).then(() => {
    const orig = btn.innerText;
    btn.innerText = '✓ Copied';
    btn.classList.add('copied');
    setTimeout(() => {
      btn.innerText = orig;
      btn.classList.remove('copied');
    }, 1500);
  }).catch(err => {
    console.error('Copy failed:', err);
  });
}

// Sorting Controls
function setSort(tabKey, colName) {
  const state = tabStates[tabKey];
  if (state.sort === colName) {
    state.dir = state.dir === 'asc' ? 'desc' : 'asc';
  } else {
    state.sort = colName;
    state.dir = (colName === 'start_date' || colName === 'event_count' || colName === 'id' || colName === 'rating' || colName === 'favorite') ? 'desc' : 'asc';
  }

  // Update button active state & arrows in sort bar
  const sortBar = document.getElementById(`${tabKey}-sort-bar`);
  if (sortBar) {
    sortBar.querySelectorAll('.sort-btn').forEach(btn => {
      if (btn.getAttribute('data-col') === colName) {
        btn.classList.add('active');
        const dirSpan = btn.querySelector('.sort-dir');
        if (dirSpan) dirSpan.innerText = state.dir === 'asc' ? '▲' : '▼';
      } else {
        btn.classList.remove('active');
        const dirSpan = btn.querySelector('.sort-dir');
        if (dirSpan) dirSpan.innerText = '';
      }
    });
  }

  resetAndFetch(tabKey);
}

function changeLimit(tabKey) {
  const selectEl = document.getElementById(`${tabKey}-limit`);
  if (selectEl) {
    tabStates[tabKey].limit = parseInt(selectEl.value, 10);
    resetAndFetch(tabKey);
  }
}

function resetAndFetch(tabKey) {
  tabStates[tabKey].offset = 0;
  fetchTabItems(tabKey, false);
}

function debounceTrigger(tabKey) {
  clearTimeout(debounceTimers[tabKey]);
  debounceTimers[tabKey] = setTimeout(() => {
    resetAndFetch(tabKey);
  }, 250);
}

// Specific Filters
function filterByTag(tag) {
  tabStates.events.currentTag = tabStates.events.currentTag === tag ? '' : tag;
  const eventTabBtn = document.querySelectorAll('.tab-btn')[1];
  switchTab('events', { target: eventTabBtn });
  resetAndFetch('events');
}

function clearEventFilters() {
  document.getElementById('event-search').value = '';
  document.getElementById('event-from').value = '';
  document.getElementById('event-to').value = '';
  tabStates.events.currentTag = '';
  resetAndFetch('events');
}

function filterContactsSource(src, btn) {
  document.querySelectorAll('#contact-source-filters .tag-pill').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  tabStates.contacts.source = src;
  resetAndFetch('contacts');
}

function filterMediaType(type, btn) {
  document.querySelectorAll('#media-type-filters .tag-pill').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  tabStates.media.mediaType = type;
  resetAndFetch('media');
}

function toggleVendorFavFilter(btn) {
  tabStates.vendors.favoriteOnly = !tabStates.vendors.favoriteOnly;
  btn.classList.toggle('active', tabStates.vendors.favoriteOnly);
  resetAndFetch('vendors');
}

function populateCategoryFilter(selectId, categories) {
  const sel = document.getElementById(selectId);
  if (!sel) return;
  const currentVal = sel.value;
  sel.innerHTML = '<option value="">All Categories</option>';
  categories.forEach(cat => {
    if (cat) {
      const opt = document.createElement('option');
      opt.value = cat;
      opt.innerText = cat;
      if (cat === currentVal) opt.selected = true;
      sel.appendChild(opt);
    }
  });
}

function populateLinksCategoryPills(categories) {
  const container = document.getElementById('links-category-filters');
  if (!container) return;
  const current = tabStates.links.category;
  container.innerHTML = `<button class="tag-pill ${!current ? 'active' : ''}" onclick="filterLinksCategory('', this)">All Links</button>`;
  categories.forEach(cat => {
    if (cat) {
      const btn = document.createElement('button');
      btn.className = `tag-pill ${current === cat ? 'active' : ''}`;
      btn.innerText = cat;
      btn.onclick = () => filterLinksCategory(cat, btn);
      container.appendChild(btn);
    }
  });
}

function filterLinksCategory(cat, btn) {
  document.querySelectorAll('#links-category-filters .tag-pill').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  tabStates.links.category = cat;
  resetAndFetch('links');
}

function switchCommerceSubtab(subtab) {
  document.getElementById('subtab-payments-btn').classList.toggle('active', subtab === 'payments');
  document.getElementById('subtab-referrals-btn').classList.toggle('active', subtab === 'referrals');
  document.getElementById('view-payments').style.display = subtab === 'payments' ? 'block' : 'none';
  document.getElementById('view-referrals').style.display = subtab === 'referrals' ? 'block' : 'none';

  if (subtab === 'payments' && tabStates.payments.items.length === 0) {
    resetAndFetch('payments');
    initIntersectionObserver('payments');
  } else if (subtab === 'referrals' && tabStates.referrals.items.length === 0) {
    resetAndFetch('referrals');
    initIntersectionObserver('referrals');
  }
}

// Auto-Merge Duplicates Trigger
async function triggerAutoMerge() {
  if (!confirm('Scan and automatically merge duplicate contacts between manual CRM and Google Contacts?')) return;
  try {
    const res = await fetch('/api/contacts/merge', { 
      method: 'POST', 
      headers: {'Content-Type': 'application/json'}, 
      body: JSON.stringify({ auto: true }) 
    });
    const data = await res.json();
    alert(`Auto-merge complete!\\nMerged ${data.merged_count} contact(s).`);
    resetAndFetch('contacts');
    loadStats();
  } catch (err) {
    alert('Error during contact auto-merge: ' + err);
  }
}

// Modal Detail View
async function showEventModal(eventId) {
  try {
    const res = await fetch('/api/events/' + eventId);
    const ev = await res.json();
    
    document.getElementById('modal-title').innerText = ev.title;
    document.getElementById('modal-meta').innerText = `${ev.start_date || 'No Date'} ${ev.place ? ' • 📍 ' + ev.place : ''}`;
    document.getElementById('modal-tags').innerHTML = (ev.tags || []).map(t => `<span class="tag-pill">${escapeHtml(t)}</span>`).join('');
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
  } catch (err) {
    console.error('Failed to show event detail:', err);
  }
}

function closeModal() {
  document.getElementById('event-modal').style.display = 'none';
}

// Main Tab Navigation
function switchTab(tabName, evt) {
  document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
  if (evt && evt.target) {
    const targetBtn = evt.target.closest('.tab-btn') || evt.target;
    targetBtn.classList.add('active');
  }
  
  const allTabs = ['overview', 'events', 'contacts', 'media', 'vendors', 'links', 'commerce'];
  allTabs.forEach(t => {
    const el = document.getElementById(`tab-${t}`);
    if (el) el.style.display = t === tabName ? 'block' : 'none';
  });

  if (tabName !== 'overview') {
    const targetKey = tabName === 'commerce' ? 'payments' : tabName;
    if (tabStates[targetKey].items.length === 0) {
      resetAndFetch(targetKey);
    }
    initIntersectionObserver(targetKey);
  }
}

// Infinite Scroll Observer Setup
function initIntersectionObserver(tabKey) {
  const sentinelId = tabStates[tabKey].sentinelId;
  const sentinel = document.getElementById(sentinelId);
  if (!sentinel) return;

  if (observers[tabKey]) {
    observers[tabKey].disconnect();
  }

  observers[tabKey] = new IntersectionObserver((entries) => {
    if (entries[0].isIntersecting) {
      const state = tabStates[tabKey];
      if (state && !state.loading && state.hasMore) {
        fetchTabItems(tabKey, true);
      }
    }
  }, { rootMargin: '250px' });

  observers[tabKey].observe(sentinel);
}

// Initial Boot
loadStats();
resetAndFetch('events');
initIntersectionObserver('events');
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
