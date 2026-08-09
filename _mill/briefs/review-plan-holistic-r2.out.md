MILL_REVIEW_BEGIN
# Review: plan-format: drop the v3 suffix and sweep every reference by script — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-09
```

## Findings

### [NIT:consistency] Sweep leaves the renamed doc's own H1 title hyphenated inconsistently with the rest of the repo
**Location:** batch 1 card 2 (sweeper) / batch 2 card 5 (`docs/reference/plan-format.md` prose erasure)
**Issue:** `docs/reference/plan-format-v3.md:1` reads `# Plan format v3 — flat card list` (space, not hyphen, between "Plan" and "format" — this is the exact line the plan's own `roadmap-18`/pattern-count override cites as the reason the 6th sweep pattern `plan format v3` was added). Applying that pattern's replacement (`plan format` , lowercase tail, leading-char case preserved) turns the title into `# Plan format — flat card list`, while every other reference to the file/format in the same document and repo-wide (the filename itself, the pinned-status blockquote two lines below, `README.md`, `docs/overview.md`, etc.) uses the hyphenated compound `plan-format`. Card 5's seven numbered rewrite sites do not include the H1, so this self-inconsistency (title vs. body, in the same file) survives the task with no card assigned to fix it.
**Fix:** Add an eighth item to card 5 (or a line in card 4) rewriting the H1 to `# Plan-format — flat card list` (or equivalent hyphenated form) alongside the other prose sites.

### [NIT:consistency] Override note's "four citations" count for shed-followups.md's stale path is off by one
**Location:** batch 4 card 13, Block 1 item 2 (and the batch's own `## Batch Scope` restating the same claim)
**Issue:** Card 13 instructs writing "its four citations of the doc's pre-rename path" into the override note. `grep -ni 'plan-format-v3\.md' manifest/designs/shed-followups.md` returns five hits (lines 53, 74, 120, 192, 214), not four — only one of those (`:53`) carries the full `docs/reference/` prefix; the other four are bare `plan-format-v3.md` mentions. Whichever subset the plan intends, "four" does not match a plain grep of the artifact the note is describing, and this note is a durable record tasks C and E will read.
**Fix:** Re-run the grep during card 13 and state the verified count (five, or name which occurrence is deliberately excluded from the tally and why) rather than carrying forward the unverified "four."

## Verdict

REQUEST_CHANGES
Two NIT-level prose/count inaccuracies to fix; the mechanical design (DAG, file inventory, sweep patterns, ownership) checks out.
MILL_REVIEW_END
