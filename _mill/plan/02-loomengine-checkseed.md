# Batch: loomengine-checkseed

```yaml
task: 'preflight: split into two Shed rows -- a generic one, and loom''s own'
batch: 'loomengine-checkseed'
number: 2
cards: 5
verify: go test ./... -count=1 && go test -tags integration ./... -count=1 && go vet -tags smoke ./internal/loomcli
depends-on: []
```

## Batch Scope

This batch delivers loom's own half of the split as an exported, told-paths function: `loomengine.CheckSeed(statusPath, statusLockPath, expectedProducer string, toleratedProducers []string) (Report, error)`, plus the told-name parameters `checkCoherence` needs to stop hardcoding `"Preflight"` twice.
Because `CheckSeed` takes told paths and performs no git spawn — it stats a file, `MkdirAll`s a lock parent, and decodes JSON — its coverage lands directly at Tier 1, which is the whole point of the retargeting.

The batch is deliberately transitional and touches no other package.
`loomengine.Preflight`/`checkResolved`/`runCheck4` all survive it unchanged in behaviour: `runCheck4` keeps its own `check3BlocksSeed` derivation and its own read, and is edited only to pass the row-1 names into `checkCoherence`'s new parameters.
That is what keeps `internal/loomshed`, `internal/loomcli` and `internal/loomengine`'s own Tier-2 integration suite compiling and green through this batch, and it is why this batch has no dependency on batch 1.
The duplication between `runCheck4`'s read and `CheckSeed`'s is temporary by construction — batch 4 deletes `runCheck4` outright, and nothing is built on top of it in the meantime.

The external interface batch 3 consumes is the exported `CheckSeed` and nothing else.

## Cards

### Card 7: told producer names in checkCoherence

