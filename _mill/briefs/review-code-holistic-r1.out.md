MILL_REVIEW_BEGIN
# Review: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-08
```

## Findings

### [BLOCKING] `lyx fabric commit --help` still claims the pathspec default is `_lyx _pattern`
**Location:** `internal/fabriccli/weft_verbs.go:147`
**Issue:** The `commit` subcommand's `Long` text reads "Staging is scoped to the directories listed in the fabric config (default: `_lyx _pattern`)." Card 42 shrank `template.yaml`'s default to `_pattern` alone and made `_lyx` a structural, code-injected member (never "listed in the fabric config"); card 52 explicitly fixed the equivalent claim in `docs/overview.md` but this CLI-observable `Long` string was missed. This violates the CLI/Cobra Invariant's "Help accuracy is a review obligation" clause for an observable-behaviour change this exact task makes.
**Fix:** Reword the `Long` text to state `_lyx`/`.lyx` are structural (code-injected, never in `fabric.yaml`) and only `_pattern` is the configurable/optional default.

## NIT

### [NIT] `buildercli`/`webstercli` sync.go header comments describe a retired mechanism
**Location:** `internal/buildercli/sync.go:5-8`, `internal/webstercli/sync.go:5-8`
**Issue:** Both file-header comments still say machine-local runtime artifacts "are excluded solely by the fabric repo's `.git/info/exclude`, seeded by fabricengine's own artifact-exclude bootstrapping." Batch 6 deleted that exact mechanism (`crossModuleMachineLocalExcludes`), and batches 2-3 moved these artifacts entirely under `.lyx`, outside the `_lyx` pathspec, for a more fundamental reason than exclusion. Card 35 fixed the identical claim in `docs/reference/builder-contract.md` but missed these two source comments — `perchcli/run.go`'s equivalent comment (card 13) was correctly rewritten, making the omission here inconsistent within the same batch pattern.
**Fix:** Reword both comments to say the artifacts live under `.lyx` and never reach the `_lyx`-scoped pathspec in the first place, mirroring `perchcli/run.go`'s corrected comment.

## Verdict

REQUEST_CHANGES
One stale CLI `--help` string misdescribes the new structural pathspec default; everything else checked out clean.
MILL_REVIEW_END
