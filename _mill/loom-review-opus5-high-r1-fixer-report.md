# `loom` crucible round 1 — fixer report, tag `opus5-high-r1`

> Companion to `_mill/loom-review-opus5-high-r1.md`. Job 2 of the round: every finding recorded in
> that report, fixed. Job 1's report was committed (`8622e907c`) before a single production or test
> file was touched, per the sequencing rule.

## Summary

**5 findings recorded, 5 fixed, 0 deferred.** One commit per finding, each green before the next
started. Nothing pushed.

| ID | Severity | Status | Commit |
| --- | --- | --- | --- |
| F1 | MEDIUM | FIXED | `d41442393` |
| F2 | MEDIUM | FIXED | `59ce07aea` |
| F3 | LOW | FIXED | `73d07399b` |
| F5 | LOW | FIXED | `2b73b66e9` |
| F4 | MEDIUM | FIXED | `cf3c3fc11` |

Exactly one finding (F3) changed observable behavior; its module doc moved in the same commit, as
did F1's `CONSTRAINTS.md` bullet and F5's `doc.go` list. `manifest/roadmap.md` was not touched —
correct for a hardening pass.

## What was implemented

### F1 — enforce `refGate` constants ↔ `ledger` keys (`d41442393`)

`internal/planparser/shape_test.go`: added `refGateValuesDeclared`, which parses `shape.go`'s own
`refGate` const block out of the AST and reads each constant's **value** (not its identifier — a
correctly-named constant with a typo'd string is one of the two defects being caught), plus
`TestRefGateConstantsMatchLedger` asserting set equality with `ledger`'s keys in both directions.
Extended the file's doc comment to state that the registry rests on *two* independent syncs and why
both need an AST parse. `CONSTRAINTS.md`'s "Named enforcement" bullet named one sync where there are
two; it now names both and records that ledger completeness structurally cannot see a missing gate.

No production code changed — the defect was in what the tests guaranteed, not in the registry.

### F2 — widen the status tripwire to the v0.2.0 predicate spellings (`59ce07aea`)

`internal/planglyph/status_enforcement_test.go`: introduced `statusVocabularySelectors`
(`Status`, `Known`, `Rejected`) and matched against it in `statusHitsIn` instead of the bare
`sel.Sel.Name == "Status"`. Added two seeded self-tests — `TestStatusHitsIn_CatchesRejectedConsumer`
(whose fixture is the exact fail-open shape: treating "not a pre-resolution rejection" as
"resolved") and `TestStatusHitsIn_CatchesKnownConsumer`. Extended the file doc to explain why
`Rejected` is the spelling that mattered.

The real-tree scan (`TestStatusEnforcement_NoOutOfAllowlistConsumer`) still passes unchanged: every
existing `Known`/`Rejected` reader is already in `allowedStatusConsumers`, so the widening produced
no false positives.

### F3 — report an ambiguous Create target as the hazard it is (`73d07399b`)

`internal/planglyph/create.go`: gave `quarry.StatusAmbiguous` its own arm in `createFindings`,
raising blocking `create-already-exists` with every candidate named, instead of falling through to
`default:` and being rendered by `unreadableStatusDetail` as "answered the unrecognized resolve
status". Disposition unchanged (still blocking); only the check ID and the operator-facing sentence
change. Rewrote the `default:` arm's comment to record what moved and why, and updated
`createFindings`' own doc comment.

`manifest/designs/quarry-glyph-plan-alphabet.md`: the Create-inversion paragraph enumerated
`found`/`multipart` and both `not_found` rows but was silent on `ambiguous` — which is why the
code's disposition for it had no documented home to be checked against. Added the missing row, in
the same commit as the behavior change.

**This is the only observable behavior change in the whole round.** Before/after, live:

```
- glyph-rejected/1-create-card[blocking]:        Create target "plan:sub#Keep" answered the unrecognized resolve status "ambiguous"
+ create-already-exists/1-create-card[blocking]: Create target "plan:sub#Keep" already resolves ambiguous among existing declarations: sub#Keep, sub#Keep
```

### F5 — `doc.go`'s canonical Check-ID list made exhaustive again (`2b73b66e9`)

`internal/planglyph/doc.go`: the ref-shape centralization added a fail-closed `default:` arm to
`resolveContainment` raising `glyph-rejected` and never updated the list `doc.go` itself calls "the
canonical list a caller checks a claim against without reading Go source". Named all four raisers
(`resolve.go`, `create.go`, `containment.go`, `donecheck.go`) at the `glyph-rejected` bullet, gave
the containment bullet its own arm, and recorded that `create-already-exists` now covers `ambiguous`
alongside `found`/`multipart`.

