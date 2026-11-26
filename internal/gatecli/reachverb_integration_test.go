// reachverb_integration_test.go drives `lym gate reach` over a hubforge hub whose prime holds a small Go module.
// The coverage maps are written by hand, so no test builds or runs a coverage binary.
// It is Tier 1: it builds a real hub and spawns git.

package gatecli

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testreach"
)

// reachVerbAnswer is the part of a `lym gate reach` envelope the rows compare.
type reachVerbAnswer struct {
	Ok    bool     `json:"ok"`
	Error string   `json:"error"`
	Lines []string `json:"lines"`
}

// reachVerbGate is the hub's gate.yaml: a go scoper and a tier0 declared at the module root.
const reachVerbGate = "slots: 2\nscoper: go\ntiers:\n" +
	"  tier0: {command: 'true', timeout_min: 1, counter: go-json, parallel: 2, env: {LYM_TEST_TIER: \"0\"}}\n" +
	"gates:\n  darn-write: {tiers: [0], scope: all}\n"

// reachVerbModule is the import path of the fixture module.
const reachVerbModule = "example.com/reached"

func runReachVerb(t *testing.T, cwd string, args ...string) (int, reachVerbAnswer) {
	t.Helper()
	var out bytes.Buffer
	code := clihelp.ExecuteIn(Command(), cwd, &out, append([]string{"reach"}, args...))
	var answer reachVerbAnswer
	if err := json.Unmarshal(out.Bytes(), &answer); err != nil {
		t.Fatalf("lym gate reach %v printed %q; want one JSON envelope: %v", args, out.String(), err)
	}
	return code, answer
}

// seedReachVerbMaps writes the shared maps of the fixture module:
// alpha's TestAlphaOne covers alpha.One.
// beta's TestBetaCalls covers beta.One and reaches alpha.One through its linked functions.
func seedReachVerbMaps(t *testing.T, sharedDir string) {
	t.Helper()
	built := time.Now().Add(-time.Hour)
	alphaOne := testcover.FuncKey{Package: reachVerbModule + "/alpha", File: "alpha.go", Func: "One"}
	maps := map[string]testcover.Map{
		"alpha": {
			Commit: "alphamap", BuiltAt: built,
			Blocks: []testcover.BlockKey{{File: "alpha.go", Func: "One", Hash: "h"}},
			Tests:  []testcover.MapTest{{Name: "TestAlphaOne", FuncHash: "x", Seconds: 0.01, Blocks: []int{0}}},
		},
		"beta": {
			Commit: "betamap", BuiltAt: built,
			Blocks: []testcover.BlockKey{{File: "beta.go", Func: "One", Hash: "h"}},
			Funcs:  []testcover.FuncKey{alphaOne},
			Tests:  []testcover.MapTest{{Name: "TestBetaCalls", FuncHash: "x", Seconds: 0.01, Blocks: []int{0}, Funcs: []int{0}}},
		},
	}
	for pkg, m := range maps {
		if err := testcover.WriteMap(sharedDir, pkg, m); err != nil {
			t.Fatal(err)
		}
	}
}

