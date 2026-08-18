# Orchestrator review — burler-perch-told-geometry (T6)

Run in automode (every "[auto-pick]" answer in the Q&A log is the discussion agent's own call). Reviewed against `manifest/designs/producers-standalone.md`'s T6 brief (wave 3), cross-checked against `webster-told-geometry`'s (T7's) already-reviewed discussion since the two run in parallel off the same parent.

Spot-checked every load-bearing claim against current source — **all confirmed exact**: `burlerengine/engine.go` (31/41/97/103/111), `perchengine/engine.go` (52/58/101), `perchengine/identity.go` (33/43), `burlercli/cli.go` (104-107), `perchcli/cli.go` (146-166), `perchcli/run.go` (294/334/344), `cmd/lyx/constructoranchoring_test.go:176`, `cmd/lyx/notransients_test.go` (65/66/75/80/157), and `internal/shedadapters/perch.go:34`'s doc-comment wording. `internal/hubgeom/doc.go` and `hubgeom_test.go`'s claimed contract text (both quoted in the discussion) match verbatim.

## One real finding: T6 and T7's roadmap.md decisions are not actually compatible

This is the one thing that needs resolving before either task lands, not a nitpick.

**T7's discussion (already reviewed, `webster-told-geometry/_mill/orch-review.md`) explicitly decided *not* to touch `manifest/roadmap.md` at all:**
> "`manifest/roadmap.md` is **not** moved by this task — wave 3 completes only when T6 also lands, and the roadmap move belongs to whichever task closes the wave, or to T10."

**T6's discussion decided the opposite: it *does* edit `manifest/roadmap.md`, unconditionally, in this task's own commit** — reword the shared Planned item to name only the Webster half, and add a Done entry for the burler/perch half. Its rationale calls this "self-coordinating": *"whichever lands first shrinks the item, whichever lands second moves what remains to Done."*

The problem: only T6 has committed to *any* roadmap edit. T7 commits to none. So the "self-coordinating" protocol T6 describes in prose isn't actually implemented by either task's Decision text — it would require **both** tasks to inspect the roadmap's live state at merge time and branch on it (shrink vs. complete), but T6's Decision is a single unconditional action ("reword to name only Webster, add a Done entry for burler/perch"), not a conditional one.

Trace both merge orders:

- **T6 merges first:** roadmap.md becomes `Planned: producers standalone: producer engines — Webster [...]` + a new `Done: producers standalone: producer engines (burler/perch half)`. T7 merges second, does nothing to roadmap.md per its own decision. **Result: correct** — the Planned entry still accurately says "Webster," and it's actually still pending until T7 lands... except T7 already landed in this ordering and touched nothing, so the Planned entry is now stale (Webster is done, but roadmap still lists it Planned). Wrong.
- **T7 merges first:** roadmap.md is untouched — still says the original combined item. T6 merges second and executes its literal, unconditional Decision text: reword to name only Webster + add a Done entry for burler/perch. But Webster is *already done* at this point (T7 landed first) — so this produces a **Planned entry for already-completed work**, with no Done entry ever created for the Webster half. This is a materially wrong roadmap state that nothing else in either task's plan catches.

Either merge order leaves the roadmap wrong. The failure isn't in either task's reasoning taken alone — both are locally sound — it's that they don't agree on who owns the move, and T6's action isn't actually conditioned on T7's landed/not-landed state the way its own rationale claims.

**Recommendation:** resolve before either lands, not after. The simplest fix is for T6 to drop its roadmap edit and adopt T7's rule verbatim — defer the whole move to whichever task closes the wave (i.e., neither T6 nor T7 touches `manifest/roadmap.md`; the second one to land, recognizing the wave is now complete, does the single one-shot move to Done). That is one dropped Decision in T6's discussion, not a redesign — everything else in this task's `Out` section already treats roadmap-adjacent doc moves as out of scope by the same logic (`docs/overview.md`, `CONSTRAINTS.md`), so this brings the roadmap in line with that pattern rather than against it.

## Everything else: correct, and the discussion's own catches are sound

