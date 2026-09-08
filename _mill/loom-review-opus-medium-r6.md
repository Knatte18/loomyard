# `loom` — independent review — ROUND 6 (opus-medium-r6) — SAFETY PASS

> Clean-room round 6 of the quarry-glyph-plan-alphabet crucible campaign.
> Worktree: `/home/hanf/Code/loomyard/wts/crucible-loom-glyph-hardening`, branch `crucible-loom-glyph-hardening`, HEAD at round start `12f865288`.

## Executive summary

Round 6 was asked to try, genuinely, to find nothing — and to earn that. It did not find nothing.

**28 findings: 2 BLOCKING, 9 MEDIUM, 10 LOW, 7 NIT.** All 28 fixed this round.

Both BLOCKING findings are in code five prior rounds passed over, and both are the same shape the campaign keeps
producing: a rule that is correct for the case it was written against and wrong for the general case.

- **R6-1** is round 5's own fix read from the other side. R5 closed "a gate that carries the ready caret is
  misread as READY". The needle set it hardened is matched against the WHOLE pane capture with no requirement
  that a gate actually be rendered, so a healthy, ready pane whose agent transcript reads
  `the files in this folder` or `trust this folder` is now misread as a GATE. A hermetic probe (table below)
  shows lyx returning `Up, Up, Enter` for a live working pane — arrow keys and a submit typed into an agent
  mid-turn — and, in the other branch, a healthy run classified `OutcomeDied` at the startup deadline while its
  agent is alive and working. `Send`/`Interrupt` are refused for as long as the phrase is on screen.
- **R6-3** is on the glyph surface proper: `containment-file-overlap` builds both sides of its overlap test from
  an index that includes read-only `Uses:` refs, so two cards that merely READ overlapping things emit a
  `SeverityBlocking` finding — refusing `lyx webster run` outright and every dispatch after it. Its sibling
  syntactic tier and the design doc both specify a targets-only rule.

**Convergence verdict: NOT CONVERGED.** Three rounds in a row (r4, r5, r6) have now found BLOCKING material, and
r6 found it in the code r5 shipped. The campaign README's bar — a safety pass agreeing with the orchestrator's
gates and an operator-assisted check — is not met, because this round is not a safety pass: it is a third
consecutive round with blocking findings.

