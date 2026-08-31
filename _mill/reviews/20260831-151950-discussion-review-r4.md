MILL_REVIEW_BEGIN
# Review: Prefer raw fetch, scope large tree listings

```yaml
duration_s: 143.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:design] No disposition for a non-empty directory path
**Section:** Decisions → "`github-read.sh` requires a non-empty path"
**Issue:** The only directory reasoning is for the *empty* path ("An empty path would address a directory"); nothing decides what `github-read.sh acme/x src` does when `src` is a real directory — raw answers non-2xx and hands off to the `gh api` contents fallback, whose non-2xx-only failure trigger does not cover a 2xx directory response, so the script's stated behaviour ("writes whatever bytes it receives to stdout", no body inspection) emits a directory payload as if it were file content, exit 0. This is the same silent-wrong-content failure the `-f` decision exists to prevent, and no harness scenario in the Testing section covers it.
**Fix:** Add a Decision stating the intended outcome for a path that names a directory (accept-as-is, or detect and `die`), and name the corresponding harness scenario.

### [NIT:consistency] `mill:conversation` is not a real skill reference
**Section:** Decisions → temp-file scope note (line 157); Constraints (line 335)
**Issue:** The cited authority for "never write to a system temp directory" is `mill:conversation`; the only conversation skill in the tree is `plugins/scribe/skills/conversation/SKILL.md` (`scribe:conversation`), whose rule text is scoped to "ephemeral files — drafts, scratch fixtures, debug dumps". The substance of the conclusion (harness scratches under `<repo>/.scratch/`) matches `github-tree-selftest.sh:46` and is correct.
**Fix:** Rename the citation to `scribe:conversation`.

### [NIT:consistency] Two source citations do not match the file
**Section:** Decisions → "Where the fallback's HTTP status comes from"; "The usage line and its exit code"
**Issue:** `github-tree.sh:135-138` is cited for the one-line body collapse and generic `failed (exit …): <body>` form; in the file the collapse is line 137 and the generic `die` is line 150 (`github-tree.sh:129-131` for the status regex is correct). Separately, test 16 is cited as asserting the exit-2/exit-1 distinction, but `github-tree-selftest.sh:356,362` assert only `status -ne 0`; test 21 (line 435) is the one that asserts `-eq 2`.
**Fix:** Correct both citations so the plan writer lands on the right lines.

## Verdict

REQUEST_CHANGES
One undecided failure mode: a path naming a directory has no stated behaviour.
MILL_REVIEW_END
