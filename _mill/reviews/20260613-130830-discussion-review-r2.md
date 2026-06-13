I've verified all the key claims. The roadmap confirms worktree is milestone 4 (discussion correct); the doc's "milestone 5" header is stale, which the discussion already flags as a follow-up. Now I have enough to assess the discussion holistically.

Key observations:
1. `internal/board/config.go` `LoadConfig` re-wraps "not initialized" — discussion correctly mirrors this for worktree (line 286). Verified accurate.
2. `config.Load` emits `not initialized: _mhgo/ directory not found in %s` — discussion says `not initialized: _mhgo/ directory not found`. Accurate enough.
3. `RunGit` signature matches discussion exactly.
4. `output.Ok`/`Err` match. Note: `output.Ok` injects `"ok":true` and ignores marshal errors.
5. main.go dispatch + doc comment Modules list — discussion correctly notes muxpoc is named `muxpoc` (line 15), discussion says "`muxpoc`" — accurate.
6. The `--force` flag for remove: discussion doesn't specify *how* it's parsed (flag package vs positional). Board/muxpoc use `flag.NewFlagSet`. The discussion describes RunCLI signature but never states whether `--force` and the `<slug>` positional are parsed via flag package, and flag ordering (flags-before-positional) is a known Go `flag` gotcha.

Let me check one more thing: whether the discussion addresses the `list` porcelain `bare` line / worktree with no branch, and the dirty-check edge case for the source path when run from the hub. Let me also verify config defaults map structure for worktree (only branch_prefix).

The discussion is thorough. Let me finalize my assessment of gaps.

Key potential gaps:
- **`--force` / slug argument parsing unspecified**: Go's `flag` package stops parsing flags at the first non-flag argument, so `remove <slug> --force` would treat `--force` as positional unless reordered. The discussion specifies the behavior but not the parsing mechanism or argument order tolerance. This is a real feasibility/ambiguity issue given board/muxpoc both use `flag.NewFlagSet`.
- **`list` porcelain `branch` mapping**: porcelain emits `branch refs/heads/<name>`. The discussion says `branch <ref>` → `"branch"` but doesn't say whether to strip `refs/heads/`. Ambiguous output shape.
- **`bare` worktree line**: `git worktree list --porcelain` can emit a `bare` line for bare main repos. Not addressed, but bare repos are out of the intended use case — minor.

Let me verify the porcelain branch format concern is real and not already disambiguated.

The discussion at line 149 says `branch <ref>` → `"branch"`. Git's porcelain emits `branch refs/heads/main`. The JSON example at line 214 shows `"branch":"..."` without indicating stripping. This is a genuine ambiguity for a plan writer.

I now have enough to write the review.

MILL_REVIEW_BEGIN
# Review: mhgo-worktree-module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-13
```

## Findings

### [GAP] `--force` / slug argument parsing unspecified
**Section:** Decisions › `remove` checks for uncommitted changes; Technical context › cli.go
**Issue:** `RunCLI` signature is given, but how `remove` parses the `<slug>` positional plus `--force` is undefined; Go's `flag` package (used by board and muxpoc) stops flag parsing at the first non-flag arg, so `remove <slug> --force` would treat `--force` as positional unless explicitly reordered.
**Fix:** State the parsing mechanism (flag.NewFlagSet vs manual scan) and whether `--force` is accepted in any position relative to the slug.

### [GAP] `list` porcelain branch ref not normalized
**Section:** Decisions › `list` is a thin wrapper; Technical context › JSON output shapes
**Issue:** Porcelain emits `branch refs/heads/<name>`, but the spec maps `branch <ref>` → `"branch"` and the JSON example shows `"branch":"..."` without saying whether `refs/heads/` is stripped, leaving the output shape ambiguous for tests asserting `branch == "master"`.
**Fix:** Specify whether `"branch"` is the full ref (`refs/heads/main`) or the short name, and align the `list_test.go` shape assertion accordingly.

### [NOTE] `bare` main-worktree porcelain line unhandled
**Section:** Decisions › `list` is a thin wrapper
**Issue:** `git worktree list --porcelain` emits a `bare` line (and omits `branch`) for a bare main repo; the block-parsing rules cover `worktree`/`HEAD`/`branch`/`detached` only, so a `bare` line's mapping is undefined. Out of the intended use case but unspecified.
**Fix:** Note `bare` is out of scope, or define its JSON handling.

### [NOTE] Source-worktree dirty check vs hub path resolution
**Section:** Decisions › `add` fails if source is dirty; Technical context › Path resolution
**Issue:** `add` is cwd-authoritative and the dirty check runs against `os.Getwd()`; the discussion correctly says source can be any worktree, but does not state behavior when cwd is the bare-container directory or `_board` (i.e. not itself a worktree), where `git status` would resolve to the hub or fail.
**Fix:** State that `add` requires cwd to be a valid checkout and how a non-worktree cwd is reported.

## Verdict

GAPS_FOUND
Two argument/output ambiguities (`--force` parsing, porcelain branch ref) need resolution before plan writing.
MILL_REVIEW_END