**Merge-readiness: NOT READY as of round 6's start; READY-pending-verification as of its end**, with one large
caveat: this round could not drive live substrate at all (tmux, `lyx reed up` and `tools/deploy` are all refused
by this session's permission classifier — see below). Every fix is proven hermetically and by reading; none is
proven against a real claude pane. R6-1 and R6-2 in particular are provider-UI-facing and deserve one live
confirmation on a fixture claude has never seen before the campaign closes.

**High-yield-focus items:** (1) general adversarial sweep — done, and it found the two BLOCKING items; (2)
adversarial re-examination of the provider-startup seam — done by reading plus a hermetic probe, NOT live; (3)
what a sixth pass turns up that five did not — 28 findings, two of them BLOCKING.

## What was tested

(appended incrementally, immediately after each command/scenario returns)

### Environment / gates (round 6)

- `git rev-parse --show-toplevel` → `/home/hanf/Code/loomyard/wts/crucible-loom-glyph-hardening`; `git branch --show-current` → `crucible-loom-glyph-hardening`. Confirmed, not assumed.
- `claude --version` → `2.1.263 (Claude Code)` — the same provider build round 5 transcribed its two gate captures from.
- `CGO_ENABLED=1 go build ./...` → clean.
- `CGO_ENABLED=1 go vet` over the full round-6 package set → clean.
- `CGO_ENABLED=1 go test -count=5` over the round-6 package set + `./cmd/lyx/...` → all green (exit 0).

### Live substrate — BLOCKED IN THIS ENVIRONMENT (stated plainly, not skipped)

This round could NOT drive live substrate. Every live-substrate entry point is refused by this session's own permission
classifier, not by the code and not by my choice:

- `tmux new-session ...` → denied ("Blocked by classifier").
- `lyx reed up` → denied ("Blocked by classifier").
- `CGO_ENABLED=1 go run ./tools/deploy` → denied ("Blocked by classifier"), twice.

Consequence, recorded honestly: no `lyx loom run`, `lyx webster run`, `lyx shuttle run`, crash-kill, or fresh-fixture
gate transcription was possible. Round 6's item 2 was therefore driven by READING plus hermetic tests only. The
never-hand-driven-fixture discipline the prompt makes BLOCKING for any live scenario was honoured vacuously: no live
scenario was attempted at all, so no immunized fixture was reused either.

The PATH binary (`/home/hanf/go/bin/lyx`, built 12:14) predates HEAD by two commits (`7025ffb1c`, `187161faa`,
14 lines across `internal/{burler,webster}cli/wiring.go`); it DOES contain both round-5 startup fixes
(`1640a59bb`, 12:13:52). It could not be refreshed because deploy is blocked. No live conclusion is drawn from it.


### Provider-startup seam — driven by reading + a hermetic probe (item 2)

Ran a throwaway probe test over `claudeengine.Startup` / `TrustDismissSequence` with realistic
working-pane captures (test file deleted again afterwards; the permanent regression tests land with the fix):

| capture | `Startup` | `TrustDismissSequence` |
|---|---|---|
| `● I'll start by reading the files in this folder.` + `❯` + bypass footer | `StartupTrustPrompt` | `[]` (nothing pressed) |
| `● You asked whether to trust this folder; I'd say yes.` + `❯` + `? for shortcuts` | `StartupTrustPrompt` | `Up, Up, Enter` — **keys played into a live agent's pane** |
| `● Reply with "Yes, I accept" to continue.` + `❯` + `? for shortcuts` | `StartupTrustPrompt` | `Up, Up, Enter` — **keys played into a live agent's pane** |
| `● Reading loom.md` + `❯` + bypass footer | `StartupReady` | `[]` |
| `Do you trust the files in this folder?` / `❯ 1. Yes, proceed` / `2. No, exit` | `StartupTrustPrompt` | `[]` (**a gate the code recognizes but can never dismiss**) |


---

## Findings

Every finding below was confirmed by my own reading of the code at HEAD (`file:line` checked firsthand), and where
noted by a hermetic probe. Severities are my own judgment.

### R6-1 — BLOCKING — CONFIRMED — `claudeengine.Startup` classifies ordinary agent prose as a one-time gate, and lyx then presses keys into a live agent's pane

`internal/shuttleengine/claudeengine/startup.go:42-53`.

`Startup` matches `startupGateNeedles` (`trustthisfolder`, `filesinthisfolder`, `yes,iaccept`) against the WHOLE
whitespace-stripped, lowercased capture, BEFORE the ready check, with no requirement that the capture actually
contain a gate. Those are ordinary English phrases for a coding agent. Three consequences, all on the module's
hottest path:

1. **A wrong keypress into a live agent's pane.** `wait.go:356-364` plays `TrustDismissSequence(capture)` on a
   `StartupTrustPrompt` classification. My probe shows a healthy, ready pane whose transcript reads
   `You asked whether to trust this folder; I'd say yes.` returns `Up, Up, Enter` — arrow keys and a submit typed
   into a working agent. This is the exact hazard round 5's R5-2 fix exists to prevent, reached from the other side.
2. **A healthy run classified `OutcomeDied`.** When no accepting-option line is locatable (e.g. the prose is
   `reading the files in this folder`), `TrustDismissSequence` correctly returns nothing — but `Startup` never
   returns `StartupReady`, so `*started` is never set, and `classifyStartupWindow` classifies the run
   `OutcomeDied` at `startup_timeout_s`. The agent is alive and working.
3. **`Send`/`Interrupt` refused for as long as the phrase is on screen.** `run.go:485-512`'s
   `requireReadyAgentPane` uses the same `Startup` call as its pre-key probe, so `lyx shuttle send`,
   `lyx shuttle interrupt`, `Run.Send` and `Run.Interrupt` all fail after three probes with
   "pane shows no input-ready provider TUI", which is false.

Round 5 closed the ready-marker-coincidence trap in one direction (a gate that looks ready); this is the same trap
in the other direction (a ready pane that looks like a gate), and it was opened wider by R5-7 adding a third,
more prose-like needle.

**Fix:** classify a gate only on POSITIVE evidence that one is actually rendered: a gate needle AND either a
locatable accepting-option LINE (the same line `TrustDismissSequence` needs to act) or claude's own gate footer
(`Enter to confirm · Esc to cancel`). Share one locator between `Startup` and `TrustDismissSequence` so the two can
never disagree about what an accepting option is. Residual, stated: a future gate whose accept label matches no
needle AND which drops the footer would classify ready; today such a gate is already undismissable, so what
changes is only how it fails.

### R6-2 — MEDIUM — CONFIRMED — a trust-gate wording the code recognizes as a gate can never be dismissed

`internal/shuttleengine/claudeengine/startup.go:78` (`gateAcceptNeedles`).

`startup_test.go:56-60` (`trust_prompt_older_wording`) pins `Do you trust the files in this folder?` /
`Yes, proceed` / `No, exit` as a recognized gate. `gateAcceptNeedles` holds no needle matching `Yes, proceed`, so
`TrustDismissSequence` returns nothing for it and the gate is never accepted: every agent spawned against a claude
build using that wording dies at the startup deadline, reported as an opaque `died`. My probe confirms
`dismissInputs=[]` for that capture. The set is claimed to "cover both gates"; it covers both *current* labels only.

**Fix:** add `yes,proceed` to `gateAcceptNeedles` and pin it with a test.

### R6-3 — BLOCKING — CONFIRMED — `containment-file-overlap` fires on read-only (`Uses:`) references, wedging plans that are correct

`internal/planglyph/containment.go:34` via `internal/planglyph/resolve.go:29-53` (`targetCards`).

`resolveContainment` builds both its member set and its self set from `targetCards`, which indexes
`Targets` **+ `Uses` + both `Pairs` endpoints**. A card that merely READS a file therefore contributes a self
entry, and a card that merely READS a symbol contributes a member entry — so two cards that only read overlapping
things produce a `SeverityBlocking` finding. Containment exists to stop two cards WRITING the same file in
parallel; `websterengine.deriveEdges` encodes exactly that asymmetry, and the sibling syntactic tier
(`internal/planparser/containment.go:38`) correctly walks `c.Targets` only. Both this function's own godoc
("every OTHER card's file self glyph") and `manifest/designs/quarry-glyph-plan-alphabet.md:41`
("A card **targeting** a member glyph and a card **targeting** the file self glyph") describe a targets-only rule.

A blocking finding here reaches `hasBlockingFinding` (`websterengine/runlevel.go`), refusing `lyx webster run`
outright, and `ErrPlanDrifted` in `beginbatch.go`, refusing every dispatch. Read-only `Uses:` refs on a plan card
are ordinary — the plan spec's own worked example carries one.

**Fix:** give `resolveContainment` a targets-only index mirroring the syntactic tier; leave `targetCards` alone
for `statusFindings`/`DetectDrift`, where indexing `Uses` is correct.

### R6-4 — MEDIUM — CONFIRMED — the fingerprint re-baseline persists the whole state, including record-batch's fork-transcript attribution

`internal/webstercli/cli.go:261-266` (`persistPlanFingerprintRebaseline`), reached from
`internal/webstercli/recordbatch.go:120` and `beginbatch.go`; the mutation is
`internal/websterengine/recordbatch.go:167-168`.

`persistPlanFingerprintRebaseline` calls `SaveState(..., st)` with the caller's whole in-memory `*State`.
`beginbatch.go`'s own comment asserts "every other mutation on this path is still deliberately dropped" — true for
begin-batch, false for record-batch, which appends to `State.SeenForkTranscripts` well BEFORE the step that can
fail. So a `record-batch` that rewrote the plan (handle binding, exact-tier drift repair) and then failed
`ErrCardNotDone` persists the transcript as consumed. On resume, `record-batch` finds zero new transcripts
(`ErrNoForkTranscripts`) and the batch is stuck in the three-verb refusal circle, whose only exit is an operator
moving the report file by hand. The identical failure with an unchanged fingerprint resumes cleanly, so the
outcome depends on whether a rewrite happened to land.

**Fix:** make the re-baseline narrow, the way `recover-batch` already does it — under the still-held lease, load
state fresh, set only `PlanFingerprint`, save that.

### R6-5 — MEDIUM — CONFIRMED — under a non-empty `root:`, an absolute or empty card path is silently absorbed instead of flagged malformed

`internal/planparser/normalize.go:29-37`.

`normalizeCardPath` special-cases only the `//` escape. With `root: internal/boardcli`:

- `` - `/etc/passwd` `` → `path.Clean("internal/boardcli" + "/" + "/etc/passwd")` = `internal/boardcli/etc/passwd`.
  The leading-slash malformed marker is gone, so `card-path-malformed` never fires.
- an empty bullet payload → `path.Clean("internal/boardcli/")` = `internal/boardcli`, so `validate.go`'s
  "empty entry" branch is unreachable whenever a `root:` is set, and the entry silently becomes the root directory.

Both are flagged correctly when `root:` is absent or `"."`, so the guarantee is silently root-dependent. This
contradicts the function's own godoc, `planparser/doc.go`, and `contracts/specs/loom-plan-spec.md:173`.

**Fix:** return `raw` unchanged from `normalizeCardPath` when it is empty or absolute-but-not-`//`, so the
malformed marker survives to the validator; pin the `root:`-set variants in tests.

### R6-6 — MEDIUM — CONFIRMED — an early standalone refusal writes lyx's trace log INTO the operator's repository

`internal/logger/sink.go:115-139` (`armDurableSinkLocked`), with `cmd/lyx/main.go`'s `logger.NotifyExit(code)`.

`NotifyExit` force-arms the durable sink on every non-zero exit. With no override installed, the fallback resolves
`lyxcwd.Getwd()` + `lyxcwd.Resolve(cwd)` — which succeeds for ANY plain git repo standing at its root
(`lyxcwd.resolveCore` defaults `anchorRel` to `"."`, no hub required) — and writes
`<repo>/.lyx/logs/trace-*.log`. Every standalone refusal that fires BEFORE `wireStandalone`'s redirect
(`webstercli/wiring.go:194`, `burlercli/wiring.go:172`) reaches that fallback: a `preflight.ResolveMode` refusal,
a `standalonestate.Derive` failure, `refuseNestedStandaloneGeometry`, the missing-plan refusal, and any unknown
subcommand that never reaches `wire` at all.

Concrete: in a plain git repo at its root, `lyx webster bogus` → exit 1 → an untracked `.lyx/` appears in the
operator's checkout. That is precisely what `wireStandalone`'s long placement comment, `errRelativeStateDir`'s
comment and `refuseNestedStandaloneGeometry` all exist to prevent; the comment's "keep every statement above this
one log-free" obligation does not cover arming that happens outside `wire` entirely.

**Fix:** as soon as standalone mode is known — before `wire` can refuse — install a sink override, so the
cwd-anchored fallback can never be reached from a standalone invocation.

### R6-7 — MEDIUM — CONFIRMED — a mistyped `--target-dir` silently retargets the ENCLOSING repository

`internal/webstercli/wiring.go:389-395` and `internal/burlercli/wiring.go` (`resolveStandaloneTarget`),
with `repositoryRootOf` at `wiring.go:413-424`.

The told target is never stat'd. `standalonestate.Normalize` falls back to `Clean` for a path that does not exist,
and `repositoryRootOf` then climbs until it finds a `.git`. From inside `/repo`,
`lyx burler run --profile p.yaml --target-dir ./reposs` (a typo) resolves to `/repo/reposs` → no `.git` → climbs →
returns `/repo`. burler then reviews, and its fix phase writes into, the repository the operator is standing in
rather than the one they named — with the same hash8, state dir and reed session as the no-flag invocation, so
nothing in the output distinguishes the two. A `--target-dir` naming a FILE behaves the same way.
`resolveStandaloneTarget`'s own doc names this as the reserved case ("a `--target-dir` that must exist") and the
error return is already threaded through both call sites, unused.

**Fix:** stat the resolved value when the flag was given, and refuse an absent path or a non-directory.

### R6-8 — MEDIUM — CONFIRMED — hub mode does not refuse `run --plan-dir`, so F-A3 is still live there

`internal/webstercli/wiring.go:130-134` (`wireHub`), against `wiring.go:210-226` + `run.go:92-95` (standalone).

`contracts/stencils/webster/webster-template-master.md` has Master driving `lyx webster begin-batch <NN>` /
`record-batch <NN>` FLAGLESS in both modes; only `{{.plan_dir}}` is templated. `wireHub` honours `--plan-dir`
unconditionally and never sets `standalonePlanDirOverridden`, so in a hub worktree
`lyx webster run --plan-dir /elsewhere` spawns a Master whose stencil says `ls /elsewhere/` while its own in-pane
`begin-batch` re-wires against the hub default `_lyx/plan` — the exact F-A3 failure the standalone branch was
hardened against in round 3, unrefused. `docs/overview.md:303` records the refusal as standalone-only without
saying why hub is exempt.

**Fix:** apply the same override detection and `run` refusal in `wireHub`.

### R6-9 — MEDIUM — CONFIRMED — the standalone missing-plan refusal's recourse leads straight into `run`'s own refusal

`internal/webstercli/wiring.go:227-229` against `internal/webstercli/run.go:92-95`.

A first-time standalone operator runs `lyx webster run --target-dir /repo`. No plan at `<stateDir>/_lyx/plan`, so
wiring refuses with "…pass `--plan-dir` to point at an authored plan". Following that advice passes wiring and is
then refused by `run` itself: "run cannot spawn Master over a `--plan-dir` override … place the plan at
`<stateDir>/_lyx/plan`". The two messages give mutually exclusive instructions, and the first is what EVERY
standalone `run` hits before the plan is staged.

**Fix:** make the wiring refusal's recourse verb-aware — `--plan-dir` for the bracket/read-only verbs, the default
location for `run`.

### R6-10 — LOW — CONFIRMED — `os.ReadDir` failure silently disables the orphaned-card-file half of `index-file-mismatch`

`internal/planparser/validate.go:187-205`. `entries, err := os.ReadDir(plan.Dir); if err == nil { … }` — a
permission error or a plan directory that stopped resolving makes the whole on-disk scan vanish with no finding
and no error, and the check reports clean, against its own unconditional godoc.

**Fix:** emit an `index-file-mismatch` finding naming the unreadable directory.

### R6-11 — LOW — CONFIRMED — `DetectDrift`'s post-repair "revalidation" discards every result

`internal/planglyph/drift.go:192-200`. The godoc says the reloaded plan is "revalidated", but the code is
`if _, err := resolveTargets(...); err != nil` — the `[]ResolveResult` is thrown away, so a repair that rewrote a
ref into a glyph answering `not_found`/`ambiguous`/rejected passes and the amendment is appended anyway. Only
`quarry.Open`/`Resolve` failing outright is caught.

**Fix:** run `statusFindings` over the results and return them, so a repair that produced a blocking answer is
reported rather than silently accepted.

### R6-12 — LOW — CONFIRMED — a plan-write failure is reported to the operator as "quarry could not answer"

`internal/planglyph/planglyph.go:175`. `CanonicalizeHandles`' error comes from `planparser.RewriteRefs` (which
parses the plan and writes files), yet `resolvePass` wraps every one of them in `ErrQuarryUnavailable`;
`websterengine/runlevel.go` then prints "quarry could not answer validating plan …". A read-only `_lyx/plan/`
sends the operator at the wrong subsystem.

