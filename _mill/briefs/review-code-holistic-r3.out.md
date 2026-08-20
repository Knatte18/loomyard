MILL_REVIEW_BEGIN
# Review: Extract scout into its own standalone repo — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-20
```

## Findings

### [NIT:consistency] toolchain.go header still calls the cache "scout-owned"
**Location:** `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain.go:2`
**Issue:** The file header reads "installing) a pinned gopls binary into a scout-owned, machine-global cache directory," even though the package is `quarry` and the actual path segment (line 42) is `"quarry"`. This predates the port and wasn't caught because card 17's sweep targeted `lyx`, not `scout`, tokens.
**Fix:** Reword to "quarry-owned" to match the renamed cache segment and package identity.

### [NIT:consistency] registry.go doc comment cites the dropped `ConfigTemplate` symbol
**Location:** `/home/knatte/Code/quarry/wts/quarry/quarry/registry.go:58`
**Issue:** `builtins()`'s doc comment says "operator overrides live only in the seeded servers.yaml (see ConfigTemplate)," but `config-template-is-dropped-not-ported` deliberately removed `ConfigTemplate` from quarry (content lives at `docs/servers.yaml.example` instead). This dangling reference wasn't caught by any card's grep, since it names neither `lyx` nor `scout`.
**Fix:** Reword the parenthetical to point at `docs/servers.yaml.example` instead of the nonexistent `ConfigTemplate`.

### [NIT:consistency] `resolveContext`'s `cwd` parameter is now dead
**Location:** `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go:462-488`
**Issue:** Card 26 specifies the signature `resolveContext(cwd, dir, configFlag, stateDirFlag string) (...)` carried over from `lookupContext(cwd, dir string)`, but once the in-hub branch (the only consumer of `cwd`) was deleted, the function body never references `cwd` at all — it's an unused parameter that compiles fine but is vestigial, and could trip a stricter linter (`unparam`) under card 47's optional `golangci-lint run`.
**Fix:** Drop `cwd` from `resolveContext`'s signature and its four call sites, or note in a comment why it's kept for signature symmetry with the (now-deleted) `lookupContext`.

### [NIT:consistency] `docs/research/quarry-holistic-fix-log.md` has no Round 2 entry
**Location:** `/home/knatte/Code/loomyard/wts/scout-extract-standalone-repo/docs/research/quarry-holistic-fix-log.md:6`
**Issue:** The file's own header states "Each round that lands quarry-side fixes appends its own `## Round N` section here." Round 2's fixes landed quarry-side changes (`lspclient.go`'s `defaultLogHandler.Warn` fix, `docs/port-equivalence.md`'s comparison-count fix — both confirmed present in the current quarry tree) but only a `## Round 1` section exists in this log.
**Fix:** Append a `## Round 2` section recording round 2's quarry-side fixes and commit SHAs, per the file's own stated convention.

## Verdict

APPROVE
Extensive cross-batch verification (paths, cli, engine, daemon-state, seam guards, equivalence proof, lyx-side removal) found only minor doc/comment residue, no BLOCKING issues.
MILL_REVIEW_END
