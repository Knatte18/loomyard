// cli_test.go drives RunCLI through its seam: the bare/--help subcommand listing, every command's
// Short, and the ErrNoLanguage error-envelope path.
// It is deliberately untagged, offline, and spawn-free: it never shells out to a subprocess, never
// touches git, and never copies a fixture tree, so it never launches a language server or requires
// a git repo.
// A real "refs" query against a live language server belongs to the //go:build integration tier
// (internal/scoutengine's own integration test) and batch 4's measurement, not here.

package scoutcli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/scoutengine"
)

// TestRunCLI_NoArgsListsRefsSubcommand verifies bare "lyx scout" lists subcommands and exits 0.
func TestRunCLI_NoArgsListsRefsSubcommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{})

	if exitCode != 0 {
		t.Errorf("RunCLI() = %d; want 0 for no-arg listing", exitCode)
	}
	if got := out.String(); !strings.Contains(got, "refs") {
		t.Errorf("RunCLI() no-arg output missing subcommand %q; got: %q", "refs", got)
	}
}

// TestRunCLI_Help verifies "lyx scout --help" lists subcommands and exits 0.
func TestRunCLI_Help(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"--help"})

	if exitCode != 0 {
		t.Errorf("RunCLI(--help) = %d; want 0", exitCode)
	}
	if got := out.String(); !strings.Contains(got, "refs") {
		t.Errorf("RunCLI(--help) output missing subcommand %q; got: %q", "refs", got)
	}
}

// TestCommand_EveryCommandHasShort asserts every command node has a non-empty Short.
func TestCommand_EveryCommandHasShort(t *testing.T) {
	t.Parallel()

	violations := collectMissingShorts(Command())
	for _, v := range violations {
		t.Errorf("command %q has no Short description", v)
	}
}

// collectMissingShorts returns command paths for every node whose Short is empty.
func collectMissingShorts(cmd *cobra.Command) []string {
	var violations []string
	if cmd.Short == "" {
		violations = append(violations, cmd.CommandPath())
	}
	for _, child := range cmd.Commands() {
		violations = append(violations, collectMissingShorts(child)...)
	}
	return violations
}

// TestRunCLI_Refs_NoLanguageError verifies "refs" fails with ErrNoLanguage in an empty directory.
func TestRunCLI_Refs_NoLanguageError(t *testing.T) {
	// Chdir into a fresh, non-git temp dir so lyxcwd.Resolve degrades to
	// scoutengine.BuiltinRegistry() deterministically, independent of
	// whatever git repo or servers.yaml the test happens to run inside.
	t.Chdir(t.TempDir())

	emptyTargetDir := t.TempDir()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"refs", "MySymbol", "--target-dir", emptyTargetDir})

	if exitCode == 0 {
		t.Fatalf("RunCLI(refs MySymbol --target-dir <empty>) = 0; want non-zero exit for ErrNoLanguage")
	}

	// Assert the JSON envelope shape: exactly one object on one line, ok=false,
	// and a populated, non-empty error field.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("RunCLI output has %d lines; want exactly 1. output:\n%s", len(lines), out.String())
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &env); err != nil {
		t.Fatalf("RunCLI output is not valid JSON: %v; got: %q", err, lines[0])
	}

	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(refs MySymbol --target-dir <empty>) ok = true; want false")
	}

	errMsg, _ := env["error"].(string)
	if errMsg == "" {
		t.Errorf("RunCLI(refs MySymbol --target-dir <empty>) error field empty or missing; got envelope: %v", env)
	}
	if !strings.Contains(errMsg, "no language detected") {
		t.Errorf("RunCLI(refs MySymbol --target-dir <empty>) error = %q; want it to mention ErrNoLanguage's \"no language detected\"", errMsg)
	}
}

