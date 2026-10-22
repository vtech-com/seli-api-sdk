// Package cmd — openapi_path_coverage_test.go
//
// Guard test: every OpenAPI operation is either wrapped by a CLI command or
// listed as deliberately unwrapped, with a reason.
//
// The unit is the operation — METHOD + path — not the path alone. A path-only
// guard cannot see a verb appear on a path it already knows: the service could
// add `POST /api/members` beside the `GET` the CLI already calls, and a guard
// that tracked paths would stay green while a whole capability went unwrapped.
//
// The test asserts:
//   - every operation in the spec is in exactly one of the two maps
//   - the two maps are disjoint
//   - neither map names an operation the spec does not declare (no stale entries)
//   - every command named in implementedOps exists in the command tree
//
// Paths are the spec's own, verbatim (`/api/...`): the base URL is the host
// only, and the prefix lives in code next to each path.
//
// When the spec grows, this test fails until someone decides what the growth
// means. The failure is the decision, not noise to be silenced.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// implementedOps is every OpenAPI operation the CLI actually calls, mapped to
// the command (or commands, comma-separated) that calls it. Derived by reading
// cmd/*.go and internal/api/client.go for the literal methods and paths.
//
// The coverage tests that check flags (openapi_coverage_test.go,
// openapi_body_coverage_test.go) are driven from this map, so wrapping an
// operation here is also what puts its flags under guard.
var implementedOps = map[string]string{
	"GET /api/health": "health",

	"GET /api/members":      "members list",
	"GET /api/members/{id}": "members get",
}

// intentionallyUnwrapped is every operation the CLI deliberately does not wrap,
// each with a reason. Add here — rather than to implementedOps — when an
// operation is left without a command on purpose.
var intentionallyUnwrapped = map[string]string{
	"GET /api/capabilities":                   "plan 003 — not yet wrapped",
	"GET /api/members/{id}/permissions":       "plan 003 — not yet wrapped",
	"GET /api/roles":                          "plan 003 — not yet wrapped",
	"GET /api/roles/{id}":                     "plan 003 — not yet wrapped",
	"GET /api/delivery/{code}":                "plan 003 — not yet wrapped",
	"POST /api/delivery/{code}/label/100x150": "plan 003 — not yet wrapped",
	"POST /api/delivery/{code}/slip/a4":       "plan 003 — not yet wrapped",
}

// openAPIPathOnlySpec is the minimal subset of OpenAPI 3.x needed to enumerate
// operations: a path, and the methods hanging off it.
type openAPIPathOnlySpec struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

var httpMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true, "delete": true,
}

func specOperations(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("../api/openapi.json")
	if err != nil {
		t.Fatalf("read api/openapi.json: %v", err)
	}
	var spec openAPIPathOnlySpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse api/openapi.json: %v", err)
	}
	ops := map[string]bool{}
	for path, item := range spec.Paths {
		for method := range item {
			if httpMethods[method] {
				ops[fmt.Sprintf("%s %s", strings.ToUpper(method), path)] = true
			}
		}
	}
	if len(ops) == 0 {
		t.Fatal("openapi.json declares no operations; this guard would pass vacuously")
	}
	return ops
}

