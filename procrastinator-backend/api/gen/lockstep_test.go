// Lockstep verification for the Go OpenAPI generator (task 8.1 final check).
//
// TestRoutesMatchOperationsOneToOne asserts the spec scenario "Routes and
// operations match exactly": the set of (HTTP method, path-template) pairs
// registered by the generated chi router must be IDENTICAL to the set of
// operations declared in the source openapi.yaml document — no route without a
// corresponding operation, and no operation without a corresponding route.
//
// The test derives both sets from the files on disk rather than hardcoding
// them, so it stays meaningful if the document grows: any drift (a new
// operation added to the yaml but not regenerated, or a stray route) fails the
// lockstep check and lists the offending pairs in both directions.
//
// This is a pure source-text check: it reads openapi.yaml and openapi.gen.go
// and needs no database, so it runs without a TestMain.
package gen

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// lockstepDocPath is the source OpenAPI document, one directory up from the
// gen package (procrastinator-backend/api/openapi.yaml).
const lockstepDocPath = "../openapi.yaml"

// lockstepGenPath is the committed generated artifact in the same directory as
// this test file (procrastinator-backend/api/gen/openapi.gen.go).
const lockstepGenPath = "openapi.gen.go"

// lockstepMethodSet is the canonical set of HTTP method keys that denote an
// operation at the 4-space indentation level in the document.
var lockstepMethodSet = map[string]bool{
	"get":    true,
	"post":   true,
	"put":    true,
	"patch":  true,
	"delete": true,
}

// lockstepPathKeyRe matches a YAML path key: exactly two leading spaces, a key
// starting with /api, and a trailing colon (e.g. `  /api/users/{userId}/x:`).
// The trailing colon is required so path-level keys like `parameters:` (which
// carry no colon) are never mistaken for a path.
var lockstepPathKeyRe = regexp.MustCompile(`^  (/api[^\s]*):$`)

// lockstepOpKeyRe matches an operation key: exactly four leading spaces and a
// bare HTTP method followed by a colon (e.g. `    get:`).
var lockstepOpKeyRe = regexp.MustCompile(`^    (get|post|put|patch|delete):$`)

// lockstepRouteRe matches a chi route registration line in the generated file:
// a chi verb (Get/Post/Put/Patch/Delete) followed by options.BaseURL + a quoted
// path literal (e.g. `r.Get(options.BaseURL+"/api/users/{userId}/assets", ...`).
var lockstepRouteRe = regexp.MustCompile(`r\.(Get|Post|Put|Patch|Delete)\(options\.BaseURL\+"([^"]+)"`)

// lockstepServerInterfaceRe matches the opening of the generated ServerInterface
// block.
var lockstepServerInterfaceRe = regexp.MustCompile(`type ServerInterface interface \{`)

// lockstepServerMethodRe matches a leading-tab method signature inside the
// ServerInterface block, e.g. `\tListAssets(w http.ResponseWriter, ...)`.
var lockstepServerMethodRe = regexp.MustCompile(`^\t[A-Za-z]\w*\(`)

