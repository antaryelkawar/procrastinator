package filestorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"procrastinator-backend/commons/entity"
)

// ErrStatementUnsupportedType is returned when the statement content type is
// not application/pdf or text/csv.
var ErrStatementUnsupportedType = errors.New("unsupported statement type")

// StatementStorage stores bank statements (PDF or CSV) as local files.
type StatementStorage struct {
	dir string
}

// NewStatement creates a StatementStorage that writes files under dir.
func NewStatement(dir string) *StatementStorage {
	return &StatementStorage{dir: dir}
}

// Store sniffs the content type of data, rejects anything that is not a PDF or
// a text/CSV payload, and otherwise writes the bytes on disk under a
// UUID-named file, returning the Source record. Filename is the original name
// supplied by the caller.
func (s *StatementStorage) Store(ctx context.Context, originalName string, data []byte) (entity.Source, error) {
	contentType := sniffStatementType(data)
	if contentType == "" {
		return entity.Source{}, fmt.Errorf("%w: content is neither PDF nor CSV", ErrStatementUnsupportedType)
	}

	uuid := generateUUID()
	ext := extForStatementType(contentType)

	path := filepath.Join(s.dir, uuid+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return entity.Source{}, fmt.Errorf("write file: %w", err)
	}

	sum := sha256.Sum256(data)

	return entity.Source{
		ID:          uuid,
		Filename:    originalName,
		ContentType: contentType,
		Size:        int64(len(data)),
		Path:        path,
		SHA256:      hex.EncodeToString(sum[:]),
		UploadedAt:  time.Now(),
	}, nil
}

// sniffStatementType returns "application/pdf" or "text/csv" when the data
// matches a supported statement type, and "" otherwise. The %PDF- magic is
// checked first so PDFs are recognized even when http.DetectContentType would
// fall back to application/octet-stream. CSV is recognized as any text/* MIME
// type reported by http.DetectContentType.
//
// Go's sniffer appends "; charset=utf-8" to text/* types (e.g.
// "text/csv; charset=utf-8" for CSV-ish text), so the suffix is stripped to
// normalize the returned content type.
func sniffStatementType(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if len(data) >= 5 && string(data[:5]) == "%PDF-" {
		return "application/pdf"
	}
	ct := http.DetectContentType(data)
	if strings.HasPrefix(ct, "text/") {
		return "text/csv"
	}
	return ""
}

// extForStatementType maps a supported statement content type to its file
// extension.
func extForStatementType(contentType string) string {
	switch contentType {
	case "application/pdf":
		return ".pdf"
	case "text/csv":
		return ".csv"
	default:
		return ""
	}
}
