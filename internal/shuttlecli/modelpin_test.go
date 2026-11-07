// modelpin_test.go guards the smoke suite's model pin.
// Every site in that suite starting a real `claude` process must thread the shared smokeClaudeModel constant,
// so no future edit can silently put a live spawn back on the account-default model.
//
// It is an untagged source scan on purpose.
// The smoke files are behind `//go:build llm`, so an assertion in them runs only on the rare `-tags llm` invocation.
// That is the run that would already have spent the money.
// Reading them as text runs on every ordinary `go test`, before any spawn happens.
// The constant can therefore only be named here, never used as a value, since it does not exist in an untagged build.
//
// No process is spawned and no scan root is resolved: the test's own working directory is the package directory,
// so the Test Tier Purity Invariant is satisfied with no allowlist entry.

package shuttlecli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// modelPinConstantName is the shared constant every real-`claude` spawn site in the smoke suite must thread.
// Named as a string, not an identifier, because the constant is declared in a `//go:build llm` file.
const modelPinConstantName = "smokeClaudeModel"

// wantSpawnSiteCount is how many sites in the smoke suite start a real `claude`.
// The scan fails loudly when its count differs, so a recogniser that stops matching cannot pass vacuously.
// Raise it in the same commit as a new smoke test, never ahead of one.
const wantSpawnSiteCount = 15

// spawnSiteOpeners recognises the two shapes a real-`claude` spawn takes in this package's smoke suite.
// Each shape is paired with the bracket that closes its argument list.
// A bare `RunCLI(` is this package's own seam for `lym shuttle run`.
// A qualified `reedcli.RunCLI(` is a reed verb that spawns no provider, so the pattern does not match it.
// The scan keeps a RunCLI call only when its argument list carries the "run" verb.
var spawnSiteOpeners = []struct {
	name    string
	pattern *regexp.Regexp
	closer  byte
}{
	{"shuttlecli.RunCLI run verb", regexp.MustCompile(`(^|[^.\w])RunCLI\(`), ')'},
	{"shuttleengine.Spec literal", regexp.MustCompile(`shuttleengine\.Spec\{`), '}'},
}

// TestSmokeSuite_EveryRealClaudeSpawnPinsTheModel fails when a real-claude spawn site omits the model pin.
//
// Nothing else in the suite fails when a site drops the pin:
// build, vet and the hermetic tests stay green, and the only symptom is a larger bill on the next `-tags llm` run.
// The defect is invisible while it costs money.
//
//testtiming:keep a guard that fires when a real-claude spawn site omits the model pin, which no covering test asserts
func TestSmokeSuite_EveryRealClaudeSpawnPinsTheModel(t *testing.T) {
	tiergate.Require(t, 0)
	smokeFiles, err := filepath.Glob("smoke_*_test.go")
	if err != nil {
		t.Fatalf("glob smoke files: %v", err)
	}
	if len(smokeFiles) == 0 {
		t.Fatal("no smoke_*_test.go files found; the scan would pass vacuously")
	}

	foundSites := 0
	for _, file := range smokeFiles {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatalf("read %s: %v", file, readErr)
		}
		source := string(data)

		for _, opener := range spawnSiteOpeners {
			for _, match := range opener.pattern.FindAllStringIndex(source, -1) {
				// The match's end is just past its opening bracket, which is where the argument list starts.
				argumentList, ok := bracketedSpan(source, match[1]-1, opener.closer)
				if !ok {
					t.Fatalf("%s: unbalanced %s at byte %d; the scan cannot be trusted", file, opener.name, match[0])
				}
				// A RunCLI call without the "run" verb starts no provider process, so the scan skips it.
				if opener.closer == ')' && !strings.Contains(argumentList, `"run"`) {
					continue
				}
				foundSites++
				if !strings.Contains(argumentList, modelPinConstantName) {
					t.Errorf("%s: a %s starts a real claude process without threading %s — every live spawn in this suite must pin the cheap model, and a site that omits it costs real money on the next -tags llm run with nothing else going red. Site:\n%s",
						file, opener.name, modelPinConstantName, argumentList)
				}
			}
		}
	}

	if foundSites != wantSpawnSiteCount {
		t.Errorf("recognised %d real-claude spawn sites across %v; want %d — either a smoke test was added or removed without updating wantSpawnSiteCount, or the recogniser stopped matching and this guard is now passing vacuously",
			foundSites, smokeFiles, wantSpawnSiteCount)
	}
}

// TestSmokeSuite_ModelPinConstantIsDeclaredAndNonEmpty fails when the model pin constant is missing or empty.
// Without this check, deleting the declaration and every reference to it together would go unnoticed.
//
//testtiming:keep a guard that fires when the model-pin constant is missing or empty, which no covering test asserts
func TestSmokeSuite_ModelPinConstantIsDeclaredAndNonEmpty(t *testing.T) {
	tiergate.Require(t, 0)
	smokeFiles, err := filepath.Glob("smoke_*_test.go")
	if err != nil {
		t.Fatalf("glob smoke files: %v", err)
	}

	// The capture is `*`, not `+`, so an empty-string declaration still matches.
	// The match is reported as the empty pin it is, not as a missing declaration, which would misdescribe the change.
	declaration := regexp.MustCompile(`const\s+` + modelPinConstantName + `\s*=\s*"([^"]*)"`)
	for _, file := range smokeFiles {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatalf("read %s: %v", file, readErr)
		}
		if match := declaration.FindStringSubmatch(string(data)); match != nil {
			if match[1] == "" {
				t.Errorf("%s declares %s as the empty string; an empty --model defers to the account default, which is what the pin exists to prevent", file, modelPinConstantName)
			}
			return
		}
	}
	t.Errorf("no smoke file declares a non-empty %s constant; the model pin the whole suite depends on is gone", modelPinConstantName)
}

// bracketedSpan returns the substring of source from the opening bracket at openIndex through its balancing closer.
// It reports whether that closer was found.
// It skips interpreted and raw string literals and rune literals while balancing.
// A bracket inside a prompt string cannot throw the count off, and smoke prompts routinely contain them.
func bracketedSpan(source string, openIndex int, closer byte) (string, bool) {
	opener := source[openIndex]
	depth := 0
	for i := openIndex; i < len(source); i++ {
		switch c := source[i]; c {
		case '"', '`', '\'':
			end, ok := literalEnd(source, i)
			if !ok {
				return "", false
			}
			i = end
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return source[openIndex : i+1], true
			}
		}
	}
	return "", false
}

// literalEnd returns the index of the closing quote of the Go literal starting at startIndex.
// It reports whether that quote was found.
// A raw string (backtick) has no escapes; the other two honour backslash escapes.
func literalEnd(source string, startIndex int) (int, bool) {
	quote := source[startIndex]
	for i := startIndex + 1; i < len(source); i++ {
		if quote != '`' && source[i] == '\\' {
			i++
			continue
		}
		if source[i] == quote {
			return i, true
		}
	}
	return 0, false
}
