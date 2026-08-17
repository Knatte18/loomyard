# Orchestrator review — shuttle-reed-told-geometry (T3)

Reviewed against `manifest/designs/producers-standalone.md`'s T3 brief (told-geometry, wave 1).
Spot-checked the load-bearing claims against current source — all confirmed accurate: `reedengine.HubLogsDir` at `lifecycle.go:35`, `New(cfg Config, layout *lyxcwd.Location)` at `lock.go:39`, `ServerName`/`SessionName`/`socketName` in `server.go`, `strand.go:175-176`'s two `WorktreePath()` reads, `tokenvocab.Ctx{Layout}`'s two-token registry, `fabricengine.HubScratchDir` at `junctionnames.go:135`, and `constructoranchoring_test.go` rows 96/144 exactly as described.

## Scope verdict: correct on the core, one significant unplanned architectural addition

The core signature changes match the design closely: `shuttleengine.NewRunner`/`FindRun`/`runDirRoot` taking plain `anchorRoot`/`worktreeRoot` strings, `reedengine.New(cfg, Geometry)`, `tokenvocab.Ctx` collapsing to two plain fields — all pinned by the design's own T3 brief, not invented here.

**`HubLogsDir`'s move to `fabricengine` is faithful execution, not an addition** — the design's own T3 Files list explicitly names `cmd/lyx/constructoranchoring_test.go`'s `reedengine.HubLogsDir` rows (96, 144) as in scope, and the brief itself requires the Treadle Runner-Seam Invariant reword this move produces. I initially expected this to be a scope stretch; it isn't — the design already committed to it, this discussion just executed it.

**`WorktreeRoot` as a `Geometry` field is a correctly self-identified design-doc gap, not scope creep.** The design's own T3 table only lists five fields and omits it; `strand.go:175-176` genuinely reads `WorktreePath()` twice, and once `SessionName` is told rather than derived, there's no way to recover the worktree root from anything else in the struct. The discussion labels this explicitly as "an omission found during exploration, not a scope addition" — that's an accurate self-assessment, and the fix is minimal (one more struct field, sourced the same way as the others).

**The one item that genuinely exceeds T3's brief: `internal/hubgeom`.** This is a brand-new shared package, with its own `doc.go`, a `docs/overview.md` entry, and a `docs/shared-libs/README.md` entry — and it does not appear anywhere in `manifest/designs/producers-standalone.md` (confirmed: zero matches for "hubgeom" in the design doc). The technical case for it is sound on its own terms — nine call sites building the same seven-field struct by hand is a real duplication-and-swap-hazard risk, and "reuse, never duplicate" is stated as a standing operator rule. But the package is explicitly designed to be extended by **T6 and T7**, per this discussion's own rationale: *"a generic name (`hubgeom`, not `reedgeom`) is chosen so T6 ... and T7 ... add `BurlerGeometry`/`PerchGeometry`/`WebsterGeometry` to the same package instead of forcing a rename."* Neither T6's nor T7's brief in the design doc mentions any shared geometry-teller package — those tasks haven't been discussed yet, and their own discussion phases have had no chance to weigh in on this naming or shape before it's decided here, in wave 1, by a task whose own dependency line reads "Depends on. Nothing."

This isn't necessarily wrong — early, well-reasoned convergence on a shared helper can save real rework — but it is a governance point worth flagging explicitly rather than letting it pass silently: T3 is making an architectural decision on T6/T7's behalf, three waves early.

**A related, more concrete risk**: the discussion is explicit that `manifest/designs/producers-standalone.md` itself will *not* be amended to record `hubgeom`'s existence (reasoned as out of scope for the same-commit docs rule, which is a defensible reading of CLAUDE.md). That means anyone who spawns T6 or T7 by reading only the design doc — without also reading this discussion file — will not learn `hubgeom` exists, and could re-derive the seven-field construction inline exactly the way this task worked to avoid. The discussion says "this discussion file is the corrected record for T3," but T6/T7's own future discussion phases are the ones that actually need to find that pointer, and nothing in the design doc routes them here. Worth adding a one-line breadcrumb to the design doc's T6/T7 entries (or to `hubgeom/doc.go`, which a T6/T7 explorer would more plausibly find via grep) even if the full discussion isn't amended.

**Everything else is correctly excluded.** No standalone CLI entry, no `preflight`/`buildinfo`/`standalonestate` (T5), no config-loader change (T2), `burlerengine`/`perchengine`/`websterengine`/`scoutengine` left alone except the two one-token `FindRun` call fixups explicitly permitted by the design brief itself, no new told-geometry invariant (correctly deferred to T10), no roadmap edit. The one real cross-task adjacency — T1 also touching `constructoranchoring_test.go`, at non-overlapping rows (71-72/120-121 vs. 96/144) — is correctly identified and matches T1's own discussion of the same adjacency.

## Minor notes (non-blocking)

- The claim that `reedengine.HubLogsDir` "moves" to `fabricengine.HubLogsDir` is precise: confirmed the function doesn't already exist there, so this is a genuine relocation, not a rename over an existing duplicate.
- No factual errors found in the construction-site enumeration, the test-fixture inventory, or the import-graph-after table.

## Bottom line

Core scope is correct and, on `HubLogsDir`, more faithful to the design than it first appears.
`internal/hubgeom` is the one item to flag before implementation starts: technically sound, but it's new architecture invented mid-task that quietly binds two not-yet-discussed future tasks. Confirm this is wanted now (versus letting T6's own discussion decide whether to introduce a shared teller when that task actually exists), and at minimum leave a findable pointer to it for whoever picks up T6/T7.
