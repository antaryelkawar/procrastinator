package commons

import (
	"regexp"
	"strings"
	"unicode"
)

// amountRE matches strictly positive, exact decimal strings: one or more
// digits, optionally followed by a single dot and one or more more digits.
// It rejects signs, exponents, empty strings, and malformed dot placement.
var amountRE = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// IsValidAmount reports whether s is a strictly positive exact decimal amount.
//
// The input must match `^[0-9]+(\.[0-9]+)?$` (no sign, no exponent, at most
// one dot) and its value must be strictly greater than zero. Because the
// regex already excludes any sign, "not greater than zero" reduces to every
// digit being zero, which is rejected explicitly. No floating point is used:
// the value is compared purely by its digit characters.
func IsValidAmount(s string) bool {
	if !amountRE.MatchString(s) {
		return false
	}
	for _, r := range s {
		if r != '0' && r != '.' {
			return true
		}
	}
	return false
}

// IsValidCurrency reports whether s is a valid currency code: exactly 3
// characters long and every character a letter (ISO-4217 style).
func IsValidCurrency(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// NormalizeDescription returns the canonical form of a movement description:
// whitespace collapsed to single spaces and the result lowercased.
func NormalizeDescription(s string) string {
	return strings.ToLower(CollapseWhitespace(s))
}
