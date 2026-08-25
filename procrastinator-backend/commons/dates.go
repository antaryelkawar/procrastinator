package commons

import (
	"fmt"
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
