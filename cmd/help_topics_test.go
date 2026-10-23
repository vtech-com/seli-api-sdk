package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vtech-com/seli-api-sdk/internal/api"
)

// helpTopicNames are the cross-cutting pages a command help page may reference.
// A command page that says `seli help output` and finds nothing is a page that
// lies, so the set is asserted rather than assumed.
var helpTopicNames = []string{
	"tenancy",
	"output",
	"exit-codes",
}

func topicCmd(t *testing.T, name string) *cobra.Command {
	t.Helper()
	for _, c := range rootCmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("help topic %q is not registered on rootCmd", name)
	return nil
}

func TestHelpTopicsAreRegisteredAndNotRunnable(t *testing.T) {
	for _, name := range helpTopicNames {
		c := topicCmd(t, name)

		// A help topic must never execute. Cobra decides this by the absence
		// of Run/RunE, and only then lists it under "Additional help topics".
		if c.Runnable() {
			t.Errorf("help topic %q is runnable; it must have no Run/RunE", name)
		}
		if !c.IsAdditionalHelpTopicCommand() {
			t.Errorf("help topic %q is not treated as an additional help topic by cobra", name)
		}
		if strings.TrimSpace(c.Long) == "" {
			t.Errorf("help topic %q has an empty Long; the page is the whole point", name)
		}
		if strings.TrimSpace(c.Short) == "" {
			t.Errorf("help topic %q has an empty Short; it labels the root listing", name)
		}
	}
}

// The topics are reachable the way a caller reaches them: `seli help <topic>`,
// and they show up under "Additional help topics" in the root help.
func TestHelpTopicsAreReachableFromTheCLI(t *testing.T) {
	t.Parallel()

	rootHelp := cliRun(t, "", nil, "--help")
	for _, name := range helpTopicNames {
		name := name
		if !strings.Contains(rootHelp.stdout, name) {
			t.Errorf("root help does not list the %q topic:\n%s", name, rootHelp.stdout)
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res := cliRun(t, "", nil, "help", name)
			if res.exit != 0 {
				t.Fatalf("`seli help %s` exit = %d\nstderr: %s", name, res.exit, res.stderr)
			}
			first := strings.SplitN(topicCmd(t, name).Long, "\n", 2)[0]
			if !strings.Contains(res.stdout, first) {
				t.Errorf("`seli help %s` does not print the topic page, got:\n%s", name, res.stdout)
			}
		})
	}
}

// Any help page — topic or command — that points at `seli help <target>` is
// making a promise the reader will try to keep. A pointer to a topic that does
// not exist is the same class of defect as a page that lies about its output:
// the reader follows it and finds nothing. This walks the whole command tree,
// because command pages reference topics far more often than topics reference
// each other.
func TestHelpReferencesToTopicsResolve(t *testing.T) {
	// Register cobra's built-ins unconditionally, so this test sees the same
	// command tree whether or not another test in the package got there first.
	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd()

	// A reference must resolve to a topic that is actually registered, not
	// merely one this test file lists: a topic dropped from init() would
	// otherwise leave every pointer to it dangling and unnoticed.
	known := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		if c.IsAdditionalHelpTopicCommand() {
			known[c.Name()] = true
		}
	}

	references := 0
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		// Cobra authors these two pages, not us; `help` even documents itself
		// with the literal "seli help [path to command]".
		if c.Parent() == rootCmd && cobraAuthoredCommands[c.Name()] {
			return
		}
		for _, text := range []string{c.Long, c.Short} {
			for _, line := range strings.Split(text, "\n") {
				idx := strings.Index(line, "seli help ")
				if idx < 0 {
					continue
				}
				references++
				fields := strings.Fields(line[idx+len("seli help "):])
				if len(fields) == 0 {
					t.Errorf("%q writes `seli help` with no target", c.CommandPath())
					continue
				}
				// A reference may end a sentence — "See seli help output." —
				// so the punctuation a sentence leaves behind is not part of
				// the topic name.
				if target := strings.TrimRight(fields[0], ".,;:"); !known[target] {
					t.Errorf("%q references `seli help %s`, which is not a registered topic",
						c.CommandPath(), target)
				}
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)

	if references == 0 {
		t.Fatal("no page references a help topic; this guard would pass vacuously")
	}
}

