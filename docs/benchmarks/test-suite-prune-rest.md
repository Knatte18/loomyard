# Test suite prune (rest): measurement report

This report records the per-package test count, wall time, serial time and non-test statement coverage of every in-scope package before and after the `test-suite-prune-rest` task, measured on one machine.
The before figures come from `go run ./cmd/testtiming -tags integration,tmux` and from `go test -count=1 -tags integration,tmux -covermode=set -coverpkg=./... -coverprofile=<file> ./...`, both run from the repository root.

## Method

- Commit measured: `9cd12fd198c1a295fa70b4d12dd455a249d6f8ef`, this branch's base commit, on an untouched tree.
- In scope: every package with a `## ` section in [test-redundancy.md](test-redundancy.md) except `internal/fabricengine`, `internal/websterengine`, `internal/reedengine`, `internal/shuttleengine`, `internal/loomcli`, `internal/shedadapters` and everything under `internal/testkit/`.
- Tests: the top-level test count `testtiming` reports for the package.
  The count was identical in all three runs.
- Wall: `testtiming`'s package elapsed time, the median of three runs.
- Serial: the sum of the package's top-level test times, the median of three runs.
- Coverage: covered statements over total statements, summed from each block's statement field of the profile, over the non-test files directly in the package's own directory.
  A block counts as covered when any test binary of the run executed it.
- Before coverage profile and per-package figures: `.scratch/before-cover.out` and `.scratch/before-cov-by-pkg.tsv` of the measuring worktree; both are reproducible from the measured commit with the coverage command above.
- Machine: Linux, 12 logical CPUs.

## Load average at the start of each run

| Run | `/proc/loadavg` (1, 5, 15 minutes) |
|---|---|
| Timing run 1 | 4.59 4.61 3.31 |
| Timing run 2 | 3.67 5.49 4.12 |
| Timing run 3 | 4.08 8.18 5.95 |
| Coverage run | 2.57 8.34 6.90 |

## Before

Wall and serial are in seconds.

