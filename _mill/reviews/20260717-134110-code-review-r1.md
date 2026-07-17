MILL_REVIEW_BEGIN
# Review: Master Builder: new, parallel fork-based implementation module — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-17
```

## Summary

Reviewed all nine batch plans and the full implementation manifest (websterengine,
webstercli, the shuttleengine/claudeengine/builderengine seam extensions, cmd/lyx
registration, sandbox suite, and the docs/rename sweep). Traced every batch's
external-interface promise into the next batch's actual consumer:

- Batch 1 (seam-extensions): `ForkAudit`'s three new fields, `AuditForksIncremental`,
  `ModelSwitchSequence`, `Runner.Inject`, and the four exported builderengine helpers
  all match their cards exactly, including the five package-local fake-Engine stub
  additions across builderengine/buildercli/shuttlecli test files (verified all five
  files carry both new stub methods).
- Batch 2 (webster-foundation): hubgeometry `WebsterDir`/`WebsterReportsDir`/
  `WebsterPromptsDir`, `websterengine.Config`/`LoadConfig`/`ConfigTemplate` (template.yaml
  values match the card verbatim), `Role`/`ResolveRoles`, `State`/`BatchState` schema
  (field-for-field as specified, including the flat `SeenForkTranscripts []string` the
  plan settled on in place of discussion.md's earlier `AttributedForkTranscripts` map —
  a refinement consistently applied across every consuming batch), and configreg
  registration all present and consistent.
- Batch 3 (webster-audit-policy): `CheckFork`/`CheckParent`/`ForkWarnings`/
  `NewTranscripts`/`SettleRetry`/`ClassifyAttribution` implement the pinned
  transcript-count-before-report-presence check order exactly, weft-reference pattern
  built from `layout.WeftWorktree()`/`hubgeometry.WeftSuffix` (no literal geometry
  token), and `audit_test.go` table-drives every violation/allowed class from Card 14.
- Batch 4 (webster-templates): both templates carry every required literal statement
  (digest-field bullets, outcome-key bullets, the four NEVER bans, the bracket
  sequence) and `template_test.go` pins all of them plus `stencil.Fill` marker
  round-trips.
- Batch 5 (bracket-verbs): `RestartChain`, `BeginBatch` (gates, idempotent model
  assertion, chain-anchor first-write-wins, prompt render/write, state update), and
  `RecordBatch` (settle-retry, attribution-before-report-presence, unconditional
  attribution advance, digest persistence) all match their cards; `beginbatch_test.go`
  exercises pause/fingerprint/model-idempotence/prev-digest/chain-anchor/unknown-role.
- Batch 6 (recover-batch): spawn-or-attach decision, `archiveStaleReport`,
  builder-template cold orientation, elapsed-since-spawn measured from `SpawnedAt`
  across re-entrant calls, and the done/stuck/dead substrate-cleanup parity rules all
  match the card and discussion's re-entrant bounded long-poll design.
- Batch 7 (run-level): `Run`'s gate order (lock → validate incl. zero-batch pre-flight
  → state/reclaim → fingerprint/--fresh → pause-clear → stale-archive → spawn),
  `mapMasterDone`'s summary-required-on-done / audit-cross-check, and the three typed
  Master*Error mappings all match; `doc.go`'s reuse inventory (Card 32) is present.
- Batch 8 (webstercli-registration): `websterCLI`'s three adapted seams, the
  `WorktreeRoot: c.layout.Cwd` convention (matches buildercli's own established
  pattern exactly — not a bug), `websterWeftPathspec`'s three exclusions
  (`*.lock`, pause flag, `*/webster/prompts/*`), all seven verbs wired with the
  correct weft-commit points, and `cmd/lyx` registration (main.go, helptree_test.go)
  confirmed. The Card 38 rewording of the three pre-existing tier-purity-tripping
  test-file comments (`config_test.go`/`state_test.go`/`template_test.go`) was
  verified done — none of the three now contain the banned raw substrings.
- Batch 9 (sandbox-and-docs): `SANDBOX-WEBSTER-SUITE.md` W1/W2 fully authored per
  spec (including W2's three separately-verdicted assertions), `suite.go`/`main.go`
  wiring and `sandbox-webster-suite.cmd` (byte-identical to the builder launcher
  except the subcommand token) all present; `builder-contract.md`/`overview.md`
  contract deltas and the roadmap/long-term-ideas rename sweep are complete — a
  repo-wide grep for "Master Builder" outside `_mill/` (which intentionally keeps
  the task slug/history) turns up only the one deliberate "né \"Master Builder\""
  cross-reference the card itself specifies.

No out-of-plan files were found; every file in the batch manifests' Context/
Edits/Creates lists is accounted for. No global utility duplication (webster
consistently imports rather than re-implements builderengine's shared helpers,
per the reuse-by-import-never-copy decision, with the two documented,
intentional exceptions — webster's own `RestartChain`/archive-timestamp copies
— explicitly justified by the state-type/unexported-constant constraints the
plan itself calls out). No constraint violations found: Hub Geometry, CLI/Cobra,
Shuttle Provider-Seam, Weft Git, Sandbox Coverage, Test Tier Purity, and Hermetic
Git invariants are all satisfied by the code as written.

## Verdict

APPROVE
Implementation matches the plan end-to-end across all nine batches with no
blocking defects found.
MILL_REVIEW_END
