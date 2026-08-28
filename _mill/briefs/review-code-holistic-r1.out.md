MILL_REVIEW_BEGIN
# Review: reed: attach's layout computation scales header pane height with terminal height — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-28
```

## Findings

None.

The implementation was checked end to end against all three batches:

- Batch 1 (`render/rules.go`, `height.go`, `layout.go`, `pins_test.go`): `planCells`/`cellPlan` extraction is exact, `Rules`' exported signature and behavior are unchanged, `placement.strip` is threaded correctly from `stackHeights`'s `isStrip[i]`, and `FixedHeightPins` matches the Shared Decision `pins-are-render-placed-heights-never-raw-config` — pins are read off `plan.headerHeight`/`pl.height` post-clamp, never `p.Header.HeightRows`/`p.CollapsedStripRows` raw. `pins_test.go` covers every case card 2 specifies, including the drift-guard re-parse of `Rules`' own layout string.
- Batch 2 (`apply.go`, `windowsize.go`, `attach.go` + tests): `toRenderInputs` is the single mapping site both `planLayout` and `fixedHeightPins` share, matching the Shared Decision `one-mapping-site-from-state-to-render-inputs`. `resizePinHookArgvs` encodes the clear-then-array-entries shape exactly per `hook-body-is-one-array-entry-per-pin` (unconditional clear, `-a` from the second pin on, no bare `";"` element). `installResizePinsLocked` is non-fatal per `hook-failure-is-non-fatal-everywhere`. Both install statements land at the two named points in `applyLayoutLocked` and `AttachArgv` with no guard reordering, and the guard-skip paths (`len(live) < 2`, `!anyPlacedStrand`) issue no `set-hook` call at all, matching `guard-skip-leaves-a-stale-array-deliberately`. Tests in `windowsize_test.go`/`apply_test.go`/`attach_test.go` cover the argv shape, both install-statement positions, the zero-pin clear, the guard-skip silence, and non-fatal `set-hook` failure on both call sites.
- Batch 3 (integration tests, `doc.go`, `probe.go`, `manifest/roadmap.md`): `attachgeometry_integration_test.go`'s new cases cover the post-attach growth resize, the bare-attach ~50-row-threshold reproduction, and the dead-strip-pin isolation, each with the doc-comment framing the plan specifies. `contract_integration_test.go`'s new `TestMultiplexerContract` section pins array independence and firing-after-resize at the wire level, placed before `list-sessions`/teardown as required, and restores window state via the two named mechanisms (`set-hook -u` clear, `set-option -uw window-size`) with no capture-restore idiom. `doc.go`'s three edits (new bullet, "chained attach" addendum, "Subcommand set" required/optional split plus the rewritten closing sentence) are present and consistent with `probe.go`'s unchanged `requiredSubcommands`. `manifest/roadmap.md`'s watchdog-daemon item narrows only the resize clause as specified, preserving the pane-reap half verbatim.

No out-of-plan files, no duplicated helpers between batches (`resizePinHookArgvs`/`installResizePinsLocked` are defined once, in `windowsize.go`, and consumed from both `apply.go` and `attach.go`), and no constraint violations found (Told-Geometry Invariant respected — `fixedHeightPins`/`installResizePinsLocked` take a told box and touch no `lyxcwd`; CLI/Cobra and Config Strictness invariants are untouched by this task's scope).

## Verdict

APPROVE
Implementation matches the plan and Shared Decisions across all three batches with no constraint violations found.
MILL_REVIEW_END
