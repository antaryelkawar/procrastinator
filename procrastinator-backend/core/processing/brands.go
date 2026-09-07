// Package processing provides pure product-description parsing: brand
// extraction from a lexicon and splitting of raw descriptions into
// brand, name, and model components, plus multi-worker LLM extraction
// driven by a Chatter port (adapter in infra/llm) and consensus over the
// resulting per-worker extractions.
package processing

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"unicode"
)

// BrandLexicon holds a set of canonical brand names and supports
// case-insensitive, word-boundary, longest-match lookup.
type BrandLexicon struct {
	brands  []string
	byLower map[string]int
}

// defaultBrands is the embedded canonical brand list.
var defaultBrands = []string{
	"Cooler Master", "LG", "Samsung", "Whirlpool", "IFB", "Bosch",
	"Sony", "HP", "Dell", "Lenovo", "Apple", "Canon", "Nikon",
	"Toshiba", "Haier", "Voltas", "Videocon", "Hitachi", "Panasonic",
	"Sharp", "Daikin", "Blue Star", "Crompton", "Bajaj", "V-Guard",
	"Philips", "JBL", "Boat", "OnePlus", "Xiaomi", "Nokia", "Motorola",
}

// NewBrandLexicon returns a lexicon pre-loaded with the embedded canonical
// brand list.
func NewBrandLexicon() *BrandLexicon {
	return buildLexicon(defaultBrands)
}

// LoadBrandLexicon reads a JSON file at path containing an array of brand
// name strings and returns a new BrandLexicon built from that list.
// It returns an error if the file cannot be read or the JSON is invalid.
func LoadBrandLexicon(path string) (*BrandLexicon, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var brands []string
	if err := json.Unmarshal(data, &brands); err != nil {
		return nil, err
	}
	return buildLexicon(brands), nil
}

// NewBrandLexiconFromList builds a BrandLexicon from an explicit brand list.
// It is the composition-root hook for PROCRASTINATOR_BRAND_LEXICON (a JSON path
// parsed by the config package into a []string).
func NewBrandLexiconFromList(brands []string) *BrandLexicon {
	return buildLexicon(brands)
}

// SetDefaultBrandLexicon overrides the brand lexicon consulted by Consensus to
// validate/correct extracted brands. It is a composition-root hook: the config
// package parses PROCRASTINATOR_BRAND_LEXICON and the composition root applies
// the override before the server starts serving. The embedded canonical list
// (NewBrandLexicon) is the default.
func SetDefaultBrandLexicon(lex *BrandLexicon) {
	defaultConsensusLexicon = lex
}

// buildLexicon constructs a BrandLexicon from the given brand list,
// precomputing the lowercase lookup map.
func buildLexicon(brands []string) *BrandLexicon {
	lex := &BrandLexicon{
		brands:  make([]string, len(brands)),
		byLower: make(map[string]int, len(brands)),
	}
	for i, b := range brands {
		lex.brands[i] = b
		lex.byLower[strings.ToLower(b)] = i
	}
	return lex
}

// FindBrand performs a case-insensitive, word-boundary, longest-match search
// for any brand in the lexicon within text. It returns the canonical brand
// name and true if found, or "" and false otherwise.
//
// Word-boundary means the brand must not be embedded within a longer word:
// "LG" matches in "LG FHP10" but NOT in "FLAG" or "ALGOL".
//
// Longest-match means that if both "Cooler Master" and "Master" were in the
// lexicon, "Cooler Master" would be preferred when both match.
func (l *BrandLexicon) FindBrand(text string) (string, bool) {
	lowerText := strings.ToLower(text)
	if lowerText == "" {
		return "", false
	}

	// Build an index slice sorted by brand length descending for longest-match.
	indices := make([]int, len(l.brands))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(a, b int) bool {
		la := len(l.brands[indices[a]])
		lb := len(l.brands[indices[b]])
		if la != lb {
			return la > lb
		}
		// Tie-break by lexicon index for determinism.
		return indices[a] < indices[b]
	})

	for _, idx := range indices {
		lowerBrand := strings.ToLower(l.brands[idx])
		if brandAtWordBoundary(lowerText, lowerBrand) {
			return l.brands[idx], true
		}
	}
	return "", false
}

// brandAtWordBoundary reports whether lowerBrand appears in lowerText at a
// word boundary: the character immediately before the match (if any) must be
// a non-letter, and the character immediately after the match (if any) must
// be a non-letter.
func brandAtWordBoundary(lowerText, lowerBrand string) bool {
	if len(lowerBrand) == 0 {
		return false
	}
	start := 0
	for {
		pos := strings.Index(lowerText[start:], lowerBrand)
		if pos < 0 {
			return false
		}
		pos += start
		end := pos + len(lowerBrand)

		// Check left boundary: start of string or non-letter.
		if pos > 0 && isLetter(lowerText[pos-1]) {
			start = pos + 1
			continue
		}
		// Check right boundary: end of string or non-letter.
		if end < len(lowerText) && isLetter(lowerText[end]) {
			start = pos + 1
			continue
		}
		return true
	}
}

// isLetter reports whether c is a Unicode letter.
func isLetter(c byte) bool {
	// For ASCII text (the common case in product descriptions), check
	// alphabetic range directly. For non-ASCII bytes (UTF-8 continuation or
	// high bytes), treat as a letter to avoid false boundary matches.
	if c < 0x80 {
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	// Non-ASCII byte: decode as rune to determine if it's a letter.
	r := rune(c)
	return unicode.IsLetter(r)
}
