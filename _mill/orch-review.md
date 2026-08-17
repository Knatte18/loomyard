# Orchestrator review — producer-standalone-capability discussion draft

Reviewed: `_mill/discussion.md`, `manifest/designs/producers-standalone.md`, `manifest/roadmap.md` diff.
Method: read both files in full, then independently verified every load-bearing factual claim against the current tree in this worktree (not taken on the doc's word) — `lyxcwd.Resolve`'s actual gate, scout's synthetic-`Location` mechanism, `reedengine`'s current `lyxcwd` reference counts, and a sample of `file:line` citations across layers 1–3 and task T1. Also ran `TestEnforcement_MarkdownLinks` and diffed the worktree against `main`.

## Verdict

**Sound. No factual claim I checked was wrong, several were verified to the exact line number.** Two structural gaps worth resolving before this decomposition is copy-pasted into wiki tasks, and one low-severity wording nit in `discussion.md`'s own Testing section. Nothing here blocks proceeding.

## What verified clean

- `git diff --stat main...` matches the claimed scope exactly: `_mill/discussion.md`, `_mill/status.md`, `manifest/designs/producers-standalone.md`, `manifest/roadmap.md` — no stray production-code diff.
- `go test ./internal/lyxcwd/... -run TestEnforcement_MarkdownLinks` passes, so every link inside `manifest/` (including the new design doc's ~15 cross-references into `CONSTRAINTS.md`, `shed.md`, `loom.md`, `hardener.md`) resolves both file and anchor.
- The doc's central claim — `lyxcwd.Resolve` proves far less than the originating discovery task assumed — checks out against `lyxcwd.go:104-184` to the letter: `hubPath := filepath.Dir(workTreeRoot)` is unconditional, `RepoName` is a bare `TrimSuffix` with no presence check, `anchorRel` defaults to `"."`, and the doc comment at `lyxcwd.go:73` literally says "Resolve does NOT check for `_lyx/`". Every clause of the doc's paraphrase matches the source.
- The scout-as-precedent correction checks out: `scoutcli/cli.go:455` (`resolveLocation`) and its doc comment confirm the synthesized `Location` is bounded to `AnchorPath()`-only consumers exactly as claimed, and `lookupContext` does fall back to `scoutengine.BuiltinRegistry()`.
- The `reedengine` shrinkage numbers are exact: `lock.go` ×3, `lifecycle.go` ×1, `strand.go`/`header.go` ×0 `lyxcwd.` references, confirmed by direct grep — matches the "Corrections" table precisely, including the `header.go:16` transitive-through-`tokenvocab.Ctx.Layout` detail.
- T1's citation (`webstercli/cli.go:194` → `loomengine.PlanDir(layout)`) and the Layer 2 code block (`burlercli/cli.go`'s unconditional `Resolve`→five-loader sequence) both match the source verbatim, including line numbers.
- Task/wave arithmetic is internally consistent: 3+2+2+2+1 = 10 tasks across 5 waves as claimed, and the compile-order "Depends on" edges I traced (T5→T3,T4; T7→T3,T4; T6→T2,T3,T4,T5) are real — each is a genuine signature-change-forces-recompile dependency, not a guess.

## Findings

### 1 (medium) — No task actually wires Webster's standalone CLI entry

The doc's own "What it is" section states the goal covers `lyx burler run`, `lyx perch run`, and "eventually for Webster." T6 adds the `PersistentPreRunE` branch-around-`Resolve` plus `--stencils-dir` — but only for `internal/burlercli` and `internal/perchcli`. T7 converts `websterengine`/`webstercli` off `*lyxcwd.Location`, which is necessary but not sufficient: nothing in the ten-task list gives `webstercli` the T6-equivalent branch. After all ten tasks land, `lyx webster run` (if that standalone invocation is meant to exist) would still hard-fail in its own pre-run exactly as it does today.

T7's brief does say Webster's work is "deprioritised relative to burler/perch for the standalone goal," which reads as acknowledging this — but it stops short of saying so explicitly. Worth one sentence in T7 (or a note in "What is deliberately not in scope") stating plainly that Webster's standalone CLI entry is out of scope for this decomposition and left for a follow-up task, so a future reader doesn't assume T7 alone finishes the job the intro promised for Webster.

### 2 (low) — T10's dependency list omits T8

T10's brief is to land the three-tier invariant in `CONSTRAINTS.md`, which is the model T8 makes real by lifting the orchestrator-agnostic preflight out of `loomengine`. T10's `Depends on` line reads "T1 through T7 (T9 optional)" — T8 is missing. Every other task's dependency list in this doc is reasoned precisely (compile-order only, e.g. T5's citing the exact call site that forces the edge), which makes this omission stand out rather than read as deliberate. Wave placement saves it in practice (T8 is wave 2, T10 is wave 5, so nothing actually races) — but worth a one-word fix for consistency before this becomes a wiki task, since a task's "Depends on" line is presumably what a future spawn-ordering check reads.

### 3 (low) — Testing section's link-coverage claim is narrower than it reads

`discussion.md`'s Testing section says the markdown-link test "walks `manifest/`... Both new/changed files are inside that walk." Confirmed accurate for the two `manifest/` files, but `TestEnforcement_MarkdownLinks` walks only `{"manifest", "docs"}` (`docslink_test.go:406`) — `_mill/` is never in scope. `discussion.md` itself carries three relative links (to the design doc, to the roadmap, and to `CONSTRAINTS.md#producer-pointer-rule-invariant`); none of them are mechanically checked by the cited command. I hand-verified all three resolve (the anchor exists at `CONSTRAINTS.md:466`), so there is no live defect — but the sentence as written could be misread as certifying `discussion.md`'s own links, which it does not. Minor rewording would close the gap between what's claimed and what the command actually proves.

## Not re-litigated

The "Corrections to the originating discovery task" section is itself doing review work — it already documents five of its own predecessor's errors with evidence, and I independently confirmed the two highest-stakes ones (the `Resolve` gate, and scout's precedent status) rather than trusting the correction on its own say-so. I did not re-verify all five; the two checked were the ones the doc itself flags as the ones that "would have misdirected the work badly," which is also where independent verification buys the most.
