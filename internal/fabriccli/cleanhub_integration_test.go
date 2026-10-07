//go:build integration

// cleanhub_integration_test.go drives the fabric CLI's read-only, refusal and record-keeping verbs against one real hub built by hubforge, as ordered scenarios:
// the clone-time config commit, pairs, status, prune, the shortname record and the refusals that need a resolved cwd but touch no pair state.
// The anchored scenario covers what only a non-"." anchor can show.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// perWorktreeModuleNames returns configreg.Modules() filtered to every entry that is not hub-wide
// -- the set CloneAndWire's per-worktree reconcile loop materializes, derived fresh every call so
// a newly registered module never silently makes these tests wrong.
func perWorktreeModuleNames() []string {
	var names []string
	for _, m := range configreg.Modules() {
		if m.HubWide {
			continue
		}
		names = append(names, m.Name)
	}
	return names
}

// hubRelativeWeftTarget returns the hub-relative, slash-separated path fabricengine.Mutations.Append
// would have recorded for h's weft prime worktree, matching the conversion CommitWeftPaths's own
// recording site performs.
func hubRelativeWeftTarget(t *testing.T, h *hubforge.Hub) string {
	t.Helper()

	rel, err := filepath.Rel(h.Path, h.PrimeRecords())
	if err != nil {
		t.Fatalf("filepath.Rel(%s, %s): %v", h.Path, h.PrimeRecords(), err)
	}
	return filepath.ToSlash(rel)
}

// runShortnameVerb runs `fabric shortname <args>` from the hub's prime worktree and decodes the envelope.
func runShortnameVerb(t *testing.T, h *hubforge.Hub, args ...string) (exit int, env map[string]any) {
	t.Helper()
	var out bytes.Buffer
	exit = fabriccli.RunCLIIn(h.PrimeWorktree(), &out, append([]string{"shortname"}, args...))
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode shortname envelope: %v\noutput: %s", err, out.String())
	}
	return exit, env
}

// removeShortnameRecord deletes .lyx-shortname from the board and commits the removal, leaving a hub with no shortname.
func removeShortnameRecord(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	board := h.BoardDir()
	if err := os.Remove(filepath.Join(board, fabricengine.ShortnameFileName)); err != nil {
		t.Fatalf("remove %s: %v", fabricengine.ShortnameFileName, err)
	}
	if _, _, err := fabricengine.NewBolt(board).Commit("test: remove the shortname record", fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("commit the removal: %v", err)
	}
}

// boardGit runs git in the board worktree and returns trimmed stdout.
func boardGit(t *testing.T, h *hubforge.Hub, args ...string) string {
	t.Helper()
	out, err := gitexec.Run(args, h.BoardDir())
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(out)
}

