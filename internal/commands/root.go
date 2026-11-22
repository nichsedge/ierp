package commands

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/portfolio"
)

func openDB() (*sql.DB, error) {
	database, err := db.Open(config.DBPath())
	if err != nil {
		return nil, fmt.Errorf("error opening database: %w", err)
	}
	return database, nil
}

// PrintUsage writes the CLI command usage guide to the specified writer.
func PrintUsage(w io.Writer) {
	fmt.Fprintf(w, "%s%siERP — Individual Enterprise Resource Planning (Go)%s\n\n", config.Bold, config.Cyan, config.Reset)
	fmt.Fprintln(w, "Usage: ierp <command> [flags]")
	fmt.Fprintln(w, "\nCommands:")
	fmt.Fprintln(w, "  init-db              Initialize SQLite schema, FTS5 virtual table, and triggers")
	fmt.Fprintln(w, "  event                Event management (add, list, search, ingest, delete)")
	fmt.Fprintln(w, "  events               Alias for event")
	fmt.Fprintln(w, "  add                  Generic entity creation wrapper (event, expense, contact, etc.)")
	fmt.Fprintln(w, "  insert               Log a structured timeline event")
	fmt.Fprintln(w, "  list                 List recent events with filters")
	fmt.Fprintln(w, "  search               FTS5 full-text search with BM25 ranking")
	fmt.Fprintln(w, "  delete               Delete a timeline event by ID")
	fmt.Fprintln(w, "  ingest-events        Ingest event from Markdown travelogue or JSON")
	fmt.Fprintln(w, "  contacts             List CRM contacts")
	fmt.Fprintln(w, "  insert-contact       Record a new CRM contact")
	fmt.Fprintln(w, "  set-tier             Update Dunbar tier & cadence for a contact")
	fmt.Fprintln(w, "  radar                Show relationship reconnection radar")
	fmt.Fprintln(w, "  daily-reconnection   Get the top priority relationship needing touchpoint today")
	fmt.Fprintln(w, "  insert-snapshot      Record a balance sheet / net worth snapshot")
	fmt.Fprintln(w, "  update-snapshot      Correct an existing snapshot in place")
	fmt.Fprintln(w, "  delete-snapshot      Void a snapshot by ID")
	fmt.Fprintln(w, "  export transparency  Export privacy-safe runway & allocation metrics")
	fmt.Fprintln(w, "  snapshots            List net worth balance snapshots")
	fmt.Fprintln(w, "  insert-commitment    Record a recurring fixed burn expense")
	fmt.Fprintln(w, "  commitments          List recurring financial commitments")
	fmt.Fprintln(w, "  expense              Expense tracking (add, list, summary, ingest, delete)")
	fmt.Fprintln(w, "  insert-expense       Record a single expense transaction")
	fmt.Fprintln(w, "  expenses             List recent expense transactions")
	fmt.Fprintln(w, "  ingest-expenses      Ingest expenses from Markdown story or JSON")
	fmt.Fprintln(w, "  runway               Calculate sovereign runway and wealth metrics")
	fmt.Fprintln(w, "  r2                   Cloudflare R2 sync (status, push, pull, auto)")
	fmt.Fprintln(w, "  sync portfolio       Ingest latest multi-asset portfolio snapshot & runway")
	fmt.Fprintln(w, "  sync gh-projects     Sync GitHub repositories into events.db")
	fmt.Fprintln(w, "  export gh-projects   Export GitHub repositories to github_repos_all.json")
	fmt.Fprintln(w, "  export garden        Export Digital Garden notes (projects, PDRs, retros, gadgets)")
	fmt.Fprintln(w, "  export commerce      Export payment destinations and referrals to nichsedge.github.io")
	fmt.Fprintln(w, "  export check [--fix] Verify that downstream data exports match events.db")
	fmt.Fprintln(w, "  dashboard            Launch web UI dashboard")
	fmt.Fprintln(w, "  audit                Run deep life audit & scan domain health pulses")
	fmt.Fprintln(w, "  projects             List active strategic initiatives")
	fmt.Fprintln(w, "  insert-project       Create a new strategic initiative")
	fmt.Fprintln(w, "  decisions            List decision journal entries (PDR)")
	fmt.Fprintln(w, "  insert-decision      Record a new decision with confidence calibration")
	fmt.Fprintln(w, "  review-decision      Review and calibrate an existing decision outcome")
	fmt.Fprintln(w, "  reviews              List sprint retrospectives")
	fmt.Fprintln(w, "  insert-review        Record a weekly/monthly/quarterly retrospective")
	fmt.Fprintln(w, "  antivision           List active anti-vision negative constraints")
	fmt.Fprintln(w, "  insert-antivision    Record a new anti-vision constraint")
	fmt.Fprintln(w, "  roles                List vocational role explorations")
	fmt.Fprintln(w, "  insert-role          Record a role exploration")
	fmt.Fprintln(w, "  skills               List capability & skills inventory")
	fmt.Fprintln(w, "  insert-skill         Record a skill or capability")
	fmt.Fprintln(w, "  gadgets              List hardware assets")
	fmt.Fprintln(w, "  insert-gadget        Register a hardware device/asset")
	fmt.Fprintln(w, "  vendors              List merchants & service vendors")
	fmt.Fprintln(w, "  insert-vendor        Register a merchant or service vendor")
	fmt.Fprintln(w, "  pay                  List payment destination accounts")
	fmt.Fprintln(w, "  insert-pay           Add a payment destination account")
	fmt.Fprintln(w, "  referrals            List affiliate & referral codes")
	fmt.Fprintln(w, "  insert-referral      Register an affiliate/referral code")
	fmt.Fprintln(w, "  maintenance          List preventive maintenance items")
	fmt.Fprintln(w, "  insert-maintenance   Schedule a preventive maintenance task")
	fmt.Fprintln(w, "  complete-maintenance Mark a maintenance task completed and auto-reschedule")
	fmt.Fprintln(w, "")
}