// TestRunCLIIn_TargetDirResolvesAgainstInjectedSeamCwd proves the --target-dir defaulting rebase
// reaches a consumer: a relative --target-dir resolves against the seam cwd RunCLIIn injects,
// never the process cwd, and an absolute --target-dir is honoured unchanged.
// DetectLanguage's ErrNoLanguage message names the resolved targetDir verbatim ("searched markers
// ... under %s"), so it doubles as the observation point without needing any marker file on disk.
func TestRunCLIIn_TargetDirResolvesAgainstInjectedSeamCwd(t *testing.T) {
	t.Parallel()

	seamCwd := t.TempDir()

	var out bytes.Buffer
	exitCode := RunCLIIn(seamCwd, &out, []string{"refs", "MySymbol", "--target-dir", "sub"})
	if exitCode == 0 {
		t.Fatalf("RunCLIIn(seamCwd, refs MySymbol --target-dir sub) = 0; want non-zero exit for ErrNoLanguage")
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLIIn output is not valid JSON: %v; got: %q", err, out.String())
	}
	errMsg, _ := env["error"].(string)
	wantRelResolved := filepath.Join(seamCwd, "sub")
	if !strings.Contains(errMsg, wantRelResolved) {
		t.Errorf("RunCLIIn(seamCwd, refs --target-dir \"sub\") error = %q; want it to reference the seam-cwd-resolved dir %q", errMsg, wantRelResolved)
	}

	out.Reset()
	absDir := t.TempDir()
	exitCode = RunCLIIn(seamCwd, &out, []string{"refs", "MySymbol", "--target-dir", absDir})
	if exitCode == 0 {
		t.Fatalf("RunCLIIn(seamCwd, refs MySymbol --target-dir %s) = 0; want non-zero exit for ErrNoLanguage", absDir)
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLIIn output is not valid JSON: %v; got: %q", err, out.String())
	}
	errMsg, _ = env["error"].(string)
	if !strings.Contains(errMsg, absDir) {
		t.Errorf("RunCLIIn(seamCwd, refs --target-dir %s) error = %q; want it to reference the absolute dir unchanged", absDir, errMsg)
	}
}

// TestRunCLI_Definition_NoLanguageError verifies "definition" fails with ErrNoLanguage in an empty
// directory.
func TestRunCLI_Definition_NoLanguageError(t *testing.T) {
	// Chdir into a fresh, non-git temp dir so lyxcwd.Resolve degrades to
	// scoutengine.BuiltinRegistry() deterministically, independent of
	// whatever git repo or servers.yaml the test happens to run inside.
	t.Chdir(t.TempDir())

	emptyTargetDir := t.TempDir()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"definition", "MySymbol", "--target-dir", emptyTargetDir})

	if exitCode == 0 {
		t.Fatalf("RunCLI(definition MySymbol --target-dir <empty>) = 0; want non-zero exit for ErrNoLanguage")
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("RunCLI output has %d lines; want exactly 1. output:\n%s", len(lines), out.String())
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &env); err != nil {
		t.Fatalf("RunCLI output is not valid JSON: %v; got: %q", err, lines[0])
	}

	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(definition MySymbol --target-dir <empty>) ok = true; want false")
	}

	errMsg, _ := env["error"].(string)
	if errMsg == "" {
		t.Errorf("RunCLI(definition MySymbol --target-dir <empty>) error field empty or missing; got envelope: %v", env)
	}
	if !strings.Contains(errMsg, "no language detected") {
		t.Errorf("RunCLI(definition MySymbol --target-dir <empty>) error = %q; want it to mention ErrNoLanguage's \"no language detected\"", errMsg)
	}
}

// TestRunCLI_Symbol_NoLanguageError verifies "symbol" fails with ErrNoLanguage in an empty
// directory.
func TestRunCLI_Symbol_NoLanguageError(t *testing.T) {
	// Chdir into a fresh, non-git temp dir so lyxcwd.Resolve degrades to
	// scoutengine.BuiltinRegistry() deterministically, independent of
	// whatever git repo or servers.yaml the test happens to run inside.
	t.Chdir(t.TempDir())

	emptyTargetDir := t.TempDir()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"symbol", "MySymbol", "--target-dir", emptyTargetDir})

	if exitCode == 0 {
		t.Fatalf("RunCLI(symbol MySymbol --target-dir <empty>) = 0; want non-zero exit for ErrNoLanguage")
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("RunCLI output has %d lines; want exactly 1. output:\n%s", len(lines), out.String())
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &env); err != nil {
		t.Fatalf("RunCLI output is not valid JSON: %v; got: %q", err, lines[0])
	}

	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(symbol MySymbol --target-dir <empty>) ok = true; want false")
	}

	errMsg, _ := env["error"].(string)
	if errMsg == "" {
		t.Errorf("RunCLI(symbol MySymbol --target-dir <empty>) error field empty or missing; got envelope: %v", env)
	}
	if !strings.Contains(errMsg, "no language detected") {
		t.Errorf("RunCLI(symbol MySymbol --target-dir <empty>) error = %q; want it to mention ErrNoLanguage's \"no language detected\"", errMsg)
	}
}

