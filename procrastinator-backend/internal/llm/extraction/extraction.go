// Package extraction provides parsing and validation of LLM extraction
// results for invoice and warranty documents.
package extraction

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// thoughtRe matches <thought>...</thought> blocks (case-sensitive, lowercase
// tags only, non-greedy, possibly spanning multiple lines).
var thoughtRe = regexp.MustCompile(`(?s)<thought>.*?</thought>`)

// priceRe validates a price as a non-negative decimal string with an optional
// fractional part.
var priceRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// currencyRe validates a currency code as exactly three ASCII letters.
var currencyRe = regexp.MustCompile(`^[A-Za-z]{3}$`)

// ErrUnparseable indicates that the raw LLM response could not be parsed as
// a valid extraction payload. All errors returned by ParseExtraction wrap
// this sentinel.
var ErrUnparseable = errors.New("extraction: unparseable payload")

// Extraction holds the validated fields extracted from an LLM response.
// Zero-value fields indicate that the corresponding value was absent or
// failed validation in the raw payload.
type Extraction struct {
	// Classification is the document type: "invoice", "warranty", or "other".
	// "other" is the degraded default for missing, null, empty, or
	// unrecognized values.
	Classification string
	// Brand is the product brand. Empty if absent or null.
	Brand string
	// Model is the product model. Empty if absent or null.
	Model string
	// SerialNumber is the product serial number. Empty if absent or null.
	SerialNumber string
	// PurchaseDate is the parsed purchase date. Zero if absent or
	// unparseable.
	PurchaseDate time.Time
	// Price is the exact decimal price string. Empty if absent or invalid.
	Price string
	// Currency is the uppercased ISO 4217 currency code. Empty if absent
	// or invalid.
	Currency string
	// WarrantyStart is the parsed warranty start date. Zero if absent or
	// unparseable.
	WarrantyStart time.Time
	// WarrantyEnd is the parsed warranty end date. Zero if absent or
	// unparseable.
	WarrantyEnd time.Time
	// RawPayload is the original, unmodified LLM response text, including
	// any <thought> blocks that were stripped before parsing.
	RawPayload string
}

// wire is the JSON-decoded shape of the LLM extraction response.
type wire struct {
	DocumentType  *string `json:"document_type"`
	Brand         *string `json:"brand"`
	Model         *string `json:"model"`
	SerialNumber  *string `json:"serial_number"`
	PurchaseDate  *string `json:"purchase_date"`
	Price         any     `json:"price"`
	Currency      *string `json:"currency"`
	WarrantyStart *string `json:"warranty_start"`
	WarrantyEnd   *string `json:"warranty_end"`
}

// ParseExtraction parses and validates an LLM extraction response.
//
// The raw input is first stripped of all <thought>...</thought> blocks
// (case-sensitive, lowercase tags only) before JSON parsing. The original
// raw input, unmodified, is preserved as Extraction.RawPayload.
//
// After stripping, the result must begin with '{' (after trimming
// whitespace); otherwise ErrUnparseable is returned. The JSON is decoded
// with UseNumber so that numeric literals are kept exact, and the stream
// is verified to be exhausted (no trailing data).
//
// Field validation:
//   - Classification: "invoice", "warranty", or "other" kept verbatim;
//     anything else (including missing, null, empty) degrades to "other".
//   - Brand/Model/SerialNumber: nil -> ""; otherwise the string verbatim.
//   - PurchaseDate/WarrantyStart/WarrantyEnd: parsed via time.DateOnly then
//     time.RFC3339; failure -> zero time.
//   - Price: JSON string or number whose literal matches
//     ^[0-9]+(\.[0-9]+)?$; otherwise "".
//   - Currency: must match ^[A-Za-z]{3}$; uppercased; otherwise "".
//
// On any error, a zero Extraction and an error wrapping ErrUnparseable are
// returned.
func ParseExtraction(raw []byte) (Extraction, error) {
	stripped := stripThoughts(raw)
	trimmed := bytes.TrimSpace(stripped)

	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Extraction{}, fmt.Errorf("extraction: payload does not start with '{': %w", ErrUnparseable)
	}

	dec := json.NewDecoder(bytes.NewReader(stripped))
	dec.UseNumber()

	var w wire
	if err := dec.Decode(&w); err != nil {
		return Extraction{}, fmt.Errorf("extraction: %w: %v", ErrUnparseable, err)
	}

	// Ensure the stream is exhausted; any trailing data is an error.
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return Extraction{}, fmt.Errorf("extraction: trailing data: %w", ErrUnparseable)
	}

	var ex Extraction
	ex.RawPayload = string(raw)

	// Classification: keep invoice/warranty/other verbatim, else degrade.
	switch {
	case w.DocumentType != nil && (*w.DocumentType == "invoice" ||
		*w.DocumentType == "warranty" || *w.DocumentType == "other"):
		ex.Classification = *w.DocumentType
	default:
		ex.Classification = "other"
	}

	// String-pointer fields: nil -> "", otherwise verbatim.
	if w.Brand != nil {
		ex.Brand = *w.Brand
	}
	if w.Model != nil {
		ex.Model = *w.Model
	}
	if w.SerialNumber != nil {
		ex.SerialNumber = *w.SerialNumber
	}

	// Dates: nil/empty -> zero; otherwise DateOnly then RFC3339.
	ex.PurchaseDate = parseDate(w.PurchaseDate)
	ex.WarrantyStart = parseDate(w.WarrantyStart)
	ex.WarrantyEnd = parseDate(w.WarrantyEnd)

	// Price: string or number literal must match ^[0-9]+(\.[0-9]+)?$.
	ex.Price = parsePrice(w.Price)

	// Currency: exactly 3 ASCII letters -> uppercased.
	if w.Currency != nil {
		s := *w.Currency
		if currencyRe.MatchString(s) {
			ex.Currency = strings.ToUpper(s)
		}
	}

	return ex, nil
}

// stripThoughts removes all <thought>...</thought> blocks from raw
// (case-sensitive, lowercase tags only, non-greedy). Surrounding bytes
// are preserved verbatim. An unclosed <thought> tag is left as-is.
func stripThoughts(raw []byte) []byte {
	return thoughtRe.ReplaceAll(raw, nil)
}

// parseDate parses a date string pointer, trying time.DateOnly then
// time.RFC3339. Returns zero time on nil, empty string, or parse failure.
func parseDate(s *string) time.Time {
	if s == nil || *s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.DateOnly, *s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, *s); err == nil {
		return t
	}
	return time.Time{}
}

// parsePrice extracts the price literal from a JSON value. Accepts a JSON
// string or json.Number whose literal matches ^[0-9]+(\.[0-9]+)?$; returns
// "" for nil or any other type or format.
func parsePrice(v any) string {
	if v == nil {
		return ""
	}
	var lit string
	switch val := v.(type) {
	case string:
		lit = val
	case json.Number:
		lit = val.String()
	default:
		return ""
	}
	if priceRe.MatchString(lit) {
		return lit
	}
	return ""
}
