MILL_REVIEW_BEGIN
# Review: Prefer raw fetch, scope large tree listings

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:design] Symlink disposition undefined on the raw path
**Section:** "A path naming a directory is a hard error, detected on the fallback path"
**Issue:** The die for `symlink`/`submodule` is reachable only through the fallback type probe, and the discussion states no premise about what `raw.githubusercontent.com` returns for a symlink — the directory case gets an explicit "raw answers non-2xx" argument, the symlink case gets none, so a raw 200 would emit the link target as file content, the same silent-wrong-content class the probe exists to eliminate.
**Fix:** State the raw-path disposition for symlinks explicitly — either an observed-non-2xx premise pinned by the same live-capture note used for the `gh api` shapes, or an accepted, recorded limitation that the symlink guard is fallback-only.

### [NIT:consistency] `$errfile` temp file has no creation/cleanup spec
**Section:** "Where the fallback's HTTP status comes from" vs "buffers the raw body to a temp file"
**Issue:** The buffering Decision specifies exactly one `mktemp` file with a `trap ... EXIT`; the status Decision introduces a second file, `$errfile`, whose creation and cleanup are never stated — while the testing plan asserts the scratch `TMPDIR` is *empty* after a failed read, which only holds if `errfile` is trapped too.
**Fix:** Say in the buffering Decision that both `$tmp` and `$errfile` come from `mktemp` and share one `trap`.

### [NIT:scope] Default-1000 and boundary fixtures unspecified in size/origin
**Section:** Testing → "Guard boundary", "Default ceiling is 1000"
**Issue:** Three scenarios require fixtures of ~1000/1001 tree entries, but the harness convention is checked-in raw response JSON under `testdata/github-tree/bodies/`, and the discussion does not say whether these are checked in (a large blob) or generated at harness runtime.
**Fix:** State which, since the existing convention would otherwise imply a multi-hundred-KB committed fixture.

### [NIT:decision] `--`-prefixed positional in `github-read.sh` unstated
**Section:** "Flag parsing accepts flags before positionals only" (github-tree.sh only)
**Issue:** `github-read.sh` is specified with two positionals and no flag parsing, so `github-read.sh acme/x --foo` passes validation (`-` is in the accepted set) and reaches the API, while the same token exits 2 in `github-tree.sh` — a cross-script divergence the discussion elsewhere argues explicitly against.
**Fix:** State the disposition — either mirror the leading-`--` usage-error rule, or record the divergence as accepted with its reason.

## Verdict

REQUEST_CHANGES
Symlink handling on the raw-success path is undecided; remaining findings are minor.
MILL_REVIEW_END