// landReachVerbChange replaces from with to in the file of pkg, commits the change in root and returns the commit.
func landReachVerbChange(t *testing.T, root, pkg, from, to string) string {
	t.Helper()
	file := filepath.Join(root, pkg, pkg+".go")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(strings.Replace(string(data), from, to, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	gitkit.Git(t, root, "commit", "-q", "-a", "-m", "change "+pkg)
	return gitkit.RevParse(t, root, "HEAD")
}

// readFilesUnder returns the content of every file under dir, by path.
func readFilesUnder(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestGateReach(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	prime, pair := h.PrimeWorktree(), h.PairCodeWorktree(hubforge.TemplatePairSlug)
	seedTestMapModule(t, h, reachVerbModule, reachVerbGate, map[string]map[string]int{
		"alpha": {"TestAlphaOne": 0}, "beta": {"TestBetaCalls": 0}, "gamma": nil,
	})
	seed := gitkit.RevParse(t, prime, "HEAD")
	sharedDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
	seedReachVerbMaps(t, sharedDir)
	// The index is written with its lock file removed again, so a reader that locked it would recreate the file.
	if err := testreach.WriteIndex(testreach.Module{Dir: prime, Rel: ".", Path: reachVerbModule}, sharedDir); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(sharedDir, "index.json")
	if err := os.Remove(lock.TrackedFileLockPath(indexPath, ".lock")); err != nil {
		t.Fatal(err)
	}

	// Three landings on the parent branch: the second changes alpha.One, which the test reaches.
	landingGammaOne := landReachVerbChange(t, prime, "gamma", "return 1 }", "return 9 }")
	landingAlphaOne := landReachVerbChange(t, prime, "alpha", "return 1 }", "return 9 }")
	landingGammaTwo := landReachVerbChange(t, prime, "gamma", "return 2 }", "return 8 }")
	// A side branch off the seed holds one landing that changes alpha.One too.
	gitkit.Git(t, prime, "checkout", "-q", "-b", "side", seed)
	sideLanding := landReachVerbChange(t, prime, "alpha", "return 1 }", "return 7 }")
	gitkit.Git(t, prime, "checkout", "-q", "main")
	gitkit.Git(t, pair, "merge", "--ff-only", "-q", "main")
	short := func(sha string) string { return sha[:reachShortSHALength] }

	const alphaTest = "alpha.TestAlphaOne"
	tests := []struct {
		name      string
		cwd       string
		args      []string
		wantExit  int
		wantError []string
		wantLines []string
		notLines  []string
	}{
		{
			name: "BothAFunctionAndTest", cwd: prime, wantExit: 1,
			args:      []string{"alpha/alpha.go#One", "--test", alphaTest, "--since", seed},
			wantError: []string{reachFunctionForm, reachTestForm},
		},
		{
			name: "NeitherAFunctionNorTest", cwd: prime, wantExit: 1,
			wantError: []string{reachFunctionForm, reachTestForm},
		},
		{
			name: "SinceWithoutTest", cwd: prime, wantExit: 1,
			args:      []string{"alpha/alpha.go#One", "--since", seed},
			wantError: []string{reachFunctionForm, reachTestForm},
		},
		{
			name: "BranchWithoutTest", cwd: prime, wantExit: 1,
			args:      []string{"alpha/alpha.go#One", "--branch", "side"},
			wantError: []string{reachFunctionForm, reachTestForm},
		},
		{
			name: "TestWithoutSince", cwd: prime, wantExit: 1,
			args:      []string{"--test", alphaTest},
			wantError: []string{"--since", "last commit you know was green"},
		},
		{
			name: "TestWithNoDot", cwd: prime, wantExit: 1,
			args:      []string{"--test", "TestAlphaOne", "--since", seed},
			wantError: []string{"<package>.<Test>"},
		},
		{
			name: "FunctionWithNoHash", cwd: prime, wantExit: 1,
			args:      []string{"alpha/alpha.go"},
			wantError: []string{"<file>#<Func>"},
		},
		{
			name: "BranchNamingNoBranch", cwd: prime, wantExit: 1,
			args:      []string{"--test", alphaTest, "--since", seed, "--branch", "no-such-branch"},
			wantError: []string{`"no-such-branch" names no branch`, "drop --branch"},
		},
		{
			name: "SinceResolvingToNoCommit", cwd: prime, wantExit: 1,
			args:      []string{"--test", alphaTest, "--since", "no-such-commit"},
			wantError: []string{"names no commit", "pass a commit SHA or a ref that names a commit"},
		},
		{
			name: "SinceNotOnTheBranchesFirstParentHistory", cwd: prime, wantExit: 1,
			args:      []string{"--test", alphaTest, "--since", sideLanding},
			wantError: []string{"not on the first-parent history of main"},
		},
		{
			name: "OutsideAHub", cwd: t.TempDir(), wantExit: 1,
			args:      []string{"alpha/alpha.go#One"},
			wantError: []string{"in no hub", "worktree of the hub"},
		},
		{
			name: "AFunctionReachedByTestsOfTwoPackagesListsTheOwnPackageFirstWithTheMapCommit", cwd: prime,
			args: []string{"alpha/alpha.go#One"},
			wantLines: []string{
				"reach: alpha.One: TestAlphaOne, beta.TestBetaCalls",
				"map of alpha from commit alphamap",
				"map of beta from commit betamap",
			},
		},
		{
			name: "AFunctionNoMappedTestReachesNamesItsPackagesMap", cwd: prime,
			args:      []string{"alpha/alpha.go#Two"},
			wantLines: []string{"no mapped test reaches alpha.Two", "map of alpha from commit alphamap"},
			notLines:  []string{"no coverage map holds"},
		},
		{
			name: "AFunctionOfAPackageWithNoMapSaysNoMapHoldsIt", cwd: prime,
			args:      []string{"gamma/gamma.go#One"},
			wantLines: []string{"no mapped test reaches gamma.One", "no coverage map holds the package gamma"},
		},
		{
			name: "ATestListsTheOneLandingWhoseChangedFunctionItReaches", cwd: prime,
			args: []string{"--test", alphaTest, "--since", seed},
			wantLines: []string{
				"commits checked: 3", "on main", "the map of alpha is from commit alphamap", short(landingAlphaOne) + " changes alpha.One",
			},
			notLines: []string{short(landingGammaOne), short(landingGammaTwo), short(sideLanding)},
		},
		{
			name: "ASinceGivenAsAShortSHAWalksTheSameRange", cwd: prime,
			args:      []string{"--test", alphaTest, "--since", short(seed)},
			wantLines: []string{"commits checked: 3", short(landingAlphaOne) + " changes alpha.One"},
			notLines:  []string{short(landingGammaOne), short(landingGammaTwo)},
		},
		{
			name: "APairWalksItsParentBranch", cwd: pair,
			args:      []string{"--test", alphaTest, "--since", seed},
			wantLines: []string{"commits checked: 3", "on main", short(landingAlphaOne) + " changes alpha.One"},
		},
		{
			name: "TheNamedBranchIsWalkedInsteadOfTheParentBranch", cwd: prime,
			args:      []string{"--test", alphaTest, "--since", seed, "--branch", "side"},
			wantLines: []string{"commits checked: 1", "on side", short(sideLanding) + " changes alpha.One"},
			notLines:  []string{short(landingAlphaOne)},
		},
		{
			name: "ATestNoMapHoldsWalksNothingAndNamesTheMapsRead", cwd: prime,
			args:      []string{"--test", "alpha.TestNope", "--since", seed},
			wantLines: []string{"no coverage map holds alpha.TestNope", "alphamap", "betamap"},
			notLines:  []string{"commits checked", "changes"},
		},
	}
	before := readFilesUnder(t, sharedDir)
	t.Run("Verb", func(t *testing.T) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				code, answer := runReachVerb(t, tt.cwd, tt.args...)
				if code != tt.wantExit || answer.Ok != (tt.wantExit == 0) {
					t.Fatalf("lym gate reach %v = exit %d, ok %v, error %q; want exit %d", tt.args, code, answer.Ok, answer.Error, tt.wantExit)
				}
				for _, want := range tt.wantError {
					if !strings.Contains(answer.Error, want) || !strings.Contains(answer.Error, "way forward:") {
						t.Errorf("lym gate reach %v error = %q; want one naming %q and its way forward", tt.args, answer.Error, want)
					}
				}
				lines := strings.Join(answer.Lines, "\n")
				for _, want := range tt.wantLines {
					if !strings.Contains(lines, want) {
						t.Errorf("lym gate reach %v lines = %q; want one holding %q", tt.args, lines, want)
					}
				}
				for _, unwanted := range tt.notLines {
					if strings.Contains(lines, unwanted) {
						t.Errorf("lym gate reach %v lines = %q; want none holding %q", tt.args, lines, unwanted)
					}
				}
			})
		}
	})
	if diff := cmp.Diff(before, readFilesUnder(t, sharedDir)); diff != "" {
		t.Errorf("the maps directory changed (-before +after):\n%s", diff)
	}

	t.Run("ARangeOverTheRequestsLimitIsRefusedNamingALaterSince", func(t *testing.T) {
		t.Parallel()
		call := func(limit int) (int, reachVerbAnswer) {
			var out bytes.Buffer
			code := runReach(lymcwd.WithCwd(context.Background(), prime), &out, reachRequest{test: alphaTest, since: seed, limit: limit})
			var answer reachVerbAnswer
			if err := json.Unmarshal(out.Bytes(), &answer); err != nil {
				t.Fatalf("runReach printed %q; want one JSON envelope: %v", out.String(), err)
			}
			return code, answer
		}
		code, answer := call(2)
		if code != 1 || !strings.Contains(answer.Error, "more than 2 commits") || !strings.Contains(answer.Error, "later --since") {
			t.Errorf("runReach(limit 2) = exit %d, error %q; want a refusal naming the cap of 2 and a later --since", code, answer.Error)
		}
		if code, answer := call(3); code != 0 || !strings.Contains(strings.Join(answer.Lines, "\n"), "commits checked: 3") {
			t.Errorf("runReach(limit 3) = exit %d, lines %q; want the three landings checked", code, answer.Lines)
		}
	})

	t.Run("AHubWhoseScoperIsNotGoPrintsItsNote", func(t *testing.T) {
		t.Parallel()
		other := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		otherPrime := seedTestMapModule(t, other, reachVerbModule, strings.Replace(reachVerbGate, "scoper: go", "scoper: none", 1),
			map[string]map[string]int{"alpha": {"TestAlphaOne": 0}})
		code, answer := runReachVerb(t, otherPrime, "alpha/alpha.go#One")
		if code != 0 || !strings.Contains(strings.Join(answer.Lines, "\n"), "the hub's gate.scoper is none, not go") {
			t.Errorf("lym gate reach = exit %d, lines %q; want exit 0 and a note naming the scoper", code, answer.Lines)
		}
	})
}