// TestRunCLI_CleanHubScenario runs the read-only, refusal and shortname checks over one hub.
// Steps run serially in this order: the shortname steps that rewrite the record come after the ones that read it, and the pollution step comes last because it replaces the warp _lyx junction with a tracked directory.
// The scenario calls t.Parallel as a whole; no step does, because they share the one hub.
func TestRunCLI_CleanHubScenario(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: \"\"\n")

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"CloneConfigCommit_WeftPrimeCleanAfterClone", func(t *testing.T) {
			// A freshly-built hub's weft prime worktree is clean, and git ls-files reports every per-worktree module's config file.
			if status := gitkit.GitStatusPorcelain(t, h.PrimeRecords()); status != "" {
				t.Errorf("weft prime status --porcelain = %q; want empty (clean)", status)
			}

			tracked := gitkit.LsFiles(t, h.PrimeRecords())
			for _, name := range perWorktreeModuleNames() {
				want := configengine.ConfigFileRel(name)
				if !slices.Contains(tracked, want) {
					t.Errorf("git ls-files in weft prime does not contain %q; got %v", want, tracked)
				}
			}
		}},
		{"CloneConfigCommit_OneCommitNotOnePerModule", func(t *testing.T) {
			// The weft primary branch carries the clone's config commit subject exactly once, and a second ReconcileAll over the same weft base reports Applied false for every module.
			out := gitOutputCLI(t, h.PrimeRecords(), "log", "--oneline")

			const wantSubject = "fabric clone: record module configs"
			count := strings.Count(out, wantSubject)
			if count != 1 {
				t.Errorf("weft primary git log --oneline contains %q %d times; want exactly 1\nlog:\n%s", wantSubject, count, out)
			}

			results, err := configsync.ReconcileAll(h.WeftBase, h.BoardDir(), true)
			if err != nil {
				t.Fatalf("second ReconcileAll: %v", err)
			}
			for _, r := range results {
				if r.Applied {
					t.Errorf("second ReconcileAll: module %s reported Applied true; want false (nothing left to reconcile)", r.Module)
				}
			}
		}},
		{"CloneConfigCommit_MutationRecordShape", func(t *testing.T) {
			// The mutation record CloneAndWire produced holds one KindFileWritten entry per per-worktree module followed by a KindCommitCreated entry targeting the weft worktree, commit last -- array order is part of the vocabulary.
			entries := h.Mutations.Entries()
			moduleNames := perWorktreeModuleNames()

			wantLen := len(moduleNames) + 1
			if len(entries) < wantLen {
				t.Fatalf("mutation record has %d entries; want at least %d (one KindFileWritten per module plus a trailing KindCommitCreated)\nentries: %+v", len(entries), wantLen, entries)
			}
			tail := entries[len(entries)-wantLen:]

			for i, name := range moduleNames {
				e := tail[i]
				if e.Kind != fabricengine.KindFileWritten {
					t.Errorf("tail entry %d Kind = %q; want %q (module %s)", i, e.Kind, fabricengine.KindFileWritten, name)
				}
				wantSuffix := "/" + configengine.ConfigFileRel(name)
				if !strings.HasSuffix(e.Target, wantSuffix) {
					t.Errorf("tail entry %d Target = %q; want a suffix of %q (module %s)", i, e.Target, wantSuffix, name)
				}
			}

			last := tail[len(tail)-1]
			if last.Kind != fabricengine.KindCommitCreated {
				t.Errorf("last mutation record entry Kind = %q; want %q", last.Kind, fabricengine.KindCommitCreated)
			}
			if wantTarget := hubRelativeWeftTarget(t, h); last.Target != wantTarget {
				t.Errorf("last entry Target = %q; want %q (the weft worktree)", last.Target, wantTarget)
			}
		}},
		{"PairsReturnsPairsKey", func(t *testing.T) {
			code, output := runFabric(t, h.PrimeWorktree(), "pairs")
			if code != 0 {
				t.Errorf("RunCLI(pairs) = %d; want 0\noutput: %s", code, output)
			}

			result := envelope.Decode(t, output)
			if !result.OK {
				t.Errorf("RunCLI(pairs) ok = %v; want true", result.OK)
			}
			if _, hasPairs := result.Raw["pairs"]; !hasPairs {
				t.Errorf("RunCLI(pairs) output missing 'pairs' key; got %v", result)
			}
		}},
		{"ReadOnlyVerbsOmitMutationsKey", func(t *testing.T) {
			// list, pairs, status and diff never carry a "mutations" key: nothing was mutated, so the which-verbs scope decision is machine-held rather than a convention.
			warpSHA := strings.TrimSpace(gitOutputCLI(t, h.PrimeWorktree(), "rev-parse", "HEAD"))

			tests := []struct {
				name string
				args []string
			}{
				{name: "List", args: []string{"list"}},
				{name: "Pairs", args: []string{"pairs"}},
				{name: "Status", args: []string{"status"}},
				{name: "Diff", args: []string{"diff", warpSHA}},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					code, output := runFabric(t, h.PrimeWorktree(), tt.args...)
					if code != 0 {
						t.Fatalf("RunCLI(%v) = %d; want 0\noutput: %s", tt.args, code, output)
					}
					result := envelope.Decode(t, output)
					if _, present := result.Raw["mutations"]; present {
						t.Errorf("RunCLI(%v) output has a 'mutations' key; want it absent from a read-only verb's envelope: %v", tt.args, result)
					}
					if _, rawPartial := result.Raw["partial"]; rawPartial || result.Partial != nil {
						t.Errorf("RunCLI(%v) output has a 'partial' key; want it absent from a read-only verb's envelope: %v", tt.args, result)
					}
				})
			}
		}},
		{"StatusReportsNoMergeInProgressOnACleanPair", func(t *testing.T) {
			// status on a pair with no parked merge reports "merge_in_progress" present and false, alongside the pre-existing "changes" key.
			code, output := runFabric(t, h.PrimeWorktree(), "status")
			if code != 0 {
				t.Fatalf("RunCLI(status) = %d; want 0\noutput: %s", code, output)
			}

			result := envelope.Decode(t, output)
			inProgress, present := result.Raw["merge_in_progress"]
			if !present {
				t.Fatalf("RunCLI(status) output missing 'merge_in_progress' key; got %v", result)
			}
			if inProgress != false {
				t.Errorf("RunCLI(status) merge_in_progress = %v; want false", inProgress)
			}
			if _, present := result.Raw["changes"]; !present {
				t.Errorf("RunCLI(status) output missing 'changes' key; got %v", result)
			}
		}},
		{"FallbackCwdMatchesInjectedCwd", func(t *testing.T) {
			// RunCLI's process-cwd fallback and RunCLIIn's explicit-cwd branch agree on one read-only verb: "pairs" through RunCLIIn in this process, then through RunCLI in a separate OS process whose working directory is the prime worktree via exec.Command's Dir field, never a Chdir on this test binary.
			// The subprocess is this test binary re-exec'd and intercepted by cli_test.go's package-level init, gated by lyxFabricCLISubprocessCwdEnv.
			injectedCode, injectedOut := runFabric(t, h.PrimeWorktree(), "pairs")
			if injectedCode != 0 {
				t.Fatalf("RunCLIIn(pairs) = %d; want 0\noutput: %s", injectedCode, injectedOut)
			}

			cmd := exec.Command(os.Args[0])
			cmd.Dir = h.PrimeWorktree()
			cmd.Env = append(os.Environ(), lyxFabricCLISubprocessCwdEnv+"=1")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			fallbackOut, runErr := cmd.Output()

			fallbackCode := 0
			if runErr != nil {
				var exitErr *exec.ExitError
				if !errors.As(runErr, &exitErr) {
					t.Fatalf("subprocess RunCLI(pairs) failed to run: %v\nstderr: %s", runErr, stderr.String())
				}
				fallbackCode = exitErr.ExitCode()
			}

			if fallbackCode != injectedCode {
				t.Errorf("RunCLI (process-cwd fallback) exit code = %d; want %d (RunCLIIn's)\nstderr: %s", fallbackCode, injectedCode, stderr.String())
			}
			if string(fallbackOut) != injectedOut {
				t.Errorf("RunCLI (process-cwd fallback) output = %q; want it identical to RunCLIIn(h.PrimeWorktree(), …) output %q", fallbackOut, injectedOut)
			}
		}},
		{"PruneEmitsAnEmptyArrayNotNull", func(t *testing.T) {
			// prune's "entries" key is an empty array, never null, on a hub with nothing stale.
			code, output := runFabric(t, h.Location.WorktreePath(), "prune")
			if code != 0 {
				t.Fatalf("RunCLI(prune) = %d; want 0\noutput: %s", code, output)
			}

			env := envelope.Decode(t, output)
			raw, present := env.Raw["entries"]
			if !present {
				t.Fatalf("prune envelope has no \"entries\" key\noutput: %s", output)
			}
			if raw == nil {
				t.Fatalf("prune envelope has \"entries\":null; want an empty array\noutput: %s", output)
			}
			entries, isArray := raw.([]any)
			if !isArray {
				t.Fatalf("prune \"entries\" is %T; want a JSON array\noutput: %s", raw, output)
			}
			if len(entries) != 0 {
				t.Errorf("prune \"entries\" has %d element(s) on a hub with nothing stale; want 0", len(entries))
			}
		}},
		{"MergeContinueAndAbortTogetherFail", func(t *testing.T) {
			// "merge --continue --abort" fails with a usage-shaped error envelope.
			// Driven against a real hub so PersistentPreRunE's cwd resolution succeeds first and the mutual-exclusion refusal is the only error the run produces.
			code, output := runFabric(t, h.PrimeWorktree(), "merge", "--continue", "--abort")
			if code != 1 {
				t.Fatalf("RunCLI(merge --continue --abort) = %d; want 1\noutput: %s", code, output)
			}

			result := envelope.Decode(t, output)
			if result.OK {
				t.Errorf("RunCLI(merge --continue --abort) ok = true; want false")
			}
			if result.Error == "" {
				t.Errorf("RunCLI(merge --continue --abort) error is empty; want a usage-shaped message naming the mutual exclusion")
			}
		}},
		{"MergeRejectsFlagsItWouldOtherwiseIgnore", func(t *testing.T) {
			// "merge --abort -m <msg>" used to be accepted with the message silently discarded, so the pre-flight rejects it the same way it rejects --squash alongside --abort.
			// Driven against a real pair so the check is proven to sit ahead of the engine call.
			tests := []struct {
				name    string
				args    []string
				wantErr string
			}{
				{
					name:    "MessageWithAbort",
					args:    []string{"merge", "--abort", "-m", "ignored message"},
					wantErr: "usage: -m cannot be combined with --abort",
				},
				{
					name:    "SquashWithAbort",
					args:    []string{"merge", "--abort", "--squash"},
					wantErr: "usage: --squash cannot be combined with --continue or --abort",
				},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					code, output := runFabric(t, h.PrimeWorktree(), tt.args...)
					if code != 1 {
						t.Fatalf("RunCLI(%v) = %d; want 1\noutput: %s", tt.args, code, output)
					}
					result := envelope.RequireErr(t, output, "")
					if result.Error != tt.wantErr {
						t.Errorf("RunCLI(%v) error = %q; want %q", tt.args, result.Error, tt.wantErr)
					}
				})
			}
		}},
		{"MergeNonexistentBranchReportsSourceNotFound", func(t *testing.T) {
			// The aggregated MergeGuardError names the source-not-found reason and never claims the weft counterpart is not fabric-managed: the weft side is not a merge participant.
			code, output := runFabric(t, h.PrimeWorktree(), "merge", "nonexistent-branch")
			if code != 1 {
				t.Fatalf("RunCLI(merge nonexistent-branch) = %d; want 1\noutput: %s", code, output)
			}

			result := envelope.RequireErr(t, output, "source branch not found")
			if strings.Contains(result.Error, "source branch is not fabric-managed") {
				t.Errorf("RunCLI(merge nonexistent-branch) error = %q; must not claim the weft counterpart is not fabric-managed (unreachable now the weft side is not a merge participant)", result.Error)
			}
		}},
		{"MergeStageRequiresAtLeastOnePath", func(t *testing.T) {
			// With no paths merge-stage refuses rather than succeed vacuously, since a caller that passed nothing meant to pass something.
			if code, output := runFabric(t, h.PrimeWorktree(), "merge-stage"); code == 0 {
				t.Errorf("RunCLI(merge-stage) with no paths = 0; want a refusal\noutput: %s", output)
			}
		}},
		{"RemoveNothingLeftIsNotFound", func(t *testing.T) {
			code, output := runFabric(t, h.PrimeWorktree(), "remove", "no-such-pair")
			if code == 0 {
				t.Fatalf("remove of a slug with nothing left exited 0; output: %s", output)
			}
			envelope.RequireErr(t, output, "not found")
		}},
		{"CloneConfigCommit_PairInheritsConfigs", func(t *testing.T) {
			// A pair created off a freshly-built hub has its anchored loom.yaml config on disk, the direct end-to-end proof that the clone's config commit reaches a forked pair.
			hubforge.AddPair(t, h, "pair-inherits-configs")

			anchoredWeftBase := filepath.Join(h.PairRecordsSibling("pair-inherits-configs"), h.Anchor)
			loomConfigPath := configengine.ConfigFile(anchoredWeftBase, "loom")
			if _, err := os.Stat(loomConfigPath); err != nil {
				t.Errorf("pair weft sibling loom config %s: %v; want it present on disk (inherited from the clone commit)", loomConfigPath, err)
			}
		}},
		{"ShortnameVerb_PrintsRecordedShortname", func(t *testing.T) {
			exit, env := runShortnameVerb(t, h)
			if exit != 0 || env["ok"] != true {
				t.Fatalf("shortname = %d, %v; want exit 0 ok", exit, env)
			}
			if env["shortname"] != hubforge.TestShortname {
				t.Errorf("shortname = %v; want %q", env["shortname"], hubforge.TestShortname)
			}
		}},
		{"ShortnameVerb_SameShortnameIsANoOp", func(t *testing.T) {
			before := boardGit(t, h, "rev-parse", "HEAD")

			exit, env := runShortnameVerb(t, h, hubforge.TestShortname)
			if exit != 0 || env["shortname"] != hubforge.TestShortname {
				t.Fatalf("same shortname = %d, %v; want exit 0", exit, env)
			}
			if after := boardGit(t, h, "rev-parse", "HEAD"); after != before {
				t.Errorf("weft:main moved from %s to %s; want no new commit", before, after)
			}
		}},
		{"ShortnameVerb_DifferentShortnameRefusesNamingTheRecordedOne", func(t *testing.T) {
			exit, env := runShortnameVerb(t, h, "zz")
			if exit == 0 || env["ok"] != false {
				t.Fatalf("different shortname = %d, %v; want a refusal", exit, env)
			}
			if !strings.Contains(env["error"].(string), `"`+hubforge.TestShortname+`"`) {
				t.Errorf("error = %q; want it to name the recorded shortname %q", env["error"], hubforge.TestShortname)
			}
		}},
		{"ShortnameVerb_RecordsOnAHubWithNoShortname", func(t *testing.T) {
			// Relies on the previous steps leaving TestShortname recorded: the record is removed here and the verb then writes "zz".
			removeShortnameRecord(t, h)

			exit, env := runShortnameVerb(t, h)
			if exit == 0 || !strings.Contains(env["error"].(string), "lyx fabric shortname <shortname>") {
				t.Fatalf("no-arg on a hub with no shortname = %d, %v; want a refusal naming `lyx fabric shortname <shortname>`", exit, env)
			}

			exit, env = runShortnameVerb(t, h, "zz")
			if exit != 0 || env["shortname"] != "zz" || env["partial"] != false {
				t.Fatalf("record = %d, %v; want exit 0 with shortname zz", exit, env)
			}
			if got := boardGit(t, h, "show", "HEAD:"+fabricengine.ShortnameFileName); got != "zz" {
				t.Errorf("%s at weft:main's tip = %q; want zz", fabricengine.ShortnameFileName, got)
			}
		}},
		{"ShortnameVerb_CommitsTheRecordAlone", func(t *testing.T) {
			// A pending board write is not this verb's to commit: the record lands alone and the board change stays uncommitted.
			// Relies on the previous step having recorded "zz".
			removeShortnameRecord(t, h)
			pending := filepath.Join(h.BoardDir(), "pending-board-write.md")
			if err := os.WriteFile(pending, []byte("half-written\n"), 0o644); err != nil {
				t.Fatalf("write pending board file: %v", err)
			}

			if exit, env := runShortnameVerb(t, h, "zz"); exit != 0 {
				t.Fatalf("record = %d, %v; want exit 0", exit, env)
			}
			if got := boardGit(t, h, "show", "--name-only", "--format=", "HEAD"); got != fabricengine.ShortnameFileName {
				t.Errorf("files in the shortname commit = %q; want only %s", got, fabricengine.ShortnameFileName)
			}
			if got := boardGit(t, h, "status", "--porcelain", "--", "pending-board-write.md"); got == "" {
				t.Errorf("the pending board file was committed; want it left uncommitted")
			}
			if err := os.Remove(pending); err != nil {
				t.Fatalf("remove pending board file: %v", err)
			}
		}},
		{"PairsReportsPollutionEntryWithRemedy", func(t *testing.T) {
			// With a file tracked directly under _lyx in the warp index, "fabric pairs" still emits a pollution entry naming that path with a non-empty "remedy" key, pinning the pollution JSON shape at the CLI boundary.
			// Last step: the warp _lyx junction becomes a tracked directory.
			hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _lyx\n")

			// A real hub already wires _lyx as a junction onto the weft config.
			// Pollution means an operator replaced that junction with a real, tracked directory, so the junction must be removed first -- writing through it would land the file on the weft side instead of polluting the warp index.
			// The explicit "-f" on the git add is likewise needed:
			// WireJunctionsWith seeded _lyx into the warp's .git/info/exclude, and git refuses to add an explicitly-named ignored path without it.
			warpLyxDir := filepath.Join(h.PrimeWorktree(), lyxdirs.LyxDirName)
			if err := fslink.Remove(warpLyxDir); err != nil {
				t.Fatalf("remove warp _lyx junction: %v", err)
			}
			if err := os.MkdirAll(warpLyxDir, 0o755); err != nil {
				t.Fatalf("mkdir warp _lyx dir: %v", err)
			}
			trackedFile := filepath.Join(warpLyxDir, "PATTERN.md")
			if err := os.WriteFile(trackedFile, []byte("# constraints\n"), 0o644); err != nil {
				t.Fatalf("write tracked file: %v", err)
			}
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "add", "-f", "--", lyxdirs.LyxDirName)
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "-m", "accidentally track _lyx")

			code, output := runFabric(t, h.PrimeWorktree(), "pairs")
			if code != 0 {
				t.Errorf("RunCLI(pairs) = %d; want 0\noutput: %s", code, output)
			}

			result := envelope.Decode(t, output)
			pairs, ok := result.Raw["pairs"].([]any)
			if !ok || len(pairs) == 0 {
				t.Fatalf("RunCLI(pairs) 'pairs' = %v; want a non-empty array", result.Raw["pairs"])
			}

			var found map[string]any
			for _, p := range pairs {
				pair, ok := p.(map[string]any)
				if !ok {
					continue
				}
				pollution, ok := pair["pollution"].([]any)
				if !ok {
					continue
				}
				for _, e := range pollution {
					entry, ok := e.(map[string]any)
					if !ok {
						continue
					}
					if path, _ := entry["path"].(string); strings.Contains(path, "PATTERN.md") {
						found = entry
					}
				}
			}
			if found == nil {
				t.Fatalf("no pollution entry naming PATTERN.md found in %+v", result)
			}
			remedy, _ := found["remedy"].(string)
			if remedy == "" {
				t.Errorf("pollution entry %+v has empty/missing 'remedy'; want a non-empty git rm --cached remedy", found)
			}
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}

