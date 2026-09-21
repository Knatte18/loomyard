# batten review — sonnet-xhigh-r5

Round 5. Independent, clean-room review + fix of the `batten` module, per
`_mill/batten-review-prompt.md`. This file is built incrementally per that
prompt's "Log as you go" rule: observations and provisional findings are
appended as each scenario/command returns, before the final executive
summary and severity ordering are written.

Clean-room note: this review's own findings list (below) was written before
reading any prior round's `_mill/batten-review-*` material. Prior-round
context was read only afterward, to re-confirm previously-fixed behavior and
re-evaluate the two deferred items — never to shape the findings list itself.

## What was tested

### Static / code reading (Job 1, phase 1)

- Read `internal/battenshed/*.go` (create.go, seamchild.go, innerrun.go,
  teardown.go, ctx.go, deps.go, stuck.go, doc.go) and every corresponding
  `_test.go`, plus `seam_enforcement_test.go`.
- Read `internal/battenrecipe/*.go` (battenrecipe.go, names.go, doc.go) and
  its tests (recipe_test.go, coverage_guard_test.go, seam_enforcement_test.go;
  skimmed fixture_test.go).
- Read `internal/battencli/*.go` in full: arm.go, bootstrapverb.go,
  cli.go, commitstatus.go, paths.go, refusal.go, wire.go, and
  `lifecycle_integration_test.go` in full. Skimmed the remaining `_test.go`
  files' test-name inventory (arm_seed_test.go, cli_test.go,
  commitstatus_test.go, paths_test.go, refusal_test.go, run_test.go,
  step_test.go, wire_test.go) to confirm coverage shape; read
  `TestChildSpawnError` and the write-seed-failure tests in full.
- Read `contracts/recipes/batten-recipe.yaml`.
- Recovered and read the deleted design doc:
  `git show 8ac857ce1~1:manifest/designs/seeded-shed.md` in full.
- Read `docs/overview.md`'s batten entry (lines 369-380) and nested-Shed
  paragraph (lines 412-413).
- Read `CONSTRAINTS.md` in full (390 lines) — all invariants, not just the
  batten-named ones.
- Read `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s F22 section in full
  (lines 529-551).
- Diffed every non-batten file this campaign has touched since its branch
  point (`git diff --stat 29d9e6a42^..HEAD`, then full diffs on the
  interesting ones): `CONSTRAINTS.md`, `internal/fabricengine/fabric.go`
  + `doc.go` (RequireDrivableWorktree), `internal/loomcli/bootstrap_test.go`
  (driverFieldReadCarveOuts), `internal/boardcli/cli.go`,
  `internal/shedrecipe/entries_batten.go` (+test),
  `internal/reedengine/{spawn.go,doc.go,panebin.go,...}`,
  `internal/shell/{shell.go,posix.go,pwsh.go}` (the #017 Pane Binary
  Resolution merge). Grepped the whole campaign diff for stray corrupted
  unicode (curly quotes) as a cheap blast-radius smoke check.
- Grepped every production call site of `fabricengine.RequireWarpWorktree`
  and confirmed the only caller outside `fabricengine` itself is
  `internal/fabriccli` (an owner-set package per the Fabric Vocabulary
  Invariant) — `internal/battencli` uses the vocabulary-neutral
  `RequireDrivableWorktree` wrapper exclusively. No second non-owner
  bare-caller found (re-confirms F6[R4]'s fix is still complete).
- Delegated a fact-gathering-only (no-judgment) sub-task to a fork for
  boundary code I needed precise citations on rather than my own review
  judgment: `internal/shedengine`'s Stuck-vs-hard-error status handling,
  `plugins/ly/skills/ly-drive/SKILL.md`'s step loop, and — most
  importantly — `internal/loomcli/start.go`'s exact `--no-attach` blocking
  contract for both `driver: go` and `driver: llm`. Findings folded in
  below (see "Focus item 4" and "Boundary facts").

### Hermetic gate (baseline, before any fix)

- `go build ./...` — clean, no output.
- `go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...` — clean.
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...` — all `ok`.
- `go vet ./...` — clean (`VET_EXIT=0`).
- `go test -count=1 ./...` — all `ok`, zero FAIL/panic across every package in the repo (94-line summary, one `ok` line per package).

### Operational note: pre-existing scratchpad content

