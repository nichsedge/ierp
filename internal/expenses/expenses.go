package expenses

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Expense represents an expense record in iERP.
type Expense struct {
	ID        int     `json:"id"`
	Date      string  `json:"date"`
	Title     string  `json:"title"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	Category  string  `json:"category"`
	Account   string  `json:"account,omitempty"`
	Notes     string  `json:"notes,omitempty"`
	Tags      string  `json:"tags,omitempty"`
	Type      string  `json:"type,omitempty"`
	Source    string  `json:"source,omitempty"`
	CreatedAt string  `json:"created_at,omitempty"`
	UpdatedAt string  `json:"updated_at,omitempty"`
}

// ListOptions controls filtering and pagination for expenses.
type ListOptions struct {
	Limit     int
	Offset    int
	Category  string
	Tag       string
	Type      string // 'EXPENSE' or 'INCOME'
	Month     string // 'YYYY-MM'
	StartDate string // 'YYYY-MM-DD'
	EndDate   string // 'YYYY-MM-DD'
}

// CategoryBreakdown represents total spending grouped by category.
type CategoryBreakdown struct {
	Category string  `json:"category"`
	Total    float64 `json:"total"`
	Count    int     `json:"count"`
}

// ExpenseSummary represents an aggregated spending summary.
type ExpenseSummary struct {
	Period     string              `json:"period"`
	Total      float64             `json:"total"`
	Count      int                 `json:"count"`
	Currency   string              `json:"currency"`
	ByCategory []CategoryBreakdown `json:"by_category"`
}

// InsertExpense records a single expense transaction.
func InsertExpense(database *sql.DB, e Expense) (int, error) {
	if e.Date == "" {
		e.Date = time.Now().Format("2006-01-02")
	}
	if e.Currency == "" {
		e.Currency = "IDR"
	}
	e.Currency = strings.ToUpper(strings.TrimSpace(e.Currency))

	if e.Category == "" {
		e.Category = "general"
	}
	e.Category = strings.ToLower(strings.TrimSpace(e.Category))

	if e.Source == "" {
		e.Source = "manual"
	}
	if e.Type == "" {
		e.Type = "EXPENSE"
	}
	e.Type = strings.ToUpper(strings.TrimSpace(e.Type))

	res, err := database.Exec(`
		INSERT INTO expenses (
			date, title, amount, currency, category, account, notes, tags, type, source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.Date, strings.TrimSpace(e.Title), e.Amount, e.Currency,
		e.Category, strings.TrimSpace(e.Account), e.Notes, strings.TrimSpace(e.Tags), e.Type, e.Source)
	if err != nil {
		return 0, fmt.Errorf("failed to insert expense: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// UpdateExpense updates an existing expense record.
func UpdateExpense(database *sql.DB, id int, e Expense) (bool, error) {
	if e.Currency == "" {
		e.Currency = "IDR"
	}
	e.Currency = strings.ToUpper(strings.TrimSpace(e.Currency))

	if e.Category == "" {
		e.Category = "general"
	}
	e.Category = strings.ToLower(strings.TrimSpace(e.Category))

	if e.Type == "" {
		e.Type = "EXPENSE"
	}
	e.Type = strings.ToUpper(strings.TrimSpace(e.Type))

	res, err := database.Exec(`
		UPDATE expenses
		SET date = ?, title = ?, amount = ?, currency = ?, category = ?,
		    account = ?, notes = ?, tags = ?, type = ?, source = ?, updated_at = datetime('now', 'localtime')
		WHERE id = ?
	`, e.Date, strings.TrimSpace(e.Title), e.Amount, e.Currency,
		e.Category, strings.TrimSpace(e.Account), e.Notes, strings.TrimSpace(e.Tags), e.Type, e.Source, id)
	if err != nil {
		return false, fmt.Errorf("failed to update expense: %w", err)
	}

	ra, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return ra > 0, nil
}

// DeleteExpense removes an expense record by ID.
func DeleteExpense(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("DELETE FROM expenses WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("failed to delete expense: %w", err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return ra > 0, nil
}

// GetExpenseByID fetches a single expense by ID.
func GetExpenseByID(database *sql.DB, id int) (*Expense, error) {
	var e Expense
	err := database.QueryRow(`
		SELECT id, date, title, amount, currency, category,
		       COALESCE(account, ''), COALESCE(notes, ''), COALESCE(tags, ''),
		       COALESCE(type, 'EXPENSE'), COALESCE(source, ''), COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM expenses
		WHERE id = ?
	`, id).Scan(
		&e.ID, &e.Date, &e.Title, &e.Amount, &e.Currency, &e.Category,
		&e.Account, &e.Notes, &e.Tags, &e.Type, &e.Source, &e.CreatedAt, &e.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ListExpenses retrieves expenses with filtering and ordering.
func ListExpenses(database *sql.DB, opts ListOptions) ([]Expense, error) {
	var conditions []string
	var args []interface{}

	if opts.Category != "" {
		conditions = append(conditions, "category = ?")
		args = append(args, strings.ToLower(strings.TrimSpace(opts.Category)))
	}

	if opts.Type != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, strings.ToUpper(strings.TrimSpace(opts.Type)))
	}

	if opts.Tag != "" {
		conditions = append(conditions, "tags LIKE ?")
		args = append(args, "%"+strings.TrimSpace(opts.Tag)+"%")
	}

	if opts.Month != "" {
		conditions = append(conditions, "date LIKE ?")
		args = append(args, strings.TrimSpace(opts.Month)+"%")
	}

	if opts.StartDate != "" {
		conditions = append(conditions, "date >= ?")
		args = append(args, opts.StartDate)
	}

	if opts.EndDate != "" {
		conditions = append(conditions, "date <= ?")
		args = append(args, opts.EndDate)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT id, date, title, amount, currency, category,
		       COALESCE(account, ''), COALESCE(notes, ''), COALESCE(tags, ''),
		       COALESCE(type, 'EXPENSE'), COALESCE(source, ''), COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM expenses
		%s
		ORDER BY date DESC, id DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, limit, opts.Offset)

	rows, err := database.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed querying expenses: %w", err)
	}
	defer rows.Close()

	var results []Expense
	for rows.Next() {
		var e Expense
		if err := rows.Scan(
			&e.ID, &e.Date, &e.Title, &e.Amount, &e.Currency, &e.Category,
			&e.Account, &e.Notes, &e.Tags, &e.Type, &e.Source, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, nil
}

// GetSummary calculates total spending and category breakdown for a given period.
func GetSummary(database *sql.DB, month, startDate, endDate string) (*ExpenseSummary, error) {
	var conditions []string
	var args []interface{}
	periodLabel := "All Time"

	if month != "" {
		conditions = append(conditions, "date LIKE ?")
		args = append(args, strings.TrimSpace(month)+"%")
		periodLabel = month
	} else if startDate != "" || endDate != "" {
		if startDate != "" {
			conditions = append(conditions, "date >= ?")
			args = append(args, startDate)
		}
		if endDate != "" {
			conditions = append(conditions, "date <= ?")
			args = append(args, endDate)
		}
		periodLabel = fmt.Sprintf("%s to %s", startDate, endDate)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// 1. Total and count
	totQuery := fmt.Sprintf(`
		SELECT COALESCE(SUM(amount), 0), COUNT(*)
		FROM expenses
		%s
	`, whereClause)

	summary := &ExpenseSummary{
		Period:   periodLabel,
		Currency: "IDR",
	}

	if err := database.QueryRow(totQuery, args...).Scan(&summary.Total, &summary.Count); err != nil {
		return nil, fmt.Errorf("failed calculating total expense: %w", err)
	}

	// 2. Breakdown by category
	catQuery := fmt.Sprintf(`
		SELECT category, COALESCE(SUM(amount), 0) AS cat_total, COUNT(*) AS cat_count
		FROM expenses
		%s
		GROUP BY category
		ORDER BY cat_total DESC
	`, whereClause)

	rows, err := database.Query(catQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed calculating category breakdown: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var b CategoryBreakdown
		if err := rows.Scan(&b.Category, &b.Total, &b.Count); err != nil {
			return nil, err
		}
		summary.ByCategory = append(summary.ByCategory, b)
	}

	return summary, nil
}

// IngestResult records the outcome of a batch ingestion.
type IngestResult struct {
	Count     int       `json:"count"`
	Total     float64   `json:"total"`
	Expenses  []Expense `json:"expenses"`
	SourceDoc string    `json:"source_doc"`
}

// IngestFile ingests expenses from either a Markdown file or a JSON file into iERP.
func IngestFile(database *sql.DB, filePath, defaultTag string) (*IngestResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read file %s: %w", filePath, err)
	}

	content := strings.TrimSpace(string(data))
	fileName := filepath.Base(filePath)

	// Check if JSON
	if strings.HasPrefix(content, "[") || strings.HasPrefix(content, "{") {
		return ingestJSON(database, content, fileName, defaultTag)
	}

	// Otherwise parse as Markdown
	return ingestMarkdown(database, content, fileName, defaultTag)
}

func ingestJSON(database *sql.DB, content, fileName, defaultTag string) (*IngestResult, error) {
	// Attempt parsing as []Expense or []map[string]interface{}
	var items []map[string]interface{}

	if strings.HasPrefix(content, "{") {
		var single map[string]interface{}
		if err := json.Unmarshal([]byte(content), &single); err != nil {
			return nil, fmt.Errorf("invalid JSON object: %w", err)
		}
		items = append(items, single)
	} else {
		if err := json.Unmarshal([]byte(content), &items); err != nil {
			return nil, fmt.Errorf("invalid JSON array: %w", err)
		}
	}

	tx, err := database.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO expenses (date, title, amount, currency, category, account, notes, tags, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var inserted []Expense
	var total float64

	nowDate := time.Now().Format("2006-01-02")

	for _, item := range items {
		title := fmt.Sprintf("%v", item["title"])
		if title == "" || title == "<nil>" {
			continue
		}

		// Amount extraction
		var amt float64
		if rawAmt, ok := item["amount"]; ok {
			switch v := rawAmt.(type) {
			case float64:
				amt = v
			case int:
				amt = float64(v)
			case string:
				cleaned := reDigitsOnly.ReplaceAllString(v, "")
				amt, _ = strconv.ParseFloat(cleaned, 64)
			}
		}

		// If amount was stored in cents (e.g. from Sans Finance > 1,000,000 with no decimal part)
		// Check if currency is IDR and raw was integer > 10000000 (100k IDR in cents is 10,000,000)
		// Sans Finance amount in cents: if user exported from Sans Finance
		if rawAmtCents, ok := item["amount_cents"]; ok {
			if c, ok := rawAmtCents.(float64); ok && c > 0 {
				amt = c / 100.0
			}
		}

		dateStr := nowDate
		if d, ok := item["date"]; ok {
			switch v := d.(type) {
			case string:
				dateStr = v
			case float64: // epoch ms
				t := time.UnixMilli(int64(v))
				dateStr = t.Format("2006-01-02")
			}
		}

		currency := "IDR"
		if c, ok := item["currency"]; ok && fmt.Sprintf("%v", c) != "" {
			currency = strings.ToUpper(fmt.Sprintf("%v", c))
		}

		category := "general"
		if c, ok := item["category"]; ok && fmt.Sprintf("%v", c) != "" {
			category = strings.ToLower(fmt.Sprintf("%v", c))
		} else if catName, ok := item["categoryName"]; ok && fmt.Sprintf("%v", catName) != "" {
			category = strings.ToLower(fmt.Sprintf("%v", catName))
		}

		account := ""
		if a, ok := item["account"]; ok {
			account = fmt.Sprintf("%v", a)
		}

		notes := ""
		if n, ok := item["details"]; ok {
			notes = fmt.Sprintf("%v", n)
		} else if n, ok := item["notes"]; ok {
			notes = fmt.Sprintf("%v", n)
		}

		var tagsList []string
		if defaultTag != "" {
			tagsList = append(tagsList, defaultTag)
		}
		if rawTags, ok := item["tags"]; ok {
			switch vt := rawTags.(type) {
			case []interface{}:
				for _, t := range vt {
					tagsList = append(tagsList, fmt.Sprintf("%v", t))
				}
			case string:
				tagsList = append(tagsList, vt)
			}
		}
		tagsStr := strings.Join(tagsList, ",")

		if _, err := stmt.Exec(dateStr, title, amt, currency, category, account, notes, tagsStr, "json:"+fileName); err != nil {
			return nil, fmt.Errorf("failed executing insert for %s: %w", title, err)
		}

		inserted = append(inserted, Expense{
			Date:     dateStr,
			Title:    title,
			Amount:   amt,
			Currency: currency,
			Category: category,
			Account:  account,
			Notes:    notes,
			Tags:     tagsStr,
			Source:   "json:" + fileName,
		})
		total += amt
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &IngestResult{
		Count:     len(inserted),
		Total:     total,
		Expenses:  inserted,
		SourceDoc: fileName,
	}, nil
}

var (
	reFrontmatterDate = regexp.MustCompile(`(?m)^date:\s*([0-9]{4}-[0-9]{2}-[0-9]{2})`)
	reDigitsOnly      = regexp.MustCompile(`[^\d.]`)
)

func ingestMarkdown(database *sql.DB, content, fileName, defaultTag string) (*IngestResult, error) {
	// 1. Extract date from frontmatter
	entryDate := time.Now().Format("2006-01-02")
	if m := reFrontmatterDate.FindStringSubmatch(content); len(m) > 1 {
		entryDate = m[1]
	}

	// 2. Identify and parse markdown table
	lines := strings.Split(content, "\n")
	var tableLines []string
	inTable := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			lower := strings.ToLower(trimmed)
			if !inTable && (strings.Contains(lower, "category") || strings.Contains(lower, "cost") || strings.Contains(lower, "amount") || strings.Contains(lower, "idr")) {
				inTable = true
				tableLines = append(tableLines, trimmed)
				continue
			}
			if inTable {
				tableLines = append(tableLines, trimmed)
			}
		} else if inTable {
			// End of table
			if len(tableLines) > 2 {
				break
			}
			inTable = false
			tableLines = nil
		}
	}

	if len(tableLines) < 3 {
		return nil, fmt.Errorf("no expense table found in markdown document")
	}

	// Parse header to find column indexes
	headerCols := splitTableRow(tableLines[0])
	catIdx, detailsIdx, idrIdx, sgdIdx := -1, -1, -1, -1

	for idx, col := range headerCols {
		cl := strings.ToLower(col)
		if strings.Contains(cl, "category") {
			catIdx = idx
		} else if strings.Contains(cl, "cost") || strings.Contains(cl, "details") || strings.Contains(cl, "item") || strings.Contains(cl, "description") {
			detailsIdx = idx
		} else if strings.Contains(cl, "idr") {
			idrIdx = idx
		} else if strings.Contains(cl, "sgd") {
			sgdIdx = idx
		}
	}

	if catIdx == -1 && detailsIdx == -1 {
		return nil, fmt.Errorf("could not identify required columns in markdown table")
	}

	tx, err := database.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO expenses (date, title, amount, currency, category, account, notes, tags, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var inserted []Expense
	var total float64

	tag := "travel"
	if defaultTag != "" {
		tag = defaultTag
	}

	// Parse body rows (skipping header and divider)
	for _, rowStr := range tableLines[2:] {
		cols := splitTableRow(rowStr)
		if len(cols) == 0 {
			continue
		}

		catRaw := ""
		if catIdx >= 0 && catIdx < len(cols) {
			catRaw = cleanMarkdownCell(cols[catIdx])
		}

		// Skip total / summary row
		if strings.Contains(strings.ToLower(catRaw), "total") {
			continue
		}

		detailsRaw := ""
		if detailsIdx >= 0 && detailsIdx < len(cols) {
			detailsRaw = cleanMarkdownCell(cols[detailsIdx])
		}

		title := detailsRaw
		if title == "" {
			title = catRaw
		} else if catRaw != "" && !strings.Contains(strings.ToLower(detailsRaw), strings.ToLower(catRaw)) {
			title = fmt.Sprintf("%s: %s", catRaw, detailsRaw)
		}

		var amount float64
		currency := "IDR"
		var notesParts []string

		// Check IDR column first
		if idrIdx >= 0 && idrIdx < len(cols) {
			valStr := cleanMarkdownCell(cols[idrIdx])
			num := extractNumber(valStr)
			if num > 0 {
				amount = num
			}
		}

		// Check SGD column
		if sgdIdx >= 0 && sgdIdx < len(cols) {
			valStr := cleanMarkdownCell(cols[sgdIdx])
			if valStr != "" && valStr != "—" && valStr != "-" {
				notesParts = append(notesParts, fmt.Sprintf("SGD: %s", valStr))
				if amount == 0 {
					num := extractNumber(valStr)
					if num > 0 {
						amount = num
						currency = "SGD"
					}
				}
			}
		}

		if amount == 0 {
			continue
		}

		category := normalizeCategory(catRaw)
		notes := strings.Join(notesParts, " | ")
		tags := fmt.Sprintf("%s,%s", tag, category)

		if _, err := stmt.Exec(entryDate, title, amount, currency, category, "", notes, tags, "markdown:"+fileName); err != nil {
			return nil, fmt.Errorf("failed inserting table row '%s': %w", title, err)
		}

		inserted = append(inserted, Expense{
			Date:     entryDate,
			Title:    title,
			Amount:   amount,
			Currency: currency,
			Category: category,
			Notes:    notes,
			Tags:     tags,
			Source:   "markdown:" + fileName,
		})
		total += amount
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &IngestResult{
		Count:     len(inserted),
		Total:     total,
		Expenses:  inserted,
		SourceDoc: fileName,
	}, nil
}

func splitTableRow(row string) []string {
	trimmed := strings.Trim(strings.TrimSpace(row), "|")
	parts := strings.Split(trimmed, "|")
	var res []string
	for _, p := range parts {
		res = append(res, strings.TrimSpace(p))
	}
	return res
}

func cleanMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "[[", "")
	s = strings.ReplaceAll(s, "]]", "")
	return strings.TrimSpace(s)
}

func extractNumber(s string) float64 {
	// Strip "Rp", "~", commas, spaces, dots if thousands separator
	clean := strings.ReplaceAll(s, "Rp", "")
	clean = strings.ReplaceAll(clean, "~", "")
	clean = strings.ReplaceAll(clean, "S$", "")
	clean = strings.ReplaceAll(clean, "$", "")
	clean = strings.TrimSpace(clean)

	// In Indonesian notation: 2.800.000 or 14,177
	if strings.Contains(clean, ".") && !strings.Contains(clean, ",") {
		// Could be 2.800.000 (thousands separators) or 12.00 (decimal)
		parts := strings.Split(clean, ".")
		if len(parts) > 2 || (len(parts) == 2 && len(parts[1]) == 3) {
			clean = strings.ReplaceAll(clean, ".", "")
		}
	} else if strings.Contains(clean, ",") && !strings.Contains(clean, ".") {
		// Could be 2,800,000 (thousands separators)
		parts := strings.Split(clean, ",")
		if len(parts) > 2 || (len(parts) == 2 && len(parts[1]) == 3) {
			clean = strings.ReplaceAll(clean, ",", "")
		} else {
			clean = strings.ReplaceAll(clean, ",", ".")
		}
	} else if strings.Contains(clean, ".") && strings.Contains(clean, ",") {
		// Standard IDR 2.800.000,00 or US 2,800,000.00
		lastDot := strings.LastIndex(clean, ".")
		lastComma := strings.LastIndex(clean, ",")
		if lastDot > lastComma {
			// US: 2,800,000.00
			clean = strings.ReplaceAll(clean, ",", "")
		} else {
			// EU/ID: 2.800.000,00
			clean = strings.ReplaceAll(clean, ".", "")
			clean = strings.ReplaceAll(clean, ",", ".")
		}
	}

	val, _ := strconv.ParseFloat(clean, 64)
	return val
}

func normalizeCategory(raw string) string {
	r := strings.ToLower(raw)
	switch {
	case strings.Contains(r, "flight"):
		return "flights"
	case strings.Contains(r, "hotel") || strings.Contains(r, "accommodat") || strings.Contains(r, "lodging"):
		return "lodging"
	case strings.Contains(r, "food") || strings.Contains(r, "drink") || strings.Contains(r, "makan"):
		return "food"
	case strings.Contains(r, "transit") || strings.Contains(r, "transport"):
		return "transport"
	case strings.Contains(r, "connect") || strings.Contains(r, "roaming") || strings.Contains(r, "utility") || strings.Contains(r, "bill"):
		return "utilities"
	default:
		clean := strings.ToLower(strings.TrimSpace(raw))
		if clean == "" {
			return "general"
		}
		return clean
	}
}
