package filestorage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"procrastinator-backend/infra/filestorage"
)

// TestPutValidTypes verifies Put succeeds for each supported content type and
// returns a Source with all fields populated.
func TestPutValidTypes(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantType string
		wantExt  string
	}{
		{
			name:     "pdf",
			data:     []byte("%PDF-1.4\n" + "trailer stuff"),
			wantType: "application/pdf",
			wantExt:  ".pdf",
		},
		{
			name:     "png",
			data:     []byte("\x89PNG\r\n\x1a\n" + "png payload"),
			wantType: "image/png",
			wantExt:  ".png",
		},
		{
			name:     "jpeg",
			data:     []byte("\xFF\xD8\xFF\xDB" + "jpeg payload"),
			wantType: "image/jpeg",
			wantExt:  ".jpg",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := filestorage.New(t.TempDir())
			src, err := s.Put(context.Background(), tt.data)
			if err != nil {
				t.Fatalf("Put() error = %v, want nil", err)
			}

			if src.ContentType != tt.wantType {
				t.Errorf("ContentType = %q, want %q", src.ContentType, tt.wantType)
			}
			if src.Size != int64(len(tt.data)) {
				t.Errorf("Size = %d, want %d", src.Size, len(tt.data))
			}
			if src.ID == "" {
				t.Error("ID is empty, want non-empty")
			}
			if src.Path == "" {
				t.Error("Path is empty, want non-empty")
			}
			if src.SHA256 == "" {
				t.Error("SHA256 is empty, want non-empty")
			}
			if src.Filename == "" {
				t.Error("Filename is empty, want non-empty")
			}
			if src.UploadedAt.IsZero() {
				t.Error("UploadedAt is zero, want non-zero")
			}

			// Path must end with the expected extension.
			wantSuffix := tt.wantExt
			if len(src.Path) < len(wantSuffix) || src.Path[len(src.Path)-len(wantSuffix):] != wantSuffix {
				t.Errorf("Path = %q, want suffix %q", src.Path, wantSuffix)
			}
		})
	}
}

// TestPutUnsupportedType verifies Put returns ErrUnsupportedType for content
// that is not PDF, PNG, or JPEG.
func TestPutUnsupportedType(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "plain text", data: []byte("hello world")},
		{name: "empty bytes", data: []byte{}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := filestorage.New(t.TempDir())
			_, err := s.Put(context.Background(), tt.data)
			if !errors.Is(err, filestorage.ErrUnsupportedType) {
				t.Fatalf("Put() error = %v, want errors.Is(err, ErrUnsupportedType)", err)
			}
		})
	}
}

// TestPutFileOnDisk verifies the file written at Source.Path matches the input
// data byte-for-byte.
func TestPutFileOnDisk(t *testing.T) {
	t.Parallel()
	s := filestorage.New(t.TempDir())
	input := []byte("%PDF-1.4\nfile-on-disk content")

	src, err := s.Put(context.Background(), input)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	onDisk, err := os.ReadFile(src.Path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v, want nil", src.Path, err)
	}
	if string(onDisk) != string(input) {
		t.Errorf("file on disk = %q, want %q", onDisk, input)
	}
}

// TestPutSHA256 verifies Source.SHA256 is the hex-encoded SHA-256 of the input.
func TestPutSHA256(t *testing.T) {
	t.Parallel()
	s := filestorage.New(t.TempDir())
	input := []byte("%PDF-1.4\nsha content")

	src, err := s.Put(context.Background(), input)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	sum := sha256.Sum256(input)
	want := hex.EncodeToString(sum[:])
	if src.SHA256 != want {
		t.Errorf("SHA256 = %q, want %q", src.SHA256, want)
	}
}

// TestPutPathFormat verifies Source.Path is <dir>/<uuid><ext> where the uuid is
// a 36-character 8-4-4-4-12 hex string.
func TestPutPathFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := filestorage.New(dir)
	input := []byte("%PDF-1.4\npath format content")

	src, err := s.Put(context.Background(), input)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	name := filepath.Base(src.Path)
	re := regexp.MustCompile(`^([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(\.pdf|\.png|\.jpg)$`)
	m := re.FindStringSubmatch(name)
	if m == nil {
		t.Fatalf("Path = %q, want <dir>/<uuid><ext> with uuid in 8-4-4-4-12 hex form", src.Path)
	}
	if m[1] != src.ID {
		t.Errorf("Path uuid = %q, want ID %q", m[1], src.ID)
	}
	// The file must live directly under the storage dir.
	if filepath.Dir(src.Path) != dir {
		t.Errorf("Path = %q, want file directly under dir %q", src.Path, dir)
	}
}

// TestPutPDFMagicPriority verifies data starting with %PDF- is detected as
// application/pdf even when http.DetectContentType would return
// application/octet-stream.
func TestPutPDFMagicPriority(t *testing.T) {
	t.Parallel()
	s := filestorage.New(t.TempDir())
	// %PDF- magic but no PDF structure that net/http's sniffing recognizes.
	input := []byte("%PDF-XX")

	src, err := s.Put(context.Background(), input)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}
	if src.ContentType != "application/pdf" {
		t.Errorf("ContentType = %q, want application/pdf", src.ContentType)
	}
}
