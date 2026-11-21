package expenses

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// BackfillStats summarizes the results of a Sans Finance database migration.
type BackfillStats struct {
	AccountsImported    int     `json:"accounts_imported"`
	ExpensesImported    int     `json:"expenses_imported"`
	ExpensesSkipped     int     `json:"expenses_skipped"`
	TotalExpenseAmount  float64 `json:"total_expense_amount"`
	SourceDB            string  `json:"source_db"`
}

// BackfillFromSansFinance imports all accounts, categories, and historical transactions
// from a Sans Finance SQLite database into iERP.
func BackfillFromSansFinance(targetDB *sql.DB, sansDBPath string) (*BackfillStats, error) {
	srcDB, err := sql.Open("sqlite", sansDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open Sans Finance SQLite database at %s: %w", sansDBPath, err)
	}
	defer srcDB.Close()

	stats := &BackfillStats{
		SourceDB: sansDBPath,
	}

	// 1. Backfill Live Accounts into iERP `accounts` table
	accRows, err := srcDB.Query(`
		SELECT name, type, balance, currency
		FROM accounts
		WHERE type IN ('Cash', 'Bank Account', 'P2P Lending', 'Investment')
	`)
	if err == nil {
		defer accRows.Close()

		txAcc, err := targetDB.Begin()
		if err == nil {
			stmtAcc, err := txAcc.Prepare(`
				INSERT INTO accounts (name, type, balance, currency, updated_at)
				VALUES (?, ?, ?, ?, datetime('now', 'localtime'))
				ON CONFLICT(name) DO UPDATE SET
					type = excluded.type,
					balance = excluded.balance,
					currency = excluded.currency,
					updated_at = excluded.updated_at;
			`)
			if err == nil {
				defer stmtAcc.Close()
				for accRows.Next() {
					var name, accType, curr string
					var balanceCents float64
					if err := accRows.Scan(&name, &accType, &balanceCents, &curr); err == nil {
						if curr == "" {
							curr = "IDR"
						}
						balIDR := balanceCents / 100.0
						if _, err := stmtAcc.Exec(name, accType, balIDR, curr); err == nil {
							stats.AccountsImported++
						}
					}
				}
				_ = txAcc.Commit()
			}
		}
	}

	// 2. Fetch tags map for transactions in Sans Finance
	tagMap := make(map[int64][]string)
	tagRows, err := srcDB.Query(`
		SELECT r.expenseId, t.name
		FROM expense_tag_ref r
		JOIN tags t ON r.tagId = t.id
	`)
	if err == nil {
		defer tagRows.Close()
		for tagRows.Next() {
			var expID int64
			var tagName string
			if err := tagRows.Scan(&expID, &tagName); err == nil {
				tagMap[expID] = append(tagMap[expID], tagName)
			}
		}
	}

	// 3. Query all historical expenses from Sans Finance
	expQuery := `
		SELECT 
			e.id,
			e.date,
			e.title,
			COALESCE(e.details, ''),
			e.amount,
			COALESCE(c.name, 'general'),
			COALESCE(a.name, 'Wallet'),
			COALESCE(e.type, 'EXPENSE'),
			COALESCE(e.currency, 'IDR')
		FROM expenses e
		LEFT JOIN categories c ON e.category_id = c.id
		LEFT JOIN accounts a ON e.account_id = a.id
		ORDER BY e.date ASC
	`
	rows, err := srcDB.Query(expQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query expenses from Sans Finance: %w", err)
	}
	defer rows.Close()

	// 4. Batch insert into iERP `expenses`
	tx, err := targetDB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed starting target transaction: %w", err)
	}
	defer tx.Rollback()

	insertStmt, err := tx.Prepare(`
		INSERT INTO expenses (date, title, amount, currency, category, account, notes, tags, type, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'sansfinance_backfill')
	`)
	if err != nil {
		return nil, fmt.Errorf("failed preparing insert statement: %w", err)
	}
	defer insertStmt.Close()

	// Prepare duplicate check query
	checkStmt, err := targetDB.Prepare(`
		SELECT id FROM expenses WHERE date = ? AND title = ? AND amount = ? LIMIT 1
	`)
	if err != nil {
		return nil, fmt.Errorf("failed preparing duplicate check: %w", err)
	}
	defer checkStmt.Close()

	for rows.Next() {
		var (
			rawID       int64
			rawDateMs   int64
			title       string
			details     string
			amountCents int64
			category    string
			account     string
			expType     string
			currency    string
		)

		if err := rows.Scan(
			&rawID, &rawDateMs, &title, &details, &amountCents,
			&category, &account, &expType, &currency,
		); err != nil {
			continue
		}

		title = strings.TrimSpace(title)
		if title == "" {
			continue
		}

		// Convert date
		var dateStr string
		if rawDateMs > 0 {
			dateStr = time.UnixMilli(rawDateMs).Format("2006-01-02")
		} else {
			dateStr = "2026-01-01"
		}

		amount := float64(amountCents) / 100.0
		categoryClean := strings.ToLower(strings.TrimSpace(category))
		expTypeClean := strings.ToUpper(strings.TrimSpace(expType))
		currencyClean := strings.ToUpper(strings.TrimSpace(currency))

		// Check tags
		var tagsStr string
		if tagsList, ok := tagMap[rawID]; ok && len(tagsList) > 0 {
			tagsStr = strings.Join(tagsList, ",")
		}

		// Duplicate check
		var existingID int
		if err := checkStmt.QueryRow(dateStr, title, amount).Scan(&existingID); err == nil && existingID > 0 {
			stats.ExpensesSkipped++
			continue
		}

		if _, err := insertStmt.Exec(
			dateStr, title, amount, currencyClean, categoryClean,
			account, details, tagsStr, expTypeClean,
		); err != nil {
			return nil, fmt.Errorf("failed inserting backfilled expense '%s': %w", title, err)
		}

		stats.ExpensesImported++
		if expTypeClean == "EXPENSE" {
			stats.TotalExpenseAmount += amount
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed committing backfill transaction: %w", err)
	}

	return stats, nil
}
