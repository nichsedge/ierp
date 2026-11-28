package commands

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/expenses"
)

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func handleExpenses(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return handleListExpenses(ctx, nil, stdout, stderr)
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "add", "insert":
		return handleInsertExpense(ctx, subArgs, stdout, stderr)
	case "list":
		return handleListExpenses(ctx, subArgs, stdout, stderr)
	case "summary", "stats":
		return handleExpenseSummary(ctx, subArgs, stdout, stderr)
	case "ingest", "import":
		return handleIngestExpenses(ctx, subArgs, stdout, stderr)
	case "backfill":
		return handleBackfillExpenses(ctx, subArgs, stdout, stderr)
	case "delete", "rm":
		return handleDeleteExpense(ctx, subArgs, stdout, stderr)
	default:
		if strings.HasPrefix(sub, "-") {
			return handleListExpenses(ctx, args, stdout, stderr)
		}
		fmt.Fprintf(stderr, "%sUnknown expenses subcommand: %s%s\n", config.Red, sub, config.Reset)
		fmt.Fprintln(stderr, "Usage: ierp expense [add|list|summary|ingest|backfill|delete] [flags]")
		return fmt.Errorf("unknown expenses subcommand: %s", sub)
	}
}

func handleIncome(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return handleListExpenses(ctx, []string{"--type", "INCOME"}, stdout, stderr)
	}
	sub := args[0]
	subArgs := args[1:]
	switch sub {
	case "add", "insert":
		return handleInsertExpense(ctx, append([]string{"--type", "INCOME"}, subArgs...), stdout, stderr)
	case "list":
		return handleListExpenses(ctx, append([]string{"--type", "INCOME"}, subArgs...), stdout, stderr)
	default:
		if strings.HasPrefix(sub, "-") {
			return handleListExpenses(ctx, append([]string{"--type", "INCOME"}, args...), stdout, stderr)
		}
		return handleInsertExpense(ctx, append([]string{"--type", "INCOME"}, args...), stdout, stderr)
	}
}