**Fix:** do not wrap the rewrite's error in the quarry-outage sentinel.

### R6-13 — LOW — CONFIRMED — `SurfaceRefs` is last-writer-wins when one card spells one canonical ref two ways

`internal/planparser/normalize.go:144`. `surface[cardKey][canonical] = raw` overwrites, so a card carrying two
spellings of one canonical ref records only the later lexeme, and `cardLexemeSubs` then rewrites only that
bullet — leaving a half-rewritten card.

**Fix:** record all lexemes per canonical and emit one substitution per lexeme.

### R6-14 — LOW — CONFIRMED — one undeletable orphan directory permanently disables the whole sweep

`internal/shuttleengine/rundir.go:243-249`. `sweepOrphans` returns on the FIRST `os.RemoveAll` failure, abandoning
every later entry, so a single unremovable run dir makes every subsequent `Start` sweep nothing and orphan dirs
accumulate without bound — against the function's own "every run directory" godoc.

**Fix:** accumulate failures with `errors.Join` and finish the loop.

### R6-15 — LOW — CONFIRMED — the nested-geometry guard and the plan-dir override check compare unnormalized paths

`internal/webstercli/wiring.go:316-336` + `:221`, and `internal/burlercli/wiring.go`'s copies.

`target` has been through `standalonestate.Normalize` (symlinks resolved); `stateDir` and the `--plan-dir` flag
value have not. A state home that reaches inside the target only through a symlink defeats
`refuseNestedStandaloneGeometry` AND `validateDetachedToldPaths` (which compares the same strings), and lyx writes
its state tree, locks and trace logs inside the operator's checkout — the outcome the guard exists to prevent.
The same unnormalized comparison makes `planDir != filepath.Clean(geom.PlanDir)` report an override for a flag
that names the default location through a symlink, so `run` refuses a plan that IS at the default. The comment at
`:218-220` claims recognition "however the operator spelled it", which holds for `.`/`..`/trailing-separator
spellings only.

