package portfolio

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunHelpAndUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"help"}, &stdout, &stderr); err != nil {
		t.Fatalf("Run help failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "iERP Portfolio Engine") {
		t.Errorf("expected help banner, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run(context.Background(), []string{"nonexistent-cmd"}, &stdout, &stderr); err == nil {
		t.Fatal("expected error for unknown portfolio command, got nil")
	}
}

func TestIsMonthlyWeekdayRun(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)

	cases := []struct {
		name string
		date time.Time
		want bool
	}{
		{"25th Wednesday", time.Date(2026, 11, 25, 17, 0, 0, 0, wib), true},
		{"25th Saturday", time.Date(2026, 4, 25, 17, 0, 0, 0, wib), false},
		{"26th Sunday", time.Date(2026, 4, 26, 17, 0, 0, 0, wib), false},
		{"27th Monday (after 25th Sat)", time.Date(2026, 4, 27, 17, 0, 0, 0, wib), true},
		{"25th Sunday", time.Date(2026, 10, 25, 17, 0, 0, 0, wib), false},
		{"26th Monday (after 25th Sun)", time.Date(2026, 10, 26, 17, 0, 0, 0, wib), true},
		{"27th Tuesday (after 25th Sun)", time.Date(2026, 10, 27, 17, 0, 0, 0, wib), false},
	}

	for _, tc := range cases {
		if got := isMonthlyWeekdayRun(tc.date); got != tc.want {
			t.Errorf("%s (%s): got %v, want %v", tc.name, tc.date.Format("2006-01-02 Mon"), got, tc.want)
		}
	}
}
