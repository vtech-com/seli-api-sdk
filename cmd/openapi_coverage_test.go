// Package cmd — openapi_coverage_test.go
//
// Guard test: OpenAPI query params ⟶ cobra flags
//
// This test prevents the class of bugs where an OpenAPI endpoint supports a
// query parameter but the corresponding CLI command never exposes a flag for it
// (the list endpoints gain a filter, the CLI never learns of it).
//
// For each wrapped GET operation in implementedOps, the test:
//  1. Parses api/openapi.json and extracts every in:query parameter of the
//     operation, following $ref into components.parameters.
//  2. Normalises the param name to the flag name the CLI uses. The rule is one
//     rule, not a table of cases: camelCase and snake_case both become
//     kebab-case, so Seli's `pageSize` is `--page-size`.
//  3. Applies a small alias map for deliberate renames that the rule cannot
//     derive (empty today — every Seli query parameter follows the rule).
//  4. Asserts the wrapped command registers a flag for every param, OR the param
//     is listed in intentionallyUnexposed with a documented rationale.
//
// The commands come from implementedOps, not from a second list kept here: an
// operation that gains a command is checked the moment it is listed there, and
// there is no mapping to forget to extend.
//
// If a param is added to the OpenAPI spec in the future, this test will
// immediately catch any command that does not expose it.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	kebabLowerUpper = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	kebabAcronym    = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
)

// toKebab converts an OpenAPI parameter or body-field name to the expected
// cobra flag name: camelCase and snake_case both become kebab-case.
//
//	pageSize  → page-size
//	sort_by   → sort-by
//	userID    → user-id
func toKebab(name string) string {
	name = kebabAcronym.ReplaceAllString(name, "${1}-${2}")
	name = kebabLowerUpper.ReplaceAllString(name, "${1}-${2}")
	name = strings.ReplaceAll(name, "_", "-")
	return strings.ToLower(name)
}

// paramToFlagName is the flag a query parameter is expected to surface as,
// after the alias map has had its say.
func paramToFlagName(param string) string {
	if alias, ok := queryParamAliases[param]; ok {
		return alias
	}
	return toKebab(param)
}

// queryParamAliases maps OpenAPI query param names to the cobra flag name
// actually registered when the naming convention differs from toKebab.
// Document the reason for each alias.
//
// Empty today: every query parameter Seli declares (`page`, `pageSize`,
// `status`) follows the camelCase → kebab-case rule.
var queryParamAliases = map[string]string{}

// intentionallyUnexposed lists query params that the CLI deliberately does not
// expose as flags, keyed by "<METHOD> <path>" and then by param name, with a
// documented rationale. Params in this list are skipped by the test rather than
// failing it. Any omission here must be a conscious, reviewed decision — not an
// accident.
var intentionallyUnexposed = map[string]map[string]string{
	// No intentionally unexposed params at this time.
	// Add entries here if a param is deliberately not surfaced:
	//   "GET /api/some/path": {"paramName": "reason why it is not exposed"},
}

// openAPISpec is the subset of OpenAPI 3.x these guards read.
type openAPISpec struct {
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Parameters map[string]openAPIParameter `json:"parameters"`
		Schemas    map[string]openAPISchema    `json:"schemas"`
	} `json:"components"`
}

type openAPIOperation struct {
	Parameters  []openAPIParameter  `json:"parameters"`
	RequestBody *openAPIRequestBody `json:"requestBody"`
}

type openAPIParameter struct {
	Ref      string `json:"$ref"`
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
}

type openAPIRequestBody struct {
	Required bool `json:"required"`
	Content  map[string]struct {
		Schema openAPISchema `json:"schema"`
	} `json:"content"`
}

type openAPISchema struct {
	Ref        string                     `json:"$ref"`
	Type       json.RawMessage            `json:"type"`
	Properties map[string]json.RawMessage `json:"properties"`
	AllOf      []openAPISchema            `json:"allOf"`
}

func loadSpec(t *testing.T) *openAPISpec {
	t.Helper()
	data, err := os.ReadFile("../api/openapi.json")
	if err != nil {
		t.Fatalf("read api/openapi.json: %v", err)
	}
	var spec openAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse api/openapi.json: %v", err)
	}
	return &spec
}

// operation returns the operation for "METHOD /path", or nil if the spec has
// none.
func (s *openAPISpec) operation(op string) *openAPIOperation {
	method, path, _ := strings.Cut(op, " ")
	raw, ok := s.Paths[path][strings.ToLower(method)]
	if !ok {
		return nil
	}
	var o openAPIOperation
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil
	}
	return &o
}

// resolveParam follows a $ref into components.parameters.
func (s *openAPISpec) resolveParam(p openAPIParameter) openAPIParameter {
	const prefix = "#/components/parameters/"
	if strings.HasPrefix(p.Ref, prefix) {
		if r, ok := s.Components.Parameters[strings.TrimPrefix(p.Ref, prefix)]; ok {
			return r
		}
	}
	return p
}

