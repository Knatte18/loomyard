MILL_REVIEW_BEGIN
# Review: prowler: installable Claude Code plugin (Go), hosted in LoomYard — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetmax
reviewer_self_id: Claude Sonnet 5 (model id claude-sonnet-5), per system self-report
reviewed_file: plan/
date: 2026-07-30
```

## Findings

### [BLOCKING] Versioned marketplace `source` contradicts every real example checked
**Location:** discussion.md "plugin-placement-and-marketplace" decision; Batch 2 Card 13 (`.claude-plugin/marketplace.json`) + the "Install-path verification is manual" batch-local decision.
**Issue:** The plan defaults to `"source": "./plugins/prowler/1.0.0"` (versioned), gated only by a manual post-ship `/plugin install` check with a fallback documented. I read two real, locally-available marketplace manifests to check this platform claim: the official Anthropic marketplace cache (`~/.claude/plugins/marketplaces/claude-plugins-official/.claude-plugin/marketplace.json`) has 50+ local-path `source` entries and every one is flat (`./plugins/<name>`), including several (`clangd-lsp`, `csharp-lsp`, `cwc-makers`) that carry an explicit sibling `"version"` field alongside the flat path — proving version and source-path are decoupled. weblens' own cached marketplace entry (`~/.claude/plugins/marketplaces/millhouse/.claude-plugin/marketplace.json`) is `"source": "./plugins/weblens"`, flat, despite `"version": "1.0.0"`. Zero real examples anywhere use a versioned-subdir local source.
**Fix:** Flip the default to the flat layout (`plugins/prowler/`, `source: ./plugins/prowler`) now, given this evidence, rather than shipping the versioned form and only discovering the failure at a manual, non-automatable gate the mill pipeline cannot run for you.

### [BLOCKING] Build-lock staleness reclaim has no ownership token — a live builder can be "stolen from," corrupting cleanup
**Location:** Batch 2, Card 8 (`run.sh`), steps 5, 6, and 9.
**Issue:** The stale-lock reclaim (mtime > ~120s → `rm -rf "$LOCK"` + retry) identifies the lock purely by path/age, not by owner. If a legitimate build simply runs long — plausible for a first-ever `go build` on a cold module cache fetching chromedp/goquery/go-readability, especially on the operator's Windows box (this repo's own benchmarks doc records ~4x AV-scanning overhead there) — a second waiter can misjudge the live build as orphaned, `rm -rf` its lock, and acquire a fresh one. The original (still-alive) builder's own `trap 'rm -rf "$LOCK"' EXIT` then fires unconditionally on its later exit, deleting the new holder's lock out from under it. Card 9's `selftest.sh` only exercises a synthetic no-live-owner staleness case (backdated mtime, no process behind it), so it would not catch this race.
**Fix:** Write an owner token (PID or random ID) into the lock dir at acquire time; both the staleness-reclaim check and the exit-trap cleanup must verify the token still matches before removing the directory.

### [NIT] "pinned exactly to weblens' proven pattern" overclaims for the SKILL.md invocation
**Location:** discussion.md "skill-contract" decision; Batch 2 Card 12.
**Issue:** The real weblens `SKILL.md` (`~/.claude/plugins/cache/millhouse/weblens/1.0.0/skills/weblens/SKILL.md`) invokes `bash ${CLAUDE_SKILL_DIR}/../../scripts/run.sh <urls> > _millhouse/scratch/weblens-output.md` — redirecting run.sh's whole stdout to one fixed shared file, not capturing a path via `path=$(...)`. The `${CLAUDE_SKILL_DIR}/../../scripts/run.sh` hop and `$0`-self-locating `run.sh` are confirmed identical to the real file; only the capture mechanic differs, by prowler's own deliberate unique-output-file design.
**Fix:** Soften the prose to "the path-resolution hop is proven identical to weblens; `path=$(...)` capture is prowler's own addition," rather than claiming the whole invocation line is pinned exactly to weblens.

## Verdict

REQUEST_CHANGES
Marketplace-source evidence and a build-lock ownership gap need addressing before this plan ships.
MILL_REVIEW_END