// TestRunCLI_Symbol_TreatsFileLineColArgumentAsLiteralSearchString proves symbolCommand never
// position-parses "file:line:col" arguments, treating them as literal search strings.
func TestRunCLI_Symbol_TreatsFileLineColArgumentAsLiteralSearchString(t *testing.T) {
	const arg = "foo.go:1:1"

	query := symbolQuery(arg)
	if query.Symbol != arg {
		t.Errorf("symbolQuery(%q).Symbol = %q; want %q", arg, query.Symbol, arg)
	}
	if query.Pos != nil {
		t.Errorf("symbolQuery(%q).Pos = %+v; want nil — the argument must never be position-parsed", arg, query.Pos)
	}

	// The same string, driven through parseQuery (the converter
	// refs/definition use), DOES parse as a position — proving symbolQuery's
	// literal-search-string behavior is a deliberate divergence, not an
	// accident of parseQuery(arg) happening to leave Pos unset for this
	// particular string.
	base := t.TempDir()
	parsed, err := parseQuery(base, arg)
	if err != nil {
		t.Fatalf("parseQuery(%q, %q) error = %v; want nil", base, arg, err)
	}
	if parsed.Pos == nil {
		t.Fatalf("parseQuery(%q).Pos = nil; want a parsed position, to prove symbolQuery's divergence from parseQuery is meaningful", arg)
	}

	t.Chdir(t.TempDir())
	emptyTargetDir := t.TempDir()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"symbol", arg, "--target-dir", emptyTargetDir})

	if exitCode == 0 {
		t.Fatalf("RunCLI(symbol %s --target-dir <empty>) = 0; want non-zero exit for ErrNoLanguage", arg)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI output is not valid JSON: %v; got: %q", err, out.String())
	}

	errMsg, _ := env["error"].(string)
	if !strings.Contains(errMsg, "no language detected") {
		t.Errorf("RunCLI(symbol %s --target-dir <empty>) error = %q; want it to mention ErrNoLanguage's \"no language detected\"", arg, errMsg)
	}
}

// TestEmitLookupResult_AmbiguousSymbolExitsTwo tests emitLookupResult's handling of ambiguous and
// not-found cases.
func TestEmitLookupResult_AmbiguousSymbolExitsTwo(t *testing.T) {
	tests := []struct {
		name         string
		resultsField string
		err          error
		wantCode     int
		wantOk       bool
		checkBody    func(t *testing.T, env map[string]any)
	}{
		{
			name:         "ambiguous",
			resultsField: "references",
			err: &scoutengine.ErrAmbiguousSymbol{
				Symbol:     "Foo",
				Candidates: []string{"a.go:1:1", "b.go:2:2"},
			},
			wantCode: 2,
			wantOk:   true,
			checkBody: func(t *testing.T, env map[string]any) {
				t.Helper()
				candidates, ok := env["candidates"].([]any)
				if !ok {
					t.Fatalf("envelope %v missing []any \"candidates\" field", env)
				}
				want := []string{"a.go:1:1", "b.go:2:2"}
				if len(candidates) != len(want) {
					t.Fatalf("candidates = %v; want %v", candidates, want)
				}
				for i, c := range candidates {
					if c != want[i] {
						t.Errorf("candidates[%d] = %v; want %v", i, c, want[i])
					}
				}
			},
		},
		{
			name:         "not_found",
			resultsField: "definitions",
			err:          &scoutengine.ErrSymbolNotFound{Symbol: "Bar", TargetDir: "/tmp"},
			wantCode:     1,
			wantOk:       false,
			checkBody: func(t *testing.T, env map[string]any) {
				t.Helper()
				errMsg, _ := env["error"].(string)
				if !strings.Contains(errMsg, "Bar") {
					t.Errorf("error = %q; want it to mention %q", errMsg, "Bar")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			ctx, es := clihelp.NewExitContext(context.Background())

			emitLookupResult(ctx, &out, tt.resultsField, nil, tt.err)

			if es.Code() != tt.wantCode {
				t.Errorf("es.Code() = %d; want %d", es.Code(), tt.wantCode)
			}

			var env map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
				t.Fatalf("emitLookupResult output is not valid JSON: %v; got: %q", err, out.String())
			}

			if ok, _ := env["ok"].(bool); ok != tt.wantOk {
				t.Errorf("envelope ok = %v; want %v", ok, tt.wantOk)
			}

			tt.checkBody(t, env)
		})
	}
}