This session's scratchpad directory
(`.../c7ce4f6e-42d7-4add-825c-c14fded/scratchpad`) already contained
leftover shell scripts and logs from what its own `env.sh` comment names
"the batten r4 live-driving scenarios" (fixture-hub build/drive/teardown
scripts, `s-*.sh` scenario drivers, `success-run*.log`). This is not
`_mill/batten-review-*` material (the clean-room bar), so reading script
*names* to confirm disposal is not a clean-room violation, but I did not
read the narrative log content, to avoid priming my own findings. Verified
directly: the fixture hub directory these scripts built
(`scratchpad/hub`) no longer exists on disk (disposed), no stray `lyx`
process, no stray tmux socket, and the operator's real standing bench
(`~/Code/lyx-test-HUB`) is present and untouched. This round builds its own
fresh, separately-named fixture hub rather than reusing or trusting that
leftover state, and its own live-driving deliverable is independent of it.

(Live-driving scenarios continue below as they are run.)

## Findings (provisional — filled in during Job 1, ranked at the end)

Recorded as spotted; severity/CONFIRMED-vs-PLAUSIBLE finalized once live
verification is complete.

### F1 (NIT, CONFIRMED) — corrupted doc comment in internal/shell/posix.go:13

`internal/shell/posix.go:13`:
```go
// Quote wraps s in POSIX single quotes, escaping embedded quotes via '\” idiom.
```
The closing of the escape idiom is a stray curly close-quote character (`”`,
U+201D) where the source clearly intends the POSIX `'\''` idiom (visible in
the function body one line below, and in the identical comment's own
pre-edit wording: `git log -p` shows the prior text read `... via '\'' idiom.`
verbatim). Traced to commit `bf0b20038` ("Spawned agent panes resolve the
spawning lyx binary"), which reached this branch via the F7[R2]/#017 merge
(`949dffef6`) this campaign's own round-context note references. Confirmed
by `git diff 29d9e6a42^..HEAD | grep -P '[""'']'` — this is the only
instance of stray curly-quote corruption anywhere in the campaign's own
diff. Outside batten's three packages, but squarely inside this round's own
"blast-radius sweep on this campaign's own cross-cutting fixes" focus point,
since the file arrived via a merge this campaign's own round history
records and no round has yet read it byte-for-byte. Fix: restore the
correct `'\''` text.

### F2 (LOW, PLAUSIBLE pending live/unit confirmation) — child-output truncation in wire.go's childSpawnError can split a multi-byte UTF-8 rune

`internal/battencli/wire.go:110-112` (`childSpawnError`):
```go
if len(trimmed) > maxChildOutputInError {
    trimmed = trimmed[:maxChildOutputInError] + " ... (truncated)"
}
```
`trimmed[:maxChildOutputInError]` is a byte-index slice of a Go string. If
the child's real stdout/stderr (a real `lyx loom start` bootstrap failure,
which can legitimately contain non-ASCII — a task title, a file path, a
Claude response fragment folded into an error) puts a multi-byte UTF-8
rune's bytes across that boundary, the truncated string ends with an
invalid partial UTF-8 sequence. `fmt.Errorf("%w: %s", ...)` does not
validate UTF-8, so the invalid bytes propagate into the returned error's
`.Error()` text; JSON-encoding it later (`internal/output`) silently
replaces the broken tail with U+FFFD replacement characters rather than
erroring, so the practical blast radius is a slightly garbled last few
characters of a truncated diagnostic, not a crash or data loss elsewhere.
`TestChildSpawnError` (wire_test.go) exercises truncation only with a pure
ASCII `strings.Repeat("x", ...)` fixture, so this path is untested against
non-ASCII input. Fix: truncate on a rune boundary (e.g.
`strings.ToValidUTF8` after a byte slice, or walk back to the last
complete rune via `utf8.DecodeLastRuneInString`).

### F3 (MEDIUM, CONFIRMED live) — a disagreeing child seed at Seed-Child hard-errors instead of routing to Stuck

`internal/battenshed/seamchild.go`'s `childRecipeRefusal` only recognizes
`ErrUnknownRecipe`/`ErrUnsupportedChildRecipe` as Stuck-worthy business
refusals; every other `WriteSeed` error is treated as "a path-resolution or
write failure" and returned as a hard error (per both the package doc
comment on `SeedChildDeps.WriteSeed`, deps.go:92-97, and the matching
comment in seamchild.go:54-58). But `internal/battencli/wire.go`'s
`WriteSeed` closure (wire.go:309-329) calls `shedrun.WriteSeed` directly
after its own two recipe checks, and `shedrun.WriteSeed`
(`internal/shedrun/seed.go:172-175`) has a THIRD failure mode neither
sentinel wraps: an existing child seed that disagrees on recipe, driver, or
params returns a plain `fmt.Errorf("... refusing to overwrite with
disagreeing seed ...")`. That is exactly the same *kind* of business
judgment a human can act on (fix or delete the child's hand-seed, or
correct the Board type) as the two recognized refusals — not a mechanism
failure — yet it currently surfaces as a run-halting hard error (shedengine
`StateFailed`) rather than a `Stuck` verdict with a named remedy.
Reachability: normally Seed-Child runs once and returns Done, so this path
is unreachable in the ordinary flow; it is reachable if the child's own
`self` seed is hand-written or otherwise pre-exists with different values
before Seed-Child (re)commits its own — exactly the sabotage/hand-seed
shape high-yield items 8/9/11 already ask this round to drive.