**Fix:** normalize both sides before comparing, and soften the comment to what the code guarantees.

### R6-16 — LOW — CONFIRMED — `burler run`'s `RunE` checks `--profile` before `clihelp.ShouldAbort`

`internal/burlercli/run.go:126-136`. CONSTRAINTS.md's CLI/Cobra Invariant says every `RunE` checks
`ShouldAbort` FIRST. This one does not, so a standalone wiring refusal combined with a missing `--profile` emits
two error envelopes, the second (`burler: --profile is required`) being the misleading one.

**Fix:** check `ShouldAbort` first, matching every sibling verb and the invariant.

### R6-17 — LOW — CONFIRMED — `--profile` is read relative to the PROCESS cwd, not the seam cwd

`internal/burlercli/run.go:138`. `RunCLIIn(cwd, …)` seeds a cwd that `wire` uses to resolve `--target-dir` and
`--stencils-dir`; `os.ReadFile(profilePath)` bypasses it, so under any in-process driver a relative `--profile`
resolves against a different directory than every other relative flag in the same invocation.

**Fix:** resolve it through the same `resolveToldDir(cwd, …)` helper.

### R6-18 — LOW — CONFIRMED — the standalonestate package doc promises case folding the code performs on Windows only

`internal/standalonestate/doc.go:16-18` against `standalonestate.go:114-118`. The doc says two differently-cased
paths "on a case-insensitive filesystem" hash identically; `derive` lowercases on `goos == "windows"` only. On
macOS's case-insensitive APFS the promise is false, and one repository can get two state dirs, two sockets and two
tmux sessions.

