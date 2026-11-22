package commands

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/nichsedge/ierp/internal/career"
	"github.com/nichsedge/ierp/internal/config"
)

func handleInsertRole(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-role", flag.ContinueOnError)
	fs.SetOutput(stderr)
	role := fs.String("role", "", "Role title (required)")
	category := fs.String("category", "", "Category (engineering, design, trading, writing, education)")
	status := fs.String("status", "explored", "Status (explored, active, pivoted, shelved)")
	dateLogged := fs.String("date", time.Now().Format("2006-01-02"), "Date logged (YYYY-MM-DD)")
	notes := fs.String("notes", "", "Reflection or notes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *role == "" {
		fmt.Fprintf(stderr, "%sError: --role is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("role is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	id, err := career.InsertRole(database, career.RoleExploration{
		Role:       *role,
		Category:   *category,
		Status:     *status,
		DateLogged: *dateLogged,
		Notes:      *notes,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	fmt.Fprintf(stdout, "%sRole exploration #%d recorded: %s%s\n", config.Green, id, *role, config.Reset)
	return nil
}

func handleListRoles(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("roles", flag.ContinueOnError)
	fs.SetOutput(stderr)
	status := fs.String("status", "", "Filter by status (explored, active, pivoted, all)")
	jsonOutput := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	roles, err := career.ListRoles(database, *status)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	if *jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(roles)
		return nil
	}

	fmt.Fprintf(stdout, "%s=== Role Explorations (%d entries) ===%s\n", config.Bold, len(roles), config.Reset)
	for _, r := range roles {
		fmt.Fprintf(stdout, "#%-3d | %-26s | %-18s | %-10s | %s\n", r.ID, r.Role, r.Category, r.Status, r.DateLogged)
	}
	return nil
}

func handleInsertSkill(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-skill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	skill := fs.String("skill", "", "Skill name (required)")
	skillType := fs.String("type", "soft", "Type (soft, hard)")
	status := fs.String("status", "active", "Status (active, learning, dormant)")
	dateLogged := fs.String("date", time.Now().Format("2006-01-02"), "Date logged (YYYY-MM-DD)")
	notes := fs.String("notes", "", "Notes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *skill == "" {
		fmt.Fprintf(stderr, "%sError: --skill is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("skill is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	id, err := career.InsertSkill(database, career.SkillInventory{
		Skill:      *skill,
		SkillType:  *skillType,
		Status:     *status,
		DateLogged: *dateLogged,
		Notes:      *notes,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	fmt.Fprintf(stdout, "%sSkill #%d recorded: %s (%s)%s\n", config.Green, id, *skill, *skillType, config.Reset)
	return nil
}

func handleListSkills(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("skills", flag.ContinueOnError)
	fs.SetOutput(stderr)
	skillType := fs.String("type", "", "Filter by type (soft, hard, all)")
	jsonOutput := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	skills, err := career.ListSkills(database, *skillType)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	if *jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(skills)
		return nil
	}

	fmt.Fprintf(stdout, "%s=== Capability & Skills Inventory (%d entries) ===%s\n", config.Bold, len(skills), config.Reset)
	for _, s := range skills {
		fmt.Fprintf(stdout, "#%-3d | %-24s | %-6s | %-8s | %s\n", s.ID, s.Skill, s.SkillType, s.Status, s.DateLogged)
	}
	return nil
}
