package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/finance"
)

func snapshotDeviation(prev *finance.Snapshot, liquid float64) float64 {
	if prev == nil || prev.LiquidCash <= 0 {
		return 0
	}
	return finance.Deviation(prev.LiquidCash, liquid)
}

func printDeviationWarning(stderr io.Writer, prev *finance.Snapshot, liquid, dev float64) {
	fmt.Fprintf(stderr, "%s⚠️  Refusing snapshot: liquid cash deviates %.0f%% from the previous one.%s\n",
		config.Red, dev*100, config.Reset)
	fmt.Fprintf(stderr, "   Previous (%s): %s %.2f\n", prev.SnapshotDate, prev.Currency, prev.LiquidCash)
	fmt.Fprintf(stderr, "   New:              %s %.2f\n", prev.Currency, liquid)
	fmt.Fprintf(stderr, "   If this is intentional, re-run with --force.\n")
}

func handleInsertSnapshot(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-snapshot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dateStr := fs.String("date", "", "Snapshot date (YYYY-MM-DD)")
	liquid := fs.Float64("liquid", 0, "Liquid cash reserves")
	invest := fs.Float64("investments", 0, "Investments")
	assets := fs.Float64("assets", 0, "Hard assets")
	liab := fs.Float64("liabilities", 0, "Liabilities/Debt")
	currency := fs.String("currency", "IDR", "Currency")
	notes := fs.String("notes", "", "Notes")
	force := fs.Bool("force", false, "Bypass the deviation guard for a large swing in liquid cash")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	prev, err := finance.GetLatestSnapshot(database)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	if dev := snapshotDeviation(prev, *liquid); dev > finance.MaxSnapshotDeviation {
		if !*force {
			printDeviationWarning(stderr, prev, *liquid, dev)
			return fmt.Errorf("liquid cash deviation exceeds %.0f%% threshold", finance.MaxSnapshotDeviation*100)
		}
		fmt.Fprintf(stderr, "%s⚠️  --force used: recording a %.0f%% deviation from %s%s\n",
			config.Yellow, dev*100, prev.SnapshotDate, config.Reset)
	}

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
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	netWorth := (*liquid + *invest + *assets) - *liab
	fmt.Fprintf(stdout, "%sBalance sheet snapshot #%d saved!%s Net Worth: %s %.2f\n", config.Green, id, config.Reset, *currency, netWorth)
	return nil
}

