package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// requiredSlots are the sections every command page carries, in this order.
//
// The order matters as much as the presence: it is the order a caller needs
// them in. What is this for, how do I spell the call, what does each flag do,
// what comes back.
var requiredSlots = []string{"PURPOSE", "USAGE", "FLAGS", "OUTPUT"}

// retiredSlots are headings the pages do not carry. A caveat belongs to the
// flag it qualifies; an example belongs beside the flag it demonstrates; the
// command tree already says what the sibling commands are; and INPUT was the
// flag list under another name.
var retiredSlots = []string{"INPUT", "CAVEATS", "EXAMPLES", "SEE ALSO"}

// cobraAuthoredCommands are the pages cobra writes, not us.
var cobraAuthoredCommands = map[string]bool{"help": true, "completion": true}

// globalFlags are the persistent flags registered on the root command. They are
// documented once, on the root page; a command page need not repeat them.
var globalFlags = map[string]bool{"api-url": true, "verbose": true, "tenant": true, "help": true}

// callableCommands are the pages that document a call: every command a caller
// can actually run. A pure group documents a noun and a help topic documents a
// contract; neither gets the skeleton.
//
// "Runnable and has no children" is how a call is told from a group here, and
// not Runnable() alone: enableUnknownSubcommandErrors gives every group a RunE
// (Execute() calls it in production; unknown_command_test.go calls it here), so
// asking Runnable() after that has happened would sweep every group into the
// skeleton check, and would do so only when the tests ran in a particular
// order. The tree is walked when a test asks, not at package init: commands
// register themselves in init(), which runs after package-level vars.
func callableCommands(t *testing.T) []*cobra.Command {
	t.Helper()
	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd()

	var out []*cobra.Command
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if cobraAuthoredCommands[c.Name()] && c.Parent() == rootCmd {
			return
		}
		if c != rootCmd && c.Runnable() && !c.HasSubCommands() {
			out = append(out, c)
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)

	// The walk must see the real tree. An empty one would pass every check
	// below without examining a single page.
	if len(out) < 8 {
		t.Fatalf("found only %d callable commands; the command tree is not registered, so these checks would pass vacuously", len(out))
	}
	return out
}

func TestLeafCommandsFollowTheSkeleton(t *testing.T) {
	for _, c := range callableCommands(t) {
		checkSkeleton(t, c)
	}
}

func checkSkeleton(t *testing.T, c *cobra.Command) {
	t.Helper()
	for _, problem := range skeletonProblems(c.CommandPath(), c.Long) {
		t.Error(problem)
	}
}

// skeletonProblems checks one page's long help against the skeleton. It takes
// the text, not the command, so the check can be shown to fail on a page that
// is missing a section.
func skeletonProblems(path, long string) []string {
	if strings.TrimSpace(long) == "" {
		return []string{path + " has no long help; a caller cannot build a call from a Short line"}
	}

	var problems []string

	// Slot headers sit alone on their own line. A trailing parenthetical would
	// make the page unparseable by anything but a human.
	seen := map[string]int{}
	for i, line := range strings.Split(long, "\n") {
		for _, slot := range requiredSlots {
			if line == slot {
				if _, dup := seen[slot]; dup {
					problems = append(problems, path+" repeats the "+slot+" header")
				}
				seen[slot] = i
			}
		}
	}

	prev := -1
	for _, slot := range requiredSlots {
		at, ok := seen[slot]
		if !ok {
			problems = append(problems, path+" is missing the "+slot+" section")
			continue
		}
		if at < prev {
			problems = append(problems, path+" has "+slot+" out of order")
		}
		prev = at
	}
	return problems
}

// The skeleton check must fail on a page that breaks it.
func TestSkeletonProblemsDetectsABrokenPage(t *testing.T) {
	whole := "Do a thing.\n\nPURPOSE\n  x\n\nUSAGE\n  seli x\n\nFLAGS\n  none\n\nOUTPUT\n  .data"
	if got := skeletonProblems("seli x", whole); len(got) != 0 {
		t.Fatalf("a complete page reported problems: %v", got)
	}
	for slot, page := range map[string]string{
		"PURPOSE": strings.Replace(whole, "PURPOSE", "WHY", 1),
		"USAGE":   strings.Replace(whole, "\nUSAGE\n", "\nUSAGE (see below)\n", 1),
		"FLAGS":   strings.Replace(whole, "FLAGS", "OPTIONS", 1),
		"OUTPUT":  strings.Replace(whole, "OUTPUT", "RESULT", 1),
	} {
		got := strings.Join(skeletonProblems("seli x", page), "\n")
		if !strings.Contains(got, "missing the "+slot+" section") {
			t.Errorf("a page without %s was not flagged, got: %q", slot, got)
		}
	}
	if got := skeletonProblems("seli x", ""); len(got) == 0 {
		t.Error("a page with no long help was not flagged")
	}
	swapped := strings.Replace(strings.Replace(whole, "FLAGS", "TMP", 1), "OUTPUT", "FLAGS", 1)
	swapped = strings.Replace(swapped, "TMP", "OUTPUT", 1)
	if got := strings.Join(skeletonProblems("seli x", swapped), "\n"); !strings.Contains(got, "out of order") {
		t.Errorf("sections out of order were not flagged, got: %q", got)
	}
}

// The retired headings are easy to reintroduce by habit, and each one puts a
// fact somewhere a reader has to go looking for it.
func TestPagesDoNotCarryRetiredSections(t *testing.T) {
	for _, c := range callableCommands(t) {
		for _, line := range strings.Split(c.Long, "\n") {
			for _, slot := range retiredSlots {
				if line == slot {
					t.Errorf("%q carries the retired %s section", c.CommandPath(), slot)
				}
			}
		}
	}
}

