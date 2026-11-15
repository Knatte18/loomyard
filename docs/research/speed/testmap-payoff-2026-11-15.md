# Do the coverage maps pay for themselves? Upkeep time against gate time saved, 2026-11-15

Board task `testmap-payoff-measure` asked, before `testmap-maps-in-weft` builds the maps' check-in,
whether the upkeep takes too long, goes stale fast, and loads the machine.
This report names the facts; the decision on `testmap-maps-in-weft` is the operator's.
One machine, one run per measurement unless a row says otherwise.

## Answer in brief

- A pass after a landing re-measured 55, 10 and 119 packages for the small, medium and large commit,
  and took 23 to 25 minutes, 7.7 minutes and 31 minutes of wall time.
  The tests the commit really affects cost 1.5 s and 21 s of that;
  the rest is whole-package rebuilds that a selector miss queues.
- The gate time the maps saved on the packages each diff touches was 0 s, 4.2 s and 2.8 s.
  An upkeep pass costs a hundred to several hundred times the gate time it saves on those packages.
- After one landing 54 of 211 maps need a whole rebuild; after ten, 77 do, and 2 more hold affected tests.
  The maps are about a third stale after the first few landings and stay there until a pass runs.
- A gate run beside a pass took 3.6 s against 3.9 s, 27.3 s against 26.0 s and 10.2 s against 10.3 s.
  No slowdown is visible in the gate, but the machine's other work was about as large as the pass's.

## Inputs

The three commits are landed commits that change Go code.
Size is the added plus deleted lines of their `.go` files, test files included.

| Class | Commit | Landed | Go lines changed | Title |
|---|---|---|---|---|
| small | `4df9f81c` | 2026-11-11 | 7 | orch: default the session model to opus and effort to medium |
| medium | `79f0eb4a` | 2026-11-12 | 348 | Add `lym gate redundant` and the adoptcli coverage-map pilot report |
| large | `36621f0e` | 2026-11-12 | 1278 | Prime dobby starts coverage-map upkeep on remaining work |

The maps are a copy of `<bolt>/.lym/testcover/`, taken today, which holds 211 package maps.
Nothing under Bolt was written, rebuilt or removed.

The entry says the maps were built on 2026-11-12.
The hub's upkeep passes have kept them current in place since:
most maps already stand at `343405cf`, the head of `main` today, a few at older commits.
The copy is therefore the 2026-11-12 build as the passes have moved it, not a frozen build.

## Method

`lym gate testmap` runs only in the hub's prime worktree and writes to Bolt, so it cannot run on a copy.
A scratch program, not committed, replayed a pass on a copy of the maps:

1. It rewound each copied map to the commit's parent: the map's commit, file hashes, linked-package hashes and
   environment stamp are recomputed from the parent's tree.
   The entries keep the coverage measured at `343405cf`, the same stand-in the
   [2026-11-12 pilot](coverage-map-pilot-2026-11-12/report.md) used.
2. It planned the upkeep at the commit with the real `testcover.Selector` and `testcover.PlanUpkeep`.
3. It measured the queue the way `lym gate testmap` measures a package:
   `testcover.Toolchain` builds the coverage binary, each test runs alone in rounds of four parallel tests,
   the self-check runs on the first profile, and the map is written after each round.
   A package the selector missed for is rebuilt whole, as the pass does.
   It leaves out what needs a hub: the slot, the dirty-file and HEAD reads between rounds, and the closing audit.
4. It ran the tier 0 and tier 1 gate of one touched package through `lym gate test --wait -C <tree>`,
   once alone and once starting 2 s into step 3, with `-- -count=1` and a no-op flag that tells the two runs apart.
   The gate's green record answered some runs in 0.1 s; those are marked below and replaced by a run it did not answer.

The pass holds one of the hub's three gate slots when it runs for real; the replay holds none and never yields,
so its load on the machine is an upper bound.

## Measurements

### 1. Incremental upkeep

| Commit | Maps | Plan | Affected tests | Packages missed → rebuilt whole | Packages re-measured | Tests re-measured | Wall |
|---|---|---|---|---|---|---|---|
| small `4df9f81c` | 211 | 3.1 s | 1 in 1 map | 54 | 55 | 1959 | 1499 s (25.0 min); first run 1368 s |
| medium `79f0eb4a` | 211 | 4.7 s | 4 in 3 maps | 7 | 10 | 523 | 459 s (7.7 min) |
| large `36621f0e` | 211 | 1.8 s | 0 | 119 | 119 | 3975 | 1877 s (31.3 min) |

Wall is the measure phase, after the 5 s rewind and the plan.
The small commit ran twice; both runs planned the same and re-measured the same, and the table shows the second.

Why the queue is mostly whole rebuilds:

| Commit | Cause of the misses |
|---|---|
| small | a non-Go file changed (`internal/orchengine/template.yaml`) in a package 54 maps link |
| medium | `internal/gatecli/redundant.go` adds an import of `github.com/spf13/cobra` that 7 maps' linked sets lack |
| large | `internal/configengine/layers.go` changes the type `Spec` that 118 maps link; one more map misses on a changed `//go:` line |

The affected entries alone:

| Commit | Jobs | Tests | Wall |
|---|---|---|---|
| small | 1 | 3 | 1.5 s |
| medium | 3 | 15 | 20.9 s |
| large | 0 | 0 | 0 s |

The tests counted are those run, which includes the self-check's re-queued entries and the probe a job runs to get a profile.
Whole rebuilds cost the rest: 1497 s, 438 s and 1877 s.

