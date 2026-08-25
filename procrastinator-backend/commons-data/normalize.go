package data

import (
	"strings"
)

// CollapseWhitespace trims leading/trailing whitespace and collapses internal
// whitespace runs to a single space. Returns empty string for empty or whitespace-only input.
func CollapseWhitespace(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// NormalizeSerial normalizes a serial number by collapsing whitespace and converting to uppercase.
func NormalizeSerial(s string) string {
	return strings.ToUpper(CollapseWhitespace(s))
}

// NormalizeName normalizes a name by collapsing whitespace and converting to lowercase.
func NormalizeName(s string) string {
	return strings.ToLower(CollapseWhitespace(s))
}