// Cobra's own Flags block is not what a caller reads (the pages are the
// contract), so a flag that the FLAGS section forgets is a flag with no
// documentation anywhere.
func TestFlagsSectionDocumentsEveryFlag(t *testing.T) {
	for _, c := range callableCommands(t) {
		section := sectionOf(c.Long, "FLAGS")
		if section == "" {
			continue // reported by checkSkeleton
		}
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if globalFlags[f.Name] {
				return
			}
			if !strings.Contains(section, "--"+f.Name) {
				t.Errorf("%q defines --%s but FLAGS does not document it", c.CommandPath(), f.Name)
			}
		})
	}
}

// The reverse worry: a page that names a flag the command does not define
// sends the reader to run something that will be rejected.
func TestHelpDoesNotNameFlagsThatDoNotExist(t *testing.T) {
	for _, c := range callableCommands(t) {
		// Only FLAGS is checked: OUTPUT and PURPOSE routinely mention another
		// command's flags on purpose.
		input := sectionOf(c.Long, "FLAGS")
		if input == "" {
			continue
		}
		for _, name := range flagNamesIn(input) {
			if globalFlags[name] || c.Flags().Lookup(name) != nil {
				continue
			}
			t.Errorf("%q documents --%s in FLAGS, but defines no such flag", c.CommandPath(), name)
		}
	}
}

// Every page a caller reads should show the call it is reading about. A USAGE
// line that does not begin with the command's own path is describing something
// else.
func TestUsageSectionShowsThisCommand(t *testing.T) {
	for _, c := range callableCommands(t) {
		usage := sectionOf(c.Long, "USAGE")
		if usage == "" {
			continue
		}
		if !strings.Contains(usage, c.CommandPath()) {
			t.Errorf("%q has a USAGE section that never spells %q", c.CommandPath(), c.CommandPath())
		}
	}
}

// There is one output shape, and no flag to ask for another. A page that offers
// `-o json` or explains "table mode" is describing a CLI that does not exist —
// and the reader will run what it says.
//
// This walks every page, not only the leaves: a group page or a help topic can
// describe an output mode just as wrongly as a command page can.
func TestNoPageAdvertisesAnOutputMode(t *testing.T) {
	banned := []string{"-o json", "-o table", "-o quiet", "--output", "table mode", "quiet mode", "json mode"}

	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd()

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if !cobraAuthoredCommands[c.Name()] || c.Parent() != rootCmd {
			for _, line := range strings.Split(c.Long+"\n"+c.Short, "\n") {
				for _, phrase := range banned {
					if strings.Contains(line, phrase) {
						t.Errorf("%q says %q; there is one output shape and no output flag\n    %s",
							c.CommandPath(), phrase, strings.TrimSpace(line))
					}
				}
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
}

// Every page that documents a call must say where its result lives. The
// envelope is the whole contract: a caller that reaches for `.id` on a `get`
// gets null, because the object is at `.data`.
func TestOutputSectionNamesTheEnvelope(t *testing.T) {
	// These commands print nothing at all on success, which their pages say.
	silent := map[string]bool{
		"seli config set":                  true,
		"seli config set-default-tenant":   true,
		"seli config unset-default-tenant": true,
	}
	for _, c := range callableCommands(t) {
		if silent[c.CommandPath()] {
			continue
		}
		out := sectionOf(c.Long, "OUTPUT")
		if !strings.Contains(out, ".data") && !strings.Contains(out, `"data"`) {
			t.Errorf("%q has an OUTPUT section that never mentions data; where does the result live?", c.CommandPath())
		}
	}
}

// Command sorting is off, so a group's children display in the order they were
// registered — and init() runs in file-name order. A group whose children are
// spread across two files therefore lists them in an order nobody chose.
// Reading comes before writing, so `list` leads.
func TestListIsTheFirstCommandInItsGroup(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		var visible []*cobra.Command
		for _, sub := range c.Commands() {
			if sub.IsAvailableCommand() {
				visible = append(visible, sub)
			}
		}
		hasList := false
		for _, sub := range visible {
			if sub.Name() == "list" {
				hasList = true
			}
		}
		if hasList && visible[0].Name() != "list" {
			t.Errorf("%q lists %q before %q; registration order is display order",
				c.CommandPath(), visible[0].Name(), "list")
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
}

// sectionOf returns the body of one slot, up to the next slot header.
func sectionOf(long, slot string) string {
	lines := strings.Split(long, "\n")
	start := -1
	for i, l := range lines {
		if l == slot {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	for i := start; i < len(lines); i++ {
		for _, other := range requiredSlots {
			if lines[i] == other {
				return strings.Join(lines[start:i], "\n")
			}
		}
	}
	return strings.Join(lines[start:], "\n")
}

// flagNamesIn pulls every --long-flag out of a chunk of help text.
func flagNamesIn(text string) []string {
	var out []string
	for _, field := range strings.Fields(text) {
		if !strings.HasPrefix(field, "--") || len(field) < 3 {
			continue
		}
		name := strings.TrimPrefix(field, "--")
		// Trim the punctuation a sentence leaves behind: `--all,` `--tags.` `--ids"`
		name = strings.TrimRight(name, ".,;:\"')")
		if name == "" || strings.ContainsAny(name, "<>|") {
			continue
		}
		out = append(out, name)
	}
	return out
}
