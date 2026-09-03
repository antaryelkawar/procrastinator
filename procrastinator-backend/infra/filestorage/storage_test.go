package filestorage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"procrastinator-backend/commons/user"
	"procrastinator-backend/infra/filestorage"
)

// userCtx returns a context carrying the given user ID.
func userCtx(t *testing.T, id string) context.Context {
	t.Helper()
	return user.WithUser(context.Background(), id)
}

// countFiles walks root and returns the number of regular files found.
func countFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%q) error = %v, want nil", root, err)
	}
	return n
}

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
			src, err := s.Put(userCtx(t, "acme"), tt.data)
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

			// Path must be user-scoped: acme/<uuid><ext>.
			wantPrefix := "acme/"
			if len(src.Path) < len(wantPrefix) || src.Path[:len(wantPrefix)] != wantPrefix {
				t.Errorf("Path = %q, want prefix %q", src.Path, wantPrefix)
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
			_, err := s.Put(userCtx(t, "acme"), tt.data)
			if !errors.Is(err, filestorage.ErrUnsupportedType) {
				t.Fatalf("Put() error = %v, want errors.Is(err, ErrUnsupportedType)", err)
			}
		})
	}
}

// TestPutFileOnDisk verifies the file written at the user-scoped path
// (dir/<user>/... matching Source.Path) matches the input data byte-for-byte.
func TestPutFileOnDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := filestorage.New(dir)
	input := []byte("%PDF-1.4\nfile-on-disk content")

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

// TestPutSHA256 verifies Source.SHA256 is the hex-encoded SHA-256 of the input.
func TestPutSHA256(t *testing.T) {
	t.Parallel()
	s := filestorage.New(t.TempDir())
	input := []byte("%PDF-1.4\nsha content")

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

// TestPutPathFormat verifies Source.Path is <user>/<uuid><ext> where the uuid
// is a 36-character 8-4-4-4-12 hex string, and that the physical file exists
// at dir/<user>/<uuid><ext>.
func TestPutPathFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := filestorage.New(dir)
	input := []byte("%PDF-1.4\npath format content")

	src, err := s.Put(userCtx(t, "acme"), input)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	wantPath := "acme/" + src.ID
	re := regexp.MustCompile(`^acme/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(\.pdf|\.png|\.jpg)$`)
	m := re.FindStringSubmatch(src.Path)
	if m == nil {
		t.Fatalf("Path = %q, want <uuid><ext> with uuid in 8-4-4-4-12 hex form", src.Path)
	}
	if src.Path != wantPath+m[2] {
		t.Errorf("Path = %q, want %q", src.Path, wantPath+m[2])
	}
	if m[1] != src.ID {
		t.Errorf("Path uuid = %q, want ID %q", m[1], src.ID)
	}
	// The physical file must live under dir/<user>/.
	physical := filepath.Join(dir, "acme", src.ID+m[2])
	if _, err := os.Stat(physical); err != nil {
		t.Errorf("file at %q: %v, want it to exist", physical, err)
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

	src, err := s.Put(userCtx(t, "acme"), input)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}
	if src.ContentType != "application/pdf" {
		t.Errorf("ContentType = %q, want application/pdf", src.ContentType)
	}
}

// TestPutUserScopedKey verifies that for a context carrying user "acme",
// the file bytes land on disk at acme/<uuid><ext> and Source.Path is exactly
// that user-relative key.
func TestPutUserScopedKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := filestorage.New(dir)
	input := []byte("%PDF-1.4\nuser scoped content")

	src, err := s.Put(userCtx(t, "acme"), input)
	if err != nil {
		t.Fatalf("Put() error = %v, want nil", err)
	}

	key := "acme/" + src.ID + ".pdf"
	if src.Path != key {
		t.Errorf("Path = %q, want %q", src.Path, key)
	}

	onDisk, err := os.ReadFile(filepath.Join(dir, src.Path))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v, want nil", key, err)
	}
	if string(onDisk) != string(input) {
		t.Errorf("file on disk = %q, want %q", onDisk, input)
	}
}

// TestPutNoUserFailsClosed verifies that Put without a user in the context
// fails with user.ErrNoUser before sniffing or any I/O: no file is written
// to disk.
func TestPutNoUserFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "valid pdf payload", data: []byte("%PDF-1.4\nno user content")},
		// No user must win over the unsupported-type check: proves the
		// user check runs before sniffing.
		{name: "unsupported plain text payload", data: []byte("hello world")},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			s := filestorage.New(dir)

			_, err := s.Put(context.Background(), tt.data)
			if !errors.Is(err, user.ErrNoUser) {
				t.Fatalf("Put() error = %v, want errors.Is(err, user.ErrNoUser)", err)
			}

			if n := countFiles(t, dir); n != 0 {
				t.Errorf("files on disk = %d, want 0", n)
			}
		})
	}
}

// TestPutUserPrefixDisjoint verifies that uploads under different Users
// are strictly scoped: every key begins with its own user prefix and no file
// appears under the other user's directory.
func TestPutUserPrefixDisjoint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := filestorage.New(dir)

	for _, tc := range []struct {
		user string
		data []byte
	}{
		{user: "acme", data: []byte("%PDF-1.4\nacme payload")},
		{user: "acme", data: []byte("\x89PNG\r\n\x1a\n" + "acme png")},
		{user: "globex", data: []byte("%PDF-1.4\nglobex payload")},
		{user: "globex", data: []byte("\xFF\xD8\xFF\xDB" + "globex jpeg")},
	} {
		src, err := s.Put(userCtx(t, tc.user), tc.data)
		if err != nil {
			t.Fatalf("Put(user %q) error = %v, want nil", tc.user, err)
		}
		wantPrefix := tc.user + "/"
		if len(src.Path) < len(wantPrefix) || src.Path[:len(wantPrefix)] != wantPrefix {
			t.Errorf("Path = %q, want prefix %q", src.Path, wantPrefix)
		}

		physical := filepath.Join(dir, src.Path)
		if _, err := os.Stat(physical); err != nil {
			t.Errorf("file at %q: %v, want it to exist", physical, err)
		}
	}

	// Every file on disk must sit exactly one level below its own user
	// directory: dir/<user>/<file>, with nothing under the other user's
	// directory and nothing loose in the root.
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return rerr
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 2 || (parts[0] != "acme" && parts[0] != "globex") {
			t.Errorf("file %q is not user-scoped (expected acme/<file> or globex/<file>)", rel)
			return nil
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%q) error = %v, want nil", dir, err)
	}
}
