# loom review — round 3 (fable5-high-r3) — glyph-hardening campaign, TWO MISSIONS

Reviewer-fixer: crucible round agent (Fable 5, high effort), 2026-09-07.
Mission A: adversarial review + live verification of `d7c52df6c` (webster standalone mode: `NewDetachedRunner` for F16, `standalonegeom.LogsDir` sink redirect for F22).
Mission B: third-pass safety sweep over loom's glyph surface + attempt to close the live Rename-through-Webster gap.

Status: JOB 1 IN PROGRESS — provisional findings recorded as spotted, per the log-as-you-go rule.

## Executive summary

(to be completed at end of Job 1)

## Findings (provisional until Job 1 closes)

### F1 [Mission B] — resolvePass raises blocking `glyph-not-found` on a file-rename pair's New side — any plan carrying a file rename fails validation

- **Where:** `internal/planglyph/planglyph.go` (`collectGlyphTargets` collects `p.New` unconditionally; `resolvePass` excludes only Create targets from `statusFindings`), `internal/planglyph/resolve.go` (`statusFindings` reports `not_found` as blocking).
- **Scenario:** a plan carries a file-rename pair, e.g. `` `//internal/boardengine/rows.go` -> `//internal/boardengine/rowsjson.go` `` (the exact shape the spec's own worked example, card 5 of `contracts/specs/loom-plan-spec.md`, demonstrates). Both sides canonicalize at parse time to file self glyphs (`normalize.go`'s `canonicalizeCard` runs on both `Pairs` endpoints). The New side (`internal/boardengine/rowsjson.go#`) names a file that only exists AFTER the rename lands, so quarry resolves it `not_found` — and `statusFindings` reports that as the blocking finding `glyph-not-found`. `Plan-Validate`, `Plan-Revalidate`, webster's run-entry gate, and every `ValidateDispatch` before the rename card lands all refuse the plan.
- **Contrast with the pure layer:** `planparser.checkPathMissing` deliberately never checks `Pairs.New` ("its Refs are skipped entirely (so a Rename's New side is never checked)") and maintains `renameTargetsUnion` to satisfy LATER cards' refs to the destination. The resolve-backed layer forgot the same exclusion.
- **Severity:** BLOCKING. **Confidence:** CONFIRMED by code trace; live/unit repro pending (see What was tested).
- **Fix:** exclude Rename pairs' New-side refs from `statusFindings`' input, mirroring the Create-target exclusion (a `renameNewTargetSet` alongside `createTargetSet`); keep resolving them is fine but their `not_found` must not report. Add an integration test with a file-rename plan.

### F2 [Mission B] — `websterengine.Run` validates the WHOLE plan at entry — a mid-plan resume with any completed Create/Delete/Rename card is permanently wedged

- **Where:** `internal/websterengine/runlevel.go:349` — `planglyph.Validate(plan, deps.Geom.WorktreeRoot)` runs before `LoadState`, unscoped by completed cards.
- **Scenario:** a run completes batch 1 (a Create card; its symbol now exists), Master crashes (laptop reboot, session killed). Operator resumes via `lyx webster run` — the resume path the master stencil itself advertises twice ("fully resumable later with `lyx webster run`"). Run entry re-resolves the whole plan: the completed Create target resolves `found` → blocking `create-already-exists` → run refused. Same for a completed Delete (`glyph-not-found`) or Rename (`glyph-not-found` + `rename-old-unresolved`). `--fresh` does not help (it only fires on fingerprint mismatch, and would discard the run anyway). This is the exact wedge round 1 fixed at the begin-batch boundary (`ValidateDispatch`), left open at the run-entry boundary. It hits hub mode too: loom's `Webster` row re-invokes `websterengine.Run` on shed resume, and Webster carries no `on_stuck` — a human is the only recovery.
- **Severity:** BLOCKING. **Confidence:** CONFIRMED by code trace (validation at line 349 precedes state load at 407; resume-with-matching-fingerprint proceeds without re-init); live repro pending.
- **Fix:** load state first (or peek), compute `completedCards(batches, st, 0)`, validate via `ValidateDispatch`; keep the approval gate (ValidateDispatch runs the format-only pure set — assert `plan.Approved` alongside, or validate whole-plan only when `st == nil`/fresh). Add a state-seeded regression test.