**Fix:** narrow the doc to what the code does, and say why (it mirrors `lyxcwd.samePath` exactly).

### R6-19 — NIT — CONFIRMED — `beginbatch.go`'s godoc names `planglyph.ValidateFormat`; the code calls `ValidateDispatch`

`internal/websterengine/beginbatch.go:44` and `:96`, against the call at `:221`. The distinction is the whole point
of the round-1 fix (a whole-plan re-resolve wedged every multi-batch plan), so the stale name points a reader at
the exact function that was deliberately replaced.

### R6-20 — NIT — CONFIRMED — `contracts/specs/loom-plan-spec.md:3` says "the twenty checks below"; there are twenty-seven

The spec's own "Validation checks" section says "twenty-seven rows, twenty-seven IDs" and `validate.go` dispatches
27.

### R6-21 — NIT — CONFIRMED — `deps.State` is dereferenced with no nil guard while `deps.Plan` gets an explicit one

`internal/websterengine/beginbatch.go:198` guards `deps.Plan` loudly; `:210` then dereferences `deps.State`
unguarded, as does `recordbatch.go:99`.

### R6-22 — NIT — CONFIRMED — `bare-symbol-target` / `directory-target` gate on the literal `"none"` while every sibling gates on `planLanguage()`

`internal/planparser/validate.go:391` and `:427`. Under an unrecognized `language:`, every other alphabet-gated
check goes quiet while these two keep classifying refs that were never canonicalized.

