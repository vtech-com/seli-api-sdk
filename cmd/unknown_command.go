package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// -----------------------------------------------------------------------------
// Make cobra's own "unknown command" detection fire for subcommand groups,
// not just the root.
//
// Cobra's default Args validator (legacyArgs, see spf13/cobra args.go) only
// raises "unknown command %q for %q" at the root: `!cmd.HasParent()`. For a
// group command — one with subcommands but no Run/RunE of its own — unmatched
// trailing args are accepted without error, cmd.Runnable() is false, and
// cobra's execute() takes the `if !c.Runnable() { return flag.ErrHelp }`
// branch: it silently prints the group's help with exit 0.
//
// enableUnknownSubcommandErrors walks the full command tree once, after all
// commands have been registered, and gives every non-runnable group command a
// RunE that mirrors cobra's own root-level message (including cobra's real
// SuggestionsFor distance computation — reused, not reimplemented) for any
// unmatched subcommand-like argument. A bare invocation of the group (no args)
// still just prints help, unchanged.
// -----------------------------------------------------------------------------

func enableUnknownSubcommandErrors(c *cobra.Command) {
	for _, child := range c.Commands() {
		enableUnknownSubcommandErrors(child)
	}
	if !c.HasSubCommands() || c.Runnable() {
		return
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return fmt.Errorf("unknown command %q for %q%s", args[0], cmd.CommandPath(), suggestionsBlock(cmd, args[0]))
	}
}

// suggestionsBlock reproduces cobra's unexported findSuggestions formatting
// (see spf13/cobra command.go) on top of the exported SuggestionsFor, so the
// "Did you mean this?" text stays byte-for-byte consistent with cobra's own
// root-level output.
func suggestionsBlock(c *cobra.Command, arg string) string {
	if c.DisableSuggestions {
		return ""
	}
	// Self-heal the distance threshold exactly as cobra's own (unexported)
	// findSuggestions does. The *exported* SuggestionsFor does NOT do this, so
	// with the zero-value default only prefix matches would suggest.
	if c.SuggestionsMinimumDistance <= 0 {
		c.SuggestionsMinimumDistance = 2
	}
	var sb strings.Builder
	if suggestions := c.SuggestionsFor(arg); len(suggestions) > 0 {
		sb.WriteString("\n\nDid you mean this?\n")
		for _, s := range suggestions {
			_, _ = fmt.Fprintf(&sb, "\t%v\n", s)
		}
	}
	return sb.String()
}
