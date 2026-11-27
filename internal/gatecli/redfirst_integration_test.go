// redfirst_integration_test.go drives `lym gate red-first` over a hubforge hub whose prime holds a small Go module.
// The tests run by the verb's tier are built and run by the real go command.
// It is Tier 1: it builds a real hub, spawns git and runs go test.

package gatecli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// redFirstGateYAML is a hub whose key `rf` runs tier0 as a real `go test` over the explicit targets.
const redFirstGateYAML = "slots: 2\nscoper: go\n" +
	"tiers:\n  tier0: {command: 'go test -json -count=1 {targets} {args}', all_targets: ./..., timeout_min: 2, counter: go-json, env: {LYM_TEST_TIER: \"0\"}}\n" +
	"gates:\n  rf: {tiers: [0], scope: diff}\n"

// redFirstModule is the import path of the fixture module.
// It is the import path of the repository, so the module builds alone beside its own stand-in tiergate.
const redFirstModule = "github.com/Knatte18/loomyard"

// redFirstTiergateStub stands in for the tier gate in the fixture module: every test runs.
const redFirstTiergateStub = "package tiergate\n\nimport \"testing\"\n\n// Require stands in for the tier gate: every test runs.\nfunc Require(t testing.TB, tier int) {}\n"

// seedRedFirstModule writes the gate config gate into the hub h and commits a Go module to its prime worktree.
// The module holds the packages calc and other, each a source file with two one-line functions, One and Two, and no test.
// It returns the prime worktree.
func seedRedFirstModule(t *testing.T, h *hubforge.Hub, gate string) string {
	t.Helper()
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gate), 0o644); err != nil {
		t.Fatal(err)
	}
	prime := h.PrimeWorktree()
	plankit.WriteTree(t, prime, map[string]string{
		"go.mod":         "module " + redFirstModule + "\n\ngo 1.21\n",
		"calc/calc.go":   "package calc\n\nfunc One() int { return 1 }\n\nfunc Two() int { return 2 }\n",
		"other/other.go": "package other\n\nfunc One() int { return 1 }\n\nfunc Two() int { return 2 }\n",
	})
	gitkit.Git(t, prime, "add", "go.mod", "calc", "other")
	gitkit.Git(t, prime, "commit", "-q", "-m", "seed the "+redFirstModule+" module")
	return prime
}

