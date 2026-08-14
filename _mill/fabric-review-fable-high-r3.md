# fabric — independent review, round 3 (`fable-high-r3`)

Clean-room review of the `fabric` module per `_mill/fabric-review-prompt.md`.
Primary mission: the destruction chokepoint (`internal/fabricengine/destroy.go`), specifically the seeded containment TOCTOU residual.
Report is built incrementally during Job 1; executive summary and final severity ordering are written last.

## Executive summary

Round 3 was weighted, as instructed, on the destruction chokepoint, and the seeded residual is REAL and REPRODUCED: a containment TOCTOU in `destroy.go` lets a gated `remove --force` delete a file OUTSIDE the hub through an intermediate symlink flipped in the window between the gate's check and the executor's act (finding **M1**). It defeats exactly the property `doc.go` calls "the one thing `--force` can never override." I graded it **MEDIUM** — real out-of-hub data loss, but requiring an adversarial concurrent writer inside hub geometry plus tight timing; the normal single-instance flow is unaffected.

The fix closes the window rather than shrinking it: the two arbitrary-path executors (`removePath`, `removeLink`) now perform their removal through an `os.Root` (Go 1.26) rooted at the gate's declared container, so component resolution and the unlink are one `openat` chain that atomically refuses to traverse a symlink escaping the container, while still removing a final-component junction link as a link. This binds resolution to the act — a second `EvalSymlinks` would only have narrowed the same class of gap. I verified the `os.Root` semantics against every case the executors depend on (intermediate escape, escaping leaf link, dir-link, absent target, `..`-escape, recursive tree with escaping children) via standalone probes before writing the fix.

Secondary target **N4** (dirtiness-probe TOCTOU) was given a dedicated attempt; I could not improve it beyond PLAUSIBLE / CONFIRMED-by-source and traced exactly why (the only reachable probe→RemoveAll paths are either pre-checked or bypass the probe via `force`), matching the accepted documented residual. No unrelated defects surfaced in the rest of the module during this pass.

**Top risks:** M1 (fixed this round). No BLOCKING findings.

Counts: 1 MEDIUM (M1), 0 LOW, 0 NIT beyond the residuals noted. Windows path behavior remains out of scope (unreachable from Linux) and is the standing limit on the verdict.

**Merge-readiness:** MERGEABLE once M1's fix lands green (it does — see fixer report). The chokepoint's core promise is restored and regression-guarded.

## Scope assessment (plan vs shipped)

- The module's scope matches `doc.go`'s spec: the one-repo illusion, the destruction chokepoint, the mutation record, the correspondence-index write path, snapshot trailers, clone-does-everything. Nothing plan-promised is silently dropped that this pass found.
- The destruction chokepoint delivers its intended shape (closed ownership enum, gate-executes-not-approves, `--force`-answers-dirtiness-only, bypass guard) — the ONE gap is M1, which is a completeness hole in the containment CHECK→ACT binding, not a scope omission. The chokepoint's own doc already names "the gate's checks are not atomic with its acts" as a stated limit; M1 shows that limit had real teeth for the arbitrary-path executors, and the fix removes those teeth for the containment dimension via act-time rooted resolution.
- No shipped-beyond-scope over-reach found. No deferred-that-should-be-v1 found beyond the acknowledged Windows gap and the accepted N4/RebuildIndex residuals.

## Code findings

### M1 (PRIMARY, CONFIRMED) — containment TOCTOU: gated `remove --force` deletes files OUTSIDE the hub through an intermediate symlink flipped in the check→act window

