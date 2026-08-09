```yaml
slug: batcher-standalone-split
title: "batcher: split out of webster into a standalone configreg module with its own batcher.yaml"
depends_on: ["plan-format-drop-v3-suffix"]
brief: |
  Extract internal/batcher out of webster as a standalone configreg-registered module with its own batcher.yaml, move both live batcher.Select call sites onto batcher's own entry point, and have reconcile report — never honour — a leftover webster.yaml batcher key in an existing worktree.
```

# batcher: split out of webster into a standalone configreg module with its own batcher.yaml

## Why

`Batchifier` is a `Shed`-level producer, position 8 in `loom`'s list, between `Plan-Review` and `Webster`, so batching is no longer webster-internal execution policy.

Two-step consequence: extract `batcher` as a standalone module now, and absorb the `Batchifier` producer into `loom` via `Shed` later, when the batchifier choice becomes part of `loom`'s producer-list configuration.

The key cannot go straight to `loom.yaml` today: both live `batcher.Select` call sites are webster's, and `Shed` — the thing that would own a `loom.yaml` batchifier key — does not exist, so making webster read `loom.yaml` would either break standalone `lyx webster run` or couple two modules' configs for no live benefit.

**Rejected alternatives:**

- Dropping `Batchifier` from `loom`'s producer list to preserve the shipped `internal/batcher` framing — would subordinate the newest decision to older doc text.
- Surfacing `Batchifier`'s position as unresolved — the user already resolved it.
- Moving the key straight to `loom.yaml` now — unworkable, since no `loom.yaml` reader exists.
- Leaving it in `webster.yaml` — contradicts the split.
- A `loom.yaml` key with `webster.yaml` fallback — a transition mechanism with no transition to serve.

## What needs to happen

1. **Module shape.**
   "Standalone" means a `configreg`-registered config module, not a `lyx batcher` command, because a batchifier has no user-facing verb.

   Two consequences, since the opposite was briefly assumed:
   - The CLI / Cobra Invariant does not apply, because nothing is registered on the cobra root.
   - Sandbox Suite Coverage does not apply, because `cmd/lyx/sandbox_coverage_test.go:38–47` enumerates `newRoot().Commands()`, i.e. cobra registration rather than `configreg`, so adding a `**Covers:** batcher` tag would actively fail that test's drift assert.

2. **Config wiring.**
   `internal/batcher` gains the loading of `batcher.yaml` and exposes an entry point returning the active `Batcher` — the natural extension of the `Select`-by-name seam it already has.
   `websterengine.Config.Batcher` is therefore removed, not retained.

   The earlier "retained" note is superseded: retaining the field would leave webster holding a yaml key it no longer owns, and populating it from `batcher.yaml` would be exactly the cross-module config coupling the `loom.yaml` option was rejected for.

3. **The inventory.**
   Both call sites move, not one:
   - `internal/websterengine/runlevel.go:332` — `batcher.Select(deps.Config.Batcher)` becomes a call into `batcher`'s own entry point.
   - `internal/webstercli/cli.go:160` — the `PersistentPreRunE` fail-fast gate, whose behaviour is preserved and only whose source changes.
   - `internal/websterengine/template.yaml:3` — where the `batcher: ""` key physically lives, with its explanatory comment.
   - `internal/webstercli/verbs_test.go:221–223` plus the whole gate-test pair at `:696–732` — `TestPersistentPreRunE_UnknownBatcherFailsFast` and `TestPersistentPreRunE_DefaultBatcherResolves`.
     Both string-replace `batcher: ""` out of the template, so both break the moment the key leaves it — the pair is taken whole, not just the `:697` comment.
   - `internal/websterengine/doc.go:12` and `:25–27`.
   - `docs/overview.md:267`.
   - `internal/websterengine/config_test.go:125`'s `cfg.Batcher == "identity"` assertion, which moves into `internal/batcher`'s own tests with the field.

4. **Migration.**
   Reconcile reports a leftover `webster.yaml` `batcher:` value once, as an orphaned key, and otherwise ignores it — never silently dropped, never read.

   Rationale: honouring it would reinstate the cross-module read this split exists to remove, invisibly, so two worktrees with identical `batcher.yaml` files could batch differently.

   **Rejected alternatives:**
   - Honouring the old key as a fallback.
   - Silently ignoring it.

5. **Doc amendments.**
   - `internal/batcher/doc.go`'s package comment must stop saying batching is "100% webster's own execution-policy decision" and instead say it is a standalone step webster consumes today and `Shed` will drive as producer #8 once built.
   - `CONSTRAINTS.md`'s Batcher Registry+Config Invariant, both the ownership claim and the `webster.yaml` config-key pin.
   - `docs/overview.md:271`'s batcher module-table entry, which pins the key to `webster.yaml`.
   - The renamed `plan-format.md`'s "Batch is gone / the card is the unit" section, where the card stays the plan's unit but the "entirely internal to webster" framing goes.

## Scope

This task does not change the `Batcher` interface, the registry, or `Select` itself — those stay untouched.
What changes is where the name fed to `Select` is configured, plus the module's registration and docs.

This task does not edit `loom.md`; row 8 of `loom.md`'s producer table is task E's, written after this task lands.

## Sequencing

`depends_on: plan-format-drop-v3-suffix` — this task edits the renamed file.

Task E depends on this task in turn, since `loom.md:56`'s row 8 must reflect whatever this task lands.

## Acceptance

The config relocation is the only behavioural change.
TDD candidates:

- A test asserting the active batchifier resolves from `batcher.yaml` through `batcher`'s own entry point.
- A `configreg` test asserting `batcher` appears in the module list, mirroring `internal/configreg/configreg_test.go:17`'s existing shape.
- A migration test covering an existing worktree whose `webster.yaml` still carries a `batcher:` value.

`internal/batcher`'s existing registry and `Select` tests must pass untouched, since that is the evidence that only the configuration source moved and not the batching itself.

Name the Cwd Resolution Invariant as relevant if the config-key move touches path resolution.