// TestRunCLI_Refs_RequiresAtLeastOneArg verifies "refs" requires at least one argument.
func TestRunCLI_Refs_RequiresAtLeastOneArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"bare", []string{"refs"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := RunCLI(&out, tt.args)

			if exitCode == 0 {
				t.Fatalf("RunCLI(%v) = 0; want non-zero exit for arg-count violation", tt.args)
			}

			var env map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
				t.Fatalf("RunCLI(%v) output is not valid JSON: %v; got: %q", tt.args, err, out.String())
			}
			if ok, _ := env["ok"].(bool); ok {
				t.Errorf("RunCLI(%v) ok = true; want false", tt.args)
			}
		})
	}
}

// TestRunCLI_Refs_TwoArgsIsBatchMode verifies two or more arguments enable batch mode.
func TestRunCLI_Refs_TwoArgsIsBatchMode(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"refs", "one", "two", "--target-dir", t.TempDir()})

	if exitCode != 3 {
		t.Fatalf("RunCLI(refs one two --target-dir <empty>) = %d; want 3 (worst-outcome rank for an all-error batch)", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI output is not valid JSON: %v; got: %q", err, out.String())
	}

	results, ok := env["results"].([]any)
	if !ok {
		t.Fatalf("envelope %v missing []any \"results\" field", env)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d; want 2", len(results))
	}

	wantSymbols := []string{"one", "two"}
	for i, r := range results {
		entry, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %v; want a JSON object", i, r)
		}
		if status, _ := entry["status"].(string); status != "error" {
			t.Errorf("results[%d][\"status\"] = %q; want \"error\"", i, status)
		}
		if symbol, _ := entry["symbol"].(string); symbol != wantSymbols[i] {
			t.Errorf("results[%d][\"symbol\"] = %q; want %q", i, symbol, wantSymbols[i])
		}
	}
}

// TestBatchRunner_WorstOutcomeWinsExitCode verifies runBatch sets exit code to the worst status
// present.
func TestBatchRunner_WorstOutcomeWinsExitCode(t *testing.T) {
	t.Parallel()

	outcomes := map[string]struct {
		status batchStatus
		fields map[string]any
	}{
		"a": {statusFound, map[string]any{"references": []any{}}},
		"b": {statusNotFound, nil},
		"c": {statusAmbiguous, map[string]any{"candidates": []string{"x"}}},
		"d": {statusError, map[string]any{"error": "boom"}},
	}
	lookupOne := func(symbol string) (batchStatus, map[string]any) {
		o := outcomes[symbol]
		return o.status, o.fields
	}

	tests := []struct {
		name       string
		symbols    []string
		wantCode   int
		wantStatus []string
	}{
		{"all_found", []string{"a"}, 0, []string{"found"}},
		{"found_and_not_found", []string{"a", "b"}, 1, []string{"found", "not_found"}},
		{"found_not_found_ambiguous", []string{"a", "b", "c"}, 2, []string{"found", "not_found", "ambiguous"}},
		{"found_not_found_ambiguous_error", []string{"a", "b", "c", "d"}, 3, []string{"found", "not_found", "ambiguous", "error"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			ctx, es := clihelp.NewExitContext(context.Background())

			runBatch(ctx, &out, tt.symbols, lookupOne)

			if es.Code() != tt.wantCode {
				t.Errorf("es.Code() = %d; want %d", es.Code(), tt.wantCode)
			}

			var env map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
				t.Fatalf("runBatch output is not valid JSON: %v; got: %q", err, out.String())
			}

			results, ok := env["results"].([]any)
			if !ok {
				t.Fatalf("envelope %v missing []any \"results\" field", env)
			}
			if len(results) != len(tt.wantStatus) {
				t.Fatalf("len(results) = %d; want %d", len(results), len(tt.wantStatus))
			}
			for i, r := range results {
				entry, ok := r.(map[string]any)
				if !ok {
					t.Fatalf("results[%d] = %v; want a JSON object", i, r)
				}
				if status, _ := entry["status"].(string); status != tt.wantStatus[i] {
					t.Errorf("results[%d][\"status\"] = %q; want %q", i, status, tt.wantStatus[i])
				}
			}
		})
	}
}