- **Where:** `internal/fabricengine/destroy.go` `removePath` (act at lines ~699-719) and, by the same class, `removeLink` (~753-768); reachable from `removeLaunchers` (launchers.go), `removeStalePair`/`removeWeftWorktree` fallbacks, `teardownHub`/`resetHub`, junction removals.
- **Scenario (CONFIRMED, reproduced live):** plant a symlink at an intermediate path segment of a gate target — `<Hub>/_launchers/<slug>` — dangling (target absent) at the moment `checkPathRequest` runs. `checkPathRequest`'s absent-target short-circuit (`os.Lstat(req.target)` → ENOENT → `return nil`, destroy.go:593) skips ALL four checks. `removePath` then re-`Lstat`s the SAME nominal path at a later instant (destroy.go:699) and, if the symlink has since been flipped live-and-escaping (→ a dir OUTSIDE the hub) in that window, `os.Remove(req.target)` follows the intermediate symlink and deletes the outside file, appending `path_removed` and reporting `ok:true` (partial) for a removal that landed outside the hub.
- **Confirmation method:** built a real hub from local bare git remotes (`git init --bare` warp + weft, `lyx fabric clone`, `lyx fabric add race`), planted `_launchers/race` as a dangling symlink, and — using a throwaway env-gated sleep inserted between `checkPathRequest` and the act in `removePath` (reverted before any fix) — flipped the symlink live→`$SCRATCH/outside` inside the window. Result across 3 independent trials: `OUT/ide.sh` = **DELETED** every time, a file that never lived under the hub, via a gated `remove --force`. An external high-rate compiled toggler (6 workers, atomic rename) across 60 trials produced 0 hits UNINSTRUMENTED — the window between the two `Lstat`s is nanosecond-scale, which is why the deterministic sleep-widened confirmation is the honest evidence; the window's EXISTENCE (not its natural hit-rate) is what matters, and it is proven.
- **Root cause:** the check resolves the nominal path at one instant (or short-circuits on absent) and the executor acts on the same nominal path at a later instant, with nothing tying the resolution to the unlink. `filepath.EvalSymlinks`/`resolveAncestorSymlinks` at check time cannot bind the act.
- **Severity: MEDIUM.** It defeats containment — which `doc.go` calls "the one thing `--force` can never override" — with real data loss OUTSIDE the hub, but requires an adversarial concurrent writer with write access to hub geometry AND tight timing; the normal single-instance flow is safe. Grading MEDIUM (not BLOCKING) on that basis, while noting it is exactly the class the chokepoint exists to make impossible.
- **Fix (window-closing, not window-shrinking):** perform the removal through an `os.Root` (Go 1.26) rooted at the gate's declared `container`, using the container-relative path. `os.OpenRoot`+`Root.Remove`/`Root.RemoveAll` resolve each path component and unlink as one openat chain that atomically REFUSES to traverse a symlink escaping the container ("path escapes from parent"), while still removing a final-component link as a link (junction removal unaffected). Verified via standalone probes: intermediate escape refused + outside preserved; escaping-leaf link removed as a link, target preserved; legitimate nested removal works; absent target → IsNotExist (idempotence preserved). This binds resolution to the act, closing the window rather than narrowing it. Apply to both `removePath` and `removeLink`; add an integration regression test racing the toggle the same way, plus a deterministic escaping-symlink test that would fail on the old nominal-path executor.
- **Note:** the existing check-phase resolution stays as defense-in-depth; the act-time rooted removal is the actual closer.

### N4 (secondary target) — dirtiness-probe TOCTOU: attempted, still PLAUSIBLE, not improvable to CONFIRMED

- **Where:** `checkPathDirtiness` (destroy.go) runs `git status --porcelain` then the executor acts, with no lock spanning the two.
- **Attempt / analysis (source-traced, this round):** enumerated every reachable probe-then-destructive-act path where the dirtiness scope is real (not `dirtinessNA`) AND force is false (force short-circuits the probe at destroy.go:636):
  - `removeGitWorktree` / `resetHardTo` delegate the act to git, which re-validates at its own instant — no window this review can widen.
  - The only `RemoveAll`/`os.Remove` sites carrying a real scope are the two teardown fallbacks (`removeWarpWorktreeDir`, `removeStalePair`), and BOTH pass `force: true`, so `checkPathDirtiness` returns before probing — there is no probe→RemoveAll pairing there at all.
  - `removeWarpWorktreeDir`'s primary request can carry `force:false`+`dirtyScopeAll`, but `Remove` runs its OWN `worktreeDirty(scopeAll)` pre-check first (remove.go:73-81) and aborts if dirty, so the fallback-with-force=false path is reached only when the worktree was clean at the pre-check but `git worktree remove` failed for a NON-dirtiness reason (e.g. a `git worktree lock`). Only in that already-narrow state does a probe→RemoveAll window exist, and threading it needs the target to be clean at the pre-check, still-clean at the fallback probe, and dirty by the RemoveAll instant.
- **Verdict:** I could not construct a live, isolable repro either, for the same reason round 2 and the orchestrator could not — the reachable window is guarded by an earlier pre-check and/or bypassed by `force`. Recording as PLAUSIBLE / CONFIRMED-by-source, matching the accepted documented residual in `doc.go` ("The gate's checks are not atomic with its acts"). Not separately fixed: it is the documented accepted narrow residual, and no evidence this round shows it worse than documented. The M1 fix does not address it (dirtiness is orthogonal to containment), nor need it.

### Other adversarial chokepoint shapes tried (no new escape)

- **Symlink loop (A→B→A)** at a gate-resolved intermediate: `EvalSymlinks` fails ELOOP → `resolveAncestorSymlinks` lexical fallback (current behavior). Under the M1 `os.Root` fix the act-time `openat` returns ELOOP → refused. No escape survives the fix.
- **`..`-relative symlink target**: `os.Root` probe confirms `Remove("../out/leaf")` → "path escapes from parent", target preserved. Handled by the fix.
- **Launcher-dir direct `os.Remove(launcherDir)`** (launchers.go:242, allowlisted): removes the FINAL-component link only (never follows it), so a symlink AT `_launchers/<slug>` is removed as a link — safe. An escape would require `_launchers` itself (the container / hub geometry) to be replaced by a symlink, a materially higher bar than the confirmed `_launchers/<slug>` intermediate. Recorded as a lower-priority residual; the M1 executors are the confirmed-vulnerable path. Left as-is (routing it through a destroy.go helper would move an allowlisted call and is not justified by the narrower exposure).
- **Worktree-removal fallback recursion**: `Root.RemoveAll(<slug>)` over a worktree tree containing escaping junction children removes the children as links and preserves their targets (probed) — the fallback is safe, indeed safer, under the fix.

