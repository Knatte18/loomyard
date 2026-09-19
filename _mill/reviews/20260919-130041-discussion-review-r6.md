MILL_REVIEW_BEGIN
# Review: Shed-generic watchdog for ly-drive and loom's CLI verbs

```yaml
duration_s: 148.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (self-assessed; exact build unknown)
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Cobra pre-run premise is false in this repo
**Section:** `### generic-package-resolves-nothing`
**Issue:** The rationale rests on "cobra runs only the nearest non-nil `PersistentPreRunE`", but `cmd/lyx/main.go:98` sets `cobra.EnableTraverseRunHooks = true` (and line 79 comments "Modules' PersistentPreRunE hooks run after root's"), so every hook from root down to the executed command fires; the real reason `lifecyclecli`'s pre-run is skipped under `lyx shed` is that `lifecycle` is not an ancestor of `shed run`.
**Fix:** Restate the premise as the ancestor-chain fact and say explicitly that `shedcli`'s own pre-run runs in addition to the root's (logger/`seedStencils`), not instead of it.

### [BLOCKING:design] Positional-arg refusal parity has no stated mechanism
**Section:** `### flags-and-args-belong-to-the-arming-module` (Consequence, line 147)
**Issue:** The discussion requires `lyx shed run --recipe lifecycle <slug>` and `lyx lifecycle run <slug>` to refuse a missing/extra argument identically, but lifecycle uses `Args: cobra.ExactArgs(1)` (`run.go:33`, `status.go:32`), which cobra validates at `command.go:968` — *before* any `PersistentPreRunE` (line 985) — while the shed path applies the recipe's arg contract inside the pre-run, a different mechanism with a different error string and ordering relative to `--recipe` resolution.
**Fix:** Decide the mechanism (e.g. the table entry supplies a `cobra.PositionalArgs` the shed verb's `Args` is set from, so both paths run the same validator) rather than leaving the parity test to discover the divergence.

### [NIT:scope] Sandbox Suite Coverage not addressed for `shedcli`
**Demoted-from:** BLOCKING
**Section:** `## Constraints` / `## Testing`
**Issue:** `cmd/lyx/sandbox_coverage_test.go` fails for any module registered in `newRoot()` that has no `**Covers:**` tag in `tools/sandbox/*SUITE.md` and no `excludedModules` entry; registering `shedcli.Command()` triggers it, yet the Sandbox Suite Coverage invariant appears in neither the Constraints list nor the CLI-tree testing bullet (which names only helptree/drift/longlist/jsonhelp).
**Fix:** State the disposition — a sandbox scenario tag for `shed` or an `excludedModules` entry with its reason — in the same commit.

### [NIT:scope] `plugins/ly/skills/INDEX.md` has no disposition
**Demoted-from:** BLOCKING
**Section:** `### ly-drive-drives-any-recipe` rules (a)–(f); Scope doc-update list
**Issue:** Rules (a)–(f) are scoped to "the whole skill file" and rule (e) to the skill's frontmatter, but `plugins/ly/skills/INDEX.md:5` carries the same loom-specific description verbatim ("Drive a loom task … `lyx loom step`") plus line 7's prose, and the Scope's doc list names only the design doc, `docs/overview.md`, `roadmap.md` and package headers.
**Fix:** Name `INDEX.md` as in scope and say its row tracks the generalized `description`.

### [NIT:consistency] Conditional export of `verbUsesLightweightWiring`
**Section:** `### per-verb-arming-preserves-lightweight-wiring`
**Issue:** "exported if the shed table needs to read it" is left conditional, yet `loomcli.Arm(cwd, verb, args)` already takes the verb and routes internally, so the table never reads the predicate.
**Fix:** Say plainly that the predicate stays unexported and `Arm` is its sole caller.

## Verdict

REQUEST_CHANGES
One false cobra premise, an unspecified arg-parity mechanism, and two unaddressed artefacts.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