- **Context:**
  - `internal/loomengine/report.go`
  - `internal/loomshed/loomshed.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/status.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/loomengine/coherence.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `checkCoherence`'s signature to `checkCoherence(shed shedengine.Status, product Status, expectedProducer string, toleratedProducers []string) []Failure`.
  Replace the hardcoded `"Preflight"` literal in the `current_producer` rule with `expectedProducer`, in both the comparison and the failure `Reason` string.
  Replace the hardcoded `"Preflight"` literal in the fresh-start history rule with a membership test against `toleratedProducers` — use `slices.Contains(toleratedProducers, h.Producer)`, importing `slices` (the repo already uses it, e.g. `internal/loomcli/smoke_test.go`) — and reword its failure `Reason` so it names the tolerated set rather than a single producer.
  Rewrite the three prose blocks in this file that assert a one-row world and become false after the split.
  (1) The file header currently says the validator checks "the fresh-start invariants Preflight enforces"; it now describes the invariants `CheckSeed` enforces, with the producer names told by the caller rather than known here.
  (2) The comment above the `current_producer` rule currently argues that "check 4 is only ever reached while Preflight's own gate holds, and that gate is only satisfied by the very first producer in loom's list — so a coherent seed's `current_producer` must always name it"; it must instead state that `shedengine.Run` persists the *next* row's name into `current_producer` and appends the finished row's history entry **before** calling that next row, so at the instant loom's own seed row runs the file already names that row — and that the expected name is therefore told by the caller, not derived here.
  (3) The comment above the fresh-start history rule currently says "a `Stuck` outcome at row 1 (`Preflight` itself) leaves one `Preflight` entry behind"; it must now cover both rows — a `Stuck` at either the generic row or loom's own row leaves that row's own entry behind, which is why both names are in the tolerated set, and an entry naming any producer outside that set is the real half-finished signal.
  Keep every other rule, comment and failure message in the file byte-identical, including the `shed.Error`/`shed.Activity` never-validated note and `isRFC3339UTC`.
  Do not change `checkCoherence`'s behaviour for a caller that passes `"Preflight"` and `[]string{"Preflight"}` — that equivalence is what keeps this batch's Tier-2 suite green.
- **Commit:** `refactor(loomengine): tell checkCoherence the expected and tolerated producer names`

### Card 8: CheckSeed over told paths

- **Context:**
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/coherence.go`
  - `internal/loomengine/report.go`
  - `internal/loomengine/status.go`
  - `internal/state/state.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/status.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/seed.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomengine/seed.go` declaring one exported function, `CheckSeed(statusPath, statusLockPath, expectedProducer string, toleratedProducers []string) (Report, error)`.
  Its body is `runCheck4`'s check-4 half (`internal/loomengine/preflight.go` lines 87 onward), retargeted from `LoomStatusFile(l)`/`LoomStatusLock(l)` onto the two told path parameters, starting from a zero `Report` rather than one already carrying tier-1/tier-2 failures, and ending with the same `report.OK = len(report.Failures) == 0` assignment.
  Drop the `check3BlocksSeed` derivation and its `switch` branch entirely: a stat failure satisfying `os.IsNotExist` is `CheckSeedMissing` unconditionally, and any other stat failure is `CheckSeedUnreadable` carrying `err.Error()`.
  Keep the `os.MkdirAll(filepath.Dir(statusLockPath), 0o755)` guard verbatim, including its full explanatory comment, retargeted at the told lock path — `internal/lock` opens with `O_CREATE` but never creates parents, and without it a worktree with no ephemeral tree escalates to a hard infra error instead of honouring the report-not-error contract.
  Keep the `state.ReadJSONStrict[shedengine.Status]` call, the `errors.Is(rerr, state.ErrDecode)` split between a determined `CheckSeedIncoherent` verdict and an escalated infra error, the `!found` TOCTOU guard synthesizing a non-nil error rather than returning `Report{}, nil`, and the `product` unmarshal branch whose failure is a determined `CheckSeedIncoherent` with the "product does not decode as loom's status shape" reason.
  Pass `expectedProducer` and `toleratedProducers` straight through to `checkCoherence`.
  `CheckSeed`'s doc comment must state: the report-not-error contract in the same three-case shape `internal/preflight/doc.go` records; that it takes told absolute paths and resolves no geometry of its own; that the expected and tolerated producer names are told by the caller because they are the caller's own durable row identities; and **the step-1 pre-emption rule, stated once and generally** — *every verdict `shedengine.Run`'s step-1 read gate already hard-errors or short-circuits on is unreachable when `CheckSeed` is driven as a producer row*, because step 1 reads the same told path through the same `state.ReadJSONStrict[shedengine.Status]` decoder before any producer is looked up.
  Name the four verdicts that rule covers and why each is still kept: `CheckSeedMissing` (step 1's `!found` hard-errors, "status file %q does not exist; Shed never seeds one"); `CheckSeedIncoherent` via the `state.ErrDecode` branch (step 1's identical strict read errors first); `CheckSeedIncoherent` via `checkCoherence`'s invalid-state rule (step 1's `!st.State.valid()` hard-errors); and `CheckSeedIncoherent` via `checkCoherence`'s `StateDone` rule (not an error but a short-circuit — step 1 returns `RunDone` without calling any producer).
  State that all four are kept because `CheckSeed` is an exported function over told paths, not a private helper of one row, with a Tier-1 suite that calls it directly, and that deleting verdicts from the closed set `contracts/specs/loom-status-spec.md` pins would trade a documented contract for nothing.
  State separately that the genuine stat-error branch and the TOCTOU guard carry **no** unreachability claim, because both turn on a filesystem state change between step 1's read and this function's own stat, which no gate can pre-empt.
  Do not re-derive whether a failure is a downstream consequence of an earlier tier's failure — `CheckSeed` receives no tier-1/tier-2 report and must not reference one.
  Do not delete or rename any of the four loom-specific `CheckID` constants.
- **Commit:** `feat(loomengine): add CheckSeed, the seed-coherence check over told paths`

### Card 9: point runCheck4 at the told-name coherence signature

- **Context:**
  - `internal/loomengine/coherence.go`
  - `internal/loomengine/seed.go`
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `internal/loomengine/preflight.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `runCheck4`'s single `checkCoherence(shed, product)` call to `checkCoherence(shed, product, "Preflight", []string{"Preflight"})`, preserving today's exact behaviour for every caller of `Preflight` and `checkResolved`.
  Change nothing else in this file — `check3BlocksSeed`, the stat switch, the `MkdirAll` guard, the TOCTOU guard and every doc comment stay exactly as they are.
  Add a one-line comment above the call noting that these two literals are transitional: batch 4 deletes this function outright, and `internal/loomshed` is what tells the real names to `CheckSeed`.
  Do not delete `Preflight`, `checkResolved` or `runCheck4` in this batch.
- **Commit:** `refactor(loomengine): pass row-1's names through runCheck4's checkCoherence call`

### Card 10: extend the coherence table for two rows

- **Context:**
  - `internal/loomengine/coherence.go`
  - `internal/loomengine/report.go`
  - `internal/shedengine/status.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/loomengine/coherence_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Thread the two new arguments through the single `checkCoherence(shed, product)` call site in `TestCheckCoherence`'s subtest body.
  Give the test table two new optional fields so a case can override them, defaulting to loom's own row: expected name `"Loom-Preflight"` and tolerated set `[]string{"Preflight", "Loom-Preflight"}`.
  Retarget `validFreshShed`'s `CurrentProducer` from `"Preflight"` to `"Loom-Preflight"` and update its doc comment, since the baseline this table mutates from is now the post-row-1 shape `Shed` itself persists.
  Rename the existing `CurrentProducerNotPreflight` case to reflect the told expectation and keep its `CheckSeedIncoherent` assertion.
  Add these cases: `current_producer` equal to the told expected name passes; `current_producer` equal to the generic row's name `"Preflight"` now **fails** with `CheckSeedIncoherent` (it is the previous row, not the expected one); a history containing only a `"Preflight"` `Done` entry passes; a history containing only `"Loom-Preflight"` entries passes; a history mixing both passes; a history naming any third producer (e.g. `"Discussion-Write"`) yields `CheckHalfFinished`.
  Keep every existing mandatory-field, state, outcome-validity, RFC3339, `start_sha` and `pause_requested` case asserting exactly what it asserts today, adjusting only the producer names inside their fixtures so they remain consistent with the new baseline.
  Update the two long explanatory comments on the retry-deadlock cases so they name both tolerated rows rather than only row 1.
  This file stays untagged and pure — `checkCoherence` performs no I/O, so no fixture, no `t.TempDir`, and none of the substrings named in the `untagged-tests-carry-no-spawn-token` Shared Decision.
