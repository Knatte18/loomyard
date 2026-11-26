// mutate_integration_test.go drives `lym gate mutate` over hubforge hubs whose prime holds a small Go module.
// The module's coverage maps are built by the real `lym gate test --map`.
// Its mutants are built and run by the real go command.
// It is Tier 1: it builds real hubs, spawns git and builds and runs real binaries.

package gatecli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// mutateModule is the import path of the fixture module.
// It is the import path a coverage map reads tiers from, so the module builds alone beside its own stand-in tiergate.
const mutateModule = "github.com/Knatte18/loomyard"

// mutateSupportFiles are the fixture module's files that no landing changes.
// calc holds the functions each landing changes, and its tests check only some of them.
// TestScaleRuns runs Scale and checks nothing, and TestOtherCalls of package other reaches it too.
// Nothing runs Spare.
// Tally has a dropped decrement that makes its loop endless.
// Hoard has a negated loop condition that keeps memory for ever.
// Each yields a mutant that ends unjudged.
var mutateSupportFiles = map[string]string{
	"internal/testkit/tiergate/tiergate.go": "package tiergate\n\nimport \"testing\"\n\n// Require stands in for the tier gate: every test runs.\nfunc Require(t testing.TB, tier int) {}\n",
	"calc/calc_test.go": `package calc

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestClampChecked(t *testing.T) {
	tiergate.Require(t, 0)
	if Clamp(-1) != 0 || Clamp(5) != 5 {
		t.Fatal("Clamp is wrong")
	}
}

func TestClampCheckedAgain(t *testing.T) {
	tiergate.Require(t, 0)
	if Clamp(-1) != 0 || Clamp(5) != 5 {
		t.Fatal("Clamp is wrong")
	}
}

func TestScaleRuns(t *testing.T) {
	tiergate.Require(t, 0)
	Scale(2)
	Scale(200)
}

func TestTallyChecked(t *testing.T) {
	tiergate.Require(t, 0)
	if Tally(3) != 3 {
		t.Fatal("Tally(3) != 3")
	}
}

func TestHoardChecked(t *testing.T) {
	tiergate.Require(t, 0)
	if Hoard(0) != 0 {
		t.Fatal("Hoard(0) != 0")
	}
}
`,
	"other/other.go": "package other\n\nimport \"github.com/Knatte18/loomyard/calc\"\n\nfunc Run() int { return calc.Scale(1) }\n",
	"other/other_test.go": `package other

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestOtherCalls(t *testing.T) {
	tiergate.Require(t, 0)
	Run()
}
`,
}

// Indexes of the functions of calc in mutateCalcFunctions, in the order calc.go declares them.
const (
	calcClamp = iota
	calcScale
	calcTally
	calcHoard
	calcSpare
)

// mutateCalcFunctions holds the seed text and the landed text of each function of calc.
// The seed text stands where a function has not landed yet, and every landed text changes lines.
var mutateCalcFunctions = [...]struct{ seed, landed string }{
	calcClamp: {
		"func Clamp(n int) int {\n\treturn n\n}\n",
		"func Clamp(n int) int {\n\tif n < 0 {\n\t\treturn 0\n\t}\n\treturn n\n}\n",
	},
	calcScale: {
		"func Scale(n int) int {\n\treturn n\n}\n",
		"func Scale(n int) int {\n\tlimit := 100\n\tif n > limit {\n\t\treturn limit\n\t}\n\treturn n\n}\n",
	},
	calcTally: {
		"func Tally(n int) int {\n\treturn n\n}\n",
		"func Tally(n int) int {\n\tsteps := 0\n\tfor n > 0 {\n\t\tn -= 1\n\t\tsteps += 1\n\t}\n\treturn steps\n}\n",
	},
	calcHoard: {
		"func Hoard(n int) int {\n\treturn n\n}\n",
		"const hoardStop = 384\n\nfunc Hoard(n int) int {\n\tvar kept [][]byte\n\tfor i := 1; i <= n; i++ {\n" +
			"\t\tif len(kept) == hoardStop {\n\t\t\tpanic(\"Hoard kept hoardStop megabytes: no memory cap ended the run\")\n\t\t}\n" +
			"\t\tblock := make([]byte, 1<<20)\n\t\tfor at := range block {\n\t\t\tblock[at] = 1\n\t\t}\n\t\tkept = append(kept, block)\n\t}\n" +
			"\treturn len(kept)\n}\n",
	},
	calcSpare: {
		"func Spare(n int) int {\n\treturn n\n}\n",
		"func Spare(n int) int {\n\treturn n + 5\n}\n",
	},
}

