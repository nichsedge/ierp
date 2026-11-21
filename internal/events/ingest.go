package events

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

	"github.com/nichsedge/ierp/internal/contacts"
)

var (
	reFrontmatterBlock = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?(.*)$`)
	reWikilink         = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)
	reDurationDays     = regexp.MustCompile(`(?i)(?:a\s+)?(\d+|two|three|four|five|six|seven)\s*-?\s*days?`)
)

// IngestResult records the outcome of an event ingestion.
type IngestResult struct {
	EventID   int      `json:"event_id"`
	Title     string   `json:"title"`
	StartDate string   `json:"start_date"`
	EndDate   string   `json:"end_date"`
	Place     string   `json:"place"`
	Tags      []string `json:"tags"`
	Contacts  []string `json:"contacts"`
	SourceDoc string   `json:"source_doc"`
}

// IngestFile reads a markdown or JSON file and inserts it as an event into iERP.
func IngestFile(database *sql.DB, filePath, defaultTag, contactsOverride, placeOverride string) (*IngestResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	content := string(data)
	absPath, _ := filepath.Abs(filePath)

	if strings.HasSuffix(strings.ToLower(filePath), ".json") {
		return ingestJSON(database, content, absPath, defaultTag, contactsOverride, placeOverride)
	}

	return ingestMarkdown(database, content, absPath, defaultTag, contactsOverride, placeOverride)
}

func ingestMarkdown(database *sql.DB, content, filePath, defaultTag, contactsOverride, placeOverride string) (*IngestResult, error) {
	var title, startDate, endDate, place string
	var tags []string
	var contactRefs []string
	body := content

	// Parse YAML frontmatter if present
	if match := reFrontmatterBlock.FindStringSubmatch(content); len(match) == 3 {
		fm := match[1]
		body = strings.TrimSpace(match[2])

		for _, line := range strings.Split(fm, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)

			switch key {
			case "title":
				title = val
			case "date":
				startDate = val
			case "end_date", "end":
				endDate = val
			case "place", "location":
				place = val
			case "tags":
				val = strings.Trim(val, "[]")
				for _, t := range strings.Split(val, ",") {
					if tr := strings.Trim(strings.TrimSpace(t), `"'`); tr != "" {
						tags = append(tags, tr)
					}
				}
			case "contacts", "contact":
				val = strings.Trim(val, "[]")
				for _, c := range strings.Split(val, ",") {
					if cr := strings.Trim(strings.TrimSpace(c), `"'`); cr != "" {
						contactRefs = append(contactRefs, cr)
					}
				}
			}
		}
	}

	// CLI Overrides
	if placeOverride != "" {
		place = placeOverride
	}
	if contactsOverride != "" {
		for _, c := range strings.Split(contactsOverride, ",") {
			if cr := strings.TrimSpace(c); cr != "" {
				contactRefs = append(contactRefs, cr)
			}
		}
	}

	// Fallback title: first markdown heading or filename
	if title == "" {
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
				break
			}
		}
	}
	if title == "" {
		base := filepath.Base(filePath)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	// Fallback date: today
	if startDate == "" {
		startDate = time.Now().Format("2006-01-02")
	}

	// Extract place from wikilinks if not specified
	if place == "" {
		wikiMatches := reWikilink.FindAllStringSubmatch(content, -1)
		for _, wm := range wikiMatches {
			w := wm[1]
			// Exclude common structural tags like [[Travel]], [[Read]], [[Write]]
			if !strings.EqualFold(w, "travel") && !strings.EqualFold(w, "read") && !strings.EqualFold(w, "write") && !strings.EqualFold(w, "gojek") {
				place = w
				break
			}
		}
	}

	// Calculate end_date if multi-day detected
	if endDate == "" {
		numDays := 1
		textToScan := title + " "
		if len(body) > 500 {
			textToScan += body[:500]
		} else {
			textToScan += body
		}

		if dm := reDurationDays.FindStringSubmatch(textToScan); len(dm) > 1 {
			w := strings.ToLower(dm[1])
			switch w {
			case "two", "2":
				numDays = 2
			case "three", "3":
				numDays = 3
			case "four", "4":
				numDays = 4
			case "five", "5":
				numDays = 5
			case "six", "6":
				numDays = 6
			case "seven", "7":
				numDays = 7
			default:
				if n, err := strconv.Atoi(w); err == nil && n > 0 && n <= 30 {
					numDays = n
				}
			}
		}
		if numDays > 1 {
			if t, err := time.Parse("2006-01-02", startDate); err == nil {
				endDate = t.AddDate(0, 0, numDays-1).Format("2006-01-02")
			}
		}
	}

	// Automatic contact detection if none provided
	if len(contactRefs) == 0 {
		lowerContent := strings.ToLower(content)
		if strings.Contains(lowerContent, "firzana") || strings.Contains(lowerContent, "my gf") || strings.Contains(lowerContent, "with gf") {
			if _, cname, err := contacts.ResolveContact(database, "Firzana"); err == nil && cname != "" {
				contactRefs = append(contactRefs, cname)
			} else {
				contactRefs = append(contactRefs, "Firzana Farah Amira")
			}
		}
	}

	// Align title if contact detected and not already mentioned in title
	if len(contactRefs) > 0 {
		firstContact := contactRefs[0]
		shortName := strings.Split(firstContact, " ")[0]
		if !strings.Contains(strings.ToLower(title), "with") && !strings.Contains(strings.ToLower(title), strings.ToLower(shortName)) {
			title = fmt.Sprintf("%s with %s", title, shortName)
		}
	}

	// Tags processing
	tagMap := make(map[string]bool)
	for _, t := range tags {
		tagMap[strings.ToLower(t)] = true
	}
	if defaultTag != "" {
		for _, t := range strings.Split(defaultTag, ",") {
			if tr := strings.TrimSpace(t); tr != "" {
				tagMap[strings.ToLower(tr)] = true
			}
		}
	}
	if place != "" {
		tagMap[strings.ToLower(place)] = true
	}
	tagMap["travel"] = true

	var finalTags []string
	for t := range tagMap {
		finalTags = append(finalTags, t)
	}

	urlRef := fmt.Sprintf("file://%s", filePath)

	ev := Event{
		Title:     title,
		Place:     place,
		StartDate: startDate,
		EndDate:   endDate,
		Tags:      finalTags,
		URL:       urlRef,
		Notes:     body,
	}

	evID, linkedContacts, err := InsertEvent(database, ev, contactRefs)
	if err != nil {
		return nil, fmt.Errorf("failed inserting event: %w", err)
	}

	return &IngestResult{
		EventID:   evID,
		Title:     ev.Title,
		StartDate: ev.StartDate,
		EndDate:   ev.EndDate,
		Place:     ev.Place,
		Tags:      ev.Tags,
		Contacts:  linkedContacts,
		SourceDoc: filePath,
	}, nil
}