### F3 [Mission B] — no `rename-not-done` done-check: a fork that skips its Rename card records clean

- **Where:** `internal/planglyph/donecheck.go` (`DoneChecks` covers Create/Delete groups only), `internal/planglyph/handle.go` (`BindHandles` covers Create declarations only), `internal/websterengine/recordbatch.go` (no other rename-completion gate).
- **Scenario:** a batch's card carries `Rename: old -> plan:new`. The fork commits something unrelated (or nothing rename-shaped), writes `status: done`. `DoneChecks` finds no Create/Delete groups → passes. `BindHandles` finds no Declarations → passes. The delta contains no rename → `DetectDrift` has nothing to intersect → passes. The batch records terminal `done` with the rename never performed. Later cards referencing the to-side handle are excluded from resolve (handle-shaped), so no later gate catches it either — the failure surfaces only when an agent hits the missing symbol, or never.
- **Contrast:** Create has `create-not-done`, Delete has `delete-not-done`; Rename has the same mechanical verdict available (old must no longer resolve; the to-side handle's expected glyph must resolve) and lacks it.
- **Severity:** MEDIUM. **Confidence:** CONFIRMED by code reading (no gate exists); live repro not required to establish absence.
- **Fix:** extend `DoneChecks` with `rename-not-done`: for each Rename group pair, the Old side still resolving found/multipart blocks, and the New side (via `resolveKeyFor`) still not resolving blocks. File-rename pairs (self glyphs) get the same treatment — old file glyph must stop resolving, new one must resolve.

### F4 [Mission B] — identity substitutions: already-canonical handles are "rewritten" (byte-identical) on every validation pass, contradicting RewriteRefs' own no-op contract

- **Where:** `internal/planglyph/handle.go` (`CanonicalizeHandles` builds `subs[owners[0]] = canonical` even when `owners[0] == canonical`), `internal/planparser/rewrite.go` (`rewriteBulletLine` reports `changed` for an identity substitution; `rewriteCardFile` then writes byte-identical content, violating the doc claim "A card file whose bytes do not actually change is left byte-identical (not even rewritten with identical bytes)").
- **Scenario:** every `ValidateDispatch`/`ValidateFormat` call over a plan whose handles are already canonical (i.e. every call after the first) re-runs `quarry.Name`, produces identity subs, calls `RewriteRefs` (rewrites every declaring card file with identical bytes, bumping mtimes), reports `rewrote=true`, and forces a full plan re-parse — every begin-batch, every gate, forever.
- **Severity:** LOW (wasted work + doc/behavior mismatch; no incorrect output). **Confidence:** CONFIRMED by code trace.
- **Fix:** skip identity pairs when building `subs` in `CanonicalizeHandles` (one-line filter), and/or make `rewriteBulletLine` report unchanged when the rebuilt line equals the input.

### F5 [Mission B] — stale "format-4" doc comments in `planparser/plan.go` while `recognizedFormat = 5`

- **Where:** `internal/planparser/plan.go` — file header ("one flat, format-4 plan-format card"), `Plan.Format` field doc ("The only version Validate currently recognizes is 4"), `Card` doc ("one flat, format-4 plan-format card"). `internal/planparser/validate.go` has `recognizedFormat = 5`; the spec and stencils say `format: 5`.
- **Severity:** NIT. **Confidence:** CONFIRMED.
- **Fix:** update the three comments to format-5.

### F6 [Mission B] — `renameCardPairs` claims "plan-wide" but receives the pending plan; DetectDrift's gate one can never see the executing batch's own Rename card

- **Where:** `internal/planglyph/drift.go` (`renameCardPairs` doc: "indexes every declared Rename card's own Old->New pair, plan-wide"), `internal/websterengine/recordbatch.go:266` (passes `PendingPlan(...)` which excludes the recording batch's own cards).
- **Scenario:** the executing batch's Rename card's own delta entry never matches gate one (its pair is not in the pending plan); it is instead absorbed by gate two (nothing pending references the old glyph in a well-formed plan) — safe in the well-formed case. In the ill-formed case (a pending card still referencing the OLD glyph), the declared rename is auto-repaired as if it were drift, with an "exact"-tier amendment recorded for a change that was a declared card outcome, not drift. Behavior is safe either way; the semantics and the doc claim are wrong, and gate one is reachable only for a rename executed AHEAD of its declaring pending card.
- **Severity:** LOW (doc/semantics; amendment log mislabels a declared rename as drift in the ill-formed-plan case). **Confidence:** CONFIRMED by code trace.
- **Fix:** thread the full plan into `renameCardPairs` (e.g. `DetectDrift(fullPlan, pending, ...)` or pass the pair index separately), so a declared pair is recognized regardless of its card's completion state; align the doc comment.

### F-A1 [Mission A] — standalone `lyx webster run` STILL cannot start Master: no reed session exists on webster's standalone geometry and no verb can create one

- **Where:** `internal/webstercli/run.go` (never brings reed up), `internal/burlercli` (same gap for standalone `lyx burler run`), `internal/reedcli` (hub-only: its pre-run requires `lyxcwd.Resolve`, so `lyx reed up` cannot target the derived state directory's geometry — socket `lyx-<hash8>`, state `<stateDir>/.lyx/reed.json`).
- **Observed live (post-`d7c52df6c`, deployed binary at `060629a75`):** `lyx webster run --target-dir /home/knatte/Code/lyx-r3-standalone --plan-dir <plan>` from the plain repo → `{"error":"webster: start master: shuttle: add strand: no reed session; run \"lyx reed up\"","ok":false}`. The advised recourse is impossible: `lyx reed up` is hub-mode-only (run from the plain repo it either fails `lyxcwd.Resolve` or — as happened here, because `/home/knatte/Code` happens to carry hub-shaped containers — anchors a DIFFERENT, hub-shaped session at socket `lyx-Code-f8c4907f`, which webster's standalone geometry (`lyx-b1c70921`) never finds). Retrying `run` with that session up refused identically.
- **Assessment:** `d7c52df6c` genuinely removed F16's proximate refusal — the run now gets PAST `NewDetachedRunner` (pre-fix it died at the constructor's told-path verdict, per round 1's F16 transcript) and reaches reed's own pre-flight — but the mission-A observable ("Master actually starts now") is still false end-to-end. The `#004` task's own tests deliberately stop at the "no session" error (`TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError` asserts exactly that error as the success criterion), and its discussion's scope never mentions session bring-up, so this was never covered. `loomcli`'s own `run`/`drive` verbs solve the identical problem in hub mode by calling `c.reed.Up()` in-process before driving — webster/burler standalone have no equivalent.
- **Severity:** BLOCKING (the standalone mode's documented entry point remains dead). **Confidence:** CONFIRMED LIVE.
- **Fix:** call the wired reed engine's `Up()` (idempotent) in `webstercli`'s `run` verb before dispatching to `websterengine.Run`, mirroring `loomcli/run.go:145`; same for `burlercli`'s run path. Then re-verify the whole standalone scenario live.

### F-A2 [Mission A, observation — fix confirmed working] — F22's sink redirect verified live

The failed run attempts each wrote their `trace-*.log` under `/home/knatte/.local/state/lyx/b1c70921/.lyx/logs/` (five files observed), and the target repo's `git status` stayed clean of webster-authored content. The `.lyx/` that DID briefly appear in the target came from the accidental hub-mode `lyx reed up` above (reed state + hub-mode trace logs anchored at cwd), i.e. from reed's pre-existing hub path — not from the standalone webster code `d7c52df6c` touched. No finding against the F22 fix itself; full-run confirmation pending the F-A1 fix.

### F-B7 [Mission B] — `lyx loom run` silently resumes an INHERITED task's state: no slug-vs-worktree guard on the status file

- **Where:** `internal/loomcli/run.go`/`drive.go` seed-or-resume path (status file seeded only "when it is absent"); observed via the sanctioned sandbox hub.
- **Observed live:** `lyx fabric add glyph-rename-format` run FROM round 2's completed `glyph-demo-greet` worktree (the documented fork-from-HEAD behavior, and this round's prompt-recommended seeding path) carries the weft's `_lyx/loom/status.json`, `_lyx/discussion/`, `_lyx/plan/`, `_lyx/webster/` into the new pair. `lyx loom run` in the NEW worktree then resumed the OLD task's terminal state — `lyx loom status` reported `"slug":"glyph-demo-greet"` (not the current worktree), `current_producer: Finalize`, `state: failed`, and the driver re-failed Finalize on the old run's missing summary artifact. No guard notices that the status file's recorded slug does not match the worktree it is driving.
- **Scenario beyond this rig:** any operator continuing work by forking a task worktree (the exact "exact continuation" feature `fabric add`'s own help advertises) gets a new task whose loom run silently belongs to the old slug — with an inherited `state: running` this could re-drive the OLD task's producers against the NEW task's board entry.
- **Severity:** MEDIUM. **Confidence:** CONFIRMED LIVE.
- **Fix:** at loom's seed/resume boundary, refuse loudly when the existing status file's `slug` differs from the current worktree's slug, naming both and the recorded state, with the recourse spelled out (reset the inherited `_lyx` task state, or run from the original worktree). (Recovered here by an operator weft commit removing the inherited state; the fresh run then seeded correctly.)

### Mission A — statically clean so far

`NewRunner`'s containment assertion is byte-untouched by `d7c52df6c` (`validateToldPaths` unchanged; all four hub callers — `burlercli` hub, `webstercli` hub, `shuttlecli` (hub-only, requires `lyxcwd.Resolve`), `loomcli` — still construct via `NewRunner`). `NewDetachedRunner` validates strict disjointness in both directions plus non-empty/absolute on all three paths, and error strings name the constructor. `wait.go`'s `AuditForks(sessionID, paneCwd)` is behavior-preserving in hub mode (`NewRunner` sets `paneCwd = anchorPath`). The sink redirect resets `sinkOnce`, so even a pre-redirect arming attempt (which fails in a plain repo — `lyxcwd.Resolve` refuses) cannot pin the sink to the wrong directory; the AST guard (`cmd/lyx/prerunlogging_test.go`) pins root pre-run ordering. `sweepOrphansOpportunistic` reads reed state at `anchorPath/.lyx` = `stateDir/.lyx`, matching `reedengine`'s own `stateDir()` derivation for standalone geometry. Live verification pending.

## Scope assessment

- Mission A: the fix is narrow and matches its CONSTRAINTS.md claim; nothing shipped beyond scope spotted in the diff (18 files, all accounted for: the two wirings, logger's atomic set-with-worktree-root, the detached constructor + its validator, LogsDir, tests, tier-purity allowlist entry, CONSTRAINTS line).
- Mission B: design-intent vs shipped is aligned for the surfaces read (spec ↔ validate.go check set ↔ planglyph doc.go ID list ↔ recipe row wiring ↔ stencils), with the exceptions recorded as findings above.

### F1b [Mission B] — duplicate finding attribution: a rename endpoint is double-counted (Targets projection + Pairs walk)

- **Where:** `internal/planglyph/resolve.go` (`targetCards` adds a card once per Targets entry and again per Pairs endpoint; both endpoints of every pair are ALSO projected into Targets by the parser).
- **Observed live:** the F1 repro reports the identical `glyph-not-found` finding twice for the same card+target (see What was tested).
- **Severity:** LOW. **Confidence:** CONFIRMED (observed).
- **Fix:** dedupe per (target, card) in `targetCards`.

## What was tested

All commands run from the worktree root with the deployed dev binary at HEAD; `CGO_ENABLED=1` throughout (default on this machine).

- `go build ./...` — PASS (exit 0).
- `go vet` over the full expanded package set (loom*, shed*, hubgeom, planparser, planglyph, shuttleengine, standalonegeom, webstercli, burlercli, logger) — PASS.
- `go test -count=5` over the same set + `./cmd/lyx/...` — PASS (no failures; exit 0).
- `go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/webstercli/... ./internal/burlercli/...` — PASS.
- `go test ./...` (whole repo) — PASS.
- **Stale-binary check (round 2's hazard, re-observed):** the installed `/home/knatte/go/bin/lyx` predated `d7c52df6c` (built Sep 6 19:38; the merge landed 21:16). Redeployed via `CGO_ENABLED=1 go run ./tools/deploy` → `Deployed lyx @ 060629a75` before any live driving.
- **F4 live repro (identity rewrite):** backdated the farewell plan's two card files to 10:00, ran `lyx webster validate` (no plan change of any kind — the handle was already canonical) → both files' mtimes jumped to the validation instant, proving the byte-identical rewrite + reload fires on every pass.
- **Mission A live attempt 1 (pre-any-fix):** `lyx webster run --target-dir /home/knatte/Code/lyx-r3-standalone --plan-dir /home/knatte/Code/lyx-r3-plan-farewell` → `webster: start master: shuttle: add strand: no reed session; run "lyx reed up"` (F-A1). Trace logs from the attempt landed at `~/.local/state/lyx/b1c70921/.lyx/logs/trace-*.log` (F22 fix observed working); a `_lyx/webster/state.json` with zero batches was created there. `lyx reed up` from the plain repo anchored a hub-shaped session (socket `lyx-Code-f8c4907f`) because `/home/knatte/Code` carries hub containers; retry refused identically; session torn down with `lyx reed down`, stray `.lyx` removed from the target via `git clean`.
- **Mission B live (hub mode, the Rename gap):** board task `glyph-rename-format` ("Rename FormatGreeting to ComposeGreeting in services/api", pure-rename brief) upserted in the round-2 sandbox hub; `lyx fabric add glyph-rename-format` run FROM the completed `glyph-demo-greet` worktree so the new pair's HEAD carries the pre-existing `FormatGreeting` (`lyx quarry resolve 'services/api#FormatGreeting'` → found). First `lyx loom run` resumed round 2's INHERITED terminal state (F-B7, observed verbatim: status slug `glyph-demo-greet`, Finalize failed on the old run's missing summary). Recovered by an operator weft commit removing the inherited `_lyx/{loom,discussion,plan,webster}`; re-ran `lyx loom run` — attach commands: `cd /home/knatte/Code/lyx-test-HUB/glyph-rename-format && lyx reed status` / `lyx reed attach` (socket `lyx-lyx-test-HUB-d919e29a`, session `glyph-rename-format`). Producer trail observed live via `lyx loom status` polling: `Loom-Preflight → Discussion-Write (real session) → Discussion-Validate → Discussion-Bouncer …` (in progress; completed trail recorded below as it lands). Decision record read: properly rename-shaped (D1 rename in place, no alias; D2 test renamed too).
- **F1 live repro (standalone `lyx webster validate`):** seeded a plain git repo `/home/knatte/Code/lyx-r3-standalone` (greeter module, committed) and a plan `/home/knatte/Code/lyx-r3-plan-filerename` whose single card carries the file-rename pair `` `//greeter/greeter.go` -> `//greeter/hello.go` `` (approved: true, Rename mechanic present). `lyx webster validate --target-dir ... --plan-dir ...` (cwd: the plain repo) → **refused with 2 blocking findings**: `glyph-not-found` / `target "greeter/hello.go#"'s unit does not exist`, reported twice for card `1-greeter-file-rename`. F1 CONFIRMED live; the duplication is F1b.
