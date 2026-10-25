# PATTERN-shuttle-provider-seam

Provider specifics live only under `internal/shuttleengine/<engine>`, and only `internal/shuttleengines` constructs or imports a concrete engine.

- `shuttleengine` and `reedengine` never reference a concrete engine's specifics, and `shuttleengine` never imports a concrete engine.
- Only `internal/shuttleengines` imports or constructs a concrete engine; the scan in `internal/shuttleengine`'s `seam_enforcement_test.go` covers production files.
- An engine's config module reaches `configreg` only through `internal/shuttleengines`, which lists each registered engine's module for it.
- An engine's name, a registry key such as `shuttle.yaml`'s `engine: claude` or a `modelspec` engine name, is not a provider specific and may appear outside its directory.
- An engine's scrub keys, the environment keys reed's server spawn strips, reach reed only through `shuttleengines.EnvScrub()`, which every production `reedengine.New` caller passes.
- reed's strand provider tag is opaque: shuttle sets it to the run's engine name, reed stores it and never interprets it.
  An engine's session namer reaches reed's watchdog only through `shuttleengines.SessionNamers()`, keyed by that tag.
- An engine's record-reading capabilities reach a reader outside the engines only through `shuttleengines.RecordReaders()`, as adapters that expose reading alone.
- A `*Run` issued by `Start` or `StartGated` has already resolved its provider's startup probe.
  No caller outside `internal/shuttleengine` probes provider readiness or plays startup-gate keys.

## Model choice mapping

A resolved model-spec reaches a spawn only as a `shuttleengine.ModelChoice` built by `shuttleengine.NewModelChoice`; no other production code reads a `modelspec.Resolved` field.
A later engine then takes a role by its model key alone, since the engine, model, effort and version travel together.

- Outside `internal/modelspec` and `NewModelChoice`, nothing reads `Resolved.Engine`, `Resolved.Model` or `Resolved.Params`.
- Every function that resolves a model key returns a `shuttleengine.ModelChoice`, or a list or map of them, built by `NewModelChoice`.
  The resolvers it binds are the seat resolvers, burler's per-round lists and the Bouncer pair, the driver, arbiter and tackler settings, treadle's profile and webster's roles.
- A timeout or spawn budget the same resolver also resolves stays beside the choice, never inside it.

### Enforcement

`TestModelChoiceMappingRule` in `internal/shuttleengine/modelchoicemapping_enforcement_test.go` walks the production files, skips `internal/modelspec/`, and parses each file that imports `internal/modelspec`.
It reports the file and line of a selector `x.Engine`, `x.Model` or `x.Params` where `x` is an identifier holding a `modelspec.Resolved`, bar inside `NewModelChoice` in `internal/shuttleengine/modelchoice.go`.
An identifier holds one when it is assigned from a call of a method named `Resolve`, is declared with the type `modelspec.Resolved`, or is the value of a range over a map or slice of it.

The scan is syntactic, so it is a tripwire and not a proof.
It cannot see a `Resolved` reached through a struct field or a call chain; review covers those.

## Shuttle stop

A shuttle run's strand is stopped from Go only through shuttle's stop verb, which settles the run's record first.
Removing the strand any other way leaves the record saying the run is live, and an attach probe or a Wait then reads a strand that is gone.

- `internal/shuttleengine` owns the stop verb; every other removal of a run's strand goes through it.
- `internal/reedengine` owns the strand removal itself, and `internal/reedcli` exposes it as reed's own remove verb.
- `internal/testkit/shuttlefake` is the test fake of the stop seam.
  Both it and `reedengine` declare `RemoveStrand` and select it nowhere, so the scan carries no entry for them; a selector in either is a finding until an entry with a reason is added.
- `internal/loomcli` removes the loom driver's strand and `internal/orchcli` the orch strand.
  Both are exempt because no attach probe or Wait reads their records.
- Enforced by `cmd/lyx/removestrand_test.go`, which fails on a selector expression selecting `RemoveStrand` in a package outside that list, so a method value handed to a wrapper is caught as well as a call.
- Bound: the scan sees the selector, not whose strand it removes.
  A call inside an allowed package on a run's strand that bypasses the stop verb is caught by review.

## Completion signal

Every code path in `internal/shuttleengine` that finalizes a NEGATIVE answer to "did this run finish" consults `allOutputFilesExist` over the run's `OutputFiles` first.

- The negative answers are `OutcomeDied`, `OutcomeTimeout`, `runOutcomeStopped`, a mechanism-failure `error`, and `verdictRespawnEligible`.
  `stopOutcome` in `stop.go` is the one site that chooses `runOutcomeStopped`, and it consults `allOutputFilesExist` first.
- The check is reached directly or through the three helpers that own it: `classifyDeadlineExpiry` and `finishedDespiteMechanismFailure` (`wait.go`), and `soleFinishedCandidate` (`attach.go`).
- A clock expiring, `reed` losing a strand, `reed.Status` erroring, the events file staying unreadable, `reed.json` being absent or undecodable each answer "has something gone wrong", never "did this run finish".
  The run's output files are its return value.
- Stated at length in `wait.go`'s own file doc comment, under the same heading.
  Six instances were fixed across crucible rounds `opus5-high-r4` (both deadlines), `fable5-high-r5` (both retry-exhausted caps), `fable5-xhigh-r6` (`Attach`'s `dispositionCandidate`) and `opus5-high-r7` (`Attach`'s three reed-state gates).

### The done-when list

A reed strand's done-when list is the one place reed learns a strand finished.
Only `shuttleengine` fills it, from an ungated run's `OutputFiles`;
a gated run's strand carries none, since its outputs existing does not mean its gate passed.
Only `reedengine`'s resume reads it, dropping a non-live strand whose list is non-empty and fully present instead of relaunching it.
Reed cannot know that a gate passed,
so a finished gated run's strand is still relaunched when shuttle's own teardown did not run,
and the relaunch types no prompt.

### Enforcement

A **tripwire, not a completeness proof** (`internal/shuttleengine/completionsignal_enforcement_test.go`).
Two AST scans pin the audited negative-verdict return sites and the audited `allOutputFilesExist` call sites in `wait.go`, `attach.go` and `stop.go`, so adding an exit or deleting a guard fails loudly and forces a human to confirm.
It is deliberately not a `shape.go`-style ledger, because that mechanism needs a closed value enum and these outcomes span three types in two files.
