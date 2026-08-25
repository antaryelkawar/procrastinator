package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// thoughtRE matches thought blocks (including across newlines) for stripping
// before JSON extraction. Compiled once at package level.
var thoughtRE = regexp.MustCompile(`(?s)<thought>.*?</thought>`)

// metadataKeyRE validates metadata keys as snake_case: a lowercase letter
// followed by at most 63 lowercase letters, digits, or underscores.
// Compiled once at package level.
var metadataKeyRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// metadataMaxSize is the maximum JSON-encoded size (in bytes) of the
// filtered metadata object.
const metadataMaxSize = 8192

// rawExtraction mirrors the LLM's JSON schema with untyped fields so that
// numbers are preserved via json.Number (UseNumber) before being validated
// into the Extraction struct.
type rawExtraction struct {
	Classification any `json:"classification"`
	Brand          any `json:"brand"`
	Model          any `json:"model"`
	SerialNumber   any `json:"serial_number"`
	PurchaseDate   any `json:"purchase_date"`
	WarrantyEnd    any `json:"warranty_end"`
	Price          any `json:"price"`
	Currency       any `json:"currency"`
	Metadata       any `json:"metadata"`
}

// ParseExtraction extracts a validated Extraction from an LLM response payload.
//
// The raw payload may contain thought blocks (<thought>...</thought>) before,
// after, or around the JSON object; these are stripped before locating the
// first JSON object. RawPayload preserves the original unmodified input.
//
// Metadata contract: keys are validated as snake_case matching
// ^[a-z][a-z0-9_]{0,63}$; entries with non-matching keys or null values are
// dropped. If the JSON-encoded filtered metadata exceeds 8192 bytes, an error
// is returned.
//
// Errors: no JSON object found, malformed JSON, trailing data after the JSON
// object, or oversized metadata.
func ParseExtraction(raw string) (Extraction, error) {
	stripped := thoughtRE.ReplaceAllString(raw, "")

	idx := strings.Index(stripped, "{")
	if idx == -1 {
		return Extraction{}, errors.New("no JSON object found in payload")
	}

	payload := stripped[idx:]

	// Find the end of the first top-level JSON object by counting braces
	// (skipping strings). This lets us detect trailing data that
	// json.Decoder's internal buffer would swallow.
	end, ok := findObjectEnd(payload)
	if !ok {
		return Extraction{}, errors.New("malformed JSON: unterminated object")
	}

	dec := json.NewDecoder(strings.NewReader(payload[:end]))
	dec.UseNumber()

	var r rawExtraction
	if err := dec.Decode(&r); err != nil {
		return Extraction{}, fmt.Errorf("decode extraction payload: %w", err)
	}

	if rest := strings.TrimSpace(payload[end:]); rest != "" {
		return Extraction{}, fmt.Errorf("trailing data after JSON object: %q", rest)
	}

	ext := Extraction{
		RawPayload: raw,
	}

	if s, ok := r.Classification.(string); ok && ValidDocType(s) {
		ext.Classification = s
	} else {
		ext.Classification = DocTypeOther
	}

	ext.Brand = strPtr(r.Brand)
	ext.Model = strPtr(r.Model)
	ext.SerialNumber = strPtr(r.SerialNumber)
	ext.PurchaseDate = datePtr(r.PurchaseDate)
	ext.WarrantyEnd = datePtr(r.WarrantyEnd)
	ext.Price = pricePtr(r.Price)
	ext.Currency = currencyPtr(r.Currency)

	if m, ok := r.Metadata.(map[string]any); ok {
		ext.Metadata = m
	} else {
		ext.Metadata = map[string]any{}
	}

	// Filter metadata: drop entries with non-snake_case keys or null values.
	filtered := make(map[string]any, len(ext.Metadata))
	for k, v := range ext.Metadata {
		if metadataKeyRE.MatchString(k) && v != nil {
			filtered[k] = v
		}
	}
	ext.Metadata = filtered

	if encoded, err := json.Marshal(ext.Metadata); err != nil {
		return Extraction{}, fmt.Errorf("encode metadata: %w", err)
	} else if len(encoded) > metadataMaxSize {
		return Extraction{}, fmt.Errorf("metadata exceeds size limit: %d bytes", len(encoded))
	}

	return ext, nil
}

// findObjectEnd returns the index just past the closing brace of the first
// top-level JSON object starting at s[0]. It tracks string literals and
// escape sequences to avoid counting braces inside strings. Returns (0, false)
// if the object is never terminated.
func findObjectEnd(s string) (int, bool) {
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

// strPtr returns a pointer to s if s is a non-empty string, else nil.
func strPtr(v any) *string {
	if s, ok := v.(string); ok && s != "" {
		return &s
	}
	return nil
}

// datePtr parses v as a date string. Returns nil for empty, non-string, or
// unparseable input.
func datePtr(v any) *time.Time {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	t, err := ParseDate(s)
	if err != nil {
		return nil
	}
	return &t
}

// pricePtr handles price as json.Number (exact decimal string), string, or nil.
func pricePtr(v any) *string {
	switch p := v.(type) {
	case json.Number:
		s := p.String()
		return &s
	case string:
		if p != "" {
			return &p
		}
	}
	return nil
}

// currencyPtr returns a pointer to s only if s is exactly three alphabetic
// characters, else nil.
func currencyPtr(v any) *string {
	s, ok := v.(string)
	if !ok || len(s) != 3 {
		return nil
	}
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return nil
		}
	}
	return &s
}
