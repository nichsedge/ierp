package commerce

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// PaymentAccount represents a bank account or payment provider destination.
type PaymentAccount struct {
	ID        int    `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Category  string `json:"category,omitempty"`
	Number    string `json:"number,omitempty"`
	Recipient string `json:"recipient,omitempty"`
	Details   string `json:"details,omitempty"`
	DetailsID string `json:"details_id,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Referral represents an affiliate discount code or referral link.
type Referral struct {
	ID        int    `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Category  string `json:"category,omitempty"`
	Code      string `json:"code,omitempty"`
	Link      string `json:"link,omitempty"`
	Benefit   string `json:"benefit,omitempty"`
	Status    string `json:"status"`
	IsPublic  bool   `json:"is_public"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

var nonSlugRegex = regexp.MustCompile(`[^\w\s-]`)
var spaceHyphenRegex = regexp.MustCompile(`[\s_-]+`)

func generateSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonSlugRegex.ReplaceAllString(s, "")
	s = spaceHyphenRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return fmt.Sprintf("slug-%d", time.Now().Unix())
	}
	return s
}

// InsertPaymentAccount inserts a payment account.
func InsertPaymentAccount(database *sql.DB, p PaymentAccount) (int, error) {
	if p.Slug == "" {
		p.Slug = generateSlug(p.Name)
	}

	res, err := database.Exec(`
		INSERT INTO payment_accounts (slug, name, category, number, recipient, details, details_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, p.Slug, strings.TrimSpace(p.Name), p.Category, p.Number, p.Recipient, p.Details, p.DetailsID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert payment account: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListPaymentAccounts retrieves payment accounts.
func ListPaymentAccounts(database *sql.DB, category string) ([]PaymentAccount, error) {
	query := `
		SELECT id, slug, name, COALESCE(category, 'bank'), COALESCE(number, ''),
		       COALESCE(recipient, ''), COALESCE(details, ''), COALESCE(details_id, ''),
		       COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM payment_accounts
		WHERE 1=1
	`
	var params []any
	if category != "" {
		query += " AND category = ?"
		params = append(params, strings.ToLower(category))
	}
	query += " ORDER BY id ASC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PaymentAccount
	for rows.Next() {
		var p PaymentAccount
		if err := rows.Scan(
			&p.ID, &p.Slug, &p.Name, &p.Category, &p.Number,
			&p.Recipient, &p.Details, &p.DetailsID,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

// DeletePaymentAccount deletes a payment account by ID.
func DeletePaymentAccount(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("DELETE FROM payment_accounts WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// InsertReferral records a referral code or link.
func InsertReferral(database *sql.DB, r Referral) (int, error) {
	if r.Slug == "" {
		r.Slug = generateSlug(r.Name)
	}
	if r.Status == "" {
		r.Status = "ACTIVE"
	}
	isPublicInt := 1
	if !r.IsPublic {
		isPublicInt = 0
	}

	res, err := database.Exec(`
		INSERT INTO referrals (slug, name, category, code, link, benefit, status, is_public)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, r.Slug, strings.TrimSpace(r.Name), r.Category, r.Code, r.Link, r.Benefit, r.Status, isPublicInt)
	if err != nil {
		return 0, fmt.Errorf("failed to insert referral: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListReferrals retrieves referrals.
func ListReferrals(database *sql.DB, category string, status string) ([]Referral, error) {
	query := `
		SELECT id, slug, name, COALESCE(category, 'general'), COALESCE(code, ''),
		       COALESCE(link, ''), COALESCE(benefit, ''), COALESCE(status, 'ACTIVE'),
		       COALESCE(is_public, 1), COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM referrals
		WHERE 1=1
	`
	var params []any
	if category != "" {
		query += " AND category = ?"
		params = append(params, strings.ToLower(category))
	}
	if status != "" && strings.ToLower(status) != "all" {
		query += " AND status = ?"
		params = append(params, strings.ToUpper(status))
	}
	query += " ORDER BY id ASC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Referral
	for rows.Next() {
		var r Referral
		var isPublicInt int
		if err := rows.Scan(
			&r.ID, &r.Slug, &r.Name, &r.Category, &r.Code,
			&r.Link, &r.Benefit, &r.Status, &isPublicInt,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		r.IsPublic = (isPublicInt == 1)
		list = append(list, r)
	}
	return list, nil
}

// DeleteReferral removes a referral by ID.
func DeleteReferral(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("DELETE FROM referrals WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}
