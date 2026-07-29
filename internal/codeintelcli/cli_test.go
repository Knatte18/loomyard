// cli_test.go drives RunCLI through its seam: the bare/--help subcommand listing,
// every command's Short, and the ErrNoLanguage error-envelope path. It is
// deliberately untagged, offline, and spawn-free: it never shells out to a
// subprocess, never touches git, and never copies a fixture tree, so it never
// launches a language server or requires a git repo. A real "refs" query against a
// live language server belongs to the //go:build integration tier
// (internal/codeintelengine's own integration test) and batch 4's measurement, not
// here.

package codeintelcli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/codeintelengine"
)

// TestRunCLI_NoArgsListsRefsSubcommand verifies that "lyx codeintel" with no
// subcommand lists the "refs" subcommand and exits 0 — matching every other
// module group's bare-invocation behavior (clihelp.GroupRunE).
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

// TestRunCLI_Help verifies that "lyx codeintel --help" also lists the "refs"
// subcommand and exits 0, mirroring the bare-invocation assertion above for the
// explicit --help path.
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

// TestCommand_EveryCommandHasShort walks the full command tree returned by
// Command() and asserts every node (the "codeintel" group and each subcommand)
// carries a non-empty Short — the same structural self-documentation contract
// cmd/lyx/drift_test.go enforces repo-wide, checked locally here so this module's
// own test suite catches a missing Short before the root-level guard would.
func TestCommand_EveryCommandHasShort(t *testing.T) {
	t.Parallel()

	violations := collectMissingShorts(Command())
	for _, v := range violations {
		t.Errorf("command %q has no Short description", v)
	}
}

// collectMissingShorts performs a depth-first walk of the command tree rooted at
// cmd and returns the command path of every node whose Short is empty.
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

