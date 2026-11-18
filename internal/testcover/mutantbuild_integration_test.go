// mutantbuild_integration_test.go builds a real mutant binary with the go command and runs its test with each mutant on.
// It is Tier 1: it spawns the go command and the binary it builds.

package testcover_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// typedLiteralSource holds a rune literal compared with a byte and an integer literal divided into one.
// A mutant that turned either literal into an int or a rune would not compile.
const typedLiteralSource = `package p

func Leading(s string) bool {
	return s[0] == '0'
}

func Halve(b byte) byte {
	return b / 2
}
`

// typedLiteralTest checks both functions of typedLiteralSource, so every mutant of theirs fails it.
const typedLiteralTest = `package p

import "testing"

func TestTyped(t *testing.T) {
	if !Leading("01") || Halve(4) != 2 {
		t.Fatal("Leading or Halve is wrong")
	}
}
`

// embeddedLineSource returns the text an embed directive fills from text.txt.
const embeddedLineSource = `package p

import _ "embed"

//go:embed text.txt
var text string

func Text() string {
	return text
}
`

// embeddedLineTest passes only when Text returns the text the WANT_TEXT variable holds.
const embeddedLineTest = `package p

import (
	"os"
	"testing"
)

func TestText(t *testing.T) {
	if got, want := Text(), os.Getenv("WANT_TEXT"); got != want {
		t.Fatalf("Text() = %q; want %q", got, want)
	}
}
`

// embeddedLineText is the embedded file.
// It holds a colon key and value, an equals key and value, and a bare key.
// It also holds an equals before a colon, a colon followed by blanks only, and a plain line.
const embeddedLineText = "name: lym\nnote = x\nlist:\na=b: c\nkey:  \n\n  plain\n"

// buildFixtureMutants writes a module whose package p holds files.
// It plans the mutants of the blocks of p.go that spans mark and builds their binary.
// The test fails when a mutant does not compile.
// It returns the plan, the package directory, the binary and the environment to run the binary in.
func buildFixtureMutants(t *testing.T, files map[string]string, spans [][2]string) (*testcover.MutantPlan, string, string, []string) {
	t.Helper()
	module := t.TempDir()
	pkgDir := filepath.Join(module, "p")
	if err := os.Mkdir(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "go.mod"), []byte("module m\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(pkgDir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	source := files["p.go"]
	var blocks []testcover.ProfileBlock
	for _, span := range spans {
		start := strings.Index(source, span[0])
		end := start + strings.Index(source[start:], span[1]) + len(span[1])
		startLine, startCol := lineCol(source, start)
		endLine, endCol := lineCol(source, end)
		blocks = append(blocks, testcover.ProfileBlock{File: "m/p/p.go", StartLine: startLine, StartCol: startCol, EndLine: endLine, EndCol: endCol, Statements: 1})
	}
	keys, err := testcover.KeyBlocks(pkgDir, blocks)
	if err != nil {
		t.Fatal(err)
	}
	targets := testcover.BlockSet{}
	for _, key := range keys {
		targets[key.String()] = struct{}{}
	}
	plan, err := testcover.PlanMutants(pkgDir, "m/p", blocks, targets)
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := plan.WriteOverlay(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GOFLAGS=", "GOWORK=off", "GOTOOLCHAIN=local")
	bin := filepath.Join(t.TempDir(), "mutants.test")
	if err := (testcover.Toolchain{Go: "go", Env: env, Dir: module}).BuildMutantBinary("m/p", overlay, bin); err != nil {
		t.Fatalf("BuildMutantBinary = %v; want every mutant to compile", err)
	}
	return plan, pkgDir, bin, env
}

func TestMutantBinary_KeepsTheTypeOfAByteContextLiteral(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	plan, pkgDir, bin, env := buildFixtureMutants(t,
		map[string]string{"p.go": typedLiteralSource, "p_test.go": typedLiteralTest},
		[][2]string{{"{\n\treturn s[0]", "'0'\n}"}, {"{\n\treturn b / 2", "2\n}"}})
	// Leading holds a comparison, the index 0 and the rune '0'; Halve holds the 2.
	if len(plan.Mutants) != 4 {
		t.Fatalf("PlanMutants = %+v; want 4 mutants", plan.Mutants)
	}
	for id := range len(plan.Mutants) + 1 {
		want := testcover.ActionFail
		if id == 0 {
			want = testcover.ActionPass
		}
		runner := testcover.Toolchain{Env: append(env, testcover.MutantEnv+"="+strconv.Itoa(id))}
		result, err := runner.RunAlone(bin, pkgDir, "", "TestTyped")
		if err != nil {
			t.Fatal(err)
		}
		if result.Action != want {
			t.Errorf("RunAlone(TestTyped) with mutant %d = %q; want %q", id, result.Action, want)
		}
	}
}

func TestMutantBinary_ChangesTheValueOfAnEmbeddedKeyValueLine(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	plan, pkgDir, bin, env := buildFixtureMutants(t,
		map[string]string{"p.go": embeddedLineSource, "p_test.go": embeddedLineTest, "text.txt": embeddedLineText},
		[][2]string{{"{\n\treturn text", "text\n}"}})
	// marked returns the embedded file with its line replaced by markedLine.
	marked := func(line, markedLine string) string {
		return strings.Replace(embeddedLineText, line, markedLine, 1)
	}
	// The mutants count from 1 in the order of the file's non-blank lines.
	tests := []struct {
		name string
		id   int
		want string
	}{
		{"NoMutant", 0, embeddedLineText},
		{"ColonLine_ValueChangesKeyStays", 1, marked("name: lym", "name: ~lym")},
		{"EqualsLine_ValueChangesKeyStays", 2, marked("note = x", "note = ~x")},
		{"KeyWithNoValue_LineChangesAtItsStart", 3, marked("list:", "~list:")},
		{"EqualsBeforeColon_EarlierSeparatorOpensTheValue", 4, marked("a=b: c", "a=~b: c")},
		{"ColonFollowedByBlanks_LineChangesAtItsStart", 5, marked("key:  ", "~key:  ")},
		{"PlainLine_ChangesAfterItsIndent", 6, marked("  plain", "  ~plain")},
	}
	if len(plan.Mutants) != len(tests)-1 {
		t.Fatalf("PlanMutants = %+v; want one mutant per non-blank line, %d", plan.Mutants, len(tests)-1)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runner := testcover.Toolchain{Env: append(env, testcover.MutantEnv+"="+strconv.Itoa(tt.id), "WANT_TEXT="+tt.want)}
			result, err := runner.RunAlone(bin, pkgDir, "", "TestText")
			if err != nil {
				t.Fatal(err)
			}
			if result.Action != testcover.ActionPass {
				t.Errorf("RunAlone(TestText) with mutant %d = %q; want %q, Text() returning %q", tt.id, result.Action, testcover.ActionPass, tt.want)
			}
		})
	}
}
