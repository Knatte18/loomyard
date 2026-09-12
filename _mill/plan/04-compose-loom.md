# Batch: compose-loom

```yaml
task: 'self-report Tier 2: per-agent friction notes for unsupervised runs'
batch: 'compose-loom'
number: 4
cards: 3
verify: go test ./internal/loomengine/... ./contracts/stencils/...
depends-on: [1, 2]
```

## Batch Scope

This batch injects the friction directive into the two `internal/loomengine` prompt composers — `composePrompt` (Discussion-Write) and `composePlanPrompt` (Plan-Write) — and adds the matching `{{.friction_directive}}` marker to their two stencils.

It depends on batch 1 for `friction.Directive` / `friction.NotePath` / `friction.WarnIfMarkerAbsent` / `friction.MarkerName`, and on batch 2 for `loomengine.LoomFrictionDir` and `Config.Friction`.

`loomengine` is the one of the three prompt-composing engines that **derives** the friction directory rather than being told it, because `loomengine` is `LoomFrictionDir`'s own declarer and `DiscussionSpec`/`PlanSpec` already take a `*lyxcwd.Location` and a `Config`.
That is the Cwd Resolution Invariant working as written ("a module's own durable subdirectory is its own constant joined onto `AnchorPath()`"), not a Told-Geometry violation.
No new parameter is added to either Spec factory.

This batch exposes no new interface to later batches;
batch 7 changes nothing in `loomengine`.

## Cards

### Card 13: add `{{.friction_directive}}` to the two loom stencils

- **Context:**
  - `contracts/stencils/webster/webster-template-master.md`
  - `contracts/stencils/friction/friction-directive-implementer.md`
  - `contracts/stencils/friction/friction-directive-interview.md`
  - `internal/stencil/stencil.go`
- **Edits:**
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `contracts/stencils/loom/loom-template-plan.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add the literal marker `{{.friction_directive}}` on its own line to both stencils, placed the way `pattern_directive` already is in `contracts/stencils/loom/loom-template-plan.md:25` — as its own block, before the file's first work instruction, so an agent reads it in the same position in every prompt that carries it.

  In `loom-template-plan.md` the new marker sits immediately after the existing `{{.pattern_directive}}` line;
  `loom-template-discussion.md` has no `pattern_directive` today and gains only the friction marker.

  Update each file's opening HTML banner comment to describe the marker exactly as `webster-template-master.md:5` and `loom-template-plan.md:6` already describe `pattern_directive`: which markers are required, and that `friction_directive` is an optional marker filled via `stencil.FillOptional` that renders as nothing when Tier 2 is off.
  Keep the existing statement in both banners that there are no `{{if}}`/`{{range}}` conditionals anywhere in the file, and do not introduce one — a required marker inside a conditional branch renders silently blank when present-but-empty.
- **Commit:** `feat(stencils): add the friction_directive marker to loom's two producer prompts`

