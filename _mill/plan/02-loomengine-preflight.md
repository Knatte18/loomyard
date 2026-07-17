# Batch: loomengine-preflight

```yaml
task: 'loom: Preflight phase (precondition validation)'
batch: loomengine-preflight
number: 2
cards: 5
verify: go test -tags integration ./internal/loomengine/ && go test -run 'TestTierPurity|TestHermeticGitEnv' ./cmd/lyx/
depends-on: [1]
```

## Batch Scope

The new `internal/loomengine` package: the canonical `_lyx/status.json` Go type, the pure
coherence validator, the `Report`/`Failure`/`CheckID` result types, and the `Preflight`
orchestrator that runs the four checks over git/filesystem state. It consumes batch 1's helpers
(`state.ReadJSONStrict`, `hubgeometry.LoomStatusFile`/`LoomStatusLock`, `warpengine.HostClean`) and
the existing `warpengine.PairInSync` / `hubgeometry.Resolve`. This is one batch because it is a
single cohesive new package; the cards below are its files. No external module consumes this yet —
the future phase-machine skeleton (a later task) will be the first caller.

## Cards

### Card 4: status.json Go type

- **Context:**
  - `docs/reference/status-schema.md`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/status.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `loomengine` with the canonical status type in `status.go`.
  Define `type Status struct` with json tags exactly per `status-schema.md`: `Slug string
  json:"slug"`, `Parent string json:"parent"`, `Phase string json:"phase"`, `Stage string
  json:"stage"`, `Narration string json:"narration"`, `History []HistoryEntry json:"history"`,
  `StartSha *string json:"start_sha"`, `PauseRequested bool json:"pause_requested"`, `NextAction
  *string json:"next_action"`. Define `type HistoryEntry struct` with `Phase string
  json:"phase"`, `Outcome string json:"outcome"`, `BouncedTo *string json:"bounced_to,omitempty"`,
  `Ts string json:"ts"`. Per field-presence-and-nullability: nullable fields (`StartSha`,
  `NextAction`, `HistoryEntry.BouncedTo`) are `*string` (nil ⇔ JSON null/absent); mandatory
  strings and `PauseRequested`/`History` are value types. No `schema_version` field. Godoc the
  type, pointing at `docs/reference/status-schema.md` as the pinned contract.
- **Commit:** `feat(loomengine): add canonical status.json Status type`

### Card 5: Report / Failure / CheckID result types

- **Context:** none
- **Edits:** none
- **Creates:**
  - `internal/loomengine/report.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `report.go` define `type CheckID string` with a const block for the closed
  set: `CheckGeometry = "geometry"`, `CheckWorktreeRoot = "worktree-root"`, `CheckWorktreeClean =
  "worktree-clean"`, `CheckWeftPairing = "weft-pairing"`, `CheckWeftSync = "weft-sync"`,
  `CheckJunction = "junction"`, `CheckSeedMissing = "seed-missing"`, `CheckSeedUnreadable =
  "seed-unreadable"`, `CheckSeedIncoherent = "seed-incoherent"`, `CheckHalfFinished =
  "half-finished"`. Define `type Failure struct { Check CheckID; Reason string }` and
  `type Report struct { OK bool; Failures []Failure }`. Add a small helper the orchestrator uses
  to append a failure and keep `OK` consistent (e.g. a method or a local builder), such that
  `OK == (len(Failures) == 0)`. Godoc each type per report-shape.
- **Commit:** `feat(loomengine): add Report/Failure/CheckID result types`

### Card 6: coherence validator (pure, TDD)

- **Context:**
  - `docs/reference/status-schema.md`
  - `internal/loomengine/status.go`
  - `internal/loomengine/report.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/coherence.go`
  - `internal/loomengine/coherence_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `coherence.go` add a pure `func checkCoherence(s Status) []Failure` (no I/O)
  implementing `status-schema.md`'s validation checklist plus the fresh-start invariants:
  (1) mandatory-string non-empty — `Slug`/`Parent`/`Phase`/`Stage`/`Narration`, any empty →
  a `CheckSeedIncoherent` failure naming the field (this is how a *missing* mandatory string is
  detected, since strict decode zero-fills absent fields); (2) enum validity — `Phase` ∈
  `{preflight,discussion,plan,builder,raddle,finalize,done}`, `Stage` ∈ `{produce,gate}`, each
  `History[].Outcome` ∈ `{approved,stuck}` → `CheckSeedIncoherent`; (3) `History[].BouncedTo`
  non-nil only when `Outcome == "stuck"` → `CheckSeedIncoherent`; (4) every timestamp field
  (`History[].Ts`) is RFC3339 UTC (parse with `time.Parse(time.RFC3339, ...)` and require a `Z`
  / zero offset) → `CheckSeedIncoherent`; (5) fresh-start invariants — `len(History) != 0` OR
  `StartSha != nil` OR `NextAction != nil` OR `PauseRequested` → a `CheckHalfFinished` failure.
  The nullable/bool fields are NOT presence-checked (their zero value is valid; per
  field-presence-and-nullability). Return all failures found (collect, do not short-circuit).
  `coherence_test.go` is untagged (Tier 1, in-memory `Status` values, no spawn, no git) and is a
  TDD driver: table tests for each rule — valid seed → nil; empty/absent mandatory string →
  `CheckSeedIncoherent`; bad enum; `bounced_to` without stuck; non-RFC3339 / non-UTC ts;
  non-empty history / set `start_sha` / set `next_action` / `pause_requested:true` →
  `CheckHalfFinished`.
