package filestorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"procrastinator-backend/commons/entity"
	"procrastinator-backend/commons/repo"
	"procrastinator-backend/commons/user"
)

// TextStorage implements repo.FileStorage for arbitrary bytes stored as a
// text/plain source. Unlike Storage, it performs no content-type sniffing:
// every payload is accepted and stored as-is.
type TextStorage struct {
	dir string
}

var _ repo.FileStorage = (*TextStorage)(nil)

// NewText creates a TextStorage that writes files under dir/<userID>/ for
// each user present in the context.
func NewText(dir string) *TextStorage {
	return &TextStorage{dir: dir}
}

// Put stores the given bytes on disk under the user-scoped key
// <userID>/<uuid>.txt and returns the Source record with ContentType
// "text/plain". The user ID must be present in ctx (see user.WithUser);
// otherwise Put fails closed with user.ErrNoUser before any I/O.
// Source.Path is the user-relative key, not an absolute path.
func (s *TextStorage) Put(ctx context.Context, payload []byte) (entity.Source, error) {
	tid, err := user.UserFrom(ctx)
	if err != nil {
		return entity.Source{}, err
	}

	uuid := generateUUID()

	key := tid + "/" + uuid + ".txt"
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
		ContentType: "text/plain",
		Size:        int64(len(payload)),
		Path:        key,
		SHA256:      hex.EncodeToString(sum[:]),
		UploadedAt:  time.Now(),
	}, nil
}

// Get returns the stored bytes for the user-relative key (Source.Path), read
// from dir/<key>.
func (s *TextStorage) Get(_ context.Context, key string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, key))
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return data, nil
}