- **Commit:** `test(loomengine): cover the two-row coherence rules in the Tier-1 table`

### Card 11: Tier-1 CheckSeed suite

- **Context:**
  - `internal/loomengine/seed.go`
  - `internal/loomengine/report.go`
  - `internal/loomengine/status.go`
  - `internal/loomengine/coherence_test.go`
  - `internal/state/state.go`
  - `internal/shedengine/status.go`
  - `docs/benchmarks/running-tests.md`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/seed_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create an **untagged** (Tier-1) `package loomengine` test file driving `CheckSeed` directly over `t.TempDir` paths, with a local helper that marshals a `Status` product into a `shedengine.Status` shell and writes it with `state.WriteJSON`.
  Cover these scenarios, each asserting on the returned `Report` rather than on an error unless stated: a status file that does not exist → `Report.OK` false carrying `CheckSeedMissing`, nil error; a file whose bytes are not valid JSON → `CheckSeedIncoherent`, nil error; a file carrying an unknown top-level field → `CheckSeedIncoherent`, nil error; a file whose `product` is valid JSON but does not unmarshal as loom's `Status` shape (e.g. `"product": {"slug": 7}`) → `CheckSeedIncoherent` whose reason contains `product does not decode as loom's status shape`, nil error; a coherent post-row-1 seed (`current_producer` equal to the told expected name, `state` running, one `Done` history entry naming a tolerated producer with an RFC3339 UTC timestamp, product carrying non-empty `slug`/`parent` and a null `start_sha`) → `Report.OK` true with no failures.
  Add the `MkdirAll`-guard regression case, which has no Tier-2 equivalent today: point `statusLockPath` at a path several directory levels deep that does not exist yet, alongside a coherent status file, and assert `CheckSeed` returns a determined verdict with a nil error rather than escalating to an infra error — the guard is what stops a worktree with no ephemeral tree from breaking the report-not-error contract.
  Add one case proving the told names are genuinely told rather than defaulted: the same coherent file checked against a different `expectedProducer` yields `CheckSeedIncoherent`.
  Use plain string literals for the told names — `internal/loomengine` must not import `internal/loomshed`, which would be an import cycle.
  This file must contain none of the substrings named in the `untagged-tests-carry-no-spawn-token` Shared Decision.
- **Commit:** `test(loomengine): add the Tier-1 CheckSeed suite`

## Batch Tests

Verified by the batch's three-command `verify:` chain.

Tier 1 covers the two files this batch adds or rewrites: `internal/loomengine/seed_test.go` (six `CheckSeed` scenarios plus the `MkdirAll`-guard regression and the told-name proof) and `internal/loomengine/coherence_test.go` (the two-row rule table).
It also re-runs `cmd/lyx/tierpurity_test.go`, which is the machine check that both new untagged files stay spawn-free.

Tier 2 is the load-bearing half of this batch's gate even though it adds nothing: `internal/loomengine/preflight_integration_test.go` still drives `Preflight`/`checkResolved` end-to-end across all four preconditions, and it is what proves card 7's parameterization and card 9's call-site change are genuinely behaviour-preserving for row 1.
If `checkCoherence`'s rewritten rules drifted from today's semantics, `TestPreflight_HealthyPairAndSeed` and `TestPreflight_SeedHalfFinished` are where it would surface.

`go vet -tags smoke ./internal/loomcli` is unaffected by this batch and is run only to keep the three-command chain uniform across batches.
