package audit

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/nichsedge/ierp/internal/decisions"
	"github.com/nichsedge/ierp/internal/finance"
	"github.com/nichsedge/ierp/internal/lifeops"
	"github.com/nichsedge/ierp/internal/projects"
	"github.com/nichsedge/ierp/internal/radar"
	"github.com/nichsedge/ierp/internal/reviews"
)

// AuditFinding represents an identified data gap or operational recommendation.
type AuditFinding struct {
	Domain      string `json:"domain"`
	Severity    string `json:"severity"` // "action_needed", "warning", "tip"
	Title       string `json:"title"`
	Description string `json:"description"`
	Command     string `json:"command"`
}

// AuditResult aggregates all findings across domains.
type AuditResult struct {
	GeneratedAt   string         `json:"generated_at"`
	TotalFindings int            `json:"total_findings"`
	ActionsNeeded int            `json:"actions_needed"`
	Warnings      int            `json:"warnings"`
	Tips          int            `json:"tips"`
	Findings      []AuditFinding `json:"findings"`
}

// RunAudit scans database state across life domains and reports recommendations.
func RunAudit(database *sql.DB) (*AuditResult, error) {
	now := time.Now()
	todayStr := now.Format("2006-01-02")
	var findings []AuditFinding

	// 1. Sovereign Treasury & Runway Audit
	snapshots, err := finance.ListSnapshots(database, 1)
	if err == nil {
		if len(snapshots) == 0 {
			findings = append(findings, AuditFinding{
				Domain:      "Treasury",
				Severity:    "action_needed",
				Title:       "No Net Worth Snapshot",
				Description: "You haven't recorded a balance sheet snapshot. iERP cannot calculate your liquid reserves or wealth trajectory.",
				Command:     "ierp insert-snapshot --liquid <amount> --investments <amount>",
			})
		} else {
			latestSnap := snapshots[0]
			daysSinceSnap := 999
			if len(latestSnap.SnapshotDate) >= 10 {
				if t, err := time.Parse("2006-01-02", latestSnap.SnapshotDate[:10]); err == nil {
					daysSinceSnap = int(now.Sub(t).Hours() / 24)
				}
			}
			if daysSinceSnap > 30 {
				findings = append(findings, AuditFinding{
					Domain:      "Treasury",
					Severity:    "warning",
					Title:       "Outdated Net Worth Snapshot",
					Description: fmt.Sprintf("Latest snapshot is %d days old (%s). Monthly balance checkup recommended.", daysSinceSnap, latestSnap.SnapshotDate),
					Command:     "ierp insert-snapshot --liquid <amount>",
				})
			}
		}
	}

	commitments, err := finance.ListCommitments(database, "active", "")
	if err == nil {
		if len(commitments) == 0 {
			findings = append(findings, AuditFinding{
				Domain:      "Treasury",
				Severity:    "action_needed",
				Title:       "No Recurring Commitments Logged",
				Description: "Zero recurring expenses logged. Fixed burn rate is Rp0, so runway cannot be accurately calculated.",
				Command:     "ierp insert-commitment --name <Rent/SaaS> --amount <amount> --frequency monthly",
			})
		} else {
			runway, err := finance.ComputeRunway(database)
			if err == nil && runway.RunwayMonths < 6 && !runway.IsInfinite {
				findings = append(findings, AuditFinding{
					Domain:      "Treasury",
					Severity:    "warning",
					Title:       "Lean Freedom Runway",
					Description: fmt.Sprintf("Current runway is %.1f months (%s). Consider cutting recurring burn.", runway.RunwayMonths, runway.StatusLabel),
					Command:     "ierp runway",
				})
			}
		}
	}

	// 2. Decision Journal Audit
	overdueDecisions, _, err := decisions.GetDecisionAlerts(database, 14)
	if err == nil && len(overdueDecisions) > 0 {
		var titles []string
		for i, d := range overdueDecisions {
			if i >= 3 {
				break
			}
			titles = append(titles, fmt.Sprintf("#%d %s", d.ID, d.Title))
		}
		findings = append(findings, AuditFinding{
			Domain:      "Decisions",
			Severity:    "action_needed",
			Title:       fmt.Sprintf("%d Decision(s) Due for Review", len(overdueDecisions)),
			Description: fmt.Sprintf("Decisions reached scheduled review date: %s. Calibrate your hypothesis with actual outcomes.", strings.Join(titles, ", ")),
			Command:     fmt.Sprintf("ierp review-decision %d --outcome <results>", overdueDecisions[0].ID),
		})
	}

	allDecisions, err := decisions.ListDecisions(database, "", nil)
	if err == nil && len(allDecisions) == 0 {
		findings = append(findings, AuditFinding{
			Domain:      "Decisions",
			Severity:    "tip",
			Title:       "Decision Journal Unused",
			Description: "Logging 1–2 key bets per quarter prevents revisionist memory and sharpens your decision calibration over years.",
			Command:     "ierp insert-decision --title <Bet> --choice <Chosen Option> --confidence <1-10> --review-date <YYYY-MM-DD>",
		})
	}

	// 3. Human Capital & Reconnection Radar
	t1 := 1
	t1Overdue, err := radar.ComputeRadar(database, &t1, true, 5, 0)
	if err == nil && len(t1Overdue) > 0 {
		var names []string
		for i, c := range t1Overdue {
			if i >= 3 {
				break
			}
			names = append(names, fmt.Sprintf("%s (%dd late)", c.Name, c.DaysOverdue))
		}
		findings = append(findings, AuditFinding{
			Domain:      "Radar",
			Severity:    "warning",
			Title:       fmt.Sprintf("%d Inner Circle (Tier 1) Contact(s) Overdue", len(t1Overdue)),
			Description: fmt.Sprintf("Close relationships needing touchpoints: %s.", strings.Join(names, ", ")),
			Command:     "ierp radar --tier 1 --overdue-only",
		})
	}

	radarSummary, err := radar.GetRadarSummary(database)
	if err == nil && radarSummary.TierCounts[1] == 0 && radarSummary.TotalContacts > 0 {
		findings = append(findings, AuditFinding{
			Domain:      "Radar",
			Severity:    "tip",
			Title:       "No Contacts Assigned to Tier 1",
			Description: "All contacts are currently on default tiers. Designate your inner circle (family, closest friends) to activate radar alerts.",
			Command:     "ierp set-tier --contact-id <id> --tier 1 --cadence 14",
		})
	}

	// 4. Life Ops & Maintenance Audit
	pendingMaint, err := lifeops.ListMaintenance(database, "pending", "", false)
	if err == nil {
		var overdueMaint []lifeops.MaintenanceItem
		for _, m := range pendingMaint {
			if m.IsOverdue {
				overdueMaint = append(overdueMaint, m)
			}
		}
		if len(overdueMaint) > 0 {
			var names []string
			for i, m := range overdueMaint {
				if i >= 3 {
					break
				}
				names = append(names, fmt.Sprintf("#%d %s (due %s)", m.ID, m.Name, m.DueDate))
			}
			findings = append(findings, AuditFinding{
				Domain:      "Life Ops",
				Severity:    "action_needed",
				Title:       fmt.Sprintf("%d Maintenance / Expiration Item(s) Overdue", len(overdueMaint)),
				Description: fmt.Sprintf("Overdue items: %s.", strings.Join(names, ", ")),
				Command:     fmt.Sprintf("ierp complete-maintenance %d", overdueMaint[0].ID),
			})
		} else if len(pendingMaint) == 0 {
			findings = append(findings, AuditFinding{
				Domain:      "Life Ops",
				Severity:    "tip",
				Title:       "No Preventive Maintenance Scheduled",
				Description: "Keep life frictionless by scheduling vehicle oil changes, AC cleaning, and passport/domain expirations.",
				Command:     "ierp insert-maintenance --name <Task/Doc> --due-date <YYYY-MM-DD> --interval <days>",
			})
		}
	}

	// 5. Sprint Retrospectives
	retros, err := reviews.ListRetrospectives(database, "", 1)
	if err == nil {
		if len(retros) == 0 {
			findings = append(findings, AuditFinding{
				Domain:      "Retrospectives",
				Severity:    "action_needed",
				Title:       "No Retrospectives Logged",
				Description: "Double-loop learning requires weekly or monthly retrospectives to identify what drained your energy and what to focus on next.",
				Command:     "ierp insert-review --type weekly --start <YYYY-MM-DD> --end <YYYY-MM-DD> --wins <...> --focus <...>",
			})
		} else {
			latestRetro := retros[0]
			daysSinceRetro := 999
			if len(latestRetro.PeriodEnd) >= 10 {
				if t, err := time.Parse("2006-01-02", latestRetro.PeriodEnd[:10]); err == nil {
					daysSinceRetro = int(now.Sub(t).Hours() / 24)
				}
			}
			if daysSinceRetro > 14 {
				findings = append(findings, AuditFinding{
					Domain:      "Retrospectives",
					Severity:    "warning",
					Title:       fmt.Sprintf("Retrospective Due (%d days since last review)", daysSinceRetro),
					Description: fmt.Sprintf("Last review covered up to %s. Time to reflect on the recent sprint.", latestRetro.PeriodEnd),
					Command:     "ierp insert-review --type weekly --start <YYYY-MM-DD> --end <YYYY-MM-DD>",
				})
			}
		}
	}

	// 6. Strategic Projects & Unlinked Events
	projs, err := projects.ListProjects(database, "active")
	if err == nil && len(projs) == 0 {
		findings = append(findings, AuditFinding{
			Domain:      "Projects",
			Severity:    "tip",
			Title:       "No Active Strategic Initiatives",
			Description: "Group your current quarterly bets and goals into projects so daily timeline events contribute to clear outcomes.",
			Command:     "ierp insert-project --title <Project Name> --priority high",
		})
	}

	var unlinkedCount int
	_ = database.QueryRow(`
		SELECT COUNT(*) FROM events 
		WHERE project_id IS NULL AND start_date >= date('now', '-30 days')
	`).Scan(&unlinkedCount)

	if unlinkedCount > 5 {
		findings = append(findings, AuditFinding{
			Domain:      "Projects",
			Severity:    "tip",
			Title:       fmt.Sprintf("%d Recent Events Unlinked to Projects", unlinkedCount),
			Description: "Linking daily events to projects clarifies which life bets consume the bulk of your time.",
			Command:     "ierp list --limit 10",
		})
	}

	result := &AuditResult{
		GeneratedAt:   todayStr,
		TotalFindings: len(findings),
		Findings:      findings,
	}

	for _, f := range findings {
		switch f.Severity {
		case "action_needed":
			result.ActionsNeeded++
		case "warning":
			result.Warnings++
		case "tip":
			result.Tips++
		}
	}

	return result, nil
}
