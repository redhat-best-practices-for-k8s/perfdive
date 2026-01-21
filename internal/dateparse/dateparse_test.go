package dateparse

import (
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"MM-DD-YYYY format", "01-15-2025", false},
		{"ISO format", "2025-01-15", false},
		{"MM/DD/YYYY format", "01/15/2025", false},
		{"YYYY/MM/DD format", "2025/01/15", false},
		{"Month Day Year format", "Jan 15, 2025", false},
		{"Full Month Day Year format", "January 15, 2025", false},
		{"Invalid format", "15-01-2025", true},
		{"Invalid format 2", "not-a-date", true},
		{"Empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestParseRelativeDate(t *testing.T) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	tests := []struct {
		name     string
		input    string
		expected time.Time
		wantErr  bool
	}{
		{"today", "today", today, false},
		{"yesterday", "yesterday", today.AddDate(0, 0, -1), false},
		{"1 day ago", "1 day ago", today.AddDate(0, 0, -1), false},
		{"5 days ago", "5 days ago", today.AddDate(0, 0, -5), false},
		{"1 week ago", "1 week ago", today.AddDate(0, 0, -7), false},
		{"2 weeks ago", "2 weeks ago", today.AddDate(0, 0, -14), false},
		{"1 month ago", "1 month ago", today.AddDate(0, -1, 0), false},
		{"3 months ago", "3 months ago", today.AddDate(0, -3, 0), false},
		{"invalid", "not-a-relative-date", time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRelativeDate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRelativeDate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.expected) {
				t.Errorf("ParseRelativeDate(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseDateOrRelative(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"ISO date", "2025-01-15", false},
		{"MM-DD-YYYY date", "01-15-2025", false},
		{"today", "today", false},
		{"yesterday", "yesterday", false},
		{"2 weeks ago", "2 weeks ago", false},
		{"invalid", "not-valid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDateOrRelative(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDateOrRelative(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestParseNamedPeriod(t *testing.T) {
	tests := []struct {
		name    string
		period  string
		wantErr bool
	}{
		{"this-week", "this-week", false},
		{"last-week", "last-week", false},
		{"this-month", "this-month", false},
		{"last-month", "last-month", false},
		{"this-quarter", "this-quarter", false},
		{"last-quarter", "last-quarter", false},
		{"this-year", "this-year", false},
		{"last-year", "last-year", false},
		{"invalid", "invalid-period", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := ParseNamedPeriod(tt.period)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseNamedPeriod(%q) error = %v, wantErr %v", tt.period, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if start.IsZero() || end.IsZero() {
					t.Errorf("ParseNamedPeriod(%q) returned zero time values", tt.period)
				}
				if start.After(end) {
					t.Errorf("ParseNamedPeriod(%q) start %v is after end %v", tt.period, start, end)
				}
			}
		})
	}
}

func TestValidateDateRange(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		start   time.Time
		end     time.Time
		wantErr bool
	}{
		{"valid range", now.AddDate(0, 0, -7), now, false},
		{"same day", now, now, false},
		{"invalid range (start after end)", now, now.AddDate(0, 0, -7), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDateRange(tt.start, tt.end)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDateRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFormatFunctions(t *testing.T) {
	// Use a fixed date for testing
	testDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

	t.Run("FormatForDisplay", func(t *testing.T) {
		got := FormatForDisplay(testDate)
		expected := "January 15, 2025"
		if got != expected {
			t.Errorf("FormatForDisplay() = %v, want %v", got, expected)
		}
	})

	t.Run("FormatForAPI", func(t *testing.T) {
		got := FormatForAPI(testDate)
		expected := "01-15-2025"
		if got != expected {
			t.Errorf("FormatForAPI() = %v, want %v", got, expected)
		}
	})

	t.Run("FormatISO", func(t *testing.T) {
		got := FormatISO(testDate)
		expected := "2025-01-15"
		if got != expected {
			t.Errorf("FormatISO() = %v, want %v", got, expected)
		}
	})
}
