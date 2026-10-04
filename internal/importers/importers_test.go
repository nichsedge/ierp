package importers_test

import (
	"testing"

	"github.com/nichsedge/ierp/internal/importers"
)

func TestParseDateToISO(t *testing.T) {
	cases := []struct {
		input     string
		wantStart string
		wantEnd   string
	}{
		{"2024-01-15", "2024-01-15", ""},
		{"January 15, 2024", "2024-01-15", ""},
		{"15 January 2024", "2024-01-15", ""},
		{"2024-01-01 -> 2024-01-10", "2024-01-01", "2024-01-10"},
		{"January 1, 2024 → January 10, 2024", "2024-01-01", "2024-01-10"},
	}

	for _, c := range cases {
		start, end := importers.ParseDateToISO(c.input)
		if start != c.wantStart || end != c.wantEnd {
			t.Errorf("ParseDateToISO(%q) = (%q, %q); want (%q, %q)", c.input, start, end, c.wantStart, c.wantEnd)
		}
	}
}