**Live-confirmed** (see "Scenario: F3" above): a genuine pre-existing,
committed, disagreeing child seed makes `lyx batten step`/`run` return a
**hard error** (`kind: "producer"`, persisted `state: "failed"`, no
`stuck_reason` key) rather than `Stuck`/`blocked` with a named remedy —
live-contrasted side by side against Board `type: "batten"`
(`ErrUnsupportedChildRecipe`), which correctly halts `blocked` with a
`stuck_reason` naming the Board type and the fix. Both cases are equally
resumable (the same `battenPreRun`/`battenPreStep` switch resumes silently
from `StateRunning`, `StateBlocked`, `StateFailed`, and `StatePaused`
alike) and no data is lost or corrupted either way — the defect is purely
in classification/reporting consistency, not correctness of the refusal
itself: an operator or automation (ly-drive's own `kind: "producer"` gets
exactly one retry before giving up, versus a `blocked` envelope it
recognizes immediately and stops on cleanly) sees a materially worse,
inconsistent signal for what is, in substance, the same kind of
business-judgment refusal as the two recognized ones. Severity: MEDIUM —
real, live-reproduced, reachable via an ordinary sabotage/hand-seed
scenario this campaign explicitly probes, and inconsistent with the
established, documented refusal shape right next to it in the same
function; not BLOCKING because nothing is lost, corrupted, or
irrecoverable. Fix: add a sentinel (e.g. `battenshed.ErrDisagreeingChildSeed`)
wrapped by `wire.go`'s `WriteSeed` closure when `shedrun.WriteSeed`'s own
"refusing to overwrite with disagreeing seed" error is the cause, and
recognize it in `childRecipeRefusal` alongside the other two, routing to
Stuck with a reason naming both the existing and incoming seed values.

### F4 (NIT, CONFIRMED live) — TestBattenIntegration_NonPrimeRefusal doesn't cover step/pause

`internal/battencli/lifecycle_integration_test.go`'s
`TestBattenIntegration_NonPrimeRefusal` only drives `run` and `status` from
a task-worktree cwd (line 566: `for _, verb := range []string{"run",
"status"} {`), while its sibling `TestBattenIntegration_WeftPrimeRefusal`
drives all four verbs (`run`, `step`, `status`, `pause`) from the weft
prime. **Live-confirmed** (see "Scenario: high-yield item 1" above): `step`
and `pause` both refuse correctly and identically from inside a task
worktree, exactly as `run`/`status` do. Confirmed a pure test-coverage
completeness gap, not a behavior bug. Fix: extend the loop to all four
verbs to match `WeftPrimeRefusal`'s own completeness.

(Live-driving scenarios continue below.)

### Live driving — fixture hub

Built fresh, distinct from any leftover state: bare warp+weft remotes seeded
with a minimal Go project (`module r5fx`), wired via
`lyx fabric clone --into <scratch> ...`, under
`.../scratchpad/r5hub/hub/r5fx-LYXHUB` (outside both the loomyard tree and
`$HOME/Code`). **Before any Board task or Worktree-Create**, committed and
pushed a `loom.yaml` override with `selfreport: false` onto the fixture
hub's prime weft (`main-weft`) — verified via `grep -n '^selfreport'` showing
`false` before doing anything else. Dev binary redeployed from current
source immediately before driving (`go run ./tools/deploy -dev`,
commit `9a8cc321a`).

### Scenario: high-yield item 1 — Batten Bookend Invariant (CONFIRMED, no defect)

Seeded Board task `bookend-a` (`type: loom`), ran `lyx batten step bookend-a`
once from prime (Worktree-Create only). From inside the freshly-created task
worktree (`<hub>/bookend-a`), ran `run`, `step`, `status`, and `pause` — all
four refused identically, naming both worktree names (`"bookend-a"` /
`"r5fx"`) and telling the operator to re-run from prime. Also confirmed from
the weft sibling (`r5fx-weft`) and from `_board`: both refuse by name
(`"is the weft sibling of a pair, not a warp worktree"` /
`"is the hub's _board checkout, not a warp worktree"`). All four verbs
refuse correctly from all three wrong vantage points — this closes **F4**
below as a pure test-coverage gap, not a behavior bug.