## Docs & operability findings

- `doc.go`'s "The gate's checks are not atomic with its acts" section correctly states the general limit but implied the exposure was bounded to dirtiness ("a write landing in that window is destroyed") — M1 shows the CONTAINMENT dimension had a real out-of-hub escape for the arbitrary-path executors. The M1 fix's doc update (in the same commit) records that the arbitrary-path executors now bind resolution to the act via `os.Root`, so containment is no longer subject to that window; the dirtiness window (N4) remains as documented.
- No other doc/code drift found this pass. Help text (`lyx fabric --help`) matches the shipped verb set.

## Review status

Job 1 (independent review) COMPLETE at this point — findings formed and written to disk before any production/test file is touched for the fix. Job 2 (fixes) follows below in the fixer report file.

## What was tested

Observations appended immediately after each command/scenario returns.

### Code-reading pass (pre-substrate)

Read in full: `internal/fabricengine/doc.go` (spec), `destroy.go`, `ancestors.go`, `remove.go`, `launchers.go`, `portals.go`, `prune.go`, `cleanup.go`, `dirtiness.go`, and the pathRequest regions of `junction.go`, `weftwiring.go`, `clone.go`, `add.go`, `unwire.go`; `CONSTRAINTS.md` in full; `internal/fslink` API surface.
Enumerated all 16 `pathRequest{` construction sites via grep (junction.go:591, weftwiring.go:177/209, clone.go:629/685, launchers.go:209/230, prune.go:278/310, add.go:270, unwire.go:151, portals.go:64, remove.go:224/273, destroy.go:778/888).

Static observations feeding the TOCTOU hunt (to be confirmed live):

- `checkPathRequest`'s absent-target short-circuit (`os.Lstat(req.target)` → `nil` on ENOENT) means a target that is absent at check time SKIPS all four checks, yet `removePath` re-`Lstat`s at act time and removes if the target is present THEN — so a dangling-then-live intermediate symlink gets an entirely UNCHECKED removal, not merely a containment-fallback one. Two distinct windows: (1) absent-at-check → present-at-act = zero checks run; (2) dangling-symlink ancestry at check → `resolveAncestorSymlinks` lexical fallback treats path as contained → act follows the now-live symlink.
- Executors act on the NOMINAL `req.target`: `removePath` (`RemoveAll`/`os.Remove`), `removeLink` (`fslink.Remove` = `os.Remove`). `removeGitWorktree` and `resetHardTo` delegate the act to git, which re-validates worktree registration at its own instant — much narrower exposure.
- Go toolchain is 1.26 (`go.mod`), so `os.Root` (openat-based, symlink-escape-proof traversal; `Root.Remove`/`Root.RemoveAll` available) is a candidate act-time enforcement layer that closes the window rather than shrinking it.
- `fslink.Remove` is plain `os.Remove` with idempotence — no Windows-specific removal path, so a root-relative removal has identical leaf semantics.
- The exported `RemoveAll` seam var in destroy.go has no current test consumer (grep found zero assignments outside its declaration).

### Live substrate driving

- Hermetic gates: `go build ./...` rc=0; `go vet` (4 fabric pkgs) rc=0; `go test ... -count=5` all `ok`.
- Integration baseline: `go test -tags integration` (fabricengine/fabriccli/gitexec/gitrepo) all `ok`, 0 FAIL/panic markers.
- Built real hub from local bare remotes: `git init --bare -b main testrepo.git` (with an initial commit) + empty `git init --bare -b main testrepo-weft.git`, then `lyx fabric clone <weft.git> <warp.git>` → hub materialised with `_board`, `testrepo`, `testrepo-weft`, junctions wired. `lyx fabric add task1` → pair + portal + launchers + junctions + branches pushed. Drove clone/add/remove directly, foreground.
- **os.Root fix-design probes (standalone Go programs, Go 1.26):**
  - intermediate escaping symlink `container/evil -> outside`: `Root.Remove("evil/canary.txt")` and `Root.RemoveAll(...)` both fail with "path escapes from parent"; outside canary PRESERVED.
  - final-component escaping symlink (junction-shaped): `Root.Remove(link)` removes the LINK only; escaping target PRESERVED. Same for a dir-link.
  - `Root.Lstat(escaping-leaf)` → no error, reports symlink (junction leaf detection works).
  - `Root.Remove(absent)` → IsNotExist (idempotence preserved).
  - `Root.Remove("../out/leaf")` → "path escapes from parent", preserved.
- **M1 deterministic confirmation:** see M1 finding above — 3/3 escapes with the check→act window widened by throwaway instrumentation (reverted).