### F4 — drive a real `quarry.DeltaGit` answer through `DetectDrift` (`cf3c3fc11`)

`internal/planglyph/drift_integration_test.go`: added `realRenameDelta`, which builds a real
two-commit git fixture with a real Go rename, calls the real `Delta` (spawning git through
`(*quarry.Repo).DeltaGit`), and returns quarry's own answer — **failing, never skipping**, when that
answer carries no matching exact-tier pair, since a skip would restore the blind spot being closed.
Two tests drive it through gate one from both sides:

- `TestDetectDrift_RealDeltaGateOneRecognizesDeclaredRename` — a declared Rename card's own outcome
  must produce no finding, no rewrite, and no amendments file.
- `TestDetectDrift_RealDeltaExactTierRepairsUndeclaredRename` — an undeclared rename must rewrite the
  referencing card and append exactly one six-field amendment.

Extended the file doc to say what the synthetic-delta tests can and cannot cover.

## Not-false-green proof (every new test sabotage-proved)

| Fix | Sabotage applied | Result |
| --- | --- | --- |
| F1 | Added a 14th `refGate` constant (`gateSabotage`) with no `ledger` entry | `TestRefGateConstantsMatchLedger` **FAILS**; `TestLedgerCompleteness` **PASSES** — the gap, demonstrated |
| F1 | Typo'd a `ledger` key (`prosa-path-onIy`) so it matches no constant | New test **FAILS** in both directions with both messages |
| F2 | Reverted the matcher to the pre-fix `sel.Sel.Name == "Status"` | Both new self-tests **FAIL**, each reporting `[]` — zero hits, the exact blind spot |
| F3 | Removed the `StatusAmbiguous` arm so it falls back to `default:` | Test **FAILS**, reproducing the pre-fix message verbatim |
| F4 | Simulated quarry's `Symbol.ID` spelling moving (`sub#New`→`sub::New`) | Whole hermetic tier stays **green**; new real-delta tier **FAILS** loudly, dumping quarry's actual answer |
| F4 | Broke `renameCardPairs`' `resolveKeyFor` normalization (the campaign's named regression) | Both tiers **FAIL**, as they should |

The F4 rows are the pair worth reading together: they show the new tier catches a class — a change in
quarry's own emitted contract — that the hermetic tier structurally cannot, because a test that
authors both sides of a comparison cannot check that the two sides agree in production.

Every sabotage was reverted and the suite re-run green before the next step.

## Test commands run, and results

Hermetic (final tree, `cf3c3fc11`):

| Command | Result |
| --- | --- |
| `go build ./...` | exit 0 |
| `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/...` | exit 0 |
| `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/...` | all 8 packages `ok`, exit 0 |
| `go vet -tags integration ./internal/planglyph/...` | exit 0 |
| `go test -tags integration -count=2 ./internal/planglyph/...` | `ok`, exit 0 |
| `gofmt -l internal/planparser/ internal/planglyph/` | no output |

Live smoke (`tmux` confirmed present at `/usr/bin/tmux` first, so a skip cannot masquerade as a pass):

`go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` — **9 pass, 2 fail.**
The two failures are **pre-existing and unrelated to this round**, proven rather than assumed: I
checked out the round's seed commit `8503e22f3` (detached HEAD, clean tree, nothing of mine present)
and re-ran them there, where **both fail identically**.

- `TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy` — a poisoned `status.json` produces
  `state: decode failed: json: unknown field "__smoke_unknown_field__"` and a refusal envelope where
  the test wants the tmux handover reached; driver log empty.
- `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` — the Discussion-Write shuttle run times
  out after ~61s and the machine does not advance.

Both sit in loom's driver bootstrap / phase machine, which this campaign's prompt puts explicitly out
of scope, and neither touches `internal/planparser` or `internal/planglyph`. Reported here, not
fixed: fixing them is a different concern and folding it into this round would violate the
one-concern-per-round rule. They are worth a targeted follow-up.

## Live re-verification against the rebuilt binary

Rebuilt first (mandatory — live driving tests the built binary, not the source tree):
`CGO_ENABLED=1 go build -ldflags "-X …buildinfo.Channel=dev" -o <scratch>/bin/lyx ./cmd/lyx`, from
`cf3c3fc11`. Then every scenario re-driven:

