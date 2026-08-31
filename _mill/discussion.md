# Discussion: Surface merge-in-progress in fabric status

```yaml
task: Surface merge-in-progress in fabric status
slug: fabric-status-merge-in-progress
status: discussing
parent: main
```

## Problem

`fabricengine.Fabric.MergeInProgress()` (`internal/fabricengine/mergelifecycle.go:418`) already answers "does fabric have a merge parked on this pair", but nothing at the CLI boundary asks it.
An operator — or an agent driving `lyx fabric` — who wants to know whether a pair is mid-merge has no verb to ask;
they can only infer it from a *different* verb refusing with `*ErrMergeInProgress` (`internal/fabricengine/mergeerrors.go:150`), which means learning the state by attempting a mutation.

Why now: this is the sole item in `manifest/roadmap.md`'s **Planned** section — `fabric: surface merge-in-progress in \`lyx fabric status\``.
The Go API shipped with the merge lifecycle;
folding it into the `status` verb's output was deliberately deferred as a small follow-up, and this task is that follow-up.

## Scope

**In:**

- `internal/fabriccli/weft_verbs.go` — `statusCmd`'s `RunE` calls `fab.MergeInProgress()` and adds a `merge_in_progress` boolean to the success envelope.
- `statusCmd`'s `Long` help text — a short paragraph describing the new field.
- A new integration test in `internal/fabriccli` covering both the `false` and the `true` case over a real `hubforge` hub.
- `docs/overview.md` — the `fabric` bullet's sentence describing what `status` reports (currently line 210).
- `manifest/roadmap.md` — move the item from **Planned** to **Done**.

**Out:**

- `internal/fabricengine` — no engine change at all. `MergeInProgress()` is used exactly as it ships; its signature, semantics and doc comment are untouched.
- Foreign (plain-git) merge state. `MergeInProgress` deliberately never consults `foreignMergeStatePresent` (`mergelifecycle.go:414-417`); surfacing that separately is new semantics, not exposing the existing API, and is not in this task.
- Richer merge detail. The on-disk `mergeState` record (`internal/fabricengine/mergestate.go:47`) carries verb/source/outcomes/`StartedAt`, but no exported accessor returns it. Exposing it would mean designing and shipping new engine API.
- A new verb. This folds into the existing `status` verb; no `lyx fabric merge-status` is added.
- Any other verb's envelope. `commit`, `push`, `pull`, `sync`, `diff`, and the merge verbs are unchanged.
- The read-only/mutating envelope split. `status` stays read-only: still no `mutations` and no `partial` key.

## Decisions

### field-shape

- Decision: one boolean key, `merge_in_progress`, at the top level of `status`'s success envelope — alongside the existing `changes` key.
- Rationale: `MergeInProgress()` returns `(bool, error)` and nothing more. The roadmap item is scoped as "expose the existing `MergeInProgress` Go API on the status verb's output", so the CLI field mirrors the API's own return shape one-to-one. Snake_case matches every other multi-word key fabric already emits (`no_weft_correspondence`, `weft_pulled`, `warp_advanced`, `rewrite_detected`).
- Rejected: a nested `merge` object carrying verb/source/outcomes/`started_at` from `mergeState` — needs new exported engine API to reach that struct, which is out of scope. Also rejected: a bool *plus* an optional detail object, for the same reason.

### key-always-present

- Decision: the key is emitted unconditionally, `false` when no merge is parked.
- Rationale: `internal/fabriccli/envelope.go`'s header comment states the contract fabric holds — "a consumer therefore never has to distinguish absent from false, and the key set does not vary by outcome: that is the property that lets a test assert the shape once per verb instead of once per path". An omitted-when-false key would be the first fabric envelope field to break that.
- Rejected: emit only when `true` (smaller output, but forces every consumer into an absent-vs-false branch).

### error-handling

- Decision: a non-nil error from `fab.MergeInProgress()` fails the whole verb via `clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))` and returns — byte-for-byte the same shape as the existing `fab.Status()` error path at `weft_verbs.go:127-130`.
- Rationale: an error here means fabric cannot read its own merge-state record — the pair's state is unknown, not known-clean. Reporting `false` would assert a fact the process did not observe, which is the same failure mode `sync`'s `KindPushSpawned` comment (`weft_verbs.go:298-300`) already calls out as forbidden. Failing loudly is also what every other fabric read-only verb does on an engine error.
- Rejected: swallow the error and emit `false`; omit the key on error (breaks the always-present decision above).

### probe-ordering

