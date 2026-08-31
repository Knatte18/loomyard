MILL_REVIEW_BEGIN
# Review: Prefer raw fetch, scope large tree listings — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-31
```

## Findings

### [BLOCKING:design] emit()/output unreachable in the --children branch
**Location:** Batch 1, Card 2 vs Card 3.
**Issue:** Card 2 declares `output=()` and the `emit` function together with `queue=()`/`head=0` — the exact three-line block Card 3 says the `--children` branch bypasses entirely, inserting its branch "immediately before" it — yet Card 3's own branch requires calling `emit` for every listed entry. Read literally and in card order, a `--children` run reaches a branch where neither `output` nor the `emit` function has been defined yet, since both sit textually inside the still-unreached recursive-walk arm.
**Fix:** State explicitly that `output=()` and the `emit` function's declaration are hoisted above the `CHILDREN` branch point and shared by both arms, and that only `queue=()`/`head=0` remain inside the recursive-only arm.

### [BLOCKING:design] Real private repo names committed via CAPTURE.md
**Location:** Batch 2, Card 8.
**Issue:** Card 8 names specific private repositories (`Knatte18/enk-hub`, `Knatte18/fellesutgifter`) as live-capture targets and requires writing "the command run" for each capture into `plugins/prowler/scripts/testdata/github-read/CAPTURE.md`, which is tracked testdata shipped with an installable plugin (`README.md`: "ships as an installable Claude Code plugin"). This permanently discloses the names/existence of private personal repositories in shipped, version-controlled content, and the plan states no decision to redact them.
**Fix:** Have Card 8 record captures with repo identifiers genericized (e.g. `<private-repo>`) in `CAPTURE.md` rather than embedding the literal owner/repo strings.

### [NIT:scope] Card 6 omits the stub-gh context it depends on
**Location:** Batch 1, Card 6.
**Issue:** Card 6's guard scenarios route two generated large fixtures through the absolute-path mechanism Card 4 added to `testdata/github-tree/bin/gh`, and the `--children`-ceiling scenario reuses `trunc1-root-nonrec.json` under the same stub's dispatch rules, but `testdata/github-tree/bin/gh` is not in Card 6's `Context:` (Card 5's is, for the identical dependency).
**Fix:** Add `plugins/prowler/scripts/testdata/github-tree/bin/gh` to Card 6's `Context:` for consistency with Card 5.

## Verdict

REQUEST_CHANGES
Two BLOCKING design gaps: an emit/output ordering conflict between Cards 2–3, and private repo names committed via Card 8's CAPTURE.md.
MILL_REVIEW_END