// TestRoutesMatchOperationsOneToOne asserts exact 1:1 lockstep between the
// routes registered by the generated chi router and the operations declared in
// the openapi.yaml document.
func TestRoutesMatchOperationsOneToOne(t *testing.T) {
	docAbs, err := filepath.Abs(lockstepDocPath)
	if err != nil {
		t.Fatalf("resolving absolute path of %s: %v", lockstepDocPath, err)
	}
	if _, err := os.Stat(docAbs); err != nil {
		t.Fatalf("%s not found at %s: %v", lockstepDocPath, docAbs, err)
	}

	docBytes, err := os.ReadFile(docAbs)
	if err != nil {
		t.Fatalf("reading %s: %v", docAbs, err)
	}
	genBytes, err := os.ReadFile(lockstepGenPath)
	if err != nil {
		t.Fatalf("reading %s: %v", lockstepGenPath, err)
	}

	docOps := lockstepDocOperations(string(docBytes))
	routerRoutes := lockstepRouterRoutes(string(genBytes))
	serverMethods := lockstepServerInterfaceMethodCount(string(genBytes))

	t.Logf("documented operations: %d", len(docOps))
	t.Logf("registered routes:     %d", len(routerRoutes))
	t.Logf("ServerInterface methods: %d", serverMethods)

	// Cardinality check first: the three counts must agree.
	if len(docOps) != len(routerRoutes) {
		t.Errorf("cardinality mismatch: %d documented operations but %d registered routes", len(docOps), len(routerRoutes))
	}
	if serverMethods != len(routerRoutes) {
		t.Errorf("ServerInterface has %d methods but the router registers %d routes", serverMethods, len(routerRoutes))
	}

	// Exact set equality: every documented operation has a route, and every
	// route is a documented operation.
	docOnly := lockstepSetDiff(docOps, routerRoutes)
	routeOnly := lockstepSetDiff(routerRoutes, docOps)

	if len(docOnly) > 0 {
		t.Errorf("documented but not routed (missing route): %s", strings.Join(docOnly, ", "))
	}
	if len(routeOnly) > 0 {
		t.Errorf("routed but not documented (extra route): %s", strings.Join(routeOnly, ", "))
	}

	if len(docOps) == 0 {
		t.Fatal("parsed zero documented operations from openapi.yaml; the parser likely broke — refusing to pass vacuously")
	}
	if len(routerRoutes) == 0 {
		t.Fatal("parsed zero registered routes from openapi.gen.go; the parser likely broke — refusing to pass vacuously")
	}

	if len(docOps) == 0 || len(docOnly) != 0 || len(routeOnly) != 0 {
		// Defensive: the equality assertions above should already have fired.
		return
	}

	t.Logf("lockstep OK: %d operations == %d routes == %d ServerInterface methods", len(docOps), len(routerRoutes), serverMethods)
}

// lockstepDocOperations extracts the set of "METHOD PATH" operation pairs from
// the OpenAPI document text. A path key is a 2-space-indented key starting with
// /api and ending in a colon; under it, a 4-space-indented bare HTTP method
// (get/post/put/patch/delete) denotes an operation. Path-level `parameters:`
// and any other keys are ignored.
func lockstepDocOperations(doc string) map[string]bool {
	ops := make(map[string]bool)
	currentPath := ""
	for _, line := range strings.Split(doc, "\n") {
		// Strip a trailing carriage return so the colon match is robust to CRLF.
		line = strings.TrimSuffix(line, "\r")

		if m := lockstepPathKeyRe.FindStringSubmatch(line); m != nil {
			currentPath = m[1]
			continue
		}
		if currentPath == "" {
			continue
		}
		if m := lockstepOpKeyRe.FindStringSubmatch(line); m != nil {
			if lockstepMethodSet[m[1]] {
				ops[strings.ToUpper(m[1])+" "+currentPath] = true
			}
		}
	}
	return ops
}

// lockstepRouterRoutes extracts the set of "METHOD PATH" pairs registered by
// the generated chi router, i.e. the (chi verb, quoted path literal) pairs from
// lines of the form `r.Get(options.BaseURL+"<path>", ...`.
func lockstepRouterRoutes(gen string) map[string]bool {
	routes := make(map[string]bool)
	for _, line := range strings.Split(gen, "\n") {
		if m := lockstepRouteRe.FindStringSubmatch(line); m != nil {
			routes[strings.ToUpper(m[1])+" "+m[2]] = true
		}
	}
	return routes
}

// lockstepServerInterfaceMethodCount counts the leading-tab method signatures
// in the generated ServerInterface block.
func lockstepServerInterfaceMethodCount(gen string) int {
	lines := strings.Split(gen, "\n")
	inBlock := false
	count := 0
	for _, line := range lines {
		if !inBlock {
			if lockstepServerInterfaceRe.MatchString(line) {
				inBlock = true
			}
			continue
		}
		// A non-indented line (e.g. the closing `}` at column 0) ends the block.
		if line != "" && !strings.HasPrefix(line, "\t") {
			break
		}
		if lockstepServerMethodRe.MatchString(line) {
			count++
		}
	}
	return count
}

// lockstepSetDiff returns the sorted keys present in a but absent from b.
func lockstepSetDiff(a, b map[string]bool) []string {
	var missing []string
	for k := range a {
		if !b[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	return missing
}