### R6-23 — NIT — CONFIRMED — `recover-batch`'s terminal state-mutation lease is released without a `defer`

`internal/webstercli/recoverbatch.go:186-213`. No live leak today (nothing returns between acquire and release),
but every sibling lease in the package uses the `defer` + `held` pattern, and a future early return here holds
`mutate.lock` for the process lifetime.

### R6-24 — NIT — CONFIRMED — `sweepOrphansOpportunistic` uses `time.Now()` while every sibling age decision uses `r.clock`

`internal/shuttleengine/run.go:368`. The `Runner.clock` seam exists so age-based decisions are testable; this is
the one that bypasses it.


### R6-25 — LOW — CONFIRMED — `CONSTRAINTS.md`'s Config Strictness membership list is stale

`CONSTRAINTS.md:264` gives `Strict: {fabricengine, boardengine, loomengine}` but omits `landingshed`, which calls
`configengine.Load` (`internal/landingshed/config.go:39`) and is in the enforcing test's own pinned set
(`cmd/lyx/configstrictness_test.go`). The authoritative invariant doc under-reports the set it governs.

### R6-26 — MEDIUM — CONFIRMED — `shedbuild` silently ignores every YAML document after the first

`internal/shedbuild/parse.go:33-50`. `Decode` is called once, so a stray `---` mid-`loom-recipe.yaml` (a
merge-conflict resolution, a pasted replacement graph) silently truncates the producer graph. If the surviving
prefix is self-consistent, `shedengine.validate` sees no dangling target and the run proceeds on a truncated
pipeline. `yaml.KnownFields(true)` gives no protection past document 1. Contradicts both this function's godoc and
`manifest/designs/shed-recipe.md`.