// editRedFirstCalc replaces from with to in calc.go of root, leaving the change uncommitted.
func editRedFirstCalc(t *testing.T, root, from, to string) {
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

func TestGateRedFirst(t *testing.T) {
	tiergate.Require(t, 1)
	// The rows run one after another: each rewrites the one working tree that the next row's diff starts from.
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := seedRedFirstModule(t, h, redFirstGateYAML)
	plankit.WriteTree(t, prime, map[string]string{"internal/testkit/tiergate/tiergate.go": redFirstTiergateStub})
	// The package other holds a test named TestTwoIsTwo that fails wherever it runs, and a TestOther the diff will change.
	writeOtherTests := func(otherBody string) {
		t.Helper()
		source := "package other\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n" +
			"func TestTwoIsTwo(t *testing.T) {\n\ttiergate.Require(t, 0)\n\tt.Fatal(\"fails wherever it runs\")\n}\n\n" +
			"func TestOther(t *testing.T) {\n\ttiergate.Require(t, 0)\n" + otherBody + "}\n"
		if err := os.WriteFile(filepath.Join(prime, "other", "other_test.go"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeOtherTests("\t_ = 1\n")
	// The package spec embeds text.txt, whose base text is "base".
	plankit.WriteTree(t, prime, map[string]string{
		"spec/spec.go":  "package spec\n\nimport _ \"embed\"\n\n// Text is the embedded text.\n//\n//go:embed text.txt\nvar Text string\n",
		"spec/text.txt": "base\n",
	})
	gitkit.Git(t, prime, "add", "internal", "other", "spec")
	gitkit.Git(t, prime, "commit", "-q", "-m", "add the tier gate stand-in")
	base := gitkit.RevParse(t, prime, "HEAD")

	// The fix changes One from 1 to 2.
	fix := func() {
		t.Helper()
		editRedFirstCalc(t, prime, "func One() int { return 1 }", "func One() int { return 2 }")
	}
	writeTest := func(name, body string) {
		t.Helper()
		source := "package calc\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\nfunc " + name +
			"(t *testing.T) {\n\ttiergate.Require(t, 0)\n" + body + "}\n"
		if err := os.WriteFile(filepath.Join(prime, "calc", "calc_test.go"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func() (int, string, string) {
		t.Helper()
		var out bytes.Buffer
		code := clihelp.ExecuteIn(Command(), prime, &out, []string{"red-first", "--gate", "rf", "--base", base})
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		var answer struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &answer); err != nil {
			t.Fatalf("red-first printed %q; want the envelope on its last line: %v", out.String(), err)
		}
		return code, lines[len(lines)-1], answer.Error
	}

	fix()
	t.Run("ADiffWithNoTestExitsWithTheNoTestCode", func(t *testing.T) {
		code, _, message := run()
		if code != gateslot.RedFirstNoTestExit || !strings.Contains(message, "adds or changes no test") || !strings.Contains(message, "No test seam") {
			t.Errorf("red-first = exit %d, error %q; want exit %d naming the missing test and the No test seam way forward", code, message, gateslot.RedFirstNoTestExit)
		}
	})
	t.Run("ATestThatFailsOnTheBaseExitsZeroNamingIt", func(t *testing.T) {
		writeTest("TestOneIsTwo", "\tif One() != 2 {\n\t\tt.Fatal(\"One() != 2\")\n\t}\n")
		code, envelope, message := run()
		if code != 0 || !strings.Contains(envelope, `"red_tests":["github.com/Knatte18/loomyard/calc.TestOneIsTwo"]`) {
			t.Errorf("red-first = exit %d, envelope %q, error %q; want exit 0 naming TestOneIsTwo as red", code, envelope, message)
		}
	})
	t.Run("ATestThatPassesOnTheBaseExitsWithTheGreenCode", func(t *testing.T) {
		writeTest("TestTwoIsTwo", "\tif Two() != 2 {\n\t\tt.Fatal(\"Two() != 2\")\n\t}\n")
		code, _, message := run()
		if code != gateslot.RedFirstGreenExit || !strings.Contains(message, "does not show the bug") || !strings.Contains(message, `under a title opening "No bug fixed"`) {
			t.Errorf("red-first = exit %d, error %q; want exit %d saying the test does not show the bug and naming the No bug fixed way forward", code, message, gateslot.RedFirstGreenExit)
		}
	})
	t.Run("AChangedTestDoesNotRunUnderItsNameInAnotherTargetPackage", func(t *testing.T) {
		// calc.TestTwoIsTwo and other.TestOther pass on the base, and the unchanged other.TestTwoIsTwo fails wherever it runs.
		// A run that put both names in one pattern over both packages would run it and judge none of the tests.
		writeOtherTests("\t_ = 2\n")
		code, _, message := run()
		if code != gateslot.RedFirstGreenExit || !strings.Contains(message, "does not show the bug") {
			t.Errorf("red-first = exit %d, error %q; want exit %d, the changed tests passing in their own packages", code, message, gateslot.RedFirstGreenExit)
		}
	})
	t.Run("ANonGoFileNoProductionFileEmbedsLeavesTheVerdictToTheTests", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(prime, "calc", "data.json"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, message := run()
		if code != gateslot.RedFirstGreenExit || !strings.Contains(message, "does not show the bug") {
			t.Errorf("red-first = exit %d, error %q; want exit %d, the changed tests passing on the base", code, message, gateslot.RedFirstGreenExit)
		}
	})
	t.Run("ATestOfAnEmbeddedFileTheDiffChangesRunsOnTheBasesText", func(t *testing.T) {
		plankit.WriteTree(t, prime, map[string]string{
			"spec/text.txt": "fixed\n",
			"spec/spec_test.go": "package spec\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n" +
				"func TestTextIsFixed(t *testing.T) {\n\ttiergate.Require(t, 0)\n\tif Text != \"fixed\\n\" {\n\t\tt.Fatalf(\"Text = %q\", Text)\n\t}\n}\n",
		})
		code, envelope, message := run()
		if code != 0 || !strings.Contains(envelope, `"red_tests":["github.com/Knatte18/loomyard/spec.TestTextIsFixed"]`) {
			t.Errorf("red-first = exit %d, envelope %q, error %q; want exit 0 naming TestTextIsFixed as red on the base's text", code, envelope, message)
		}
	})
}

func TestGateRedFirst_RefusesWhatItCannotDiffOrBuild(t *testing.T) {
	tiergate.Require(t, 1)
	// The rows run one after another: each rewrites the one hub's gate config.
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := seedRedFirstModule(t, h, redFirstGateYAML)
	tests := []struct {
		name string
		gate string
		args []string
		want []string
	}{
		{
			"NoBaseInAWorktreeWithNoParentBranch", redFirstGateYAML, nil,
			[]string{"the worktree has no parent branch to diff from", "pass --base"},
		},
		{
			"ABaseNamingNoCommit", redFirstGateYAML, []string{"--base", "no-such-commit"},
			[]string{"names no commit", "pass the SHA of a commit"},
		},
		{
			"ATierDirHoldingNoGoModule", strings.Replace(redFirstGateYAML, "counter: go-json", "counter: go-json, dir: calc", 1), []string{"--base", "HEAD"},
			[]string{"holds no go.mod naming a module inside the worktree", "declare tier0 or tier1 with its dir at the Go module's root"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(tt.gate), 0o644); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			code := clihelp.ExecuteIn(Command(), prime, &out, append([]string{"red-first", "--gate", "rf"}, tt.args...))
			var answer struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &answer); err != nil {
				t.Fatalf("red-first printed %q; want one JSON envelope: %v", out.String(), err)
			}
			if code != 1 {
				t.Errorf("red-first = exit %d, error %q; want exit 1", code, answer.Error)
			}
			for _, want := range tt.want {
				if !strings.Contains(answer.Error, want) {
					t.Errorf("red-first error = %q; want it to contain %q", answer.Error, want)
				}
			}
		})
	}
}

func TestGateRedFirst_AHubWhoseScoperIsNotGoExitsUnjudged(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := seedRedFirstModule(t, h, strings.Replace(redFirstGateYAML, "scoper: go", "scoper: none", 1))
	var out bytes.Buffer
	code := clihelp.ExecuteIn(Command(), prime, &out, []string{"red-first", "--gate", "rf", "--base", gitkit.RevParse(t, prime, "HEAD")})
	var answer struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &answer); err != nil {
		t.Fatalf("red-first printed %q; want one JSON envelope: %v", out.String(), err)
	}
	if code != gateslot.RedFirstUnjudgedExit || answer.OK || !strings.Contains(answer.Error, "scoper is none") || !strings.Contains(answer.Error, "a red-first gate passes this exit") {
		t.Errorf("red-first = exit %d, ok %v, error %q; want exit %d naming the scoper and that a red-first gate passes it", code, answer.OK, answer.Error, gateslot.RedFirstUnjudgedExit)
	}
}
