MILL_REVIEW_BEGIN
# Review: self-report Tier 2: per-agent friction notes for unsupervised runs — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-12
```

## Findings

No findings. I read all eight batch files, the overview's Shared Decisions, and every source file listed in the manifest, and cross-checked each Shared Decision and each card's requirements against the actual diff.

Specifics verified:

- `internal/friction` is a true leaf (stdlib + logger + stencil + stencilstore only), enforced by an allowlist AST test; `Directive`/`NotePath`/`WarnIfMarkerAbsent`/`EnsureDir`/`MarkerName`/`ReportFileName` all match the batch-1 spec exactly, including the "off is an empty string, never a bool" propagation and the non-clobbering first-free-name scan with its documented concurrency caveat.
- `loomengine.LoomFrictionDir`/`LoomFrictionArchivePrefix` are built on `LoomScratchDir` (never re-joining `.lyx`/`loom` a second time), and `Config.Friction`/`FrictionTimeoutMin` are validated exactly as specified (non-empty-only model-spec check, zero-accepted timeout, "missing keys" migration contract preserved).
- `internal/frictionengine`'s Told-Geometry seam allowlist, `Deps` validation order, scan/skip/spawn/archive sequence, stale-report delete before spec build, and the never-archive-on-failure rule all match card 9-12 verbatim, including the deliberately broader `!= OutcomeDone` failure check (covers Died/Timeout/Asking and any future outcome, a safe superset of the plan's explicit three).
- The three composer batches (loom, burler, webster) apply the "swallow `friction.Directive` errors, never propagate" decision consistently and asymmetrically beside the still-propagating `pattern.Directive` calls, exactly as the Shared Decision requires; the three `Fill`→`FillOptional` conversions land at exactly the three named sites and nowhere else.
- `burlerengine.New`'s fifth parameter and `RunOpts.NoteID` are threaded correctly; `shedadapters.BurlerProducer` composes the `"burler-" + filepath.Base(runDir) + "-r" + round` stem only inside the per-attempt loop, never in `probeLiveRound`'s attach spec, matching the card 18 carve-out.
- `websterengine`'s `FrictionDir` field is replicated on all three Deps structs (not funneled through `RunDeps` alone) and every one of the four `render.go` composers resolves the correct role/stem pair; `internal/webstercli` resolves the friction directory tolerantly in `wireHub` only, standalone stays empty, matching the "why webstercli must resolve it itself" rationale.
- `internal/loomcli`'s single `frictionDir` resolution in `wire`, the once-per-task clear (bound `seedErr`, clear only on the nil branch, ensure on both), `drive`'s unconditional ensure, and the `RunDone`/`RunBlocked`-only reflection trigger with envelope key `friction` (never `"filed"`) are all present and match cards 22-24 exactly, including the extracted `ensureFrictionDirAfterSeed`/`shouldReflectFriction`/`reflectFriction` seams card 26 asks for.
- Test coverage is thorough and consistently follows the "enabled / disabled (no stencil read) / marker-free" three-case shape across every composer batch (loom, burler, all four webster composers), plus the dedicated `frictionengine` scan/archive/failure suite and the `loomcli`/`webstercli` wiring tests. `render_test.go` itself carries no friction tests, but `template_test.go` (the file that already owns `Render*Prompt` round-trip tests) covers all four composers' three cases in full — a reasonable within-batch split, not a coverage gap.
- Docs batch: `docs/overview.md`'s module tree gets both new rows with the tree's `└──` terminator preserved, `manifest/roadmap.md` moves the item to Done pointing at the two packages' godoc, and `manifest/designs/self-report-tier2.md`'s Status/Open-questions section is closed in place with its heading renamed, and the two inbound links from `self-report-tier1.md`/`loom-step.md` still resolve since the file stays on disk.
- No out-of-plan files, no duplicated helpers across batches, no constraint violations found (Cwd Resolution, Told-Geometry, Lyxdirs Single-Declarer, Stencil Ownership, Config Strictness, Completion Signal, and the new Friction Leaf Invariant all hold).

## Verdict

APPROVE
All eight batches match the plan, Shared Decisions, and CONSTRAINTS.md with no discrepancies found.
MILL_REVIEW_END