// pathCoverageProblems is the whole check as a pure function, so the test below
// can prove it fails on a spec that has outgrown the maps — without editing the
// maps themselves.
func pathCoverageProblems(ops map[string]bool, implemented, unwrapped map[string]string) []string {
	var problems []string

	for op := range implemented {
		if _, dup := unwrapped[op]; dup {
			problems = append(problems, fmt.Sprintf("%q is in both implementedOps and intentionallyUnwrapped; remove it from one", op))
		}
	}

	// No stale entries: an operation the API dropped must not linger in either
	// map, claiming a command wraps something that is gone.
	for _, m := range []struct {
		name string
		set  map[string]string
	}{{"implementedOps", implemented}, {"intentionallyUnwrapped", unwrapped}} {
		for op := range m.set {
			if !ops[op] {
				problems = append(problems, fmt.Sprintf("%s lists %q, which openapi.json does not declare; remove the stale entry", m.name, op))
			}
		}
	}

	for op := range ops {
		_, impl := implemented[op]
		_, unwrap := unwrapped[op]
		if !impl && !unwrap {
			problems = append(problems, fmt.Sprintf("openapi.json declares %q, which is in neither implementedOps nor intentionallyUnwrapped; "+
				"wrap it in a CLI command and list it in implementedOps, OR add it to intentionallyUnwrapped with a reason", op))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestOpenAPIPathCoverage(t *testing.T) {
	for _, p := range pathCoverageProblems(specOperations(t), implementedOps, intentionallyUnwrapped) {
		t.Error(p)
	}
}

// The guard has to fail when it should. Each case below is a way the maps and
// the spec fall out of step; none of them touches the real maps.
func TestPathCoverageProblemsDetectsDrift(t *testing.T) {
	ops := map[string]bool{"GET /api/a": true, "POST /api/a": true}
	reason := "plan 003 — not yet wrapped"

	cases := []struct {
		name        string
		impl, unw   map[string]string
		wantProblem string
	}{
		{"operation in neither map", map[string]string{"GET /api/a": "a"}, nil, `"POST /api/a", which is in neither`},
		{"operation in both maps", map[string]string{"GET /api/a": "a", "POST /api/a": "b"}, map[string]string{"POST /api/a": reason}, "is in both"},
		{"stale implemented entry", map[string]string{"GET /api/a": "a", "POST /api/a": "b", "GET /api/gone": "c"}, nil, `implementedOps lists "GET /api/gone"`},
		{"stale unwrapped entry", map[string]string{"GET /api/a": "a", "POST /api/a": "b"}, map[string]string{"DELETE /api/a": reason}, `intentionallyUnwrapped lists "DELETE /api/a"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(pathCoverageProblems(ops, tc.impl, tc.unw), "\n")
			if !strings.Contains(got, tc.wantProblem) {
				t.Errorf("want a problem containing %q, got:\n%s", tc.wantProblem, got)
			}
		})
	}

	if got := pathCoverageProblems(ops, map[string]string{"GET /api/a": "a"}, map[string]string{"POST /api/a": reason}); len(got) != 0 {
		t.Errorf("a consistent pair of maps reported problems: %v", got)
	}
}

// A reason that says nothing is a reason nobody revisits. Every entry in
// intentionallyUnwrapped carries one, and "TODO" is not one.
func TestIntentionallyUnwrappedCarryAReason(t *testing.T) {
	for op, reason := range intentionallyUnwrapped {
		if len(reason) < 25 {
			t.Errorf("%q is listed as unwrapped without a real reason: %q", op, reason)
		}
	}
}

// commandsFor resolves an implementedOps value to the cobra commands it names.
// A value may name several commands, separated by ", ".
func commandsFor(t *testing.T, op, value string) []*cobra.Command {
	t.Helper()
	var out []*cobra.Command
	for _, path := range strings.Split(value, ", ") {
		c := findCommandPath(rootCmd, strings.TrimSpace(path))
		if c == nil {
			t.Errorf("implementedOps maps %q to %q, which is not a command in the tree", op, path)
			continue
		}
		out = append(out, c)
	}
	return out
}

// findCommandPath walks the command tree for the command addressed by a
// space-separated path (e.g. "members list"), relative to root.
func findCommandPath(root *cobra.Command, path string) *cobra.Command {
	cur := root
	for _, part := range strings.Fields(path) {
		var next *cobra.Command
		for _, child := range cur.Commands() {
			if child.Name() == part {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

// implementedOps names the command that calls each operation. If a command is
// renamed or removed, the entry would keep claiming coverage for a call nobody
// makes.
func TestImplementedOpsNameRealCommands(t *testing.T) {
	for op, value := range implementedOps {
		for _, c := range commandsFor(t, op, value) {
			if !c.Runnable() {
				t.Errorf("implementedOps maps %q to %q, which is not a runnable command", op, value)
			}
		}
	}
}
