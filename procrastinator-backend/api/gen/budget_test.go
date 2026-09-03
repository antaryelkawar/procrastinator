// Performance and size budgets for the OpenAPI codegen pipeline (tasks 7.1/7.2/7.3).
//
// These tests assert that the codegen pipeline stays within its time and size
// budgets. They run the EXACT pinned generator invocations recorded in the
// top-level Makefile (oapi-codegen v2.8.0, openapi-typescript 7.13.0,
// redoc-cli 0.13.21) into a scratch directory, so the measured work is the
// same work the build gate performs — just redirected into a temp dir so the
// committed artifacts are never touched.
//
// A shared timing harness (budgetRunGenerator) is used by both the
// full-codegen test (7.1) and the drift-check test (7.2) so both are measured
// with identical instrumentation. Each test warms up the Go tool once (discarded,
// not counted) so the budget measures steady-state generation rather than the
// one-time tool download/compile.
package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Budget constants (spec: "Codegen performance and size budgets").
const (
	// budgetFullCodegen is the time budget for a full codegen run
	// (Go + TS + docs generators).
	budgetFullCodegen = 30 * time.Second

	// budgetDriftCheck is the time budget for the drift-check step
	// (re-run all generators + diff against committed artifacts).
	budgetDriftCheck = 30 * time.Second

	// budgetMaxDocSize is the maximum allowed size of the committed
	// openapi.yaml document, in bytes.
	budgetMaxDocSize = 200 * 1024
)

// budgetResolvePaths resolves the absolute paths used by the budget tests. All
// paths are made absolute so the generator invocations are unambiguous
// regardless of the process working directory (the gen package dir is
// procrastinator-backend/api/gen).
func budgetResolvePaths(t *testing.T) (doc, uiRoot, tsCLI, redocCLI, backendDir string) {
	t.Helper()
	abs := func(p string) string {
		a, err := filepath.Abs(p)
		if err != nil {
			t.Fatalf("resolving absolute path of %s: %v", p, err)
		}
		return a
	}
	doc = abs("../openapi.yaml")
	uiRoot = abs("../../../ui")
	backendDir = abs("../..") // procrastinator-backend (module root)
	tsCLI = filepath.Join(uiRoot, "node_modules", "openapi-typescript", "bin", "cli.js")
	redocCLI = filepath.Join(uiRoot, "node_modules", "redoc-cli", "index.js")
	for _, p := range []string{doc, tsCLI, redocCLI} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("required codegen input not found at %s: %v", p, err)
		}
	}
	return
}

// budgetRunGenerator is the shared timing harness: it runs one pinned generator
// command via os/exec with cmd.Dir set to dir, measures elapsed time with
// time.Now()/time.Since(), logs the duration via t.Logf, fails the test with
// the combined command output on a non-zero exit, and returns the elapsed time.
func budgetRunGenerator(t *testing.T, dir, name string, argv ...string) time.Duration {
	t.Helper()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	t.Logf("%s: elapsed %s (dir=%s, cmd=%q)", name, elapsed, dir, argv)
	if err != nil {
		t.Fatalf("%s generator failed (elapsed %s): %v\n%s", name, elapsed, err, out)
	}
	return elapsed
}

// budgetRunAllGenerators re-runs all three pinned generators (Go, TS, docs) into
// the scratch dir and returns the elapsed time for each generator.
func budgetRunAllGenerators(t *testing.T, doc, uiRoot, backendDir, scratch string) (goDur, tsDur, docsDur time.Duration) {
	t.Helper()
	goOut := filepath.Join(scratch, "openapi.gen.go")
	docsOut := filepath.Join(scratch, "index.html")

	goDur = budgetRunGenerator(t, backendDir, "go",
		"go", "run",
		"github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0",
		"-package", "gen",
		"-generate", "types,chi-server",
		"-o", goOut,
		doc,
	)

	tsOutPath := filepath.Join(scratch, "paths.d.ts")
	tsDur = budgetRunGenerator(t, uiRoot, "ts",
		"node",
		filepath.Join(uiRoot, "node_modules", "openapi-typescript", "bin", "cli.js"),
		doc,
		"-o", tsOutPath,
	)

	docsDur = budgetRunGenerator(t, uiRoot, "docs",
		"node",
		filepath.Join(uiRoot, "node_modules", "redoc-cli", "index.js"),
		"build", doc,
		"--output", docsOut,
	)
	return
}

// budgetWarmupGo runs the Go generator once into a throwaway temp file and
// discards the output. This populates the Go build cache so the timed run
// measures steady-state generation, not the one-time tool download/compile.
// This warm-up time is intentionally NOT counted toward the budget: the
// one-time tool bootstrap (downloading + compiling the pinned oapi-codegen
// module) is excluded so the budget reflects generation cost on a warm cache,
// which is what the build gate sees after the first run.
func budgetWarmupGo(t *testing.T, doc, backendDir string) {
	t.Helper()
	warmupOut := filepath.Join(t.TempDir(), "warmup.go")
	_ = budgetRunGenerator(t, backendDir, "warmup-go",
		"go", "run",
		"github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0",
		"-package", "gen",
		"-generate", "types,chi-server",
		"-o", warmupOut,
		doc,
	)
}

