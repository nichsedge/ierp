package finance

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

// Snapshot represents a net worth balance sheet record.
type Snapshot struct {
	ID           int     `json:"id"`
	SnapshotDate string  `json:"snapshot_date"`
	LiquidCash   float64 `json:"liquid_cash"`
	Investments  float64 `json:"investments"`
	HardAssets   float64 `json:"hard_assets"`
	Liabilities  float64 `json:"liabilities"`
	NetWorth     float64 `json:"net_worth"`
	Currency     string  `json:"currency"`
	Notes        string  `json:"notes"`
	CreatedAt    string  `json:"created_at"`
}

// InsertSnapshot records a balance sheet snapshot.
func InsertSnapshot(database *sql.DB, s Snapshot) (int, error) {
	if s.SnapshotDate == "" {
		s.SnapshotDate = time.Now().Format("2006-01-02")
	}
	if s.Currency == "" {
		s.Currency = "IDR"
	}

	res, err := database.Exec(`
		INSERT INTO networth_snapshots (
			snapshot_date, liquid_cash, investments, hard_assets, liabilities, currency, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, s.SnapshotDate, s.LiquidCash, s.Investments, s.HardAssets, s.Liabilities, s.Currency, s.Notes)
	if err != nil {
		return 0, fmt.Errorf("failed to insert snapshot: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListSnapshots retrieves net worth snapshots in chronological order.
func ListSnapshots(database *sql.DB, limit int) ([]Snapshot, error) {
	if limit <= 0 {
		limit = 30
	}

	rows, err := database.Query(`
		SELECT id, snapshot_date, COALESCE(liquid_cash, 0), COALESCE(investments, 0),
		       COALESCE(hard_assets, 0), COALESCE(liabilities, 0), COALESCE(currency, 'IDR'),
		       COALESCE(notes, ''), COALESCE(created_at, '')
		FROM networth_snapshots
		ORDER BY snapshot_date DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		var s Snapshot
		if err := rows.Scan(
			&s.ID, &s.SnapshotDate, &s.LiquidCash, &s.Investments,
			&s.HardAssets, &s.Liabilities, &s.Currency,
			&s.Notes, &s.CreatedAt,
		); err != nil {
			return nil, err
		}
		s.NetWorth = (s.LiquidCash + s.Investments + s.HardAssets) - s.Liabilities
		snapshots = append(snapshots, s)
	}

	return snapshots, nil
}

// GetLatestSnapshot retrieves the most recent net worth snapshot.
func GetLatestSnapshot(database *sql.DB) (*Snapshot, error) {
	snaps, err := ListSnapshots(database, 1)
	if err != nil {
		return nil, err
	}
	if len(snaps) == 0 {
		return nil, nil
	}
	return &snaps[0], nil
}

// Commitment represents a recurring fixed burn expense.
type Commitment struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	Category         string  `json:"category"`
	Amount           float64 `json:"amount"`
	MonthlyAmount    float64 `json:"monthly_amount"`
	Currency         string  `json:"currency"`
	Frequency        string  `json:"frequency"`
	PaymentAccountID *int    `json:"payment_account_id,omitempty"`
	Status           string  `json:"status"`
	RenewalDate      string  `json:"renewal_date,omitempty"`
	Notes            string  `json:"notes"`
	AccountName      string  `json:"account_name,omitempty"`
}

