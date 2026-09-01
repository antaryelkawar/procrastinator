// Package pdftext extracts the text layer from PDF documents.
package pdftext

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ErrNotAPDF is returned when the input bytes cannot be parsed as a PDF.
var ErrNotAPDF = errors.New("not a valid PDF")

// Extractor extracts the text layer from in-memory PDF data.
// A zero-value Extractor is ready to use.
type Extractor struct{}

// New creates an Extractor.
func New() *Extractor {
	return &Extractor{}
}

// ExtractText parses data as a PDF and returns the concatenated text layer of
// all pages, joined by newlines so multi-page text stays line-structured.
// A PDF that parses cleanly but carries no extractable text (e.g. an
// image-only/scanned document) yields "" with a nil error. Bytes that are not
// a valid PDF at all yield a non-nil error.
//
// The library reports one Text entry per drawn character; GetStyledTexts
// merges consecutive entries on the same line (same font, size, and Y) into a
// sentence, so each returned part is a text run that approximates one line.
func (e *Extractor) ExtractText(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotAPDF, err)
	}

	sentences, err := r.GetStyledTexts()
	if err != nil {
		return "", fmt.Errorf("extract pdf text: %w", err)
	}

	parts := make([]string, 0, len(sentences))
	for _, s := range sentences {
		if s.S != "" {
			parts = append(parts, s.S)
		}
	}

	return strings.Join(parts, "\n"), nil
}
