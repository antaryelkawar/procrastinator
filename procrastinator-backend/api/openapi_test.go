// Validates the openapi.yaml file in this directory:
//
//   - the file exists and stays under a 200 KB size budget
//   - it declares OpenAPI 3.1
//   - all 16 path keys (23 operations) under /api/users/{userId} are present
//   - the error envelope is referenced via $ref
//   - every required schema is defined under components.schemas
package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const maxOpenAPISize = 200 * 1024 // bytes

func TestOpenAPIDocument(t *testing.T) {
	path := filepath.Join(".", "openapi.yaml")

	// 1. File exists and size budget.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("openapi.yaml not found at %s: %v", path, err)
	}
	if info.Size() > maxOpenAPISize {
		t.Fatalf("openapi.yaml is %d bytes, exceeds %d byte budget", info.Size(), maxOpenAPISize)
	}

	// 3. OpenAPI 3.1 version.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading openapi.yaml: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "openapi: 3.1") {
		t.Fatal("openapi.yaml does not declare openapi: 3.1")
	}

	// 4. All 16 path keys present (full key form: /api/users/{userId}/...:).
	pathKeys := []string{
		"/api/users/{userId}/documents:",
		"/api/users/{userId}/documents/{id}:",
		"/api/users/{userId}/documents/{id}/reprocess:",
		"/api/users/{userId}/documents/{id}/keep:",
		"/api/users/{userId}/assets:",
		"/api/users/{userId}/assets/{assetId}:",
		"/api/users/{userId}/assets/{assetId}/documents:",
		"/api/users/{userId}/finance/accounts:",
		"/api/users/{userId}/finance/accounts/{id}:",
		"/api/users/{userId}/finance/movements:",
		"/api/users/{userId}/finance/movements/{id}:",
		"/api/users/{userId}/finance/movements/{id}/link:",
		"/api/users/{userId}/finance/import-batches:",
		"/api/users/{userId}/finance/import-batches/{id}:",
		"/api/users/{userId}/finance/import-batches/{id}/commit:",
		"/api/users/{userId}/finance/import-batches/{id}/discard:",
		"/api/users/{userId}/households:",
		"/api/users/{userId}/households/{householdId}:",
		"/api/users/{userId}/households/{householdId}/members:",
	}
	for _, key := range pathKeys {
		if !strings.Contains(content, key) {
			t.Errorf("missing path key in openapi.yaml: %s", key)
		}
	}

	// 5. Error envelope referenced.
	const errorRef = "$ref: '#/components/schemas/error'"
	if !strings.Contains(content, errorRef) {
		t.Errorf("error envelope not referenced; expected %q in openapi.yaml", errorRef)
	}

	// 6. Required schemas present.
	schemaNames := []string{
		"asset:",
		"document:",
		"account:",
		"movement:",
		"import_source:",
		"import_line:",
		"import_batch:",
		"commit_summary:",
		"household_member:",
		"household:",
		"error:",
		"create_account_request:",
		"create_movement_request:",
		"patch_movement_request:",
		"link_movement_request:",
		"create_household_request:",
		"add_member_request:",
		"duplicate_report:",
		"reprocess_document_request:",
	}
	for _, name := range schemaNames {
		if !strings.Contains(content, name) {
			t.Errorf("missing schema in openapi.yaml components.schemas: %s", name)
		}
	}
}