// resolveSchema follows a $ref into components.schemas and flattens allOf, so a
// body described as a composition still lists every property.
func (s *openAPISpec) resolveSchema(sc openAPISchema) openAPISchema {
	const prefix = "#/components/schemas/"
	for depth := 0; strings.HasPrefix(sc.Ref, prefix) && depth < 8; depth++ {
		r, ok := s.Components.Schemas[strings.TrimPrefix(sc.Ref, prefix)]
		if !ok {
			break
		}
		sc = r
	}
	if len(sc.AllOf) > 0 {
		merged := openAPISchema{Type: sc.Type, Properties: map[string]json.RawMessage{}}
		for k, v := range sc.Properties {
			merged.Properties[k] = v
		}
		for _, part := range sc.AllOf {
			for k, v := range s.resolveSchema(part).Properties {
				merged.Properties[k] = v
			}
		}
		sc = merged
	}
	return sc
}

// queryParams lists the in:query parameter names of an operation.
func (s *openAPISpec) queryParams(o *openAPIOperation) []string {
	var names []string
	for _, p := range o.Parameters {
		if p = s.resolveParam(p); p.In == "query" {
			names = append(names, p.Name)
		}
	}
	return names
}

// queryParamProblems is the check for one command as a pure function: every
// query param needs a flag, an alias to one, or a documented exemption.
func queryParamProblems(opKey, cmdName string, params []string, hasFlag func(string) bool, unexposed map[string]string) []string {
	var problems []string
	for _, param := range params {
		if _, skip := unexposed[param]; skip {
			continue
		}
		flag := paramToFlagName(param)
		if !hasFlag(flag) {
			problems = append(problems, fmt.Sprintf(
				"%s (%s): OpenAPI query param %q expects cobra flag --%s but it is not registered; "+
					"add the flag, add an alias to queryParamAliases, or add %q to intentionallyUnexposed",
				cmdName, opKey, param, flag, param))
		}
	}
	return problems
}

func TestOpenAPICoverage(t *testing.T) {
	spec := loadSpec(t)

	var ops []string
	for op := range implementedOps {
		if strings.HasPrefix(op, "GET ") {
			ops = append(ops, op)
		}
	}
	sort.Strings(ops)

	checked := 0
	for _, op := range ops {
		op := op
		t.Run(op, func(t *testing.T) {
			o := spec.operation(op)
			if o == nil {
				t.Fatalf("%s is in implementedOps but openapi.json has no such operation", op)
			}
			params := spec.queryParams(o)
			for _, c := range commandsFor(t, op, implementedOps[op]) {
				c := c
				checked += len(params)
				for _, p := range queryParamProblems(op, c.CommandPath(), params,
					func(n string) bool { return c.Flags().Lookup(n) != nil }, intentionallyUnexposed[op]) {
					t.Error(p)
				}
			}
		})
	}

	if len(ops) == 0 || checked == 0 {
		t.Fatalf("checked %d query params across %d wrapped GETs; this guard would pass vacuously", checked, len(ops))
	}
}

// The name rule is the contract the guard enforces, so it is pinned directly.
func TestToKebab(t *testing.T) {
	for in, want := range map[string]string{
		"page":     "page",
		"pageSize": "page-size",
		"sort_by":  "sort-by",
		"userID":   "user-id",
		"XTenant":  "x-tenant",
		"status":   "status",
	} {
		if got := toKebab(in); got != want {
			t.Errorf("toKebab(%q) = %q, want %q", in, got, want)
		}
	}
	if got := paramToFlagName("pageSize"); got != "page-size" {
		t.Errorf("paramToFlagName(pageSize) = %q, want page-size", got)
	}
}

// The guard has to fail when a flag goes missing, and has to honour the two
// ways of saying a missing flag is deliberate.
func TestQueryParamProblemsDetectsAMissingFlag(t *testing.T) {
	has := func(names ...string) func(string) bool {
		return func(n string) bool {
			for _, x := range names {
				if x == n {
					return true
				}
			}
			return false
		}
	}
	params := []string{"page", "pageSize", "status"}

	if got := queryParamProblems("GET /x", "seli x", params, has("page", "page-size", "status"), nil); len(got) != 0 {
		t.Errorf("all flags present, got problems: %v", got)
	}

	got := strings.Join(queryParamProblems("GET /x", "seli x", params, has("page", "status"), nil), "\n")
	if !strings.Contains(got, `"pageSize" expects cobra flag --page-size`) {
		t.Errorf("a missing --page-size must be named in the failure, got:\n%q", got)
	}

	// The camelCase name is not a flag: `--pageSize` would pass a naive guard.
	if got := queryParamProblems("GET /x", "seli x", params, has("page", "pageSize", "status"), nil); len(got) == 0 {
		t.Error("a flag spelled --pageSize must not satisfy the pageSize param")
	}

	if got := queryParamProblems("GET /x", "seli x", params, has("page", "status"), map[string]string{"pageSize": "reason"}); len(got) != 0 {
		t.Errorf("an intentionallyUnexposed param must be skipped, got: %v", got)
	}
}
