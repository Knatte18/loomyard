# Gate runs: new binaries and launches per tier, 2026-10-31

## Verdict

A whole-suite gate run on an unchanged tree launched 143 test binaries that were new to the Go test cache (86 in tier0, 50 in tier1, 7 in tier2), and a cold cache would build 532.
Priced at about 10 s per new binary, the whole-suite run costs about 1430 s of scanner hold on OSL-1046, against 42.8 s of measured launch time on Neve.
The largest cut is to stop linking one test binary per package per tier: one binary per package for all three tiers saves 354 of 532 cold, and a single change to a widely imported package rebuilds about one third of all test binaries per tier.

## Method

Two gate runs on Neve, each measured once, from the task worktree, with the hub's gate config (`go` scoper, `-trimpath -p=4`, tier0 to tier2; tier3 is manual and never ran):

- Run 1: `lym gate test --wait --upto 2 ./internal/gateslot ./internal/gatecli`, a two-package scope.
  The branch had no diff, so a default diff scope would have run no tier.
- Run 2: `lym gate test --wait --upto 2 --all`, the whole suite.

Observed numbers come from the tier records and logs under the board's gate run directory and from `lym gate report`.
Derived numbers come from `go list -tags tierN -test` over the module (package and dependency graph only, no build) and from reading the test kits' source.
A raw module-wide `go build` or `go test` was not run, so what `go build ./...` links is not observed.

A test binary is launched only when the Go test cache misses: the binary is linked to compute its cache key, but a hit never executes it.
The count of launched test binaries per tier is therefore the number of packages the tier's log shows without `(cached)`, and it matches the number of `launches` entries in the tier record.

## Cache state

Neve's build and test cache was warm from earlier runs of this module, with the same `-trimpath` flags.
The 143 are therefore a lower bound for a run after a toolchain change or on a cold machine, and the 532 below is the cold upper bound.
Under `-trimpath` an unchanged package links to the same content, so a binary the scanner has seen once is known the next time; only content that changed since the last gate run on the machine is new.

## Count per tier

Run 2, observed:

| Tier | Test packages | Cached (not launched) | Launched, new to the cache | `launch_seconds` | Wall |
|---|---|---|---|---|---|
| tier0 | 176 | 90 | 86 | 5.9 | 22.2 s |
| tier1 | 175 | 125 | 50 | 31.9 | 125.5 s |
| tier2 | 175 | 168 | 7 | 5.0 | 51.7 s |
| Total | | 383 | 143 | 42.8 | 199.4 s |

Run 1, observed: tier0 and tier2 launched no test binary (all cached), and tier1 launched one (`internal/gatecli`, 0.12 s of launch time, 15.5 s wall for the three tiers).

Derived, the packages that link a test binary per tier on a cold cache: 178 in tier0, 177 in tier1 and 177 in tier2, 532 in all.
The tiers select disjoint test files by build tag, so the three binaries of one package differ and no tier reuses another's.

Other starts per `--upto 2 --all` call, derived from `internal/gatecli` and the tier commands:

| Start | New binary? | Count per call |
|---|---|---|
| `go build ./...` (tier1) | not observed; up to 11 `main` packages, which the toolchain may compile without linking | 1 start |
| `go vet -tags tier0` to `tier3` (tier1) | no, the `vet` tool is the toolchain's own | 4 starts, each running `vet` once per package |
| Prebuilt `lym` (tiers 1 and 2 each build it) | one per changed tree, the same content for both tiers and for later runs | 2 builds, 1 new |
| `lymbin.BuildWithLDFlags` stamped build in `internal/reedcli` (tier2) | yes, always compiled, never the prebuilt one | 1 |
| `cmd/lym` cross-compile test (`go build -o /dev/null ./...`, tier1) | not observed, likely a discarded result | 1 start |
| Tests that spawn the prebuilt `lym` | no, a known hash after the build | many |

## What decides the number of new binaries

