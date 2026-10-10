//go:build integration

// hubmutation_integration_test.go drives the fabric CLI's mutating verbs — the two-sided bypass push, "push" from a task pair and from the prime, "add", "remove" and "reconcile" — against one real hub as an ordered scenario, asserting each verb's envelope and exit-code contract:
// a pre-flight failure is a bare error carrying neither "mutations" nor "partial", reconcile heals missing hub-wide config and treats a failed config push as non-fatal, a pair that vanished mid-walk is not a failure, and a pair reconcile could not repair is.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestRunCLI_HubMutationScenario runs the mutating-verb checks over one hub.
// Steps run serially in this order, each one leaving state the next tolerates:
// the steps that break the board remote or plant an unrepairable pair come last, because every later reconcile would otherwise see that damage.
// The scenario calls t.Parallel as a whole; no step does, because they share the one hub.
func TestRunCLI_HubMutationScenario(t *testing.T) {
	t.Parallel()

	// A copy's relocation rewrites the tracked .lyx-warp binding, which the clean-tree assertions would find dirty.
	h := hubforge.NewHub(t, ".")

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"BypassPushAdvancesBothUpstreams", func(t *testing.T) {
			// With unpushed commits on both sides, --warp-path/--weft-path bypass push exits 0 and both bare upstreams' HEAD matches their local checkout's HEAD.
			// Add one more commit on top of the weft side's already-pushed history, so the weft side has something genuinely unpushed to push.
			placeholderFile := filepath.Join(h.PrimeRecords(), lyxdirs.LyxDirName, "placeholder")
			if err := os.WriteFile(placeholderFile, []byte("bypass push test"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			gitkit.MustRun(t, h.PrimeRecords(), "git", "add", ".")
			gitkit.MustRun(t, h.PrimeRecords(), "git", "commit", "-q", "-m", "weft bypass push")

			// The prime pair's weft branch, so the bare-side assertion below checks that branch specifically rather than the bare's own HEAD symref -- a real hub's weft bare also carries weft:main's own "main" branch (the board checkout), and the bare's default HEAD may not name the prime pair's branch.
			weftBranch := strings.TrimSpace(gitOutputCLI(t, h.PrimeRecords(), "rev-parse", "--abbrev-ref", "HEAD"))
			wantWeftSHA := gitkit.RevParse(t, h.PrimeRecords(), "HEAD")
			wantWarpSHA := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD")

			// A real forked push child cannot itself be observed from a test binary (it would re-exec the test binary), so the synchronous bypass handler it runs is the deterministic proof that a supplied path is pushed.
			var out bytes.Buffer
			exitCode := fabriccli.RunCLI(&out, []string{
				"--warp-path", h.PrimeWorktree(),
				"--weft-path", h.PrimeRecords(),
				"push",
			})
			if exitCode != 0 {
				t.Fatalf("RunCLI bypass push = %d; want 0\noutput: %s", exitCode, out.String())
			}

			envelope.RequireOK(t, out.String())

			if gotWeftSHA := gitkit.RevParse(t, h.RecordsBare, "refs/heads/"+weftBranch); gotWeftSHA != wantWeftSHA {
				t.Errorf("weft bare %s = %s; want %s (the unpushed commit was not pushed)", weftBranch, gotWeftSHA, wantWeftSHA)
			}
			if gotWarpSHA := gitkit.RevParse(t, h.CodeBare, "HEAD"); gotWarpSHA != wantWarpSHA {
				t.Errorf("warp bare HEAD = %s; want %s (the unpushed commit was not pushed)", gotWarpSHA, wantWarpSHA)
			}
		}},
		{"PushPushesBothSidesOfATaskPair", func(t *testing.T) {
			// With unpushed commits on both sides of a task pair, "lyx fabric push" from the pair's code worktree exits 0, both bare upstreams sit at their local checkout's HEAD, and nothing is reported as skipped.
			// The pair is removed again so the later steps see the hub they expect.
			const slug = "push-both-sides-pair"
			if code, output := runFabric(t, h.PrimeWorktree(), "add", slug); code != 0 {
				t.Fatalf("RunCLI(add) = %d; want 0\noutput: %s", code, output)
			}
			pairWarp, pairWeft := h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)
			gitkit.CommitFile(t, pairWarp, "warp-push.txt", "warp\n", "warp push")
			gitkit.CommitFile(t, pairWeft, filepath.Join(lyxdirs.LyxDirName, "weft-push.txt"), "weft\n", "weft push")

			weftBranch := gitkit.CurrentBranch(t, pairWeft)
			wantWeftSHA := gitkit.RevParse(t, pairWeft, "HEAD")
			wantWarpSHA := gitkit.RevParse(t, pairWarp, "HEAD")

			code, output := runFabric(t, pairWarp, "push")
			if code != 0 {
				t.Fatalf("RunCLI(push) = %d; want 0\noutput: %s", code, output)
			}
			env := envelope.RequireOK(t, output)
			if _, skipped := env.Raw["code_push_skipped"]; skipped {
				t.Errorf("push envelope carries code_push_skipped in a task pair: %s", output)
			}

			if got := gitkit.RevParse(t, h.RecordsBare, "refs/heads/"+weftBranch); got != wantWeftSHA {
				t.Errorf("weft bare %s = %s; want %s (the weft side was not pushed)", weftBranch, got, wantWeftSHA)
			}
			if got := gitkit.RevParse(t, h.CodeBare, "refs/heads/"+slug); got != wantWarpSHA {
				t.Errorf("warp bare %s = %s; want %s (the warp side was not pushed)", slug, got, wantWarpSHA)
			}

			if code, output := runFabric(t, h.PrimeWorktree(), "remove", slug); code != 0 {
				t.Fatalf("RunCLI(remove) = %d; want 0\noutput: %s", code, output)
			}
		}},
		{"AddLeftoverWarpIsBarePreflightError", func(t *testing.T) {
			// A removed pair's landed warp branch still on origin, beside an archived weft branch and no local weft branch, blocks the re-add with a pre-flight failure, so a bare error carrying neither `mutations` nor `partial`.
			const slug = "leftover-slug"

			if code, output := runFabric(t, h.PrimeWorktree(), "add", slug); code != 0 {
				t.Fatalf("first add exit = %d; output: %s", code, output)
			}
			warp := h.PairCodeWorktree(slug)
			gitkit.CommitFile(t, warp, "work.txt", "work\n", "warp work")
			gitkit.MustRun(t, warp, "git", "push", "--quiet", "origin", slug)
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--squash", slug)
			gitkit.Git(t, h.PrimeWorktree(), "commit", "-m", "land "+slug)
			if code, output := runFabric(t, h.PrimeWorktree(), "remove", slug); code != 0 {
				t.Fatalf("remove exit = %d; output: %s", code, output)
			}

			code, output := runFabric(t, h.PrimeWorktree(), "add", slug)
			if code == 0 {
				t.Fatalf("second add exit = 0; want non-zero; output: %s", output)
			}
			env := envelope.RequireErr(t, output, slug)
			if _, present := env.Raw["mutations"]; present {
				t.Errorf("envelope carries \"mutations\"; a pre-flight failure must not")
			}
			if env.Partial != nil {
				t.Errorf("envelope carries \"partial\"; a pre-flight failure must not")
			}
		}},
		{"Reconcile_HealsMissingRepoWideConfig", func(t *testing.T) {
			// reconcile self-heals the hub-wide fabric.yaml and board.yaml: on a hub that has none, it used to fail with "not initialized here; run \"lyx fabric reconcile\"" — prescribing the command that just failed — and must instead materialize the configs, commit them in _board and proceed. board.yaml is seeded from the prime's copy when it has one, else from the template with a warning.
			// Rows run in this order on the one hub: each row deletes the configs again, which are committed by the previous row's heal.
			const primeBoard = "types:\n  spike: A time-boxed investigation\n"

			tests := []struct {
				name         string
				primeHasCopy bool
				wantSeed     string
				wantWarning  bool
			}{
				{name: "prime copy seeds board.yaml", primeHasCopy: true, wantSeed: "prime"},
				{name: "no prime copy falls back to the template", wantSeed: "template", wantWarning: true},
			}
			for _, tt := range tests {
				if !t.Run(tt.name, func(t *testing.T) {
					// hubforge.NewHub always materializes the hub-wide configs as part of building a real hub, so the "missing config" state this test exists to heal must be produced by hand here -- an operator deleting the files is exactly the scenario the healing logic guards against.
					fabricPath := configengine.ConfigFile(h.BoardDir(), "fabric")
					boardPath := configengine.ConfigFile(h.BoardDir(), "board")
					for _, path := range []string{fabricPath, boardPath} {
						if err := os.Remove(path); err != nil {
							t.Fatalf("remove hub-wide config %s: %v", path, err)
						}
					}
					if tt.primeHasCopy {
						hubforge.SeedConfig(t, h, map[string]string{"board": primeBoard})
					} else if err := os.Remove(configengine.ConfigFile(h.WeftBase, "board")); err != nil && !os.IsNotExist(err) {
						t.Fatalf("remove prime board config: %v", err)
					}

					code, output := runFabric(t, h.PrimeWorktree(), "reconcile")
					if code != 0 {
						t.Fatalf("RunCLI(reconcile) = %d; want 0 (reconcile must heal the missing config, not report it)\noutput: %s", code, output)
					}

					if _, err := os.Stat(fabricPath); err != nil {
						t.Errorf("hub-wide fabric config not materialized at %s: %v", fabricPath, err)
					}
					boardYAML, err := os.ReadFile(boardPath)
					if err != nil {
						t.Fatalf("hub-wide board config not materialized at %s: %v", boardPath, err)
					}
					if got := strings.Contains(string(boardYAML), "spike:"); got != tt.primeHasCopy {
						t.Errorf("board.yaml carries the prime's custom type = %v; want %v\n%s", got, tt.primeHasCopy, boardYAML)
					}

					result := envelope.Decode(t, output)
					hubConfig, _ := result.Raw["hub_config"].([]any)
					boardSeed := ""
					for _, entry := range hubConfig {
						fields, _ := entry.(map[string]any)
						if fields["module"] == "board" {
							boardSeed, _ = fields["seed"].(string)
						}
					}
					if boardSeed != tt.wantSeed {
						t.Errorf("hub_config board seed = %q; want %q\nhub_config: %v", boardSeed, tt.wantSeed, hubConfig)
					}
					warnings, _ := result.Raw["warnings"].([]any)
					if tt.wantWarning != (len(warnings) > 0) || (tt.wantWarning && !strings.Contains(warnings[0].(string), boardPath)) {
						t.Errorf("warnings = %v; want one naming %s = %v", warnings, boardPath, tt.wantWarning)
					}

					if status := strings.TrimSpace(gitOutputCLI(t, h.BoardDir(), "status", "--porcelain")); status != "" {
						t.Errorf("_board is dirty after reconcile; want the hub-wide configs committed:\n%s", status)
					}
				}) {
					return
				}
			}
		}},
		{"ReconcileDoesNotFailOnAPairThatVanishedMidWalk", func(t *testing.T) {
			// The counter-assertion to the unrepairable-pair step below; the two must be read together.
			// Once a per-pair error drives reconcile's exit code, the verb becomes sensitive to a race it cannot avoid: `git worktree list` is read once, before the per-pair loop, so a concurrent `lyx fabric remove`/`prune` can delete a pair between the enumeration and the iteration that reaches it.
			// Without the vanished-mid-walk verdict, that ordinary teardown would fail every enclosing reconcile.
			// Deleting the directory while leaving git's registration in place is exactly the state the race produces, and needs no actual concurrency to construct.
			const slug = "vanished-cli"
			hubforge.AddPair(t, h, slug)

			if err := os.RemoveAll(h.PairCodeWorktree(slug)); err != nil {
				t.Fatalf("remove warp worktree directory (leaving git's registration behind): %v", err)
			}

			code, output := runFabric(t, h.Location.WorktreePath(), "reconcile")
			if code != 0 {
				t.Fatalf("RunCLI(reconcile) with a pair that vanished mid-walk = %d; want 0 — a concurrent teardown is not a reconcile failure\noutput: %s", code, output)
			}

			env := envelope.RequireOK(t, output)

			pairs, isArray := env.Raw["pairs"].([]any)
			if !isArray {
				t.Fatalf("envelope has no \"pairs\" array\noutput: %s", output)
			}
			var sawVanished bool
			for _, raw := range pairs {
				pair, isMap := raw.(map[string]any)
				if !isMap {
					continue
				}
				if action, _ := pair["action"].(string); action == string(fabricengine.ReconcileActionVanishedMidWalk) {
					sawVanished = true
					if reason, _ := pair["error"].(string); reason != "" {
						t.Errorf("vanished pair carries error %q; want none — nothing failed to reconcile", reason)
					}
				}
			}
			if !sawVanished {
				t.Errorf("no pair reported %q\noutput: %s", fabricengine.ReconcileActionVanishedMidWalk, output)
			}
		}},
		{"Remove_RefusesDriftedPortalJunctionWithRefusalObject", func(t *testing.T) {
			// A real gate refusal through "fabric remove --force", reached via fabricengine.RefusalOf's own contract rather than a hand-rolled stub: the pair's portal link is hand-wired pointing at a directory OTHER than its real portal target, so removePortal's ownership check refuses, and its error propagates through Remove's %w-wrapped chain to runRemove's errWithRecord call.
			// The "refusal" object carries all four keys (check, what, target, reason), the flattened "error" string is still present alongside it, and this is the repo's only positive assertion that the "refusal" object reaches an envelope.
			hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: \"\"\n")

			const slug = "remove-refusal-portal"
			if code, output := runFabric(t, h.PrimeWorktree(), "add", slug); code != 0 {
				t.Fatalf("RunCLI(add) = %d; want 0\noutput: %s", code, output)
			}

			// The correct portal link is already wired by "fabric add" above; drift it onto a WRONG target — anywhere other than its real portal target — so removePortal's ownership check refuses.
			wrongTarget := t.TempDir()
			portalLink := h.PairPortalLink(slug)
			if err := fslink.Remove(portalLink); err != nil {
				t.Fatalf("remove correctly-wired portal link: %v", err)
			}
			if err := fslink.CreateDirLink(portalLink, wrongTarget); err != nil {
				t.Fatalf("create drifted portal link: %v", err)
			}

			code, output := runFabric(t, h.PrimeWorktree(), "remove", "--force", slug)
			if code != 1 {
				t.Fatalf("RunCLI(remove) = %d; want 1 (a drifted portal junction must be refused)\noutput: %s", code, output)
			}

			result := envelope.Decode(t, output)
			if result.OK {
				t.Errorf("RunCLI(remove) ok = true; want false")
			}
			if result.Error == "" {
				t.Errorf("RunCLI(remove) output missing non-empty 'error'; want the flattened error string alongside the refusal object")
			}
			refusal, ok := result.Raw["refusal"].(map[string]any)
			if !ok {
				t.Fatalf("RunCLI(remove) output missing 'refusal' object; got %v", result)
			}
			for _, key := range []string{"check", "what", "target", "reason"} {
				val, present := refusal[key]
				if !present {
					t.Errorf("refusal object missing key %q: %v", key, refusal)
					continue
				}
				if s, _ := val.(string); s == "" {
					t.Errorf("refusal[%q] = %q; want a non-empty string", key, s)
				}
			}
			if check, _ := refusal["check"].(string); check != string(fabricengine.CheckOwnership) {
				t.Errorf("refusal[\"check\"] = %q; want %q", check, fabricengine.CheckOwnership)
			}
			if _, present := result.Raw["mutations"]; !present {
				t.Errorf("RunCLI(remove) output missing 'mutations' key on the failure path")
			}
		}},
		{"PrimePushReportsItsUnpushedCodeBranchAndPushesRecords", func(t *testing.T) {
			// With an unpushed commit on the prime's code branch, "lyx fabric push" from the prime exits 0, pushes the records side, leaves the code branch on its bare unchanged, and names the skipped branch in code_push_skipped.
			warpBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
			warpBareBefore := gitkit.RevParse(t, h.CodeBare, "refs/heads/"+warpBranch)

			gitkit.CommitFile(t, h.PrimeWorktree(), "local-only.txt", "local\n", "local warp work")
			gitkit.CommitFile(t, h.PrimeRecords(), filepath.Join(lyxdirs.LyxDirName, "weft-after-skip.txt"), "weft\n", "weft after skip")
			weftBranch := gitkit.CurrentBranch(t, h.PrimeRecords())

			code, output := runFabric(t, h.PrimeWorktree(), "push")
			if code != 0 {
				t.Fatalf("RunCLI(push) from the prime = %d; want 0\noutput: %s", code, output)
			}
			env := envelope.RequireOK(t, output)
			skipped, _ := env.Raw["code_push_skipped"].(string)
			if !strings.Contains(skipped, warpBranch) {
				t.Errorf("code_push_skipped = %q; want it to name branch %q", skipped, warpBranch)
			}

			if got, want := gitkit.RevParse(t, h.RecordsBare, "refs/heads/"+weftBranch), gitkit.RevParse(t, h.PrimeRecords(), "HEAD"); got != want {
				t.Errorf("weft bare %s = %s; want %s (the records side must be pushed)", weftBranch, got, want)
			}
			if got := gitkit.RevParse(t, h.CodeBare, "refs/heads/"+warpBranch); got != warpBareBefore {
				t.Errorf("warp bare %s = %s; want unchanged %s (the prime's code side is never pushed)", warpBranch, got, warpBareBefore)
			}
		}},
		{"Reconcile_HubConfigPushFailureIsNonFatal", func(t *testing.T) {
			// With the committed hub-wide configs removed and the board remote pointing at an unreachable path, the healing commit lands but its push fails; reconcile still exits 0 and reports the failure under hub_config_detail, leaving the files healed on disk.
			boardDir := h.BoardDir()

			for _, module := range []string{"fabric", "board"} {
				if err := os.Remove(configengine.ConfigFile(boardDir, module)); err != nil {
					t.Fatalf("remove hub-wide %s config: %v", module, err)
				}
			}
			gitkit.MustRun(t, boardDir, "git", "add", "-A")
			gitkit.MustRun(t, boardDir, "git", "commit", "-m", "test fixture: drop hub-wide configs")
			unreachable := filepath.ToSlash(filepath.Join(t.TempDir(), "does-not-exist.git"))
			gitkit.MustRun(t, boardDir, "git", "remote", "set-url", "origin", unreachable)

			code, output := runFabric(t, h.PrimeWorktree(), "reconcile")
			if code != 0 {
				t.Fatalf("RunCLI(reconcile) = %d; want 0 (a failed hub config push must be non-fatal)\noutput: %s", code, output)
			}

			result := envelope.RequireOK(t, output)
			detail, _ := result.Raw["hub_config_detail"].(string)
			if !strings.Contains(detail, "hub-wide config committed but push failed") {
				t.Errorf("hub_config_detail = %q; want it to report the committed-but-unpushed config", detail)
			}
			for _, module := range []string{"fabric", "board"} {
				if _, err := os.Stat(configengine.ConfigFile(boardDir, module)); err != nil {
					t.Errorf("hub-wide %s config not healed: %v", module, err)
				}
			}
		}},
		{"ReconcileReportsAFailedPairAsAFailure", func(t *testing.T) {
			// The full envelope contract on the pair-failure path: exit 1, "ok":false, a non-empty "error", and — the half a bare non-zero exit would lose — the per-pair "pairs" report still present, so a caller learns WHICH pair failed.
			// Before the fix, reconcile exited through okWithRecord, so a reconcile that failed to re-point a junction printed "ok":true and exited 0 while carrying the reason only in pairs[].error.
			// The failing state is induced through adoption's ONE remaining hard refusal — a warp-side real .lyx holding an entry that collides with a non-directory at the weft target — because that refusal is deterministic and needs no racing.
			l := h.Location
			warpWorktree := l.WorktreePath()
			warpDotLyx := filepath.Join(warpWorktree, l.AnchorRel, lyxdirs.DotLyxDirName)
			weftDotLyx := filepath.Join(h.PrimeRecords(), l.AnchorRel, lyxdirs.DotLyxDirName)

			// Tear the wired junction down to a real directory, then plant a collision adoption still
			// refuses: the same name is a FILE on the warp side and a DIRECTORY on the weft side.
			if err := fslink.Remove(warpDotLyx); err != nil {
				t.Fatalf("remove wired .lyx junction: %v", err)
			}
			if err := os.MkdirAll(warpDotLyx, 0o755); err != nil {
				t.Fatalf("mkdir real warp .lyx: %v", err)
			}
			if err := os.WriteFile(filepath.Join(warpDotLyx, "logs"), []byte("a file, not a directory"), 0o644); err != nil {
				t.Fatalf("seed warp-side colliding file: %v", err)
			}
			if err := os.MkdirAll(filepath.Join(weftDotLyx, "logs"), 0o755); err != nil {
				t.Fatalf("mkdir weft-side colliding directory: %v", err)
			}

			code, output := runFabric(t, warpWorktree, "reconcile")
			if code != 1 {
				t.Fatalf("RunCLI(reconcile) with an unrepairable pair = %d; want 1\noutput: %s", code, output)
			}

			env := envelope.RequireErr(t, output, "")
			if env.Error == "" {
				t.Errorf("envelope carries no \"error\" string; want one naming the failed repair\noutput: %s", output)
			}

			// The per-pair report must survive the failure path, and the failing pair must still carry its own reason — an exit code alone tells a caller nothing about which pair to go fix.
			pairs, ok := env.Raw["pairs"].([]any)
			if !ok || len(pairs) == 0 {
				t.Fatalf("envelope has no non-empty \"pairs\" array on the failure path\noutput: %s", output)
			}
			var sawPairError bool
			for _, raw := range pairs {
				pair, isMap := raw.(map[string]any)
				if !isMap {
					continue
				}
				if reason, _ := pair["error"].(string); reason != "" {
					sawPairError = true
				}
			}
			if !sawPairError {
				t.Errorf("no pair in the envelope carries an \"error\"; want the failing pair's own reason\noutput: %s", output)
			}

			// The fixed key set holds on this path too.
			if _, present := env.Raw["mutations"]; !present {
				t.Errorf("envelope is missing the always-present \"mutations\" key\noutput: %s", output)
			}
			if env.Partial == nil {
				t.Errorf("envelope is missing the always-present \"partial\" key\noutput: %s", output)
			}
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}
