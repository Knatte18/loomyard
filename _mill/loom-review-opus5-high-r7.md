# `loom` — independent review, round 7 (`opus5-high-r7`)

Clean-room round-7 review of the `loom` module per `_mill/loom-review-prompt.md`.
Written incrementally during Job 1 ("Log as you go"); the executive summary and final severity ordering were written last.

## Status

Job 1 in progress — this file is appended to as each command/scenario returns.

## What was tested

(Appended incrementally. Exact commands + observed results.)

### Environment preflight

```
git branch --show-current   -> crucible-loom-refshape-registry
git status --short          -> clean
which tmux                  -> /usr/bin/tmux          (present; smoke tests will not skip-as-pass)
go version                  -> go1.26.0 linux/amd64
which gcc                   -> /usr/bin/gcc           (cgo build prerequisite satisfied)
```

### Hermetic gates

```
go build ./...                                 -> BUILD_OK (clean)
go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... \
       ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... \
       ./internal/webstercli/... ./internal/shuttleengine/...
                                               -> clean, no diagnostics
go test -count=5 <the nine in-scope packages + ./cmd/lyx/...>
                                               -> EXIT=0, all ok
```

## Findings

(Appended provisionally as spotted.)

### F1 — `Attach`'s three reed-state gates abandon a finished run without consulting the file contract (the SIXTH instance of the recurring shape)

**Severity: MEDIUM. CONFIRMED** (traced statically here; live reproduction recorded under "What was tested").

`internal/shuttleengine/attach.go:64`, `:68`, `:76`.

Round 6 put the file-contract-first guard at the TOP of `dispositionCandidate` (`attach.go:301`).
But `dispositionCandidate` is called at `attach.go:84`, which is downstream of three earlier
gate returns that abandon every candidate before any disposition is computed:

| line | gate | returns |
|---|---|---|
| 64 | `reedengine.LoadState` errored (unreadable/truncated `reed.json`) | `(Result{}, false, err)` |
| 68 | `LoadState` reported the state file ABSENT | `(Result{}, false, err)` |
| 76 | `r.reed.Status()` errored (incl. reed's foreign-session refusal) | `(Result{}, false, err)` |

All three answer "has reed's own bookkeeping gone wrong", never "did this run finish" — the exact
sentence `finishedDespiteMechanismFailure`'s own doc comment (`wait.go:456-478`) uses for round 5's
two `Wait`-side caps. This is the same defect shape, one layer earlier, on the entry side:
`Attach`'s twin of round 5's fix, exactly as round 6's `dispositionCandidate` guard was `Attach`'s
twin of round 4's.

**Failure scenario (inputs/state → wrong behavior).**
1. A driver crashes after its agent wrote every declared output file but before any `Wait`
   classified the run, so `run.json` is stranded at `outcome:"running"` with all `OutputFiles` on
   disk — the exact state round 6's guard exists to harvest, and the state
   `manifest/designs/loom.md`'s crash-recovery step 2 promises is harvested as `done`.
2. reed's own strand table then becomes unreadable (`kill -9`/full disk/power loss during a reed
   write — reed's `unreadableStateError` documents this as "not rare"), or absent (a
   `git clean -xdf` of `.lyx`, which `run.go:361` itself names as "a sanctioned operator action
   under the Durable-vs-Ephemeral State Invariant"), or `reed.Status()` refuses (reed's
   foreign-session refusal after a renamed worktree or a copied `.lyx` — the very cases
   `errStrandNotTracked`'s doc comment enumerates).
3. The next resume's `SingleLLMProducer.Call` probes `Attach` first (`singlellm.go:118`). `Attach`
   collects the one matching candidate, then hits one of the three gates and returns an error.
4. `singlellm.go:119-124` turns that into a hard producer error
   (`shedadapters: <row> (shuttle): shuttle attach: …`), which fails the whole Shed step. The
   finished, expensive LLM output sitting on disk is never harvested; the step needs operator
   intervention instead of self-healing.

The `Wait`-side twin of exactly this is already fixed and documented: `loom.md:351` records round 5
reproducing "a run whose every declared output file was on disk, its `reed.json` truncated mid-run
… abandoned with a mechanism error rather than classified `done`". `Attach` still does that.

**Why this is not one of the five closed instances.** The five are `wait.go`'s two deadline paths,
`wait.go`'s two mechanism-failure caps, and `attach.go:301`'s `dispositionCandidate` guard. All
three sites here are *upstream* of `dispositionCandidate` and none of them is reachable from
`Wait` at all. Grepping `allOutputFilesExist(` still finds exactly the audited 7 sites; these three
gates are the ones that were never counted because they return an `error`, not a verdict.

**Suggested fix.** The direct twin of `finishedDespiteMechanismFailure`: before each of the three
gate returns, check whether a candidate's file contract already answers the question, and attach it
instead of erroring. The check needs no reed read at all (round 6's own comment: "a run that
FINISHED whatever reed now thinks of its pane"), and the reconstructed run's `Wait` then harvests
it as `OutcomeDone` through round 5's own `finishedDespiteMechanismFailure` even with reed still
broken. Keep it conservative: only when exactly ONE candidate carries `outcome:"running"` with a
satisfied contract; otherwise fall through to today's error unchanged, so every healthy-reed path
stays byte-identical.

### F2 — `attach.go`'s not-tracked comment claims reed repairs an unreadable `reed.json`; reed explicitly refuses to

**Severity: LOW. CONFIRMED** (both sides read).

`internal/shuttleengine/attach.go:332-335` justifies the age escape with:

> An absent or unreadable reed.json is repaired in-band by "lyx reed up", or simply by
> "lyx loom run", which calls reed.Up() itself.

That is true for ABSENT and false for UNREADABLE. `internal/reedengine/state.go:158-160` states the
opposite in as many words — "Repairing the file automatically is deliberately not offered: every
repair reed could perform amounts to discarding the strand table" — and
`unreadableStateError`'s own comment (`state.go:145-148`) records that reed refuses "EVERY verb that
loads state — up, resume, status, add, remove, even attach" on a corrupt file. So the one remedy
this comment offers an operator for the unreadable case does not exist; reed's actual remedies are
`lyx reed down` or deleting the file by hand.

This matters beyond tidiness: it is the sentence a future reader would rely on to conclude the
unreadable case self-heals — which is precisely the wrong conclusion for F1's scenario.

### F3 — the package doc's `Attach` summary predates the file-contract-first guard

**Severity: NIT. CONFIRMED.**

`internal/shuttleengine/doc.go:58-63` still describes `Attach` as answering its question "on
live-agent evidence plus the persisted `RunState.Outcome`" and frames those as "a deliberate
two-line-of-defence framing". Since round 6 there is a third, higher-precedence line of defence —
the file contract, consulted at `attach.go:301` BEFORE the liveness dispatch — and F1 adds two more
places it is consulted. A reader who takes `doc.go` as the package's summary of `Attach` gets a
description that no longer matches `attach.go`. `attach.go`'s own file doc comment (`attach.go:1-5`)
has the same gap.

## Docs & operability findings

(Appended provisionally as spotted.)

- F2 and F3 above are comment/doc-accuracy findings; both are fixed in Job 2 alongside their code.
- `manifest/designs/loom.md:352-357` (crash-recovery step 2) describes `Attach`'s file-contract-first
  rule but places it entirely inside the matched-record disposition, with no mention that three
  reed-state gates sit ahead of it. It needs the F1 fix recorded in the same change.
