// Package cmd — openapi_body_coverage_test.go
//
// Guard test: OpenAPI requestBody fields ⟶ cobra flags (or --from-json escape hatch)
//
// This test prevents the class of bugs where a write command's request-body
// field exists in the OpenAPI spec but is never wired to a CLI flag.
//
// No wrapped operation has a request body today: health and members are reads.
// Plan 003 wraps the first two writes (the delivery label and slip POSTs, whose
// optional body is `extraFields`). The mechanism is here now so that the first
// write command lands under guard rather than next to it.
//
// For every operation in implementedOps whose spec entry declares a requestBody,
// each command that wraps it must satisfy ONE of:
//
//	(a) it registers --from-json, a generic body mechanism that covers ALL
//	    fields (the per-field assertion is then skipped), OR
//	(b) it registers a cobra flag for every property of the body schema, the
//	    flag name being the property name in kebab-case (extraFields →
//	    --extra-fields) after bodyFieldAliasMap has had its say, OR
//	(c) the property is listed in intentionallyUnexposedBodyFields with a reason.
//
// The commands come from implementedOps, not from a second list kept here, so a
// wrapped write cannot escape the guard by being left out of a mapping: listing
// the operation in implementedOps is what puts its body under guard.
//
// A body that is not a JSON object with named properties (a bare array, say)
// cannot be spread across flags, so it requires --from-json.
//
// Because no real wrapped operation has a body, the mechanism is proven against
// the spec's own POST operations and synthetic cobra commands (see
// TestBodyCoverageDetectsAWrappedPostWithoutFlags).
package cmd

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// bodyFieldToFlagName converts an OpenAPI requestBody field name to the
// expected cobra flag name. The base transform is toKebab (camelCase and
// snake_case → kebab-case); the alias map handles cases that differ from it.
func bodyFieldToFlagName(field string) string {
	if alias, ok := bodyFieldAliasMap[field]; ok {
		return alias
	}
	return toKebab(field)
}

// bodyFieldAliasMap maps OpenAPI body field names to the cobra flag name
// actually registered when the naming convention differs from toKebab.
// Document the reason for each alias, and the operation it is for.
//
// Empty today: no wrapped operation has a body. Plan 003 decides how
// `extraFields` (an array of {label, value} pairs) is spelled on the two
// delivery commands — a repeatable flag, or --from-json — and records it here.
var bodyFieldAliasMap = map[string]string{}

// intentionallyUnexposedBodyFields lists body fields that the CLI deliberately
// does not expose as flags, keyed by "<METHOD> <path>". Each entry maps field
// name → rationale. Fields in this list are skipped rather than failing the
// test. Any omission here must be a conscious, reviewed decision — not an
// accident. Add an entry only for a field that is genuinely server-managed or
// derived, on a command that does NOT use --from-json.
var intentionallyUnexposedBodyFields = map[string]map[string]string{}

// bodyProblems checks one wrapped operation against one command's flags.
func (s *openAPISpec) bodyProblems(opKey, cmdName string, o *openAPIOperation, hasFlag func(string) bool) []string {
	content, ok := o.RequestBody.Content["application/json"]
	if !ok {
		return []string{fmt.Sprintf("%s (%s): requestBody has no application/json content; this guard only understands JSON bodies — extend it before wrapping this operation", cmdName, opKey)}
	}

	// Escape hatch: --from-json covers all fields generically.
	if hasFlag("from-json") {
		return nil
	}

	schema := s.resolveSchema(content.Schema)
	if len(schema.Properties) == 0 {
		return []string{fmt.Sprintf("%s (%s): the request body has no named properties to spread across flags, so the command must register --from-json", cmdName, opKey)}
	}

	var problems []string
	for field := range schema.Properties {
		if _, skip := intentionallyUnexposedBodyFields[opKey][field]; skip {
			continue
		}
		flag := bodyFieldToFlagName(field)
		if !hasFlag(flag) {
			problems = append(problems, fmt.Sprintf(
				"%s (%s): body field %q expects cobra flag --%s but it is not registered; "+
					"add the flag, register --from-json, add an alias to bodyFieldAliasMap, "+
					"or add %q to intentionallyUnexposedBodyFields for this operation",
				cmdName, opKey, field, flag, field))
		}
	}
	sort.Strings(problems)
	return problems
}

// bodyProblemsForOps runs the body check over every wrapped operation that has
// a request body. lookup resolves a command path to its command. It returns the
// problems found and how many operation/command pairs it actually checked.
func (s *openAPISpec) bodyProblemsForOps(implemented map[string]string, lookup func(path string) *cobra.Command) (problems []string, checked int) {
	ops := make([]string, 0, len(implemented))
	for op := range implemented {
		ops = append(ops, op)
	}
	sort.Strings(ops)

	for _, op := range ops {
		o := s.operation(op)
		if o == nil {
			problems = append(problems, fmt.Sprintf("%s is in implementedOps but openapi.json has no such operation", op))
			continue
		}
		if o.RequestBody == nil {
			continue // nothing to carry; a read, or a write with no body
		}
		for _, path := range strings.Split(implemented[op], ", ") {
			c := lookup(strings.TrimSpace(path))
			if c == nil {
				problems = append(problems, fmt.Sprintf("%s: implementedOps names %q, which is not a command in the tree", op, path))
				continue
			}
			checked++
			problems = append(problems, s.bodyProblems(op, c.CommandPath(), o,
				func(n string) bool { return c.Flags().Lookup(n) != nil })...)
		}
	}
	return problems, checked
}

