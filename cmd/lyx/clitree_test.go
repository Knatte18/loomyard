// clitree_test.go derives the CLI / Cobra Invariant's per-command obligations from one walk of newRoot(), so no hand-kept list of modules or verbs exists to drift.

package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// walkCommands calls fn on cmd and every descendant, skipping cobra's help and completion subtrees.
func walkCommands(cmd *cobra.Command, fn func(*cobra.Command)) {
	if name := cmd.Name(); name == "help" || name == "completion" {
		return
	}
	fn(cmd)
	for _, child := range cmd.Commands() {
		walkCommands(child, fn)
	}
}

// visibleChildren returns the non-hidden children of cmd, without cobra's help and completion.
func visibleChildren(cmd *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, child := range cmd.Commands() {
		if name := child.Name(); name == "help" || name == "completion" || child.Hidden {
			continue
		}
		out = append(out, child)
	}
	return out
}

// argsFor returns the argument vector that addresses cmd from the root.
func argsFor(cmd *cobra.Command) []string {
	return strings.Fields(cmd.CommandPath())[1:]
}

// cliTreeFindings are groups whose bare or bogus invocation differs from the invariant today.
// Each is a finding to fix in the group's own CLI and then delete from here.
// An entry fails once its invocation meets the invariant, and an entry naming a group that no longer exists fails as stale.
var cliTreeFindings = []scankit.Entry{
	{
		Key: "config#bare",
		Why: "refuses with \"not a git repository\" instead of listing its verbs",
	},
	{
		Key: "config#bogus",
		Why: "refuses with \"not a git repository\" before reaching the unknown-subcommand refusal",
	},
	{
		Key: "selfreport#bogus",
		Why: "prints help and exits 0 instead of refusing an unknown subcommand",
	},
}

// bareProblems runs a bare group invocation and returns how it breaks the invariant, with its output.
func bareProblems(args []string, children []*cobra.Command) ([]string, string) {
	var out bytes.Buffer
	code := run(args, &out)
	got := out.String()
	var problems []string
	if code != 0 {
		problems = append(problems, fmt.Sprintf("exit %d; want 0 for bare group listing", code))
	}
	if strings.Contains(got, `"ok":false`) {
		problems = append(problems, "emitted an error envelope; want plain help text")
	}
	for _, child := range children {
		if !strings.Contains(got, child.Name()) {
			problems = append(problems, fmt.Sprintf("listing does not name %q", child.Name()))
		}
	}
	return problems, got
}

// bogusProblems runs an unknown-subcommand invocation and returns how it breaks the invariant, with its output.
func bogusProblems(args []string) ([]string, string) {
	var out bytes.Buffer
	code := run(args, &out)
	got := out.String()
	var problems []string
	if code != 1 {
		problems = append(problems, fmt.Sprintf("exit %d; want 1", code))
	}
	env, err := envelope.Parse(got)
	switch {
	case err != nil:
		problems = append(problems, err.Error())
	case env.OK:
		problems = append(problems, "ok = true; want an error envelope")
	case !strings.Contains(env.Error, "unknown subcommand"):
		problems = append(problems, fmt.Sprintf("error %q does not name an unknown subcommand", env.Error))
	}
	return problems, got
}

// checkInvocation fails t for every problem, unless the invocation is a known finding, which fails only once its problems are gone.
func checkInvocation(t *testing.T, args []string, known bool, problems []string, out string) {
	t.Helper()
	switch {
	case known && len(problems) == 0:
		t.Errorf("run(%v) now meets the invariant; remove its cliTreeFindings entry", args)
	case known:
		t.Skipf("known finding: %s", strings.Join(problems, "; "))
	case len(problems) > 0:
		t.Errorf("run(%v): %s\noutput: %s", args, strings.Join(problems, "; "), out)
	}
}

// TestCLITree_EveryCommand walks newRoot() from a cwd that is not a git repository.
func TestCLITree_EveryCommand(t *testing.T) {
	t.Chdir(t.TempDir())

	findings := scankit.NewAllowlist(cliTreeFindings)
	root := newRoot()
	walked := 0
	walkCommands(root, func(cmd *cobra.Command) {
		walked++
		if cmd.Short == "" {
			t.Errorf("command %q has no Short description", cmd.CommandPath())
		}

		children := visibleChildren(cmd)
		if len(children) == 0 || cmd == root {
			return
		}
		args := argsFor(cmd)

		name := strings.Join(args, " ")

		t.Run(name+"/bare", func(t *testing.T) {
			problems, out := bareProblems(args, children)
			checkInvocation(t, args, findings.Allowed(name+"#bare"), problems, out)
		})

		t.Run(name+"/bogus", func(t *testing.T) {
			bogus := append(append([]string{}, args...), "bogus")
			problems, out := bogusProblems(bogus)
			checkInvocation(t, bogus, findings.Allowed(name+"#bogus"), problems, out)
		})
	})
	if walked < 2 {
		t.Fatalf("walked %d commands; the tree walk is not reaching the mounted modules", walked)
	}
	findings.RequireNoStale(t)
}

// TestCLITree_RootLongNamesEveryModule asserts root.Long names every mounted module.
func TestCLITree_RootLongNamesEveryModule(t *testing.T) {
	root := newRoot()
	children := visibleChildren(root)
	if len(children) == 0 {
		t.Fatal("newRoot() mounts no modules")
	}
	for _, child := range children {
		if !strings.Contains(root.Long, child.Name()) {
			t.Errorf("root.Long does not name mounted module %q; add it to the Available modules list in newRoot()", child.Name())
		}
	}
}
