package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestNoSubcommandIsRegisteredTwice walks the whole command tree. A command added
// to its parent twice shows up twice in help and makes the second registration
// dead code; it happens when two branches each extend the same AddCommand call.
func TestNoSubcommandIsRegisteredTwice(t *testing.T) {
	count := 0
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		seen := map[string]bool{}
		for _, sub := range c.Commands() {
			count++
			if seen[sub.Name()] {
				t.Errorf("%q registers the subcommand %q more than once", c.CommandPath(), sub.Name())
			}
			seen[sub.Name()] = true
			walk(sub)
		}
	}
	walk(rootCmd)

	if count == 0 {
		t.Fatal("the command tree is empty; this guard would pass vacuously")
	}
}

// The walk above can only see a duplicate if cobra hands both copies back. This
// shows that it does, on a tree built to contain one.
func TestDuplicateSubcommandIsVisibleToTheWalk(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.AddCommand(&cobra.Command{Use: "dup"}, &cobra.Command{Use: "dup"})

	seen := map[string]int{}
	for _, sub := range root.Commands() {
		seen[sub.Name()]++
	}
	if seen["dup"] != 2 {
		t.Fatalf("cobra returned %d copies of the duplicate, want 2; the guard above could not see a double registration", seen["dup"])
	}
}
