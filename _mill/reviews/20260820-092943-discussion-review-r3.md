MILL_REVIEW_BEGIN
# Review: Extract scout into its own standalone repo

```yaml
duration_s: 355.0
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: /home/knatte/Code/loomyard/wts/scout-extract-standalone-repo/_mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Step 4's benchmark-symbol ground truth does not exist
**Section:** Testing → "Task-wide verification", step 4
**Issue:** All five recorded Go positions in `docs/research/scout-multilang.md:69-73` are stale — `internal/hubgeometry` no longer exists at all (symbols 1 and 2), `output.Err` is at `internal/output/output.go:27:6` not `:32:6`, `state.ReadJSON[T]` at `internal/state/state.go:59:6` not `:49:6` — and `:44` states the ground truth (`.scratch/codeintel/ground-truth.json`) is gitignored and "not part of this commit", so the claim that these symbols "have established ground truth, which makes this a real check" is false on both halves. Both binaries would return `ErrSymbolNotFound` for identical stale inputs and the envelope comparison would pass while proving nothing.
**Fix:** Replace the symbol source with a query set resolved fresh at the port commit, and require step 4 to assert a non-empty, non-error result on the `lyx scout` side before the two envelopes are compared.

### [NIT:decision] Toolchain cache is a third path axis with no disposition
**Demoted-from:** BLOCKING
**Section:** `config-and-state-paths`, `state-path-ownership-moves-to-the-caller`
**Issue:** The discussion splits "the two axes that `anchorRoot` conflates" and asserts config/state resolution moves wholly into `internal/cli/`, but `internal/scoutengine/toolchain.go:34-53` derives a third path inside the engine — `os.UserCacheDir()/lyx/tools/go/<version>` and `/install.lock` — carrying a literal `lyx` segment. That is the same engine-derives-its-own-path property the state decision fixes, and the same "meaningless lyx segment inside `os.UserCacheDir()`" the state decision's rejected-alternative rules out.
**Fix:** State whether the `lyx` segment becomes `quarry` and whether the toolchain cache root is told from `internal/cli/` like state and config, or deliberately left engine-derived with a stated reason.

### [BLOCKING:design] No test-isolation seam for the two new machine-global roots
**Section:** `config-and-state-paths`; Testing, TDD candidates 1-2 and the "ported unchanged" list
**Issue:** `os.UserConfigDir()` and `os.UserCacheDir()` are process-global and unaffected by cwd, yet no seam is specified for redirecting them under test — unlike `toolchain.go:30`, which already exists precisely for this (`var userCacheDir = os.UserCacheDir`). TDD candidate 1 explicitly tests the `os.UserConfigDir()` default tier, which cannot be exercised hermetically on Windows or macOS by env alone; and `internal/scoutcli/cli_test.go` is listed "ported unchanged" while its isolation mechanism is `t.Chdir(t.TempDir())` (`:84,167,207,798`) chosen so `lyxcwd.Resolve` degrades — a premise this task deletes, leaving the ported tests reading the operator's real `servers.yaml`.
**Fix:** Name the redirection seam (package-level `userConfigDir`/`userCacheDir` vars, matching `toolchain.go`'s existing pattern) and move `cli_test.go` off the ported-unchanged list.

### [NIT:decision] `ConfigTemplate()`/`template.yaml` disposition unstated
**Section:** Technical context → "What scout actually is"; `config-path-ownership-moves-to-the-caller`
**Issue:** `scoutengine.ConfigTemplate()` has zero call sites in Loomyard (confirmed by grep; `doc.go:283` records "no lyx config reconcile wiring for servers.yaml yet"), and the new precedence chain seeds nothing either — so the port carries dead exported API into quarry's *public* package.
**Fix:** Say whether `template.go`/`template.yaml` are dropped, ported as-is, or wired to a seeding verb.

### [NIT:scope] No quarry-side lyx-vocabulary sweep
**Section:** Scope; `mechanical-move-not-hand-transcription`
**Issue:** 87 case-insensitive `lyx` occurrences sit across 16 `scoutengine` files — `registry.go:6,99`, `ensureserver.go:40,160,257,260,496`, `daemonstate.go`, `toolchain.go:10` (a dangling `_mill/discussion.md` citation), and `template.yaml:2`, which is embedded, operator-visible content. The port program is restricted to imports and package clauses, and only `doc.go` is named for rewriting, so the rest survives verbatim. The Loomyard side gets a mandated two-sweep enumeration; the quarry side gets none.
**Fix:** Add a quarry-side vocabulary sweep to scope, calling out `template.yaml` as user-visible rather than comment-only.

### [NIT:scope] Quarry's supported-platform set is never stated
**Section:** `dependency-strategy-copy-vs-replace`; `config-and-state-paths`
**Issue:** `internal/proc` contains only `proc_linux.go` and `proc_windows.go` — GOOS-suffixed with no darwin or BSD file — so copying it verbatim yields a quarry that does not compile on macOS, while the config rationale reasons about `~/Library/…` and the repo is to be "presentable as standalone".
**Fix:** State the supported GOOS set explicitly and whether a darwin `proc` implementation is in or out of this task.

## Verdict

REQUEST_CHANGES
Equivalence proof rests on deleted symbols; a third path axis and test isolation are unspecified.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
