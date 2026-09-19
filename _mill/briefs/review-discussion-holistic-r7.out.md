MILL_REVIEW_BEGIN
# Review: Seeded Shed core: run addressing, seed contract, batten

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-20250514-class assistant (self-assessed; exact build unknown)
reviewed_file: _mill/discussion.md
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Batten's new `step` has no seeded status or BuildShed
**Section:** `seed-verb-plus-auto-seed-on-the-batten-entry-path` ("Consequence: batten gains a `step` verb")
**Issue:** The decision names only the table entry, `StepBusyKind`, and the `seed.json` auto-seed, but batten's **status.json** seeding lives in `lifecyclePreRun` — a `PreRun` hook `stepCmd` never calls — and `specFor` sets `BuildShed` only for `verb == "run"` (`internal/lifecyclecli/arm.go:88`); `stepLocked`'s read gate treats an absent status as a hard error ("Shed never seeds a status file", `internal/shedengine/run.go:88`), and `shedverbs/step.go:121` would report it as `kind: "producer"`, the one kind the supervisor skill retries.
**Fix:** Decide batten's `PreStep` hook (which pre-flight and status-seed it performs, and its refusal-kind mapping), its `BuildShed` arm for `verb == "step"` (`battenrecipe.New` vs a loom-style `buildLoomShed` analogue), and the `Step` verb texts the CLI/Cobra `Short` rule requires.

### [BLOCKING:scope] Rename sweep cannot reach CONSTRAINTS.md
**Section:** Technical context, "Enumerate by literal"
**Issue:** The sweep is four literals (`_lyx/loom`, `.lyx/lifecycle`, `--recipe`, `lyx lifecycle`) over `contracts/`, `plugins/`, `tools/`, `docs/`, `manifest/` — which excludes repo-root `CONSTRAINTS.md` and never matches the package-name literals; `CONSTRAINTS.md:24` (Told-Geometry bound packages, naming `lifecycleshed`, `lifecyclerecipe`) and `:154` (interactive-handoff exception, naming `lyx lifecycle status --watch`) both go stale, and the discussion's Constraints section names only the Bookend heading and the CLI/Cobra deviations line.
**Fix:** Add `lifecycleshed`/`lifecyclerecipe`/`lifecyclecli` to the sweep literals and `CONSTRAINTS.md` to the swept paths, and enumerate all four CONSTRAINTS lines the rename touches.

### [BLOCKING:consistency] Shed Verb-Set bullet asserting shedcli derives no paths
**Section:** `shedcli-resolves-the-location-and-arm-gains-a-location-taking-form`
**Issue:** `CONSTRAINTS.md:112` states that neither `shedverbs` nor `shedcli` is in the Told-Geometry bound list because "their identical no-derived-paths obligation is carried by this invariant's own no-resolver clause"; this task makes `shedcli` call `lyxcwd.Resolve` and construct `shedrun` seed paths from the result, so the bullet becomes false, and the discussion justifies the change only against `resolvePersistentPreRun`'s doc comment.
**Fix:** State whether the bullet is amended in the same commit (and to what), or why `shedcli` reaching paths solely through `shedrun` constructors still satisfies it.

## Verdict

REQUEST_CHANGES
Batten's `step` wiring is unspecified; two CONSTRAINTS obligations are unenumerated.
MILL_REVIEW_END
