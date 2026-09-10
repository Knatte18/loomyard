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

### Live smoke suite (real substrate, real tmux, zero provider subprocesses)

```
which tmux -> /usr/bin/tmux (present, so no test could skip-as-pass)
go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1  -> EXIT=0
```

All 12 cases PASS, none SKIP: `TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning` (4.09s),
`TestSmokeBootstrap_BringsUpSessionStrandAndDriver`, `…_SecondInvocationDoesNotSpawnASecondDriver`,
`TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` (the assertion commit `aba2c270a`
rewrote), `…_RefusesOnNeverSeededPair`, `…_FailureBeforeFirstPersistLeavesNonEmptyLog`,
`TestSmokeBootstrap_MalformedStatusProceedsToHandoverAndLogsWhy` (the `state.ErrDecode` fix's guard),
`TestSmokeFabricAdd_RunLauncherExistsThenGoneAfterRemove`, `…_CleanlinessOrderingAfterSeedCommit`,
`…_OriginRecordSelfHealsAfterCrashBetweenWriteAndCommit`, `…_ConcurrentSpawnHandshakeYieldsOneDriver`,
`…_DiedDriverProceedsToHandoverAndLogsWhy`.
Teardown confirmed: `pgrep -af tmux` afterwards matched only my own probe command line, and
`tmux ls` answered "error connecting to /tmp/tmux-1000/default (No such file or directory)" — zero
stray tmux servers.

### Thread A — live driving through the REAL built binary (regression spot-check)

I built a real fabric hub end to end from the CLI rather than reusing a Go fixture helper:

```
CGO_ENABLED=1 go build -o <scr>/bin/lyx ./cmd/lyx
git init --bare -b main <scr>/remotes/demo3.git ; …/demo3-weft.git   (warp pushed, weft empty)
lyx fabric clone <scr>/remotes/demo3-weft.git <scr>/remotes/demo3.git   -> ok:true, hub demo3-HUB wired
lyx fabric add r7task                                                    -> ok, pair r7task / r7task-weft
```

then planted a real quarry-resolvable Go tree in the warp worktree (`internal/rows` with `Row`,
`MapRow`, `Helper`; `internal/out` with `Ok`; later `dup_a.go`/`dup_b.go` declaring `Dup` twice) plus
a real `_lyx/plan/`, and drove `lyx loom validate-plan` for each scenario. Every command was run
foreground from the worktree root and its full JSON envelope read.

| # | scenario | observed | verdict |
|---|---|---|---|
| A0 | well-formed plan, Create handle declared + referenced, Rename to-side a `plan:` handle | `{"ok":true,"plan_dir":…}` for both `validate-plan` and `--require-approved` | matches spec |
| A1 | `Create` targeting an already-existing symbol (`plan:internal/rows#Helper`) | `create-already-exists/1-…[blocking]: Create target "plan:internal/rows#Helper" already resolves found` | Create inversion correct |
| A2 | `Create` into a brand-new unit (`plan:internal/brandnew#Thing`) | `create-new-unit/…[informational]`, envelope `ok:true` with the finding surfaced under `findings` | matches spec's informational rule |
| A3 | reference a member that does not exist (`internal/out#NoSuchSymbol`) | `glyph-not-found/…[blocking]: …'s unit exists but the member is missing — check for a misspelled member` | `not_found` + `Unit: found` branch correct |
| A4 | `Rename` to-side spelled as a glyph, not a handle | `rename-to-not-handle/…[blocking]: … has a new side that is a glyph, not a plan: handle` (+ the consequent `handle-dangling`) | `gateRenameTo` `dispFinding` correct |
| A5 | genuinely AMBIGUOUS member (`Dup` declared in two files of one unit) | `glyph-ambiguous/…[blocking]: target "internal/rows#Dup" is ambiguous among candidates: internal/rows#Dup (internal/rows/dup_a.go), internal/rows#Dup (internal/rows/dup_b.go)` | real `ambiguous` status, listed candidates — the quarry-v0.2.0 `Status.Known()` call site driven for real |
| A6 | `Create` inversion over that same ambiguous target | `create-already-exists/…[blocking]: … already resolves ambiguous among existing declarations: …dup_a.go, …dup_b.go` | matches the spec's explicit ambiguous-is-create-already-exists rule |
| A7 | resolve-backed containment: card 2 edits `internal/rows#Helper`, card 1 edits `//internal/rows/rows.go` | `containment-file-overlap/2-…[blocking]: card 2's member glyph physically overlaps card 1-…'s own file self glyph naming "internal/rows/rows.go"` | tier 2 correct |
| A8 | syntactic containment: card 1 `internal/rows#`, card 2 `//internal/rows/rows.go` | `containment-unit-overlap/2-…[blocking]: card 2's file self glyph … overlaps card 1-…'s own self glyph naming that file's whole unit "internal/rows"` | tier 1 correct |
| A9 | quarry infrastructure error (`chmod 000 internal/rows`, so quarry cannot read the tree) | `{"error":"loom: quarry could not answer validating plan at …: planglyph: quarry could not answer: resolve: engine: read .gitignore …: permission denied","ok":false}` | `ErrQuarryUnavailable` disposition correct — reported as a gate failure, NOT as "plan is not yet valid", so a quarry outage cannot silently pass a `Create` card |
| A10 | bare package-qualified symbols on both Rename sides and on an Edit | three `bare-symbol-target[blocking]` + `rename-to-not-handle … is a bare symbol` + `rename-from-not-glyph … is a bare symbol`; **no panic** | `refKindSymbol`'s `dispFinding` entries in `gateBareSymbolTarget`/`gateRenameTo`/`gateRenameFrom` exercised live |
| A12 | real `Prosa:` group under `language: none` with a glyph, a bare symbol and a path | `prosa-symbol-target[blocking]` for the glyph and the symbol, none for the path | `gateProsaPathOnly` (path `dispKeep`, others `dispFinding`) exercised live |
| A13 | the same Prosa group under `language: go` | `bare-symbol-target` + two `prosa-symbol-target` | the glyph-enabled branch of the same check |

**Fail-closed `lookup` — answered.** The prompt asks whether a symbol-shaped ref reaching a gate
with no entry for `refKindSymbol` actually panics live. It cannot, and that is correct by
construction, not an untested gap: every one of the thirteen policies in `internal/planparser/shape.go`'s
`ledger` declares all four kinds explicitly (verified by reading the map, lines 107-186), and
`TestLedgerCompleteness` plus the two AST sync meta-tests make that mechanical. The panic in
`lookupIn` (`shape.go:196-198`) guards the addition of a FIFTH `refKind`, which is exactly what its
own doc comment claims. A10/A12 drove `refKindSymbol` through three `dispFinding` gates live with no
panic and correct findings, which is the reachable half of the question.

### Thread A — DetectDrift against real git deltas

`lyx webster record-batch <NN>` was NOT reachable for a live drive: `RecordBatch` reaches
`planglyph.DetectDrift` only behind the bracket-discipline check, an incremental fork audit over real
provider transcripts, and the per-card done-checks (`internal/websterengine/recordbatch.go:87`,
`:273-330`), so driving it through the CLI would have required a genuine LLM-driven webster batch —
which this campaign's cost declaration forbids and which its design intent says the mechanics under
test do not need. Stating that explicitly rather than skipping silently, per the prompt.

What I drove instead is the same code against the same real substrate one layer in — the
`//go:build integration` suite, which builds REAL git fixture repos and calls the REAL
`quarry.Repo.Resolve`/`Delta`:

```
go test -tags integration ./internal/planglyph/... \
  -run 'DetectDrift|Delta|Containment|Create|Resolve|Handle|Done' -count=1 -v
```

60 cases, all PASS. The ones that carry this round's regression question: 
`TestDetectDrift_RealDeltaGateOneRecognizesDeclaredRename` and
`TestDetectDrift_RealDeltaExactTierRepairsUndeclaredRename` (both build a real git rename commit and
run the real `Delta`), `TestDetectDrift_RenameMatchingCardPairProducesNoFindingNoAmendment` (the
`renameCardPairs` regression a prior round hit for real — still closed),
`TestDetectDrift_DeletedAndStillReferencedProducesBlockingFinding` (evidence tier stays blocking and
never repairs), and the whole `TestCreateFindings_*`/`TestCanonicalizeHandles_*`/`TestBindHandles_*`
families.

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
