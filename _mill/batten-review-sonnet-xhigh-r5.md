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

### F3 (severity TBD, PLAUSIBLE pending live confirmation) — a disagreeing child seed at Seed-Child hard-errors instead of routing to Stuck

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
before Seed-Child (re)commits its own, which is exactly the sabotage/
hand-seed shape this round's high-yield items 8/9/11 already ask me to
drive. Plan: reproduce live via `lyx shed step` against a freshly created
task worktree, hand-seeding the child's own `_lyx/shed/self/seed.json`
with a disagreeing value before letting Seed-Child run, and observe the
actual verdict/envelope. Severity and fix approach (wrap the disagreement
in a new sentinel, e.g. `ErrDisagreeingChildSeed`, and treat it as Stuck)
to be finalized after that live scenario.

### F4 (NIT candidate, test-coverage gap) — TestBattenIntegration_NonPrimeRefusal doesn't cover step/pause

`internal/battencli/lifecycle_integration_test.go`'s
`TestBattenIntegration_NonPrimeRefusal` only drives `run` and `status` from
a task-worktree cwd (line 566: `for _, verb := range []string{"run",
"status"} {`), while its sibling `TestBattenIntegration_WeftPrimeRefusal`
drives all four verbs (`run`, `step`, `status`, `pause`) from the weft
prime. High-yield focus item 1 asks me to drive all of `run/step/status`
from inside a freshly-created task worktree live; if `step`/`pause` refuse
correctly there (expected, since `armAt`'s bookend check runs ahead of the
verb branch for every verb), this is a test-coverage completeness NIT
rather than a behavior bug — extend the loop to all four verbs to match
`WeftPrimeRefusal`'s own completeness. To be confirmed live before fixing.

(Live-driving scenarios continue below.)