| Package | Tests | Wall | Serial | Coverage |
|---|---|---|---|---|
| `cmd/lyx` | 87 | 20.03 | 19.93 | 74.7% (56/75) |
| `cmd/testtiming` | 7 | 0.01 | 0.00 | 47.6% (273/574) |
| `contracts/specs` | 4 | 0.01 | 0.00 | 100.0% (9/9) |
| `contracts/stencils` | 22 | 0.06 | 0.05 | 88.9% (8/9) |
| `internal/agentname` | 7 | 0.01 | 0.00 | 98.5% (67/68) |
| `internal/batcher` | 13 | 0.01 | 0.00 | 95.7% (22/23) |
| `internal/battencli` | 104 | 20.71 | 20.67 | 81.6% (412/505) |
| `internal/battenrecipe` | 8 | 0.02 | 0.00 | 80.0% (16/20) |
| `internal/battenshed` | 97 | 0.06 | 0.02 | 86.9% (373/429) |
| `internal/boardcli` | 43 | 0.99 | 0.94 | 82.2% (364/443) |
| `internal/boardengine` | 111 | 0.67 | 0.60 | 92.0% (1072/1165) |
| `internal/boardengine/boardtest` | 9 | 2.00 | 10.37 | no statements |
| `internal/buildinfo` | 2 | 0.00 | 0.00 | 100.0% (1/1) |
| `internal/burlercli` | 25 | 0.18 | 0.13 | 78.8% (104/132) |
| `internal/burlerengine` | 50 | 0.21 | 0.18 | 92.6% (326/352) |
| `internal/clihelp` | 20 | 0.01 | 0.00 | 92.1% (70/76) |
| `internal/cliwire` | 20 | 0.30 | 0.28 | 95.0% (95/100) |
| `internal/configcli` | 36 | 1.12 | 1.09 | 84.2% (230/273) |
| `internal/configengine` | 51 | 0.03 | 0.00 | 78.3% (108/138) |
| `internal/configreg` | 11 | 0.19 | 0.18 | 100.0% (14/14) |
| `internal/configsync` | 10 | 0.05 | 0.01 | 85.5% (130/152) |
| `internal/discussionparser` | 23 | 0.01 | 0.00 | 94.8% (73/77) |
| `internal/envsource` | 6 | 0.01 | 0.00 | 88.6% (31/35) |
| `internal/fabriccli` | 88 | 23.99 | 23.95 | 77.6% (529/682) |
| `internal/friction` | 13 | 0.02 | 0.00 | 96.1% (49/51) |
| `internal/frictionengine` | 24 | 0.21 | 0.18 | 84.2% (149/177) |
| `internal/fslink` | 5 | 0.01 | 0.00 | 80.4% (45/56) |
| `internal/fsx` | 3 | 0.01 | 0.00 | 81.2% (26/32) |
| `internal/gitexec` | 12 | 0.15 | 0.14 | 97.9% (46/47) |
| `internal/githubclient` | 14 | 0.03 | 0.01 | 71.8% (127/177) |
| `internal/gitkit` | 15 | 0.76 | 1.18 | 81.1% (180/222) |
| `internal/gitrepo` | 113 | 4.12 | 4.08 | 81.9% (422/515) |
| `internal/gitrepo/internal/gitoracle` | 1 | 0.01 | 0.00 | 78.4% (29/37) |
| `internal/hubforge` | 13 | 1.11 | 4.61 | 77.1% (135/175) |
| `internal/hubgeom` | 14 | 1.09 | 1.08 | 89.7% (52/58) |
| `internal/idecli` | 3 | 0.43 | 0.42 | 73.8% (31/42) |
| `internal/ideengine` | 17 | 4.43 | 4.42 | 84.5% (98/116) |
| `internal/landingshed` | 147 | 2.33 | 2.22 | 90.0% (422/469) |
| `internal/lock` | 3 | 0.01 | 0.00 | 88.9% (16/18) |
| `internal/logger` | 68 | 0.16 | 0.13 | 93.8% (240/256) |
| `internal/loggerconfig` | 8 | 0.01 | 0.00 | 88.0% (22/25) |
| `internal/loomengine` | 95 | 0.18 | 0.06 | 88.1% (258/293) |
| `internal/loomrecipe` | 48 | 0.34 | 0.31 | 88.9% (56/63) |
| `internal/loomshed` | 72 | 0.18 | 0.14 | 89.6% (450/502) |
| `internal/lyxcwd` | 34 | 1.93 | 2.28 | 88.8% (71/80) |
| `internal/mergeresolve` | 22 | 0.07 | 0.02 | 78.4% (109/139) |
| `internal/modelspec` | 15 | 0.01 | 0.00 | 97.1% (135/139) |
| `internal/orchcli` | 31 | 1.29 | 1.24 | 57.4% (217/378) |
| `internal/orchengine` | 128 | 0.46 | 0.20 | 86.9% (597/687) |
| `internal/output` | 10 | 0.00 | 0.00 | 100.0% (12/12) |
| `internal/pairteardown` | 14 | 5.27 | 5.27 | 83.8% (109/130) |
| `internal/parentreview` | 58 | 0.05 | 0.00 | 84.3% (323/383) |
| `internal/pattern` | 21 | 0.04 | 0.00 | 96.8% (92/95) |
| `internal/planglyph` | 109 | 0.23 | 0.17 | 94.6% (594/628) |
| `internal/planparser` | 118 | 0.06 | 0.19 | 95.7% (1023/1069) |
| `internal/preflight` | 18 | 1.05 | 5.10 | 90.6% (58/64) |
| `internal/preflightshed` | 8 | 0.43 | 1.09 | 87.8% (36/41) |
| `internal/proc` | 8 | 0.01 | 0.00 | 76.2% (16/21) |
| `internal/quarrycli` | 10 | 0.05 | 0.03 | 69.2% (74/107) |
| `internal/reedcli` | 101 | 215.92 | 216.24 | 79.4% (363/457) |
| `internal/reedengine/render` | 34 | 0.01 | 0.00 | 97.9% (191/195) |
| `internal/selfreportcli` | 13 | 0.01 | 0.00 | 96.7% (29/30) |
| `internal/selfreportengine` | 13 | 0.01 | 0.00 | 94.7% (72/76) |
| `internal/shedbuild` | 23 | 0.02 | 0.00 | 98.6% (70/71) |
| `internal/shedcheck` | 3 | 0.01 | 0.00 | 98.5% (128/130) |
| `internal/shedcli` | 35 | 3.38 | 3.36 | 73.7% (101/137) |
| `internal/shedengine` | 152 | 0.11 | 0.03 | 94.3% (378/401) |
| `internal/shedrecipe` | 98 | 0.20 | 0.10 | 93.5% (673/720) |
| `internal/shedrun` | 44 | 0.01 | 0.00 | 87.8% (108/123) |
| `internal/shedtransient` | 3 | 0.01 | 0.00 | 100.0% (12/12) |
| `internal/shedverbs` | 71 | 0.07 | 0.02 | 91.7% (321/350) |
| `internal/shell` | 20 | 0.01 | 0.00 | 96.8% (30/31) |
| `internal/shuttlecli` | 9 | 0.03 | 0.01 | 56.1% (60/107) |
| `internal/shuttleengine/claudeengine` | 101 | 0.32 | 0.26 | 91.4% (655/717) |
| `internal/standalonegeom` | 11 | 0.71 | 0.70 | 100.0% (7/7) |
| `internal/standalonestate` | 17 | 0.01 | 0.00 | 97.5% (39/40) |
| `internal/state` | 17 | 0.03 | 0.01 | 90.2% (55/61) |
| `internal/statuscommit` | 8 | 0.01 | 0.00 | 100.0% (25/25) |
| `internal/stencil` | 33 | 0.01 | 0.00 | 91.1% (112/123) |
| `internal/stencilcli` | 7 | 2.61 | 2.59 | 80.8% (249/308) |
| `internal/stencilstore` | 25 | 0.01 | 0.00 | 86.4% (171/198) |
| `internal/summaryparser` | 14 | 0.01 | 0.00 | 95.1% (39/41) |
| `internal/tokenvocab` | 7 | 0.01 | 0.00 | 100.0% (8/8) |
| `internal/treadleengine` | 45 | 0.39 | 0.36 | 83.0% (421/507) |
| `internal/verifyrun` | 5 | 0.82 | 0.80 | 85.7% (24/28) |
| `internal/verifytree` | 13 | 0.57 | 0.55 | 81.8% (72/88) |
| `internal/vscode` | 14 | 0.03 | 0.00 | 82.1% (124/151) |
| `internal/webstercli` | 104 | 12.64 | 12.55 | 77.9% (686/881) |
| `internal/weftname` | 3 | 0.00 | 0.00 | 100.0% (2/2) |
| `internal/yamlengine` | 61 | 0.01 | 0.00 | 91.8% (394/429) |
| `tools/codestats` | 10 | 0.01 | 0.00 | 75.3% (174/231) |
| `tools/deploy` | 7 | 0.01 | 0.00 | 25.6% (46/180) |
| `tools/godocreflow` | 21 | 0.03 | 0.00 | 70.5% (232/329) |
| `tools/internal/devbin` | 3 | 0.00 | 0.00 | 73.3% (11/15) |
| `tools/mdreflow` | 4 | 0.01 | 0.00 | 78.2% (258/330) |
| `tools/sandbox` | 71 | 0.04 | 0.01 | 70.1% (211/301) |
| `tools/wordswap` | 11 | 0.01 | 0.00 | 57.5% (96/167) |

## After

The `rerun-measure-after` card fills this section with the same table, measured on the pruned tree with the same commands, and names every package whose coverage lost a block.
