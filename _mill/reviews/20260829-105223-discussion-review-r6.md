MILL_REVIEW_BEGIN
# Review: prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call

```yaml
duration_s: 121.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-29
```

## Findings

### [BLOCKING:design] TSV safety rests on a false git-path premise
**Section:** "One API response, both fields — combined `--jq` stream"
**Issue:** The rationale states "git rejects a tab in a path component and a newline cannot appear in one either" — git's path validation rejects only NUL, `/`, and the `.`/`..`/`.git` component names; tabs and newlines are legal in tracked filenames and GitHub returns them JSON-escaped, so the parse-safety argument for `type<TAB>sha<TAB>path` (and for newline-separated stdout) is unfounded.
**Fix:** Restate the real constraint and record a decision: either accept it as a documented limitation like the path whitelist, or detect an entry whose path contains a tab/newline and fail loudly, with a fixture pinning whichever is chosen.

### [NIT:decision] `settings.json` / `plugin.json` disposition unstated
**Section:** Scope (Out) / Technical context
**Issue:** `plugins/prowler/settings.json` (permission allow-list `Bash(bash *)`, `Bash(go *)`) and `.claude-plugin/plugin.json` are never named; a plan writer cannot tell whether the new `bash "$TREE_SH" …` invocation needs an allow entry or a version bump.
**Fix:** Add one Out line saying both files are unchanged — `Bash(bash *)` already covers the invocation and no version bump is taken.

### [NIT:scope] Harness portability envelope unnamed
**Section:** "Offline harness with a stub `gh`" / Testing
**Issue:** `run.sh` explicitly handles Windows (`uname -s` → `.exe`), but the harness's stub-`gh`-on-`PATH` mechanism (extensionless executable, exec bit, `PATH=` for the gh-missing case) is never scoped to a platform, unlike `selftest.sh`'s header which documents what it deliberately cannot assert.
**Fix:** State the harness's supported platform(s) in the same "NOT covered here" note the technical context already asks it to mirror.

## Verdict

REQUEST_CHANGES
One core parsing decision rests on an incorrect claim about legal git path characters.
MILL_REVIEW_END
