# Effect of the test-speed cuts, 2026-11-28

Wall and test time of the packages the cuts changed, before and after, with both sides run under the same heavy load.
The data of every call is in [calls.tsv](calls.tsv).

- Side A is the tree of `ea00087ec`, before the cuts.
- Side B is `e9a8aa10f`, `main` after them.

## Findings

- The cuts show at tier1.
  The five tier1 calls took 2.52 minutes of wall time on A and 1.40 on B: 1.13 minutes, or 45 %, less.
- The clearest results are `internal/battencli` (62 % less wall time) and `internal/webstercli` (56 % less).
  In both, B was faster although its call was issued at the higher load.
- tier0 took 39.2 s on A and 26.3 s on B.
  This is not evidence of a cut: most of the difference is `cmd/lym` and `internal/loomcli`,
  and `cmd/lym` lost a test to another package and ran B at half A's load.
- Each package, tier and side ran once.
  A single difference of a few seconds, or of less than about 40 %, cannot be told from load.

## Wall time

Seconds from the call holding its gate slot to its end, slot wait excluded.
Load is the 1-minute load average when the call was issued.
There is one run per cell, so there is no mean over repeats.

| Package | Tier | A | Load A | B | Load B | B − A | B against A |
|---|---|---|---|---|---|---|---|
| `internal/gatecli` | 0 | 3.2 | 32 | 3.1 | 34 | −0.1 | −3 % |
| `internal/gatecli` | 1 | 39.6 | 25 | 31.2 | 24 | −8.4 | −21 % |
| `internal/battencli` | 0 | 2.8 | 23 | 2.4 | 28 | −0.4 | −14 % |
| `internal/battencli` | 1 | 44.4 | 27 | 16.8 | 34 | −27.6 | −62 % |
| `internal/webstercli` | 0 | 2.0 | 33 | 2.8 | 27 | +0.8 | +40 % |
| `internal/webstercli` | 1 | 24.3 | 22 | 10.6 | 31 | −13.7 | −56 % |
| `internal/loomcli` | 0 | 9.4 | 28 | 5.1 | 28 | −4.3 | −46 % |
| `internal/loomcli` | 1 | 21.5 | 38 | 12.0 | 24 | −9.5 | −44 % |
| `cmd/lym` | 0 | 21.8 | 35 | 12.9 | 17 | −8.9 | −41 % |
| `cmd/lym` | 1 | 21.5 | 16 | 13.2 | 36 | −8.3 | −39 % |

| Total | A, minutes | B, minutes | B − A, minutes | B against A |
|---|---|---|---|---|
| tier0 | 0.65 | 0.44 | −0.22 | −33 % |
| tier1 | 2.52 | 1.40 | −1.13 | −45 % |
| both | 3.18 | 1.84 | −1.34 | −42 % |

## Test time

The package's time on go's `ok` line, which leaves out the build and vet before the tests.

| Package | tier0 A | tier0 B | tier1 A | tier1 B | tier1 B against A |
|---|---|---|---|---|---|
| `internal/gatecli` | 1.4 | 1.4 | 33.4 | 24.4 | −27 % |
| `internal/battencli` | 1.0 | 0.8 | 36.1 | 14.8 | −59 % |
| `internal/webstercli` | 0.7 | 0.8 | 17.7 | 5.0 | −72 % |
| `internal/loomcli` | 7.0 | 2.8 | 15.0 | 6.3 | −58 % |
| `cmd/lym` | 19.2 | 10.3 | 14.1 | 9.2 | −35 % |

## Where each cut shows

A parent test with parallel subtests reports only its own body's time,
so for a test made parallel the B figure below is its longest row, not the whole test.

| Cut | Shows | Evidence, tier1 |
|---|---|---|
| Parallel rows in `internal/gatecli` | yes | `TestGateTest_Scopes` took 20.5 s on A and `TestGateTest_Scenario` 19.4 s; their longest rows on B took 3.5 s and 2.3 s. |
| Parallel .NET red-first rows | yes | `TestGateRedFirst_Dotnet` took 32.9 s on A, nearly the whole package's 33.4 s; its two rows on B took 8.2 s and 6.7 s. |
| Parallel `internal/battencli` scenario | yes | `TestBattenIntegration_Rows` took 24.3 s on A; its longest group on B took 5.5 s. |
| Parallel `internal/webstercli` tests | yes | `TestResetCmd` took 6.8 s on A; its longest case on B took 1.3 s. The package's test time fell 72 %. |
| Parallel `internal/loomcli` tests | probably | The package's test time fell 58 % while B ran more tests, but B's call was issued at a load of 24 against A's 38. |
| Prebuilt `lym` only when a test may use it | not measured | The cut is in `lym gate test`, and one installed `lym` ran both sides. |
| PR-Review tests over real reed | not measured | The commit's session tests are in `internal/landingshed`, which is not one of the five packages. |

`internal/gatecli` gained the least of the packages the parallel rows cut.
Its longest single test on B was `TestGateTest_DotnetFixture` at 13.1 s, against a package time of 24.4 s.

No cut targets `cmd/lym`'s tests, yet its wall time fell about 40 % at both tiers.
At tier0, `TestGofmt_ModuleFilesAreFormatted` (7.4 s on A) moved to `internal/testkit` between the two trees.
The rest of its difference is unexplained, and it shows how far a package can move without a cut aimed at it.

## How load limits the numbers

- The machine has 12 threads, and the 1-minute load at the calls' issue ran from 16 to 38.
  Every time here is inflated.
- The load was sampled when a call was issued, not when its tests started.
  Calls waited up to 746 s for a gate slot in between, so the load during a run may differ from the table's.
- A and B of a pair started between 16 seconds and 36 minutes apart.
  The widest gap is `cmd/lym` tier0, where the measuring was paused between the two sides.
- The hub has two gate slots, and what held the other one during each run was not recorded.
- One run per side gives no spread within a package.
  The agreement across packages at tier1, all five faster on B, is the support for the result;
  the size of any one package's gain is uncertain.

## Comparability of the trees

B holds every commit between the two trees, not only the cuts, and its test counts differ from A's (see `calls.tsv`).
The difference between the sides is therefore the effect of all those commits on these packages.

The five packages are those the cuts changed.
The [package report at `ea00087ec`](../test-time-measure-2026-11-27/report.md) ran under other load,
so none of its times is compared with these.

## Method

- One call per package, tier and side, in the order A then B: `lym gate test --tier <n> --no-carry ./<pkg> -- -count=1`.
  `--no-carry` and `-count=1` keep an earlier pass or the test cache from answering a call.
- Side A ran in a separate worktree with `ea00087ec`'s tree committed on top of the branch.
  Both sides used the hub's gate configuration.
- Wall time at tier1 includes `go build ./...`, `go vet` of the package and the test binary's link, as well as the tests.
- Neither side was run once beforehand to warm the Go build cache.
- All 20 calls passed.
- Go 1.26.8 on an AMD Ryzen AI 7 445.
