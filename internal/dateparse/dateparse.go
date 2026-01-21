package dateparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Supported date formats
var dateFormats = []string{
	"01-02-2006", // MM-DD-YYYY (primary format)
	"2006-01-02", // ISO 8601 (YYYY-MM-DD)
	"01/02/2006", // MM/DD/YYYY
	"2006/01/02", // YYYY/MM/DD
	"Jan 2, 2006",     // Month Day, Year
	"January 2, 2006", // Full Month Day, Year
	"2 Jan 2006",      // Day Month Year
	"2 January 2006",  // Day Full Month Year
}

// NamedPeriod represents a named time period
type NamedPeriod struct {
	Name      string
	StartDate time.Time
	EndDate   time.Time
}

// GetNamedPeriods returns available named periods based on current time
func GetNamedPeriods() map[string]NamedPeriod {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	periods := make(map[string]NamedPeriod)

	// This week (Sunday to Saturday, or Monday to Sunday depending on locale)
	weekday := int(today.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday is 7
	}
	thisWeekStart := today.AddDate(0, 0, -weekday+1) // Monday
	thisWeekEnd := thisWeekStart.AddDate(0, 0, 6)     // Sunday
	periods["this-week"] = NamedPeriod{"This Week", thisWeekStart, thisWeekEnd}

	// Last week
	lastWeekStart := thisWeekStart.AddDate(0, 0, -7)
	lastWeekEnd := thisWeekStart.AddDate(0, 0, -1)
	periods["last-week"] = NamedPeriod{"Last Week", lastWeekStart, lastWeekEnd}

	// This month
	thisMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	thisMonthEnd := thisMonthStart.AddDate(0, 1, -1)
	periods["this-month"] = NamedPeriod{"This Month", thisMonthStart, thisMonthEnd}

	// Last month
	lastMonthEnd := thisMonthStart.AddDate(0, 0, -1)
	lastMonthStart := time.Date(lastMonthEnd.Year(), lastMonthEnd.Month(), 1, 0, 0, 0, 0, now.Location())
	periods["last-month"] = NamedPeriod{"Last Month", lastMonthStart, lastMonthEnd}

	// This quarter
	quarter := (int(now.Month()) - 1) / 3
	thisQuarterStart := time.Date(now.Year(), time.Month(quarter*3+1), 1, 0, 0, 0, 0, now.Location())
	thisQuarterEnd := thisQuarterStart.AddDate(0, 3, -1)
	periods["this-quarter"] = NamedPeriod{"This Quarter", thisQuarterStart, thisQuarterEnd}

	// Last quarter
	lastQuarterEnd := thisQuarterStart.AddDate(0, 0, -1)
	lastQuarterStart := time.Date(lastQuarterEnd.Year(), time.Month((int(lastQuarterEnd.Month())-1)/3*3+1), 1, 0, 0, 0, 0, now.Location())
	periods["last-quarter"] = NamedPeriod{"Last Quarter", lastQuarterStart, lastQuarterEnd}

	// This year
	thisYearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	thisYearEnd := time.Date(now.Year(), 12, 31, 0, 0, 0, 0, now.Location())
	periods["this-year"] = NamedPeriod{"This Year", thisYearStart, thisYearEnd}

	// Last year
	lastYearStart := time.Date(now.Year()-1, 1, 1, 0, 0, 0, 0, now.Location())
	lastYearEnd := time.Date(now.Year()-1, 12, 31, 0, 0, 0, 0, now.Location())
	periods["last-year"] = NamedPeriod{"Last Year", lastYearStart, lastYearEnd}

	// Quarters by name (Q1-Q4 for current year)
	for q := 1; q <= 4; q++ {
		qStart := time.Date(now.Year(), time.Month((q-1)*3+1), 1, 0, 0, 0, 0, now.Location())
		qEnd := qStart.AddDate(0, 3, -1)
		key := fmt.Sprintf("q%d-%d", q, now.Year())
		periods[key] = NamedPeriod{fmt.Sprintf("Q%d %d", q, now.Year()), qStart, qEnd}
	}

	// Quarters for last year
	for q := 1; q <= 4; q++ {
		qStart := time.Date(now.Year()-1, time.Month((q-1)*3+1), 1, 0, 0, 0, 0, now.Location())
		qEnd := qStart.AddDate(0, 3, -1)
		key := fmt.Sprintf("q%d-%d", q, now.Year()-1)
		periods[key] = NamedPeriod{fmt.Sprintf("Q%d %d", q, now.Year()-1), qStart, qEnd}
	}

	return periods
}

