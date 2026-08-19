// modelpin_test.go is R2-F3's regression guard: every site in this package's smoke suite that
// starts a REAL `claude` process must thread the shared smokeClaudeModel constant, so no future edit
// can silently put a live spawn back on the account-default model.
//
// It is an UNTAGGED source scan on purpose. The smoke files are behind `//go:build smoke`, so an
// assertion written inside them only runs on the rare `-tags smoke` invocation — which is exactly
// the run that would already have spent the money. Reading them as text runs on every ordinary
// `go test`, before any spawn happens. That also means the constant can only be referred to by name
// here, never used as a value, since it does not exist in an untagged build.
//
// No process is spawned and no scan root is resolved: the test's own working directory IS the
// package directory, so the Test Tier Purity Invariant is satisfied with no allowlist entry.

package shuttlecli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// modelPinConstantName is the shared constant every real-`claude` spawn site in the smoke suite must
// thread. Named as a string rather than referenced as an identifier because it is declared in a
// `//go:build smoke` file and does not exist in this untagged build.
const modelPinConstantName = "smokeClaudeModel"

// wantSpawnSiteCount is how many sites in the smoke suite start a real `claude`, pinned so the scan
// fails loudly rather than passing vacuously if the recogniser stops matching. Raise it in the same
// commit as a new smoke test, never ahead of one.
// It is 4: `lyx shuttle run` through this package's own RunCLI in smoke_run_test.go and twice in
// smoke_guardrail_test.go, plus the direct Runner.Start in smoke_interrupt_test.go.
const wantSpawnSiteCount = 4

// spawnSiteOpeners recognises the two shapes a real-`claude` spawn takes in this package's smoke
// suite, each paired with the bracket that closes its argument list.
// A bare `RunCLI(` is this package's own seam (`lyx shuttle run`); a qualified one (`reedcli.RunCLI`)
// is a reed verb that spawns no provider, which is why the pattern requires the call to be
// unqualified AND to carry a "run" verb.
var spawnSiteOpeners = []struct {
	name    string
	pattern *regexp.Regexp
	closer  byte
}{
	{"shuttlecli.RunCLI run verb", regexp.MustCompile(`(^|[^.\w])RunCLI\(`), ')'},
	{"shuttleengine.Spec literal", regexp.MustCompile(`shuttleengine\.Spec\{`), '}'},
}

// TestSmokeSuite_EveryRealClaudeSpawnPinsTheModel is the guard itself.
//
// Round 1 pinned all four smoke spawns to the cheap model via a shared constant but added nothing
// that fails when a site stops referencing it: deleting `"--model", smokeClaudeModel,` from any one
// call left build, vet, and the whole hermetic suite green, and the only symptom was a larger bill
// on the next `-tags smoke` run — a defect that is invisible precisely while it is costing money.
func TestSmokeSuite_EveryRealClaudeSpawnPinsTheModel(t *testing.T) {
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
				// FindAllStringIndex's end is just past the opening bracket, which is where the
				// argument list starts.
				argumentList, ok := bracketedSpan(source, match[1]-1, opener.closer)
				if !ok {
					t.Fatalf("%s: unbalanced %s at byte %d; the scan cannot be trusted", file, opener.name, match[0])
				}
				// A RunCLI call that is not the `run` verb (there is none today, but `interrupt`
				// and `send` are the obvious future additions) starts no provider process.
				if opener.closer == ')' && !strings.Contains(argumentList, `"run"`) {
					continue
				}
				foundSites++
				if !strings.Contains(argumentList, modelPinConstantName) {
					t.Errorf("%s: a %s starts a real claude process without threading %s — every live spawn in this suite must pin the cheap model, and a site that omits it costs real money on the next -tags smoke run with nothing else going red. Site:\n%s",
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

// TestSmokeSuite_ModelPinConstantIsDeclaredAndNonEmpty pins the other half: the constant every site
// threads must actually exist and carry a value.
// Without this, deleting the declaration and the references together would leave the scan above
// finding nothing to complain about.
func TestSmokeSuite_ModelPinConstantIsDeclaredAndNonEmpty(t *testing.T) {
	smokeFiles, err := filepath.Glob("smoke_*_test.go")
	if err != nil {
		t.Fatalf("glob smoke files: %v", err)
	}

	// The capture is `*`, not `+`, so an empty-string declaration still MATCHES and is reported as
	// the empty pin it is, rather than falling through to the "no declaration at all" message below
	// and misdescribing what changed.
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

// bracketedSpan returns the substring of source from the opening bracket at openIndex through its
// balancing closer, and reports whether that closer was found.
// It skips over interpreted and raw string literals and over rune literals while balancing, so a
// bracket inside a prompt string cannot throw the count off — smoke prompts routinely contain them.
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

// literalEnd returns the index of the closing quote of the Go literal starting at startIndex, and
// reports whether it was found.
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