// The tenancy page documents code: the three tenant_source values, and which
// commands require a tenant. If either changes in code, the page must follow.
func TestTenancyTopicMatchesTheCode(t *testing.T) {
	page := topicCmd(t, "tenancy").Long

	for _, source := range []string{api.TenantFromFlag, api.TenantFromEnv, api.TenantFromConfig} {
		if want := fmt.Sprintf(`"tenant_source": %q`, source); !strings.Contains(page, want) {
			t.Errorf("tenancy topic never shows %s", want)
		}
	}
	for _, want := range []string{"SELI_TENANT", "default_tenant", "TENANT_REQUIRED"} {
		if !strings.Contains(page, want) {
			t.Errorf("tenancy topic does not mention %s", want)
		}
	}

	// The page lists health and members as requiring a tenant; the code says so
	// in these constants. Flip one and the page is wrong.
	if !api.HealthRequiresTenant || !api.MembersRequiresTenant {
		t.Error("api.HealthRequiresTenant / api.MembersRequiresTenant changed: update the tenancy topic's WHICH COMMANDS REQUIRE A TENANT")
	}
	for _, cmd := range []string{"health", "members list", "members get"} {
		if !strings.Contains(page, cmd) {
			t.Errorf("tenancy topic does not name %q", cmd)
		}
	}
}

// The output page documents the envelope and the pagination keys the API sends.
func TestOutputTopicNamesTheEnvelope(t *testing.T) {
	page := topicCmd(t, "output").Long

	for _, want := range []string{
		`{ "data": …, "meta": { … } }`,
		`{ "error": { … }, "data": [ … ], "meta": { … } }`,
		"pagination.page", "pagination.pageSize", "pagination.total", "pagination.totalPages",
		"tenant_source", "request_id", "stderr", "stdout",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("output topic does not mention %q", want)
		}
	}
}

// The exit-codes page lists every code ExitCodeFor can return, and every local
// error code the CLI raises, so that a code added in api/errors.go cannot go
// unexplained.
func TestExitCodesTopicCoversEveryCode(t *testing.T) {
	page := topicCmd(t, "exit-codes").Long

	for code := 0; code <= 8; code++ {
		if !strings.Contains(page, fmt.Sprintf("\n  %d   ", code)) {
			t.Errorf("exit-codes topic has no row for exit %d", code)
		}
	}

	for _, code := range []string{
		api.ErrTenantRequired, api.ErrInvalidMemberID, api.ErrNetwork, api.ErrValidation,
		api.ErrAuthMissingHeader, api.ErrInsecureBaseURL, api.ErrAPIURLMissing,
	} {
		if !strings.Contains(page, code) {
			t.Errorf("exit-codes topic never mentions the local error code %s", code)
		}
	}

	// The exit code each local code maps to is stated next to it, so check the
	// statement against ExitCodeFor rather than trusting the prose.
	for code, want := range map[string]int{
		api.ErrTenantRequired:    5,
		api.ErrInvalidMemberID:   5,
		api.ErrValidation:        5,
		api.ErrAPIURLMissing:     5,
		api.ErrInsecureBaseURL:   5,
		api.ErrAuthMissingHeader: 2,
		api.ErrNetwork:           6,
	} {
		status := map[int]int{5: 400, 2: 401, 6: 0}[want]
		if got := api.ExitCodeFor(&api.APIError{Code: code, HTTPStatus: status}); got != want {
			t.Errorf("ExitCodeFor(%s, status %d) = %d, but the exit-codes topic files it under %d", code, status, got, want)
		}
	}
}

// The host is configuration, never text on a page. A topic that names one sends
// the reader to a host that may no longer be the right one.
func TestHelpTopicsNameNoHost(t *testing.T) {
	for _, name := range helpTopicNames {
		page := topicCmd(t, name).Long
		for _, scheme := range []string{"https://", "http://"} {
			if strings.Contains(page, scheme) {
				t.Errorf("help topic %q contains %q: the API host belongs to configuration, not to a page", name, scheme)
			}
		}
	}
}
