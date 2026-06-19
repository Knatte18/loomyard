Confirmed. The directory-tree block (L122-135) is in the `## Weft overlay model` section (after `### Status`), and contains four `see modules/X.md` comments. Card 1 explicitly excludes altering the Weft overlay model section's content, and Card 2 only handles the weft Status note and Path-Invariants method list — neither card fixes L126-129. The batch-2 broken-link grep `modules/board\.md|modules/worktree\.md|modules/ide\.md|modules/muxpoc\.md` will match these four lines, so the verification will report a failure that no card addresses.

This is a coverage gap. Also the `init` paragraph (L185) links to `modules/board.md#init`; Card 1 says keep the init paragraph but remove every link to modules/board.md — needs clarification on what the `#init` anchor retargets to.

I have enough to produce a thorough review. Let me compile findings.

MILL_REVIEW_BEGIN
# Review: Reconcile stale design docs (stateless + weft model) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-19
```

## Findings

### [BLOCKING] overview tree-block module links left dangling
**Location:** Batch 1, Card 1 (+ Card 2)
**Issue:** overview.md L126-129's directory-tree fenced block has `see modules/board.md`, `…/worktree.md`, `…/ide.md`, `…/muxpoc.md`, but it sits inside the `## Weft overlay model` section which Card 1 explicitly says not to alter and Card 2 does not touch; the batch-2 broken-link grep (`modules/board\.md|…`) will match these four lines, so verification fails with no card fixing them.
**Fix:** Add a requirement (Card 1 or Card 2) to strip the `(see modules/<x>.md)` comments from the L122-135 tree block, or update them to point at `#modules`.

### [BLOCKING] init paragraph link to modules/board.md#init unresolved
**Location:** Batch 1, Card 1
**Issue:** Card 1 says "Keep the `init` paragraph" yet also "remove every link to `modules/board.md`"; overview.md L185 is `[modules/board.md#init](modules/board.md#init)`, which the batch-2 grep flags and no requirement specifies a retarget for.
**Fix:** State explicitly that the init paragraph's `modules/board.md#init` link is dropped (describe init inline) or retargeted to `overview.md#modules`.

### [NIT] config.md bare "(see state.md)" not in grep scope
**Location:** Batch 2, Card 8
**Issue:** config.md L19 contains a bare textual `(see state.md)` that the batch-2 broken-link grep (`shared-libs/state\.md`) won't catch; it is only removed incidentally because Card 8 deletes the whole `.lyx/` L19-20 block.
**Fix:** None strictly needed (Card 8's line removal covers it); optionally note the bare reference explicitly so an implementer who keeps the line still removes it.

## Verdict

REQUEST_CHANGES
Two BLOCKING coverage gaps: overview tree-block and init-paragraph links to deleted module docs are unaddressed.
MILL_REVIEW_END