// Run executes an iERP CLI command with the given arguments, standard output, and standard error streams.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		PrintUsage(stdout)
		return nil
	}

	cmd := args[0]
	cmdArgs := args[1:]

	switch cmd {
	case "init-db":
		return handleInitDB(ctx, cmdArgs, stdout, stderr)
	case "add":
		return handleAdd(ctx, cmdArgs, stdout, stderr)
	case "event", "events":
		return handleEvents(ctx, cmdArgs, stdout, stderr)
	case "insert":
		return handleInsertEvent(ctx, cmdArgs, stdout, stderr)
	case "list":
		return handleListEvents(ctx, cmdArgs, stdout, stderr)
	case "search":
		return handleSearchEvents(ctx, cmdArgs, stdout, stderr)
	case "delete", "delete-event":
		return handleDeleteEvent(ctx, cmdArgs, stdout, stderr)
	case "ingest-events", "ingest-event":
		return handleIngestEvents(ctx, cmdArgs, stdout, stderr)
	case "contacts":
		return handleListContacts(ctx, cmdArgs, stdout, stderr)
	case "insert-contact":
		return handleInsertContact(ctx, cmdArgs, stdout, stderr)
	case "set-tier":
		return handleSetTier(ctx, cmdArgs, stdout, stderr)
	case "radar":
		return handleRadar(ctx, cmdArgs, stdout, stderr)
	case "daily-reconnection":
		return handleDailyReconnection(ctx, cmdArgs, stdout, stderr)
	case "insert-snapshot":
		return handleInsertSnapshot(ctx, cmdArgs, stdout, stderr)
	case "update-snapshot":
		return handleUpdateSnapshot(ctx, cmdArgs, stdout, stderr)
	case "delete-snapshot":
		return handleDeleteSnapshot(ctx, cmdArgs, stdout, stderr)
	case "snapshots":
		return handleListSnapshots(ctx, cmdArgs, stdout, stderr)
	case "insert-commitment":
		return handleInsertCommitment(ctx, cmdArgs, stdout, stderr)
	case "commitments":
		return handleListCommitments(ctx, cmdArgs, stdout, stderr)
	case "expense", "expenses":
		return handleExpenses(ctx, cmdArgs, stdout, stderr)
	case "income":
		return handleIncome(ctx, cmdArgs, stdout, stderr)
	case "insert-expense":
		return handleInsertExpense(ctx, cmdArgs, stdout, stderr)
	case "ingest-expenses", "ingest-expense":
		return handleIngestExpenses(ctx, cmdArgs, stdout, stderr)
	case "runway":
		return handleRunway(ctx, cmdArgs, stdout, stderr)
	case "insert-maintenance":
		return handleInsertMaintenance(ctx, cmdArgs, stdout, stderr)
	case "maintenance":
		return handleListMaintenance(ctx, cmdArgs, stdout, stderr)
	case "complete-maintenance":
		return handleCompleteMaintenance(ctx, cmdArgs, stdout, stderr)
	case "insert-project":
		return handleInsertProject(ctx, cmdArgs, stdout, stderr)
	case "projects":
		return handleListProjects(ctx, cmdArgs, stdout, stderr)
	case "insert-decision":
		return handleInsertDecision(ctx, cmdArgs, stdout, stderr)
	case "decisions":
		return handleListDecisions(ctx, cmdArgs, stdout, stderr)
	case "review-decision":
		return handleReviewDecision(ctx, cmdArgs, stdout, stderr)
	case "insert-review":
		return handleInsertReview(ctx, cmdArgs, stdout, stderr)
	case "reviews":
		return handleListReviews(ctx, cmdArgs, stdout, stderr)
	case "insert-antivision":
		return handleInsertAntiVision(ctx, cmdArgs, stdout, stderr)
	case "antivision":
		return handleListAntiVision(ctx, cmdArgs, stdout, stderr)
	case "update-antivision":
		return handleUpdateAntiVision(ctx, cmdArgs, stdout, stderr)
	case "delete-antivision":
		return handleDeleteAntiVision(ctx, cmdArgs, stdout, stderr)
	case "insert-role":
		return handleInsertRole(ctx, cmdArgs, stdout, stderr)
	case "roles":
		return handleListRoles(ctx, cmdArgs, stdout, stderr)
	case "insert-skill":
		return handleInsertSkill(ctx, cmdArgs, stdout, stderr)
	case "skills":
		return handleListSkills(ctx, cmdArgs, stdout, stderr)
	case "insert-gadget":
		return handleInsertGadget(ctx, cmdArgs, stdout, stderr)
	case "gadgets":
		return handleListGadgets(ctx, cmdArgs, stdout, stderr)
	case "insert-vendor":
		return handleInsertVendor(ctx, cmdArgs, stdout, stderr)
	case "vendors":
		return handleListVendors(ctx, cmdArgs, stdout, stderr)
	case "insert-pay":
		return handleInsertPay(ctx, cmdArgs, stdout, stderr)
	case "pay":
		return handleListPay(ctx, cmdArgs, stdout, stderr)
	case "insert-referral":
		return handleInsertReferral(ctx, cmdArgs, stdout, stderr)
	case "referrals":
		return handleListReferrals(ctx, cmdArgs, stdout, stderr)
	case "audit":
		return handleAudit(ctx, cmdArgs, stdout, stderr)
	case "dashboard":
		return handleDashboard(ctx, cmdArgs, stdout, stderr)
	case "export-garden":
		return handleExportGarden(ctx, cmdArgs, stdout, stderr)
	case "export-commerce":
		return handleExportCommerce(ctx, cmdArgs, stdout, stderr)
	case "export-github", "export-gh-projects":
		return handleExportGitHub(ctx, cmdArgs, stdout, stderr)
	case "export-transparency":
		return handleExportTransparency(ctx, cmdArgs, stdout, stderr)
	case "export":
		return handleExport(ctx, cmdArgs, stdout, stderr)
	case "sync-portfolio":
		return handleSyncPortfolio(ctx, cmdArgs, stdout, stderr)
	case "portfolio":
		portfolio.RunCLI(cmdArgs)
		return nil
	case "sync-github", "sync-gh-projects":
		return handleSyncGitHub(ctx, cmdArgs, stdout, stderr)
	case "sync":
		return handleSync(ctx, cmdArgs, stdout, stderr)
	case "r2":
		return handleR2(ctx, cmdArgs, stdout, stderr)
	case "help", "--help", "-h":
		PrintUsage(stdout)
		return nil
	default:
		fmt.Fprintf(stderr, "%sUnknown command: %s%s\n\n", config.Red, cmd, config.Reset)
		PrintUsage(stderr)
		return fmt.Errorf("unknown command: %s", cmd)
	}
}
