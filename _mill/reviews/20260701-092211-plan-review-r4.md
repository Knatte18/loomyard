MILL_REVIEW_BEGIN
# Review: Add lyx init --undo / deinit command — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-01
```

## Findings

### [BLOCKING] `TestUnwireJunctions_RealDirectoryGuard` likely never reaches the real-directory branch
**Location:** batch 03-warpengine-unwire-junctions, Card 7
**Issue:** The test pre-creates a real directory at `l.HostLyxLink(slug)` without ever calling `WireJunctions`/`Add` for that slug, so `l.WeftLyxDirFor(slug)` (the target) does not exist. Per Card 6's mandated check order, `unseedLyxJunction` resolves the target via `filepath.EvalSymlinks(target)` *before* calling `fslink.IsLink(link)` — so this test hits the "weft directory missing/unreachable" branch instead of the intended "host repo already contains a real directory" branch. Weak assertions (`assert an error`, `nothing touched`) let it pass without exercising the guard it's named for.
**Fix:** Have the test create (or `Add`) the corresponding weft-side target directory first so `EvalSymlinks(target)` succeeds, then create the real host-side directory, so the test genuinely exercises the `fslink.IsLink == false` branch per the load-bearing check-order decision in the batch scope.

### [NIT] `init`'s `Short` stays silent on `--undo` while behavior becomes bidirectional
**Location:** batch 04-initcli-undo, Card 8
**Issue:** `Short: "scaffold _lyx/config/ in the current directory"` no longer fully describes the command once `--undo` reverses that scaffolding; the plan justifies this via `configcli`'s `--print`/`--apply` precedent, but that precedent itself only documents flags in `Long`, not a fully bidirectional verb change.
**Fix:** Consider a small `Short` tweak (e.g. append "(or reverse it with --undo)") for discoverability in `--help` subcommand listings, though this is stylistic, not a correctness issue.

## Verdict

REQUEST_CHANGES
Card 7's RealDirectoryGuard test setup doesn't create a resolvable weft target, so it likely misses its intended branch.
MILL_REVIEW_END