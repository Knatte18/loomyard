MILL_REVIEW_BEGIN
# Review: finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: claude-opus-5 (best-effort; behaviourally an Opus-class Claude)
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [BLOCKING:design] Allowlist self-expiry misses renamed-away files
**Section:** `### allowlist-is-keyed-and-self-expiring`
**Issue:** Stale detection is defined only as "an allowlisted link now resolves", but task B renames `docs/reference/plan-format-v3.md` to `plan-format.md` (`shed-followups.md:206`), so the two entries keyed on that filename are never visited by the walk and linger silently forever — the exact failure the decision claims to prevent, affecting 2 of 7 entries.
**Fix:** Define staleness as "allowlist key not matched by any scanned broken link", covering both the now-resolves case and the file-no-longer-exists case.

### [BLOCKING:design] The invariant citation stays invisible to the new checker
**Section:** `### weft-git-invariant-citation` vs `### link-check-is-a-permanent-go-test`
**Issue:** The rationale for the permanent test rests on the spec's claim that it "would have caught ... the non-existent Weft Git Invariant citation", but the chosen repair is a by-name prose citation, which the checker cannot see (the discussion itself notes prose mentions are invisible, under `constraints-entry`); the rejected-alternatives list weighs only name-vs-line-number.
**Fix:** Either decide on a real markdown link (e.g. `../../CONSTRAINTS.md#fabric-git-invariant-warp--weft`, which the checker would verify since `finalize.md` is inside the walk) or state explicitly that this break class remains uncovered and drop the premise from the test's rationale.

### [NIT:decision] Target form for the 5 scout sites unspecified
**Section:** `### scout-redesign-target-is-the-package-doc`, repair items 6-10
**Issue:** Items 1-2 give a concrete link path (`../../internal/fabricengine/doc.go`), items 6-10 say only "`internal/scoutengine`'s package doc"; the cited precedent, `roadmap.md:219`, is a plain prose mention with no link at all, while `raddle.md:29/60/84` uses the linked form — so the implementer can legitimately produce either.
**Fix:** State whether the five sites become markdown links to `../../internal/scoutengine/doc.go` or unlinked prose mentions.

### [NIT:scope] `finalize.md:12` carries the same rename residue as `:11`
**Section:** "`finalize.md` residue inventory"
**Issue:** The inventory is headed "Confirmed residue as of this branch" and lists `:11`'s "absorbed into `fabric` once that lands", but omits `:12`'s adjacent "that's `warp cleanup`'s (future: `fabric`'s) existing, separate job" — the shipped spelling is `lyx fabric cleanup` (`internal/fabriccli/fabric.go:249`), so the doc names a command that does not exist.
**Fix:** Add `:12` to the inventory so the end-to-end re-read has it on record rather than relying on discovery.

### [NIT:design] Checker grammar leaves an open implementer choice
**Section:** `### constraints-entry`, "Link-checker implementation notes"
**Issue:** Reference-style links and `<...>` autolinks are "out of the checker's grammar unless the implementer chooses to include them", so the CONSTRAINTS.md honesty paragraph cannot be written until that choice is made; fenced-code-block handling and GitHub's duplicate-heading `-1` suffix are unstated (the `.scratch/linkcheck.py` reference impl handles neither).
**Fix:** Fix the grammar decision here (inline links only is a fine answer) and say whether fenced blocks are skipped.

### [NIT:consistency] Allowlist entry count stated three ways
**Section:** `### allowlist-is-keyed-and-self-expiring`, Scope, Testing
**Issue:** Scope says "allowlist for the 8 remaining breaks", the decision heading says "The 8 entries, with owners", the table has 7 rows, and Testing/Technical context both say 7 entries; only the trailing note reconciles them.
**Fix:** Say "7 entries covering 8 link instances" consistently in all four places.

### [NIT:consistency] Test placement lacks the not-an-ownership-claim caveat
**Section:** `### link-check-is-a-permanent-go-test`, `### constraints-entry`
**Issue:** The Cwd Resolution Invariant says `internal/lyxcwd` owns cwd resolution "and nothing else"; the Fabric Vocabulary Invariant handles the same tension by stating in CONSTRAINTS.md that its placement there is file-layout convenience, not ownership — the new Markdown Link Integrity section is not asked to carry the equivalent sentence.
**Fix:** Require the new CONSTRAINTS.md section to state the same placement caveat.

## Verdict

REQUEST_CHANGES
Allowlist expiry gap and an uncheckable invariant citation need deciding before plan writing.
MILL_REVIEW_END
