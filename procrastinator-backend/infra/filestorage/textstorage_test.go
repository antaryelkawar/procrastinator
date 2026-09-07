package filestorage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"procrastinator-backend/commons/user"
	"procrastinator-backend/infra/filestorage"
)

// TestTextPut verifies TextStorage.Put succeeds for plain-text payloads and
// returns a Source with ContentType "text/plain" and all fields populated.
func TestTextPut(t *testing.T) {
	input := []byte("pasted free text content")

	s := filestorage.NewText(t.TempDir())
	src, err := s.Put(userCtx(t, "acme"), input)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	if src.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", src.ContentType)
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
	if src.Filename != src.ID {
		t.Errorf("Filename = %q, want ID %q", src.Filename, src.ID)
	}
	if src.Size != int64(len(input)) {
		t.Errorf("Size = %d, want %d", src.Size, len(input))
	}
	if src.UploadedAt.IsZero() {
		t.Error("UploadedAt is zero, want non-zero")
	}

	// Path must be user-scoped: acme/<uuid>.txt.
	wantPrefix := "acme/"
	if !strings.HasPrefix(src.Path, wantPrefix) {
		t.Errorf("Path = %q, want prefix %q", src.Path, wantPrefix)
	}
	if !strings.HasSuffix(src.Path, ".txt") {
		t.Errorf("Path = %q, want suffix .txt", src.Path)
	}
}

// TestTextPutSHA256 verifies TextStorage.Put returns the hex-encoded SHA-256
// of the input.
func TestTextPutSHA256(t *testing.T) {
	input := []byte("sha content for text")

	s := filestorage.NewText(t.TempDir())
	src, err := s.Put(userCtx(t, "acme"), input)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	sum := sha256.Sum256(input)
	want := hex.EncodeToString(sum[:])
	if src.SHA256 != want {
		t.Errorf("SHA256 = %q, want %q", src.SHA256, want)
	}
}

// TestTextPutFileOnDisk verifies the file written at the user-scoped path
// (dir/<user>/<uuid>.txt matching Source.Path) matches the input bytes.
func TestTextPutFileOnDisk(t *testing.T) {
	dir := t.TempDir()
	s := filestorage.NewText(dir)
	input := []byte("file-on-disk text content")

	src, err := s.Put(userCtx(t, "acme"), input)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	onDisk, err := os.ReadFile(filepath.Join(dir, src.Path))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v, want nil", src.Path, err)
	}
	if string(onDisk) != string(input) {
		t.Errorf("file on disk = %q, want %q", onDisk, input)
	}
}

// TestTextPutNoUserFailsClosed verifies TextStorage.Put without a user in the
// context fails with user.ErrNoUser before any I/O: no file is written.
func TestTextPutNoUserFailsClosed(t *testing.T) {
	dir := t.TempDir()
	s := filestorage.NewText(dir)
	input := []byte("no user text content")

	_, err := s.Put(context.Background(), input)
	if !errors.Is(err, user.ErrNoUser) {
		t.Fatalf("Put() error = %v, want errors.Is(err, user.ErrNoUser)", err)
	}

	if n := countFiles(t, dir); n != 0 {
		t.Errorf("files on disk = %d, want 0", n)
	}
}
