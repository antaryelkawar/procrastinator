// Drift check for the Go OpenAPI generator (design D5).
//
// TestCodegenDrift re-runs the pinned oapi-codegen generator into a scratch
// location and compares the freshly-generated output against the committed
// artifact (openapi.gen.go). It proves that:
//
//   - generation is deterministic (two runs are byte-for-byte identical),
//   - the committed artifact is in sync with the source document (a fresh
//     clone builds from committed artifacts with the drift check reporting
//     in-sync),
//   - the check has teeth (a stale generated file is detected), and
//   - re-running codegen clears drift (an edited document + re-run is stable).
//
// The comparison normalizes CRLF->LF on both sides because git is configured
// with core.autocrlf=true, so a fresh Windows clone may check out the committed
// artifact with CRLF while the generator emits LF.
package gen

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// committedGenPath is the committed generated artifact, relative to the test
// package directory (procrastinator-backend/api/gen).
const committedGenPath = "openapi.gen.go"

// staleFileMessage names the stale file so a developer knows exactly what to
// regenerate. It must match the drift-check contract (spec: stale → fail
// naming the file).
const staleFileMessage = "STALE generated file: procrastinator-backend/api/gen/openapi.gen.go — re-run 'make codegen'"

// TestCodegenDrift verifies the Go generator drift check (tasks 6.2/6.3/6.4/6.5).
func TestCodegenDrift(t *testing.T) {
	// Resolve the real document with an absolute path so the generator
	// invocation is unambiguous regardless of the process working directory.
	// The source document lives one level up (procrastinator-backend/api/).
	docPath, err := filepath.Abs("../openapi.yaml")
	if err != nil {
		t.Fatalf("resolving absolute path of openapi.yaml: %v", err)
	}
	if _, err := os.Stat(docPath); err != nil {
		t.Fatalf("openapi.yaml not found at %s: %v", docPath, err)
	}

	committed, err := os.ReadFile(committedGenPath)
	if err != nil {
		t.Fatalf("reading committed %s: %v", committedGenPath, err)
	}

	t.Run("deterministic", func(t *testing.T) {
		out1 := filepath.Join(t.TempDir(), "gen1.go")
		out2 := filepath.Join(t.TempDir(), "gen2.go")
		driftRunOapiCodegen(t, docPath, out1)
		driftRunOapiCodegen(t, docPath, out2)

		g1, err := os.ReadFile(out1)
		if err != nil {
			t.Fatalf("reading first generated output: %v", err)
		}
		g2, err := os.ReadFile(out2)
		if err != nil {
			t.Fatalf("reading second generated output: %v", err)
		}
		if !bytes.Equal(driftNormalizeEOL(g1), driftNormalizeEOL(g2)) {
			t.Fatalf("generator is not deterministic: two runs against %s differ (after EOL normalization)", docPath)
		}
	})

	t.Run("regen_matches_committed", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "regen.go")
		driftRunOapiCodegen(t, docPath, out)
		regen, err := os.ReadFile(out)
		if err != nil {
			t.Fatalf("reading regenerated output: %v", err)
		}
		if !driftGeneratedMatches(regen, committed) {
			t.Fatalf("%s", staleFileMessage)
		}
	})

	t.Run("stale_detected", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "fresh.go")
		driftRunOapiCodegen(t, docPath, out)
		fresh, err := os.ReadFile(out)
		if err != nil {
			t.Fatalf("reading fresh regenerated output: %v", err)
		}

		// A stale committed artifact: append a sentinel marker line so it
		// differs from the fresh regen.
		stale := append(append([]byte{}, committed...), []byte("\n// __drift_sentinel__\n")...)

		if driftGeneratedMatches(fresh, stale) {
			t.Fatalf("drift check did not detect a stale generated file: comparison vacuously passed despite a sentinel marker difference")
		}
	})

	t.Run("edit_creates_drift_rerun_clears", func(t *testing.T) {
		docBytes, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("reading openapi.yaml for modification: %v", err)
		}
		modified := string(docBytes)
		componentsIdx := strings.Index(modified, "\ncomponents:")
		if componentsIdx < 0 {
			t.Fatal("openapi.yaml missing top-level components: section")
		}
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
		modified = modified[:componentsIdx] + newOp + modified[componentsIdx:]

		tmpDoc := filepath.Join(t.TempDir(), "modified.yaml")
		if err := os.WriteFile(tmpDoc, []byte(modified), 0o644); err != nil {
			t.Fatalf("writing modified document: %v", err)
		}

		// (a) Editing the document makes the committed artifact stale.
		scratchMod := filepath.Join(t.TempDir(), "mod1.go")
		driftRunOapiCodegen(t, tmpDoc, scratchMod)
		mod1, err := os.ReadFile(scratchMod)
		if err != nil {
			t.Fatalf("reading first modified-doc output: %v", err)
		}
		if driftGeneratedMatches(mod1, committed) {
			t.Fatalf("expected drift after editing the document, but the modified-doc regen matched the committed artifact (drift not detected)")
		}

		// (b) Re-running codegen is stable: a second regen of the same
		// modified document matches the first, so after `make codegen` +
		// commit the drift check would report in-sync.
		scratchMod2 := filepath.Join(t.TempDir(), "mod2.go")
		driftRunOapiCodegen(t, tmpDoc, scratchMod2)
		mod2, err := os.ReadFile(scratchMod2)
		if err != nil {
			t.Fatalf("reading second modified-doc output: %v", err)
		}
		if !driftGeneratedMatches(mod1, mod2) {
			t.Fatalf("re-running codegen against the same modified document is not stable: scratchMod2 does not match scratchMod (drift would not be cleared by a re-run)")
		}
	})
}

// driftRunOapiCodegen runs the pinned oapi-codegen generator against docPath,
// writing to outPath. It uses absolute paths for both the input doc and the
// output scratch file. On error it fails the test with the combined command
// output.
func driftRunOapiCodegen(t *testing.T, docPath, outPath string) {
	t.Helper()
	cmd := exec.Command("go", "run",
		"github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0",
		"-package", "gen",
		"-generate", "types,chi-server",
		"-o", outPath,
		docPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("oapi-codegen failed (doc=%s, out=%s): %v\n%s", docPath, outPath, err, out)
	}
}

// driftNormalizeEOL replaces CRLF with LF so that generated (LF) output and
// committed (possibly CRLF under core.autocrlf=true) artifacts compare
// identically.
func driftNormalizeEOL(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

// driftGeneratedMatches reports whether two generated artifacts are equal
// after EOL normalization.
func driftGeneratedMatches(regen, committed []byte) bool {
	return bytes.Equal(driftNormalizeEOL(regen), driftNormalizeEOL(committed))
}