func ingestJSON(database *sql.DB, content, filePath, defaultTag, contactsOverride, placeOverride string) (*IngestResult, error) {
	var ev Event
	if err := json.Unmarshal([]byte(content), &ev); err != nil {
		return nil, fmt.Errorf("failed to parse JSON event: %w", err)
	}

	if placeOverride != "" {
		ev.Place = placeOverride
	}
	if ev.URL == "" {
		ev.URL = fmt.Sprintf("file://%s", filePath)
	}

	var contactRefs []string
	for _, c := range ev.Contacts {
		if c.Name != "" {
			contactRefs = append(contactRefs, c.Name)
		}
	}
	if contactsOverride != "" {
		for _, c := range strings.Split(contactsOverride, ",") {
			if cr := strings.TrimSpace(c); cr != "" {
				contactRefs = append(contactRefs, cr)
			}
		}
	}
	if defaultTag != "" {
		for _, t := range strings.Split(defaultTag, ",") {
			if tr := strings.TrimSpace(t); tr != "" {
				ev.Tags = append(ev.Tags, tr)
			}
		}
	}

	evID, linked, err := InsertEvent(database, ev, contactRefs)
	if err != nil {
		return nil, err
	}

	return &IngestResult{
		EventID:   evID,
		Title:     ev.Title,
		StartDate: ev.StartDate,
		EndDate:   ev.EndDate,
		Place:     ev.Place,
		Tags:      ev.Tags,
		Contacts:  linked,
		SourceDoc: filePath,
	}, nil
}
