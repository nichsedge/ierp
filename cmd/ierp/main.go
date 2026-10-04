package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nichsedge/ierp/internal/audit"
	"github.com/nichsedge/ierp/internal/commerce"
	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/contacts"
	"github.com/nichsedge/ierp/internal/dashboard"
	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/decisions"
	"github.com/nichsedge/ierp/internal/events"
	"github.com/nichsedge/ierp/internal/finance"
	"github.com/nichsedge/ierp/internal/gadgets"
	"github.com/nichsedge/ierp/internal/garden"
	"github.com/nichsedge/ierp/internal/lifeops"
	"github.com/nichsedge/ierp/internal/projects"
	"github.com/nichsedge/ierp/internal/radar"
	"github.com/nichsedge/ierp/internal/reviews"
	"github.com/nichsedge/ierp/internal/vendors"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "init-db":
		handleInitDB(args)
	case "insert":
		handleInsertEvent(args)
	case "list":
		handleListEvents(args)
	case "search":
		handleSearchEvents(args)
	case "contacts":
		handleListContacts(args)
	case "insert-contact":
		handleInsertContact(args)
	case "set-tier":
		handleSetTier(args)
	case "radar":
		handleRadar(args)
	case "daily-reconnection":
		handleDailyReconnection(args)
	case "insert-snapshot":
		handleInsertSnapshot(args)
	case "snapshots":
		handleListSnapshots(args)
	case "insert-commitment":
		handleInsertCommitment(args)
	case "commitments":
		handleListCommitments(args)
	case "runway":
		handleRunway(args)
	case "insert-maintenance":
		handleInsertMaintenance(args)
	case "maintenance":
		handleListMaintenance(args)
	case "complete-maintenance":
		handleCompleteMaintenance(args)
	case "insert-project":
		handleInsertProject(args)
	case "projects":
		handleListProjects(args)
	case "insert-decision":
		handleInsertDecision(args)
	case "decisions":
		handleListDecisions(args)
	case "review-decision":
		handleReviewDecision(args)
	case "insert-review":
		handleInsertReview(args)
	case "reviews":
		handleListReviews(args)
	case "insert-gadget":
		handleInsertGadget(args)
	case "gadgets":
		handleListGadgets(args)
	case "insert-vendor":
		handleInsertVendor(args)
	case "vendors":
		handleListVendors(args)
	case "insert-pay":
		handleInsertPay(args)
	case "pay":
		handleListPay(args)
	case "insert-referral":
		handleInsertReferral(args)
	case "referrals":
		handleListReferrals(args)
	case "audit":
		handleAudit(args)
	case "dashboard":
		handleDashboard(args)
	case "export-garden":
		handleExportGarden(args)
	case "export-commerce":
		handleExportCommerce(args)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "%sUnknown command: %s%s\n\n", config.Red, cmd, config.Reset)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf("%s%siERP — Individual Enterprise Resource Planning (Go)%s\n\n", config.Bold, config.Cyan, config.Reset)
	fmt.Println("Usage: ierp <command> [flags]")
	fmt.Println("\nCommands:")
	fmt.Println("  init-db              Initialize SQLite schema, FTS5 virtual table, and triggers")
	fmt.Println("  insert               Log a structured timeline event")
	fmt.Println("  list                 List recent events with filters")
	fmt.Println("  search               FTS5 full-text search with BM25 ranking")
	fmt.Println("  contacts             List CRM contacts")
	fmt.Println("  insert-contact       Record a new CRM contact")
	fmt.Println("  set-tier             Update Dunbar tier & cadence for a contact")
	fmt.Println("  radar                Show relationship reconnection radar")
	fmt.Println("  daily-reconnection   Get the top priority relationship needing touchpoint today")
	fmt.Println("  insert-snapshot      Record a balance sheet / net worth snapshot")
	fmt.Println("  snapshots            List net worth balance snapshots")
	fmt.Println("  insert-commitment    Record a recurring fixed burn expense")
	fmt.Println("  commitments          List recurring financial commitments")
	fmt.Println("  runway               Calculate sovereign runway and wealth metrics")
	fmt.Println("  insert-maintenance   Schedule preventive maintenance or document renewal")
	fmt.Println("  maintenance          List scheduled maintenance items")
	fmt.Println("  complete-maintenance Mark maintenance task completed and reschedule")
	fmt.Println("  insert-project       Create a strategic initiative or bet")
	fmt.Println("  projects             List strategic initiatives")
	fmt.Println("  insert-decision      Record a judgment bet in decision journal")
	fmt.Println("  decisions            List decisions with review status")
	fmt.Println("  review-decision      Record retrospective review outcome for a decision")
	fmt.Println("  insert-review        Record a weekly/monthly sprint retrospective")
	fmt.Println("  reviews              List retrospective reviews")
	fmt.Println("  insert-gadget        Register a hardware asset")
	fmt.Println("  gadgets              List registered hardware gadgets")
	fmt.Println("  insert-vendor        Register a merchant or service vendor")
	fmt.Println("  vendors              List merchants and vendors")
	fmt.Println("  insert-pay           Add a payment destination account")
	fmt.Println("  pay                  List payment accounts")
	fmt.Println("  insert-referral      Add an affiliate referral code/link")
	fmt.Println("  referrals            List referral links")
	fmt.Println("  audit                Run deep life audit engine scanning for data gaps")
}

