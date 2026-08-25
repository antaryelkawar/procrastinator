// Package filestorage implements data.FileStorage as a local-filesystem
// storage backed by content-type sniffing and UUID-named files.
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

	data "procrastinator-backend/commons-data"
)

// ErrUnsupportedType is returned when the content type is not PDF, PNG, or JPEG.
var ErrUnsupportedType = errors.New("unsupported file type")

// Storage implements data.FileStorage.
type Storage struct {
	dir string
}

var _ data.FileStorage = (*Storage)(nil)

// New creates a Storage that writes files under dir.
func New(dir string) *Storage {
	return &Storage{dir: dir}
}

// Put stores the given bytes on disk under a UUID-named file and returns the Source record.
func (s *Storage) Put(ctx context.Context, payload []byte) (data.Source, error) {
	contentType := sniffType(payload)
	if contentType == "" {
		return data.Source{}, fmt.Errorf("%w: %q", ErrUnsupportedType, payload[:min(10, len(payload))])
	}

	uuid := generateUUID()
	ext := extForType(contentType)

	path := filepath.Join(s.dir, uuid+ext)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return data.Source{}, fmt.Errorf("write file: %w", err)
	}

	sum := sha256.Sum256(payload)

	return data.Source{
		ID:          uuid,
		Filename:    uuid,
		ContentType: contentType,
		Size:        int64(len(payload)),
		Path:        path,
		SHA256:      hex.EncodeToString(sum[:]),
		UploadedAt:  time.Now(),
	}, nil
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
