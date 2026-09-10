# `loom` fixer report — round `sonnet5-xhigh-r8`

Companion to `_mill/loom-review-sonnet5-xhigh-r8.md`. Each finding fixed, verified, and committed
separately per "Commit per fix."

## F2 — `validate-discussion`/`validate-plan` now use the lightweight wiring path

**What changed:**
- `internal/loomcli/cli.go`: renamed `verbReadsStatusOnly` -> `verbUsesLightweightWiring`, extended
  its switch to include `"validate-discussion"`/`"validate-plan"` alongside `"status"`/`"pause"`.
- `internal/loomcli/wiring.go`: renamed `wireStatusPathsOnly` -> `wireLightweight`, extended it to
  also fill `c.env.AnchorPath`/`WorktreeRoot`/`DecisionRecordPath`/`SupportLogPath` — the only four
  `c.env` fields `validateDiscussionCmd`/`validatePlanCmd` read — from plain `location` accessors, no
  I/O, no config load. Doc comments on both functions updated to record the rationale and the round
  tag.
- `internal/loomcli/wiring_test.go`: `TestVerbReadsStatusOnly` -> `TestVerbUsesLightweightWiring`,
  `validate-discussion`/`validate-plan` flipped from `want: false` to `want: true`.
  `TestWireStatusPathsOnly_FillsTheStatusPathsWithoutLoadingAnyConfig` ->
  `TestWireLightweight_FillsThePathsWithoutLoadingAnyConfig`, extended with assertions that the four
  new `c.env` fields are correctly filled (this is the regression guard for F2 itself — it would fail
  if a future edit reverted the fill or wired the wrong accessor).
- `internal/loomcli/wiring_commitstatus_test.go`: `TestWireStatusPathsOnly_CommitStatusFilled` ->
  `TestWireLightweight_CommitStatusFilled`, call site updated to the renamed method.

**Why fixed this way:** matches the existing `status`/`pause` precedent exactly rather than inventing
a second lightweight-wiring mechanism — one function, one predicate, four verbs. The rename
(`*StatusPathsOnly` -> `*Lightweight`) was made because the function's own name would otherwise be
actively misleading once it covers two verbs that have nothing to do with "status."

**Verification:**
- `go build ./...` — clean.
- `go vet ./internal/loomcli/...` — clean.
- `go test -count=3 ./internal/loomcli/...` — all `ok`, including the two renamed/extended regression
  tests above.
- Live, against the real built binary and this round's own hand-built hub fixture
  (`fixture2/clonehere/warp-bare-HUB/loom-live-check`, real `lyx fabric clone`/`lyx fabric add`, real
  `_lyx/config/loom.yaml`): appended a malformed line to the real `loom.yaml`, then:
  - `lyx loom validate-plan` -> `{"ok":true,...}` — completely unaffected by the broken config
    (before this fix, this call would have hard-failed with a YAML parse error from `wire()`).
  - `lyx loom validate-discussion` -> `{"error":"loom: discussion is not yet valid","findings":[...
    "support log does not exist"]}` — reports the actual discussion state, not a config error.
  - `lyx loom drive` (a full-`wire()` verb, deliberately unchanged by this fix) -> still correctly
    hard-refuses with `"parse existing YAML: yaml: line 8: mapping values are not allowed..."`,
    confirming `run`/`drive`'s early-refusal behavior is untouched.
  - Restored the original `loom.yaml` afterward.

**Docs:** no design-doc update needed — F2 changes an internal CLI-wiring cost, not an observable
contract, invariant, or check; `wiring.go`'s own doc comments (the durable record for this decision)
were updated in the same change.

## F1 — `AuditForks`-failure orphan: documented as a new Accepted residual, not code-fixed

**Disposition: deferred, with reasons — see the review report's own F1 writeup for the full argument.**
Fixing this safely requires `RunState` to distinguish two currently-indistinguishable preserved-run
reasons (`KeepPane` debugging vs. `AuditForks`-diagnosis), which is a real design decision (a new
persisted field, or a `websterengine`-style dedicated reclaim step generalized to `BurlerProducer`),
not a mechanical fix a review-round agent should make unilaterally.

**What was done instead:** documented as a new named "Accepted residual" in
`manifest/designs/loom.md`'s "Crash recovery" section, matching the convention the two existing
entries there already use (see the fix below), so the gap is durably recorded rather than left to be
rediscovered.

_(doc fix landing next, see following commit)_

## Deferred items

- **F1's actual code fix** (a persisted "preserved-run reason" field, or a websterengine-style reclaim
  step generalized to `BurlerProducer`'s cluster-fan path) — deferred to an operator decision, per the
  reasons in the review report's F1 write-up. Documented as a new Accepted residual instead (see
  above), so the gap is visible rather than silently open.
- Nothing else deferred — every other finding (F2) is fixed in full above.

## Test commands run (cumulative, this round)

- `go build ./...`
- `go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/...`
- `go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/... ./cmd/lyx/...`
- `go test ./...` (full repo)
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1`
- `go test -tags integration ./internal/planglyph/... -run "RealDelta"`
- `go test -race ./internal/shuttleengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/loomengine/...`
- `go test -race -tags smoke ./internal/loomcli/... -run Smoke -v -count=1`
- `go test -count=3 ./internal/loomcli/...` (post-F2-fix)
- Live driving: real `lyx` binary built fresh, real hand-built hub fixture via `lyx fabric
  clone`/`lyx fabric add`, `lyx loom validate-plan`/`validate-discussion`/`status`/`drive` invoked
  directly, both pre- and post-fix.

## Changed files

- `internal/loomcli/cli.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomcli/wiring_commitstatus_test.go`
- `manifest/designs/loom.md` (F1's new Accepted-residual entry, next commit)
- `_mill/loom-review-sonnet5-xhigh-r8.md`, `_mill/loom-review-sonnet5-xhigh-r8-fixer-report.md`
