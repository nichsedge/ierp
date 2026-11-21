package events_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/contacts"
	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/events"
)

func setupTestDBWithContact(t *testing.T) *sql.DB {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_events_ingest.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	// Insert contact Firzana Farah Amira
	_, err = contacts.InsertContact(database, contacts.Contact{
		Name: "Firzana Farah Amira",
	})
	if err != nil {
		t.Fatalf("failed to insert contact: %v", err)
	}

	return database
}

func TestIngestMarkdownEvent(t *testing.T) {
	database := setupTestDBWithContact(t)
	defer database.Close()

	mdContent := `---
title: "Two Days in Singapore"
date: 2026-11-20
tags: [travelogue]
publish_external: false
---

A two-day trip to [[Singapore]]. More travel pieces live under [[Travel]].

at 7 we gathered at palmerah station. we went to duri and then move to airport train station.
my gf and i arrived at changi airport.
`

	tmpFile, err := os.CreateTemp("", "travelogue-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(mdContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	res, err := events.IngestFile(database, tmpFile.Name(), "singapore", "", "")
	if err != nil {
		t.Fatalf("IngestFile failed: %v", err)
	}

	if res.StartDate != "2026-11-20" {
		t.Errorf("expected start date 2026-11-20, got %s", res.StartDate)
	}
	if res.EndDate != "2026-11-21" {
		t.Errorf("expected end date 2026-11-21, got %s", res.EndDate)
	}
	if res.Place != "Singapore" {
		t.Errorf("expected place Singapore, got %s", res.Place)
	}
	if len(res.Contacts) != 1 || res.Contacts[0] != "Firzana Farah Amira" {
		t.Errorf("expected linked contact Firzana Farah Amira, got %v", res.Contacts)
	}
	if res.Title != "Two Days in Singapore with Firzana" {
		t.Errorf("expected title 'Two Days in Singapore with Firzana', got %s", res.Title)
	}

	// Verify event exists in DB
	ev, err := events.GetEvent(database, res.EventID)
	if err != nil {
		t.Fatalf("GetEvent failed: %v", err)
	}
	if ev.Title != res.Title {
		t.Errorf("expected title %s, got %s", res.Title, ev.Title)
	}
}
