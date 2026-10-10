// roleguard_test.go pins the sandboxed-role guard of the fabric command tree: which subcommands a task-session role may run, and which strand names the guard lets through.
// It drives the built command tree against an empty directory, so a passing verb stops at its own worktree-resolution error and spawns nothing.

package fabriccli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/spf13/cobra"
)

// guardVerb is a registered fabric subcommand, the arguments that satisfy its arity and whether the guard classes it a reader.
type guardVerb struct {
	args   []string
	reader bool
}

// guardVerbs pins every registered subcommand: a verb added later fails the walk until it is classified here.
var guardVerbs = map[string]guardVerb{
	"list":        {args: []string{"list"}, reader: true},
	"pairs":       {args: []string{"pairs"}, reader: true},
	"status":      {args: []string{"status"}, reader: true},
	"diff":        {args: []string{"diff", "abc"}, reader: true},
	"shortname":   {args: []string{"shortname"}, reader: true},
	"prune":       {args: []string{"prune"}, reader: true},
	"cleanup":     {args: []string{"cleanup"}, reader: true},
	"clone":       {args: []string{"clone", "https://example.invalid/w"}},
	"add":         {args: []string{"add", "slug"}},
	"remove":      {args: []string{"remove", "slug"}},
	"checkout":    {args: []string{"checkout"}},
	"reconcile":   {args: []string{"reconcile"}},
	"unwire":      {args: []string{"unwire"}},
	"commit":      {args: []string{"commit"}},
	"push":        {args: []string{"push"}},
	"pull":        {args: []string{"pull"}},
	"sync":        {args: []string{"sync"}},
	"merge":       {args: []string{"merge", "branch"}},
	"merge-in":    {args: []string{"merge-in", "branch"}},
	"merge-stage": {args: []string{"merge-stage", "path"}},
}

// refusalMarker is the text every guard refusal carries.
const refusalMarker = "from the task session"

// runUnderParent mounts the fabric command under a bare parent as main.go does and runs args from an empty directory.
func runUnderParent(t *testing.T, args ...string) (code int, out string) {
	t.Helper()
	root := &cobra.Command{Use: "lyx"}
	root.AddCommand(fabriccli.Command())
	var buf bytes.Buffer
	code = clihelp.ExecuteIn(root, t.TempDir(), &buf, append([]string{"fabric"}, args...))
	return code, buf.String()
}

// TestSandboxedRoleGuard sets the strand name with t.Setenv, so it names that process-global variable in place of t.Parallel.
func TestSandboxedRoleGuard(t *testing.T) {
	const (
		webster  = "ly:slug:webster"
		recovery = "ly:slug:recovery"
		conflict = "ly:slug:conflict"
	)
	// passingNames are the strand names the guard lets run every verb.
	passingNames := []string{"ly:slug:driver", "ly:slug:driver-2", "ly:slug:orch", "ly:orch", "ly:slug", "not a name", ""}

	t.Run("every subcommand is a pinned reader or refused, and --help always passes", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, webster)

		for _, sub := range fabriccli.Command().Commands() {
			if name := sub.Name(); name == "help" || name == "completion" {
				continue
			}
			verb, ok := guardVerbs[sub.Name()]
			if !ok {
				t.Errorf("subcommand %q is not pinned in guardVerbs; classify it as a reader or refused", sub.Name())
				continue
			}
			code, out := runUnderParent(t, verb.args...)
			if verb.reader == strings.Contains(out, refusalMarker) {
				t.Errorf("%s under %s: guard refused = %v; want %v; output: %s", sub.Name(), webster, !verb.reader, !verb.reader, out)
			}
			if !verb.reader && code != 1 {
				t.Errorf("%s under %s = %d; want exit 1", sub.Name(), webster, code)
			}
			if _, helpOut := runUnderParent(t, append([]string{sub.Name()}, "--help")...); strings.Contains(helpOut, refusalMarker) {
				t.Errorf("%s --help under %s was refused: %s", sub.Name(), webster, helpOut)
			}
		}
		for name := range guardVerbs {
			if _, _, err := fabriccli.Command().Find([]string{name}); err != nil {
				t.Errorf("guardVerbs pins %q, which is not a registered subcommand: %v", name, err)
			}
		}
	})

	t.Run("every fabric reader verb is a guard reader", func(t *testing.T) {
		for _, name := range fabricengine.FabricReaderVerbs() {
			if verb, ok := guardVerbs[name]; !ok || !verb.reader {
				t.Errorf("fabricengine reader verb %q is not a guard reader", name)
			}
		}
	})

	t.Run("add clone push and sync refuse under task roles and run on otherwise", func(t *testing.T) {
		for _, verb := range []string{"add", "clone", "push", "sync"} {
			for _, strand := range []string{webster, recovery, conflict} {
				t.Setenv(agentname.StrandNameEnv, strand)
				code, out := runUnderParent(t, guardVerbs[verb].args...)
				if code != 1 || !strings.Contains(out, refusalMarker) {
					t.Errorf("%s under %s = %d, output %s; want a guard refusal", verb, strand, code, out)
				}
				if verb == "add" && strand == webster {
					for _, want := range []string{"hubforge", "status: FAILED"} {
						if !strings.Contains(out, want) {
							t.Errorf("add refusal under %s lacks %q: %s", strand, want, out)
						}
					}
				}
			}
			for _, strand := range passingNames {
				t.Setenv(agentname.StrandNameEnv, strand)
				if _, out := runUnderParent(t, guardVerbs[verb].args...); strings.Contains(out, refusalMarker) {
					t.Errorf("%s under %q was refused: %s", verb, strand, out)
				}
			}
		}
	})

	t.Run("reader forms and add --help pass under every name", func(t *testing.T) {
		for _, strand := range append([]string{webster, recovery, conflict}, passingNames...) {
			t.Setenv(agentname.StrandNameEnv, strand)
			for _, args := range [][]string{{"list"}, {"pairs"}, {"shortname"}, {"prune"}, {"cleanup"}, {"add", "--help"}} {
				if _, out := runUnderParent(t, args...); strings.Contains(out, refusalMarker) {
					t.Errorf("%v under %q was refused: %s", args, strand, out)
				}
			}
		}
	})

	t.Run("a mutating form of a reader verb refuses under a task role", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, webster)
		for _, args := range [][]string{{"prune", "--apply"}, {"cleanup", "--apply"}, {"shortname", "xy"}} {
			if code, out := runUnderParent(t, args...); code != 1 || !strings.Contains(out, refusalMarker) {
				t.Errorf("%v under %s = %d, output %s; want a guard refusal", args, webster, code, out)
			}
		}
	})

	t.Run("a bypass flag passes only on push", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, webster)
		if _, out := runUnderParent(t, "push", "--weft-path", t.TempDir()); strings.Contains(out, refusalMarker) {
			t.Errorf("push with --weft-path was refused: %s", out)
		}
		for _, args := range [][]string{{"add", "slug", "--warp-path", "x"}, {"clone", "u", "--warp-path", "x"}, {"status", "--weft-path", "x"}} {
			if code, out := runUnderParent(t, args...); code != 1 || !strings.Contains(out, refusalMarker) {
				t.Errorf("%v under %s = %d, output %s; want a guard refusal", args, webster, code, out)
			}
		}
	})

	t.Run("RunCLIIn with no parent passes the guard", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, webster)
		var buf bytes.Buffer
		fabriccli.RunCLIIn(t.TempDir(), &buf, []string{"sync"})
		if strings.Contains(buf.String(), refusalMarker) {
			t.Errorf("sync through RunCLIIn was refused: %s", buf.String())
		}
	})
}
