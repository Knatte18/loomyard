MILL_REVIEW_BEGIN
# Review: PATTERN wiring: conditional constraint-injection into every agent

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: C:\Code\loomyard\wts\pattern-wiring\_mill\discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Materialisation makes `created`/`exists` unreachable
**Section:** weft-target-materialisation vs result-shapes / Testing (`internal/initengine`)
**Issue:** `init.go:67` runs `WireJunctions` before the `os.Stat(cwd/_lyx)` at `init.go:75`; once `seedLyxJunction` does `MkdirAll(target)` first, the stat through the fresh junction succeeds, so `InitResult.LyxDir` reports `"exists"` on a first init — and the required test "`InitResult.PatternDir` reports `created` on first run" (Testing, initengine) can never pass.
**Fix:** State which side wins — either stat the weft target before `WireJunctions`, or redefine the created/exists vocabulary for both `LyxDir` and `PatternDir` — and record the observable `lyx init` output change (`lyx_dir` at `initcli.go:98`).

### [GAP] `drift.PairInSync` has no slug to enumerate junctions with
**Section:** junction-health-check
**Issue:** All three sites use the Here-anchored pair (`HostLyxLinkHere()`/`WeftLyxDir()` at `reconcile.go:146`, `status.go:148`, `drift.go:74`), but the prescribed loop source `l.HostJunctions(slug)` is Hub/slug-anchored, and `PairInSync(l *hubgeometry.Layout)` (`drift.go:38`) carries no slug at all; the scope's new geometry surface lists no Here-anchored junction enumerator.
**Fix:** Name the accessor the three sites loop over (e.g. a `HostJunctionsHere()` mirroring `HostLyxLinkHere()`/`WeftLyxDir()`), or state where each site derives its slug.

### [GAP] "Entries that currently match something" is undefined
**Section:** weft-pathspec-tolerance
**Issue:** The filter predicate is unspecified, and three readings break real callers: an untracked-only `_pattern/PATTERN.md` must count as a match (a `git ls-files` predicate would drop the very first PATTERN commit); an index-only match must count (`undo.go:93-94` commits a just-`RemoveAll`'d `_lyx`); and `CommitWeft`'s pathspec carries `:(exclude)` magic from `buildercli/weft.go:77`, `webstercli` and `perchcli`, which must never be filtered out (dropping one re-stages machine-local artifacts, against CONSTRAINTS' Cross-module exclusions bullet).
**Fix:** Define the predicate explicitly — worktree-or-index match, and exclusion-magic entries always passed through untouched — and add a test for an `:(exclude)`-bearing pathspec.

### [GAP] Stale pathspec/junction help and docs not enumerated
**Section:** Constraints (CLI/Cobra Invariant) / weft-persistence
**Issue:** Widening the default breaks help and docs the discussion never names: `fabriccli/weft_verbs.go:167` states "Staging is scoped to the directories listed in the fabric config (default: _lyx)"; `fabricengine/template.yaml:2`'s inline comment says "_lyx is the default"; `docs/overview.md:96-99` lists exactly one junction (`<host>/_lyx → <hub>/<slug>-weft/_lyx`). Help accuracy is review-blocking under the invariant.
**Fix:** Add `lyx fabric commit|push|sync`'s `Long`, the `template.yaml` comment, and overview.md's junction section to the same-commit doc list.

### [NOTE] Existing worktrees stay inert with no signal
**Section:** weft-persistence (migration consequence)
**Issue:** `configsync.ReconcileAll` → `yamlengine.Reconcile` keeps a present `pathspec:` value and adds no key, so every already-initialised worktree keeps `pathspec: _lyx` and never persists PATTERN — verifiable now rather than deferred to mill-plan, and nothing (`fabric status`, `lyx init`) reports the narrow pathspec.
**Fix:** Record the confirmed semantics and say whether any detection/warning surface is in or out of scope.

### [NOTE] Anchor-divergence rationale overstates the risk
**Section:** directive-shape-and-wording (relative pointer)
**Issue:** For `hubgeometry.Resolve`-built layouts `RelPath = Rel(WorktreeRoot, Cwd)` (`hubgeometry.go:63,93`), so `WorktreeRoot+RelPath == Cwd` and the two anchors cannot disagree; only a `RelPath`-hardcoded constructor (`hubgeometry.go:170-186`) could diverge.
**Fix:** Restate the rationale on the accurate premise (prompt-idiom consistency), naming the one constructor where the anchors can differ.

## Verdict

GAPS_FOUND
Four gaps: init result vocabulary, drift's missing slug, undefined pathspec predicate, unlisted stale help.
MILL_REVIEW_END