### Scenario: high-yield item 11 (F22-extended) — hand-seed authoritative, disagreeing dirty-prime interaction (CONFIRMED, no defect)

`lyx shed seed handseed-a --recipe batten` before any batten auto-seed:
wrote `{"recipe":"batten","driver":"go"}`. Dirtied prime
(`echo >> README.md`), ran `lyx batten run handseed-a`: create row halted
`blocked` naming fabric's own dirty-worktree refusal
(`"source worktree has uncommitted changes"`), no task worktree created, and
the hand-written seed was left byte-for-byte untouched (re-read after the
blocked run) rather than treated as a disagreeing seed. Also confirmed
`lyx shed seed <slug> --recipe batten --driver llm` is refused outright by
`shedcli` itself (`"recipe \"batten\" has no bootstrap verb, so it cannot be
driven by an LLM"`) — so the earlier hypothesis that a hand-seeded batten
run could carry a dead/inert `driver: llm` value was wrong; `shedcli`
already guards this consistently with `battencli`'s own validator. No
finding here.

### Scenario: high-yield item 3 — Run-Shed hard-errors on a blocked child, never routes to teardown (CONFIRMED, no defect)

Fresh slug `blocked-child`, stepped through Worktree-Create + Seed-Child for
real, then hand-planted the CHILD's own `_lyx/shed/self/status.json` as
`state: "blocked"` (a legitimate live-substrate technique: the file is the
real on-disk contract `InnerRun.ReadStatus` reads, and reading it before
ever spawning means no real child process is needed to prove this
row's own dispatch logic against a real git worktree). `lyx batten step
blocked-child` returned a **hard error** (`"kind":"producer"`), naming the
child's `current_producer`/`error` and the `haltedChildRemedy` text
verbatim; the persisted prime status went to `state: "failed"`; the task
worktree was confirmed still present afterward. Exactly the documented
contract (innerrun.go:148-152, batten-recipe.yaml's own header comment) —
no finding.

### Scenario: F3 — disagreeing child seed at Seed-Child (CONFIRMED — see F3 below)

Fresh slug `seedconflict`, Worktree-Create only, then hand-planted (and
**committed on the child's own weft**, so it is a genuine pre-existing seed
rather than a same-call race) `_lyx/shed/self/seed.json` = `{"recipe":
"loom","driver":"llm"}` — disagreeing with what Seed-Child is about to
compute (`driver` always resolves from prime's own `child_driver` param,
defaulted `"go"`). `lyx batten step seedconflict` returned a **hard error**
(`"kind":"producer"`, persisted `state:"failed"`, no `stuck_reason` key in
the envelope) rather than `Stuck`/`blocked` with a named remedy. Re-ran the
identical step again: deterministic, same error, task worktree
untouched/resumable — confirms this is a classification defect, not data
loss. Contrasted live against the two *recognized* refusals
(`ErrUnknownRecipe`/`ErrUnsupportedChildRecipe`): seeded Board task
`badtype` with `type: batten` (unsupported child recipe) and confirmed it
instead halts `state:"blocked"` with a `stuck_reason` key naming the Board
type and the remedy, in the everyday-resumable-blocked shape. This
side-by-side is the basis for F3's MEDIUM severity below: the same *kind*
of business-judgment refusal is surfaced two different, inconsistent ways
depending only on which of the three `WriteSeed` refusal shapes fired.

### Scenario: high-yield item 2 — teardown ordering, live (CONFIRMED, no defect; documented dirty-child behavior also confirmed live)

Fresh slug `teardown-check`, stepped through Worktree-Create + Seed-Child,
then hand-planted the child's status to `state: "done"` (uncommitted, so
the child's own weft carried a genuine untracked change at teardown time —
an accidental but valid stand-in for "an agent left something uncommitted
in the child"). `Run-Shed` correctly read `StateDone` and routed to
`Worktree-Teardown`. First `Worktree-Teardown` attempt: **stuck**, reason
`"worktree removal failed (session shutdown already succeeded): weft
worktree has uncommitted changes; run \"lyx fabric sync\" or use --force;
..."` — confirming live that (a) session shutdown ran and succeeded first,
(b) removal was attempted only after, and refused rather than forced, (c)
the ordering is visible in the reason text itself. Committed the pending
change on the child's weft by hand (the documented operator remedy) and
re-stepped: `Worktree-Teardown` completed for real, `state: "done"`, both
the task worktree and its weft sibling confirmed gone from disk. This is a
real, unstubbed `Shutdown`+`Remove` pair executing against a real fabric
pair — no finding.

(Live driving continues: item 6 prime-lock interleave, item 5 llm child
driver + the primary full end-to-end drive, items 7/8/9 cold-machine and
sabotage scenarios, this round's own focus points.)