// TestEmitLookupResult_SuccessCarriesResolutionCompleteMarker verifies success includes
// "resolution":"complete".
func TestEmitLookupResult_SuccessCarriesResolutionCompleteMarker(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	ctx, es := clihelp.NewExitContext(context.Background())

	emitLookupResult(ctx, &out, "references", []scoutengine.Reference{{File: "a.go", Line: 1, Character: 2}}, nil)

	if es.Code() != 0 {
		t.Errorf("es.Code() = %d; want 0", es.Code())
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("emitLookupResult output is not valid JSON: %v; got: %q", err, out.String())
	}

	if resolution, _ := env["resolution"].(string); resolution != "complete" {
		t.Errorf("envelope %v missing \"resolution\":\"complete\"; got %q", env, resolution)
	}
}

// TestClassifyLookupError_FoundCarriesResolutionCompleteMarker verifies statusFound includes
// "resolution":"complete".
func TestClassifyLookupError_FoundCarriesResolutionCompleteMarker(t *testing.T) {
	t.Parallel()

	status, fields := classifyLookupError(nil, "references", []scoutengine.Reference{{File: "a.go", Line: 1, Character: 2}})

	if status != statusFound {
		t.Errorf("classifyLookupError(nil, ...) status = %q; want %q", status, statusFound)
	}
	if resolution, _ := fields["resolution"].(string); resolution != "complete" {
		t.Errorf("classifyLookupError(nil, ...) fields = %v; missing \"resolution\":\"complete\"", fields)
	}

	ambiguousStatus, ambiguousFields := classifyLookupError(&scoutengine.ErrAmbiguousSymbol{Symbol: "Foo", Candidates: []string{"a.go:1:1"}}, "references", nil)
	if ambiguousStatus != statusAmbiguous {
		t.Fatalf("classifyLookupError(ambiguous, ...) status = %q; want %q", ambiguousStatus, statusAmbiguous)
	}
	if _, ok := ambiguousFields["resolution"]; ok {
		t.Errorf("classifyLookupError(ambiguous, ...) fields = %v; want no \"resolution\" field", ambiguousFields)
	}
}