// TestFullCodegenWithinTimeBudget asserts a full codegen run (Go + TS + docs)
// completes under the full-codegen time budget (task 7.1).
func TestFullCodegenWithinTimeBudget(t *testing.T) {
	doc, uiRoot, _, _, backendDir := budgetResolvePaths(t)

	// WARM-UP: run the Go generator once into a throwaway temp file and discard
	// it, to populate the Go build cache. This excludes the one-time tool
	// download/compile from the budget so the timed run measures steady-state
	// generation. This warm-up time is NOT counted toward the budget.
	budgetWarmupGo(t, doc, backendDir)

	scratch := t.TempDir()
	start := time.Now()
	goDur, tsDur, docsDur := budgetRunAllGenerators(t, doc, uiRoot, backendDir, scratch)
	elapsed := time.Since(start)

	t.Logf("full codegen: go=%s ts=%s docs=%s total=%s (budget %s)", goDur, tsDur, docsDur, elapsed, budgetFullCodegen)
	if elapsed >= budgetFullCodegen {
		t.Fatalf("full codegen exceeded time budget: elapsed %s >= budget %s (go=%s ts=%s docs=%s)", elapsed, budgetFullCodegen, goDur, tsDur, docsDur)
	}
}

// TestDriftCheckWithinTimeBudget asserts the drift-check step (re-run all
// generators + diff against committed artifacts) adds no more than the
// drift-check time budget (task 7.2).
func TestDriftCheckWithinTimeBudget(t *testing.T) {
	doc, uiRoot, _, _, backendDir := budgetResolvePaths(t)

	// WARM-UP as in TestFullCodegenWithinTimeBudget: not counted toward budget.
	budgetWarmupGo(t, doc, backendDir)

	scratch := t.TempDir()
	start := time.Now()
	goDur, tsDur, docsDur := budgetRunAllGenerators(t, doc, uiRoot, backendDir, scratch)

	// Diff each freshly-generated scratch output against its committed artifact
	// using the existing EOL-normalized driftGeneratedMatches helper. A
	// mismatch is a drift failure — reported via t.Fatalf because a drift-check
	// step that reports drift still consumed the budget.
	committed, err := os.ReadFile(committedGenPath)
	if err != nil {
		t.Fatalf("reading committed %s: %v", committedGenPath, err)
	}
	regenGo, err := os.ReadFile(filepath.Join(scratch, "openapi.gen.go"))
	if err != nil {
		t.Fatalf("reading regenerated Go output: %v", err)
	}
	regenTS, err := os.ReadFile(filepath.Join(scratch, "paths.d.ts"))
	if err != nil {
		t.Fatalf("reading regenerated TS output: %v", err)
	}
	committedTS, err := os.ReadFile(filepath.Join(uiRoot, "src", "lib", "api", "generated", "paths.d.ts"))
	if err != nil {
		t.Fatalf("reading committed TS artifact: %v", err)
	}
	regenDocs, err := os.ReadFile(filepath.Join(scratch, "index.html"))
	if err != nil {
		t.Fatalf("reading regenerated docs output: %v", err)
	}
	committedDocs, err := os.ReadFile("../docs/index.html")
	if err != nil {
		t.Fatalf("reading committed docs artifact: %v", err)
	}

	if !driftGeneratedMatches(regenGo, committed) {
		t.Fatalf("drift check reported drift for Go artifact: regenerated %s does not match committed openapi.gen.go (re-run 'make codegen')", committedGenPath)
	}
	if !driftGeneratedMatches(regenTS, committedTS) {
		t.Fatalf("drift check reported drift for TS artifact: regenerated paths.d.ts does not match committed %s", filepath.Join(uiRoot, "src", "lib", "api", "generated", "paths.d.ts"))
	}
	if !driftGeneratedMatches(regenDocs, committedDocs) {
		t.Fatalf("drift check reported drift for docs artifact: regenerated index.html does not match committed ../docs/index.html")
	}

	elapsed := time.Since(start) // covers re-run + diff
	t.Logf("drift check: go=%s ts=%s docs=%s total=%s (budget %s)", goDur, tsDur, docsDur, elapsed, budgetDriftCheck)
	if elapsed >= budgetDriftCheck {
		t.Fatalf("drift check exceeded time budget: elapsed %s >= budget %s (go=%s ts=%s docs=%s)", elapsed, budgetDriftCheck, goDur, tsDur, docsDur)
	}
}

// TestOpenAPIDocumentWithinSizeBudget re-confirms the committed openapi.yaml is
// under the size budget (task 7.3).
func TestOpenAPIDocumentWithinSizeBudget(t *testing.T) {
	doc, _, _, _, _ := budgetResolvePaths(t)
	info, err := os.Stat(doc)
	if err != nil {
		t.Fatalf("stat openapi.yaml: %v", err)
	}
	size := info.Size()
	t.Logf("openapi.yaml size: %d bytes (budget %d bytes)", size, budgetMaxDocSize)
	if size >= budgetMaxDocSize {
		t.Fatalf("openapi.yaml exceeds size budget: %d bytes >= budget %d bytes", size, budgetMaxDocSize)
	}
}
