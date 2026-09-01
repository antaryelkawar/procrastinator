// Command genfixtures builds the two PDF fixtures for the api import tests
// (byte-accurate xref tables) and verifies them with the real pdftext
// extractor before writing them to api/testdata/.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func main() {
	const (
		textContent  = "BT /F1 12 Tf 72 720 Td (2026-08-20,-400.00,Text Layer Buy) Tj ET"
		fontDict     = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
		imageContent = "0 0 100 100 re f"
		wantText     = "2026-08-20,-400.00,Text Layer Buy"
	)

	textPDF := pdfBuild(textContent, fontDict)
	imagePDF := pdfBuild(imageContent, "")

	ext := pdftext.Extractor{}

	got, err := ext.ExtractText(textPDF)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAIL text layer extract:", err)
		os.Exit(1)
	}
	if !strings.Contains(got, wantText) {
		fmt.Fprintf(os.Stderr, "FAIL text layer: got %q, want it to contain %q\n", got, wantText)
		os.Exit(1)
	}
	fmt.Printf("OK statement_text.pdf: extracted %d bytes, contains %q\n", len(got), wantText)

	got, err = ext.ExtractText(imagePDF)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAIL image-only extract:", err)
		os.Exit(1)
	}
	if got != "" {
		fmt.Fprintf(os.Stderr, "FAIL image-only: got %q, want %q\n", got, "")
		os.Exit(1)
	}
	fmt.Println("OK statement_image_only.pdf: extracted empty string, nil error")

	outDir := filepath.Join("..", "..", "api", "testdata")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir:", err)
		os.Exit(1)
	}
	for name, data := range map[string][]byte{
		"statement_text.pdf":       textPDF,
		"statement_image_only.pdf": imagePDF,
	} {
		p := filepath.Join(outDir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", p, len(data))
	}
}