// TestLookupContext_OutsideHubReturnsAbsoluteAnchorRootAndBuiltinRegistry verifies lookupContext's
// out-of-hub path: an absolute-path anchor-root string, and the built-in registry (spot-checked on
// one entry rather than reflect.DeepEqual over the whole map, so the test does not couple to every
// future built-in entry). This is the test that observes the actual defect assert-no-callers had
// before this refactor: the equivalent derivation on this exact path used to yield an empty string,
// and because all four commands now share lookupContext, one test covers all four.
//
// It covers both an explicit dir (the anchor root is filepath.Abs(dir)) and a dir defaulted from
// cwd (the anchor root is filepath.Abs(cwd), not filepath.Abs(""))
func TestLookupContext_OutsideHubReturnsAbsoluteAnchorRootAndBuiltinRegistry(t *testing.T) {
	t.Run("explicit dir", func(t *testing.T) {
		cwd := t.TempDir()
		dir := t.TempDir()

		registry, anchorRoot, err := lookupContext(cwd, dir)
		if err != nil {
			t.Fatalf("lookupContext(%q, %q) error = %v; want nil", cwd, dir, err)
		}

		wantAbs, err := filepath.Abs(dir)
		if err != nil {
			t.Fatalf("filepath.Abs(%q) error = %v", dir, err)
		}
		if anchorRoot != wantAbs {
			t.Errorf("lookupContext(%q, %q) anchorRoot = %q; want %q", cwd, dir, anchorRoot, wantAbs)
		}

		if got, want := registry["go"], scoutengine.BuiltinRegistry()["go"]; !reflect.DeepEqual(got, want) {
			t.Errorf("lookupContext(%q, %q) registry[\"go\"] = %+v; want %+v", cwd, dir, got, want)
		}
	})

	t.Run("dir defaulted from cwd", func(t *testing.T) {
		cwd := t.TempDir()
		dir := cwd

		registry, anchorRoot, err := lookupContext(cwd, dir)
		if err != nil {
			t.Fatalf("lookupContext(%q, %q) error = %v; want nil", cwd, dir, err)
		}

		wantAbs, err := filepath.Abs(cwd)
		if err != nil {
			t.Fatalf("filepath.Abs(%q) error = %v", cwd, err)
		}
		if anchorRoot != wantAbs {
			t.Errorf("lookupContext(%q, %q) anchorRoot = %q; want %q (filepath.Abs(cwd), not filepath.Abs(\"\"))", cwd, dir, anchorRoot, wantAbs)
		}

		if got, want := registry["go"], scoutengine.BuiltinRegistry()["go"]; !reflect.DeepEqual(got, want) {
			t.Errorf("lookupContext(%q, %q) registry[\"go\"] = %+v; want %+v", cwd, dir, got, want)
		}
	})
}

// TestBuildOptions_ThreadsEveryFieldFromItsArguments verifies buildOptions threads all fields
// correctly.
func TestBuildOptions_ThreadsEveryFieldFromItsArguments(t *testing.T) {
	t.Parallel()

	registry := scoutengine.BuiltinRegistry()
	query := scoutengine.Query{Symbol: "Foo"}
	anchorRoot := "/worktree/root"

	got := buildOptions(registry, "/target", anchorRoot, "go", query, 5*time.Second)

	if got.TargetDir != "/target" {
		t.Errorf("buildOptions(...).TargetDir = %q; want %q", got.TargetDir, "/target")
	}
	if got.AnchorRoot != anchorRoot {
		t.Errorf("buildOptions(...).AnchorRoot = %q; want %q", got.AnchorRoot, anchorRoot)
	}
	if got.Lang != "go" {
		t.Errorf("buildOptions(...).Lang = %q; want %q", got.Lang, "go")
	}
	if got.Query != query {
		t.Errorf("buildOptions(...).Query = %+v; want %+v", got.Query, query)
	}
	if got.Timeout != 5*time.Second {
		t.Errorf("buildOptions(...).Timeout = %v; want %v", got.Timeout, 5*time.Second)
	}
}

// TestInFileQuery_ProducesInFileNeverPosEvenForFileLineColShapedName proves inFileQuery never
// position-parses.
func TestInFileQuery_ProducesInFileNeverPosEvenForFileLineColShapedName(t *testing.T) {
	t.Parallel()

	const name = "foo.go:1:1"

	base := t.TempDir()
	query, err := inFileQuery(base, "internal/foo/bar.go", name)
	if err != nil {
		t.Fatalf("inFileQuery(%q, %q, %q) error = %v; want nil", base, "internal/foo/bar.go", name, err)
	}

	if query.Pos != nil {
		t.Errorf("inFileQuery(...).Pos = %+v; want nil — the name must never be position-parsed", query.Pos)
	}
	if query.InFile == nil {
		t.Fatalf("inFileQuery(...).InFile = nil; want a populated *InFileQuery")
	}
	if query.InFile.Name != name {
		t.Errorf("inFileQuery(...).InFile.Name = %q; want %q", query.InFile.Name, name)
	}
	if !filepath.IsAbs(query.InFile.File) {
		t.Errorf("inFileQuery(...).InFile.File = %q; want an absolute path", query.InFile.File)
	}
}

