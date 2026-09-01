package filestorage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"procrastinator-backend/infra/filestorage"
)

// TestStatementStoreCSV verifies Store succeeds for a CSV payload and returns
// a Source with the original filename, content type, hash, and size.
func TestStatementStoreCSV(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := filestorage.NewStatement(dir)
	originalName := "statement-2026-06.csv"
	input := []byte("Date,Description,Amount,Balance\n" +
		"2026-06-01,Salary,5000.00,12345.67\n" +
		"2026-06-03,Rent,-1200.00,11145.67\n" +
		"2026-06-05,Groceries,-87.42,11058.25\n")

	src, err := s.Store(context.Background(), originalName, input)
	if err != nil {
		t.Fatalf("Store() error = %v, want nil", err)
	}

	if src.Filename != originalName {
		t.Errorf("Filename = %q, want %q", src.Filename, originalName)
	}
	if src.ContentType != "text/csv" {
		t.Errorf("ContentType = %q, want text/csv", src.ContentType)
	}
	if src.Size != int64(len(input)) {
		t.Errorf("Size = %d, want %d", src.Size, len(input))
	}
	if src.ID == "" {
		t.Error("ID is empty, want non-empty")
	}

	sum := sha256.Sum256(input)
	want := hex.EncodeToString(sum[:])
	if src.SHA256 != want {
		t.Errorf("SHA256 = %q, want %q", src.SHA256, want)
	}

	// The file must be written byte-for-byte at Source.Path.
	if src.Path == "" {
		t.Fatal("Path is empty, want non-empty")
	}
	onDisk, err := os.ReadFile(src.Path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v, want nil", src.Path, err)
	}
	if string(onDisk) != string(input) {
		t.Errorf("file on disk = %q, want %q", onDisk, input)
	}
}

// TestStatementStorePDF verifies Store succeeds for a PDF payload and returns
// a Source with the original filename and application/pdf content type.
func TestStatementStorePDF(t *testing.T) {
	t.Parallel()
	s := filestorage.NewStatement(t.TempDir())
	originalName := "statement-2026-06.pdf"
	input := []byte("%PDF-1.7\n% binary-ish stream content")

	src, err := s.Store(context.Background(), originalName, input)
	if err != nil {
		t.Fatalf("Store() error = %v, want nil", err)
	}

	if src.ContentType != "application/pdf" {
		t.Errorf("ContentType = %q, want application/pdf", src.ContentType)
	}
	if src.Filename != originalName {
		t.Errorf("Filename = %q, want %q", src.Filename, originalName)
	}
}

// TestStatementStoreUnsupportedType verifies Store returns
// ErrStatementUnsupportedType for content that is neither PDF nor text/CSV and
// writes nothing to disk.
func TestStatementStoreUnsupportedType(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "png", data: []byte("\x89PNG\r\n\x1a\n" + "png payload")},
		{name: "jpeg", data: []byte("\xFF\xD8\xFF\xDB" + "jpeg payload")},
		{name: "empty bytes", data: []byte{}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			s := filestorage.NewStatement(dir)
			_, err := s.Store(context.Background(), "statement.png", tt.data)
			if !errors.Is(err, filestorage.ErrStatementUnsupportedType) {
				t.Fatalf("Store() error = %v, want errors.Is(err, ErrStatementUnsupportedType)", err)
			}

			entries, rerr := os.ReadDir(dir) // sniff must run before any disk write
			if rerr != nil {
				t.Fatalf("ReadDir(%q) error = %v, want nil", dir, rerr)
			}
			if len(entries) != 0 {
				t.Errorf("storage dir has %d entries after failed Store, want 0", len(entries))
			}
		})
	}
}

// TestStatementStorePlainText verifies a plain-text payload is treated as CSV,
// normalizing Go sniffer output "text/csv; charset=utf-8" to "text/csv".
func TestStatementStorePlainText(t *testing.T) {
	t.Parallel()
	s := filestorage.NewStatement(t.TempDir())
	originalName := "plain.txt"
	input := []byte("just some plain text without a csv header")

	src, err := s.Store(context.Background(), originalName, input)
	if err != nil {
		t.Fatalf("Store() error = %v, want nil", err)
	}
	if src.ContentType != "text/csv" {
		t.Errorf("ContentType = %q, want text/csv", src.ContentType)
	}
	if src.Filename != originalName {
		t.Errorf("Filename = %q, want %q", src.Filename, originalName)
	}
}
