package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nichsedge/ierp/internal/antivision"
	"github.com/nichsedge/ierp/internal/career"
	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/db"
)

type ParsedData struct {
	AntiVision []struct {
		Description string   `json:"description"`
		Domains     []string `json:"domains"`
		Status      string   `json:"status"`
		Severity    int      `json:"severity"`
		DateLogged  string   `json:"date_logged"`
	} `json:"anti_vision"`
	Roles []struct {
		Role       string `json:"role"`
		Status     string `json:"status"`
		DateLogged string `json:"date_logged"`
	} `json:"roles"`
	Skills []struct {
		Skill      string `json:"skill"`
		SkillType  string `json:"skill_type"`
		Status     string `json:"status"`
		DateLogged string `json:"date_logged"`
	} `json:"skills"`
}

func main() {
	jsonPath := "/home/al/.gemini/antigravity/brain/55ecb199-d6c9-4daa-a326-aa4d4ac723fa/scratch/logs_parsed.json"
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read json: %v\n", err)
		os.Exit(1)
	}

	var data ParsedData
	if err := json.Unmarshal(raw, &data); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse json: %v\n", err)
		os.Exit(1)
	}

	database, err := db.Open(config.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open db: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// 1. Ingest AntiVision
	avCount := 0
	for _, it := range data.AntiVision {
		domainsStr := ""
		for i, d := range it.Domains {
			if i > 0 {
				domainsStr += ","
			}
			domainsStr += d
		}

		item := antivision.AntiVisionItem{
			Description: it.Description,
			Domains:     domainsStr,
			Status:      it.Status,
			Severity:    it.Severity,
			DateLogged:  it.DateLogged,
			Notes:       "Imported from logs.xlsx",
		}
		if _, err := antivision.InsertAntiVision(database, item); err != nil {
			fmt.Printf("Warning inserting %s: %v\n", it.Description, err)
		} else {
			avCount++
		}
	}
	fmt.Printf("✓ Ingested %d anti-vision items\n", avCount)

	// Role categorization map
	catMap := map[string]string{
		"system analyst":           "Engineering",
		"ui/ux":                    "Design",
		"web developer":            "Engineering",
		"blockchain developer":     "Engineering",
		"cloud computing developer": "Cloud & DevOps",
		"data analyst":             "Data & Analytics",
		"teacher":                  "Education",
		"quant / algo trader":      "Quantitative Finance",
		"philosopher / writer":     "Writing & Philosophy",
		"solution architect":       "Architecture",
		"data engineer":            "Data Engineering",
	}

	// 2. Ingest Roles
	roleCount := 0
	for _, r := range data.Roles {
		cat := catMap[r.Role]
		status := r.Status
		notes := "Imported from logs.xlsx"
		if r.Role == "data engineer" {
			status = "active"
			notes = "Primary enterprise technical discipline"
		} else if r.Role == "web developer" {
			status = "pivoted"
			notes = "Pivoted away; frontend moved to anti-vision"
		} else if r.Role == "philosopher / writer" {
			notes = "Non-commercial essayist & philosophical reflections in digital-graveyard"
		}

		role := career.RoleExploration{
			Role:       r.Role,
			Category:   cat,
			Status:     status,
			DateLogged: r.DateLogged,
			Notes:      notes,
		}
		if _, err := career.InsertRole(database, role); err != nil {
			fmt.Printf("Warning inserting role %s: %v\n", r.Role, err)
		} else {
			roleCount++
		}
	}
	fmt.Printf("✓ Ingested %d role explorations\n", roleCount)

	// 3. Ingest Skills
	skillCount := 0
	for _, s := range data.Skills {
		sk := career.SkillInventory{
			Skill:      s.Skill,
			SkillType:  s.SkillType,
			Status:     s.Status,
			DateLogged: s.DateLogged,
			Notes:      "Imported from logs.xlsx",
		}
		if _, err := career.InsertSkill(database, sk); err != nil {
			fmt.Printf("Warning inserting skill %s: %v\n", s.Skill, err)
		} else {
			skillCount++
		}
	}
	fmt.Printf("✓ Ingested %d skills\n", skillCount)
}
