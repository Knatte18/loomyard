MILL_REVIEW_BEGIN
# Review: Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise

```yaml
duration_s: 186.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude Opus 4-class model (self-assessed; exact version not verifiable from inside the session)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [NIT:scope] `_mill/` swept in by the whole-tree rule, no disposition
**Demoted-from:** BLOCKING
**Section:** § Scope ("anywhere in the repository tree") + § Decisions / repo-wide-in-one-commit
**Issue:** `_mill/` is tracked (`.gitignore` excludes only `**/_mill/*.active`), so the stated rule sweeps in `_mill/discussion.md`, `_mill/status.md`, `_mill/reviews/*`, `_mill/briefs/*` — ~70 of the 202 `lyx loom run|drive|ly-supervise` hits across 13 files — and the three dispositions (rename / leave / structural) are all framed around product code and docs, with `historical-prose-rewritten-not-glossed` arguing *for* rewriting retrospective records; the "~233 matches across 57 files" figure appears to have been counted with `_mill` already excluded, which the rule never says.
**Fix:** State a disposition for mill's own task artifacts (and any other meta/archival tree such as `crucible/`, `docs/research/`) — normally a blanket "leave, they record the task, not the tree" — as an explicit carve-out in the Scope rule rather than leaving it to the classify step.

### [NIT:design] `/proc` argv probe loses its unique discriminator
**Section:** § Decisions / split-argv-sites-are-scope-targets, disposition 2
**Issue:** `findDriverPIDs` (`internal/loomcli/smoke_test.go:249-279`) filters on `cwd == worktree` and a single argv element equal to `"drive"`; `"drive"` is unique to the loom driver, whereas `"run"` is a verb on `shuttle`, `burler`, `webster` and on loom itself, so the post-rename probe matches any such process sharing the worktree cwd — the discussion names only the reverse hazard (a token no process carries).
**Fix:** Say in the decision that the probe's discriminator must stay unique — e.g. require the adjacent `"loom"`+`"run"` argv pair rather than a lone `"run"` element.

## Verdict

APPROVE
Scope rule leaves tracked `_mill/` artifacts without a disposition.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