- **Commit:** `feat(loomengine): add pure status.json coherence validator`

### Card 7: Preflight orchestrator + caller-contract godoc

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/warpengine/drift.go`
  - `internal/warpengine/hostclean.go`
  - `internal/state/state.go`
  - `internal/loomengine/status.go`
  - `internal/loomengine/report.go`
  - `internal/loomengine/coherence.go`
  - `docs/reference/status-schema.md`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/preflight.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `preflight.go` add the public `func Preflight() (Report, error)` and an
  unexported `func checkResolved(l *hubgeometry.Layout) (Report, error)` (per Shared Decision
  "Preflight owns Getwd+Resolve"). `Preflight`: `cwd, err := hubgeometry.Getwd()`; on err → return
  `Report{}, err` (infra). `l, err := hubgeometry.Resolve(cwd)`; if `errors.Is(err,
  hubgeometry.ErrNotAGitRepo)` → return `Report{OK:false, Failures:[{CheckGeometry, ...}]}, nil`
  (short-circuit); other non-nil err → return `Report{}, err`. Then `return checkResolved(l)`.
  `checkResolved` runs the four checks:
  (check 1b) if `l.Prime == ""` → `CheckGeometry` short-circuit; if `l.RelPath != "."` →
  `CheckWorktreeRoot` short-circuit (both return immediately, single failure — see
  check-ordering-and-collection / at-worktree-root);
  then **collect** (do not short-circuit) into one `Report`:
  (check 2) `warpengine.HostClean(l)` — `err != nil` → return `Report{}, err`; else
  `!clean` → append `{CheckWorktreeClean, reason}`;
  (check 3) `os.Stat(l.WeftWorktree())` — if `os.IsNotExist` → append `{CheckWeftPairing, "weft
  not paired"}` and record "check 3 failed"; other stat err → return `Report{}, err`; if present,
  `warpengine.PairInSync(l)` — `err != nil` → return `Report{}, err`; on `!ok` classify `reason`
  by prefix (`"host on "` → `CheckWeftSync`; `"junction"` → `CheckJunction`; unknown →
  `CheckWeftSync`) and record whether the failure was `CheckWeftPairing`/`CheckJunction`;
  (check 4) read + validate the seed via `l.LoomStatusFile()` and `l.LoomStatusLock()`:
  `st, err := os.Stat(l.LoomStatusFile())`. Classification is **gated on check 3's outcome**
  (strict-read-mechanism → Error classification): if check 3 produced a `CheckJunction` OR
  `CheckWeftPairing` failure, then ANY seed stat failure — `os.IsNotExist` included — is appended
  as `{CheckSeedUnreadable, "unreadable, see check 3"}`, never `seed-missing`; else (check 3
  healthy) `os.IsNotExist` → `{CheckSeedMissing, ...}`, other stat err → `{CheckSeedUnreadable,
  ...}`. When the stat succeeds, `s, found, rerr := state.ReadJSONStrict[Status](l.LoomStatusFile(),
  l.LoomStatusLock())`; classify `rerr` by the single rule: `errors.Is(rerr, state.ErrDecode)` →
  append `{CheckSeedIncoherent, ...}`; every other non-nil `rerr` (`state.ErrRead`, a lock-acquire
  failure, anything else) → return `Report{}, rerr` (escalate); `!found` after a good stat is a
  TOCTOU race (`ReadJSONStrict` returns `(zero,false,nil)` on `IsNotExist`, so `rerr` is **nil**
  here) — return a **synthesized non-nil error** (e.g. `fmt.Errorf("loomengine: seed vanished
  between stat and read: %s", l.LoomStatusFile())`), never `Report{}, nil`, so both the escalate
  contract and `OK == (len(Failures)==0)` hold. On a clean parse, append `checkCoherence(s)`'s
  failures.
  Finally `Report.OK = len(Failures) == 0`. Godoc `Preflight` with the **caller-contract**
  required by preflight-invocation-model, verbatim intent: "Callers MUST NOT invoke Preflight
  except when the task is at the fresh/preflight stage. Invoking it on an already-advanced task
  (non-empty history, set start_sha, …) is a caller error that will be reported as a half-finished
  precondition failure, not diagnosed as misuse, because Preflight is a stateless validator." Also
  add package doc (a `doc.go`-style comment at the top of `preflight.go`, or in `status.go`)
  carrying the same caller precondition, since `internal/loomengine`'s package doc is where a
  future phase-machine implementer looks.
- **Commit:** `feat(loomengine): add Preflight orchestrator over the four preconditions`

### Card 8: Preflight integration tests + hermetic TestMain

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/warpengine/testmain_test.go`
  - `internal/warpengine/drift.go`
  - `internal/warpengine/junction.go`
  - `internal/warpengine/checkout_test.go`
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/report.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/testmain_test.go`
  - `internal/loomengine/preflight_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `testmain_test.go` (package `loomengine`) defines
  `func TestMain(m *testing.M) { lyxtest.HermeticGitEnv(); os.Exit(m.Run()) }`, mirroring
  `internal/warpengine/testmain_test.go` (Hermetic Git Test Environment Invariant).
  `preflight_integration_test.go` is `//go:build integration`-tagged (it spawns git via fixtures).
  Build a healthy paired host+weft fixture with `lyxtest.CopyPaired(t)` (gives `Hub`, `WeftPrime`,
  `Layout`). **`CopyPaired` does NOT wire the `_lyx` junction**, so first call
  `warpengine.WireJunctions(fixture.Layout, slug)` (slug = `filepath.Base(fixture.Layout.WorktreeRoot)`)
  **before** creating any `_lyx` — `WireJunctions` enforces the host-pristine invariant and errors
  if a real host `_lyx` already exists — then seed the valid `_lyx/status.json` at
  `fixture.Layout.LoomStatusFile()` (which now resolves through the wired junction to the weft
  `_lyx`). Mirror the wire-then-operate pattern in `internal/warpengine/checkout_test.go` /
  `cleanup_test.go`. Fresh seed: `phase:"discussion"`/`"preflight"`, `stage:"produce"`, empty
  history, null `start_sha`/`next_action`, `pause_requested:false`, a non-empty narration. Assert
  the anchor case (`checkResolved(fixture.Layout)`, injecting the Layout for isolation) returns
  `Report.OK`. Then mutate to trip each scenario, asserting the exact `CheckID` set each yields:
  not-a-git-repo (run from a non-repo temp dir → `Preflight()` → `geometry`, no other failures);
  subdirectory invocation (`RelPath != "."` → `worktree-root`, short-circuit); empty `Prime` →
  `geometry`; host dirty (tracked-modified, staged, and untracked-only) → `worktree-clean`; weft
  worktree removed → `weft-pairing`; host/weft on different branches → `weft-sync`; junction
  broken (remove/re-point the wired junction) → seed stat also `IsNotExist`, assert `seed-unreadable`
  (NOT `seed-missing`) appears alongside `junction`; weft removed → seed `IsNotExist` classified
  `seed-unreadable` alongside `weft-pairing`; seed missing while junction healthy → `seed-missing`;
  seed with an unknown field → `seed-incoherent`; seed with non-empty history / set `start_sha` →
  `half-finished`; multiple simultaneous failures all collected; Prime worktree with a healthy
  pair+seed → `Report.OK` (run-in-existing-or-prime-worktree). **The `not-a-git-repo` and
  `subdirectory` scenarios exercise the public `Preflight()` (which reads the process cwd via
  `hubgeometry.Getwd`), so they `os.Chdir` into the target dir and restore the original cwd via
  `defer`/`t.Cleanup`; because `os.Chdir` is process-global these two scenarios MUST NOT run under
  `t.Parallel()`.** Every other scenario drives `checkResolved(l)` with an injected Layout and
  needs no chdir. Use `lyxtest.MustRun` for git
  fixture mutations, mirroring existing warpengine integration tests. Assert only on the `CheckID`
  set per scenario, not exact `Reason` strings.
- **Commit:** `test(loomengine): add integration tests for Preflight across all preconditions`

## Batch Tests

`verify: go test -tags integration ./internal/loomengine/ && go test -run 'TestTierPurity|TestHermeticGitEnv' ./cmd/lyx/`

- The first invocation runs the whole `internal/loomengine` package with the `integration` tag,
  which builds **both** the untagged Tier-1 tests (Card 5's `coherence_test.go`, plus any
  status-type tests) and Card 8's integration tests — a single command covers both tiers for the
  new package.
- The second invocation runs only the two module-wide enforcement guards that Card 8's new test
  files could trip: `TestTierPurity_UntaggedTestsSpawnNothing` (proves `coherence_test.go` stays
  spawn-free and `preflight_integration_test.go` is correctly tagged) and
  `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` (proves the new `testmain_test.go` satisfies
  the hermetic requirement). These guards walk the whole module tree but run fast (they only scan
  `*_test.go` sources), so scoping to them via `-run` avoids `cmd/lyx`'s slow e2e suite while still
  catching the invariant violations this batch is most at risk of introducing.