- Decision: call `fab.Status()` first, then `fab.MergeInProgress()`, then emit one envelope.
- Rationale: preserves today's error precedence exactly — a broken pair that fails `Status()` keeps reporting the `Status()` error it reports today, not a new merge-state error. Both probes are read-only, so ordering has no side-effect consequence; only the error message the operator sees depends on it.
- Rejected: probe merge state first (changes which error surfaces for an already-broken pair, for no gain).

### read-only-envelope-unchanged

- Decision: `status` remains a read-only verb — the new field does not route through `okWithRecord`, so no `mutations` and no `partial` key appear.
- Rationale: nothing is mutated; `envelope.go:16-18` names `status` explicitly as one of the four read-only verbs that deliberately do not route through the record helpers. `TestRunCLI_ReadOnlyVerbsOmitMutationsKey` (`internal/fabriccli/cli_test.go:900`) pins this and must stay green **unmodified** — it is the machine-held statement of the scope decision.
- Rejected: nothing seriously considered; noted here because adding a field to a verb's envelope is exactly the change that tempts a switch to the record helpers.

### docs-in-same-commit

- Decision: the same commit updates `statusCmd`'s `Long` text, `docs/overview.md`'s `fabric` bullet, and moves the roadmap item Planned → Done.
- Rationale: `CLAUDE.md`'s "Task completion — docs land in the same commit" requires it for observable CLI behavior changes, and its roadmap rule ("moves only on completing or adding a planned item") is satisfied here — this completes the only Planned item. There is no `manifest/designs/fabric.md`, so no module design doc exists to update.
- Rejected: deferring the roadmap move to a follow-up commit.

## Technical context

**The call site.** `internal/fabriccli/weft_verbs.go:115-134` is the whole of `statusCmd`.
Its `RunE` today: `clihelp.ShouldAbort` guard → `fab.Status()` → on error `output.Err` → on success `output.Ok(out, map[string]any{"changes": changeEntriesMap(entries)})`, each wrapped in `clihelp.SetExit(cmd.Context(), …)`.
`fab` is the package-local closure variable assigned by `addWeftVerbs`'s `PersistentPreRunE` (line 110);
`status` is in `weftVerbNames` (line 28), so the handle is always resolved before `RunE` runs.

**The engine call.** `func (f *Fabric) MergeInProgress() (bool, error)` — `internal/fabricengine/mergelifecycle.go:418` — is a one-line delegation to `f.mergeRecordExists()`.
It is a read-only probe that deliberately produces no `MutationRecord`, and it never errors on foreign plain-git state.
No engine edit is needed;
the method is already exported and already used from tests (`mergecrucible_integration_test.go`).

**Envelope mechanics.** `internal/output/output.go:17-22` — `output.Ok` sets `fields["ok"] = true`, JSON-marshals, writes one line, returns 0.
There is no human-readable renderer anywhere in this path: fabric's CLI output is JSON-only, one object per line, per the CLI/Cobra Invariant.
So "surface it in the output" means exactly "add a JSON key".

**Bypass mode is irrelevant here.** `PersistentPreRunE`'s bypass branch (lines 70-81) rejects every verb except `push`, so `status` can never reach `RunE` with an unresolved `fab`.

**Test scaffolding that already exists** in `internal/fabriccli`:

- `hubforge.NewHub(t, ".")` + `hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: \"\"\n")` builds a real paired hub (the hubforge Fabric-Fixture Invariant requires this — no hand-assembled hub).
- `fabriccli.RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})` drives the verb.
- `decodeResult(t, &out)` (`cli_test.go:70`) parses the envelope into `map[string]any`.
- `setupConflictingDivergenceCLI(t, h.PrimeWorktree(), "feature", "conflict.txt")` + `branchAtCurrentHEADCLI(t, h.PrimeWeft(), "feature-weft")` followed by `RunCLIIn(…, []string{"merge-in", "feature"})` returning exit 1 is the established way to park a conflicted merge — see `TestRunCLI_MergeStageRejectsAPathThatIsNotConflicted` (`merge_cli_integration_test.go:432-445`) for the exact sequence.
- `gitOutputCLI(t, dir, args…)` (`cli_test.go:723`) for raw git assertions if needed.

**Gotcha — JSON booleans decode as `any`.** `decodeResult` returns `map[string]any`;
a JSON `false` decodes to `interface{}(false)`, so a bare `if !result["merge_in_progress"].(bool)` on a *missing* key panics rather than failing cleanly.
Existing tests use the comma-ok form (`if ok, _ := envelope["ok"].(bool); !ok`) plus a separate presence check (`if _, present := result["mutations"]; present`);
follow that pattern so a missing key is reported as a missing key.

## Constraints

From `CONSTRAINTS.md`:

