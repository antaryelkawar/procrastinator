package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewOperationYieldsScaffolding verifies that adding a new operation to
// openapi.yaml and re-running the generator produces handler scaffolding for
// it (a method in the generated ServerInterface) with no hand-written
// signature edit.
func TestNewOperationYieldsScaffolding(t *testing.T) {
	// Read the current openapi.yaml (one directory up from gen/).
	data, err := os.ReadFile("../openapi.yaml")
	if err != nil {
		t.Fatalf("reading openapi.yaml: %v", err)
	}

	// Build a modified document with a new operation appended.
	newOp := `
  /api/users/{userId}/ping:
    parameters:
      - name: userId
        in: path
        required: true
        schema:
          type: string
    get:
      operationId: ping
      summary: Health check
      responses:
        "200":
          description: OK
`

	modified := string(data)
	componentsIdx := strings.Index(modified, "\ncomponents:")
	if componentsIdx < 0 {
		t.Fatal("openapi.yaml missing components section")
	}
	modified = modified[:componentsIdx] + newOp + modified[componentsIdx:]

	// Write to a temp file.
	tmpDir := t.TempDir()
	tmpDoc := filepath.Join(tmpDir, "openapi.yaml")
	if err := os.WriteFile(tmpDoc, []byte(modified), 0o644); err != nil {
		t.Fatalf("writing temp openapi.yaml: %v", err)
	}
	tmpOut := filepath.Join(tmpDir, "gen.go")

	// Run the generator from the module root.
	cmd := exec.Command("go", "run",
		"github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0",
		"-package", "gen",
		"-generate", "types,chi-server",
		"-o", tmpOut,
		tmpDoc,
	)
	cmd.Dir = ".." // run from procrastinator-backend/
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("oapi-codegen failed: %v\n%s", err, out)
	}

	// Verify the generated output contains the new operation's method.
	generated, err := os.ReadFile(tmpOut)
	if err != nil {
		t.Fatalf("reading generated output: %v", err)
	}
	genContent := string(generated)

	if !strings.Contains(genContent, "Ping(w http.ResponseWriter, r *http.Request, userId string)") {
		t.Error("generated ServerInterface does not contain Ping method")
	}
	if !strings.Contains(genContent, "func (_ Unimplemented) Ping(") {
		t.Error("generated Unimplemented does not contain Ping stub")
	}
	if !strings.Contains(genContent, `"/api/users/{userId}/ping"`) {
		t.Error("generated router does not register /api/users/{userId}/ping route")
	}
}