// TestRunCLI_Refs_NoLanguageError verifies that "refs <symbol> --target-dir
// <empty dir>" fails through the ErrNoLanguage path: an empty temp dir has no
// registry markers, so DetectLanguage fails before References ever launches a
// language server. This exercises the engine-error-to-output.Err mapping without
// any subprocess spawn.
func TestRunCLI_Refs_NoLanguageError(t *testing.T) {
	// Chdir into a fresh, non-git temp dir so hubgeometry.Resolve degrades to
	// codeintelengine.BuiltinRegistry() deterministically, independent of
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

// TestRunCLI_Definition_NoLanguageError verifies that "definition <symbol>
// --target-dir <empty dir>" fails through the same ErrNoLanguage path
// TestRunCLI_Refs_NoLanguageError exercises for "refs" — definitionCommand
// shares the identical cwd/registry-resolution preamble.
func TestRunCLI_Definition_NoLanguageError(t *testing.T) {
	// Chdir into a fresh, non-git temp dir so hubgeometry.Resolve degrades to
	// codeintelengine.BuiltinRegistry() deterministically, independent of
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

// TestRunCLI_Symbol_NoLanguageError verifies that "symbol <query>
// --target-dir <empty dir>" fails through the same ErrNoLanguage path
// TestRunCLI_Refs_NoLanguageError exercises for "refs" — symbolCommand
// shares the identical cwd/registry-resolution preamble.
func TestRunCLI_Symbol_NoLanguageError(t *testing.T) {
	// Chdir into a fresh, non-git temp dir so hubgeometry.Resolve degrades to
	// codeintelengine.BuiltinRegistry() deterministically, independent of
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

// TestRunCLI_Symbol_TreatsFileLineColArgumentAsLiteralSearchString proves
// that symbolCommand never calls parsePosition: an argument shaped like
// "file:line:col" must still be passed through to Query.Symbol unparsed,
// not silently swallowed as a position.
//
// DetectLanguage never consults Options.Query at all (see
// internal/codeintelengine/detect.go), so RunCLI("symbol", "foo.go:1:1",
// --target-dir <empty>)'s ErrNoLanguage envelope is byte-for-byte identical
// regardless of whether the argument was kept as a literal string or
// mis-parsed as a position — confirmed empirically: "refs foo.go:1:1" and
// "symbol foo.go:1:1" against the same empty target dir produce the exact
// same error text. The ErrNoLanguage envelope therefore cannot itself prove
// which shape Query took, so this test pins the real contract two ways
// instead: (1) it exercises symbolQuery directly, the one seam
// symbolCommand's RunE actually uses to build Query.Symbol, asserting it
// keeps "foo.go:1:1" as a literal Query.Symbol with Query.Pos left nil, even
// though the same string driven through parseQuery (refs/definition's
// converter, which symbolCommand deliberately does not call) does parse as a
// position — proving the two functions diverge exactly where they must; and
// (2) it still drives the full RunCLI("symbol", ...) path to confirm the
// command reaches DetectLanguage and fails through the expected
// ErrNoLanguage envelope, exactly like TestRunCLI_Symbol_NoLanguageError.
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
	parsed, err := parseQuery(arg)
	if err != nil {
		t.Fatalf("parseQuery(%q) error = %v; want nil", arg, err)
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

// TestEmitLookupResult_AmbiguousSymbolExitsTwo tests emitLookupResult
// directly (this file is package codeintelcli, the same package
// emitLookupResult is defined in) rather than through the full RunCLI tree —
// reaching *codeintelengine.ErrAmbiguousSymbol through a live refs/definition
// call would require a real language server, out of scope for this file per
// its own header comment. It covers both the ambiguous exit-2 path and the
// not-found path, in the same table, to prove the not-found case still falls
// through to plain output.Err rather than being swept into the ambiguous
// branch.
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
			err: &codeintelengine.ErrAmbiguousSymbol{
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
			err:          &codeintelengine.ErrSymbolNotFound{Symbol: "Bar", TargetDir: "/tmp"},
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

// TestRunCLI_Refs_RequiresAtLeastOneArg verifies that Args:
// cobra.MinimumNArgs(1) still rejects a bare "refs" call (0 args) through the
// JSON error envelope, without touching detection or the registry at all. A
// 2-arg call is no longer an arg-count violation as of batch-mode-cli — see
// TestRunCLI_Refs_TwoArgsIsBatchMode for that case.
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

// TestRunCLI_Refs_TwoArgsIsBatchMode proves the opposite point from
// TestRunCLI_Refs_RequiresAtLeastOneArg: "refs one two" is valid batch-mode
// syntax now, not an arg-count rejection. It runs against an empty temp dir
// so DetectLanguage fails identically for both symbols, keeping the
// assertion deterministic and gopls-independent — an ErrNoLanguage failure
// is not "confirmed absent," so both entries classify as "error", not
// "not_found", pinning that distinction as a useful regression check.
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

// TestBatchRunner_WorstOutcomeWinsExitCode tests runBatch directly rather
// than through a live language server: a small table-driven lookupOne
// closure maps each input symbol string to a fixed (batchStatus,
// map[string]any) pair, so this test needs no engine call or subprocess at
// all. It table-drives one sub-test per possible "worst status present"
// combination, asserting both the resulting exit code (via
// clihelp.NewExitContext's exitState.Code(), mirroring
// TestEmitLookupResult_AmbiguousSymbolExitsTwo's pattern above) and that
// "results" has one entry per input symbol with the expected "status".
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

// TestClassifySymbolError_MultipleMatchesIsFoundNotAmbiguous pins the
// regression classifySymbolError exists to prevent: a future edit that
// makes classifySymbolError reuse classifyLookupError's ambiguity branch by
// mistake. Two matches is exactly the multi-candidate case that *would* be
// "ambiguous" for refs/definition (via classifyLookupError), but per
// symbol-semantics symbol has no ambiguous status at all — every match is
// just part of the found result set.
func TestClassifySymbolError_MultipleMatchesIsFoundNotAmbiguous(t *testing.T) {
	t.Parallel()

	status, fields := classifySymbolError(nil, []codeintelengine.SymbolMatch{{Name: "Foo"}, {Name: "FooBar"}})

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