// mutateCalcAt returns calc.go with the functions whose indexes landed names at their landed text.
func mutateCalcAt(landed ...int) string {
	var b strings.Builder
	b.WriteString("package calc\n")
	for i, fn := range mutateCalcFunctions {
		b.WriteString("\n")
		if slices.Contains(landed, i) {
			b.WriteString(fn.landed)
		} else {
			b.WriteString(fn.seed)
		}
	}
	return b.String()
}

// landMutateCalc writes calc.go with the functions landed names landed, commits it in root and returns the commit.
func landMutateCalc(t *testing.T, root string, landed ...int) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "calc", "calc.go"), []byte(mutateCalcAt(landed...)), 0o644); err != nil {
		t.Fatal(err)
	}
	gitkit.Git(t, root, "add", "calc", "other", "internal")
	gitkit.Git(t, root, "commit", "-q", "-m", "land calc")
	return gitkit.RevParse(t, root, "HEAD")
}

// editMutateCalc replaces from with to in calc.go of root, leaving the change uncommitted.
func editMutateCalc(t *testing.T, root, from, to string) {
	t.Helper()
	file := filepath.Join(root, "calc", "calc.go")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), from) {
		t.Fatalf("calc.go holds no %q to replace", from)
	}
	if err := os.WriteFile(file, []byte(strings.Replace(string(data), from, to, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mutateGate returns the hub's gate.yaml with the top-level lines extra adds.
// Its tier0 passes at once, so a gate call runs its report steps.
func mutateGate(extra string) string {
	return "slots: 2\nscoper: go\nredundancy_test_timeout_sec: 20\nredundancy_test_memory_mb: 128\n" + extra +
		"tiers:\n  tier0: {command: 'true', timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"0\"}}\n"
}

// lineOf returns the 1-based number of the first line of text that holds needle.
func lineOf(t *testing.T, text, needle string) int {
	t.Helper()
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return i + 1
		}
	}
	t.Fatalf("the text holds no line with %q", needle)
	return 0
}

// mutateVerbAnswer is the part of a `lym gate mutate` envelope the rows compare.
type mutateVerbAnswer struct {
	OK              bool                     `json:"ok"`
	Error           string                   `json:"error"`
	Note            string                   `json:"note"`
	Base            string                   `json:"base"`
	Mutants         MutationCounts           `json:"mutants"`
	Runs            int                      `json:"runs"`
	Survivors       []MutationSurvivor       `json:"survivors"`
	UnjudgedMutants []MutationUnjudgedMutant `json:"unjudged_mutants"`
	UncoveredBlocks []MutationUncoveredBlock `json:"uncovered_blocks"`
	NotJudged       []MutationNotJudged      `json:"not_judged"`
}

// runMutateVerb runs the command root over the directory cwd with args and returns its exit code and envelope.
func runMutateVerb(t *testing.T, root *cobra.Command, cwd string, args ...string) (int, mutateVerbAnswer) {
	t.Helper()
	var out bytes.Buffer
	code := clihelp.ExecuteIn(root, cwd, &out, args)
	var answer mutateVerbAnswer
	if err := json.Unmarshal(out.Bytes(), &answer); err != nil {
		t.Fatalf("lym gate %v printed %q; want one JSON envelope: %v", args, out.String(), err)
	}
	return code, answer
}

// requireMutateAnswer fails the test unless the verb exited 0 with an ok envelope.
func requireMutateAnswer(t *testing.T, code int, answer mutateVerbAnswer) {
	t.Helper()
	if code != 0 || !answer.OK {
		t.Fatalf("lym gate mutate = exit %d, ok %v, error %q; want exit 0", code, answer.OK, answer.Error)
	}
}

// survivorLines returns the lines of the survivors, in the order the envelope lists them.
func survivorLines(answer mutateVerbAnswer) []int {
	var lines []int
	for _, survivor := range answer.Survivors {
		lines = append(lines, survivor.Line)
	}
	return lines
}

// unjudgedAt returns the reason of the unjudged mutant on the line of calc.go, or "" when none is unjudged there.
func unjudgedAt(answer mutateVerbAnswer, line int) string {
	for _, unjudged := range answer.UnjudgedMutants {
		if unjudged.File == "calc/calc.go" && unjudged.Line == line {
			return unjudged.Reason
		}
	}
	return ""
}

// mutationStepAt returns the mutation report step of the worktree at root, with its location and its stderr buffer.
func mutationStepAt(t *testing.T, root string) (mutationStep, *lymcwd.Location, *bytes.Buffer) {
	t.Helper()
	location, err := lymcwd.ResolveWorktree(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(location))
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	step := mutationStep{reach: reachStep{
		ctx: context.Background(), stderr: &stderr, cfg: cfg.WithDefaultDir(location.AnchorRel), location: location, root: location.WorktreePath(),
		gateKey: "darn-write", environ: os.Environ(), goBinary: defaultGoBinary,
		minuteUnit: time.Minute, secondUnit: time.Second, grace: defaultPipeWaitDelay,
	}}
	return step, location, &stderr
}

// TestGateMutate mutates the diffs of a hub's module and reads the verdicts back, with real builds and runs.
func TestGateMutate(t *testing.T) {
	tiergate.Require(t, 1)
	// The test runs once, in parallel with the other tests, but its rows run one after another:
	// each builds real binaries in one hub, rewrites the hub's gate.yaml, or times a mutant against a real bound.
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	prime, pair := h.PrimeWorktree(), h.PairCodeWorktree(hubforge.TemplatePairSlug)
	setGate := func(extra string) {
		t.Helper()
		if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(mutateGate(extra)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	seedTestMapModule(t, h, mutateModule, mutateGate(""), map[string]map[string]int{"calc": nil})
	plankit.WriteTree(t, prime, mutateSupportFiles)
	// Three landings after the seed: Tally, Hoard and Spare, then Scale, then Clamp.
	seed := landMutateCalc(t, prime)
	afterTallyHoardSpare := landMutateCalc(t, prime, calcTally, calcHoard, calcSpare)
	afterScale := landMutateCalc(t, prime, calcScale, calcTally, calcHoard, calcSpare)
	landMutateCalc(t, prime, calcClamp, calcScale, calcTally, calcHoard, calcSpare)
	var built bytes.Buffer
	if code := clihelp.ExecuteIn(Command(), prime, &built, []string{"test", "--wait", "--map", "calc", "other"}); code != 0 {
		t.Fatalf("test --wait --map calc other = %d, %s; want exit 0", code, built.Bytes())
	}

	final := mutateCalcAt(calcClamp, calcScale, calcTally, calcHoard, calcSpare)
	scaleLimit, scaleIf := lineOf(t, final, "limit := 100"), lineOf(t, final, "if n > limit {")
	tallyDrop, hoardLoop, spareReturn := lineOf(t, final, "n -= 1"), lineOf(t, final, "for i := 1; i <= n; i++ {"), lineOf(t, final, "return n + 5")

	refusals := []struct {
		name      string
		cwd       string
		args      []string
		wantError string
		// wantWayForward is the text the error's way forward must name.
		wantWayForward string
	}{
		{name: "OutsideEveryHub", cwd: t.TempDir(), args: []string{"mutate"}, wantError: "is in no hub", wantWayForward: "run lym gate mutate from a worktree of the hub"},
		{name: "APrimeWithNoBase", cwd: prime, args: []string{"mutate"}, wantError: "no parent branch", wantWayForward: "pass --base"},
		{name: "ABaseNamingNoCommit", cwd: prime, args: []string{"mutate", "--base", "no-such-commit"}, wantError: "names no commit", wantWayForward: "pass the SHA of a commit"},
	}
	for _, tt := range refusals {
		t.Run("Refuses"+tt.name, func(t *testing.T) {
			code, answer := runMutateVerb(t, Command(), tt.cwd, tt.args...)
			_, wayForward, _ := strings.Cut(answer.Error, "way forward:")
			if code != 1 || answer.OK || !strings.Contains(answer.Error, tt.wantError) || !strings.Contains(wayForward, tt.wantWayForward) {
				t.Errorf("lym gate %v = exit %d, ok %v, error %q; want exit 1 with %q and a way forward naming %q",
					tt.args, code, answer.OK, answer.Error, tt.wantError, tt.wantWayForward)
			}
		})
	}

	t.Run("AHubWhoseScoperIsNotGoPrintsItsNoteAndExitsZero", func(t *testing.T) {
		other := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		otherPrime := seedTestMapModule(t, other, mutateModule, strings.Replace(mutateGate(""), "scoper: go", "scoper: none", 1),
			map[string]map[string]int{"calc": nil})
		code, answer := runMutateVerb(t, Command(), otherPrime, "mutate")
		requireMutateAnswer(t, code, answer)
		if !strings.Contains(answer.Note, "the hub's gate.scoper is none, not go") {
			t.Errorf("lym gate mutate note = %q; want one naming the scoper", answer.Note)
		}
	})

	t.Run("ChangedConditionIsKilledByTheFirstCoveringTestAndTheSecondDoesNotRun", func(t *testing.T) {
		setGate("")
		code, answer := runMutateVerb(t, Command(), prime, "mutate", "--base", afterScale)
		requireMutateAnswer(t, code, answer)
		// Clamp alone changed since afterScale: its condition and its constant, each killed by one run.
		// Both tests of Clamp cover each mutant's block, so a second run would show in the count.
		want := MutationCounts{Run: 2, Killed: 2}
		if answer.Mutants != want || answer.Runs != 2 || len(answer.Survivors) != 0 || len(answer.UnjudgedMutants) != 0 || len(answer.NotJudged) != 0 {
			t.Errorf("lym gate mutate --base afterScale = %+v, %d runs, survivors %v, unjudged %v, not judged %v; want %+v from 2 runs and nothing else",
				answer.Mutants, answer.Runs, answer.Survivors, answer.UnjudgedMutants, answer.NotJudged, want)
		}
	})

	t.Run("ABaseBeforeAnEarlierLandingJudgesItsChangeToo", func(t *testing.T) {
		setGate("")
		code, answer := runMutateVerb(t, Command(), prime, "mutate", "--base", afterTallyHoardSpare)
		requireMutateAnswer(t, code, answer)
		// Scale joins Clamp: its two mutants survive, since TestScaleRuns runs Scale and checks nothing.
		want := MutationCounts{Run: 4, Killed: 2, Survived: 2}
		if answer.Mutants != want || !slices.Equal(survivorLines(answer), []int{scaleLimit, scaleIf}) {
			t.Fatalf("lym gate mutate --base afterTallyHoardSpare = %+v, survivors on lines %v; want %+v and survivors on lines %v",
				answer.Mutants, survivorLines(answer), want, []int{scaleLimit, scaleIf})
		}
		for i, operator := range []string{testcover.OperatorChangeConstant, testcover.OperatorNegateCondition} {
			survivor := answer.Survivors[i]
			if survivor.File != "calc/calc.go" || survivor.Function != "Scale" || survivor.Operator != operator ||
				!slices.Equal(survivor.PossibleKillers, []string{"other.TestOtherCalls"}) {
				t.Errorf("survivor %d = %+v; want %s in calc/calc.go's Scale with the other package's TestOtherCalls as possible killer", i, survivor, operator)
			}
		}
	})

	var planned int
	t.Run("AWholeDiffKillsSurvivesAndLeavesUnjudgedTheMutantsOfTheMapsTests", func(t *testing.T) {
		setGate("")
		code, answer := runMutateVerb(t, Command(), prime, "mutate", "--base", seed)
		requireMutateAnswer(t, code, answer)
		// Clamp's two mutants and Tally's condition, constant and dropped increment die.
		// Scale's two survive.
		// Tally's dropped decrement ends endless at the time bound and Hoard's negated loop at the memory cap.
		want := MutationCounts{Run: 9, Killed: 5, Survived: 2, Unjudged: 2}
		if answer.Mutants != want || !slices.Equal(survivorLines(answer), []int{scaleLimit, scaleIf}) {
			t.Errorf("lym gate mutate --base seed = %+v, survivors on lines %v; want %+v and survivors on lines %v",
				answer.Mutants, survivorLines(answer), want, []int{scaleLimit, scaleIf})
		}
		planned = answer.Mutants.Run
		if reason := unjudgedAt(answer, tallyDrop); !strings.Contains(reason, "time bound") {
			t.Errorf("the dropped decrement on line %d is unjudged for %q; want its time bound", tallyDrop, reason)
		}
		if reason := unjudgedAt(answer, hoardLoop); !strings.Contains(reason, "memory cap") {
			t.Errorf("the negated loop on line %d is unjudged for %q; want the memory cap", hoardLoop, reason)
		}
		// Nothing runs Spare, so its changed block is uncovered and holds no mutant.
		if !slices.Contains(answer.UncoveredBlocks, MutationUncoveredBlock{Package: "calc", File: "calc.go", Function: "Spare"}) {
			t.Errorf("lym gate mutate uncovered blocks = %v; want Spare's", answer.UncoveredBlocks)
		}
		if reason := unjudgedAt(answer, spareReturn); reason != "" || slices.Contains(survivorLines(answer), spareReturn) {
			t.Errorf("lym gate mutate places a mutant on line %d of the uncovered Spare", spareReturn)
		}
	})

	t.Run("ADiffThatChangesNoFunctionPrintsItsNoteAndMutatesNothing", func(t *testing.T) {
		setGate("")
		code, answer := runMutateVerb(t, Command(), prime, "mutate", "--base", gitkit.RevParse(t, prime, "HEAD"))
		requireMutateAnswer(t, code, answer)
		if !strings.Contains(answer.Note, "the diff changes no function") || answer.Mutants != (MutationCounts{}) || answer.Runs != 0 {
			t.Errorf("lym gate mutate --base HEAD = note %q, %+v, %d runs; want the changes-no-function note and no mutant", answer.Note, answer.Mutants, answer.Runs)
		}
	})

	t.Run("APrimeWhoseSharedMapNoLongerMatchesItsFilesIsNotJudgedWithItsReason", func(t *testing.T) {
		setGate("")
		editMutateCalc(t, prime, "return n + 5", "return n + 6")
		t.Cleanup(func() { gitkit.Git(t, prime, "checkout", "--", "calc/calc.go") })
		code, answer := runMutateVerb(t, Command(), prime, "mutate", "--base", seed)
		requireMutateAnswer(t, code, answer)
		if len(answer.NotJudged) != 1 || answer.NotJudged[0].Package != "calc" || answer.NotJudged[0].Reason != reasonSharedMapStale ||
			answer.Mutants != (MutationCounts{}) {
			t.Errorf("lym gate mutate = %+v, not judged %+v; want calc not judged for %q and no mutant", answer.Mutants, answer.NotJudged, reasonSharedMapStale)
		}
	})

	capRows := []struct {
		name  string
		max   string
		base  string
		want  MutationCounts
		lines []int
	}{
		{name: "ACapOfOneRunsOneMutant", max: "1", base: seed, want: MutationCounts{Run: 1, Killed: 1}},
		{name: "ACapOfTwoOverTwoChangedFunctionsPlansOneMutantInEach", max: "2", base: afterTallyHoardSpare, want: MutationCounts{Run: 2, Killed: 1, Survived: 1}, lines: []int{scaleLimit}},
	}
	for _, tt := range capRows {
		t.Run(tt.name, func(t *testing.T) {
			setGate("mutation_max_mutants: " + tt.max + "\n")
			code, answer := runMutateVerb(t, Command(), prime, "mutate", "--base", tt.base)
			requireMutateAnswer(t, code, answer)
			// Every planned mutant that the cap leaves out is counted not run.
			wantAll := MutationCounts{Run: tt.want.Run, Killed: tt.want.Killed, Survived: tt.want.Survived}
			notRun := answer.Mutants.NotRun
			answer.Mutants.NotRun = 0
			if answer.Mutants != wantAll || !slices.Equal(survivorLines(answer), tt.lines) || notRun == 0 {
				t.Errorf("lym gate mutate under a cap of %s = %+v, %d not run, survivors on lines %v; want %+v, some not run and survivors on lines %v",
					tt.max, answer.Mutants, notRun, survivorLines(answer), wantAll, tt.lines)
			}
			if tt.max == "1" && notRun != planned-1 {
				t.Errorf("lym gate mutate under a cap of 1 leaves %d mutants not run; want the other %d of the whole diff", notRun, planned-1)
			}
		})
	}
	setGate("")

	// The pair starts at the prime's last landing and lands one change of its own.
	gitkit.Git(t, pair, "merge", "--ff-only", "-q", "main")
	editMutateCalc(t, pair, "\t\treturn 0\n", "\t\treturn 0 // clamped\n")
	gitkit.Git(t, pair, "commit", "-q", "-a", "-m", "annotate clamp")

	t.Run("APackageWithAnUncommittedFileIsNotJudgedWithItsReason", func(t *testing.T) {
		editMutateCalc(t, pair, "\t\treturn 0 // clamped\n", "\t\treturn 0 // clamped, edited\n")
		t.Cleanup(func() { gitkit.Git(t, pair, "checkout", "--", "calc/calc.go") })
		code, answer := runMutateVerb(t, Command(), pair, "mutate")
		requireMutateAnswer(t, code, answer)
		if len(answer.NotJudged) != 1 || answer.NotJudged[0].Package != "calc" ||
			!strings.Contains(answer.NotJudged[0].Reason, "uncommitted file") || !strings.Contains(answer.NotJudged[0].Reason, "commit it") ||
			answer.Mutants != (MutationCounts{}) {
			t.Errorf("lym gate mutate = %+v, not judged %+v; want calc not judged for its uncommitted file and no mutant", answer.Mutants, answer.NotJudged)
		}
	})

	t.Run("APackageWithAStaleWorktreeMapAndCommittedFilesIsRefreshedThenJudged", func(t *testing.T) {
		code, answer := runMutateVerb(t, Command(), pair, "mutate")
		requireMutateAnswer(t, code, answer)
		want := MutationCounts{Run: 1, Killed: 1}
		if answer.Mutants != want || len(answer.NotJudged) != 0 || len(answer.Survivors) != 0 {
			t.Errorf("lym gate mutate = %+v, not judged %+v, survivors %v; want calc refreshed and its one mutant killed: %+v",
				answer.Mutants, answer.NotJudged, answer.Survivors, want)
		}
	})

	t.Run("MutantsPlannedPastMutationBudgetAreUnjudgedAndCounted", func(t *testing.T) {
		// The budget and the coverage build's bound share one minute unit.
		// A millisecond makes the budget one millisecond and the bound 100 seconds.
		// So the build finishes and the mutants are planned before the spent budget leaves them unjudged.
		setGate("mutation_budget_min: 1\nredundancy_build_timeout_min: 100000\n")
		t.Cleanup(func() { setGate("") })
		code, answer := runMutateVerb(t, mutateCmd(defaultGoBinary, time.Millisecond, defaultPipeWaitDelay), pair)
		requireMutateAnswer(t, code, answer)
		if answer.Mutants != (MutationCounts{Unjudged: 1}) || len(answer.UnjudgedMutants) != 1 ||
			!strings.Contains(answer.UnjudgedMutants[0].Reason, "mutation_budget_min") {
			t.Errorf("lym gate mutate = %+v, unjudged %+v; want the one planned mutant unjudged for mutation_budget_min", answer.Mutants, answer.UnjudgedMutants)
		}
	})

	t.Run("APackageWithAStaleWorktreeMapIsNotJudgedPastReachBudget", func(t *testing.T) {
		editMutateCalc(t, pair, "\t\treturn 0 // clamped\n", "\t\treturn 0 // clamped again\n")
		gitkit.Git(t, pair, "commit", "-q", "-a", "-m", "annotate clamp again")
		// A nanosecond minute spends the budget at the hold, before the refresh starts any package.
		code, answer := runMutateVerb(t, mutateCmd(defaultGoBinary, time.Nanosecond, defaultPipeWaitDelay), pair)
		requireMutateAnswer(t, code, answer)
		if len(answer.NotJudged) != 1 || answer.NotJudged[0].Package != "calc" || !strings.Contains(answer.NotJudged[0].Reason, "reach_budget_min") ||
			answer.Mutants != (MutationCounts{}) {
			t.Errorf("lym gate mutate = %+v, not judged %+v; want calc not judged past reach_budget_min and no mutant", answer.Mutants, answer.NotJudged)
		}
	})

	t.Run("AGateCallListingMutationWritesItsFindingsAndPrintsOneSummaryLine", func(t *testing.T) {
		setGate("gates:\n  darn-write: {tiers: [0], scope: all, reports: [mutation]}\n")
		t.Cleanup(func() { setGate("") })
		var out, stderr bytes.Buffer
		code := runTest(lymcwd.WithCwd(context.Background(), pair), &out, testRequest{
			goBinary: defaultGoBinary, samplePeriod: defaultSamplePeriod, preTierCeiling: defaultPreTierCeiling, timeoutUnit: time.Minute,
			secondUnit: time.Second, rerunDelay: defaultRerunDelay, pipeWaitDelay: defaultPipeWaitDelay, stderr: &stderr, gate: "darn-write",
		})
		location, err := lymcwd.ResolveWorktree(pair)
		if err != nil {
			t.Fatal(err)
		}
		report, found, err := ReadMutationReport(location)
		if code != 0 || err != nil || !found {
			t.Fatalf("lym gate test --gate darn-write = exit %d, report found %v, error %v, stderr %s; want exit 0 and a report", code, found, err, stderr.String())
		}
		// The step refreshes the stale worktree map, and the one mutant of the pair's changed line in Clamp dies.
		if report.GateKey != "darn-write" || report.Mutants != (MutationCounts{Run: 1, Killed: 1}) || len(report.Functions) != 1 || report.Functions[0].Func != "Clamp" {
			t.Errorf("mutation report = %+v; want darn-write's, with Clamp's one mutant killed", report)
		}
		var summaries []string
		for _, line := range strings.Split(stderr.String(), "\n") {
			if strings.HasPrefix(line, "gate test: mutation report: ") {
				summaries = append(summaries, line)
			}
		}
		want := "gate test: mutation report: 1 mutants run, 1 killed, 0 survived, 0 unjudged, 0 uncovered changed blocks, 0 packages not judged; written to " +
			mutationReportPath(location)
		if !slices.Equal(summaries, []string{want}) {
			t.Errorf("stderr mutation report lines = %q; want only %q", summaries, want)
		}
		if matches, err := MutationReportMatches(location, report, gitkit.RevParse(t, pair, "HEAD")); err != nil || !matches {
			t.Errorf("MutationReportMatches(HEAD) = %v, %v; want the report to hold for the HEAD it was written at", matches, err)
		}
		// A commit that changes Clamp again leaves the report naming the Clamp it judged.
		editMutateCalc(t, pair, "\t\treturn 0 // clamped again\n", "\t\treturn 0 // clamped once more\n")
		gitkit.Git(t, pair, "commit", "-q", "-a", "-m", "annotate clamp once more")
		if matches, err := MutationReportMatches(location, report, gitkit.RevParse(t, pair, "HEAD")); err != nil || matches {
			t.Errorf("MutationReportMatches(a HEAD that changes Clamp again) = %v, %v; want false", matches, err)
		}
	})

	t.Run("AGateCallListingMutationWritesTheSurvivorsAndUncoveredBlocksOfItsDiff", func(t *testing.T) {
		editMutateCalc(t, pair, "limit := 100", "limit := 101")
		editMutateCalc(t, pair, "return n + 5", "return n + 6")
		gitkit.Git(t, pair, "commit", "-q", "-a", "-m", "change scale and spare")
		setGate("gates:\n  darn-write: {tiers: [0], scope: all, reports: [mutation]}\n")
		t.Cleanup(func() { setGate("") })
		var out, stderr bytes.Buffer
		code := runTest(lymcwd.WithCwd(context.Background(), pair), &out, testRequest{
			goBinary: defaultGoBinary, samplePeriod: defaultSamplePeriod, preTierCeiling: defaultPreTierCeiling, timeoutUnit: time.Minute,
			secondUnit: time.Second, rerunDelay: defaultRerunDelay, pipeWaitDelay: defaultPipeWaitDelay, stderr: &stderr, gate: "darn-write",
		})
		location, err := lymcwd.ResolveWorktree(pair)
		if err != nil {
			t.Fatal(err)
		}
		report, found, err := ReadMutationReport(location)
		if code != 0 || err != nil || !found {
			t.Fatalf("lym gate test --gate darn-write = exit %d, report found %v, error %v, stderr %s; want exit 0 and a report", code, found, err, stderr.String())
		}
		// Scale's changed constant survives, since TestScaleRuns checks nothing, and Clamp's mutant dies.
		want := MutationCounts{Run: 2, Killed: 1, Survived: 1}
		if report.Mutants != want {
			t.Errorf("mutation report mutants = %+v; want %+v", report.Mutants, want)
		}
		if len(report.Survivors) != 1 || report.Survivors[0].File != "calc/calc.go" || report.Survivors[0].Line != scaleLimit || report.Survivors[0].Function != "Scale" {
			t.Errorf("mutation report survivors = %+v; want the one mutant of calc/calc.go's Scale on line %d", report.Survivors, scaleLimit)
		}
		// Nothing runs Spare, so its changed block is uncovered.
		wantUncovered := []MutationUncoveredBlock{{Package: "calc", File: "calc.go", Function: "Spare"}}
		if !slices.Equal(report.Uncovered, wantUncovered) {
			t.Errorf("mutation report uncovered blocks = %v; want %v", report.Uncovered, wantUncovered)
		}
	})

	judgesNothing := []struct {
		name   string
		red    bool
		inHub  bool
		scoper string
		want   string
	}{
		{name: "ARedTier", red: true, inHub: true, want: noteRedGate},
		{name: "ACallOutsideAHub", want: noteNoHub},
		{name: "AScoperOtherThanGo", inHub: true, scoper: "none", want: "the hub's gate.scoper is none, not go, so the step judged nothing"},
	}
	for _, tt := range judgesNothing {
		t.Run("TheStepJudgesNothingFor"+tt.name, func(t *testing.T) {
			step, location, stderr := mutationStepAt(t, pair)
			if tt.scoper != "" {
				step.reach.cfg.Scoper = tt.scoper
			}
			step.writeReport(step.judgeReport(tt.red, tt.inHub))
			report, found, err := ReadMutationReport(location)
			if err != nil || !found {
				t.Fatalf("ReadMutationReport = found %v, error %v; want a report", found, err)
			}
			wantLine := "gate test: mutation report: " + tt.want + "; written to " + mutationReportPath(location)
			if report.Note != tt.want || report.RedGate != tt.red || len(report.Functions) != 0 || report.Mutants != (MutationCounts{}) ||
				strings.TrimSuffix(stderr.String(), "\n") != wantLine {
				t.Errorf("report = note %q, red gate %v, %d functions, %+v; stderr %q; want note %q, red gate %v, nothing judged and the line %q",
					report.Note, report.RedGate, len(report.Functions), report.Mutants, stderr.String(), tt.want, tt.red, wantLine)
			}
		})
	}

	t.Run("ARedTierCallKeepsItsExitCodeAndWritesARedGateReport", func(t *testing.T) {
		red := strings.Replace(mutateGate("gates:\n  darn-write: {tiers: [0], scope: all, reports: [mutation]}\n"), "command: 'true'", "command: 'false'", 1)
		if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(red), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { setGate("") })
		var out, stderr bytes.Buffer
		code := runTest(lymcwd.WithCwd(context.Background(), pair), &out, testRequest{
			goBinary: defaultGoBinary, samplePeriod: defaultSamplePeriod, preTierCeiling: defaultPreTierCeiling, timeoutUnit: time.Minute,
			secondUnit: time.Second, rerunDelay: defaultRerunDelay, pipeWaitDelay: defaultPipeWaitDelay, stderr: &stderr, gate: "darn-write",
		})
		location, err := lymcwd.ResolveWorktree(pair)
		if err != nil {
			t.Fatal(err)
		}
		report, found, err := ReadMutationReport(location)
		if code != 1 || err != nil || !found || !report.RedGate || report.Note != noteRedGate {
			t.Errorf("lym gate test --gate darn-write over a red tier = exit %d, report %+v, found %v, error %v; want exit 1 and a red-gate report with %q",
				code, report, found, err, noteRedGate)
		}
	})

	t.Run("APrimePackageWithNoMapIsNotJudgedAndAMutantTheCompilerRefusesIsCountedUnbuildable", func(t *testing.T) {
		base := gitkit.RevParse(t, prime, "HEAD")
		// Half's integer constant sits in a float context, where its mutant does not compile.
		plankit.WriteTree(t, prime, map[string]string{
			"unmapped/unmapped.go": "package unmapped\n\nfunc Value() int { return 1 }\n",
			"ratio/ratio.go":       "package ratio\n\nfunc Half() float64 {\n\treturn 3\n}\n",
			"ratio/ratio_test.go": "package ratio\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n" +
				"func TestHalfChecked(t *testing.T) {\n\ttiergate.Require(t, 0)\n\tif Half() != 3 {\n\t\tt.Fatal(\"Half() != 3\")\n\t}\n}\n",
		})
		gitkit.Git(t, prime, "add", "unmapped", "ratio")
		gitkit.Git(t, prime, "commit", "-q", "-m", "land unmapped and ratio")
		var built bytes.Buffer
		if code := clihelp.ExecuteIn(Command(), prime, &built, []string{"test", "--wait", "--map", "ratio"}); code != 0 {
			t.Fatalf("test --wait --map ratio = %d, %s; want exit 0", code, built.Bytes())
		}

		code, answer := runMutateVerb(t, Command(), prime, "mutate", "--base", base)
		requireMutateAnswer(t, code, answer)
		if !slices.Contains(answer.NotJudged, MutationNotJudged{Package: "unmapped", Reason: reasonNoMapToMutate}) {
			t.Errorf("lym gate mutate not judged = %+v; want unmapped for %q", answer.NotJudged, reasonNoMapToMutate)
		}
		if answer.Mutants.Unbuildable != 1 {
			t.Errorf("lym gate mutate mutants = %+v; want the one mutant of Half counted unbuildable", answer.Mutants)
		}
	})

	t.Run("ATaskWorktreeWithNoParentBranchJudgesNothing", func(t *testing.T) {
		step, location, stderr := mutationStepAt(t, pair)
		if err := os.Remove(fabricengine.OriginRecordPath(location)); err != nil {
			t.Fatal(err)
		}
		step.writeReport(step.judgeReport(false, true))
		report, found, err := ReadMutationReport(location)
		if err != nil || !found {
			t.Fatalf("ReadMutationReport = found %v, error %v; want a report", found, err)
		}
		wantLine := "gate test: mutation report: " + noteNoParent + "; written to " + mutationReportPath(location)
		if report.Note != noteNoParent || report.Prime || len(report.Functions) != 0 || strings.TrimSuffix(stderr.String(), "\n") != wantLine {
			t.Errorf("report = note %q, prime %v, %d functions; stderr %q; want note %q, no function and the line %q",
				report.Note, report.Prime, len(report.Functions), stderr.String(), noteNoParent, wantLine)
		}
	})
}
