MILL_REVIEW_BEGIN
# Review: Speed up git-fixture tests: bench, analyse, hardlink — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-13
```

## Findings

None. Verified across all four batches:

- Batch 1: the four builder test files carry `//go:build integration` as the
  first non-empty line, exactly the placement `isTierTagged` requires; no test
  logic changed; `gitquery_test.go` untouched.
- Batch 2: `lyxtest.go`'s `initRepo`/`initBareRemote` add the three quiet-config
  calls exactly where the card specifies; `hermetic.go` matches the required
  neutral-config content, `sync.Once` guard, and godoc contract (purpose, call
  site, accepted-leak note, rename warning) verbatim; `lyxtest_test.go` adds
  `TestMain` (unqualified call) plus both required behaviour tests, `t.Parallel()`
  on both.
  the config content matches the discussion's `neutral-global-config-contents`
  decision (identity + `init.defaultBranch` alongside the quiet trio).
- Batch 3: `hermeticenv_test.go` and `tierpurity_test.go` are mutually
  reciprocal (new guard file added to `allowedSpawners`); the hermetic guard's
  token set, allowlist semantics (file-level vs package-level exclusion),
  vacuous-scan floor, and deterministic failure-message ordering all match the
  card. All 22 `testmain_test.go` files use the canonical shape; package
  clauses verified against each directory's actual existing test files
  (internal form where both exist, e.g. `warpengine`/`warpcli`; external form
  where only external exists, e.g. `hubgeometry_test`, `gitexec_test`,
  `initcli_test`). `CONSTRAINTS.md` and `lyxtest/doc.go` both carry the new
  invariant with matching cross-references.
- Batch 4: `bench_test.go` is `//go:build integration`, four benchmarks
  (serial + `b.RunParallel`) exactly as specified, using `b.Loop()` (valid
  under this repo's `go 1.26`). The discovered-scope buildercli compile fix
  (`testdata_test.go`, `pause_spawnbatch_test.go`, edits to `validate_test.go`/
  `poll_test.go`/`status_test.go`/`pause_test.go`) is applied exactly as the
  card's discovered-scope note describes — cross-checked that
  `builderengineTestdataDir`/`seedPlanFixture`/`pollFakeMux` are defined
  exactly once (`testdata_test.go`) and consumed correctly by the now-untagged
  `run_test.go` and the still-tagged `poll_test.go`/`validate_test.go`;
  `smoke_test.go` correctly left untouched (out of scope, confirmed
  `//go:build smoke`). Docs (`fixture-copy.md`, `test-suite-timing.md`,
  `running-tests.md`) are internally consistent, cross-link each other, mark
  every number Windows-only, and `test-suite-timing.md` demotes the prior
  2026-07-12 block into History per the append-only discipline.
- Cross-batch: the `HermeticGitEnv` bare-name token choice is honored
  consistently everywhere (qualified form in all other packages, unqualified
  in `lyxtest`'s own test file, both correctly matched by the guard's raw
  substring). No duplicate helper/guard logic between batches. No out-of-plan
  files found — every source file in the manifest maps to a batch's
  Context/Edits/Creates list.

## Verdict

APPROVE
All four batches align with the plan, shared decisions, and CONSTRAINTS.md; no cross-batch contract breaks found.
MILL_REVIEW_END
