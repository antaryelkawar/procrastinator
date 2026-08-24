package ingest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var (
	pdfFixture   = []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n%%EOF")
	pngFixture   = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0, 0x0D, 'I', 'H', 'D', 'R', 0}
	jpegFixture  = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0, 1, 0x2A}
	textFixture  = []byte("hello world, just plain text\n")
	xmlFixture   = []byte("<?xml version=\"1.0\"?><doc></doc>")
	gifFixture   = []byte("GIF89a" + "\x01\x00\x01\x00\x00\x00\x00")
	emptyFixture = []byte{}
)

var uuidExtRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.(pdf|png|jpg)$`)

func TestSniffContentType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		data    []byte
		wantCT  string
		wantErr bool
	}{
		{
			name:   "PDF magic bytes",
			data:   pdfFixture,
			wantCT: "application/pdf",
		},
		{
			name:   "PNG magic bytes",
			data:   pngFixture,
			wantCT: "image/png",
		},
		{
			name:   "JPEG magic bytes",
			data:   jpegFixture,
			wantCT: "image/jpeg",
		},
		{
			name:    "plain text rejected",
			data:    textFixture,
			wantErr: true,
		},
		{
			name:    "XML rejected",
			data:    xmlFixture,
			wantErr: true,
		},
		{
			name:    "GIF rejected",
			data:    gifFixture,
			wantErr: true,
		},
		{
			name:    "empty rejected",
			data:    emptyFixture,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ct, err := SniffContentType(tc.data)

			if tc.wantErr {
				if !errors.Is(err, ErrUnsupportedType) {
					t.Fatalf("expected ErrUnsupportedType, got %v", err)
				}
				if ct != "" {
					t.Fatalf("expected empty content type on error, got %q", ct)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ct != tc.wantCT {
				t.Fatalf("content type = %q, want %q", ct, tc.wantCT)
			}
		})
	}
}

func TestPutStoresBytes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		data    []byte
		wantCT  string
		wantExt string
	}{
		{
			name:    "PDF stored",
			data:    pdfFixture,
			wantCT:  "application/pdf",
			wantExt: ".pdf",
		},
		{
			name:    "PNG stored",
			data:    pngFixture,
			wantCT:  "image/png",
			wantExt: ".png",
		},
		{
			name:    "JPEG stored",
			data:    jpegFixture,
			wantCT:  "image/jpeg",
			wantExt: ".jpg",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			st := NewStorage(dir)

			sf, err := st.Put(tc.data)
			if err != nil {
				t.Fatalf("Put returned unexpected error: %v", err)
			}

			// Content type
			if sf.ContentType != tc.wantCT {
				t.Errorf("ContentType = %q, want %q", sf.ContentType, tc.wantCT)
			}

			// Path is under dir
			rel, err := filepath.Rel(dir, sf.Path)
			if err != nil {
				t.Fatalf("filepath.Rel: %v", err)
			}
			if rel == "" || rel == sf.Path {
				t.Fatalf("Path %q is not under dir %q", sf.Path, dir)
			}
			if len(rel) >= 2 && rel[:2] == ".." {
				t.Fatalf("Path %q escapes storage dir %q (rel = %q)", sf.Path, dir, rel)
			}

			// Filename matches uuid.ext pattern
			base := filepath.Base(sf.Path)
			if !uuidExtRe.MatchString(base) {
				t.Errorf("filename %q does not match expected pattern", base)
			}

			// File exists and contains correct bytes
			got, err := os.ReadFile(sf.Path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if !bytes.Equal(got, tc.data) {
				t.Errorf("file contents differ from input")
			}

			// SHA256
			hash := sha256.Sum256(tc.data)
			wantHash := hex.EncodeToString(hash[:])
			if sf.SHA256 != wantHash {
				t.Errorf("SHA256 = %q, want %q", sf.SHA256, wantHash)
			}

			// ByteSize
			if sf.ByteSize != int64(len(tc.data)) {
				t.Errorf("ByteSize = %d, want %d", sf.ByteSize, len(tc.data))
			}
		})
	}
}

func TestPutRejectsUnsupportedType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data []byte
	}{
		{name: "plain text", data: textFixture},
		{name: "XML", data: xmlFixture},
		{name: "GIF", data: gifFixture},
		{name: "empty", data: emptyFixture},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			st := NewStorage(dir)

			_, err := st.Put(tc.data)
			if !errors.Is(err, ErrUnsupportedType) {
				t.Fatalf("expected ErrUnsupportedType, got %v", err)
			}

			// Storage dir must be empty
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("ReadDir: %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("expected storage dir to be empty, got %d entries", len(entries))
			}
		})
	}
}

func TestPutUniquePaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	st := NewStorage(dir)

	sf1, err := st.Put(pdfFixture)
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}

	sf2, err := st.Put(pdfFixture)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}

	if sf1.Path == sf2.Path {
		t.Fatalf("expected different paths, both are %q", sf1.Path)
	}

	// Both files exist with correct bytes
	for i, sf := range []StoredFile{sf1, sf2} {
		got, err := os.ReadFile(sf.Path)
		if err != nil {
			t.Fatalf("ReadFile #%d: %v", i+1, err)
		}
		if !bytes.Equal(got, pdfFixture) {
			t.Fatalf("file #%d contents differ from input", i+1)
		}
	}
}