func handleUpdateSnapshot(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("update-snapshot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.Int("id", 0, "Snapshot ID to correct (required)")
	dateStr := fs.String("date", "", "Snapshot date (YYYY-MM-DD)")
	liquid := fs.Float64("liquid", 0, "Liquid cash reserves")
	invest := fs.Float64("investments", 0, "Investments")
	assets := fs.Float64("assets", 0, "Hard assets")
	liab := fs.Float64("liabilities", 0, "Liabilities/Debt")
	currency := fs.String("currency", "IDR", "Currency")
	notes := fs.String("notes", "", "Notes")
	force := fs.Bool("force", false, "Bypass the deviation guard for a large swing in liquid cash")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *id <= 0 {
		fmt.Fprintf(stderr, "%sError: --id is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("id is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	snaps, err := finance.ListSnapshots(database, 500)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	targetIndex := -1
	for i := range snaps {
		if snaps[i].ID == *id {
			targetIndex = i
			break
		}
	}
	if targetIndex < 0 {
		fmt.Fprintf(stderr, "%sError: snapshot #%d not found%s\n", config.Red, *id, config.Reset)
		return fmt.Errorf("snapshot #%d not found", *id)
	}

	updated := snaps[targetIndex]
	provided := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if provided["date"] {
		updated.SnapshotDate = *dateStr
	}
	if provided["liquid"] {
		updated.LiquidCash = *liquid
	}
	if provided["investments"] {
		updated.Investments = *invest
	}
	if provided["assets"] {
		updated.HardAssets = *assets
	}
	if provided["liabilities"] {
		updated.Liabilities = *liab
	}
	if provided["currency"] {
		updated.Currency = *currency
	}
	if provided["notes"] {
		updated.Notes = *notes
	}

	var prev *finance.Snapshot
	if targetIndex+1 < len(snaps) {
		prev = &snaps[targetIndex+1]
	}
	if dev := snapshotDeviation(prev, updated.LiquidCash); dev > finance.MaxSnapshotDeviation {
		if !*force {
			printDeviationWarning(stderr, prev, updated.LiquidCash, dev)
			return fmt.Errorf("liquid cash deviation exceeds %.0f%% threshold", finance.MaxSnapshotDeviation*100)
		}
		fmt.Fprintf(stderr, "%s⚠️  --force used: recording a %.0f%% deviation from %s%s\n",
			config.Yellow, dev*100, prev.SnapshotDate, config.Reset)
	}

	ok, err := finance.UpdateSnapshot(database, *id, updated)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	if !ok {
		fmt.Fprintf(stderr, "%sError: snapshot #%d not found%s\n", config.Red, *id, config.Reset)
		return fmt.Errorf("snapshot #%d not found", *id)
	}

	netWorth := (updated.LiquidCash + updated.Investments + updated.HardAssets) - updated.Liabilities
	fmt.Fprintf(stdout, "%sSnapshot #%d updated!%s %s | Net Worth: %s %.2f\n",
		config.Green, *id, config.Reset, updated.SnapshotDate, updated.Currency, netWorth)
	return nil
}

func handleDeleteSnapshot(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("delete-snapshot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.Int("id", 0, "Snapshot ID to void (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *id <= 0 {
		fmt.Fprintf(stderr, "%sError: --id is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("id is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	ok, err := finance.DeleteSnapshot(database, *id)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	if !ok {
		fmt.Fprintf(stderr, "%sError: snapshot #%d not found%s\n", config.Red, *id, config.Reset)
		return fmt.Errorf("snapshot #%d not found", *id)
	}

	fmt.Fprintf(stdout, "%sSnapshot #%d voided%s\n", config.Green, *id, config.Reset)
	return nil
}

func handleListSnapshots(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("snapshots", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 12, "Limit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	snaps, err := finance.ListSnapshots(database, *limit)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Balance Sheet Snapshots ===%s\n", config.Bold, config.Reset)
	for _, s := range snaps {
		fmt.Fprintf(stdout, "%s | Net Worth: %s %14.2f | Liquid: %12.2f | Invest: %12.2f\n",
			s.SnapshotDate, s.Currency, s.NetWorth, s.LiquidCash, s.Investments)
	}
	return nil
}

func handleInsertCommitment(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-commitment", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "Name of recurring expense (required)")
	amount := fs.Float64("amount", 0, "Amount (required)")
	cat := fs.String("category", "saas", "Category (saas, rent, utilities, insurance, etc.)")
	freq := fs.String("frequency", "monthly", "Frequency (monthly, yearly, quarterly, weekly)")
	cur := fs.String("currency", "IDR", "Currency")
	notes := fs.String("notes", "", "Notes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" || *amount <= 0 {
		fmt.Fprintf(stderr, "%sError: --name and --amount (>0) are required%s\n", config.Red, config.Reset)
		return fmt.Errorf("name and amount are required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
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
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sRecurring commitment #%d saved: %s (%.2f / %s)%s\n", config.Green, id, *name, *amount, *freq, config.Reset)
	return nil
}

func handleListCommitments(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("commitments", flag.ContinueOnError)
	fs.SetOutput(stderr)
	status := fs.String("status", "active", "Filter by status")
	cat := fs.String("category", "", "Filter by category")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	comms, err := finance.ListCommitments(database, *status, *cat)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Recurring Commitments (%d active) ===%s\n", config.Bold, len(comms), config.Reset)
	var totalMonthly float64
	for _, c := range comms {
		totalMonthly += c.MonthlyAmount
		fmt.Fprintf(stdout, "#%-3d | %-24s | %-12s | %s %12.2f (%s)\n", c.ID, c.Name, c.Category, c.Currency, c.Amount, c.Frequency)
	}
	fmt.Fprintf(stdout, "\n%sTotal Monthly Recurring Burn:%s IDR %.2f\n", config.Bold, config.Reset, totalMonthly)
	return nil
}

func handleRunway(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	runway, err := finance.ComputeRunway(database)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Sovereign Runway & Treasury Health ===%s\n", config.Bold, config.Reset)
	fmt.Fprintf(stdout, "  Liquid Cash:               IDR %14.2f\n", runway.LiquidCash)
	fmt.Fprintf(stdout, "  Net Worth:                 IDR %14.2f\n", runway.NetWorth)
	fmt.Fprintf(stdout, "  Monthly Fixed Burn:        IDR %14.2f (%d commitments)\n", runway.CommitmentBurn, runway.CommitmentsCount)
	fmt.Fprintf(stdout, "  Discretionary Burn (30d):  IDR %14.2f (%d expenses)\n", runway.DiscretionaryBurn, runway.ExpensesCount30d)
	fmt.Fprintf(stdout, "  Total Monthly Burn:        IDR %14.2f\n", runway.MonthlyBurn)
	fmt.Fprintln(stdout, "  ---------------------------------------------")
	if runway.IsInfinite {
		fmt.Fprintf(stdout, "  Runway:                    %s%s (No active burn)%s\n", config.Green, runway.StatusLabel, config.Reset)
	} else {
		color := config.Green
		if runway.RunwayMonths < 6 {
			color = config.Red
		} else if runway.RunwayMonths < 12 {
			color = config.Yellow
		}
		fmt.Fprintf(stdout, "  Runway:              %s%.1f Months [%s]%s\n", color, runway.RunwayMonths, runway.StatusLabel, config.Reset)
	}

	if len(runway.BurnByCategory) > 0 {
		fmt.Fprintln(stdout, "\n  Burn Breakdown by Category:")
		for cat, amt := range runway.BurnByCategory {
			fmt.Fprintf(stdout, "    %-16s: IDR %12.2f\n", cat, amt)
		}
	}
	return nil
}
