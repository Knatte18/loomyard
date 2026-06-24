This confirms the contradiction. overview.md line 112 states `_board/` lives in the **Weft worktree** (i.e. `<prime>-weft/_board/`), while the discussion clones board to `<name>-HUB/_board/` (Hub top-level, sibling of the weft Prime). The topology diagram (lines 93-99) doesn't even show `_board` as a Hub child. This is a genuine GAP: the discussion's board placement directly contradicts the durable overview.md weft-overlay model, and the discussion doesn't acknowledge or reconcile this. Either overview.md needs amending too, or the board belongs inside the weft worktree.

I have enough to write the review. Let me compile findings.

Summary of verification:
- paths geometry claims (Hub, PrimeName, WeftRepoRoot): VERIFIED correct.
- git.RunGit signature: VERIFIED.
- output.Ok/Err: VERIFIED.
- main.go switch (8 cases listed): VERIFIED exactly.
- unexported weft helpers: VERIFIED unexported.
- enforcement_test: VERIFIED bans `os.Getwd`/`--show-toplevel`.
- Roadmap milestone 6 (line 90): says "skill" + "wiring host↔weft junctions" — CONTRADICTS discussion; discussion's roadmap-amendment scope only covers line 191's derivation bullet, not line 90.
- overview.md line 112: `_board/` in Weft worktree — CONTRADICTS discussion's Hub-top-level `_board` placement.
- `_board` not in `internal/paths` geometry — claim overstated.

MILL_REVIEW_BEGIN
# Review: ly-git-clone hub-creator (host, weft, board)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-24
```

## Findings

### [GAP] Board placement contradicts overview.md weft model
**Section:** Scope (In) / Decision hub-and-prime-naming
**Issue:** Discussion clones board to Hub top-level `<name>-HUB/_board/` (sibling of the weft Prime), but `docs/overview.md` line 112 states `_board/` lives **inside the Weft worktree** (`<prime>-weft/_board/`), and the topology diagram (lines 93-99) does not list `_board` as a Hub child — an unreconciled contradiction with the durable design doc.
**Fix:** Decide the canonical board location and state whether overview.md is amended (or fix the placement to match it); the plan writer cannot pick correctly as-is.

### [GAP] Roadmap milestone-6 text still says "skill" + "junctions"
**Section:** Scope (In) — "one-line roadmap amendment"
**Issue:** The scoped amendment only touches the out-of-scope derivation bullet (`roadmap.md` line 191), but milestone 6 itself (line 90) still describes the deliverable as a `ly-*` **skill** that wires "host↔weft junctions" — both directly contradicted by this task (Go command, no junctions).
**Fix:** Add amending milestone-6's line 90 (drop "skill", drop junction-wiring) to the in-scope roadmap edits, or explicitly defer it.

### [NOTE] `_board` is not part of `internal/paths` geometry
**Section:** Decision hub-and-prime-naming (rationale)
**Issue:** Rationale claims `<name>` / `<name>-weft` / `_board` are "exactly what `internal/paths` geometry expects," but `internal/paths` has no `_board` accessor — it only derives host (`PrimeName`) and weft (`WeftRepoRoot`); board location is config-driven (`board.yaml` `Path`), resolved at activation, not by geometry.
**Fix:** Soften the rationale to note `_board` is a convention the activation config later points at, not a geometry-resolved path.

### [NOTE] Task slug `ly-git-clone` vs deliverable `lyx git-clone`
**Section:** front-matter slug / Decisions
**Issue:** The slug/title is `ly-git-clone` (skill-namespace `ly-*`) while the actual deliverable is the `lyx git-clone` subcommand; harmless but invites confusion with the deferred milestone-19 skill.
**Fix:** One sentence clarifying the slug is historical (matches the old skill plan) and the artifact is the `lyx` subcommand.

## Verdict
GAPS_FOUND
Board placement and the milestone-6 roadmap text contradict durable docs; resolve before planning.
MILL_REVIEW_END
