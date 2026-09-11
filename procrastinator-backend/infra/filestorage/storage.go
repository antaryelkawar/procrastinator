// Package filestorage implements repo.FileStorage as a local-filesystem
// storage backed by content-type sniffing and user-scoped, UUID-named
// files under {OwnerID}/{uuid}{ext}.
package filestorage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// ErrUnsupportedType is returned when the content type is not PDF, PNG, or JPEG.
var ErrUnsupportedType = errors.New("unsupported file type")

// Storage implements repo.FileStorage.
type Storage struct {
	dir string
}

var _ repo.FileStorage = (*Storage)(nil)

// New creates a Storage that writes files under dir/<userID>/ for each
// user present in the context.
func New(dir string) *Storage {
	return &Storage{dir: dir}
}

// Put stores the given bytes on disk under the user-scoped key
// <userID>/<uuid><ext> and returns the Source record. The user ID must be
// present in ctx (see user.WithUser); otherwise Put fails closed with
// user.ErrNoUser before sniffing or any I/O. Source.Path is the
// user-relative key, not an absolute path.
func (s *Storage) Put(ctx context.Context, payload []byte) (entity.Source, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Source{}, err
	}

	contentType := sniffType(payload)
	if contentType == "" {
		return entity.Source{}, fmt.Errorf("%w: %q", ErrUnsupportedType, payload[:min(10, len(payload))])
	}

	uuid := generateUUID()
	ext := extForType(contentType)

	key := tid + "/" + uuid + ext
	path := filepath.Join(s.dir, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return entity.Source{}, fmt.Errorf("create dir: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return entity.Source{}, fmt.Errorf("write file: %w", err)
	}

	sum := sha256.Sum256(payload)

	return entity.Source{
		ID:          uuid,
		Filename:    uuid,
		ContentType: contentType,
		Size:        int64(len(payload)),
		Path:        key,
		SHA256:      hex.EncodeToString(sum[:]),
		UploadedAt:  time.Now(),
	}, nil
}

// Get returns the stored bytes for the user-relative key (Source.Path),
// read from dir/<key>. The key is joined with the storage dir so a malformed
// key cannot escape the storage root.
func (s *Storage) Get(_ context.Context, key string) ([]byte, error) {
	path := filepath.Join(s.dir, key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return data, nil
}

// sniffType returns "application/pdf", "image/png", or "image/jpeg" when the
// data matches a supported type, and "" otherwise. The %PDF- magic is checked
// first so PDFs are recognized even when http.DetectContentType would fall
// back to application/octet-stream.
func sniffType(data []byte) string {
	if len(data) >= 5 && string(data[:5]) == "%PDF-" {
		return "application/pdf"
	}
	ct := http.DetectContentType(data)
	switch ct {
	case "application/pdf", "image/png", "image/jpeg":
		return ct
	default:
		return ""
	}
}

// generateUUID returns a random RFC 4122 version 4 UUID in 8-4-4-4-12 form.
func generateUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is unrecoverable; panic is the documented contract.
		panic(fmt.Sprintf("filestorage: read crypto/rand: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10

	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// extForType maps a supported content type to its file extension.
func extForType(contentType string) string {
	switch contentType {
	case "application/pdf":
		return ".pdf"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	default:
		return ""
	}
}
