// roleguard_test.go pins the sandboxed-role guard of the fabric command tree:
// which subcommands a task-session role may run.
// It also pins which strand names the guard lets through.
// It pins which directories a task role may run a mutating verb in.
// It drives the built command tree against directories that hold no hub.
// A passing verb stops at its own worktree-resolution error and spawns nothing.

package fabriccli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
	"github.com/spf13/cobra"
)

// guardVerb is a registered fabric subcommand, the arguments that satisfy its arity and whether the guard classes it a reader.
type guardVerb struct {
	args   []string
	reader bool
}

// guardVerbs pins every registered subcommand: a verb added later fails the walk until it is classified here.
var guardVerbs = map[string]guardVerb{
	"list":      {args: []string{"list"}, reader: true},
	"pairs":     {args: []string{"pairs"}, reader: true},
	"status":    {args: []string{"status"}, reader: true},
	"diff":      {args: []string{"diff", "abc"}, reader: true},
	"shortname": {args: []string{"shortname"}, reader: true},
	"prune":     {args: []string{"prune"}, reader: true},
	"cleanup":   {args: []string{"cleanup"}, reader: true},
	"clone":     {args: []string{"clone", "https://example.invalid/w"}},
	"add":       {args: []string{"add", "slug"}},
	"remove":    {args: []string{"remove", "slug"}},
	// The hidden teardown child passes by name, since it inherits the removal's strand name; the walk counts it a reader.
	"remote-teardown": {args: []string{"remote-teardown"}, reader: true},
	"checkout":        {args: []string{"checkout"}},
	"reconcile":       {args: []string{"reconcile"}},
	"anchor":          {args: []string{"anchor"}},
	"unwire":          {args: []string{"unwire"}},
	"commit":          {args: []string{"commit"}},
	"push":            {args: []string{"push"}},
	"pull":            {args: []string{"pull"}},
	"sync":            {args: []string{"sync"}},
	"merge":           {args: []string{"merge", "branch"}},
	"merge-in":        {args: []string{"merge-in", "branch"}},
	"merge-stage":     {args: []string{"merge-stage", "path"}},
}

// refusalMarker is the text every guard refusal carries.
const refusalMarker = "from the task session"

// runUnderParent mounts the fabric command under a bare parent as main.go does and runs args from an empty directory.
func runUnderParent(t *testing.T, args ...string) (code int, out string) {
	t.Helper()
	return runUnderParentIn(t, t.TempDir(), args...)
}

// runUnderParentIn mounts the fabric command under a bare parent as main.go does and runs args from dir.
func runUnderParentIn(t *testing.T, dir string, args ...string) (code int, out string) {
	t.Helper()
	root := &cobra.Command{Use: "lym"}
	root.AddCommand(fabriccli.Command())
	var buf bytes.Buffer
	code = clihelp.ExecuteIn(root, dir, &buf, append([]string{"fabric"}, args...))
	return code, buf.String()
}

