package commons

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseDate parses a date string in one of several accepted formats.
// Accepted formats: ISO 8601 date (2006-01-02), RFC3339, DD-MMM-YYYY (02-Jan-2006),
// DD-MMM-YY (02-Jan-06), DD.MM.YYYY (02.01.2006), DD-MM-YY (02-01-06).
// Surrounding whitespace is trimmed first. Empty or whitespace-only input returns zero time with nil error.
// If no layout matches, returns zero time with an error mentioning the input.
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}

	layouts := []string{
		"2006-01-02",  // ISO 8601 date
		time.RFC3339,  // RFC3339
		"02-Jan-2006", // DD-MMM-YYYY
		"02-Jan-06",   // DD-MMM-YY
		"02.01.2006",  // DD.MM.YYYY
		"02-01-06",    // DD-MM-YY
	}

	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date %q", s)
}

// AddWarrantyEnd computes the warranty end date.
// If explicitEnd is non-nil, it takes precedence and a pointer to a copy is returned with ok=true.
// Otherwise the duration is parsed (e.g. "2 years", "24 months", "30 days", "2 yr", "30d",
// "P2Y6M"). If unparseable or purchase is nil, returns nil, false.
// On success, returns a pointer to purchase+duration with ok=true.
func AddWarrantyEnd(purchase *time.Time, duration string, explicitEnd *time.Time) (*time.Time, bool) {
	if explicitEnd != nil {
		cp := *explicitEnd
		return &cp, true
	}
	if purchase == nil {
		return nil, false
	}
	years, months, days, err := parseWarrantyDuration(duration)
	if err != nil {
		return nil, false
	}
	result := addWarranty(*purchase, years, months, days)
	return &result, true
}

// addWarranty adds years, months, and days to t, clamping the day to the
// last day of the resulting month (e.g. Jan 31 + 1 month = Feb 28/29).
func addWarranty(t time.Time, years, months, days int) time.Time {
	originalDay := t.Day()
	// Compute target year and month.
	totalMonths := t.Year()*12 + int(t.Month()) - 1 + years*12 + months
	targetYear := totalMonths / 12
	targetMonth := time.Month(totalMonths%12 + 1)
	// Clamp day to last day of target month.
	lastDay := lastDayOfMonth(targetYear, targetMonth)
	day := originalDay
	if day > lastDay {
		day = lastDay
	}
	result := time.Date(targetYear, targetMonth, day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	result = result.AddDate(0, 0, days)
	return result
}

func lastDayOfMonth(year int, m time.Month) int {
	return time.Date(year, m+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1).Day()
}

// parseWarrantyDuration parses a duration string into years, months, days.
// Accepts "N years/months/days" (case-insensitive, trailing "s" optional),
// "N yr/mo/d" abbreviations, and ISO-8601 "P" form (e.g. "P2Y6M", "P30D").
// Empty string, all-zero values, or negative values are errors.
func parseWarrantyDuration(s string) (int, int, int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, 0, 0, fmt.Errorf("empty duration")
	}

	if years, months, days, ok := parseISODuration(s); ok {
		if years == 0 && months == 0 && days == 0 {
			return 0, 0, 0, fmt.Errorf("zero duration %q", s)
		}
		if years < 0 || months < 0 || days < 0 {
			return 0, 0, 0, fmt.Errorf("negative duration %q", s)
		}
		return years, months, days, nil
	}

	n, unit, ok := parseNumericUnit(s)
	if !ok || n <= 0 {
		return 0, 0, 0, fmt.Errorf("unparseable duration %q", s)
	}
	switch unit {
	case "year", "yr":
		return n, 0, 0, nil
	case "month", "mo":
		return 0, n, 0, nil
	case "day", "d":
		return 0, 0, n, nil
	}
	return 0, 0, 0, fmt.Errorf("unparseable duration %q", s)
}

var isoDurationRE = regexp.MustCompile(`^p(?:(\d+)y)?(?:(\d+)m)?(?:(\d+)d)?$`)

func parseISODuration(s string) (years, months, days int, ok bool) {
	if !strings.HasPrefix(s, "p") {
		return 0, 0, 0, false
	}
	m := isoDurationRE.FindStringSubmatch(s)
	if m == nil || s == "p" {
		return 0, 0, 0, false
	}
	years = atoiDefault(m[1])
	months = atoiDefault(m[2])
	days = atoiDefault(m[3])
	return years, months, days, true
}

var numericUnitRE = regexp.MustCompile(`^(\d+)\s*(year|yr|month|mo|day|d)s?$`)

func parseNumericUnit(s string) (n int, unit string, ok bool) {
	m := numericUnitRE.FindStringSubmatch(s)
	if m == nil {
		return 0, "", false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, "", false
	}
	return n, m[2], true
}

func atoiDefault(s string) int {
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}