**The Files-list omission it caught is real.** `cmd/lyx/notransients_test.go`'s five `perchengine.RunsDir`/`ScratchDir` call sites (65/66/75/80/157) are not in T6's `Files` list in the design doc — confirmed by grep, that list only appears for T1. The discussion re-ran the enumeration against the tree instead of trusting the brief and found the gap itself, the same "enumeration obligation" pattern T4's and T7's reviews already credited.

**The `Geometry`-struct-over-loose-strings decision is correctly grounded in already-landed, not proposed, precedent.** `internal/hubgeom/doc.go` — landed in wave 2, on `main` today — literally states the geometry-struct contract and names T6 as the task adding `BurlerGeometry`/`PerchGeometry`. The discussion isn't inventing a shape; it's the one thing `hubgeom`'s own committed doc already committed T6 to. This correctly supersedes the design doc's older "plain string params" wording (T6's brief predates wave 2's `hubgeom` landing) — a stale-doc catch of the same kind T4 and T5 made, not scope drift.

**`perchengine.Geometry.GateDir` (not `WorktreeRoot`) is a well-reasoned, minor naming decision, not a compatibility risk.** T8's pinned-values table (design doc line ~521) already records that perch's `GateDir` *is* the `worktreeRoot` row, so the field-name choice loses no information for T8 to consume later — confirmed against the design doc's own T8 table.

**`perchcli` keeping `c.layout` for fabric-only call sites is correctly deferred to T8, not smuggled in.** `fabricengine.ScopedPathspec`/`Open`/`StencilsDir` at `run.go:334/344` and `cli.go` are genuinely hub-only concerns; converting those is explicitly named as T8's job in both T6's own `Out` section and independently in T8's design brief. Matches the same boundary T7's review confirmed for `RefMatcher`/`FabricBisector` — neither task pre-empts T8's CLI-side work, both only touch what their own engine needs.

**`CONSTRAINTS.md` correctly untouched.** No invariant is falsified by this task — hub mode still joins `perchDirName` onto `AnchorPath()`, just via `hubgeom` now. This is the right call and is consistent with T7's review, which found the *opposite* call correct for T7 (CONSTRAINTS.md rewords *do* belong in T7, because T7 actually ships a `<state>` root and a told stencils directory that falsify the current wording — T6 ships neither). The two discussions reach different conclusions on the same document for principled, checkable reasons, not inconsistently.

**Behaviour-preservation is taken seriously, not just asserted.** The `Out` section's "every path this task touches must resolve byte-identically" claim is backed by concrete test-strengthening plans: `TestEngine_Run_MaterializesInstructionFiles` gets an explicit swap guard (distinct `WorktreeRoot`/`AnchorRoot`, asserting round files land under one and *not* the other), and the existing `perchcli` anchoring proof (`TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd`) is correctly identified as already covering the production call site rather than needing a new test written.

**The `constructoranchoring_test.go` and `hubgeom.go` conflicts with T7 are self-flagged accurately** and match what I flagged before either task was spawned — both mechanical, not logical, and both discussions independently arrived at "whichever merges second rebases."

## Minor notes (non-blocking)

- The `hubgeom.go`/`hubgeom_test.go`/`doc.go` three-way conflict this discussion names alongside `constructoranchoring_test.go` (§Technical context, "Coordination with T7") is real but trivial — both tasks append independent functions to a short file, as I'd already flagged before spawning.

## Bottom line

One real, pre-implementation-fixable coordination gap: T6 and T7 disagree on who moves `manifest/roadmap.md` and when, and T6's "self-coordinating" split isn't actually conditioned on T7's landed state, so either merge order produces a wrong roadmap. Fix is small — drop T6's roadmap edit, adopt T7's defer-to-whoever-closes-the-wave rule in both. Everything else — the `Geometry` struct shape, the `GateDir` naming, the `perchcli`/T8 boundary, the `CONSTRAINTS.md` no-touch call, the `notransients_test.go` catch, and the swap-guard test strengthening — is correct, well-verified against current source, and consistent with T7's already-reviewed decisions. Not ready to proceed to planning as-is; resolve the roadmap-ownership conflict with T7 first (a one-paragraph discussion edit, not a re-review).