func openDB() *sql.DB {
	database, err := db.Open(config.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError opening database: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}
	return database
}

func handleInitDB(args []string) {
	fs := flag.NewFlagSet("init-db", flag.ExitOnError)
	verbose := fs.Bool("verbose", true, "Print detailed status")
	_ = fs.Parse(args)

	if err := db.InitDB(config.DBPath(), *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "%sFailed to initialize database: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}
}

func handleInsertEvent(args []string) {
	fs := flag.NewFlagSet("insert", flag.ExitOnError)
	title := fs.String("title", "", "Event title (required)")
	place := fs.String("place", "", "Location/place")
	start := fs.String("start", "", "Start date (YYYY-MM-DD)")
	end := fs.String("end", "", "End date (YYYY-MM-DD)")
	tags := fs.String("tags", "", "Comma-separated tags")
	url := fs.String("url", "", "URL reference")
	notes := fs.String("notes", "", "Notes or description")
	contactsList := fs.String("contacts", "", "Comma-separated contact names or IDs")
	projectID := fs.Int("project-id", 0, "Associated project ID")
	_ = fs.Parse(args)

	if *title == "" {
		fmt.Fprintf(os.Stderr, "%sError: --title is required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	var tagSlice []string
	if *tags != "" {
		for _, t := range strings.Split(*tags, ",") {
			if tr := strings.TrimSpace(t); tr != "" {
				tagSlice = append(tagSlice, tr)
			}
		}
	}

	var contactRefs []string
	if *contactsList != "" {
		for _, c := range strings.Split(*contactsList, ",") {
			if cr := strings.TrimSpace(c); cr != "" {
				contactRefs = append(contactRefs, cr)
			}
		}
	}

	var pID *int
	if *projectID > 0 {
		pID = projectID
	}

	ev := events.Event{
		Title:     *title,
		Place:     *place,
		StartDate: *start,
		EndDate:   *end,
		RawDate:   *start,
		Tags:      tagSlice,
		URL:       *url,
		Notes:     *notes,
		ProjectID: pID,
	}

	id, linked, err := events.InsertEvent(database, ev, contactRefs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sFailed to insert event: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sEvent #%d logged:%s %s\n", config.Green, id, config.Reset, *title)
	if len(linked) > 0 {
		fmt.Printf("  Linked contacts: %s\n", strings.Join(linked, ", "))
	}
}

func handleListEvents(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	limit := fs.Int("limit", 20, "Max events to list")
	tag := fs.String("tag", "", "Filter by tag")
	query := fs.String("q", "", "Filter by query")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	evs, total, err := events.ListEvents(database, events.ListOptions{
		Limit: *limit,
		Tag:   *tag,
		Query: *query,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Events (%d total) ===%s\n", config.Bold, total, config.Reset)
	for _, e := range evs {
		dateStr := e.StartDate
		if dateStr == "" {
			dateStr = e.RawDate
		}
		fmt.Printf("#%d | %s | %s%s%s", e.ID, dateStr, config.Cyan, e.Title, config.Reset)
		if e.Place != "" {
			fmt.Printf(" @ %s", e.Place)
		}
		if len(e.Contacts) > 0 {
			var names []string
			for _, c := range e.Contacts {
				names = append(names, c.Name)
			}
			fmt.Printf(" [%s]", strings.Join(names, ", "))
		}
		fmt.Println()
	}
}

func handleSearchEvents(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	limit := fs.Int("limit", 20, "Max results")
	_ = fs.Parse(args)

	query := strings.Join(fs.Args(), " ")
	if query == "" {
		fmt.Fprintf(os.Stderr, "%sError: search query required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	results, err := events.SearchEvents(database, query, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError searching: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sSearch results for '%s' (%d hits):%s\n", config.Bold, query, len(results), config.Reset)
	for _, r := range results {
		fmt.Printf("  #%d | %s | %s%s%s (rank: %.2f)\n", r.ID, r.StartDate, config.Cyan, r.Title, config.Reset, r.Rank)
		if r.NotesSnip != "" {
			fmt.Printf("     Snippet: %s\n", r.NotesSnip)
		}
	}
}

func handleListContacts(args []string) {
	fs := flag.NewFlagSet("contacts", flag.ExitOnError)
	limit := fs.Int("limit", 50, "Limit")
	q := fs.String("q", "", "Search query")
	tierVal := fs.Int("tier", -1, "Filter by tier (0-3)")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	var t *int
	if *tierVal >= 0 {
		t = tierVal
	}

	list, total, err := contacts.ListContacts(database, contacts.ListOptions{
		Limit: *limit,
		Query: *q,
		Tier:  t,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Contacts (%d total) ===%s\n", config.Bold, total, config.Reset)
	for _, c := range list {
		tierBadge := fmt.Sprintf("Tier %d (%dd)", c.Tier, c.CadenceDays)
		fmt.Printf("#%-3d | %-24s | %-12s | %-15s | %d events\n", c.ID, c.Name, tierBadge, c.Org, c.EventCount)
	}
}

func handleInsertContact(args []string) {
	fs := flag.NewFlagSet("insert-contact", flag.ExitOnError)
	name := fs.String("name", "", "Contact name (required)")
	org := fs.String("org", "", "Organization/Company")
	client := fs.String("client", "", "Client relation")
	location := fs.String("location", "", "Location")
	email := fs.String("email", "", "Email")
	phone := fs.String("phone", "", "Phone")
	notes := fs.String("notes", "", "Notes")
	tier := fs.Int("tier", 0, "Dunbar tier (0=Untracked, 1=14d, 2=60d, 3=180d)")
	cadence := fs.Int("cadence", 0, "Custom touch cadence in days")
	_ = fs.Parse(args)

	if *name == "" {
		fmt.Fprintf(os.Stderr, "%sError: --name is required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	id, err := contacts.InsertContact(database, contacts.Contact{
		Name:        *name,
		Org:         *org,
		Client:      *client,
		Location:    *location,
		Email:       *email,
		Phone:       *phone,
		Notes:       *notes,
		Tier:        *tier,
		CadenceDays: *cadence,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sContact #%d saved: %s%s\n", config.Green, id, *name, config.Reset)
}

func handleSetTier(args []string) {
	fs := flag.NewFlagSet("set-tier", flag.ExitOnError)
	contactID := fs.Int("contact-id", 0, "Contact ID (required)")
	tier := fs.Int("tier", 0, "Dunbar tier (0-3)")
	cadence := fs.Int("cadence", 0, "Cadence days")
	_ = fs.Parse(args)

	if *contactID <= 0 {
		fmt.Fprintf(os.Stderr, "%sError: --contact-id is required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	days := *cadence
	if days <= 0 {
		days = contacts.DefaultCadenceByTier[*tier]
	}

	updates := map[string]any{
		"tier":         *tier,
		"cadence_days": days,
	}
	ok, err := contacts.UpdateContact(database, *contactID, updates)
	if err != nil || !ok {
		fmt.Fprintf(os.Stderr, "%sFailed to update contact #%d: %v%s\n", config.Red, *contactID, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sUpdated contact #%d: Tier %d (%d days cadence)%s\n", config.Green, *contactID, *tier, days, config.Reset)
}

func handleRadar(args []string) {
	fs := flag.NewFlagSet("radar", flag.ExitOnError)
	tierVal := fs.Int("tier", 0, "Filter by tier (1, 2, or 3)")
	overdueOnly := fs.Bool("overdue-only", false, "Show only overdue relationships")
	limit := fs.Int("limit", 50, "Limit")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	var t *int
	if *tierVal > 0 {
		t = tierVal
	}

	items, err := radar.ComputeRadar(database, t, *overdueOnly, *limit, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Reconnection Radar (%d contacts) ===%s\n", config.Bold, len(items), config.Reset)
	for _, it := range items {
		status := fmt.Sprintf("%dd ago", it.DaysSinceLastTouch)
		if it.IsOverdue {
			status = fmt.Sprintf("%sOVERDUE (%dd late)%s", config.Red, it.DaysOverdue, config.Reset)
		}
		fmt.Printf("Tier %d | #%-3d | %-22s | %-14s | %s\n", it.Tier, it.ID, it.Name, fmt.Sprintf("cadence: %dd", it.CadenceDays), status)
	}
}

func handleDailyReconnection(args []string) {
	database := openDB()
	defer database.Close()

	item, err := radar.GetDailyReconnection(database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	if item == nil {
		fmt.Printf("%sAll relationships are currently within target cadence. Radar clear!%s\n", config.Green, config.Reset)
		return
	}

	fmt.Printf("%s🎯 Daily Reconnection Priority:%s\n", config.Bold, config.Reset)
	fmt.Printf("  %s%s%s (Tier %d)\n", config.Cyan, item.Name, config.Reset, item.Tier)
	fmt.Printf("  Cadence: every %d days | Overdue by: %s%d days%s\n", item.CadenceDays, config.Red, item.DaysOverdue, config.Reset)
	if item.LastSeenDate != "" {
		fmt.Printf("  Last seen: %s\n", item.LastSeenDate)
	}
	if item.Org != "" {
		fmt.Printf("  Organization: %s\n", item.Org)
	}
}

func handleInsertSnapshot(args []string) {
	fs := flag.NewFlagSet("insert-snapshot", flag.ExitOnError)
	dateStr := fs.String("date", "", "Snapshot date (YYYY-MM-DD)")
	liquid := fs.Float64("liquid", 0, "Liquid cash reserves")
	invest := fs.Float64("investments", 0, "Investments")
	assets := fs.Float64("assets", 0, "Hard assets")
	liab := fs.Float64("liabilities", 0, "Liabilities/Debt")
	currency := fs.String("currency", "IDR", "Currency")
	notes := fs.String("notes", "", "Notes")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	id, err := finance.InsertSnapshot(database, finance.Snapshot{
		SnapshotDate: *dateStr,
		LiquidCash:   *liquid,
		Investments:  *invest,
		HardAssets:   *assets,
		Liabilities:  *liab,
		Currency:     *currency,
		Notes:        *notes,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	netWorth := (*liquid + *invest + *assets) - *liab
	fmt.Printf("%sBalance sheet snapshot #%d saved!%s Net Worth: %s %.2f\n", config.Green, id, config.Reset, *currency, netWorth)
}

func handleListSnapshots(args []string) {
	fs := flag.NewFlagSet("snapshots", flag.ExitOnError)
	limit := fs.Int("limit", 12, "Limit")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	snaps, err := finance.ListSnapshots(database, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Balance Sheet Snapshots ===%s\n", config.Bold, config.Reset)
	for _, s := range snaps {
		fmt.Printf("%s | Net Worth: %s %14.2f | Liquid: %12.2f | Invest: %12.2f\n",
			s.SnapshotDate, s.Currency, s.NetWorth, s.LiquidCash, s.Investments)
	}
}

func handleInsertCommitment(args []string) {
	fs := flag.NewFlagSet("insert-commitment", flag.ExitOnError)
	name := fs.String("name", "", "Name of recurring expense (required)")
	amount := fs.Float64("amount", 0, "Amount (required)")
	cat := fs.String("category", "saas", "Category (saas, rent, utilities, insurance, etc.)")
	freq := fs.String("frequency", "monthly", "Frequency (monthly, yearly, quarterly, weekly)")
	cur := fs.String("currency", "IDR", "Currency")
	notes := fs.String("notes", "", "Notes")
	_ = fs.Parse(args)

	if *name == "" || *amount <= 0 {
		fmt.Fprintf(os.Stderr, "%sError: --name and --amount (>0) are required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	id, err := finance.InsertCommitment(database, finance.Commitment{
		Name:      *name,
		Amount:    *amount,
		Category:  *cat,
		Frequency: *freq,
		Currency:  *cur,
		Notes:     *notes,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sRecurring commitment #%d saved: %s (%.2f / %s)%s\n", config.Green, id, *name, *amount, *freq, config.Reset)
}

func handleListCommitments(args []string) {
	fs := flag.NewFlagSet("commitments", flag.ExitOnError)
	status := fs.String("status", "active", "Filter by status")
	cat := fs.String("category", "", "Filter by category")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	comms, err := finance.ListCommitments(database, *status, *cat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Recurring Commitments (%d active) ===%s\n", config.Bold, len(comms), config.Reset)
	var totalMonthly float64
	for _, c := range comms {
		totalMonthly += c.MonthlyAmount
		fmt.Printf("#%-3d | %-24s | %-12s | %s %12.2f (%s)\n", c.ID, c.Name, c.Category, c.Currency, c.Amount, c.Frequency)
	}
	fmt.Printf("\n%sTotal Monthly Recurring Burn:%s IDR %.2f\n", config.Bold, config.Reset, totalMonthly)
}

func handleRunway(args []string) {
	database := openDB()
	defer database.Close()

	runway, err := finance.ComputeRunway(database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Sovereign Runway & Treasury Health ===%s\n", config.Bold, config.Reset)
	fmt.Printf("  Liquid Cash:         IDR %14.2f\n", runway.LiquidCash)
	fmt.Printf("  Net Worth:           IDR %14.2f\n", runway.NetWorth)
	fmt.Printf("  Monthly Fixed Burn:  IDR %14.2f (%d commitments)\n", runway.MonthlyBurn, runway.CommitmentsCount)
	fmt.Println("  ---------------------------------------------")
	if runway.IsInfinite {
		fmt.Printf("  Runway:              %s%s (No active fixed burn)%s\n", config.Green, runway.StatusLabel, config.Reset)
	} else {
		color := config.Green
		if runway.RunwayMonths < 6 {
			color = config.Red
		} else if runway.RunwayMonths < 12 {
			color = config.Yellow
		}
		fmt.Printf("  Runway:              %s%.1f Months [%s]%s\n", color, runway.RunwayMonths, runway.StatusLabel, config.Reset)
	}

	if len(runway.BurnByCategory) > 0 {
		fmt.Println("\n  Burn Breakdown by Category:")
		for cat, amt := range runway.BurnByCategory {
			fmt.Printf("    %-16s: IDR %12.2f\n", cat, amt)
		}
	}
}

func handleInsertMaintenance(args []string) {
	fs := flag.NewFlagSet("insert-maintenance", flag.ExitOnError)
	name := fs.String("name", "", "Task name (required)")
	dueDate := fs.String("due-date", "", "Due date (YYYY-MM-DD, required)")
	cat := fs.String("category", "general", "Category")
	interval := fs.Int("interval", 0, "Recurring interval in days")
	cost := fs.Float64("cost", 0, "Estimated cost")
	notes := fs.String("notes", "", "Notes")
	gadgetID := fs.Int("gadget-id", 0, "Associated gadget ID")
	_ = fs.Parse(args)

	if *name == "" || *dueDate == "" {
		fmt.Fprintf(os.Stderr, "%sError: --name and --due-date are required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	var intPtr *int
	if *interval > 0 {
		intPtr = interval
	}
	var gPtr *int
	if *gadgetID > 0 {
		gPtr = gadgetID
	}

	id, err := lifeops.InsertMaintenance(database, lifeops.MaintenanceItem{
		Name:         *name,
		DueDate:      *dueDate,
		Category:     *cat,
		IntervalDays: intPtr,
		Cost:         *cost,
		Notes:        *notes,
		GadgetID:     gPtr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sMaintenance task #%d scheduled for %s: %s%s\n", config.Green, id, *dueDate, *name, config.Reset)
}

func handleListMaintenance(args []string) {
	fs := flag.NewFlagSet("maintenance", flag.ExitOnError)
	status := fs.String("status", "pending", "Status (pending, completed, all)")
	cat := fs.String("category", "", "Category")
	overdueOnly := fs.Bool("overdue-only", false, "Show only overdue items")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	items, err := lifeops.ListMaintenance(database, *status, *cat, *overdueOnly)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Preventive Maintenance (%d items) ===%s\n", config.Bold, len(items), config.Reset)
	for _, it := range items {
		dueStatus := it.DueDate
		if it.IsOverdue {
			dueStatus = fmt.Sprintf("%sOVERDUE by %dd (%s)%s", config.Red, it.DaysOverdue, it.DueDate, config.Reset)
		}
		fmt.Printf("#%-3d | %-26s | %-12s | %s\n", it.ID, it.Name, it.Category, dueStatus)
	}
}

func handleCompleteMaintenance(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "%sUsage: ierp complete-maintenance <id> [--date YYYY-MM-DD] [--cost <amount>]%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sInvalid ID: %s%s\n", config.Red, args[0], config.Reset)
		os.Exit(1)
	}

	fs := flag.NewFlagSet("complete-maintenance", flag.ExitOnError)
	dateStr := fs.String("date", "", "Completion date (YYYY-MM-DD)")
	cost := fs.Float64("cost", -1, "Actual cost incurred")
	_ = fs.Parse(args[1:])

	database := openDB()
	defer database.Close()

	var costPtr *float64
	if *cost >= 0 {
		costPtr = cost
	}

	res, err := lifeops.CompleteMaintenance(database, id, *dateStr, costPtr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	if success, _ := res["success"].(bool); !success {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, res["error"], config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sTask #%d marked completed!%s\n", config.Green, id, config.Reset)
	if rescheduled, _ := res["rescheduled"].(bool); rescheduled {
		fmt.Printf("  Auto-rescheduled next occurrence for %s%v%s (#%v)\n",
			config.Cyan, res["next_due_date"], config.Reset, res["next_item_id"])
	}
}

func handleInsertProject(args []string) {
	fs := flag.NewFlagSet("insert-project", flag.ExitOnError)
	title := fs.String("title", "", "Project title (required)")
	slug := fs.String("slug", "", "Unique slug")
	desc := fs.String("desc", "", "Description")
	priority := fs.String("priority", "medium", "Priority (low, medium, high)")
	start := fs.String("start", "", "Start date (YYYY-MM-DD)")
	target := fs.String("target", "", "Target date (YYYY-MM-DD)")
	_ = fs.Parse(args)

	if *title == "" {
		fmt.Fprintf(os.Stderr, "%sError: --title is required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	id, err := projects.InsertProject(database, projects.Project{
		Title:       *title,
		Slug:        *slug,
		Description: *desc,
		Priority:    *priority,
		StartDate:   *start,
		TargetDate:  *target,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sProject #%d saved: %s%s\n", config.Green, id, *title, config.Reset)
}

func handleListProjects(args []string) {
	fs := flag.NewFlagSet("projects", flag.ExitOnError)
	status := fs.String("status", "active", "Filter status (active, completed, all)")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	projs, err := projects.ListProjects(database, *status)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Strategic Projects (%d items) ===%s\n", config.Bold, len(projs), config.Reset)
	for _, p := range projs {
		fmt.Printf("#%-3d | %-26s | %-8s | %-8s | %s\n", p.ID, p.Title, p.Priority, p.Status, p.Slug)
	}
}

func handleInsertDecision(args []string) {
	fs := flag.NewFlagSet("insert-decision", flag.ExitOnError)
	title := fs.String("title", "", "Decision title (required)")
	choice := fs.String("choice", "", "Chosen path/option (required)")
	context := fs.String("context", "", "Context and reasoning")
	expected := fs.String("expected", "", "Expected outcome")
	confidence := fs.Int("confidence", 7, "Confidence score (1-10)")
	reviewDate := fs.String("review-date", "", "Scheduled review date (YYYY-MM-DD)")
	projID := fs.Int("project-id", 0, "Associated project ID")
	_ = fs.Parse(args)

	if *title == "" || *choice == "" {
		fmt.Fprintf(os.Stderr, "%sError: --title and --choice are required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	var pPtr *int
	if *projID > 0 {
		pPtr = projID
	}

	id, err := decisions.InsertDecision(database, decisions.Decision{
		Title:           *title,
		Choice:          *choice,
		Context:         *context,
		ExpectedOutcome: *expected,
		Confidence:      *confidence,
		ReviewDate:      *reviewDate,
		ProjectID:       pPtr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sDecision #%d recorded in journal: %s%s\n", config.Green, id, *title, config.Reset)
}

func handleListDecisions(args []string) {
	fs := flag.NewFlagSet("decisions", flag.ExitOnError)
	status := fs.String("status", "", "Filter status (pending, reviewed, all)")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	list, err := decisions.ListDecisions(database, *status, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Decision Journal (%d entries) ===%s\n", config.Bold, len(list), config.Reset)
	for _, d := range list {
		reviewStr := d.ReviewDate
		if reviewStr == "" {
			reviewStr = "no review date"
		}
		fmt.Printf("#%-3d | %-28s | %-9s | review: %s | conf: %d/10\n", d.ID, d.Title, d.Status, reviewStr, d.Confidence)
	}
}

func handleReviewDecision(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "%sUsage: ierp review-decision <id> --outcome <results>%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sInvalid ID: %s%s\n", config.Red, args[0], config.Reset)
		os.Exit(1)
	}

	fs := flag.NewFlagSet("review-decision", flag.ExitOnError)
	outcome := fs.String("outcome", "", "Actual outcome and retrospective analysis (required)")
	status := fs.String("status", "reviewed", "New status")
	_ = fs.Parse(args[1:])

	if *outcome == "" {
		fmt.Fprintf(os.Stderr, "%sError: --outcome is required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	ok, err := decisions.ReviewDecision(database, id, *outcome, *status)
	if err != nil || !ok {
		fmt.Fprintf(os.Stderr, "%sFailed to update decision #%d: %v%s\n", config.Red, id, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sDecision #%d reviewed and calibrated successfully!%s\n", config.Green, id, config.Reset)
}

func handleInsertReview(args []string) {
	fs := flag.NewFlagSet("insert-review", flag.ExitOnError)
	start := fs.String("start", "", "Period start date (YYYY-MM-DD, required)")
	end := fs.String("end", "", "Period end date (YYYY-MM-DD, required)")
	pType := fs.String("type", "weekly", "Period type (weekly, monthly, quarterly)")
	wins := fs.String("wins", "", "Wins and accomplishments")
	drains := fs.String("drains", "", "Energy drains and burnout points")
	lessons := fs.String("lessons", "", "Key lessons learned")
	focus := fs.String("focus", "", "Focus for next sprint")
	rating := fs.Int("rating", 7, "Rating (1-10)")
	notes := fs.String("notes", "", "Additional notes")
	_ = fs.Parse(args)

	if *start == "" || *end == "" {
		fmt.Fprintf(os.Stderr, "%sError: --start and --end dates are required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	id, err := reviews.InsertRetrospective(database, reviews.Retrospective{
		PeriodStart:   *start,
		PeriodEnd:     *end,
		PeriodType:    *pType,
		Wins:          *wins,
		DrainsBurnout: *drains,
		Lessons:       *lessons,
		FocusNext:     *focus,
		Rating:        *rating,
		Notes:         *notes,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sRetrospective #%d recorded (%s to %s)%s\n", config.Green, id, *start, *end, config.Reset)
}

func handleListReviews(args []string) {
	fs := flag.NewFlagSet("reviews", flag.ExitOnError)
	pType := fs.String("type", "", "Filter type")
	limit := fs.Int("limit", 10, "Limit")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	list, err := reviews.ListRetrospectives(database, *pType, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Retrospectives (%d entries) ===%s\n", config.Bold, len(list), config.Reset)
	for _, r := range list {
		fmt.Printf("#%-3d | %-9s | %s to %s | rating: %d/10\n", r.ID, r.PeriodType, r.PeriodStart, r.PeriodEnd, r.Rating)
	}
}

func handleInsertGadget(args []string) {
	fs := flag.NewFlagSet("insert-gadget", flag.ExitOnError)
	name := fs.String("name", "", "Gadget name (required)")
	brand := fs.String("brand", "", "Brand")
	model := fs.String("model", "", "Model")
	cat := fs.String("category", "computing", "Category")
	price := fs.Float64("price", 0, "Purchase price")
	date := fs.String("date", "", "Purchase date (YYYY-MM-DD)")
	serial := fs.String("serial", "", "Serial number")
	_ = fs.Parse(args)

	if *name == "" {
		fmt.Fprintf(os.Stderr, "%sError: --name is required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	id, err := gadgets.InsertGadget(database, gadgets.Gadget{
		Name:          *name,
		Brand:         *brand,
		Model:         *model,
		Category:      *cat,
		PurchasePrice: *price,
		PurchaseDate:  *date,
		SerialNumber:  *serial,
		IsPublic:      true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sHardware asset #%d saved: %s%s\n", config.Green, id, *name, config.Reset)
}

func handleListGadgets(args []string) {
	fs := flag.NewFlagSet("gadgets", flag.ExitOnError)
	cat := fs.String("category", "", "Filter category")
	status := fs.String("status", "active", "Filter status")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	list, err := gadgets.ListGadgets(database, *cat, *status)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Hardware Assets (%d items) ===%s\n", config.Bold, len(list), config.Reset)
	for _, g := range list {
		fmt.Printf("#%-3d | %-24s | %-12s | %-12s | %s %.2f\n", g.ID, g.Name, g.Brand, g.Category, g.Currency, g.PurchasePrice)
	}
}

func handleInsertVendor(args []string) {
	fs := flag.NewFlagSet("insert-vendor", flag.ExitOnError)
	name := fs.String("name", "", "Vendor name (required)")
	cat := fs.String("category", "general", "Category")
	location := fs.String("location", "", "Location")
	phone := fs.String("phone", "", "Phone")
	email := fs.String("email", "", "Email")
	url := fs.String("url", "", "URL")
	fav := fs.Bool("favorite", false, "Mark as favorite")
	notes := fs.String("notes", "", "Notes")
	_ = fs.Parse(args)

	if *name == "" {
		fmt.Fprintf(os.Stderr, "%sError: --name is required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	id, err := vendors.InsertVendor(database, vendors.Vendor{
		Name:     *name,
		Category: *cat,
		Location: *location,
		Phone:    *phone,
		Email:    *email,
		URL:      *url,
		Favorite: *fav,
		Notes:    *notes,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sVendor #%d registered: %s%s\n", config.Green, id, *name, config.Reset)
}

func handleListVendors(args []string) {
	fs := flag.NewFlagSet("vendors", flag.ExitOnError)
	cat := fs.String("category", "", "Filter category")
	favOnly := fs.Bool("favorites", false, "Favorites only")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	list, err := vendors.ListVendors(database, *cat, *favOnly)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Merchants & Vendors (%d items) ===%s\n", config.Bold, len(list), config.Reset)
	for _, v := range list {
		favBadge := " "
		if v.Favorite {
			favBadge = "★"
		}
		fmt.Printf("%s #%-3d | %-24s | %-15s | %s\n", favBadge, v.ID, v.Name, v.Category, v.Location)
	}
}

func handleInsertPay(args []string) {
	fs := flag.NewFlagSet("insert-pay", flag.ExitOnError)
	name := fs.String("name", "", "Account name (required)")
	cat := fs.String("category", "bank", "Category (bank, ewallet, crypto)")
	number := fs.String("number", "", "Account/wallet number")
	recipient := fs.String("recipient", "", "Beneficiary name")
	details := fs.String("details", "", "Details")
	_ = fs.Parse(args)

	if *name == "" {
		fmt.Fprintf(os.Stderr, "%sError: --name is required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	id, err := commerce.InsertPaymentAccount(database, commerce.PaymentAccount{
		Name:      *name,
		Category:  *cat,
		Number:    *number,
		Recipient: *recipient,
		Details:   *details,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sPayment destination #%d saved: %s%s\n", config.Green, id, *name, config.Reset)
}

func handleListPay(args []string) {
	fs := flag.NewFlagSet("pay", flag.ExitOnError)
	cat := fs.String("category", "", "Category")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	list, err := commerce.ListPaymentAccounts(database, *cat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Payment Accounts (%d accounts) ===%s\n", config.Bold, len(list), config.Reset)
	for _, p := range list {
		fmt.Printf("#%-3d | %-20s | %-10s | %s (%s)\n", p.ID, p.Name, p.Category, p.Number, p.Recipient)
	}
}

func handleInsertReferral(args []string) {
	fs := flag.NewFlagSet("insert-referral", flag.ExitOnError)
	name := fs.String("name", "", "Name/Service (required)")
	cat := fs.String("category", "general", "Category")
	code := fs.String("code", "", "Referral code")
	link := fs.String("link", "", "Referral link")
	benefit := fs.String("benefit", "", "User benefit")
	_ = fs.Parse(args)

	if *name == "" {
		fmt.Fprintf(os.Stderr, "%sError: --name is required%s\n", config.Red, config.Reset)
		os.Exit(1)
	}

	database := openDB()
	defer database.Close()

	id, err := commerce.InsertReferral(database, commerce.Referral{
		Name:     *name,
		Category: *cat,
		Code:     *code,
		Link:     *link,
		Benefit:  *benefit,
		IsPublic: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sReferral #%d registered: %s%s\n", config.Green, id, *name, config.Reset)
}

func handleListReferrals(args []string) {
	fs := flag.NewFlagSet("referrals", flag.ExitOnError)
	cat := fs.String("category", "", "Category")
	status := fs.String("status", "ACTIVE", "Status")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	list, err := commerce.ListReferrals(database, *cat, *status)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== Affiliate Referrals (%d active) ===%s\n", config.Bold, len(list), config.Reset)
	for _, r := range list {
		fmt.Printf("#%-3d | %-20s | Code: %-15s | %s\n", r.ID, r.Name, r.Code, r.Benefit)
	}
}

func handleAudit(args []string) {
	database := openDB()
	defer database.Close()

	result, err := audit.RunAudit(database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError running life audit: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%s=== iERP Life Audit & Pulse Report (%s) ===%s\n", config.Bold, result.GeneratedAt, config.Reset)
	fmt.Printf("Total Findings: %d (Action Needed: %d, Warnings: %d, Tips: %d)\n\n",
		result.TotalFindings, result.ActionsNeeded, result.Warnings, result.Tips)

	for _, f := range result.Findings {
		var icon, color string
		switch f.Severity {
		case "action_needed":
			icon = "🚨"
			color = config.Red
		case "warning":
			icon = "⚠️ "
			color = config.Yellow
		default:
			icon = "💡"
			color = config.Cyan
		}
		fmt.Printf("%s %s[%s] %s%s\n", icon, color, f.Domain, f.Title, config.Reset)
		fmt.Printf("   %s\n", f.Description)
		if f.Command != "" {
			fmt.Printf("   %sFix:%s %s\n", config.Bold, config.Reset, f.Command)
		}
		fmt.Println()
	}
}

func handleDashboard(args []string) {
	fs := flag.NewFlagSet("dashboard", flag.ExitOnError)
	host := fs.String("host", "0.0.0.0", "Host address")
	port := fs.Int("port", 8921, "Port number")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	server := dashboard.NewServer(database, *host, *port)
	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "%sDashboard server failed: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}
}

func handleExportGarden(args []string) {
	fs := flag.NewFlagSet("export-garden", flag.ExitOnError)
	gardenPath := fs.String("path", "", "Target Digital Garden content directory")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	summary, err := garden.ExportAll(database, *gardenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sGarden export failed: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sDigital Garden Deep Export Complete!%s\n", config.Green, config.Reset)
	fmt.Printf("  Strategic Initiatives:  %d notes\n", summary.Projects)
	fmt.Printf("  Decision Journal (PDR): %d notes\n", summary.Decisions)
	fmt.Printf("  Sprint Retrospectives:  %d notes\n", summary.Retrospectives)
	fmt.Printf("  Hardware Gadgets:       %d notes\n", summary.Gadgets)
}

func handleExportCommerce(args []string) {
	fs := flag.NewFlagSet("export-commerce", flag.ExitOnError)
	dataDir := fs.String("path", "", "Target portfolio data directory")
	_ = fs.Parse(args)

	database := openDB()
	defer database.Close()

	payCount, refCount, err := commerce.ExportCommerceFiles(database, *dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sCommerce export failed: %v%s\n", config.Red, err, config.Reset)
		os.Exit(1)
	}

	fmt.Printf("%sCommerce Data Export Complete!%s\n", config.Green, config.Reset)
	fmt.Printf("  Payment Accounts: %d accounts -> pay.json\n", payCount)
	fmt.Printf("  Referral Codes:   %d active links -> referrals.json\n", refCount)
}
