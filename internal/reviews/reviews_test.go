package reviews_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/reviews"
)

func TestReviewsCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_reviews.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	id, err := reviews.InsertRetrospective(database, reviews.Retrospective{
		PeriodStart:   "2026-09-01",
		PeriodEnd:     "2026-09-30",
		PeriodType:    "monthly",
		Wins:          "Completed Go migration Phase 1",
		DrainsBurnout: "Context switching",
		Lessons:       "Standard library is king",
		FocusNext:     "Web dashboard and export pipelines",
		Rating:        9,
	})
	if err != nil || id <= 0 {
		t.Fatalf("InsertRetrospective failed: id=%d, err=%v", id, err)
	}

	r, err := reviews.GetRetrospective(database, id)
	if err != nil || r == nil || r.Rating != 9 {
		t.Fatalf("GetRetrospective failed: %+v, err=%v", r, err)
	}

	ok, err := reviews.UpdateRetrospective(database, id, map[string]any{"rating": 10})
	if err != nil || !ok {
		t.Errorf("UpdateRetrospective failed: ok=%v, err=%v", ok, err)
	}

	list, err := reviews.ListRetrospectives(database, "monthly", 10)
	if err != nil || len(list) != 1 || list[0].Rating != 10 {
		t.Errorf("ListRetrospectives failed: %+v, err=%v", list, err)
	}

	ok, err = reviews.DeleteRetrospective(database, id)
	if err != nil || !ok {
		t.Errorf("DeleteRetrospective failed: ok=%v, err=%v", ok, err)
	}
}
