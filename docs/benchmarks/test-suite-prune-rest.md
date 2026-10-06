# Test suite prune (rest): measurement report

This report records the per-package test count, wall time, serial time and non-test statement coverage of every in-scope package before and after the `test-suite-prune-rest` task, measured on one machine.
The figures come from `go run ./cmd/testtiming -tags integration,tmux` and from `go test -count=1 -tags integration,tmux -covermode=set -coverpkg=./... -coverprofile=<file> ./...`, both run from the repository root.

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
| After timing run 1 | 8.26 3.30 1.29 |
| After timing run 2 | 8.41 5.44 2.29 |
| After timing run 3 | 5.91 5.89 2.77 |
| After coverage run (final) | 1.71 4.93 3.58 |

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

Measured with the commands above on the same machine, on the tree of the final `test-suite-prune-rest` commit: the tree of this branch's last prune commit plus that commit's own test edits.
The after load averages are in the load table above.
Wall, serial and their deltas are in seconds, and a delta is the after figure minus the before figure.
The coverage delta is in percentage points of the package's own statements.

| Package | Tests | Δ | Wall | Δ | Serial | Δ | Coverage | Δ |
|---|---|---|---|---|---|---|---|---|
| `cmd/lyx` | 64 | -23 | 14.58 | -5.45 | 14.53 | -5.40 | 74.7% (56/75) | +0.0 pp |
| `cmd/testtiming` | 7 | +0 | 0.01 | +0.00 | 0.00 | +0.00 | 47.6% (273/574) | +0.0 pp |
| `contracts/specs` | 3 | -1 | 0.01 | +0.00 | 0.00 | +0.00 | 100.0% (9/9) | +0.0 pp |
| `contracts/stencils` | 14 | -8 | 0.04 | -0.02 | 0.09 | +0.04 | 88.9% (8/9) | +0.0 pp |
| `internal/agentname` | 7 | +0 | 0.01 | +0.00 | 0.00 | +0.00 | 98.5% (67/68) | +0.0 pp |
| `internal/batcher` | 5 | -8 | 0.01 | +0.00 | 0.00 | +0.00 | 95.7% (22/23) | +0.0 pp |
| `internal/battencli` | 48 | -56 | 10.63 | -10.08 | 11.28 | -9.39 | 81.6% (412/505) | +0.0 pp |
| `internal/battenrecipe` | 8 | +0 | 0.01 | -0.01 | 0.00 | +0.00 | 80.0% (16/20) | +0.0 pp |
| `internal/battenshed` | 76 | -21 | 0.05 | -0.01 | 0.02 | +0.00 | 86.9% (373/429) | +0.0 pp |
| `internal/boardcli` | 12 | -31 | 0.64 | -0.35 | 0.62 | -0.32 | 82.2% (364/443) | +0.0 pp |
| `internal/boardengine` | 73 | -38 | 0.51 | -0.16 | 0.48 | -0.12 | 92.0% (1072/1165) | +0.0 pp |
| `internal/boardengine/boardtest` | 3 | -6 | 1.91 | -0.09 | 3.55 | -6.82 | no statements | n/a |
| `internal/buildinfo` | 2 | +0 | 0.01 | +0.01 | 0.00 | +0.00 | 100.0% (1/1) | +0.0 pp |
| `internal/burlercli` | 17 | -8 | 0.13 | -0.05 | 0.05 | -0.08 | 78.8% (104/132) | +0.0 pp |
| `internal/burlerengine` | 28 | -22 | 0.17 | -0.04 | 0.16 | -0.02 | 92.6% (326/352) | +0.0 pp |
| `internal/clihelp` | 11 | -9 | 0.01 | +0.00 | 0.00 | +0.00 | 92.1% (70/76) | +0.0 pp |
| `internal/cliwire` | 12 | -8 | 0.27 | -0.03 | 0.24 | -0.04 | 95.0% (95/100) | +0.0 pp |
| `internal/configcli` | 10 | -26 | 0.70 | -0.42 | 0.68 | -0.41 | 84.2% (230/273) | +0.0 pp |
| `internal/configengine` | 12 | -39 | 0.02 | -0.01 | 0.00 | +0.00 | 78.3% (108/138) | +0.0 pp |
| `internal/configreg` | 6 | -5 | 0.19 | +0.00 | 0.18 | +0.00 | 100.0% (14/14) | +0.0 pp |
| `internal/configsync` | 5 | -5 | 0.06 | +0.01 | 0.03 | +0.02 | 85.5% (130/152) | +0.0 pp |
| `internal/discussionparser` | 12 | -11 | 0.01 | +0.00 | 0.00 | +0.00 | 94.8% (73/77) | +0.0 pp |
| `internal/envsource` | 3 | -3 | 0.00 | -0.01 | 0.00 | +0.00 | 88.6% (31/35) | +0.0 pp |
| `internal/fabriccli` | 19 | -69 | 6.94 | -17.05 | 9.29 | -14.66 | 77.6% (529/682) | +0.0 pp |
| `internal/friction` | 6 | -7 | 0.01 | -0.01 | 0.00 | +0.00 | 96.1% (49/51) | +0.0 pp |
| `internal/frictionengine` | 15 | -9 | 0.16 | -0.05 | 0.52 | +0.34 | 84.2% (149/177) | +0.0 pp |
| `internal/fslink` | 5 | +0 | 0.01 | +0.00 | 0.00 | +0.00 | 80.4% (45/56) | +0.0 pp |
| `internal/fsx` | 3 | +0 | 0.01 | +0.00 | 0.00 | +0.00 | 81.2% (26/32) | +0.0 pp |
| `internal/gitexec` | 6 | -6 | 0.06 | -0.09 | 0.07 | -0.07 | 97.9% (46/47) | +0.0 pp |
| `internal/githubclient` | 10 | -4 | 0.03 | +0.00 | 0.01 | +0.00 | 71.8% (127/177) | +0.0 pp |
| `internal/gitkit` | 8 | -7 | 0.83 | +0.07 | 0.90 | -0.28 | 81.1% (180/222) | +0.0 pp |
| `internal/gitrepo` | 32 | -81 | 1.13 | -2.99 | 5.29 | +1.21 | 81.9% (422/515) | +0.0 pp |
| `internal/gitrepo/internal/gitoracle` | 1 | +0 | 0.01 | +0.00 | 0.00 | +0.00 | 78.4% (29/37) | +0.0 pp |
| `internal/hubforge` | 5 | -8 | 0.89 | -0.22 | 1.22 | -3.39 | 77.1% (135/175) | +0.0 pp |
| `internal/hubgeom` | 8 | -6 | 0.55 | -0.54 | 0.53 | -0.55 | 89.7% (52/58) | +0.0 pp |
| `internal/idecli` | 2 | -1 | 0.24 | -0.19 | 0.23 | -0.19 | 73.8% (31/42) | +0.0 pp |
| `internal/ideengine` | 4 | -13 | 2.42 | -2.01 | 2.41 | -2.01 | 84.5% (98/116) | +0.0 pp |
| `internal/landingshed` | 60 | -87 | 1.80 | -0.53 | 1.84 | -0.38 | 90.0% (422/469) | +0.0 pp |
| `internal/lock` | 3 | +0 | 0.01 | +0.00 | 0.00 | +0.00 | 88.9% (16/18) | +0.0 pp |
| `internal/logger` | 34 | -34 | 0.11 | -0.05 | 0.09 | -0.04 | 93.8% (240/256) | +0.0 pp |
| `internal/loggerconfig` | 3 | -5 | 0.01 | +0.00 | 0.00 | +0.00 | 88.0% (22/25) | +0.0 pp |
| `internal/loomengine` | 39 | -56 | 0.16 | -0.02 | 0.14 | +0.08 | 88.1% (258/293) | +0.0 pp |
| `internal/loomrecipe` | 44 | -4 | 0.41 | +0.07 | 0.40 | +0.09 | 88.9% (56/63) | +0.0 pp |
| `internal/loomshed` | 66 | -6 | 0.18 | +0.00 | 0.13 | -0.01 | 89.6% (450/502) | +0.0 pp |
| `internal/lyxcwd` | 23 | -11 | 2.03 | +0.10 | 2.01 | -0.27 | 88.8% (71/80) | +0.0 pp |
| `internal/mergeresolve` | 17 | -5 | 0.07 | +0.00 | 0.36 | +0.34 | 78.4% (109/139) | +0.0 pp |
| `internal/modelspec` | 11 | -4 | 0.01 | +0.00 | 0.00 | +0.00 | 97.1% (135/139) | +0.0 pp |
| `internal/orchcli` | 17 | -14 | 0.82 | -0.47 | 0.58 | -0.66 | 57.4% (217/378) | +0.0 pp |
| `internal/orchengine` | 118 | -10 | 0.50 | +0.04 | 0.27 | +0.07 | 86.9% (597/687) | +0.0 pp |
| `internal/output` | 4 | -6 | 0.00 | +0.00 | 0.00 | +0.00 | 100.0% (12/12) | +0.0 pp |
| `internal/pairteardown` | 4 | -10 | 3.97 | -1.30 | 3.95 | -1.32 | 83.8% (109/130) | +0.0 pp |
| `internal/parentreview` | 53 | -5 | 0.07 | +0.02 | 0.00 | +0.00 | 84.3% (323/383) | +0.0 pp |
| `internal/pattern` | 7 | -14 | 0.02 | -0.02 | 0.01 | +0.01 | 96.8% (92/95) | +0.0 pp |
| `internal/planglyph` | 74 | -35 | 0.13 | -0.10 | 0.12 | -0.05 | 94.6% (594/628) | +0.0 pp |
| `internal/planparser` | 78 | -40 | 0.04 | -0.02 | 0.17 | -0.02 | 95.7% (1023/1069) | +0.0 pp |
| `internal/preflight` | 7 | -11 | 0.77 | -0.28 | 0.98 | -4.12 | 90.6% (58/64) | +0.0 pp |
| `internal/preflightshed` | 5 | -3 | 0.30 | -0.13 | 0.30 | -0.79 | 87.8% (36/41) | +0.0 pp |
| `internal/proc` | 6 | -2 | 0.01 | +0.00 | 0.00 | +0.00 | 76.2% (16/21) | +0.0 pp |
| `internal/quarrycli` | 7 | -3 | 0.03 | -0.02 | 0.01 | -0.02 | 69.2% (74/107) | +0.0 pp |
| `internal/reedcli` | 33 | -68 | 47.07 | -168.85 | 78.49 | -137.75 | 80.7% (369/457) | +1.3 pp |
| `internal/reedengine/render` | 12 | -22 | 0.01 | +0.00 | 0.00 | +0.00 | 97.9% (191/195) | +0.0 pp |
| `internal/selfreportcli` | 3 | -10 | 0.02 | +0.01 | 0.01 | +0.01 | 96.7% (29/30) | +0.0 pp |
| `internal/selfreportengine` | 11 | -2 | 0.02 | +0.01 | 0.00 | +0.00 | 94.7% (72/76) | +0.0 pp |
| `internal/shedbuild` | 13 | -10 | 0.01 | -0.01 | 0.00 | +0.00 | 98.6% (70/71) | +0.0 pp |
| `internal/shedcheck` | 3 | +0 | 0.01 | +0.00 | 0.00 | +0.00 | 98.5% (128/130) | +0.0 pp |
| `internal/shedcli` | 17 | -18 | 1.48 | -1.90 | 1.45 | -1.91 | 73.7% (101/137) | +0.0 pp |
| `internal/shedengine` | 130 | -22 | 0.06 | -0.05 | 0.00 | -0.03 | 94.3% (378/401) | +0.0 pp |
| `internal/shedrecipe` | 94 | -4 | 0.12 | -0.08 | 0.04 | -0.06 | 93.5% (673/720) | +0.0 pp |
| `internal/shedrun` | 22 | -22 | 0.01 | +0.00 | 0.00 | +0.00 | 88.6% (109/123) | +0.8 pp |
| `internal/shedtransient` | 2 | -1 | 0.01 | +0.00 | 0.00 | +0.00 | 100.0% (12/12) | +0.0 pp |
| `internal/shedverbs` | 65 | -6 | 0.05 | -0.02 | 0.01 | -0.01 | 91.7% (321/350) | +0.0 pp |
| `internal/shell` | 11 | -9 | 0.01 | +0.00 | 0.00 | +0.00 | 96.8% (30/31) | +0.0 pp |
| `internal/shuttlecli` | 7 | -2 | 0.03 | +0.00 | 0.02 | +0.01 | 56.1% (60/107) | +0.0 pp |
| `internal/shuttleengine/claudeengine` | 85 | -16 | 0.28 | -0.04 | 0.26 | +0.00 | 91.4% (655/717) | +0.0 pp |
| `internal/standalonegeom` | 7 | -4 | 0.73 | +0.02 | 0.71 | +0.01 | 100.0% (7/7) | +0.0 pp |
| `internal/standalonestate` | 6 | -11 | 0.01 | +0.00 | 0.00 | +0.00 | 97.5% (39/40) | +0.0 pp |
| `internal/state` | 8 | -9 | 0.01 | -0.02 | 0.00 | -0.01 | 90.2% (55/61) | +0.0 pp |
| `internal/statuscommit` | 6 | -2 | 0.01 | +0.00 | 0.00 | +0.00 | 100.0% (25/25) | +0.0 pp |
| `internal/stencil` | 3 | -30 | 0.01 | +0.00 | 0.00 | +0.00 | 91.1% (112/123) | +0.0 pp |
| `internal/stencilcli` | 1 | -6 | 0.53 | -2.08 | 0.52 | -2.07 | 80.8% (249/308) | +0.0 pp |
| `internal/stencilstore` | 16 | -9 | 0.01 | +0.00 | 0.00 | +0.00 | 86.4% (171/198) | +0.0 pp |
| `internal/summaryparser` | 8 | -6 | 0.01 | +0.00 | 0.00 | +0.00 | 95.1% (39/41) | +0.0 pp |
| `internal/tokenvocab` | 5 | -2 | 0.01 | +0.00 | 0.00 | +0.00 | 100.0% (8/8) | +0.0 pp |
| `internal/treadleengine` | 33 | -12 | 0.42 | +0.03 | 0.38 | +0.02 | 83.0% (421/507) | +0.0 pp |
| `internal/verifyrun` | 3 | -2 | 0.82 | +0.00 | 0.80 | +0.00 | 85.7% (24/28) | +0.0 pp |
| `internal/verifytree` | 4 | -9 | 0.19 | -0.38 | 0.18 | -0.37 | 81.8% (72/88) | +0.0 pp |
| `internal/vscode` | 9 | -5 | 0.01 | -0.02 | 0.00 | +0.00 | 82.1% (124/151) | +0.0 pp |
| `internal/webstercli` | 68 | -36 | 3.54 | -9.10 | 3.49 | -9.06 | 77.9% (686/881) | +0.0 pp |
| `internal/weftname` | 1 | -2 | 0.01 | +0.01 | 0.00 | +0.00 | 100.0% (2/2) | +0.0 pp |
| `internal/yamlengine` | 12 | -49 | 0.01 | +0.00 | 0.00 | +0.00 | 91.8% (394/429) | +0.0 pp |
| `tools/codestats` | 9 | -1 | 0.01 | +0.00 | 0.00 | +0.00 | 75.3% (174/231) | +0.0 pp |
| `tools/deploy` | 7 | +0 | 0.01 | +0.00 | 0.00 | +0.00 | 25.6% (46/180) | +0.0 pp |
| `tools/godocreflow` | 6 | -15 | 0.01 | -0.02 | 0.00 | +0.00 | 70.5% (232/329) | +0.0 pp |
| `tools/internal/devbin` | 1 | -2 | 0.01 | +0.01 | 0.00 | +0.00 | 73.3% (11/15) | +0.0 pp |
| `tools/mdreflow` | 3 | -1 | 0.01 | +0.00 | 0.00 | +0.00 | 78.2% (258/330) | +0.0 pp |
| `tools/sandbox` | 28 | -43 | 0.03 | -0.01 | 0.01 | +0.00 | 72.1% (217/301) | +2.0 pp |
| `tools/wordswap` | 3 | -8 | 0.01 | +0.00 | 0.00 | +0.00 | 57.5% (96/167) | +0.0 pp |