// InsertCommitment records a recurring expense commitment.
func InsertCommitment(database *sql.DB, c Commitment) (int, error) {
	freq := strings.ToLower(strings.TrimSpace(c.Frequency))
	switch freq {
	case "monthly", "yearly", "quarterly", "weekly":
	default:
		freq = "monthly"
	}
	cat := strings.ToLower(strings.TrimSpace(c.Category))
	if cat == "" {
		cat = "saas"
	}
	status := strings.ToLower(strings.TrimSpace(c.Status))
	if status == "" {
		status = "active"
	}
	cur := strings.ToUpper(strings.TrimSpace(c.Currency))
	if cur == "" {
		cur = "IDR"
	}

	res, err := database.Exec(`
		INSERT INTO recurring_commitments (
			name, category, amount, currency, frequency, payment_account_id, status, renewal_date, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, strings.TrimSpace(c.Name), cat, c.Amount, cur, freq, c.PaymentAccountID, status, c.RenewalDate, c.Notes)
	if err != nil {
		return 0, fmt.Errorf("failed to insert commitment: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListCommitments lists recurring commitments with optional filtering.
func ListCommitments(database *sql.DB, status string, category string) ([]Commitment, error) {
	query := `
		SELECT c.id, c.name, COALESCE(c.category, 'saas'), COALESCE(c.amount, 0),
		       COALESCE(c.currency, 'IDR'), COALESCE(c.frequency, 'monthly'),
		       c.payment_account_id, COALESCE(c.status, 'active'),
		       COALESCE(c.renewal_date, ''), COALESCE(c.notes, ''),
		       COALESCE(p.name, '')
		FROM recurring_commitments c
		LEFT JOIN payment_accounts p ON p.id = c.payment_account_id
		WHERE 1=1
	`
	var params []any
	if status != "" {
		query += " AND c.status = ?"
		params = append(params, status)
	}
	if category != "" {
		query += " AND c.category = ?"
		params = append(params, strings.ToLower(category))
	}

	query += " ORDER BY c.amount DESC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commitments []Commitment
	for rows.Next() {
		var c Commitment
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Category, &c.Amount,
			&c.Currency, &c.Frequency, &c.PaymentAccountID,
			&c.Status, &c.RenewalDate, &c.Notes, &c.AccountName,
		); err != nil {
			return nil, err
		}

		switch c.Frequency {
		case "yearly":
			c.MonthlyAmount = c.Amount / 12.0
		case "quarterly":
			c.MonthlyAmount = c.Amount / 3.0
		case "weekly":
			c.MonthlyAmount = c.Amount * (52.0 / 12.0)
		default:
			c.MonthlyAmount = c.Amount
		}

		commitments = append(commitments, c)
	}

	return commitments, nil
}

// DeleteCommitment removes a commitment by ID.
func DeleteCommitment(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("DELETE FROM recurring_commitments WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// MonthlyBurnResult holds aggregated recurring burn metrics.
type MonthlyBurnResult struct {
	TotalMonthlyBurn float64            `json:"total_monthly_burn"`
	CommitmentsCount int                `json:"commitments_count"`
	ByCategory       map[string]float64 `json:"by_category"`
}

// ComputeMonthlyBurn computes monthly burn by category.
func ComputeMonthlyBurn(database *sql.DB, status string) (MonthlyBurnResult, error) {
	if status == "" {
		status = "active"
	}
	commitments, err := ListCommitments(database, status, "")
	if err != nil {
		return MonthlyBurnResult{}, err
	}

	result := MonthlyBurnResult{
		ByCategory: make(map[string]float64),
	}

	for _, c := range commitments {
		result.TotalMonthlyBurn += c.MonthlyAmount
		result.ByCategory[c.Category] += c.MonthlyAmount
	}
	result.CommitmentsCount = len(commitments)

	return result, nil
}

// RunwayResult holds computed sovereign runway and net worth statistics.
type RunwayResult struct {
	LiquidCash         float64            `json:"liquid_cash"`
	NetWorth           float64            `json:"net_worth"`
	MonthlyBurn        float64            `json:"monthly_burn"`
	RunwayMonths       float64            `json:"runway_months"`
	IsInfinite         bool               `json:"is_infinite"`
	StatusLabel        string             `json:"status_label"`
	BurnByCategory     map[string]float64 `json:"burn_by_category"`
	LatestSnapshotDate string             `json:"latest_snapshot_date,omitempty"`
	CommitmentsCount   int                `json:"commitments_count"`
}

// ComputeRunway calculates sovereignty runway in months.
func ComputeRunway(database *sql.DB) (RunwayResult, error) {
	snapshot, err := GetLatestSnapshot(database)
	if err != nil {
		return RunwayResult{}, err
	}

	burn, err := ComputeMonthlyBurn(database, "active")
	if err != nil {
		return RunwayResult{}, err
	}

	res := RunwayResult{
		MonthlyBurn:      burn.TotalMonthlyBurn,
		BurnByCategory:   burn.ByCategory,
		CommitmentsCount: burn.CommitmentsCount,
	}

	if snapshot != nil {
		res.LiquidCash = snapshot.LiquidCash
		res.NetWorth = snapshot.NetWorth
		res.LatestSnapshotDate = snapshot.SnapshotDate
	}

	if res.MonthlyBurn <= 0 {
		if res.LiquidCash > 0 {
			res.RunwayMonths = 999.0
			res.IsInfinite = true
			res.StatusLabel = "INFINITE"
		} else {
			res.RunwayMonths = 0.0
			res.StatusLabel = "ZERO_RESERVE"
		}
	} else {
		months := res.LiquidCash / res.MonthlyBurn
		res.RunwayMonths = math.Round(months*10) / 10
		if res.RunwayMonths >= 24 {
			res.StatusLabel = "FORTRESS (>24m)"
		} else if res.RunwayMonths >= 12 {
			res.StatusLabel = "SOVEREIGN (>12m)"
		} else if res.RunwayMonths >= 6 {
			res.StatusLabel = "STABLE (6-12m)"
		} else if res.RunwayMonths >= 3 {
			res.StatusLabel = "LEAN (3-6m)"
		} else {
			res.StatusLabel = "CRITICAL (<3m)"
		}
	}

	return res, nil
}
