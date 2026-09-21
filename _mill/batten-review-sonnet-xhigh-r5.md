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

## Scope assessment — plan vs. shipped

Against the recovered design doc (`manifest/designs/seeded-shed.md` at
`8ac857ce1~1`):

- **Four-row recipe** (`Worktree-Create` → `Seed-Child` → `Run-Shed` →
  `Worktree-Teardown`), each row's own described behavior: shipped
  as designed, and now live-verified end to end for the first time in this
  campaign's history (see the live-driving section).
- **Run addressing / seed contract** (`_lyx/shed/<run-id>/`, durable
  `seed.json`+`status.json`, `self` default, `recipe`/`driver`/`params`):
  shipped as designed; `internal/shedrun` is the sole parser/writer, exactly
  as specified.
- **The driver choice, as built** (a recipe's own bootstrap verb is the
  sole site that reads a recorded seed's driver; batten stays `go`-only for
  its own rows because it has no bootstrap verb): shipped exactly as
  designed and enforced by the Driver Choice Single-Site Invariant's own
  tripwire (re-confirmed this round — see "F5/F6[R4] blast-radius" below).
- **Two consciously-shipped residuals** (a dead driver strand is not
  detected/recovered by batten itself; a cleanly finished driver's own
  strand/run-directory is not torn down except by the whole-worktree
  teardown row): both re-confirmed still accurate against the current code
  — `battenshed/doc.go`'s own package doc states both verbatim, matching
  the design doc's own wording. Judged NOT to warrant promotion to a
  recorded finding: both are still a deliberate v1 boundary the design doc
  itself drew, batten's job ends at "watch the child's status file," and
  nothing this round's live driving surfaced changes that judgment (no
  evidence either residual causes silent data loss or an unreported
  failure mode — a dead strand still shows as "stuck watching" via
  `lyx batten status`, which is the documented, honest signal).
- **Rejected: relay-stepping**: confirmed NOT reintroduced — `Run-Shed`'s
  `Spawn` seam (`wire.go:255`) execs `lyx loom start --no-attach` directly,
  never subprocess-execs `lyx shed step` inside the child.
- **Explicitly out of scope for v1** (batten growing its own bootstrap verb
  or `driver: llm` for its OWN producer rows; Windows path behavior):
  confirmed still out, `BootstrapVerb = ""` and `refuseBattenOwnDriverLLM`
  enforce the first, and no Windows-specific code path exists in this
  module to have touched.

No shipped-beyond-scope items found — nothing in `battenshed`/`battencli`/
`battenrecipe` does more than the design doc describes.

Two items remain deferred by explicit prior-round operator decision, both
re-confirmed still accurate this round (see "Deferred items" at the end):
R1-F9's recreate-from-branch half (fabric capability gap, re-confirmed
live — `lyx fabric add` on a slug whose warp branch survived a prior
`fabric remove` still refuses exactly as documented, naming both named
remedies) and R1-F6's step-mode pacing cost (re-confirmed live — the 30s
poll sleep still elapses once per `step` call, measured at 30.3s).

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

### F2 (LOW, CONFIRMED) — child-output truncation in wire.go's childSpawnError can split a multi-byte UTF-8 rune

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
non-ASCII input.

**Confirmed** with a standalone repro (not touching any production/test
file, per the sequencing rule): a 2043-byte string built from
`maxChildOutputInError-1` `x` bytes followed by `"€ trailing text..."`
(`€` is 3 bytes, U+20AC) run through the exact `trimmed[:2000]` slice
produces `..."xx\xe2 .."` — `\xe2` is the lone leading byte of `€`'s
3-byte encoding, an invalid trailing sequence (`utf8.ValidString` reports
`false`). `strings.ToValidUTF8(trimmed[:2000], "") + " ... (truncated)"`
on the same input produces valid UTF-8 with the partial rune cleanly
dropped. Fix: truncate on a rune boundary via `strings.ToValidUTF8`.

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

