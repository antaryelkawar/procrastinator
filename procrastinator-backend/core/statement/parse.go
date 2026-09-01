// Package statement is the pure parsing layer of the statement-import
// capability. It turns uploaded bank or wallet statements (CSV or
// text-extracted PDF) into ParsedLine values: one extracted-field record per
// statement line. It has no database, HTTP, or infrastructure dependencies.
package statement

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"procrastinator-backend/commons"
	"procrastinator-backend/commons/entity"
)

// StatementFormat identifies the source format of an uploaded statement.
type StatementFormat string

// Statement format constants.
const (
	FormatCSV StatementFormat = "csv"
	FormatPDF StatementFormat = "pdf"
)

// RawLine is one raw statement line as re-serialized to a single line by
// ParseCSV (or extracted from PDF text).
type RawLine string

// Direction values for a parsed amount.
const (
	DirectionIn  = "in"
	DirectionOut = "out"
)

// ParsedLine is the extracted-field record for one statement line. Every
// outcome of ExtractFields is expressed through this struct: a valid line has
// all fields populated and Status entity.LineStatusValid, a failed line has
// Status entity.LineStatusError, an ErrorReason, and every field that WAS
// successfully extracted still populated (partial extraction).
type ParsedLine struct {
	LineRef           int    // 1-based statement line position
	RawLine           string // the raw content
	OccurredOn        *time.Time
	Amount            *string // exact decimal string, absolute value, never negative
	Direction         string  // "in" or "out"; empty when the amount was not parsed
	Description       *string
	NormDescription   *string
	ExternalReference *string
	Status            string  // one of the entity.LineStatus* constants
	ErrorReason       *string // only set when Status is entity.LineStatusError
}

// PDFTextExtractor extracts the text content of a PDF document. The concrete
// implementation lives in infra/pdftext; this consumer-side interface keeps
// the pure parsing layer free of infrastructure dependencies.
type PDFTextExtractor interface {
	ExtractText(data []byte) (string, error)
}

// ParseCSV parses a statement CSV and returns one raw string per CSV data row.
//
// Header-optional heuristic: if the first row's first field does NOT parse as
// a date via commons.ParseDate, the first row is treated as a header and
// skipped. If it does parse as a date, the file is headerless and every row is
// a data row.
//
// Each returned raw string is the CSV record re-serialized to a single line
// with an encoding/csv Writer (round-trip safe: a description containing a
// comma stays quoted and survives re-parsing).
//
// Empty or whitespace-only input returns zero lines and a nil error.
// Malformed CSV syntax returns an error wrapped with "parse statement csv".
func ParseCSV(data []byte) ([]string, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return []string{}, nil
	}

	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1 // statement rows may carry an optional 4th field

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse statement csv: %w", err)
	}

	start := 0
	if len(records) > 0 {
		if _, err := commons.ParseDate(records[0][0]); err != nil {
			start = 1 // first row is a header
		}
	}

	lines := make([]string, 0, len(records)-start)
	for _, rec := range records[start:] {
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		if err := w.Write(rec); err != nil {
			return nil, fmt.Errorf("parse statement csv: %w", err)
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return nil, fmt.Errorf("parse statement csv: %w", err)
		}
		// The Writer terminates the record with a line break; strip it so the
		// result is a single line.
		lines = append(lines, strings.TrimSuffix(buf.String(), "\n"))
	}
	return lines, nil
}

// ExtractFields extracts the fields from one raw statement line (the
// re-serialized single-line CSV format produced by ParseCSV:
// date,amount,description[,external_reference]).
//
// It never panics and never returns an error: every outcome is expressed
// through the returned ParsedLine. A line with a wrong field count (not
// exactly 3 or 4) or a malformed raw line becomes an error line with no other
// fields populated. A line with one or more field problems becomes an error
// line whose ErrorReason combines ALL problems found, while every field that
// was successfully extracted is still populated (partial extraction).
func ExtractFields(lineRef int, raw string) ParsedLine {
	problems := []string{}

	// Re-parse the raw line as a single CSV record.
	fields := parseRawLine(raw)

	line := ParsedLine{LineRef: lineRef, RawLine: raw}

	if fields == nil {
		reason := fmt.Sprintf("malformed line: %s", raw)
		line.Status = entity.LineStatusError
		line.ErrorReason = &reason
		return line
	}

	if len(fields) != 3 && len(fields) != 4 {
		reason := fmt.Sprintf("expected 3 or 4 fields, got %d", len(fields))
		line.Status = entity.LineStatusError
		line.ErrorReason = &reason
		return line
	}

	// Field 0: date.
	if d, err := commons.ParseDate(fields[0]); err == nil && !d.IsZero() {
		line.OccurredOn = &d
	} else {
		problems = append(problems, "missing or invalid date")
	}

	// Field 1: amount + direction.
	unsigned, direction, ok := parseAmount(fields[1])
	if ok {
		line.Amount = &unsigned
		line.Direction = direction
	} else {
		problems = append(problems, "missing or invalid amount")
	}

	// Field 2: description.
	desc := strings.TrimSpace(fields[2])
	if desc != "" {
		norm := commons.NormalizeDescription(desc)
		line.Description = &desc
		line.NormDescription = &norm
	} else {
		problems = append(problems, "missing description")
	}

	// Field 3: optional external reference.
	if len(fields) == 4 {
		ref := strings.TrimSpace(fields[3])
		if ref != "" {
			line.ExternalReference = &ref
		}
	}

	if len(problems) > 0 {
		reason := strings.Join(problems, "; ")
		line.Status = entity.LineStatusError
		line.ErrorReason = &reason
	} else {
		line.Status = entity.LineStatusValid
	}
	return line
}

// parseRawLine re-parses a raw line as a single CSV record. It returns nil if
// the raw line is not a single valid CSV record (malformed).
func parseRawLine(raw string) []string {
	r := csv.NewReader(strings.NewReader(raw))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil || len(records) != 1 {
		return nil
	}
	return records[0]
}

// parseAmount parses a signed or unsigned amount field. It returns the
// absolute unsigned value, the direction, and whether the amount is valid
// (a strictly positive exact decimal per commons.IsValidAmount).
func parseAmount(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	switch {
	case strings.HasPrefix(s, "-"):
		unsigned := s[1:]
		if commons.IsValidAmount(unsigned) {
			return unsigned, DirectionOut, true
		}
	case strings.HasPrefix(s, "+"):
		unsigned := s[1:]
		if commons.IsValidAmount(unsigned) {
			return unsigned, DirectionIn, true
		}
	default:
		if commons.IsValidAmount(s) {
			return s, DirectionIn, true
		}
	}
	return "", "", false
}
