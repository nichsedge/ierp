package importers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/nichsedge/ierp/internal/events"
)

var monthMap = map[string]string{
	"january": "01", "jan": "01",
	"february": "02", "feb": "02",
	"march": "03", "mar": "03",
	"april": "04", "apr": "04",
	"may": "05",
	"june": "06", "jun": "06",
	"july": "07", "jul": "07",
	"august": "08", "aug": "08",
	"september": "09", "sep": "09", "sept": "09",
	"october": "10", "oct": "10",
	"november": "11", "nov": "11",
	"december": "12", "dec": "12",
}

// ParseDateToISO parses natural language or Notion date ranges into ISO 8601 strings.
func ParseDateToISO(dateStr string) (string, string) {
	clean := strings.TrimSpace(dateStr)
	if clean == "" || strings.ToLower(clean) == "none" {
		return "", ""
	}

	if strings.Contains(clean, "->") || strings.Contains(clean, "→") {
		parts := regexp.MustCompile(`\s*(?:->|→)\s*`).Split(clean, -1)
		start := normalizeSingleDate(parts[0])
		var end string
		if len(parts) > 1 {
			end = normalizeSingleDate(parts[1])
		}
		return start, end
	}

	return normalizeSingleDate(clean), ""
}

var isoRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(?:[T\s]\d{2}:\d{2}(?::\d{2})?)?`)
var monthDayYearRegex = regexp.MustCompile(`^([A-Za-z]+)\s+(\d{1,2}),?\s*(\d{4})(?:\s+(\d{1,2}:\d{2}(?::\d{2})?))?`)
var dayMonthYearRegex = regexp.MustCompile(`^(\d{1,2})\s+([A-Za-z]+)\s+(\d{4})(?:\s+(\d{1,2}:\d{2}(?::\d{2})?))?`)

func normalizeSingleDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	if isoRegex.MatchString(s) {
		if strings.Contains(s, " ") && len(s) > 10 {
			return strings.Replace(s, " ", "T", 1)
		}
		return s
	}

	if m := monthDayYearRegex.FindStringSubmatch(s); m != nil {
		mName := strings.ToLower(m[1])
		if mNum, ok := monthMap[mName]; ok {
			day, _ := strconv.Atoi(m[2])
			year := m[3]
			timePart := m[4]
			if timePart != "" {
				return fmt.Sprintf("%s-%s-%02dT%s", year, mNum, day, timePart)
			}
			return fmt.Sprintf("%s-%s-%02d", year, mNum, day)
		}
	}

	if m := dayMonthYearRegex.FindStringSubmatch(s); m != nil {
		day, _ := strconv.Atoi(m[1])
		mName := strings.ToLower(m[2])
		if mNum, ok := monthMap[mName]; ok {
			year := m[3]
			timePart := m[4]
			if timePart != "" {
				return fmt.Sprintf("%s-%s-%02dT%s", year, mNum, day, timePart)
			}
			return fmt.Sprintf("%s-%s-%02d", year, mNum, day)
		}
	}

	return s
}

// ImportTimelineSemantic ingests Google Maps Semantic Location History JSON exports.
func ImportTimelineSemantic(database *sql.DB, filePath string) (int, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return 0, err
	}

	var data struct {
		TimelineObjects []struct {
			PlaceVisit struct {
				Location struct {
					Name    string `json:"name"`
					Address string `json:"address"`
				} `json:"location"`
				Duration struct {
					StartTimestamp string `json:"startTimestamp"`
					EndTimestamp   string `json:"endTimestamp"`
				} `json:"duration"`
			} `json:"placeVisit"`
		} `json:"timelineObjects"`
	}

	if err := json.Unmarshal(bytes, &data); err != nil {
		return 0, fmt.Errorf("failed to parse timeline JSON: %w", err)
	}

	imported := 0
	for _, obj := range data.TimelineObjects {
		pv := obj.PlaceVisit
		if pv.Location.Name == "" {
			continue
		}

		start := pv.Duration.StartTimestamp
		end := pv.Duration.EndTimestamp
		if len(start) > 19 {
			start = start[:19]
		}
		if len(end) > 19 {
			end = end[:19]
		}

		title := fmt.Sprintf("Visited %s", pv.Location.Name)
		place := pv.Location.Name
		if pv.Location.Address != "" {
			place = fmt.Sprintf("%s, %s", pv.Location.Name, pv.Location.Address)
		}

		ev := events.Event{
			Title:     title,
			Place:     place,
			StartDate: start,
			EndDate:   end,
			RawDate:   start,
			Tags:      []string{"timeline", "location"},
		}

		if _, _, err := events.InsertEvent(database, ev, nil); err == nil {
			imported++
		}
	}

	return imported, nil
}
