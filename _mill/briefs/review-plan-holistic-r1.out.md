I have thoroughly reviewed all plan files and source. The plan is well-grounded. Let me note the few minor concerns I found.

One genuine concern worth flagging: In Card 11 Remove step (2), the dirty gate runs `git status --porcelain` but the test in Card 12 case 3 (dirty with force) — when `force` is true, the dirty gate is skipped, so `git worktree remove --force` is used. That's correct. However, there's a subtle issue: when `removeLinks(target)` returns an error in step (3), it's propagated, but for the `git worktree add`-created worktree the `.git` file inside it is a regular file (not a symlink), so `removeLinks` won't touch it. Good.

Another: Card 9 List error on `bare` block — discussion confirms. Good.

The findings are minor (NITs). Let me write the review.

MILL_REVIEW_BEGIN
# Review: Build mhgo worktree module — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-13
```

## Findings

### [NIT] addRemote omits initial-branch push present in discussion
**Location:** Batch 3, Card 6 (addRemote)
**Issue:** discussion.md's `addRemote` pushes the default branch (`git push -u origin master/main`); the plan's helper only adds the remote. This is intentional and harmless (Add's happy-path push of a fresh branch to an empty bare repo still works), but diverges silently from the discussion sketch.
**Fix:** None required; optionally note in the card that the initial push is deliberately dropped.

### [NIT] Card 13 loads config before validating subcommand presence
**Location:** Batch 4, Card 13 (RunCLI)
**Issue:** `LoadConfig(cwd,"worktree")` runs before the `len(args) >= 1` usage check, so a bare `worktree` with no `_mhgo/` reports the config error rather than a usage line. This is consistent with how Card 16's routing test depends on the config-error envelope, so it is fine, but the ordering is the opposite of the usage-first phrasing in the card.
**Fix:** None required; the behavior is internally consistent and exploited by Card 16.

### [NIT] removeLinks Windows junction mode bit
**Location:** Batch 2, Card 4 (removeLinks)
**Issue:** Some Go versions report NTFS junctions as `ModeSymlink|ModeIrregular`; the `&os.ModeSymlink != 0` test still passes, but the card asserts junctions are reported "as symlinks" without the caveat.
**Fix:** None required; the bitmask check is correct as written.

## Verdict

APPROVE
Plan is complete, well-sequenced, source-grounded, and faithful to all decisions; only cosmetic nits.
MILL_REVIEW_END