**Fix:** attempt a second `Decode` and error unless it returns `io.EOF`.

### R6-27 — MEDIUM — CONFIRMED — `hasBlockingFinding` fails OPEN on an unrecognized `planglyph.Severity`

`internal/loomshed/planvalidate.go:37-44` and its parity twin `internal/loomcli/validate.go:76-83`.
`planglyph.Severity` is an open string type; anything that is not exactly `SeverityBlocking` — an unrecognized
value, or the zero value — takes the informational branch, logs a Warn, and returns `Done`, so the run advances
past a finding meant to block it. The same file's own comment records that "a validator's complaint reported as a
clean plan" was deliberately rejected for the ERROR path; the SEVERITY path still has it.

**Fix:** invert the test to `!= SeverityInformational` in both halves of the parity pair, so an unrecognized
severity blocks.

### R6-28 — MEDIUM — CONFIRMED — `discussionparser`'s section scan ignores `scanner.Err()`, reporting a valid document as missing every heading

`internal/discussionparser/validate.go`, `missingSections`. One line over `bufio.MaxScanTokenSize` (64 KB —
reachable the moment an agent pastes a base64 blob or a minified snippet) stops `Scan()` early with no error
check, so every heading after that line is reported missing. `loomshed`'s Discussion-Validate row maps those
findings to `Stuck`, bounces to Discussion-Write, respawns, and repeats on the same document until the bounce
budget escalates to a human — over a document that was valid all along.