// TestInFileQuery_ResolvesRelativePathToAbsolute verifies a relative path resolves against the
// explicit base argument rather than the process cwd.
func TestInFileQuery_ResolvesRelativePathToAbsolute(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()

	query, err := inFileQuery(cwd, "relative/bar.go", "MyFunc")
	if err != nil {
		t.Fatalf("inFileQuery(%q, %q, %q) error = %v; want nil", cwd, "relative/bar.go", "MyFunc", err)
	}

	want := filepath.Join(cwd, "relative/bar.go")
	if query.InFile == nil || query.InFile.File != want {
		t.Errorf("inFileQuery(...).InFile.File = %+v; want %q", query.InFile, want)
	}
}

// TestInFileFlag_RegisteredOnRefsAndDefinitionOnlyNotSymbol verifies --in-file is on
// refs/definition but not symbol.
func TestInFileFlag_RegisteredOnRefsAndDefinitionOnlyNotSymbol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     *cobra.Command
		wantHas bool
	}{
		{"refs", refsCommand(), true},
		{"definition", definitionCommand(), true},
		{"symbol", symbolCommand(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasFlag := tt.cmd.Flags().Lookup("in-file") != nil
			if hasFlag != tt.wantHas {
				t.Errorf("%s command has --in-file registered = %v; want %v", tt.name, hasFlag, tt.wantHas)
			}
		})
	}
}

// TestFilterWithin tests the --within filtering logic, which mitigates interface-method reference
// conflation.
func TestFilterWithin(t *testing.T) {
	t.Parallel()

	inScope1 := scoutengine.Reference{File: "/repo/internal/websterengine/poll.go", Line: 203}
	inScope2 := scoutengine.Reference{File: "/repo/internal/websterengine/state.go", Line: 10}
	crossPackage := scoutengine.Reference{File: "/repo/internal/perchengine/identity.go", Line: 44}
	// A sibling directory whose name merely starts with the same prefix —
	// proves filterWithin does not fall back to a naive string-prefix
	// check, which would wrongly treat "internal/webster" as containing
	// anything under "internal/websterengine" (they share no path
	// component boundary in common beyond the literal substring).
	prefixCollision := scoutengine.Reference{File: "/repo/internal/webstercli/cli.go", Line: 5}

	tests := []struct {
		name     string
		within   string
		baseDir  string
		refs     []scoutengine.Reference
		wantRefs []scoutengine.Reference
	}{
		{
			name:     "absolute_within_keeps_only_in_scope",
			within:   "/repo/internal/websterengine",
			baseDir:  "/anything", // unused: within is already absolute
			refs:     []scoutengine.Reference{inScope1, inScope2, crossPackage, prefixCollision},
			wantRefs: []scoutengine.Reference{inScope1, inScope2},
		},
		{
			name:     "relative_within_resolves_against_baseDir",
			within:   "internal/websterengine",
			baseDir:  "/repo",
			refs:     []scoutengine.Reference{inScope1, crossPackage},
			wantRefs: []scoutengine.Reference{inScope1},
		},
		{
			name:     "prefix_collision_directory_excluded",
			within:   "/repo/internal/webster",
			baseDir:  "/anything",
			refs:     []scoutengine.Reference{prefixCollision},
			wantRefs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := filterWithin(tt.refs, tt.within, tt.baseDir)
			if len(got) != len(tt.wantRefs) {
				t.Fatalf("filterWithin() = %v; want %v", got, tt.wantRefs)
			}
			for i := range got {
				if got[i] != tt.wantRefs[i] {
					t.Errorf("filterWithin()[%d] = %v; want %v", i, got[i], tt.wantRefs[i])
				}
			}
		})
	}
}

// TestClassifySymbolError_MultipleMatchesIsFoundNotAmbiguous verifies symbol never produces
// ambiguous status.
func TestClassifySymbolError_MultipleMatchesIsFoundNotAmbiguous(t *testing.T) {
	t.Parallel()

	status, fields := classifySymbolError(nil, []scoutengine.SymbolMatch{{Name: "Foo"}, {Name: "FooBar"}})

	if status != statusFound {
		t.Errorf("classifySymbolError(nil, <2 matches>) status = %q; want %q", status, statusFound)
	}

	symbols, ok := fields["symbols"].([]map[string]any)
	if !ok {
		t.Fatalf("fields %v missing []map[string]any \"symbols\" field", fields)
	}
	if len(symbols) != 2 {
		t.Fatalf("len(symbols) = %d; want 2", len(symbols))
	}
	wantNames := []string{"Foo", "FooBar"}
	for i, s := range symbols {
		if name, _ := s["name"].(string); name != wantNames[i] {
			t.Errorf("symbols[%d][\"name\"] = %q; want %q", i, name, wantNames[i])
		}
	}
}

