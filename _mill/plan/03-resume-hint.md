# Batch: resume-hint

```yaml
task: Investigate the unexplained lyx mux server crash
batch: resume-hint
number: 3
cards: 2
verify: go test ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...
depends-on: [2]
```

## Batch Scope

Make a dead server cheap to notice: enrich the shared no-session error in
`requireSessionLocked` so every verb (status, add, remove, and attach's pre-flight)
points the operator at `lyx mux resume` when persisted strands prove there is state
worth rebuilding — then sweep every verbatim quote of the old error string (tests,
comments, docs) and update exactly the ones whose scenario actually produces the new
message. Depends on batch 2 only because both edit `internal/muxengine/lifecycle.go`
(shared-file serialization, no semantic coupling). External interface: the enriched
error string is operator-facing; nothing downstream parses it (the JSON envelope's
`error` field carries it opaquely).

## Cards

### Card 7: Enriched no-session error in requireSessionLocked

- **Context:**
  - `internal/muxengine/state.go`
  - `internal/muxengine/spawn.go`
  - `internal/muxengine/strand.go`
- **Edits:**
  - `internal/muxengine/lifecycle.go`
  - `internal/muxengine/lifecycle_test.go`
  - `internal/muxcli/cli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add pure helper `noSessionMessage(strandCount int) string` to
  `lifecycle.go` near `requireSessionLocked`: count ≤ 0 returns today's exact
  `no mux session; run "lyx mux up"`; count ≥ 1 returns
  `no mux session (N strands persisted); run "lyx mux resume" to rebuild, or "lyx mux up" for a bare substrate`
  with N substituted (Shared Decision enriched-no-session-error). In
  `requireSessionLocked`, when `hasSession` reports the session absent, load persisted
  state via `LoadState(e.layout.DotLyxDir())`; on load error or nil state use count 0
  (never mask the no-session signal with a state-read failure — the helper's doc
  comment states this); return `fmt.Errorf` over `noSessionMessage(len(st.Strands))`.
  Update `requireSessionLocked`'s doc comment: it now also tells the operator whether
  `resume` would rebuild anything, and why `resume` (the replaying verb) is the right
  pointer after an unexplained server death while `up` is substrate-only. Add an
  untagged table-driven test for `noSessionMessage` (0, 1, 3 strands) in
  `lifecycle_test.go`. In `cli_integration_test.go`, add one integration test:
  seed the paired fixture like the existing before-up tests, additionally write a
  `.lyx/mux.json` via `muxengine.SaveState` carrying two strand records (import shape:
  see `state.go`), run `status` (or `add`) before `up`, and assert the JSON envelope's
  `error` equals the enriched message with `(2 strands persisted)`. Follow the
  existing before-up tests' fixture and assertion style.
- **Commit:** `Suggest lyx mux resume in the no-session error when strands are persisted`

### Card 8: Sweep verbatim quotes of the old no-session error

- **Context:**
  - `internal/builderengine/spawn_test.go`
  - `internal/muxengine/lifecycle.go`
  - `docs/research/linux-portability-survey.md`
  - `tools/sandbox/SANDBOX-MUX-SUITE.md`
  - `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
- **Edits:**
  - `internal/muxengine/strand.go`
  - `internal/muxcli/attach.go`
  - `internal/muxcli/cli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run `grep -rn "no mux session" --include="*.go" --include="*.md" .`
  (excluding `_mill/`) and disposition every hit; the enumeration below is the known
  starting point, not proven exhaustive (discussion.md § resume-hint note):
  1. `internal/muxengine/strand.go` doc comments (~lines 308, 351, 411) — reword the
     quoted error to reference "the friendly no-session error (see
     `requireSessionLocked`/`noSessionMessage`)" instead of pinning the exact old
     string, so the prose survives future message tweaks.
  2. `internal/muxcli/attach.go` (~line 53) — same treatment for the pre-flight
     comment.
  3. `internal/muxcli/cli_integration_test.go` add/remove-before-up tests (~lines 83,
     112) — their fixtures persist NO strands (fresh `CopyPaired`, no mux.json), so
     they correctly assert today's unchanged text; verify that and leave the
     assertions, adding a one-line comment noting the zero-strand case keeps the short
     message (the ≥1-strand case is covered by card 7's new test).
  4. `internal/builderengine/spawn_test.go` (~line 586) — synthetic
     `errors.New("no mux session")` fixture data, not an assertion on mux's message;
     verify and leave unchanged.
  5. `docs/research/linux-portability-survey.md`, `tools/sandbox/SANDBOX-MUX-SUITE.md`
     (M1), `tools/sandbox/SANDBOX-BUILDER-SUITE.md` — documented no-strand scenarios;
     verify each describes a fresh-state flow (it does per the discussion review) and
     leave unchanged.
  Any grep hit outside this list: update it if its scenario persists strands, leave it
  with a note in the commit body otherwise.
- **Commit:** `Reconcile prose and tests quoting the mux no-session error`

## Batch Tests

`verify:` runs the mux module's untagged units (including the new `noSessionMessage`
table test) plus `cmd/lyx` guards. The integration-tagged before-up tests (old and new)
are exercised once in this batch via
`go test -tags integration ./internal/muxcli/ -run FriendlyError -v -count=1` — plus
the new enriched-message test's own `-run` name — and the results reported; they stay
out of `verify:` because integration fixtures spawn git and are slower per round.
