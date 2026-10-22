package cmd

import (
	"io"
	"strings"
	"testing"
)

// TestGroupLevelTypoSuggestsSibling drives the RunE that
// enableUnknownSubcommandErrors installs on a group command with a typo'd
// subcommand and asserts the resulting error carries cobra's edit-distance
// suggestion. Guards the SuggestionsMinimumDistance self-heal: the exported
// SuggestionsFor does NOT set the threshold, so without the self-heal a real
// typo like "gett" (distance 1 from "get") would return no suggestion.
func TestGroupLevelTypoSuggestsSibling(t *testing.T) {
	enableUnknownSubcommandErrors(rootCmd)

	members := findCommandPath(rootCmd, "members")
	if members == nil {
		t.Fatal("members group not found in command tree")
		return
	}
	if members.RunE == nil {
		t.Fatal("enableUnknownSubcommandErrors did not install a RunE on the members group")
		return
	}

	err := members.RunE(members, []string{"gett", "foo"})
	if err == nil {
		t.Fatal("expected an unknown-command error for a typo'd subcommand, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, `unknown command "gett" for "seli members"`) {
		t.Errorf("error should name the unknown subcommand and its group, got %q", msg)
	}
	if !strings.Contains(msg, "Did you mean this?") || !strings.Contains(msg, "get") {
		t.Errorf("error should suggest the sibling 'get' via edit distance, got %q", msg)
	}
}

// A group with no arguments is a request for its help, not an error: only an
// unmatched argument is unknown.
func TestBareGroupPrintsHelpWithoutError(t *testing.T) {
	enableUnknownSubcommandErrors(rootCmd)

	for _, name := range []string{"members", "auth", "config"} {
		group := findCommandPath(rootCmd, name)
		if group == nil || group.RunE == nil {
			t.Fatalf("group %q missing or without the unknown-subcommand RunE", name)
		}
		group.SetOut(io.Discard)
		if err := group.RunE(group, nil); err != nil {
			t.Errorf("a bare `seli %s` must print help, got error: %v", name, err)
		}
	}
}

// End to end: the mistake reaches the caller as the CLI's own error envelope
// with the validation exit code, rather than as help text and exit 0 (which is
// what cobra does for an unmatched argument on a group).
func TestUnknownSubcommandExitsFiveWithASuggestion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"typo under a group", []string{"members", "gett"}, []string{`unknown command "gett" for "seli members"`, "Did you mean this?", "get"}},
		{"typo at the root", []string{"membrs"}, []string{`unknown command "membrs" for "seli"`, "Did you mean this?", "members"}},
		{"nonsense under a group", []string{"auth", "zzzzzz"}, []string{`unknown command "zzzzzz" for "seli auth"`}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := cliRun(t, "", nil, tc.args...)
			if res.exit != 5 {
				t.Errorf("exit = %d, want 5\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
			}
			errObj := obj(t, res.doc(t), "error")
			if errObj["code"] != "VALIDATION_ERROR" {
				t.Errorf("error.code = %v, want VALIDATION_ERROR", errObj["code"])
			}
			msg, _ := errObj["message"].(string)
			for _, want := range tc.want {
				if !strings.Contains(msg, want) {
					t.Errorf("error.message lacks %q:\n%s", want, msg)
				}
			}
		})
	}
}

// A bare group prints its help and exits 0 end to end too.
func TestBareGroupExitsZeroWithHelp(t *testing.T) {
	t.Parallel()
	res := cliRun(t, "", nil, "members")
	if res.exit != 0 {
		t.Errorf("exit = %d, want 0\nstderr: %s", res.exit, res.stderr)
	}
	for _, want := range []string{"list", "get"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("`seli members` help does not list %q:\n%s", want, res.stdout)
		}
	}
}