func handleInsertExpense(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("expense add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "Transaction title / merchant / item (required)")
	amount := fs.Float64("amount", 0, "Transaction amount (required)")
	cat := fs.String("category", "general", "Category")
	date := fs.String("date", "", "Date (YYYY-MM-DD, default today)")
	cur := fs.String("currency", "IDR", "Currency (IDR, SGD, USD)")
	acc := fs.String("account", "", "Payment/deposit account")
	expType := fs.String("type", "", "Transaction type: EXPENSE or INCOME")
	notes := fs.String("notes", "", "Notes / description")
	tags := fs.String("tags", "", "Comma-separated tags")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *title == "" || *amount <= 0 {
		fmt.Fprintf(stderr, "%sError: --title and --amount (>0) are required%s\n", config.Red, config.Reset)
		return fmt.Errorf("title and amount are required")
	}

	typeClean := strings.ToUpper(strings.TrimSpace(*expType))
	if typeClean != "" && typeClean != "EXPENSE" && typeClean != "INCOME" && typeClean != "TRANSFER" {
		fmt.Fprintf(stderr, "%sError: --type must be EXPENSE, INCOME, or TRANSFER%s\n", config.Red, config.Reset)
		return fmt.Errorf("invalid type: %s", typeClean)
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	id, err := expenses.InsertExpense(database, expenses.Expense{
		Title:    *title,
		Amount:   *amount,
		Category: *cat,
		Date:     *date,
		Currency: *cur,
		Account:  *acc,
		Notes:    *notes,
		Tags:     *tags,
		Type:     typeClean,
		Source:   "cli",
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	saved, _ := expenses.GetExpenseByID(database, id)
	resolvedType := "EXPENSE"
	if saved != nil && saved.Type != "" {
		resolvedType = saved.Type
	}

	fmt.Fprintf(stdout, "%sRecorded %s #%d: %s | %s %.2f (%s)%s\n",
		config.Green, resolvedType, id, *title, strings.ToUpper(*cur), *amount, strings.ToLower(*cat), config.Reset)
	return nil
}

func handleListExpenses(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("expense list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 30, "Max items to display")
	cat := fs.String("category", "", "Filter by category")
	tag := fs.String("tag", "", "Filter by tag")
	expType := fs.String("type", "", "Filter by type (EXPENSE, INCOME, TRANSFER, all)")
	month := fs.String("month", "", "Filter by month (YYYY-MM)")
	start := fs.String("start", "", "Filter by start date (YYYY-MM-DD)")
	end := fs.String("end", "", "Filter by end date (YYYY-MM-DD)")
	jsonOut := fs.Bool("json", false, "Output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	items, err := expenses.ListExpenses(database, expenses.ListOptions{
		Limit:     *limit,
		Category:  *cat,
		Tag:       *tag,
		Type:      *expType,
		Month:     *month,
		StartDate: *start,
		EndDate:   *end,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(items)
		return nil
	}

	fmt.Fprintf(stdout, "%s=== Cash Flow Ledger (%d records) ===%s\n", config.Bold, len(items), config.Reset)
	if len(items) == 0 {
		fmt.Fprintln(stdout, "  (No records found matching criteria)")
		return nil
	}

	var totalExpenseIDR, totalIncomeIDR float64
	for _, e := range items {
		if e.Currency == "IDR" {
			if e.Type == "INCOME" {
				totalIncomeIDR += e.Amount
			} else {
				totalExpenseIDR += e.Amount
			}
		}

		typeColor := config.Red
		typeBadge := "[EXP]"
		if e.Type == "INCOME" {
			typeColor = config.Green
			typeBadge = "[INC]"
		} else if e.Type == "TRANSFER" {
			typeColor = config.Cyan
			typeBadge = "[TRF]"
		}

		tagStr := ""
		if e.Tags != "" {
			tagStr = fmt.Sprintf(" [%s]", e.Tags)
		}
		fmt.Fprintf(stdout, "#%-3d | %s | %s%-5s%s | %-24s | %-10s | %s %12.2f%s\n",
			e.ID, e.Date, typeColor, typeBadge, config.Reset, truncateStr(e.Title, 24), e.Category, e.Currency, e.Amount, tagStr)
	}

	fmt.Fprintf(stdout, "\n%sFiltered Summary:%s\n", config.Bold, config.Reset)
	fmt.Fprintf(stdout, "  • Total Expenses: IDR %14.2f\n", totalExpenseIDR)
	fmt.Fprintf(stdout, "  • Total Income:   IDR %14.2f\n", totalIncomeIDR)
	netFlow := totalIncomeIDR - totalExpenseIDR
	netColor := config.Green
	if netFlow < 0 {
		netColor = config.Red
	}
	fmt.Fprintf(stdout, "  • Net Flow:       %sIDR %14.2f%s\n", netColor, netFlow, config.Reset)
	return nil
}

func handleExpenseSummary(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("expense summary", flag.ContinueOnError)
	fs.SetOutput(stderr)
	month := fs.String("month", "", "Month to summarize (YYYY-MM)")
	start := fs.String("start", "", "Start date (YYYY-MM-DD)")
	end := fs.String("end", "", "End date (YYYY-MM-DD)")
	jsonOut := fs.Bool("json", false, "Output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *month == "" && *start == "" && *end == "" {
		*month = time.Now().Format("2006-01")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	summary, err := expenses.GetSummary(database, *month, *start, *end)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(summary)
		return nil
	}

	fmt.Fprintf(stdout, "%s=== Cash Flow Summary: %s ===%s\n", config.Bold, summary.Period, config.Reset)
	fmt.Fprintf(stdout, "  • Total Expenses: IDR %14.2f (%d txs)\n", summary.TotalExpense, summary.ExpenseCount)
	fmt.Fprintf(stdout, "  • Total Income:   IDR %14.2f (%d txs)\n", summary.TotalIncome, summary.IncomeCount)
	netColor := config.Green
	if summary.NetCashFlow < 0 {
		netColor = config.Red
	}
	fmt.Fprintf(stdout, "  • Net Cash Flow:  %sIDR %14.2f%s\n\n", netColor, summary.NetCashFlow, config.Reset)

	if len(summary.ByCategory) > 0 {
		fmt.Fprintf(stdout, "%sExpenses by Category:%s\n", config.Bold, config.Reset)
		for _, b := range summary.ByCategory {
			pct := 0.0
			if summary.TotalExpense > 0 {
				pct = (b.Total / summary.TotalExpense) * 100.0
			}
			fmt.Fprintf(stdout, "  • %-16s: IDR %12.2f (%2d txs, %5.1f%%)\n", b.Category, b.Total, b.Count, pct)
		}
	}

	if len(summary.IncomeByCategory) > 0 {
		fmt.Fprintf(stdout, "\n%sIncome by Category:%s\n", config.Bold, config.Reset)
		for _, b := range summary.IncomeByCategory {
			pct := 0.0
			if summary.TotalIncome > 0 {
				pct = (b.Total / summary.TotalIncome) * 100.0
			}
			fmt.Fprintf(stdout, "  • %-16s: IDR %12.2f (%2d txs, %5.1f%%)\n", b.Category, b.Total, b.Count, pct)
		}
	}
	return nil
}

func handleIngestExpenses(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("expense ingest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tag := fs.String("tag", "", "Default tag to attach to ingested items")
	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintf(stderr, "%sUsage: ierp expense ingest <file.md|file.json> [--tag <tag>]%s\n", config.Red, config.Reset)
		return fmt.Errorf("missing input file")
	}

	targetPath := remaining[0]
	if strings.HasPrefix(targetPath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			targetPath = filepath.Join(home, targetPath[2:])
		}
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	res, err := expenses.IngestFile(database, targetPath, *tag)
	if err != nil {
		fmt.Fprintf(stderr, "%sError ingesting expenses: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s✅ Successfully ingested %d expenses from %s!%s\n", config.Green, res.Count, res.SourceDoc, config.Reset)
	fmt.Fprintf(stdout, "  Total Amount: IDR %.2f\n\n", res.Total)
	for _, e := range res.Expenses {
		fmt.Fprintf(stdout, "  • [%s] %-30s: %s %.2f\n", e.Category, truncateStr(e.Title, 30), e.Currency, e.Amount)
	}
	return nil
}

func handleDeleteExpense(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("expense delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.Int("id", 0, "Expense ID to delete (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *id <= 0 && len(fs.Args()) > 0 {
		if parsed, err := strconv.Atoi(fs.Args()[0]); err == nil {
			*id = parsed
		}
	}

	if *id <= 0 {
		fmt.Fprintf(stderr, "%sError: valid --id is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("valid --id is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	deleted, err := expenses.DeleteExpense(database, *id)
	if err != nil {
		fmt.Fprintf(stderr, "%sError deleting expense: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	if !deleted {
		fmt.Fprintf(stderr, "%sNo expense found with ID %d%s\n", config.Yellow, *id, config.Reset)
		return nil
	}

	fmt.Fprintf(stdout, "%sDeleted expense #%d%s\n", config.Green, *id, config.Reset)
	return nil
}

func handleBackfillExpenses(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	defaultDB := filepath.Join("data", "sans_finance_latest.sqlite")
	if home, err := os.UserHomeDir(); err == nil {
		defaultDB = filepath.Join(home, "Projects", "ierp", "data", "sans_finance_latest.sqlite")
	}

	targetPath := defaultDB
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		targetPath = args[0]
	}

	if strings.HasPrefix(targetPath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			targetPath = filepath.Join(home, targetPath[2:])
		}
	}

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		fmt.Fprintf(stderr, "%sError: source Sans Finance SQLite DB not found at %s%s\n", config.Red, targetPath, config.Reset)
		return fmt.Errorf("sans finance db not found at %s", targetPath)
	}

	fmt.Fprintf(stdout, "⏳ Starting Sans Finance backfill from: %s\n", targetPath)

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	stats, err := expenses.BackfillFromSansFinance(database, targetPath)
	if err != nil {
		fmt.Fprintf(stderr, "%sError during backfill: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s✅ Sans Finance backfill completed successfully!%s\n", config.Green, config.Reset)
	fmt.Fprintf(stdout, "  • Live Accounts Imported:  %d\n", stats.AccountsImported)
	fmt.Fprintf(stdout, "  • Expenses Imported:       %d\n", stats.ExpensesImported)
	fmt.Fprintf(stdout, "  • Expenses Skipped (dup):  %d\n", stats.ExpensesSkipped)
	fmt.Fprintf(stdout, "  • Total Expense Amount:    IDR %.2f\n", stats.TotalExpenseAmount)
	return nil
}