`cmd/testtiming` is measured read-only: its tests belong to the core task and were not edited.

Across the in-scope packages, the top-level test count went from 3377 to 1997, the summed median wall time from 335.32 s to 110.33 s and the summed serial time from 350.56 s to 150.11 s.

## Coverage

No in-scope package lost statement coverage, and a block-by-block comparison of the final profile with `.scratch/before-cover.out` finds no non-test block that the before run covered and the after run does not.
Two blocks were lost on the way and restored in the final commit, each by a row in an existing test of its package:

- `internal/configcli`: the process-cwd seam blocks of `RunCLI` and `RunCLIIn` (`configcli.go` lines 438–440 and 448–450), dropped with the deleted `TestReconcile_NotAGitRepo`; the first after run measured 228 of 273 statements.
- `internal/shedrun`: the length-mismatch block of `paramsEqual` (`seed.go` lines 201–203), whose coverage had flipped to the value-mismatch block alone; the coverage figure was unchanged by the swap.

No block is recorded as noise.
The coverage run was repeated after each restore, and the table is from the last of the three runs; the first two started at load averages of `5.91 5.89 2.77` and `2.44 5.65 3.48`.

## Wall-time rises

Thirteen packages show a wall delta above zero, each by 0.10 s or less.
Two have a scenario merge behind the rise, which runs steps serially that used to run in parallel:

- `internal/gitkit`: `TestCopiedRepoScenario` replaces nine `CopyRepo` builds with two.
- `internal/lyxcwd`: `TestResolve_AnchorScenario` replaces nine checkout copies with one.

The other rises have no scenario merge in their package: `internal/buildinfo`, `internal/configsync`, `internal/loomrecipe`, `internal/orchengine`, `internal/parentreview`, `internal/selfreportcli`, `internal/selfreportengine`, `internal/standalonegeom`, `internal/treadleengine`, `internal/weftname` and `tools/internal/devbin`.
Their tests were folded into tables or deleted, and the rises sit within the spread between the three timing runs, which started at load averages of 5.91 to 8.41.
