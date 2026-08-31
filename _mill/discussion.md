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

That inference is also only half-reliable, which shapes the scope below.
`*ErrMergeInProgress` is raised from **two** distinct predicates, and they answer different questions:

- `mergeBlocksMutation` → `f.mergeRecordExists()` — *"is **this pair** mid-merge"* (`commit.go:123`, `pull.go:237`, `checkout.go:48`, `remove.go:65`).
- `mergeSourceInFlight` — *"is some **other pair in the hub** mid-merge **on this pair's branch**"* (`remove.go:81`, implemented at `mergestate.go:209-226`, which globs every pair's `fabric-merge.json` looking for a record whose `Source` is this warp branch).

`MergeInProgress()` is the first sense only — it is a one-line delegation to `mergeRecordExists()`.
So the new field is *not* a complete read-only mirror of "the verb that refused": a `remove` can refuse with `*ErrMergeInProgress` on the hub-wide predicate while this pair's own `merge_in_progress` is legitimately `false`.
The field answers the this-pair question, and every artefact this task writes must say so.

Why now: this is the sole item in `manifest/roadmap.md`'s **Planned** section — `fabric: surface merge-in-progress in \`lyx fabric status\``.
The Go API shipped with the merge lifecycle;
folding it into the `status` verb's output was deliberately deferred as a small follow-up, and this task is that follow-up.

## Scope

**In:**

- `internal/fabriccli/weft_verbs.go` — `statusCmd`'s `RunE` calls `fab.MergeInProgress()` and adds a `merge_in_progress` boolean to the success envelope.
- `statusCmd`'s `Long` help text — a short paragraph describing the new field, which **must name the this-pair sense explicitly** (see the `field-is-this-pair-only` decision): it reports whether *this* pair has a fabric merge parked, not whether some other pair in the hub is mid-merge on this pair's branch.
- A new integration test in `internal/fabriccli` covering both the `false` and the `true` case over a real `hubforge` hub.
- `docs/overview.md` — the `fabric` bullet's sentence describing what `status` reports (currently line 210).
- `internal/fabricengine/doc.go` — the merge section's sentence at line 1116, which today asserts what `lyx fabric status` does and does not tell an operator during a parked merge (see the `docs-in-same-commit` decision for the exact disposition).
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md` — F3's `Watch` (line 144, which enumerates status's output shape) and F18's `Watch` (line 404, which enumerates what the operator checks while a merge is live).
- `manifest/roadmap.md` — move the item from **Planned** to **Done**.

**Out:**

- `internal/fabricengine`'s **behavior** — no engine code change at all. `MergeInProgress()` is used exactly as it ships; its signature, semantics and doc comment at `mergelifecycle.go:414-419` are untouched. The `doc.go` edit listed above is prose in the package's merge section, not a change to any engine API.
- `mergeSourceInFlight`'s hub-wide sense — the "some other pair is mid-merge on this branch" predicate (`mergestate.go:209-226`) is **not** surfaced by this task, in any field. It is a different subject with a different answer, it is unexported with no exported accessor, and exposing it would mean designing new engine API. `status` gains one field with one sense.
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

### field-is-this-pair-only

- Decision: `merge_in_progress` answers "does **this pair** have a fabric merge parked" and nothing else. `statusCmd`'s `Long` text must say so in those terms, and must not describe the field as "a merge is in progress" unqualified.
- Rationale: two different predicates raise `*ErrMergeInProgress` (see Problem above). Only `mergeRecordExists` — the one `MergeInProgress()` delegates to — is this-pair; `mergeSourceInFlight` is hub-wide. An unqualified help text would promise the field predicts every `ErrMergeInProgress` refusal, and it does not: `remove` refuses on either predicate, so `merge_in_progress: false` alongside a refusing `remove` is correct behavior that would read as a bug against an unqualified description.
- Rejected: also emitting a second hub-wide field — `mergeSourceInFlight` is unexported with no accessor, so this needs new engine API, which the roadmap item does not ask for. Also rejected: leaving the `Long` text unqualified and documenting the distinction only here — the operator reading `--help` is exactly who the ambiguity misleads.

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

- Decision: the same commit updates every artefact that describes `lyx fabric status`'s output — `statusCmd`'s `Long` text, `docs/overview.md:210`, `internal/fabricengine/doc.go:1116`, and `tools/sandbox/SANDBOX-FABRIC-SUITE.md` F3 and F18 — and moves the roadmap item Planned → Done.
- Rationale: `CLAUDE.md`'s "Task completion — docs land in the same commit" requires it for observable CLI behavior changes. There is no `manifest/designs/fabric.md`, but `manifest/roadmap.md:129` makes a shipped module's own package documentation its durable doc, so `internal/fabricengine/doc.go` is in scope as documentation even though no engine code changes.
- Rejected: deferring the roadmap move, or any of the doc edits, to a follow-up commit.

**How the doc inventory was enumerated.** `grep -rn "fabric status" --include=*.go --include=*.md .` over the repo, excluding `_test.go` and `_mill/`. Full hit list and per-file disposition:

| File:line | Claim it makes | Disposition |
| --- | --- | --- |
| `internal/fabricengine/doc.go:142` | `status` reports a pair not in sync, `JunctionReason` naming the cause | Out of scope — junction-drift claim, untouched by this task. |
| `internal/fabricengine/doc.go:227` | `status` is "one side-labelled view" | Out of scope — describes the engine's `Status()` return shape, which is unchanged. The new field is a CLI-envelope addition, not a `[]ChangeEntry` change. |
| `internal/fabricengine/doc.go:1116` | during a parked merge, `status` "reports a remaining weft-side conflict as an ordinary weft change indistinguishable from any other" | **In scope — edit.** The claim stays true (the field is pair-level, so it still names no path), but the passage is the merge section's account of what an operator can learn from `status` mid-merge, and after this task that account is incomplete. Add a clause noting `status` now reports the parked merge itself via `merge_in_progress`, while still not distinguishing which weft path is conflicted — the surrounding argument for why `merge-stage` must exist is unchanged and must survive the edit intact. |
| `tools/sandbox/SANDBOX-FABRIC-SUITE.md:144` (F3 `Watch`) | enumerates status's output shape for the operator running the suite | **In scope — edit.** Add the `merge_in_progress` field to the enumeration, noting it is `false` in F3's own no-merge scenario. |
| `tools/sandbox/SANDBOX-FABRIC-SUITE.md:404` (F18 `Watch`) | lists what to check while a merge is live — today, only that the sibling verbs refuse | **In scope — edit.** Add: `status` is the read-only way to ask, and must report `merge_in_progress: true` for the whole live window and `false` again after both `merge --continue` and `merge --abort`. |
| `docs/overview.md:54` | `lyx fabric status` "flags drift" | Out of scope — a friction-asymmetry example, not an output enumeration. |
| `docs/overview.md:210` | "`status` is the unified both-sides uncommitted-change view" | **In scope — edit.** Extend the sentence to name the new field. |
| `manifest/roadmap.md:12` | the Planned item itself | **In scope** — moved to Done (see `roadmap-move` below). |

### roadmap-move

- Decision: move the item into **Done**, leaving the **Planned** section heading and its lead-in in place with no items under it. The Done entry points at `internal/fabricengine`'s package documentation's merge section.
- Rationale: `manifest/roadmap.md:131` says to move an item to Done when it ships, "with a link to its module doc if one exists". No `designs/fabric.md` exists (and per `roadmap.md:129` a shipped module's Done entry points at its package documentation anyway, not a design doc), and `roadmap.md:81`'s `fabric: two-sided reset-to-SHA verb` entry already establishes the exact phrasing for this module — "See the `internal/fabricengine` package documentation's merge section." — so this entry follows it. That section is also the one being edited per `docs-in-same-commit`, so the pointer lands on a passage that actually mentions the new field.
- An empty Planned section is acceptable and must not be papered over: nothing in the Maintenance rules requires Planned to be non-empty, the numbering rules are per-section and unaffected, and promoting a Someday item to fill the gap would be an unrequested scope decision that belongs to the operator, not to this task. Do not delete the heading — the section's own lead-in ("This section holds what's committed to next") reads correctly when empty, and deleting it would break every cross-reference to "the Planned section".
- Rejected: promoting a Someday item in the same commit to keep Planned non-empty; deleting the empty Planned heading; pointing the Done entry at `internal/fabriccli`'s package documentation — it exists (`internal/fabriccli/fabric.go:9-13`) but describes the verb tree's shape rather than the merge lifecycle, so the merge section in `fabricengine` is where a reader following the link actually finds the field's subject. `fabriccli`'s package doc enumerates verbs, not envelope fields, so it makes no claim this task falsifies and needs no edit.

## Technical context

**The call site.** `internal/fabriccli/weft_verbs.go:115-134` is the whole of `statusCmd`.
Its `RunE` today: `clihelp.ShouldAbort` guard → `fab.Status()` → on error `output.Err` → on success `output.Ok(out, map[string]any{"changes": changeEntriesMap(entries)})`, each wrapped in `clihelp.SetExit(cmd.Context(), …)`.
`fab` is the package-local closure variable assigned by `addWeftVerbs`'s `PersistentPreRunE` (line 110);
`status` is in `weftVerbNames` (line 28), so the handle is always resolved before `RunE` runs.

**The engine call.** `func (f *Fabric) MergeInProgress() (bool, error)` — `internal/fabricengine/mergelifecycle.go:418` — is a one-line delegation to `f.mergeRecordExists()`.
It is a read-only probe that deliberately produces no `MutationRecord`, and it never errors on foreign plain-git state.
No engine edit is needed;
the method is already exported and already used from tests (`mergecrucible_integration_test.go`).

**The two merge predicates, and why only one is in play.** `internal/fabricengine/mergestate.go` holds both.
`mergeRecordExists()` (line 179) reads this pair's own `fabric-merge.json`;
`mergeSourceInFlight(l, warpBranch)` (line 209) globs *every* pair's record under the weft repo — `<weft gitdir>/.git/fabric-merge.json` for the prime pair and `.git/worktrees/*/` for each linked pair — and matches on `st.Source == warpBranch`.
`remove.go:65` and `remove.go:81` call them in sequence and return the *same* `&ErrMergeInProgress{}` from either, which is exactly why the error alone cannot tell an operator which condition fired.
`MergeInProgress()` exposes only the first.
`mergeSourceInFlight` is unexported and has no exported accessor, so surfacing it is not a CLI-only change — that is the mechanical reason it is out of scope, not just a scoping preference.

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
- **Q:** Which docs move in this commit? **A:** [auto-pick] `statusCmd`'s `Long`, `docs/overview.md:210`, `internal/fabricengine/doc.go:1116`, `SANDBOX-FABRIC-SUITE.md` F3 and F18, and the roadmap item Planned → Done. **Why:** CLAUDE.md requires same-commit docs for observable CLI changes, and a repo-wide `grep -rn "fabric status"` found five live claims about status's output — the disposition table under `docs-in-same-commit` records each one, including the three deliberately left alone.
- **Q:** `*ErrMergeInProgress` is raised from two predicates (this-pair `mergeRecordExists` and hub-wide `mergeSourceInFlight`) — which does the field answer, and does the other get surfaced? **A:** [auto-pick] This-pair only, named as such in the `Long` text; the hub-wide sense is explicitly out of scope. **Why:** `MergeInProgress()` delegates to `mergeRecordExists` alone, so `merge_in_progress: false` can legitimately coexist with a `remove` refusing on the hub-wide predicate; the help text has to say which question it answers, and `mergeSourceInFlight` is unexported with no accessor, so exposing it would need new engine API.
- **Q:** Moving the sole Planned item leaves that section empty — acceptable, and what does the Done entry link to? **A:** [auto-pick] Empty Planned is acceptable and the heading stays; the Done entry points at `internal/fabricengine`'s package documentation's merge section. **Why:** no Maintenance rule requires Planned to be non-empty, and filling it would be an unrequested scope call; `roadmap.md:81` already establishes that exact link phrasing for this module, and that section is the passage this commit edits.
