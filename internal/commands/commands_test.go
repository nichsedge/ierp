package commands_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nichsedge/ierp/internal/commands"
	"github.com/nichsedge/ierp/internal/db"
)

func setupTestDB(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_cmd.db")
	t.Setenv("TURSO_DATABASE_URL", "")
	t.Setenv("IERP_TURSO_URL", "")
	t.Setenv("IERP_DB", dbPath)

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	return dbPath
}

func TestCommandRunUsage(t *testing.T) {
	ctx := context.Background()

	// 1. Empty args prints usage to stdout and returns nil
	var stdout, stderr bytes.Buffer
	err := commands.Run(ctx, []string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected nil error for empty args, got %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage: ierp") {
		t.Fatalf("expected usage text in stdout, got: %s", stdout.String())
	}

	// 2. --help flag prints usage to stdout and returns nil
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"--help"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected nil error for --help, got %v", err)
	}
	if !strings.Contains(stdout.String(), "Commands:") {
		t.Fatalf("expected commands list in stdout, got: %s", stdout.String())
	}

	// 3. Unknown command returns error and prints to stderr
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"nonexistent-command-xyz"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error for unknown command, got nil")
	}
	if !strings.Contains(stderr.String(), "Unknown command: nonexistent-command-xyz") {
		t.Fatalf("expected unknown command warning in stderr, got: %s", stderr.String())
	}
}

func TestCommandEventsLifecycle(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer

	// 1. Insert structured event
	err := commands.Run(ctx, []string{"insert", "--title", "Strategy Offsite", "--place", "Bandung", "--tags", "strategy,offsite"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("insert event failed: %v (stderr: %s)", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Event #1 logged") {
		t.Fatalf("expected event logged confirmation, got: %s", stdout.String())
	}

	// 2. List events
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("list events failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Strategy Offsite") {
		t.Fatalf("expected Strategy Offsite in list output, got: %s", stdout.String())
	}

	// 3. Search events using FTS5
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"search", "Strategy"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("search events failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Strategy Offsite") {
		t.Fatalf("expected Strategy Offsite in search hits, got: %s", stdout.String())
	}

	// 4. Delete event
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"delete", "1"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("delete event failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Event #1 deleted successfully") {
		t.Fatalf("expected delete confirmation, got: %s", stdout.String())
	}
}

func TestCommandContactsLifecycle(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer

	// 1. Insert contact
	err := commands.Run(ctx, []string{"insert-contact", "--name", "Grace Hopper", "--org", "US Navy", "--tier", "1"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("insert contact failed: %v (stderr: %s)", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Contact #1 saved: Grace Hopper") {
		t.Fatalf("expected contact saved confirmation, got: %s", stdout.String())
	}

	// 2. List contacts
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"contacts"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("list contacts failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Grace Hopper") {
		t.Fatalf("expected Grace Hopper in contacts list, got: %s", stdout.String())
	}

	// 3. Set contact tier
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"set-tier", "--contact-id", "1", "--tier", "2", "--cadence", "45"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("set-tier failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Tier 2 (45 days cadence)") {
		t.Fatalf("expected tier updated confirmation, got: %s", stdout.String())
	}

	// 4. Check radar
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"radar"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("radar failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Grace Hopper") {
		t.Fatalf("expected Grace Hopper in radar, got: %s", stdout.String())
	}
}

func TestCommandExpensesLifecycle(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer

	// 1. Insert expense
	err := commands.Run(ctx, []string{"insert-expense", "--title", "Server Hosting", "--amount", "150000", "--category", "infrastructure"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("insert expense failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Recorded EXPENSE #1") {
		t.Fatalf("expected expense confirmation, got: %s", stdout.String())
	}

	// 2. Insert income
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"income", "add", "--title", "Consulting Retainer", "--amount", "25000000", "--category", "salary"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("insert income failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Recorded INCOME #2") {
		t.Fatalf("expected income confirmation, got: %s", stdout.String())
	}

	// 3. List cashflow ledger
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"expenses"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("list expenses failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Server Hosting") || !strings.Contains(stdout.String(), "Consulting Retainer") {
		t.Fatalf("expected both transactions in ledger, got: %s", stdout.String())
	}

	// 4. Cashflow summary
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"expense", "summary"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expense summary failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Total Expenses") || !strings.Contains(stdout.String(), "Total Income") {
		t.Fatalf("expected summary output, got: %s", stdout.String())
	}
}

func TestCommandSnapshotDeviationAndCorrection(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	var stdout, stderr bytes.Buffer

	// 1. Insert initial snapshot
	err := commands.Run(ctx, []string{"insert-snapshot", "--date", "2026-11-01", "--liquid", "100000000", "--investments", "500000000"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("insert snapshot failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Balance sheet snapshot #1 saved") {
		t.Fatalf("expected snapshot saved, got: %s", stdout.String())
	}

	// 2. Insert deviating snapshot (> 50% swing) without --force should fail
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"insert-snapshot", "--date", "2026-11-15", "--liquid", "30000000"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected deviation guard error without --force, got nil")
	}
	if !strings.Contains(stderr.String(), "Refusing snapshot: liquid cash deviates") {
		t.Fatalf("expected deviation refusal warning, got: %s", stderr.String())
	}

	// 3. Insert deviating snapshot WITH --force succeeds
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"insert-snapshot", "--date", "2026-11-15", "--liquid", "30000000", "--force"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("insert snapshot with --force failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Balance sheet snapshot #2 saved") {
		t.Fatalf("expected snapshot #2 saved, got: %s", stdout.String())
	}

	// 4. Void snapshot
	stdout.Reset()
	stderr.Reset()
	err = commands.Run(ctx, []string{"delete-snapshot", "--id", "2"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("delete snapshot failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Snapshot #2 voided") {
		t.Fatalf("expected snapshot voided, got: %s", stdout.String())
	}
}