### Card 14: inject the directive in `composePrompt` and `composePlanPrompt`

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/friction/friction.go`
  - `internal/stencil/stencil.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `internal/loomengine/prompt.go`
  - `internal/loomengine/discussion.go`
  - `internal/loomengine/plan.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  **`internal/loomengine/discussion.go`.**
  In `DiscussionSpec`, resolve the note path before composing the prompt: `frictionDir` is `LoomFrictionDir(layout)` when `cfg.Friction != ""` and `""` otherwise, and the note path is `friction.NotePath(frictionDir, "Discussion-Write")`.
  The stem is the producer row's own name, which is stable across sites;
  `friction.NotePath` supplies the across-invocations half, which matters here because `Discussion-Write` is re-entered whenever `Discussion-Validate` bounces to it and re-executes on every crash-resume.
  Resolve the directive with `friction.Directive(notePath, stencilsDir, friction.RoleInterview)` and pass it into `composePrompt` as a new trailing parameter.
  A directive error wraps as `fmt.Errorf("loom: DiscussionSpec: %w", err)`, matching the function's existing wrapping.

  **`internal/loomengine/prompt.go`.**
  `composePrompt` gains a trailing `frictionDirective string` parameter, adds `friction.MarkerName` to its `values` map, calls `friction.WarnIfMarkerAbsent(template, "loom-template-discussion", frictionDirective)` after reading the template and before filling, and converts its `stencil.Fill(template, values)` call at `internal/loomengine/prompt.go:30` to `stencil.FillOptional(template, values, []string{friction.MarkerName})`.
  `stencil.Fill` is literally `FillOptional(template, values, nil)`, so this is a one-line change plus the optional-names argument.
  The `WarnIfMarkerAbsent` call passes the directive rather than the note path: an empty directive is exactly the Tier-2-off case the helper must not warn on, and a non-empty directive is exactly the enabled case where a marker-free template silently drops it.

  **`internal/loomengine/plan.go`.**
  In `PlanSpec`, resolve `frictionDir` and the note path the same way, with the stem `"Plan-Write"` — re-entered whenever `Plan-Validate` or `Plan-Revalidate` bounces, so it needs the same non-clobbering treatment.
  Resolve the directive with `friction.Directive(notePath, stencilsDir, friction.RoleImplementer)` and pass it into `composePlanPrompt` as a new trailing parameter.
  `composePlanPrompt` adds `friction.MarkerName` to its `values` map, calls `friction.WarnIfMarkerAbsent(template, "loom-template-plan", frictionDirective)`, and widens its existing `stencil.FillOptional` optional-names slice at `internal/loomengine/plan.go:49` from `[]string{"pattern_directive"}` to `[]string{"pattern_directive", friction.MarkerName}` — this composer needs no `Fill` -> `FillOptional` conversion.

  Neither Spec factory gains a new exported parameter, and neither creates the friction directory: creation is owned by the three call sites batch 7 wires, and a composer that created it would be one of seven racing creators.
- **Commit:** `feat(loomengine): inject the friction directive into the discussion and plan prompts`

### Card 15: composer tests for both loom prompts

- **Context:**
  - `internal/loomengine/prompt.go`
  - `internal/loomengine/plan.go`
  - `internal/loomengine/discussion.go`
  - `internal/friction/friction.go`
  - `internal/loomengine/discussion_test.go`
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `contracts/stencils/loom/loom-template-plan.md`
- **Edits:**
  - `internal/loomengine/prompt_test.go`
  - `internal/loomengine/plan_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend both existing composer test files with the same three cases each, following the file's own existing fixture and assertion style:

  1. **Enabled.** With a non-empty `cfg.Friction` and a seeded stencils directory, the composed prompt contains the resolved absolute note path verbatim.
     This is the assertion that catches a composer wiring the wrong path — it is the one property that cannot be satisfied by a directive that rendered but pointed somewhere else.
  2. **Disabled.** With a present-but-empty `cfg.Friction`, the composed prompt renders successfully and contains no directive text, and **no friction stencil is read** — assert the no-read half the way `internal/friction`'s own tests do, by making a read observable as a failure if it happened.
  3. **Marker-free template.** A seeded stencil whose bytes carry no `{{.friction_directive}}` literal still composes **successfully** (never an error) while Tier 2 is enabled.
     This is the operator-edited-stencil and dev-build case: `internal/stencilstore/reconcile.go` never refreshes a `StateEdited` stencil and in dev mode does not refresh a `StateUntouched` one either, so it is reachable in a real worktree and must degrade to a warning rather than a failed run.

  Keep every added test untagged Tier 1: no `exec.Command`, no `gitexec`, no real spawn.
- **Commit:** `test(loomengine): cover friction injection in the discussion and plan composers`

## Batch Tests

`verify: go test ./internal/loomengine/... ./contracts/stencils/...` covers both halves.

`./internal/loomengine/...` runs the extended `prompt_test.go` and `plan_test.go` (the enabled, disabled, and marker-free cases for both composers) plus every existing test in that package, which is what catches a `Fill` -> `FillOptional` conversion that changed rendering for the markers already there.

`./contracts/stencils/...` runs `registry_test.go` and `rubric_test.go` over the two edited stencil files, catching a banner or marker edit that broke the family's own conventions.

The scope is per-batch, not whole-repo: this batch edits three Go files in one package and two stencil files, and adds no signature any other package calls.