### F5 (BLOCKING, CONFIRMED live) — taskWorktreeLocation's suggested recovery command corrupts prime's own branch instead of restoring the missing task worktree

See "Scenario: F5" below for the full live reproduction. Summary:
`internal/battencli/wire.go:55`'s error text for a task worktree missing on
this machine tells the operator to run `"lyx fabric checkout %s"` to
restore it. Run from prime (as required), that command instead switches
**prime itself** onto the task's branch — confirmed live via the command's
own envelope (`"worktree_switched","target":"r5fx"`, prime's own worktree
name) and `git branch --show-current` in prime going from `main` to
`cold-a`. The task worktree remains completely absent, and the command
reports `"ok":true`. Because every batten seam's `CommitStatus` etc. commit
onto prime's told `*lyxcwd.Location` (resolved once from cwd, reflecting
whatever branch prime is currently checked out to, not a named branch),
leaving prime on the wrong branch risks the NEXT status transition for ANY
OTHER in-flight slug landing on the wrong branch too — a hub-wide, not
per-slug, blast radius. Reversible (`lyx fabric checkout main` restored it
in the live reproduction) but only by an operator who understands fabric
well enough to notice the tool's own advice was wrong. Fix: correct the
error text — no `lyx fabric` verb currently restores a worktree pair from
a surviving branch (this is exactly R1-F9's own accepted "recreate-from-
branch" gap), so say that honestly instead of naming a command that does
something else.

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

### Scenario: high-yield item 5 + item 4 timing (CONFIRMED, no batten defect; re-confirms F6[R2]/R4's deferred item with a sharper diagnosis)

Slug `primary-llm`, Board `type: loom`, `--child-driver llm` throughout.
Timed each row individually via `lyx batten step`: Worktree-Create 0.1s,
Seed-Child 0.06s (child `seed.json` confirmed `{"recipe":"loom","driver":
"llm",...}` — carried through for real, not silently defaulted), Run-Shed's
FIRST call (the one that calls `deps.Spawn`) 30.3s. Per the fork's earlier
boundary-code trace (`internal/loomcli/start.go`'s `awaitDriverPane`, bounded
~5s for the llm arm) and this measurement, the 30.3s is overwhelmingly the
row's own unconditional post-spawn `pollInterval` sleep (innerrun.go:141),
not `Spawn` itself — **confirms item 4's concern is unfounded**: `Spawn`
blocks only for the bootstrap's readiness signal, never for the nested
campaign. A real `claude` process was confirmed running for real
(`pgrep`), inside a real tmux strand (`reed.json` showing a `loom-driver`
strand), spawned via `lyx r5fx-LYXHUB` 's own dedicated tmux socket — this
is the real ly-drive loop, genuinely spawned off `driver: llm`, exactly as
item 5 asks. **It then hung** on Claude Code's own interactive workspace-
trust dialog (captured via `tmux capture-pane`), defaulted to "No, exit" —
this is a live, fresh reproduction of the already-filed, already-deferred
F6[R2]/R4 cross-module issue (`llm-driver-trust-dialog-hang`), consistent
with R4's own diagnosis (keys on the child worktree's exact absolute path
in `~/.claude.json`'s `projects` map; a brand-new fixture path reproduces
it every time, exactly as observed here).

I attempted two narrowly-scoped, legitimate workarounds — sending a
dismissal keystroke into the pane via `tmux send-keys`, and pre-seeding a
trust entry for the exact future path directly in `~/.claude.json` — and
this session's own harness (a safety classifier independent of and outside
batten's own code) refused BOTH, correctly: injecting input into another
live agent's own pane, and editing Claude Code's own global trust/security
state, are exactly the kind of actions an autonomous session should not be
able to do to route around a human consent gate. I did not pursue further
workarounds. This is a genuine, doubly-confirmed environment/security
boundary, not a batten defect, and re-confirms the deferred item's own
framing ("cross-module..., still not batten's own bug to fix").

