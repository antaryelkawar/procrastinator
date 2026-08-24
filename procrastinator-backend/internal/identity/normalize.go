// Package identity normalizes extracted identity values and resolves them to assets.
package identity

import "strings"

// collapseWhitespace trims leading/trailing whitespace and collapses
// internal whitespace runs to a single space. Returns "" for empty or
// whitespace-only input.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// NormalizeSerial trims leading/trailing whitespace, collapses internal
// whitespace runs to a single space, and uppercases. Returns "" for
// empty or whitespace-only input.
func NormalizeSerial(s string) string {
	return strings.ToUpper(collapseWhitespace(s))
}

// NormalizeName trims leading/trailing whitespace, collapses internal
// whitespace runs to a single space, and lowercases (brand/model case
// folding). Returns "" for empty or whitespace-only input.
func NormalizeName(s string) string {
	return strings.ToLower(collapseWhitespace(s))
}