- **CLI / Cobra Invariant** — errors are JSON via `internal/output`, one object per line; every `RunE` checks `clihelp.ShouldAbort` first; non-empty `Short` on every command; `<module>cli` imports `<module>engine`, never the reverse. All satisfied by editing inside the existing `statusCmd` handler and leaving `Short` alone.
- **hubforge Fabric-Fixture Invariant** — every hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`; no hub is hand-assembled. The new test must use `hubforge.NewHub`.
- **Fabric Vocabulary Invariant** — "fabric" names the wired composite; warp/weft only where the sides must be told apart. The new field is pair-level, so its name carries neither side.
- **Told-Geometry Invariant** — not touched: `statusCmd` reaches geometry only through the already-resolved `fab` handle and `l`.

From `CLAUDE.md`:

- **Documentation lifecycle / task completion** — docs land in the same commit (see the `docs-in-same-commit` decision).
- **Markdown: semantic line breaks** — one sentence per line in every `.md` file touched (`docs/overview.md`, `manifest/roadmap.md`).
- **Worktree isolation** — all work stays in this worktree; no push to `main`.

## Testing

Single module under test: `internal/fabriccli`. No engine tests change.

**TDD candidate — the new integration test.** Write it first;
it fails on the current `statusCmd` (missing key) and passes once the field is added.

Scenarios that must be covered:

1. **Clean pair, no merge parked** — `status` on a freshly built hub emits `merge_in_progress` *present* and `false`. Presence and value are two distinct assertions: a missing key must be reported as missing, not as false.
2. **Merge parked** — after a `merge-in` that returns a conflict envelope (exit 1), `status` emits `merge_in_progress` `true`. This is the assertion that proves the field is wired to the real record rather than hardcoded.
3. **The existing `changes` key survives** — asserted in the same test, so a regression that drops or renames `changes` while adding the new key is caught here rather than by a distant test.

Regression coverage that must stay green **without modification**:

- `TestRunCLI_ReadOnlyVerbsOmitMutationsKey` (`cli_test.go:900`) — `status` still carries neither `mutations` nor `partial`. If a plan finds itself editing this test, the implementation took the wrong path.
- The `fabriccli` help-tree / args-arity tests (`cli_test.go`, `argsarity_test.go`) — `status` stays `cobra.NoArgs` with an unchanged `Short`, so these need no edit.
- `envelopecontract_integration_test.go` — unchanged; `status` is not one of the mutating verbs it pins.

Whether the two cases live in one test function with subtests or two top-level functions is mill-plan's call;
the established style in this package is one top-level `TestRunCLI_*` per behavior, with `t.Run` subtests when the cases share expensive hub setup.

Verify with the repo's normal Go build/test path (`golang:golang-build`) over `./internal/fabriccli/...` at minimum, and the full suite before handoff.

## Q&A log

- **Q:** What shape does the new status output field take — a bool, or a richer merge-detail object? **A:** [auto-pick] Single `merge_in_progress` boolean. **Why:** `MergeInProgress()` returns only a bool; the `mergeState` record has no exported accessor, so a detail object would require new engine API the roadmap item does not ask for.
- **Q:** Is the key always present, or emitted only when a merge is parked? **A:** [auto-pick] Always present, `false` when clean. **Why:** `envelope.go`'s stated contract is that fabric's key set does not vary by outcome, so a consumer never distinguishes absent from false.
- **Q:** What happens when `MergeInProgress()` returns an error? **A:** [auto-pick] Fail the verb through `output.Err`, same as the existing `fab.Status()` error path. **Why:** the state is unknown, not known-clean; emitting `false` would assert an unobserved fact.
- **Q:** Probe merge state before or after `fab.Status()`? **A:** [auto-pick] After. **Why:** preserves today's error precedence for an already-broken pair.
- **Q:** Should foreign (plain-git) merge state be surfaced alongside it? **A:** [auto-pick] No — out of scope. **Why:** `MergeInProgress` deliberately excludes `foreignMergeStatePresent`; including it changes semantics rather than exposing the shipped API.
- **Q:** Does adding a field make `status` route through `okWithRecord`? **A:** [auto-pick] No — `status` stays read-only with no `mutations`/`partial` keys. **Why:** nothing is mutated, and `TestRunCLI_ReadOnlyVerbsOmitMutationsKey` pins this as a machine-held decision.
- **Q:** Which docs move in this commit? **A:** [auto-pick] `statusCmd`'s `Long`, `docs/overview.md`'s fabric bullet, and the roadmap item Planned → Done. **Why:** CLAUDE.md requires same-commit docs for observable CLI changes, and this completes the only Planned roadmap item.