### 2. Load on a concurrent gate

The gate is tier 0 and tier 1 of one package the diff touches.

| Commit | Package | Gate alone | Gate beside the pass | Pass wall in the window |
|---|---|---|---|---|
| small | `internal/orchengine` | 3.9 s | 3.6 s | gate ran +2.0 s to +5.6 s of 1499 s |
| medium | `internal/gatecli` | 26.0 s | 27.3 s | gate ran +90.0 s to +117.4 s of 459 s |
| large | `internal/beamcli` | 10.3 s | 10.2 s | gate ran +2.0 s to +12.3 s of 1877 s |

The medium gate started at +90 s, while the pass was in its larger rebuilds;
the small and large gates started 2 s in, while the pass was building its first packages.
The pass used 1.2 to 1.4 CPUs of the machine's 12 on average in all three replays.
Other work on the machine used 0.5 CPUs (medium), 1.4 to 3.2 CPUs (small) and 2.9 CPUs (large) over the same windows,
so the gate's difference from its solo time is within what the machine's other load moves.
The load average read 0.4 to 2.4 before each run, and 8 around the large run's end.

### 3. Gate saving

Tests the maps select, from the plan, for the packages the diff touches, against the whole packages.
A package whose map missed runs whole, so it saves nothing.

| Commit | Touched packages | Selected by the maps | Whole |
|---|---|---|---|
| small | `orchengine` | whole (miss) | whole |
| medium | `gatecli`; `testcover` | `gatecli` whole (miss); `testcover` 1 of 21 mapped tests | both whole |
| large | `beamcli`, `configengine`, `configreg`, `configsync`, `dobbyengine`, `fabriccli`, `orchcli`; `runproc` | seven whole (miss); `runproc` none (current) | all whole |

Gate wall of the packages whose selection differs from the whole package; the others run the same either way.
Tier 0 and tier 1, `-count=1`, one run each; the second run of each command was answered by the green record and is left out.

| Commit | Package | Selected | Whole | Saved |
|---|---|---|---|---|
| small | none | 0 s | 0 s | 0 s |
| medium | `internal/testcover` | 6.1 s (1 test) | 10.3 s | 4.2 s |
| large | `internal/runproc` | 0 s (no test) | 2.8 s | 2.8 s |

The tier 1 step builds and vets the module before any test runs, which is why one selected test still costs 6.1 s
of the whole package's 10.3 s.
The machine's load average was 8 when these ran.

## Verdict

Does an upkeep pass after a typical landing cost less than the gate time the maps save before the next landing? No.

- A pass cost 7.7 to 31 minutes of one gate slot's worth of wall time and about 1.3 CPUs.
  The gate time saved on the packages the diff touches was 0 to 4.2 s.
  To break even the maps would need to save that much on a hundred or more gate runs between two landings.
  The repository lands about 30 commits on a day with landings (333 from 2026-11-01 to 2026-11-15), one every 48 minutes,
  so a 25 to 31 minute pass fills half or more of the gap to the next landing.
- The saving is small because the selector misses far more than it narrows.
  In all three commits the touched packages were missed or had nothing to select, bar one test in `testcover`.
  A miss runs the whole package and queues a whole rebuild, which is where the upkeep time goes.
  This is a measurement of three landings and the packages each touches;
  gates of other runs, whose diffs differ from the landing's, were not measured.
- The pass's load on a gate was not visible: within 5 percent of the solo time on all three,
  inside the noise of the machine's other work.
  The replay never yielded its slot, so a real pass takes a smaller share.

How stale the maps grow when upkeep runs only by hand:
the maps rewound to `f98e284c`, the last landing of 2026-11-12, planned at each of the ten landings on `main` since.
`current` is a map the plan keeps; `miss` is a map whose package is rebuilt whole.

| Landings since the maps | Current | Affected tests | Miss | Tests a pass would rebuild whole |
|---|---|---|---|---|
| 1 (`588ae6c5`) | 157 | 0 (1 test) | 54 | 2385 |
| 2 (`5b612ee5`) | 137 | 0 (1 test) | 74 | 3204 |
| 3 (`45d8b10e`) | 134 | 0 (1 test) | 77 | 3371 |
| 5 (`57f125eb`) | 134 | 0 (1 test) | 77 | 3373 |
| 8 (`dce431f4`) | 132 | 2 maps (6 tests) | 77 | 3377 |
| 10 (`343405cf`) | 132 | 2 maps (6 tests) | 77 | 3377 |

The share of maps a pass would rebuild whole rises from 26 percent after one landing to 36 percent after three,
then holds there: the same changed non-Go and linked files keep the same maps missed.
So the maps do not decay with every landing; a third of them are out of date within a few landings,
and a pass over them costs the 20 to 30 minutes above whether it runs after one landing or ten.
Plan time, the part that decides what is stale, takes 3 s.

## Limits

- One run per cell; the small commit's two upkeep runs differ by 9 percent (1368 s and 1499 s) under different machine load.
- The replay is a re-implementation of the measure loop, minus the slot, the status reads and the audit.
- Entries were measured at `343405cf` and rewound to the parent; the plan's selection and the rebuild costs come from
  that stand-in, so a commit far from `343405cf` would plan differently.
- The first small run's gate and the large run's second gate-alone were answered by the green record in 0.1 s;
  the table uses runs it did not answer.
- The system journal holds one sleep event today, a hibernate that returned at 12:20:38,
  before the first measurement started at 14:44.
  No sleep window crosses any measurement, so none was repeated for that cause.