// TestRunCLI_AnchoredHubScenario runs the checks only a subpath-anchored hub can show, over one hub anchored at "backend".
// Steps run serially in this order; the scenario calls t.Parallel as a whole and no step does, because they share the one hub.
func TestRunCLI_AnchoredHubScenario(t *testing.T) {
	t.Parallel()

	// "backend" is a subpath anchor, so the weft ROOT (h.PrimeRecords()) is not the anchored directory
	// (h.WeftBase) -- fabriccli.CloneAndWire records that anchor for real, so no hand-written anchor
	// marker is needed here.
	h := hubforge.NewHub(t, "backend")

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"CloneConfigCommit_AnchorScoped", func(t *testing.T) {
			// The clean-and-tracked assertion at a non-"." anchor, with committed paths prefixed by the anchor, proving the clone's config commit was anchor-scoped rather than run at the weft base, which at a non-"." anchor is a subdirectory of the worktree root.
			if status := gitkit.GitStatusPorcelain(t, h.PrimeRecords()); status != "" {
				t.Errorf("weft prime status --porcelain = %q; want empty (clean)", status)
			}

			tracked := gitkit.LsFiles(t, h.PrimeRecords())
			for _, name := range perWorktreeModuleNames() {
				want := "backend/" + configengine.ConfigFileRel(name)
				if !slices.Contains(tracked, want) {
					t.Errorf("git ls-files in weft prime does not contain %q; got %v", want, tracked)
				}
			}
		}},
		{"WeftSiblingNonAnchoredCwd_GetsWeftRefusal", func(t *testing.T) {
			// The refusal an operator sees from a weft sibling's NON-anchored directory on a subpath-anchored hub is the specific weft-sibling message, never the generic cwd-gate error, which would direct the operator deeper INTO the weft.
			code, output := runFabric(t, h.PrimeRecords(), "pairs")
			if code == 0 {
				t.Fatalf("RunCLI(pairs) from weft sibling = 0; want a refusal\noutput: %s", output)
			}
			envelope.RequireErr(t, output, "weft sibling of a pair")
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}
