package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// GoReleaser renders every repository `token:` with ApplySingleEnvOnly, which
// accepts nothing but a bare {{ .Env.NAME }} and defines no template functions.
// Anything else passes `goreleaser check` and a --skip=publish rehearsal, then
// fails the real release at the push — after the GitHub Release is already
// out. v0.1.0 shipped without its Homebrew cask for exactly that reason.
//
// The pattern is GoReleaser's own envOnlyRe (internal/tmpl/tmpl.go, v2.18).
var envOnlyToken = regexp.MustCompile(`^{{\s*\.Env\.[^.\s}]+\s*}}$`)

var tokenLine = regexp.MustCompile(`^\s*token:\s*(.+?)\s*$`)

func TestGoReleaserTokensAreEnvOnly(t *testing.T) {
	raw, err := os.ReadFile(".goreleaser.yml")
	if err != nil {
		t.Fatalf("read .goreleaser.yml: %v", err)
	}

	found := 0
	for i, line := range strings.Split(string(raw), "\n") {
		m := tokenLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		found++
		value := strings.Trim(m[1], `"'`)
		if !envOnlyToken.MatchString(value) {
			t.Errorf(".goreleaser.yml:%d: token %q must be exactly {{ .Env.NAME }}; GoReleaser rejects anything else at publish time", i+1, value)
		}
	}
	if found == 0 {
		t.Fatal(".goreleaser.yml has no token: line; the Homebrew cask needs one, so this guard is checking nothing")
	}
}