// fakeWorktree makes a directory named name under parent that resolves as a worktree root:
// its `.git` holds HEAD, objects and refs, written with no git process.
func fakeWorktree(t *testing.T, parent, name string) string {
	t.Helper()
	worktree := filepath.Join(parent, name)
	for _, dir := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(worktree, ".git", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return worktree
}

// sandboxHubDir makes a hub directory under the sandbox root of worktree and returns it as written through the worktree.
// The worktree's `.lym` is a directory link into its records sibling, as fabric wires it,
// so the directory itself lives under the sibling.
func sandboxHubDir(t *testing.T, worktree string) string {
	t.Helper()
	sibling := worktree + weftname.Suffix
	if err := os.MkdirAll(filepath.Join(sibling, lymdirs.DotLymDirName, lymdirs.HardenSandboxDirName, "hub-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(worktree, lymdirs.DotLymDirName)
	if _, err := os.Lstat(link); err != nil {
		if err := fslink.CreateDirLink(link, filepath.Join(sibling, lymdirs.DotLymDirName)); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(link, lymdirs.HardenSandboxDirName, "hub-1")
}

// recordShortname writes the repo shortname record of the hub directory hub.
func recordShortname(t *testing.T, hub, shortname string) {
	t.Helper()
	boltDir := fabricengine.BoltDir(hub)
	if err := os.MkdirAll(boltDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(boltDir, fabricengine.ShortnameFileName), []byte(shortname+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// linkTo makes a directory link in a new temp directory that points at target, and returns the link.
func linkTo(t *testing.T, target string) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "link")
	if err := fslink.CreateDirLink(link, target); err != nil {
		t.Fatal(err)
	}
	return link
}

// TestSandboxedRoleGuard sets the strand-name environment variable with t.Setenv.
// That variable is process-global, so the test cannot call t.Parallel.
func TestSandboxedRoleGuard(t *testing.T) {
	tiergate.Require(t, 0)
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
					for _, want := range []string{"hubforge", "status: FAILED", "`lym harden sandbox new` under "} {
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

	t.Run("a merge task's writer runs the merge verbs and no other mutating form", func(t *testing.T) {
		mergeVerbs := [][]string{{"merge-in", "main"}, {"merge-stage", "path"}, {"merge", "--continue"}, {"merge", "--abort"}}
		refusedForms := [][]string{{"merge", "branch"}, {"push"}, {"commit"}}

		for _, strand := range []string{"ly:beam-merge-in:darn", "ly:beam-merge-in-2:darn"} {
			t.Setenv(agentname.StrandNameEnv, strand)
			for _, args := range mergeVerbs {
				if _, out := runUnderParent(t, args...); strings.Contains(out, refusalMarker) {
					t.Errorf("%v under %s was refused: %s", args, strand, out)
				}
			}
			for _, args := range refusedForms {
				if code, out := runUnderParent(t, args...); code != 1 || !strings.Contains(out, refusalMarker) {
					t.Errorf("%v under %s = %d, output %s; want a guard refusal", args, strand, code, out)
				}
			}
		}
		for _, strand := range []string{"ly:beam-task:darn", "ly:beam-merge-in:code-fix"} {
			t.Setenv(agentname.StrandNameEnv, strand)
			for _, args := range mergeVerbs {
				if code, out := runUnderParent(t, args...); code != 1 || !strings.Contains(out, refusalMarker) {
					t.Errorf("%v under %s = %d, output %s; want a guard refusal", args, strand, code, out)
				}
			}
		}
	})

	t.Run("a push's path flags pass only for the role's own pair or a pair under its own sandbox root", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, webster)
		hubs := t.TempDir()
		recordShortname(t, hubs, "ly")
		own := fakeWorktree(t, hubs, "slug")
		ownSandboxHub := sandboxHubDir(t, own)
		other := fakeWorktree(t, hubs, "other")
		otherHubs := t.TempDir()
		recordShortname(t, otherHubs, "zz")
		sameSlugInOtherHub := fakeWorktree(t, otherHubs, "slug")

		tests := []struct {
			name     string
			weftPath string
			warpPath string
			refused  bool
		}{
			{"OwnPair", own + weftname.Suffix, own, false},
			{"OwnRecordsSiblingAlone", own + weftname.Suffix, "", false},
			{"PairUnderOwnSandboxRoot", filepath.Join(ownSandboxHub, "t1"+weftname.Suffix), filepath.Join(ownSandboxHub, "t1"), false},
			{"AnotherWorktreesPair", other + weftname.Suffix, other, true},
			{"OwnRecordsSiblingWithAnotherWorktree", own + weftname.Suffix, other, true},
			{"SameSlugPairInAnotherHub", sameSlugInOtherHub + weftname.Suffix, sameSlugInOtherHub, true},
			{"PathsOutsideAnyHub", t.TempDir(), t.TempDir(), true},
		}
		for _, tt := range tests {
			args := []string{"push"}
			if tt.weftPath != "" {
				args = append(args, "--weft-path", tt.weftPath)
			}
			if tt.warpPath != "" {
				args = append(args, "--warp-path", tt.warpPath)
			}
			code, out := runUnderParent(t, args...)
			if refused := strings.Contains(out, refusalMarker); refused != tt.refused {
				t.Errorf("%s: %v under %s refused = %v; want %v; output: %s", tt.name, args, webster, refused, tt.refused, out)
			}
			if tt.refused && code != 1 {
				t.Errorf("%s: %v under %s = %d; want exit 1", tt.name, args, webster, code)
			}
		}

		for _, args := range [][]string{{"add", "slug", "--warp-path", "x"}, {"clone", "u", "--warp-path", "x"}, {"status", "--weft-path", "x"}} {
			if code, out := runUnderParent(t, args...); code != 1 || !strings.Contains(out, refusalMarker) {
				t.Errorf("%v under %s = %d, output %s; want a guard refusal", args, webster, code, out)
			}
		}
	})

	t.Run("a task role runs mutating verbs only in a hub under its own worktree's sandbox root", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, webster)
		hubs := t.TempDir()
		recordShortname(t, hubs, "ly")
		own := fakeWorktree(t, hubs, "slug")
		other := fakeWorktree(t, hubs, "other")
		notWorktree := filepath.Join(t.TempDir(), "slug")
		ownSandboxHub := sandboxHubDir(t, own)
		ownSandboxRoot := filepath.Dir(ownSandboxHub)
		otherHubs := t.TempDir()
		recordShortname(t, otherHubs, "zz")
		sameSlugInOtherHub := fakeWorktree(t, otherHubs, "slug")

		nestedHub := filepath.Join(own, "sub", lymdirs.DotLymDirName, lymdirs.HardenSandboxDirName, "hub-1")
		if err := os.MkdirAll(nestedHub, 0o755); err != nil {
			t.Fatal(err)
		}
		escape := filepath.Join(ownSandboxRoot, "escape")
		if err := fslink.CreateDirLink(escape, t.TempDir()); err != nil {
			t.Fatal(err)
		}
		dangling := filepath.Join(ownSandboxRoot, "dangling")
		if err := fslink.CreateDirLink(dangling, filepath.Join(t.TempDir(), "absent")); err != nil {
			t.Fatal(err)
		}

		// A worktree whose records sibling holds a `.lym` with no sandbox root yet.
		freshHubs := t.TempDir()
		recordShortname(t, freshHubs, "ly")
		fresh := fakeWorktree(t, freshHubs, "slug")
		if err := os.MkdirAll(filepath.Join(fresh+weftname.Suffix, lymdirs.DotLymDirName), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := fslink.CreateDirLink(filepath.Join(fresh, lymdirs.DotLymDirName), filepath.Join(fresh+weftname.Suffix, lymdirs.DotLymDirName)); err != nil {
			t.Fatal(err)
		}

		// Each form fails at its own worktree resolution once the guard lets it through, so none spawns;
		// clone's three URLs stop it at its usage check.
		mutatingForms := [][]string{
			{"add", "slug"}, {"push"}, {"pull"}, {"sync"}, {"reconcile"}, {"checkout"},
			{"remove", "slug"}, {"cleanup", "--apply"}, {"clone", "a", "b", "c"},
		}
		tests := []struct {
			name    string
			cwd     string
			refused bool
		}{
			{"OwnSandboxHub", ownSandboxHub, false},
			{"OwnSandboxHubInItsRecordsSiblingForm", filepath.Join(own+weftname.Suffix, lymdirs.DotLymDirName, lymdirs.HardenSandboxDirName, "hub-1"), false},
			{"LinkIntoOwnSandboxHub", linkTo(t, ownSandboxHub), false},
			{"OtherWorktreesSandboxHub", sandboxHubDir(t, other), true},
			{"SandboxRootOfADirectoryNoWorktree", sandboxHubDir(t, notWorktree), true},
			{"SameSlugWorktreeInAnotherHub", sandboxHubDir(t, sameSlugInOtherHub), true},
			{"OwnWorktreeOutsideItsSandboxRoot", own, true},
			{"SandboxRootBelowASubdirectoryOfOwnWorktree", nestedHub, true},
			{"LinkInsideOwnSandboxRootPointingOut", escape, true},
		}
		for _, tt := range tests {
			for _, args := range mutatingForms {
				code, out := runUnderParentIn(t, tt.cwd, args...)
				if refused := strings.Contains(out, refusalMarker); refused != tt.refused {
					t.Errorf("%s: %v under %s refused = %v; want %v; output: %s", tt.name, args, webster, refused, tt.refused, out)
				}
				if tt.refused && code != 1 {
					t.Errorf("%s: %v under %s = %d; want exit 1", tt.name, args, webster, code)
				}
			}
		}

		cloneTests := []struct {
			name    string
			cwd     string
			into    string
			refused bool
		}{
			{"IntoOwnSandboxRootFromItsWorktree", own, filepath.Join(own, lymdirs.DotLymDirName, lymdirs.HardenSandboxDirName, "hub-2"), false},
			{"RelativeIntoOwnSandboxRootFromItsWorktree", own, filepath.Join(lymdirs.DotLymDirName, lymdirs.HardenSandboxDirName, "hub-3"), false},
			{"IntoAnotherDirectoryFromOwnSandboxHub", ownSandboxHub, hubs, true},
			{"IntoBelowALinkPointingOutOfOwnSandboxRoot", own, filepath.Join(escape, "new"), true},
			{"IntoADanglingLinkInOwnSandboxRoot", own, dangling, true},
			{"IntoOwnSandboxRootThatDoesNotExistYet", fresh, filepath.Join(fresh, lymdirs.DotLymDirName, lymdirs.HardenSandboxDirName, "hub-1"), false},
		}
		for _, tt := range cloneTests {
			_, out := runUnderParentIn(t, tt.cwd, "clone", "--into", tt.into, "a", "b", "c")
			if refused := strings.Contains(out, refusalMarker); refused != tt.refused {
				t.Errorf("%s: clone --into %s refused = %v; want %v; output: %s", tt.name, tt.into, refused, tt.refused, out)
			}
		}

		if code, out := runUnderParentIn(t, ownSandboxHub, "pull", "--weft-path", "/x"); code != 1 || !strings.Contains(out, "refuses the path flags") {
			t.Errorf("pull --weft-path from the own sandbox hub = %d, output %s; want the path-flag refusal", code, out)
		}
	})

	t.Run("ExecuteIn with no parent passes the guard", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, webster)
		var buf bytes.Buffer
		clihelp.ExecuteIn(fabriccli.Command(), t.TempDir(), &buf, []string{"sync"})
		if strings.Contains(buf.String(), refusalMarker) {
			t.Errorf("sync through ExecuteIn was refused: %s", buf.String())
		}
	})
}
