package parse

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
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
	Classification   any `json:"classification"`
	Brand            any `json:"brand"`
	Model            any `json:"model"`
	SerialNumber     any `json:"serial_number"`
	PurchaseDate     any `json:"purchase_date"`
	WarrantyEnd      any `json:"warranty_end"`
	Price            any `json:"price"`
	Currency         any `json:"currency"`
	Metadata         any `json:"metadata"`
	Confidence       any `json:"confidence"`
	Name             any `json:"name"`
	WarrantyDuration any `json:"warranty_duration"`
	AssetCategory    any `json:"asset_category"`
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
func ParseExtraction(raw string) (entity.Extraction, error) {
	stripped := thoughtRE.ReplaceAllString(raw, "")

	idx := strings.Index(stripped, "{")
	if idx == -1 {
		return entity.Extraction{}, errors.New("no JSON object found in payload")
	}

	payload := stripped[idx:]

	// Find the end of the first top-level JSON object by counting braces
	// (skipping strings). This lets us detect trailing data that
	// json.Decoder's internal buffer would swallow.
	end, ok := findObjectEnd(payload)
	if !ok {
		return entity.Extraction{}, errors.New("malformed JSON: unterminated object")
	}

	dec := json.NewDecoder(strings.NewReader(payload[:end]))
	dec.UseNumber()

	var r rawExtraction
	if err := dec.Decode(&r); err != nil {
		return entity.Extraction{}, fmt.Errorf("decode extraction payload: %w", err)
	}

	if rest := strings.TrimSpace(payload[end:]); rest != "" {
		// Provider regression observed live on 2026-09-11: the model
		// intermittently wraps the single extraction object in a JSON array
		// (`[ { ... } ]`) even though the request asks for a JSON object. The
		// brace counter above stops at the object's own `}`, so the array's
		// closing `]` surfaces as "trailing data" and BOTH consensus workers
		// fail together ("all extraction workers failed"). Accept that wrap
		// only when the object is preceded by `[` and followed by exactly `]`
		// with nothing else in between — empty arrays never reach here (no
		// `{`) and multi-element arrays leave `,{...}]` as the remainder, so
		// they still error below.
		if !isSingleElementArrayWrap(stripped[:idx], rest) {
			return entity.Extraction{}, fmt.Errorf("trailing data after JSON object: %q", rest)
		}
	}

	ext := entity.Extraction{
		RawPayload: raw,
	}

	if s, ok := r.Classification.(string); ok && entity.ValidDocType(s) {
		ext.Classification = s
	} else {
		ext.Classification = entity.DocTypeOther
	}

	ext.Brand = strPtr(r.Brand)
	ext.Model = strPtr(r.Model)
	ext.SerialNumber = strPtr(r.SerialNumber)
	ext.PurchaseDate = datePtr(r.PurchaseDate)
	ext.WarrantyEnd = datePtr(r.WarrantyEnd)
	ext.Price = pricePtr(r.Price)
	ext.Currency = currencyPtr(r.Currency)
	ext.Confidence = confidencePtr(r.Confidence)
	ext.Name = strPtr(r.Name)
	ext.WarrantyDuration = warrantyDurationStr(r.WarrantyDuration)
	ext.AssetCategory = assetCategoryPtr(r.AssetCategory)

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
		return entity.Extraction{}, fmt.Errorf("encode metadata: %w", err)
	} else if len(encoded) > metadataMaxSize {
		return entity.Extraction{}, fmt.Errorf("metadata exceeds size limit: %d bytes", len(encoded))
	}

	return ext, nil
}

// isSingleElementArrayWrap reports whether the located JSON object is wrapped
// in a JSON array with exactly one element. objectPrefix is everything before
// the object's opening brace (after thought stripping); trailing is the
// non-whitespace remainder after the object's closing brace. The wrap is
// accepted only for `[ <object> ]` — nothing else may appear inside the
// brackets.
func isSingleElementArrayWrap(objectPrefix, trailing string) bool {
	return strings.TrimSpace(objectPrefix) == "[" && strings.TrimSpace(trailing) == "]"
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
	t, err := commons.ParseDate(s)
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

// confidencePtr parses v as a float64 in the range [0.0, 1.0]. Returns nil
// for absent, non-numeric, or out-of-range values (never an error).
func confidencePtr(v any) *float64 {
	var f float64
	switch n := v.(type) {
	case json.Number:
		val, err := n.Float64()
		if err != nil {
			return nil
		}
		f = val
	case float64:
		f = n
	case int:
		f = float64(n)
	case int64:
		f = float64(n)
	default:
		return nil
	}
	if f < 0.0 || f > 1.0 {
		return nil
	}
	return &f
}

// warrantyDurationStr returns the string value if v is a non-empty string,
// else empty string (absent).
func warrantyDurationStr(v any) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return ""
}

// assetCategoryPtr returns a pointer to s only if s is a non-empty string in
// the valid asset category vocabulary, else nil (absent, never an error).
func assetCategoryPtr(v any) *string {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	if !entity.ValidAssetCategory(s) {
		return nil
	}
	return &s
}
