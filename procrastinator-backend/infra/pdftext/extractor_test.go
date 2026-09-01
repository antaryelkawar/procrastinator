package pdftext_test

import (
	"fmt"
	"strings"
	"testing"

	"procrastinator-backend/infra/pdftext"
)

// pdfObject builds a single PDF indirect object "id 0 obj ... endobj".
func pdfObject(id int, body string) string {
	return fmt.Sprintf("%d 0 obj\n%s\nendobj\n", id, body)
}

// pdfBuild assembles a minimal, valid single-page PDF from its parts,
// computing all xref offsets programmatically so the cross-reference table
// is byte-accurate.
//
// Object layout:
//
//	1: catalog
//	2: pages (count=1, kids=[3])
//	3: page (media box, contents=4, font resource=5 if fonts present)
//	4: content stream
//	5: font dictionary (only when fonts != "")
func pdfBuild(content, fonts string) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	b.WriteString("%\xE2\xE3\xCF\xD3 binary marker\n")

	offsets := make([]int, 6)
	// Offset of each "N 0 obj" line.
	off := func(s *strings.Builder) int { return s.Len() }

	offsets[1] = off(&b)
	b.WriteString(pdfObject(1, "<< /Type /Catalog /Pages 2 0 R >>"))

	offsets[2] = off(&b)
	b.WriteString(pdfObject(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"))

	pageDict := "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]"
	if fonts != "" {
		pageDict += " /Resources << /Font << /F1 5 0 R >> >>"
	} else {
		pageDict += " /Resources << >>"
	}
	pageDict += " /Contents 4 0 R >>"
	offsets[3] = off(&b)
	b.WriteString(pdfObject(3, pageDict))

	offsets[4] = off(&b)
	b.WriteString(pdfObject(4, fmt.Sprintf(
		"<< /Length %d >>\nstream\n%s\nendstream",
		len(content), content)))

	if fonts != "" {
		offsets[5] = off(&b)
		b.WriteString(pdfObject(5, fonts))
	}

	xrefOff := b.Len()
	lastObj := 4
	if fonts != "" {
		lastObj = 5
	}
	b.WriteString(fmt.Sprintf("xref\n0 %d\n", lastObj+1))
	b.WriteString("0000000000 65535 f \n")
	for i := 1; i <= lastObj; i++ {
		b.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	b.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\n", lastObj+1))
	b.WriteString(fmt.Sprintf("startxref\n%d\n", xrefOff))
	b.WriteString("%%EOF\n")

	return []byte(b.String())
}

// TestExtractTextWithTextLayer verifies ExtractText returns the text drawn on
// the page (via BT/Tj/TJ operators) for a PDF that has an extractable text
// layer.
func TestExtractTextWithTextLayer(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "Tj operator",
			content: "BT /F1 12 Tf 72 720 Td (Hello World) Tj ET",
		},
		{
			name:    "TJ array operator",
			content: "BT /F1 12 Tf 72 720 Td [(Hello) -120 (World)] TJ ET",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			font := "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
			data := pdfBuild(tt.content, font)

			e := pdftext.Extractor{}
			got, err := e.ExtractText(data)
			if err != nil {
				t.Fatalf("ExtractText() error = %v, want nil", err)
			}
			if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
				t.Errorf("ExtractText() = %q, want it to contain %q and %q", got, "Hello", "World")
			}
		})
	}
}

// TestExtractTextWithoutTextLayer verifies ExtractText returns "" and a nil
// error for a structurally valid PDF whose content stream has no text
// operators (e.g. scanned/image-only page).
func TestExtractTextWithoutTextLayer(t *testing.T) {
	t.Parallel()
	// A non-text content operator (a drawn rectangle) keeps the page
	// structurally active while producing no extractable text.
	data := pdfBuild("0 0 100 100 re f", "")

	e := pdftext.Extractor{}
	got, err := e.ExtractText(data)
	if err != nil {
		t.Fatalf("ExtractText() error = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("ExtractText() = %q, want %q", got, "")
	}
}

// TestExtractTextGarbage verifies ExtractText returns an error for bytes that
// are not a valid PDF at all.
func TestExtractTextGarbage(t *testing.T) {
	t.Parallel()
	e := pdftext.Extractor{}
	_, err := e.ExtractText([]byte("this is definitely not a pdf document"))
	if err == nil {
		t.Fatal("ExtractText() error = nil, want non-nil for non-PDF bytes")
	}
}