// TestRunCLI_AssertNoCallers_NoLanguageError verifies "assert-no-callers" fails with ErrNoLanguage
// in an empty directory.
func TestRunCLI_AssertNoCallers_NoLanguageError(t *testing.T) {
	// Chdir into a fresh, non-git temp dir so lyxcwd.Resolve degrades to
	// scoutengine.BuiltinRegistry() deterministically, independent of
	// whatever git repo or servers.yaml the test happens to run inside.
	t.Chdir(t.TempDir())

	emptyTargetDir := t.TempDir()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"assert-no-callers", "MySymbol", "--target-dir", emptyTargetDir})

	if exitCode == 0 {
		t.Fatalf("RunCLI(assert-no-callers MySymbol --target-dir <empty>) = 0; want non-zero exit for ErrNoLanguage")
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("RunCLI output has %d lines; want exactly 1. output:\n%s", len(lines), out.String())
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &env); err != nil {
		t.Fatalf("RunCLI output is not valid JSON: %v; got: %q", err, lines[0])
	}

	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(assert-no-callers MySymbol --target-dir <empty>) ok = true; want false")
	}
	if env["violation"] != nil {
		t.Errorf(`RunCLI(assert-no-callers MySymbol --target-dir <empty>) envelope carries "violation"; want absent for a lookup failure, not a real violation`)
	}

	errMsg, _ := env["error"].(string)
	if !strings.Contains(errMsg, "no language detected") {
		t.Errorf("RunCLI(assert-no-callers MySymbol --target-dir <empty>) error = %q; want it to mention ErrNoLanguage's \"no language detected\"", errMsg)
	}
}

// TestRunCLI_AssertNoCallers_RequiresExactlyOneArg verifies "assert-no-callers" requires exactly
// one argument.
func TestRunCLI_AssertNoCallers_RequiresExactlyOneArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"bare", []string{"assert-no-callers"}},
		{"two_args", []string{"assert-no-callers", "Foo", "Bar"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := RunCLI(&out, tt.args)

			if exitCode == 0 {
				t.Errorf("RunCLI(%v) = 0; want non-zero exit for wrong arg count", tt.args)
			}
		})
	}
}

// TestFilterUnexpectedCallers tests the filtering logic for unexpected callers.
func TestFilterUnexpectedCallers(t *testing.T) {
	t.Parallel()

	decl := scoutengine.Reference{File: "/repo/pkg/foo.go", Line: 10, Character: 6}
	wrapper := scoutengine.Reference{File: "/repo/pkg/wrapper.go", Line: 20, Character: 3}
	caller := scoutengine.Reference{File: "/repo/other/bar.go", Line: 5, Character: 12}

	tests := []struct {
		name      string
		refs      []scoutengine.Reference
		declRefs  []scoutengine.Reference
		exceptAbs map[string]bool
		want      []scoutengine.Reference
	}{
		{
			name:     "declaration_only_is_clean",
			refs:     []scoutengine.Reference{decl},
			declRefs: []scoutengine.Reference{decl},
			want:     nil,
		},
		{
			name:      "except_path_is_excluded",
			refs:      []scoutengine.Reference{decl, wrapper},
			declRefs:  []scoutengine.Reference{decl},
			exceptAbs: map[string]bool{"/repo/pkg/wrapper.go": true},
			want:      nil,
		},
		{
			name:     "unexpected_caller_survives",
			refs:     []scoutengine.Reference{decl, wrapper, caller},
			declRefs: []scoutengine.Reference{decl},
			exceptAbs: map[string]bool{
				"/repo/pkg/wrapper.go": true,
			},
			want: []scoutengine.Reference{caller},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := filterUnexpectedCallers(tt.refs, tt.declRefs, tt.exceptAbs)
			if len(got) != len(tt.want) {
				t.Fatalf("filterUnexpectedCallers() = %v; want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("filterUnexpectedCallers()[%d] = %v; want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