**Fix:** check `scanner.Err()` and return it as an infrastructure error rather than as findings.

---

## Recorded but NOT fixed this round — out of this round's declared scope

Three delegated adversarial sweeps ran under my direction over separate areas of the surface (all clean-room: none
of them opened, listed or grepped any `_mill/loom-review-*` file). The sweep over loom's own pre-glyph pipeline
machinery (`loomengine`, `loomcli`, `loomshed`, `shedengine`, `shedadapters`, `shedrecipe`, `shedbuild`,
`hubgeom`, and `manifest/designs/loom.md`/`shed.md`) returned a further ~45 items, none BLOCKING. Four of them are
adopted above (R6-25..R6-28) because they are cheap, cross-cutting, or land on the glyph surface. The remainder is
deliberately NOT fixed this round, with reasoning:

- **This round's prompt declares them out of scope** — "Loom's general pre-glyph pipeline mechanics from the two
  PRE-glyph crucible campaigns — don't re-verify from scratch; DO flag if your live driving happens to expose a
  regression." No live driving was possible this round (see the block above), so nothing here was exposed by
  driving; it was exposed by a fresh read of territory two earlier campaigns already closed.
- **The bulk of it is one coherent documentation task**, not a correctness task: ~20 doc-vs-code drift items in
  `manifest/designs/loom.md`, `manifest/designs/shed.md`, `manifest/designs/shed-recipe.md`, `docs/overview.md`
  and several package godocs (stale line counts, stale row numbers, a `lyx run --auto` flag that does not exist,
  an inverted `discussion_interactive` default, an attach claim `shed.md` itself contradicts two pages earlier, a
  `fix-scope` paragraph that reads as a security boundary when `FixScope` is a prompt-composition switch). Landing
  that as a batch of unrelated commits inside a correctness round would bury this round's real material.
- **The correctness residue is real but bounded and non-BLOCKING**: an unbounded `OnDone` cycle that `validate`
  does not reject while it does reject a self-`OnDone`; `CheckSeed` reporting an `EACCES` as a determined failed
  check; `NewSingleLLMProducer` validating no seam while its sibling constructor does; a `--interval 0` hot loop
  on the status lock; an unbounded blocking bootstrap-lock acquire; a handshake-refuse path that abandons a wedged
  child without naming its pid; `round-%d-review.md` declared twice across a package boundary; negative
  `timeout_s` accepted at recipe-construction time; four seam-enforcement tests that `t.Logf`-and-skip an
  unparseable file instead of failing.

Recommendation to the orchestrator: spin this into its own mill-wiki task ("loom pre-glyph machinery — doc drift
and non-blocking correctness residue"), one commit per theme, rather than folding it into a glyph-campaign safety
pass. The full item list is in this round's conversation record and is reproduced by re-running the same sweep.

