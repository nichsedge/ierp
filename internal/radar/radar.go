package radar

import (
	"database/sql"
	"sort"
	"time"

	"github.com/nichsedge/ierp/internal/contacts"
)

// RadarItem represents a contact's relationship cadence status.
type RadarItem struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	Org                string `json:"org"`
	Email              string `json:"email"`
	Phone              string `json:"phone"`
	Tier               int    `json:"tier"`
	CadenceDays        int    `json:"cadence_days"`
	LastSeenDate       string `json:"last_seen_date"`
	DaysSinceLastTouch int    `json:"days_since_last_touch"`
	IsOverdue          bool   `json:"is_overdue"`
	DaysOverdue        int    `json:"days_overdue"`
	TotalInteractions  int    `json:"total_interactions"`
	Notes              string `json:"notes"`
}

// ComputeRadar evaluates relationship cadences across contacts.
func ComputeRadar(database *sql.DB, tier *int, overdueOnly bool, limit int, offset int) ([]RadarItem, error) {
	query := `
		SELECT c.id, c.name, COALESCE(c.org, ''), COALESCE(c.email, ''), COALESCE(c.phone, ''),
		       c.tier, c.cadence_days,
		       COALESCE(MAX(e.start_date), ''),
		       COALESCE(c.date, ''),
		       COALESCE(c.notes, ''),
		       COUNT(e.id) as total_interactions
		FROM contacts c
		LEFT JOIN event_contacts ec ON ec.contact_id = c.id
		LEFT JOIN events e ON e.id = ec.event_id
		WHERE 1=1
	`
	var params []any
	if tier != nil {
		query += " AND c.tier = ?"
		params = append(params, *tier)
	} else {
		query += " AND c.tier > 0"
	}
	query += " GROUP BY c.id"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	var results []RadarItem

	for rows.Next() {
		var item RadarItem
		var contactDate, lastSeen string
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Org, &item.Email, &item.Phone,
			&item.Tier, &item.CadenceDays, &lastSeen, &contactDate,
			&item.Notes, &item.TotalInteractions,
		); err != nil {
			return nil, err
		}

		if item.CadenceDays <= 0 {
			item.CadenceDays = contacts.DefaultCadenceByTier[item.Tier]
		}

		effectiveDate := lastSeen
		if effectiveDate == "" {
			effectiveDate = contactDate
		}
		item.LastSeenDate = effectiveDate

		daysSince := 999
		if len(effectiveDate) >= 10 {
			if t, err := time.Parse("2006-01-02", effectiveDate[:10]); err == nil {
				daysSince = int(now.Sub(t).Hours() / 24)
			}
		}
		item.DaysSinceLastTouch = daysSince

		if item.Tier == 0 || item.CadenceDays <= 0 {
			item.IsOverdue = false
			item.DaysOverdue = 0
		} else {
			item.IsOverdue = daysSince > item.CadenceDays
			if item.IsOverdue {
				item.DaysOverdue = daysSince - item.CadenceDays
			} else {
				item.DaysOverdue = 0
			}
		}

		if overdueOnly && !item.IsOverdue {
			continue
		}

		results = append(results, item)
	}

	// Sort: Tier 1 first, then highest days_overdue, then highest days_since
	sort.Slice(results, func(i, j int) bool {
		if results[i].Tier != results[j].Tier {
			return results[i].Tier < results[j].Tier
		}
		if results[i].DaysOverdue != results[j].DaysOverdue {
			return results[i].DaysOverdue > results[j].DaysOverdue
		}
		return results[i].DaysSinceLastTouch > results[j].DaysSinceLastTouch
	})

	if offset > len(results) {
		return []RadarItem{}, nil
	}
	results = results[offset:]

	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	return results, nil
}

// GetDailyReconnection picks the top overdue relationship needing attention today.
func GetDailyReconnection(database *sql.DB) (*RadarItem, error) {
	items, err := ComputeRadar(database, nil, true, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

// RadarSummary contains aggregate counts for radar metrics.
type RadarSummary struct {
	TotalContacts int         `json:"total_contacts"`
	TotalOverdue  int         `json:"total_overdue"`
	TierCounts    map[int]int `json:"tier_counts"`
	TierOverdue   map[int]int `json:"tier_overdue"`
}

// GetRadarSummary returns summary counts across tiers.
func GetRadarSummary(database *sql.DB) (*RadarSummary, error) {
	items, err := ComputeRadar(database, nil, false, 0, 0)
	if err != nil {
		return nil, err
	}

	summary := &RadarSummary{
		TotalContacts: len(items),
		TierCounts:    map[int]int{1: 0, 2: 0, 3: 0},
		TierOverdue:   map[int]int{1: 0, 2: 0, 3: 0},
	}

	for _, it := range items {
		summary.TierCounts[it.Tier]++
		if it.IsOverdue {
			summary.TierOverdue[it.Tier]++
			summary.TotalOverdue++
		}
	}

	return summary, nil
}