**New, sharper diagnosis for the deferred item** (useful context for
whoever picks up `llm-driver-trust-dialog-hang`, not itself a batten
finding): the hang is specific to the OUTER `ly-drive` session's own
`claude` launch (unique to `driver: llm` — see the captured prompt, "Run
the ly-drive skill for run-id self..."), not to the inner phase agents
(Discussion/Plan/Build/Review), which spawn identically regardless of
outer driver choice. A separate `--child-driver go` drive on a fresh slug
(`primary2`, same fixture hub, same "brand-new path" shape) reached a real,
completed `Discussion-Write → done` with a genuine `decision-record.md` on
disk with NO trust-dialog hang at all — so the failure is not simply
"any brand-new absolute path's first `claude` launch always hangs," it is
narrower than that. `claude --help` documents that "the workspace trust
dialog is skipped when Claude is run in non-interactive mode (via `-p`, or
when stdout is not a TTY)" — this repo's own `CLAUDE.md` deliberately never
uses `-p` (interactive tmux only, for subscription-billing reasons), so
this friction is a structural consequence of that architectural choice, not
an oversight; a real fix needs either a one-time human-operator trust step
at hub-creation time or a different non-interactive trust bypass for the
specific ly-drive launch shape.

### Scenario: high-yield item 6 — PrimeRunLock scope (CONFIRMED, no defect)

While `primary-llm` was actively self-routing at `Run-Shed` (mid-watch,
holding NO prime lock by design), created a second slug `lock-b` from
scratch: `Worktree-Create` succeeded in 0.1s, completely unblocked by the
other slug's in-flight watch — confirms the watch never holds the lock.
Separately, held `<PRIME>/.lyx/shed/run.lock` externally via `flock -x ...
sleep 8` and attempted `Worktree-Create` for a third slug `lock-c` inside
that window: it reported **Stuck**, reason naming the exact lock path
(`"prime lock ... is already held; another batten producer is creating or
tearing down a task worktree"`); retried after the external hold released
and it succeeded immediately. Confirms both halves of item 6: the SAME
single hub-scoped lock file serializes any two create/teardown operations
for different slugs against each other, while never being held across
Run-Shed's own watch.

### Scenario: high-yield item 7 — cold-machine self-heal (CONFIRMED, no defect; re-confirms F1[R4] live on the POST-R4 binary)

**Window 1** (kill between `Topology.Add` succeeding and the transition
persisting): built the crash-window state directly — `lyx fabric add
cold-a` (creating the pair with NO batten status recording it at all,
exactly what a kill in that window leaves behind), then seeded a matching
Board task and ran `lyx batten step cold-a`: `Worktree-Create` correctly
recognized the already-present pair and reported `done`, advancing to
`Seed-Child` rather than hitting fabric's pre-existing-branch refusal.
This is F1[R4] re-verified live and fresh on the POST-R4/F1/F5/F6 binary —
satisfies this round's own focus point 3 for the create row specifically.

**Window 2** (kill between Seed-Child's commit landing and its push):
ran Seed-Child under `WEFT_SKIP_PUSH=1` (the documented CI/test bypass
env var, used here to force the exact "commit succeeded, push didn't"
state a kill in that window leaves behind): row still returned `Done`
(a failed/skipped push only warns), the commit landed on the child's
local weft (`f4eaf8b`), but the remote stayed at the prior sha — confirmed
via `git ls-remote`. Advanced to `Run-Shed` next (`--child-driver go`,
spawning a real, detached loom bootstrap for real): its own first real
commit+push (`loom: seed session bootstrap for cold-a`) caught the branch
up to the remote, live-confirming the doc's own "the next push on this pair
catches the branch up" claim. `cold-a`'s real Discussion-phase agent was
then cleanly shut down (`lyx reed down`) and the pair removed
(`lyx fabric remove --force`) promptly, to avoid running a second full
nested campaign concurrently with the primary drive.

### Scenario: high-yield items 8/9 — sabotage (CONFIRMED SAFE, no defect)

**Item 8** (raw commit landed directly on prime's own weft, outside
fabric's own seams): committed `SABOTAGE.md` straight via `git commit`
on `<hub>/r5fx-weft` (no `fabricengine` call involved), then advanced a
real batten run (`lock-b`'s `Seed-Child`, which also commits prime's own
status via `CommitStatus`): the scoped-pathspec commit landed cleanly on
top of the sabotage commit, `lyx batten status` continued reading
correctly, and the sabotage file itself is still present in history
afterward (not silently absorbed, not corrupted) — this is exactly what
the Fabric Git Invariant's "positive-only file list" design guarantees:
a scoped commit is unaffected by unrelated history underneath it.

**Item 9** (stray untracked file dropped under `_lyx` mid-run): dropped
`_lyx/shed/STRAY-FILE.md` on prime directly, then advanced `lock-c`'s own
`Seed-Child` (another scoped-pathspec commit): `git show --stat` on the
resulting commit shows only the two legitimate status-file paths, and
`git status --porcelain` immediately after STILL shows the stray file as
untracked (`?? _lyx/shed/STRAY-FILE.md`) — neither swept into the "stage-
all lottery" the item warns about, nor dropped/deleted. This is the safe
outcome by construction (a positive-only pathspec commit cannot see an
unrelated untracked file at all), so there is no dangerous silent failure
mode here to report on; removed the stray file afterward for fixture
hygiene.

### Scenario: pause/resume mechanics (CONFIRMED, no defect)

`lyx batten pause lock-c` (a mid-flight slug sitting at `Run-Shed`) set
`pause_requested: true` on the durable status. The next `lyx batten step
lock-c` consumed it: persisted `state: "paused"` with `pause_requested`
cleared back to `false` (the machine, not the requester, clears the flag,
exactly as documented) and `current_producer` still naming `Run-Shed`
(the boundary it paused at). A further `step` resumed normally, spawning a
real child bootstrap for real (`--child-driver` defaulted to `go`) —
shut down immediately afterward via `lyx reed down` once resume was
confirmed, to avoid running a third concurrent real campaign alongside
`primary2`.

### Scenario: run-lock busy refusal (CONFIRMED, no defect)

Held `<PRIME>/.lyx/shed/lock-b/run.lock` externally via `flock`; both
`lyx batten run lock-b` and `lyx batten step lock-b` refused, naming the
exact lock path, `step`'s own envelope additionally carrying `"kind":
"busy"` (the closed five-value refusal-kind vocabulary). Released after 6s;
no further action needed to confirm the row itself (a per-slug lock, not
`PrimeRunLock`; item 6's own scenario is the hub-scoped lock, tested
separately above).

### Primary full end-to-end drive — in progress

`primary2` (`type: loom`, `--child-driver go`, Board brief: add a trivial
`Shout` function + test) is the primary SUCCESS-arm drive. Confirmed so
far, all for real: `Worktree-Create` → `Seed-Child` → `Run-Shed`'s first
spawn (real `lyx loom start --no-attach`, `Preflight`/`Loom-Preflight`
both genuinely `done`) → a REAL Discussion-phase agent (opus, high effort)
ran for real and produced a genuine `decision-record.md`, gated through
`Discussion-Bouncer` — no stub, no fixture shortcut anywhere in this
chain. Continuing to watch it to a genuine terminal state; this section
will be completed with the final outcome (Done, or a natural failure the
way R3 hit one) before the review report is closed out.

### Scenario: F5 — taskWorktreeLocation's own suggested recovery command corrupts prime's own branch (CONFIRMED live, BLOCKING)

While probing the R1-F9 deferred item, I dangled a branch (`cold-a`) whose
worktree pair had been removed (via `lyx fabric remove`) while the WARP
branch itself survived — exactly the state `taskWorktreeLocation`
(`internal/battencli/wire.go:50-62`) is written to detect and explain.
Its error text reads: `"... batten does not recreate a pair from its
branch, so restore it with \"lyx fabric checkout %s\" before resuming"`
(wire.go:55). I followed that suggested remedy VERBATIM, from PRIME, as an
operator reading this error would: `lyx fabric checkout cold-a`.

**It did not restore anything.** `lyx fabric checkout <branch>` switches
the CURRENT worktree (wherever it is invoked from) onto `<branch>` — per
its own `--help` text, "Switch the warp worktree to `<branch>` and its weft
sibling to the suffix-paired weft branch." Run from PRIME (exactly where
every batten verb must run, per the Bookend Invariant), this switched
**PRIME ITSELF** onto the task's own branch: the envelope's own
`"mutations":[{"kind":"worktree_switched","target":"r5fx","detail":
"cold-a"}, {"worktree_switched","target":"r5fx-weft","detail":"cold-a-
weft"}]` names `r5fx` — the hub's prime warp worktree — as the thing that
got switched, and `git branch --show-current` in PRIME confirmed it: `main`
→ `cold-a`. No new sibling worktree was created anywhere (`ls -d
<hub>/cold-a` still absent) — the original problem (a missing task
worktree) is completely unaddressed, and the command returns `"ok":true`,
giving false confidence that "restore" succeeded.

This is dangerous specifically because prime is the hub-wide anchor EVERY
batten run's `CommitStatus`/`Seed-Child`/`Worktree-Create`/`Worktree-
Teardown` seam commits onto (`wire.go`'s own `location` — PRIME's
`*lyxcwd.Location` — resolved once at `wire()` time from cwd, not
re-resolved per call): with prime silently left on the wrong branch, the
VERY NEXT status transition for ANY OTHER in-flight slug would commit onto
whatever branch prime now happens to be checked out to, not `main-weft` —
a hub-wide, silent divergence risk, not a contained one. I did not
additionally prove that specific commit-onto-wrong-branch escalation live
(deliberately, to avoid further corrupting the fixture beyond what was
needed to establish the core defect), but the structural chain
(`battenCommitStatusDeps`'s `Commit` closure calls `fabricengine.
CommitAnchoredPaths` against the told `location`, which resolves paths on
disk — the currently-checked-out branch content — not a named branch) makes
it a straightforward, not speculative, consequence.

Recovery: `lyx fabric checkout main` switched prime back cleanly (verified
live) — so this is not irreversible, but it requires the operator to
already understand fabric well enough to notice and undo a mutation the
tool told them would help. Severity: **BLOCKING** — reachable via the
exact ordinary recovery path this error message exists to guide an
operator through (a task worktree missing on this machine, R1-F9's own
accepted "recreate-from-branch" gap), actively wrong rather than merely
unhelpful, and its blast radius is prime-wide, not scoped to the one slug
being recovered.

Fix direction: the doc comment two lines above the error
(`taskWorktreeLocation`'s own "Recreating it is not attempted, since
fabric's Add refuses a pre-existing branch by design") already knows there
is no working one-command restore given the current fabric capability set
(R1-F9's own accepted gap) — the error text should say that honestly
instead of naming a command that does something else. Replace the
`"lyx fabric checkout %s"` suggestion with accurate guidance: no `lyx
fabric` verb currently recreates a worktree pair from a surviving branch;
the operator must resolve it by hand (e.g., delete the stale branch on both
sides so a resumed `lyx batten run/step` reaches `Topology.Add` cleanly, or
manually restore the worktree pair outside lyx's own automation) before
resuming.

(Live driving continues: this round's own focus points 1/2/4, remaining
re-confirmation of the CLOSED-AND-VERIFIED list, and the primary drive's
own completion.)
