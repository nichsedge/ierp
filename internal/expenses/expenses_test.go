package expenses

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	schema := `
	CREATE TABLE expenses (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		date TEXT NOT NULL,
		title TEXT NOT NULL,
		amount REAL NOT NULL,
		currency TEXT NOT NULL DEFAULT 'IDR',
		category TEXT NOT NULL DEFAULT 'general',
		account TEXT,
		notes TEXT,
		tags TEXT,
		type TEXT NOT NULL DEFAULT 'EXPENSE',
		source TEXT DEFAULT 'manual',
		created_at TEXT DEFAULT (datetime('now', 'localtime')),
		updated_at TEXT DEFAULT (datetime('now', 'localtime'))
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create expenses schema: %v", err)
	}

	return db
}

func TestInsertAndListExpense(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := InsertExpense(db, Expense{
		Date:     "2026-11-20",
		Title:    "TransNusa Flight",
		Amount:   2800000,
		Currency: "IDR",
		Category: "Flights",
		Tags:     "travel,singapore",
	})
	if err != nil {
		t.Fatalf("InsertExpense failed: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	e, err := GetExpenseByID(db, id)
	if err != nil {
		t.Fatalf("GetExpenseByID failed: %v", err)
	}
	if e == nil || e.Title != "TransNusa Flight" || e.Amount != 2800000 || e.Category != "flights" {
		t.Fatalf("unexpected expense retrieved: %+v", e)
	}

	// List
	list, err := ListExpenses(db, ListOptions{Month: "2026-11"})
	if err != nil {
		t.Fatalf("ListExpenses failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list))
	}

	// Summary
	summary, err := GetSummary(db, "2026-11", "", "")
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if summary.Total != 2800000 || summary.Count != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestIngestMarkdownTable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	mdContent := `---
title: "Two Days in Singapore"
date: 2026-11-20
tags: [travelogue]
---

A two-day trip.

### Individual estimate Trip Cost (not included money changer cost)

*(Using ~Rp 14,177 per S$1)*

| Category | Your Cost / Details | SGD | IDR |
| --- | --- | --- | --- |
| **Flights** | TransNusa round-trip (your ticket) | — | Rp 2,800,000 |
| **Accommodation** | The Cube Capsule Hotel (your capsule) | — | Rp 327,000 |
| **Food & Drinks** | Half of shared meals & drinks (S$29.40) | S$29.40 | ~Rp 416,800 |
| **SG Transit (Est.)** | MRT & Bus (~2 days + 1 card's SimplyGo fee) | ~S$12.00 | ~Rp 170,100 |
| **ID Transit** | Airport Train (Rp 140k) + KRL (Rp 6k) + [[Gojek]] (Rp 15k) | — | Rp 161,000 |
| **Connectivity** | Roaming add-on | — | Rp 30,000 |
| **Your Total** | **All-in for 1 person** | — | **~Rp 3,904,900** |
`

	tmpFile, err := os.CreateTemp("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(mdContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	res, err := IngestFile(db, tmpFile.Name(), "singapore")
	if err != nil {
		t.Fatalf("IngestFile failed: %v", err)
	}

	if res.Count != 6 {
		t.Fatalf("expected 6 ingested items, got %d", res.Count)
	}

	// Total should be ~3,904,900
	expectedTotal := 2800000.0 + 327000.0 + 416800.0 + 170100.0 + 161000.0 + 30000.0
	if res.Total != expectedTotal {
		t.Fatalf("expected total %f, got %f", expectedTotal, res.Total)
	}

	// Verify all expenses in DB
	list, err := ListExpenses(db, ListOptions{Tag: "singapore"})
	if err != nil {
		t.Fatalf("ListExpenses failed: %v", err)
	}
	if len(list) != 6 {
		t.Fatalf("expected 6 expenses in DB, got %d", len(list))
	}
}

func TestIngestJSON(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	jsonContent := `[
		{"date": "2026-11-20", "title": "Coffee", "amount": 35000, "category": "food", "tags": ["daily"]},
		{"date": "2026-11-20", "title": "MRT", "amount": 14000, "category": "transport"}
	]`

	tmpFile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(jsonContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	res, err := IngestFile(db, tmpFile.Name(), "")
	if err != nil {
		t.Fatalf("IngestFile json failed: %v", err)
	}

	if res.Count != 2 {
		t.Fatalf("expected 2 items, got %d", res.Count)
	}
	if res.Total != 49000 {
		t.Fatalf("expected total 49000, got %f", res.Total)
	}
}