| # | Scenario | Result |
| --- | --- | --- |
| RV1 | Create + `plan:` handle canonicalization | `ok:true`; `plan:sub#Draft`→`plan:sub#Actual` in declaring **and** referencing card — unchanged |
| RV2 | Create inversion, `found` | `create-already-exists … already resolves found` — unchanged |
| RV3 | Create inversion, new unit | `create-new-unit` informational, `ok:true`, exit 0 — unchanged |
| RV4 | `glyph-ambiguous` on a non-Create target | unchanged, candidates listed |
| RV5 | **ambiguous Create target (F3)** | `create-already-exists … already resolves ambiguous among existing declarations: sub#Keep, sub#Keep` — **fixed** |
| RV6 | Rename exact-tier auto-bind via `record-batch 1` | `status: done`; handle bound in both cards; **no** `amendments.md` — unchanged |
| RV7 | Deliberate drift via `record-batch 2` | `plan-references-deleted-symbol/3-later-card[blocking]`, exit 1; **no** `amendments.md` — unchanged |
| RV8 | Exact-tier auto-**repair** via `record-batch 1` | card rewritten `sub#Gamma`→`sub#Delta`; exactly one six-field amendment — unchanged |

Every behavior except RV5 is byte-for-byte what the pre-fix binary produced. RV5 is the one intended
change.

## Changed files

| File | Change |
| --- | --- |
| `internal/planparser/shape_test.go` | F1: `refGateValuesDeclared` + `TestRefGateConstantsMatchLedger`; file doc |
| `CONSTRAINTS.md` | F1: Ref-Shape Registry Invariant's "Named enforcement" bullet |
| `internal/planglyph/status_enforcement_test.go` | F2: `statusVocabularySelectors`, widened matcher, two seeded self-tests, file doc |
| `internal/planglyph/create.go` | F3: `StatusAmbiguous` arm; `default:` and `createFindings` doc comments |
| `internal/planglyph/create_test.go` | F3: `TestCreateFindings_AmbiguousIsAlreadyExists` (real ambiguous answer) |
| `manifest/designs/quarry-glyph-plan-alphabet.md` | F3: Create-inversion's missing `ambiguous` row |
| `internal/planglyph/doc.go` | F5: Check-ID list made exhaustive |
| `internal/planglyph/drift_integration_test.go` | F4: `realRenameDelta` + two real-delta gate-one tests; file doc |
| `_mill/loom-review-opus5-high-r1.md` | Job 1 report |
| `_mill/loom-review-opus5-high-r1-fixer-report.md` | this file |

Production code touched: **one file** (`internal/planglyph/create.go`). Everything else is tests and
documentation — which is what a round that found no behavior regression should look like.

## Deferred

**Nothing deferred.** Every recorded finding, all severities, is fixed.

Two things are deliberately *reported and not fixed*, neither of them a finding of mine:

1. The two pre-existing smoke failures above — out of this campaign's scope, proven pre-existing at
   the seed commit, and a separate concern per the one-concern-per-round rule.
2. Windows path behavior remains a named, never-executed gap (no Windows host available), exactly as
   the review report records.

## Substrate teardown

- Scratch fixtures (`<scratch>/live1-HUB`, `<scratch>/live2-HUB`, the built `lyx`) removed.
- The standalone webster state directory the CLI derived for the fixture
  (`~/.local/state/lyx/e74a94dd`) removed.
- The Claude Code transcript stubs created for the fork audit
  (`~/.claude/projects/-tmp-…-live2-HUB-wt/`) removed.
- `pgrep -f tmux` back to its pre-round baseline; no tmux session was ever started by this round —
  the smoke suite manages its own and tore them down.
- Working tree clean, every commit on `crucible-loom-refshape-registry`, **nothing pushed.**

## Merge-readiness

**Merge-ready**, subject to the orchestrator's own independent verification, which is the actual gate.

The two refactors under test are behavior-preserving — established by diff audit and by eleven live
scenarios through the real binary, not by the unit suite. The three MEDIUM findings were gaps in the
machinery meant to *keep* them correct, and all three are now closed with sabotage-proved tests. The
one behavior change (F3) is a strict improvement to an operator-facing message, with its design doc
updated in the same commit.

The only thing I would not call settled is the pair of pre-existing smoke failures. They are outside
this campaign's scope and predate the round, but a reviewer should know the smoke suite is not fully
green on this branch and was not made so here.