// ParseDate attempts to parse a date string using multiple formats
// Returns the parsed time and the format used, or an error if parsing fails
func ParseDate(input string) (time.Time, error) {
	input = strings.TrimSpace(input)

	// Try each format
	for _, format := range dateFormats {
		if t, err := time.Parse(format, input); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date '%s': supported formats are MM-DD-YYYY, YYYY-MM-DD, or natural language like 'last monday'", input)
}

// ParseRelativeDate parses relative date expressions like "last monday", "2 weeks ago", etc.
func ParseRelativeDate(input string) (time.Time, error) {
	input = strings.ToLower(strings.TrimSpace(input))
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Handle "today" and "yesterday"
	if input == "today" {
		return today, nil
	}
	if input == "yesterday" {
		return today.AddDate(0, 0, -1), nil
	}

	// Handle "X days/weeks/months ago"
	agoPattern := regexp.MustCompile(`^(\d+)\s+(day|days|week|weeks|month|months)\s+ago$`)
	if matches := agoPattern.FindStringSubmatch(input); len(matches) == 3 {
		num, _ := strconv.Atoi(matches[1])
		unit := matches[2]

		switch {
		case strings.HasPrefix(unit, "day"):
			return today.AddDate(0, 0, -num), nil
		case strings.HasPrefix(unit, "week"):
			return today.AddDate(0, 0, -num*7), nil
		case strings.HasPrefix(unit, "month"):
			return today.AddDate(0, -num, 0), nil
		}
	}

	// Handle "last <weekday>"
	weekdays := map[string]time.Weekday{
		"sunday":    time.Sunday,
		"monday":    time.Monday,
		"tuesday":   time.Tuesday,
		"wednesday": time.Wednesday,
		"thursday":  time.Thursday,
		"friday":    time.Friday,
		"saturday":  time.Saturday,
	}

	lastWeekdayPattern := regexp.MustCompile(`^last\s+(\w+)$`)
	if matches := lastWeekdayPattern.FindStringSubmatch(input); len(matches) == 2 {
		weekdayName := matches[1]
		if targetWeekday, ok := weekdays[weekdayName]; ok {
			currentWeekday := today.Weekday()
			daysBack := int(currentWeekday) - int(targetWeekday)
			if daysBack <= 0 {
				daysBack += 7
			}
			return today.AddDate(0, 0, -daysBack), nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse relative date '%s'", input)
}

// ParseDateOrRelative attempts to parse as an absolute date first, then as a relative date
func ParseDateOrRelative(input string) (time.Time, error) {
	// First try absolute date parsing
	if t, err := ParseDate(input); err == nil {
		return t, nil
	}

	// Then try relative date parsing
	if t, err := ParseRelativeDate(input); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("unable to parse date '%s': try formats like '01-15-2025', '2025-01-15', 'last monday', or '2 weeks ago'", input)
}

// ParseNamedPeriod parses a named period string and returns start and end dates
func ParseNamedPeriod(name string) (time.Time, time.Time, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	periods := GetNamedPeriods()

	if period, ok := periods[name]; ok {
		return period.StartDate, period.EndDate, nil
	}

	// List available periods for error message
	available := make([]string, 0, len(periods))
	for k := range periods {
		available = append(available, k)
	}

	return time.Time{}, time.Time{}, fmt.Errorf("unknown period '%s': available periods are %s", name, strings.Join(available[:8], ", "))
}

// FormatForDisplay formats a time.Time for display in output
func FormatForDisplay(t time.Time) string {
	return t.Format("January 2, 2006")
}

// FormatForAPI formats a time.Time for API calls (MM-DD-YYYY format for perfdive internal use)
func FormatForAPI(t time.Time) string {
	return t.Format("01-02-2006")
}

// FormatISO formats a time.Time in ISO 8601 format (for GitHub API)
func FormatISO(t time.Time) string {
	return t.Format("2006-01-02")
}

// ValidateDateRange ensures start date is before end date
func ValidateDateRange(start, end time.Time) error {
	if start.After(end) {
		return fmt.Errorf("start date (%s) must be before end date (%s)",
			FormatForDisplay(start), FormatForDisplay(end))
	}
	return nil
}
