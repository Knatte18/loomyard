MILL_REVIEW_BEGIN
# Review: Rename the fabric host vocabulary to warp, and name the composite repo Fabric — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-09
```

## Findings

### [NIT] wordswap's byte-offset scan assumes ASCII-stable `strings.ToLower`
**Location:** `tools/wordswap/swap.go:117-136`
**Issue:** `swapText` computes `lowerIn := strings.ToLower(in)` once and then indexes into the original `in` using offsets found in `lowerIn`. For any rune whose lowercase mapping changes UTF-8 byte length (e.g. U+0130 İ → "i̇"), `lowerIn` and `in` desynchronize and subsequent offsets misalign, at best tripping `Result.Mismatch` (safe) and at worst risking a slice-bounds panic on `in[i:i+n]`/`in[pos:i+1]` before the reversibility check ever runs.
**Fix:** Not required for this task — none of the ~100 swept Go/shell/markdown files contain such runes, verified by the batch 3/6 sweeps landing cleanly with zero mismatches. Worth a follow-up guard (byte-length-preserving lowercase, or an explicit ASCII-only precondition documented in the package doc) before `wordswap` is reused on arbitrary non-ASCII content.

## Verdict

APPROVE
All seven batches verified faithful to plan, decisions, and CONSTRAINTS.md; no blocking issues found.
MILL_REVIEW_END