func TestOpenAPIBodyCoverage(t *testing.T) {
	spec := loadSpec(t)

	problems, checked := spec.bodyProblemsForOps(implementedOps, func(path string) *cobra.Command {
		return findCommandPath(rootCmd, path)
	})
	for _, p := range problems {
		t.Error(p)
	}
	t.Logf("checked %d wrapped operation/command pairs that carry a request body", checked)
}

// With no wrapped write today the test above checks nothing, which is exactly
// when a guard rots. This drives the mechanism against the spec's real POST
// operations and stand-in commands: a wrapped POST with neither the flags nor
// --from-json must fail, and the ways of satisfying the guard must pass.
func TestBodyCoverageDetectsAWrappedPostWithoutFlags(t *testing.T) {
	spec := loadSpec(t)
	const post = "POST /api/delivery/{code}/label/100x150"
	if o := spec.operation(post); o == nil || o.RequestBody == nil {
		t.Fatalf("%s no longer has a request body in openapi.json; pick another body-carrying POST for this test", post)
	}

	// stand-in builds a command registering exactly the given flags.
	standIn := func(flags ...string) *cobra.Command {
		c := &cobra.Command{Use: "label"}
		for _, f := range flags {
			c.Flags().String(f, "", "")
		}
		return c
	}
	run := func(c *cobra.Command) ([]string, int) {
		return spec.bodyProblemsForOps(map[string]string{post: "delivery label"}, func(string) *cobra.Command { return c })
	}

	// No flags, no --from-json: the guard must say which field has no flag.
	problems, checked := run(standIn())
	if checked != 1 {
		t.Fatalf("checked %d pairs, want 1: the guard did not see the wrapped POST", checked)
	}
	got := strings.Join(problems, "\n")
	for _, want := range []string{`body field "extraFields"`, "--extra-fields", post} {
		if !strings.Contains(got, want) {
			t.Errorf("failure for a flagless wrapped POST should contain %q, got:\n%s", want, got)
		}
	}

	// The camelCase spelling is not the flag.
	if problems, _ := run(standIn("extraFields")); len(problems) == 0 {
		t.Error("a flag spelled --extraFields must not satisfy the extraFields body field")
	}

	// --from-json covers every field.
	if problems, _ := run(standIn("from-json")); len(problems) != 0 {
		t.Errorf("--from-json is the escape hatch, got: %v", problems)
	}

	// A flag per field passes.
	if problems, _ := run(standIn("extra-fields")); len(problems) != 0 {
		t.Errorf("--extra-fields covers extraFields, got: %v", problems)
	}

	// An alias is honoured, and only while it is registered.
	bodyFieldAliasMap["extraFields"] = "extra-field"
	defer delete(bodyFieldAliasMap, "extraFields")
	if problems, _ := run(standIn("extra-field")); len(problems) != 0 {
		t.Errorf("alias extraFields→extra-field should be honoured, got: %v", problems)
	}
	if problems, _ := run(standIn("extra-fields")); len(problems) == 0 {
		t.Error("once aliased to --extra-field, --extra-fields no longer satisfies the guard")
	}
	delete(bodyFieldAliasMap, "extraFields")

	// A documented exemption passes.
	intentionallyUnexposedBodyFields[post] = map[string]string{"extraFields": "server-managed"}
	defer delete(intentionallyUnexposedBodyFields, post)
	if problems, _ := run(standIn()); len(problems) != 0 {
		t.Errorf("an intentionallyUnexposedBodyFields entry must be skipped, got: %v", problems)
	}
	delete(intentionallyUnexposedBodyFields, post)

	// Listing a body-carrying operation against a command that is not in the
	// tree is a failure, not a silent skip.
	problems, _ = spec.bodyProblemsForOps(map[string]string{post: "delivery label"}, func(string) *cobra.Command { return nil })
	if len(problems) != 1 || !strings.Contains(problems[0], "not a command in the tree") {
		t.Errorf("a missing command must be reported, got: %v", problems)
	}

	// A wrapped read has no body: nothing to check, and no false alarm.
	problems, checked = spec.bodyProblemsForOps(map[string]string{"GET /api/members": "members list"},
		func(string) *cobra.Command { return standIn() })
	if len(problems) != 0 || checked != 0 {
		t.Errorf("a GET carries no body; got problems=%v checked=%d", problems, checked)
	}
}