A test binary is new when its package or anything it imports changed since the machine last ran it.
The reverse import closure of the changed package therefore sets the count, not the number of gate keys: a second gate key on the same tree finds the same hashes.

Derived from the module's graph, per tier, the number of test binaries that import a package:

| Measure | tier0 | tier1 | tier2 |
|---|---|---|---|
| Test binaries | 178 | 177 | 177 |
| Mean binaries a package change rebuilds | 32.5 | 34.6 | 33.1 |
| Median | 11 | 13 | 13 |
| Packages imported by 100 or more test binaries | 15 | 14 | 11 |
| Packages imported by every test binary | 5 | 5 | 5 |

The five packages imported by every test binary are `internal/testkit/tmuxkit`, `internal/proc`, `internal/lymtemp`, `internal/lock` and `internal/agentname`, through the package `TestMain` in each test package.
A change to any of them rebuilds all three tiers' binaries, 532 on a cold tree.
One change to a package of mean fan-in rebuilds about 100 binaries across the three tiers.

Sanity check against the board entry: 266 s of `launch_seconds` for `code-burler-fix` at about 10 s each is about 27 new binaries, which is the size of one tier's rebuild after a mid-fan-in change on a diff scope.
This agrees with the derivation but does not prove it.

## Candidate cuts, ranked by new binaries saved

Savings are per whole-tree rebuild (cold, 532) and per one change to a package of mean fan-in (about 100 across three tiers), then the OSL-1046 gain at about 10 s per binary on the second figure.

| Rank | Cut | Saved cold | Saved per mean change | OSL-1046 per change | Risk to result equality |
|---|---|---|---|---|---|
| 1 | One test binary for the whole module per tier | 529 | about 97 | about 970 s | High: the packages share one process, so global state, `TestMain` and `init` order cross packages, and the per-package cache is lost |
| 2 | One test binary per package for all three tiers, the tier chosen at run time | 354 | about 67 | about 670 s | Medium: the tier files must stop selecting by build tag, and the `TestMain` of tier2 (tmux) differs from tier0 |
| 3 | Merge small, leaf test packages into aggregate binaries within a tier | up to the merged count, not derived here | up to the merged count | proportional | Medium: the same crossing of global state, limited to leaf packages |
| 4 | Cut the five universal imports out of the common `TestMain` path | 532, only when one of the five changes | 0 for any other change | 0 typical | Low: tmux support moves behind a test-only build of the packages that need it |
| 5 | Drop the stamped `lym` build in `internal/reedcli` by testing the channel through a flag | 1 | 1 | 10 s | Low |
| 6 | Run the tier1 vet and build steps from one `go` call, and skip the tier2 prebuild repeat | 0 | 0 | 0 new binaries | None, and it saves only known-binary starts |

Reuse of a built test binary across the gate keys of one run is not a cut: it saves nothing, because the hashes of an unchanged tree are already known after the first key, and the gate's own carry already skips a green tier on an unchanged tree.

## Gain on OSL-1046

At about 10 s per new binary:

- Run 2 on an unchanged tree launched 143 new-to-cache binaries, about 1430 s of hold, against 42.8 s measured on Neve.
- A cold run, 532 binaries, is about 5320 s.
- A diff gate after a mid-fan-in change is about 100 binaries across the three tiers, about 1000 s, an upper figure beside the 266 s and 153 s of `launch_seconds` the board entry reports, since those diff gates reached fewer packages.
- Cut 2 takes the cold run to 178 binaries, about 1780 s, and the mean-change run to about 33 binaries, about 330 s.
- Cut 1 takes the mean-change run to 3 binaries, about 30 s.

## Limits

- One warm-cache run per scope on one machine; the cold figure is derived, not measured.
- The scanner's hold applies to a binary it has not seen, so a rebuild that reproduces an earlier hash costs nothing on OSL-1046; the per-change figures assume the changed package's closure has new content.
- What `go build ./...` and the cross-compile test link is unobserved.
