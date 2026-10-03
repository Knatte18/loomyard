// clitree_test.go derives the CLI / Cobra Invariant's per-command obligations from one walk of newRoot(), so no hand-kept list of modules or verbs exists to drift.

package main

import (
	"bytes"
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
// Each is a finding to fix in the group's own CLI and then delete from here; an entry naming a group that no longer exists fails as stale.
var cliTreeFindings = []scankit.Entry{
	{
		Key: "board notes#bare",
		Why: "the alias group refuses with \"not initialized here\" outside a wired fabric instead of listing its verbs",
	},
	{
		Key: "board notes#bogus",
		Why: "emits the \"not initialized here\" envelope before the unknown-subcommand one, so the output holds two documents",
	},
	{
		Key: "config#bare",
		Why: "refuses with \"not a git repository\" instead of listing its verbs",
	},
	{
		Key: "selfreport#bogus",
		Why: "prints help and exits 0 instead of refusing an unknown subcommand",
	},
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
			if findings.Allowed(name + "#bare") {
				t.Skip("known finding")
			}
			var out bytes.Buffer
			code := run(args, &out)
			got := out.String()
			if code != 0 {
				t.Fatalf("run(%v) = %d; want 0 for bare group listing\noutput: %s", args, code, got)
			}
			if strings.Contains(got, `"ok":false`) {
				t.Errorf("run(%v) emitted an error envelope; want plain help text\noutput: %s", args, got)
			}
			for _, child := range children {
				if !strings.Contains(got, child.Name()) {
					t.Errorf("run(%v) listing does not name %q\noutput: %s", args, child.Name(), got)
				}
			}
		})

		t.Run(name+"/bogus", func(t *testing.T) {
			if findings.Allowed(name + "#bogus") {
				t.Skip("known finding")
			}
			var out bytes.Buffer
			bogus := append(append([]string{}, args...), "bogus")
			code := run(bogus, &out)
			if code != 1 {
				t.Errorf("run(%v) = %d; want 1\noutput: %s", bogus, code, out.String())
			}
			envelope.RequireErr(t, out.String(), "")
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
