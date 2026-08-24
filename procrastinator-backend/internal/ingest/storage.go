package ingest

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// ErrUnsupportedType is returned when the content type of the provided data is not supported.
var ErrUnsupportedType = errors.New("unsupported content type")

// StoredFile contains metadata about a file stored in the ingestion storage.
type StoredFile struct {
	Path        string // absolute path of the written file: <storage-dir>/<uuid><ext>
	SHA256      string // lowercase hex SHA-256 digest of the bytes
	ContentType string // sniffed: "application/pdf" | "image/png" | "image/jpeg"
	ByteSize    int64  // len(data)
}

// Storage handles the persistence of ingested files to the filesystem.
type Storage struct {
	dir string
}

// NewStorage creates a new Storage instance. It does not create the storage directory on disk.
func NewStorage(dir string) *Storage {
	return &Storage{dir: dir}
}

// SniffContentType detects the content type of the provided data based on magic bytes.
// It supports application/pdf, image/png, and image/jpeg.
func SniffContentType(data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("ingest: %w: empty data", ErrUnsupportedType)
	}

	var ct string
	// Explicit check for PDF magic bytes %PDF-
	if len(data) >= 5 && string(data[:5]) == "%PDF-" {
		ct = "application/pdf"
	} else {
		ct = http.DetectContentType(data)
	}

	switch ct {
	case "application/pdf", "image/png", "image/jpeg":
		return ct, nil
	default:
		return "", fmt.Errorf("ingest: %w: %s", ErrUnsupportedType, ct)
	}
}

// Put stores the provided data in the storage directory with a unique filename.
// It returns a StoredFile containing the metadata of the stored file.
func (s *Storage) Put(data []byte) (StoredFile, error) {
	ct, err := SniffContentType(data)
	if err != nil {
		return StoredFile{}, err
	}

	uuid, err := generateUUID()
	if err != nil {
		return StoredFile{}, fmt.Errorf("ingest: failed to generate uuid: %w", err)
	}

	var ext string
	switch ct {
	case "application/pdf":
		ext = ".pdf"
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	}

	filename := uuid + ext
	path := filepath.Join(s.dir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return StoredFile{}, fmt.Errorf("ingest: failed to write file %s: %w", path, err)
	}

	hash := sha256.Sum256(data)

	return StoredFile{
		Path:        path,
		SHA256:      hex.EncodeToString(hash[:]),
		ContentType: ct,
		ByteSize:    int64(len(data)),
	}, nil
}

func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// Set version to 4
	b[6] = (b[6] & 0x0f) | 0x40
	// Set variant to RFC 4122
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
