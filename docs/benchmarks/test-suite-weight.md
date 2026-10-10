# Test suite weight

This is a measurement report: it records one run on one machine and goes stale as the suite changes.
Regenerate it with the commands under [Regenerate](#regenerate) rather than editing the figures.

```yaml
commit: df36bb2a4
date: 2026-10-10
machine: AMD Ryzen AI 7 445, 12 threads, 30 GiB, Linux 7.0.0-31-generic x86_64
tiers: [untagged, integration, tmux]
not_measured: [llm]
```

The `llm` tier is not measured.
Its tests run live model sessions, so a run spends tokens, and its wall time and CPU are the model's rather than the suite's.
`go run ./cmd/testtiming -resources -tags llm` measures it for an operator who wants the figures.

Each tier ran once, serially, package by package, inside one gate slot.
The columns and their sources are documented in the package comment of `cmd/testtiming`; [running-tests.md](running-tests.md) names the regeneration commands.
`PROCS` is system-wide and `GIT_FIXTURE` against `GIT_CODE` is exact only for a serial package, so compare before and after on the totals.

## Regenerate

```
go run ./cmd/testtiming -resources
go run ./cmd/testtiming -resources -tags integration
go run ./cmd/testtiming -resources -tags tmux
```

## Before

### Tier 1 (untagged)

```
Resources  —  Tier 1 (offline)

PACKAGE                                       WALL       CPU   PEAK_MEM   PROCS  LEFTOVER  GIT_FIXTURE  GIT_CODE
----------------------------------------  --------  --------  ---------  ------  --------  -----------  --------
internal/reedengine                          2.19s     3.58s     195.3M     190         0            0         0
cmd/lyx                                      1.88s     2.80s     203.0M     601         0            0         3
internal/websterengine                       1.83s     6.38s     198.9M     340         0            0         3
internal/loomcli                             1.82s     4.24s     236.8M     249         0            0         2
internal/lyxcwd                              1.49s     3.77s     195.9M     877         0            0         0
internal/shuttleengine                       1.44s     3.97s     232.3M     222         0            0         0
internal/orchcli                             1.32s     1.14s     113.0M     178         0            0         0
internal/shedadapters                        1.14s     3.49s     192.2M     132         0            0         0
internal/fabricengine                        1.01s     3.57s     219.6M     194         0            0         3
internal/orchengine                          0.98s     2.41s     131.7M     207         0            0         0
internal/webstercli                          0.98s     2.25s     174.6M     185         0            0         0
internal/loomshed                            0.93s     2.09s     159.1M     409         0            0         0
internal/shedrecipe                          0.92s     2.58s     158.6M     265         0            0         0
internal/planglyph                           0.83s     2.11s     154.0M     272         0            0         0
internal/loomrecipe                          0.81s     1.64s     128.5M     216         0            0         0
internal/boardengine                         0.77s     2.06s     117.9M     317         0            0         0
internal/landingshed                         0.75s     1.83s     148.8M     137         0            0         0
internal/shedcli                             0.75s     1.41s     180.1M     144         0            0         0
internal/battencli                           0.73s     1.86s     133.6M     194         0            0         0
internal/burlercli                           0.73s     1.11s     104.0M     136         0            0         2
internal/shuttleengine/claudeengine          0.73s     2.07s     137.5M     147         0            0         0
internal/burlerengine                        0.70s     1.78s     121.2M     135         0            0         0
internal/reedcli                             0.68s     1.15s     104.0M     108         0            0         1
internal/treadleengine                       0.61s     1.39s     109.0M     143         0            0         0
internal/seatengine                          0.60s     1.41s     105.4M     114         0            0         0
internal/fabriccli                           0.60s     1.23s     115.3M     241         0            0         0
internal/planparser                          0.59s     1.80s     115.3M     162         0            0         0
internal/battenshed                          0.59s     1.60s     131.2M     127         0            0         0
internal/loomengine                          0.58s     1.46s     109.6M     116         0            0         0
internal/configreg                           0.55s     1.04s     113.8M     227         0            0         0
contracts/stencils                           0.55s     1.02s     114.0M     259         0            0         0
internal/shedverbs                           0.55s     1.39s     114.4M     155         0            0         0
internal/boardcli                            0.55s     1.20s     114.6M     149         0            0         0
tools/tokencount                             0.55s     1.31s     123.8M     110         0            0         0
internal/pairteardown                        0.53s     1.00s     100.5M     291         0            0         0
internal/quarrycli                           0.53s     0.99s     122.1M     152         0            0        13
internal/shedbuild                           0.52s     1.03s     117.5M     147         0            0         0
cmd/testtiming                               0.52s     1.10s     113.6M     551         0            0         0
internal/gitkit                              0.52s     0.87s     157.4M     136         0            0         0
internal/cliwire                             0.51s     1.04s      90.1M     110         0            0         0
internal/shuttlecli                          0.49s     0.97s      92.1M     250         0            0         3
internal/configsync                          0.49s     1.02s      94.5M     140         0            0         0
internal/battenrecipe                        0.48s     0.87s     116.8M     113         0            0         0
internal/configcli                           0.47s     0.94s     111.6M     118         0            0         1
internal/boardengine/boardtest               0.47s     1.06s      89.5M     212         0            0         0
internal/shedengine                          0.46s     1.48s     128.7M     158         0            0         0
internal/hubgeom                             0.45s     0.97s      94.1M     150         0            0         0
internal/parentreview                        0.44s     0.90s      94.3M     418         0            0         0
internal/frictionengine                      0.43s     0.91s      96.1M     135         0            0         0
internal/mergeresolve                        0.43s     0.87s      94.7M     129         0            0         0
internal/testkit/envkit                      0.43s     0.94s      99.6M     132         0            0         0
internal/gatecli                             0.42s     0.78s      96.3M     137         0            0         0
internal/hubreconcile                        0.41s     0.80s      94.3M     103         0            0         0
internal/hubforge                            0.40s     0.78s      82.6M     117         0            0         0
internal/logger                              0.40s     0.80s      73.6M     344         0            0         0
internal/githubclient                        0.40s     0.76s      88.9M     104         0            0         0
internal/testkit/shedfake                    0.40s     0.75s      88.0M     135         0            0         0
internal/gitrepo                             0.38s     0.92s      86.9M     165         0            0         0
internal/standalonegeom                      0.38s     0.78s      92.7M     107         0            0         0
internal/statuscommit                        0.37s     0.73s      85.9M      99         0            0         0
internal/gitexec                             0.37s     0.82s      52.9M     231         0            0         0
internal/testkit/tmuxkit                     0.37s     0.67s      89.2M     118         0            0         0
internal/selfreportcli                       0.37s     0.63s      69.1M     118         0            0         0
internal/shedtransient                       0.37s     0.67s      84.2M     126         0            0         0
internal/selfreportengine                    0.37s     0.66s      89.4M     102         0            0         0
internal/preflightshed                       0.37s     0.68s      86.0M     112         0            0         0
internal/ideengine                           0.36s     0.69s      86.9M     105         0            0         0
internal/stencilcli                          0.36s     0.72s      76.3M     102         0            0         0
internal/idecli                              0.35s     0.65s      92.9M     115         0            0         0
internal/testkit/shuttlefake                 0.34s     0.65s      91.2M     102         0            0         0
internal/preflight                           0.33s     0.62s      85.6M      99         0            0         0
internal/gateslot                            0.29s     0.53s      55.4M     149         0            0         0
internal/stencil                             0.26s     0.44s      50.0M     121         0            0         0
internal/vscode                              0.26s     0.55s      75.3M     113         0            0         0
internal/batcher                             0.26s     0.50s      54.6M     133         0            0         0
internal/impactset                           0.25s     0.52s      56.2M     105         0            0         0
internal/reedengine/render                   0.25s     0.49s      57.9M     110         0            0         0
internal/commentlint                         0.24s     0.46s      57.1M     137         0            0         0
internal/verifytree                          0.24s     0.48s      63.2M     121         0            0         0
internal/state                               0.24s     0.46s      45.5M     161         0            0         0
internal/lock                                0.23s     0.33s      54.9M     179         0            0         0
internal/configengine                        0.23s     0.49s      53.9M     128         0            0         0
internal/stencilstore                        0.23s     0.48s      58.1M     101         0            0         0
internal/clihelp                             0.23s     0.44s      53.5M     107         0            0         0
tools/wordswap                               0.23s     0.38s      46.5M     155         0            0         0
internal/yamlengine                          0.22s     0.52s      67.4M     247         0            0         0
tools/sandbox                                0.22s     0.47s      79.6M     104         0            0         0
internal/standalonestate                     0.21s     0.37s      54.0M     114         0            0         0
internal/discussionparser                    0.21s     0.40s      61.6M     167         0            0         0
internal/verifyrun                           0.21s     0.35s      44.8M     116         0            0         0
internal/testkit/lyxbin                      0.21s     0.32s      56.9M     102         0            0         0
internal/shedrun                             0.20s     0.41s      55.0M     122         0            0         0
internal/pattern                             0.20s     0.34s      53.9M     173         0            0         0
tools/godocreflow                            0.20s     0.34s      53.7M     256         0            0         0
internal/testkit/logcapture                  0.20s     0.31s      47.5M     132         0            0         0
internal/shell                               0.20s     0.34s      50.4M     183         0            0         0
tools/deploy                                 0.19s     0.35s      47.4M     236         0            0         0
internal/summaryparser                       0.19s     0.33s      41.6M     115         0            0         0
internal/parentdirective                     0.19s     0.30s      53.4M     161         0            0         0
internal/gitrepo/internal/gitoracle          0.19s     0.35s      59.0M     111         0            0         0
internal/testkit                             0.19s     0.30s      47.3M     113         0            0         0
internal/loggerconfig                        0.18s     0.32s      47.8M     246         0            0         0
tools/codestats                              0.18s     0.35s      55.7M     141         0            0         0
internal/testkit/scankit                     0.18s     0.36s      42.5M      92         0            0         0
internal/modelspec                           0.18s     0.37s      59.8M     101         0            0         0
contracts/specs                              0.18s     0.29s      51.6M     194         0            0         0
internal/testkit/stencilkit                  0.18s     0.31s      56.7M      90         0            0         0
internal/shedcheck                           0.17s     0.35s      51.8M      96         0            0         0
internal/testkit/llmkit                      0.17s     0.29s      49.3M     112         0            0         0
internal/buildvcs                            0.17s     0.25s      41.7M     104         0            0         0
internal/envsource                           0.17s     0.30s      50.8M     145         0            0         0
internal/friction                            0.17s     0.26s      52.3M      96         0            0         0
internal/testkit/plankit                     0.17s     0.33s      51.2M     103         0            0         0
internal/testkit/envelope                    0.17s     0.27s      43.0M     105         0            0         0
tools/mdreflow                               0.17s     0.27s      50.9M      93         0            0         0
internal/burlermarker                        0.17s     0.24s      50.1M     100         0            0         0
internal/editdirective                       0.17s     0.28s      46.5M     200         0            0         0
tools/internal/devbin                        0.17s     0.21s      44.5M     248         0            0         0
internal/fsx                                 0.16s     0.24s      53.3M     109         0            0         0
internal/testkit/locationkit                 0.16s     0.23s      44.2M      84         0            0         0
internal/proc                                0.16s     0.25s      56.7M     101         0            0         0
internal/output                              0.16s     0.24s      42.7M      89         0            0         0
internal/fslink                              0.16s     0.26s      63.5M      99         0            0         0
internal/weftname                            0.15s     0.22s      53.7M      95         0            0         0
internal/agentname                           0.15s     0.23s      48.8M      88         0            0         0
internal/buildinfo                           0.14s     0.20s      40.6M      79         0            0         0
internal/segmentcolor                        0.14s     0.21s      44.7M      55         0            0         0
internal/testkit/boardkit                    0.08s     0.26s      28.6M      41         0            0         0
internal/testkit/indexkit                    0.05s     0.00s       2.2M      31         0            0         0
internal/planindex                           0.04s     0.00s       2.7M      31         0            0         0
contracts/recipes                            0.04s     0.00s       2.0M      32         0            0         0
internal/lyxdirs                             0.03s     0.01s      16.0M      17         0            0         0  rusage
TOTAL                                       58.70s   125.43s              21130                      0        31

CPU and PEAK_MEM: the package's systemd scope cgroup (cpu.stat usage_usec, memory.peak), last sample before the scope ended.
A row marked rusage fell back to rusage because its scope could not be read.
PROCS: system-wide processes created while the package ran, so it counts anything else running on the machine.
Load average (1 min): 1.33 at start, 2.16 at end. A report reads "quiet machine" as a precondition.

Git processes by subcommand

SUBCOMMAND                         TOTAL   FIXTURE      CODE
------------------------------  --------  --------  --------
rev-parse                             25         0        25
status                                 3         0         3
unknown                                2         0         2
clone                                  1         0         1
TOTAL                                 31         0        31

FIXTURE counts git that carried the fixture marker, CODE all other git.
The split is exact for a serial package and approximate where a hub build overlaps other tests of the same package, because the marker is process-wide during a build; compare before and after on the total.
```

### Tier 2 (`integration`)

```
Resources  —  Tags: integration

PACKAGE                                       WALL       CPU   PEAK_MEM   PROCS  LEFTOVER  GIT_FIXTURE  GIT_CODE
----------------------------------------  --------  --------  ---------  ------  --------  -----------  --------
internal/fabricengine                       26.73s   150.92s     590.4M   79286         0        62560      2447
cmd/lyx                                      9.83s    18.92s     518.0M    1399         0          114        20
internal/battencli                           8.51s     8.94s     150.4M    5245         0         1326      2098
internal/webstercli                          6.05s     8.42s     186.7M    4631         0         2326       490
internal/loomcli                             4.85s    11.30s     410.9M    3165         0         1561       640
internal/fabriccli                           4.54s     9.32s     117.0M    5219         0         1694      2427
internal/testkit/lyxbin                      3.67s     5.67s     328.7M     640         0            0         0
internal/gitexec                             3.32s     0.55s      52.0M     207         0            0        29
internal/websterengine                       3.13s    10.38s     208.4M    1554         0          830       429
internal/landingshed                         2.77s     4.90s     156.3M    1486         0          451       486
internal/loomshed                            2.18s     4.72s     169.6M    1063         0           45        74
internal/gatecli                             1.82s     1.49s     101.4M     505         0           63        14
internal/orchcli                             1.66s     2.19s     114.0M    1321         0          307        50
internal/reedengine                          1.62s     0.81s      87.8M     561         0            0         0
internal/verifyrun                           1.54s     0.42s      52.0M     547         0            0         0
internal/ideengine                           1.51s     1.98s     103.0M     947         0          492       160
internal/planglyph                           1.41s     7.84s     242.7M    2584         0            0      1140
internal/shedcli                             1.40s     2.08s     181.3M    1002         0          382        89
internal/verifytree                          1.32s     0.77s      61.6M     335         0           28        79
internal/hubreconcile                        1.27s     5.70s     104.8M    2717         0         1925       137
internal/hubgeom                             1.22s     2.07s     113.5M     644         0          276        54
internal/gitrepo                             1.14s     4.12s     109.2M    1614         0          657       485
internal/hubforge                            1.12s     3.09s      98.4M    1243         0          910         8
internal/boardengine                         1.04s     2.16s     113.6M     238         0            3        62
internal/lyxcwd                              1.02s     1.66s     191.8M     253         0           13        14
internal/preflight                           0.99s     4.05s     104.7M    1633         0         1033        83
internal/shuttleengine/claudeengine          0.83s     2.27s     140.4M     331         0            0         0
internal/configcli                           0.82s     1.41s     102.9M     385         0           63       114
internal/shuttleengine                       0.78s     0.90s      95.3M     165         0            0         0
internal/boardcli                            0.75s     1.25s      96.6M     272         0            0       142
internal/burlercli                           0.74s     1.09s     112.0M     113         0            0         3
internal/boardengine/boardtest               0.70s     1.42s     101.7M     442         0           63       105
internal/treadleengine                       0.68s     1.50s     110.6M     199         0            0         0
internal/shedadapters                        0.68s     1.20s     103.7M     118         0            0         0
internal/orchengine                          0.67s     0.97s     102.7M     340         0            0         0
internal/preflightshed                       0.67s     1.44s      95.2M     709         0          439        49
internal/stencilcli                          0.67s     1.09s      92.1M     719         0           63        24
tools/tokencount                             0.60s     1.54s     128.3M     123         0            8         0
internal/loomrecipe                          0.60s     1.01s     115.1M      86         0            0         0
internal/battenrecipe                        0.58s     0.97s     109.9M     195         0            0         0
internal/gitkit                              0.58s     0.93s     182.8M     134         0           47         4
internal/mergeresolve                        0.57s     0.86s      98.0M     112         0            0         0
internal/reedcli                             0.57s     0.67s     116.4M      91         0            0         1
internal/shedrecipe                          0.54s     0.89s     109.4M     512         0            0         0
internal/burlerengine                        0.53s     1.12s     102.1M     109         0            0         0
internal/pairteardown                        0.51s     0.86s     104.6M     164         0            0         0
internal/shedbuild                           0.49s     0.84s     109.0M     303         0            0         0
internal/cliwire                             0.48s     0.90s     110.5M      88         0            0         0
internal/idecli                              0.48s     0.83s      91.8M     188         0           63         4
internal/loomengine                          0.47s     1.00s     101.9M      88         0            0         0
internal/seatengine                          0.47s     0.79s      99.4M      91         0            0         0
contracts/stencils                           0.46s     0.79s     114.3M     138         0            0         0
internal/quarrycli                           0.46s     0.71s     131.7M     128         0            0        13
internal/configreg                           0.45s     0.82s      96.9M      90         0            0         0
internal/frictionengine                      0.43s     0.87s      98.9M      85         0            0         0
cmd/testtiming                               0.40s     0.68s     104.9M     107         0            0         0
internal/battenshed                          0.39s     0.67s      88.3M      97         0            0         0
internal/configsync                          0.39s     0.74s      90.2M      98         0            0         0
internal/shuttlecli                          0.39s     0.73s     106.6M      92         0            0         3
internal/logger                              0.38s     0.84s     129.2M     166         0            0         3
internal/testkit/shuttlefake                 0.36s     0.62s      93.2M     363         0            0         0
internal/testkit/shedfake                    0.35s     0.63s      97.8M     102         0            0         0
internal/standalonegeom                      0.35s     0.64s      84.1M     111         0            0         0
internal/commentlint                         0.35s     0.53s      54.8M     123         0           11         8
internal/parentreview                        0.35s     0.58s      89.9M     106         0            0         0
internal/gateslot                            0.35s     0.59s      58.2M     111         0            0         0
internal/testkit/envkit                      0.35s     0.63s      95.3M      75         0            0         0
internal/impactset                           0.34s     0.65s      60.3M     162         0           24        11
internal/shedverbs                           0.34s     0.61s      93.3M      82         0            0         0
internal/githubclient                        0.33s     0.53s      63.9M      81         0            0         0
internal/planparser                          0.33s     0.63s      92.6M      95         0            0         0
internal/testkit/tmuxkit                     0.33s     0.62s      88.9M      92         0            0         0
internal/selfreportcli                       0.32s     0.46s      67.4M     129         0            0         0
internal/shedtransient                       0.32s     0.51s      78.6M      81         0            0         0
internal/selfreportengine                    0.32s     0.48s      65.9M      78         0            0         0
internal/statuscommit                        0.31s     0.50s      84.3M     242         0            0         0
internal/lock                                0.21s     0.27s      44.1M      76         0            0         0
internal/buildvcs                            0.20s     0.18s      41.6M      69         0            0         0
internal/batcher                             0.20s     0.31s      51.1M     123         0            0         0
internal/burlermarker                        0.19s     0.24s      47.2M      75         0            0         0
tools/wordswap                               0.19s     0.21s      40.1M      77         0            0         0
internal/clihelp                             0.19s     0.31s      49.6M      84         0            0         0
internal/modelspec                           0.19s     0.27s      49.9M     120         0            0         0
internal/shedengine                          0.19s     0.27s      49.1M     211         0            0         0
internal/configengine                        0.19s     0.28s      49.0M      89         0            0         0
internal/editdirective                       0.18s     0.32s      51.0M      87         0            0         0
internal/reedengine/render                   0.18s     0.29s      46.8M      74         0            0         0
internal/loggerconfig                        0.18s     0.31s      47.2M     107         0            0         0
internal/pattern                             0.18s     0.29s      53.6M     141         0            0         0
internal/standalonestate                     0.17s     0.29s      49.1M      77         0            0         0
internal/segmentcolor                        0.17s     0.22s      35.3M     121         0            0         0
internal/parentdirective                     0.17s     0.21s      48.8M      78         0            0         0
internal/testkit/stencilkit                  0.17s     0.25s      43.5M     107         0            0         0
internal/stencilstore                        0.17s     0.21s      43.1M      96         0            0         0
internal/friction                            0.17s     0.25s      45.9M      80         0            0         0
internal/testkit                             0.16s     0.19s      45.6M      86         0            0         0
internal/discussionparser                    0.16s     0.21s      38.5M      84         0            0         0
internal/vscode                              0.16s     0.24s      39.8M     101         0            0         0
internal/stencil                             0.16s     0.19s      42.4M     196         0            0         0
internal/shedcheck                           0.16s     0.20s      38.2M     324         0            0         0
contracts/specs                              0.16s     0.24s      41.0M      67         0            0         0
internal/testkit/logcapture                  0.16s     0.21s      44.9M     233         0            0         0
internal/testkit/plankit                     0.16s     0.22s      38.0M      72         0            0         0
internal/testkit/envelope                    0.16s     0.20s      44.6M      83         0            0         0
internal/gitrepo/internal/gitoracle          0.15s     0.23s      48.2M      69         0            0         0
tools/sandbox                                0.15s     0.22s      46.2M      73         0            0         0
internal/envsource                           0.15s     0.22s      41.7M      79         0            0         0
internal/testkit/scankit                     0.15s     0.22s      43.6M      68         0            0         0
internal/agentname                           0.15s     0.21s      45.7M      70         0            0         0
internal/yamlengine                          0.15s     0.21s      44.6M      74         0            0         0
tools/godocreflow                            0.15s     0.21s      45.5M      71         0            0         0
tools/deploy                                 0.15s     0.22s      45.3M      68         0            0         0
internal/shell                               0.15s     0.22s      43.3M      69         0            0         0
internal/fsx                                 0.14s     0.22s      50.6M      72         0            0         0
internal/summaryparser                       0.14s     0.19s      44.2M      72         0            0         0
internal/state                               0.14s     0.20s      44.1M     109         0            0         0
internal/weftname                            0.14s     0.20s      44.7M      70         0            0         0
internal/fslink                              0.14s     0.22s      43.1M      73         0            0         0
tools/internal/devbin                        0.14s     0.21s      44.8M      68         0            0         0
internal/output                              0.14s     0.19s      43.6M      67         0            0         0
tools/codestats                              0.14s     0.22s      44.7M      70         0            0         0
internal/testkit/llmkit                      0.14s     0.20s      45.1M      82         0            0         0
internal/proc                                0.13s     0.19s      45.7M      75         0            0         0
internal/buildinfo                           0.13s     0.20s      50.3M      68         0            0         0
internal/testkit/locationkit                 0.13s     0.19s      51.5M      91         0            0         0
internal/shedrun                             0.13s     0.21s      49.5M      79         0            0         0
tools/mdreflow                               0.13s     0.21s      43.7M      68         0            0         0
internal/testkit/boardkit                    0.08s     0.26s      28.4M      47         0            0         0
contracts/recipes                            0.05s     0.01s       2.2M      32         0            0         0
internal/testkit/indexkit                    0.04s     0.02s       7.5M      30         0            0         0
internal/planindex                           0.04s     0.02s       8.7M      32         0            0         0
internal/lyxdirs                             0.02s     0.00s       0.5M      29         0            0         0
TOTAL                                      128.63s   330.30s             134163                  77777     11999

CPU and PEAK_MEM: the package's systemd scope cgroup (cpu.stat usage_usec, memory.peak), last sample before the scope ended.
A row marked rusage fell back to rusage because its scope could not be read.
PROCS: system-wide processes created while the package ran, so it counts anything else running on the machine.
Load average (1 min): 2.16 at start, 3.15 at end. A report reads "quiet machine" as a precondition.

Git processes by subcommand

SUBCOMMAND                         TOTAL   FIXTURE      CODE
------------------------------  --------  --------  --------
rev-parse                          26989     23241      3748
worktree                            9913      8358      1555
commit                              4849      4464       385
add                                 4098      3698       400
rev-list                            4011      3347       664
upload-pack                         3879      3609       270
diff                                3763      2934       829
config                              3356      2959       397
branch                              2670      2340       330
clone                               2612      2563        49
for-each-ref                        2326      2063       263
push                                2246      1977       269
receive-pack                        2240      1976       264
pack-objects                        2158      1967       191
unpack-objects                      2143      1953       190
status                              1889      1359       530
reset                               1469      1395        74
checkout                            1459      1441        18
ls-remote                            866       758       108
symbolic-ref                         818       806        12
remote                               781       730        51
init                                 714       648        66
ls-files                             664       541       123
show                                 488        43       445
read-tree                            472       445        27
checkout-index                       435       411        24
fetch                                410       290       120
merge-base                           342       202       140
ls-tree                              323       177       146
show-ref                             308       308         0
merge                                205       156        49
gc                                   162       103        59
stash                                150       119        31
tag                                  122        87        35
log                                   95        62        33
switch                                63        46        17
clean                                 39        39         0
rm                                    35        32         3
merge-tree                            28        15        13
check-ignore                          25         9        16
cat-file                              21        16         5
unknown                               16         0        16
repack                                14        13         1
hash-object                           13         8         5
pull                                  12         3         9
write-tree                            12        12         0
prune-packed                           9         9         0
reflog                                 8         7         1
mv                                     6         6         0
_run_dashed_                           5         4         1
pack-refs                              5         4         1
prune                                  5         4         1
rerere                                 5         4         1
_query_                                4         4         0
commit-tree                            4         4         0
diff-index                             4         4         0
rebase                                 4         0         4
diff-tree                              3         0         3
remote-ext                             2         0         2
update-ref                             2         2         0
version                                2         0         2
_run_shell_alias_                      1         0         1
merge-ours                             1         1         0
notes                                  1         0         1
remote-curl                            1         0         1
update-index                           1         1         0
TOTAL                              89776     77777     11999

FIXTURE counts git that carried the fixture marker, CODE all other git.
The split is exact for a serial package and approximate where a hub build overlaps other tests of the same package, because the marker is process-wide during a build; compare before and after on the total.
```

### Tier 3 (`tmux`)

```
Resources  —  Tags: tmux

PACKAGE                                       WALL       CPU   PEAK_MEM   PROCS  LEFTOVER  GIT_FIXTURE  GIT_CODE
----------------------------------------  --------  --------  ---------  ------  --------  -----------  --------
internal/loomcli                            90.53s    22.84s     393.2M   15026         0         1356       804
internal/reedcli                            47.69s    54.98s    1278.2M   20104         4         1314       401
internal/reedengine                         37.50s    25.35s     238.6M   17811         0            0         0
internal/pairteardown                        3.22s     3.70s     114.1M    1649         0          333       231
cmd/lyx                                      1.87s     2.94s     188.8M     394         0            0         3
internal/webstercli                          1.24s     2.38s     170.3M     284         0            0         1
internal/orchcli                             1.23s     0.88s     112.0M     118         0            0         0
internal/lyxcwd                              0.97s     1.58s     180.5M     171         0            0         0
internal/battenrecipe                        0.97s     1.18s     120.1M    1851         0            0         0
internal/websterengine                       0.95s     2.48s     161.5M     176         0            0         3
internal/burlercli                           0.90s     1.03s     112.4M    1701         0            0         2
internal/battenshed                          0.86s     0.93s      96.4M    1437         0            0         0
internal/shuttleengine                       0.84s     0.98s      93.7M     165         0            0         0
internal/loomshed                            0.77s     1.18s     159.1M     128         0            0         0
internal/boardcli                            0.76s     0.88s      88.2M    1618         0            0         0
internal/boardengine/boardtest               0.76s     1.30s     100.6M    1887         0            0         0
internal/standalonegeom                      0.75s     0.87s      87.7M     288         0            0         1
internal/boardengine                         0.70s     0.86s      90.6M    2176         0            0         0
internal/shedcli                             0.70s     1.17s     179.6M     115         0            0         0
internal/shedadapters                        0.66s     1.05s     104.0M     103         0            0         0
internal/battencli                           0.65s     1.07s     122.4M     926         0            0         0
internal/burlerengine                        0.65s     1.08s     106.4M    1004         0            0         0
internal/loomrecipe                          0.63s     1.08s     116.4M      87         0            0         0
internal/orchengine                          0.62s     0.95s     108.2M      85         0            0         0
internal/planglyph                           0.59s     1.35s     127.1M     127         0            0         0
internal/cliwire                             0.55s     0.88s      99.3M     566         0            0         0
internal/treadleengine                       0.55s     0.83s      90.0M     131         0            0         0
internal/testkit/tmuxkit                     0.54s     0.85s      85.0M     167         0            0         0
internal/shedrecipe                          0.52s     0.87s     111.7M      87         0            0         0
internal/gitkit                              0.51s     0.79s     166.3M      88         0            0         0
internal/landingshed                         0.50s     0.88s     107.4M      86         0            0         0
internal/quarrycli                           0.50s     0.83s     134.3M     128         0            0        13
tools/tokencount                             0.47s     0.78s      99.1M     211         0            0         0
internal/loomengine                          0.46s     0.81s      89.4M      88         0            0         0
internal/shedbuild                           0.45s     0.79s     109.6M      81         0            0         0
internal/seatengine                          0.45s     0.82s     101.5M     168         0            0         0
internal/shuttleengine/claudeengine          0.45s     0.77s      89.6M      88         0            0         0
internal/configcli                           0.44s     0.72s     113.7M     604         0            0         1
contracts/stencils                           0.44s     0.85s     105.2M      99         0            0         0
internal/configreg                           0.43s     0.76s     109.8M     110         0            0         0
internal/shedverbs                           0.41s     0.70s      91.8M      96         0            0         0
internal/fabriccli                           0.41s     0.69s      93.8M     118         0            0         0
internal/planparser                          0.41s     0.66s      93.5M     130         0            0         0
internal/parentreview                        0.41s     0.70s      90.3M      81         0            0         0
internal/fabricengine                        0.40s     0.69s      91.3M     119         0            0         3
internal/frictionengine                      0.39s     0.73s      98.0M      85         0            0         0
internal/gatecli                             0.39s     0.73s     103.4M     263         0            0         0
internal/shuttlecli                          0.39s     0.72s      98.5M     109         0            0         3
cmd/testtiming                               0.38s     0.74s      93.0M     142         0            0         0
internal/hubforge                            0.38s     0.70s      99.4M      76         0            0         0
internal/preflightshed                       0.37s     0.71s      97.9M      76         0            0         0
internal/mergeresolve                        0.37s     0.73s      96.1M     100         0            0         0
internal/hubreconcile                        0.37s     0.64s      82.1M      84         0            0         0
internal/hubgeom                             0.37s     0.62s      96.9M     118         0            0         0
internal/testkit/envkit                      0.36s     0.63s      82.3M      79         0            0         0
internal/configsync                          0.35s     0.60s      87.6M      95         0            0         0
internal/stencilcli                          0.34s     0.62s      84.0M      85         0            0         0
internal/testkit/shedfake                    0.34s     0.62s      94.7M      79         0            0         0
internal/shedtransient                       0.34s     0.62s      87.6M      95         0            0         0
internal/testkit/shuttlefake                 0.33s     0.61s      91.5M      79         0            0         0
internal/preflight                           0.33s     0.61s      90.5M      82         0            0         0
internal/idecli                              0.33s     0.62s      81.9M      76         0            0         0
internal/ideengine                           0.33s     0.62s      89.2M      74         0            0         0
internal/gitrepo                             0.31s     0.53s      82.5M      81         0            0         0
internal/githubclient                        0.29s     0.47s      68.6M      78         0            0         0
internal/selfreportcli                       0.29s     0.49s      75.0M      79         0            0         0
internal/statuscommit                        0.29s     0.54s      85.0M      77         0            0         0
internal/selfreportengine                    0.28s     0.48s      67.2M      80         0            0         0
internal/pattern                             0.24s     0.38s      54.3M      93         0            0         0
internal/gateslot                            0.23s     0.34s      54.1M      81         0            0         0
internal/clihelp                             0.23s     0.35s      54.5M     406         0            0         0
internal/logger                              0.22s     0.45s     130.7M      87         0            0         0
internal/parentdirective                     0.22s     0.28s      46.5M      76         0            0         0
internal/reedengine/render                   0.22s     0.26s      52.2M      82         0            0         0
internal/commentlint                         0.21s     0.28s      50.6M     255         0            0         0
internal/gitexec                             0.20s     0.31s      49.0M      85         0            0         0
internal/buildinfo                           0.20s     0.27s      44.2M     513         0            0         0
internal/standalonestate                     0.19s     0.31s      50.0M      76         0            0         0
internal/verifytree                          0.19s     0.30s      57.1M      75         0            0         0
internal/lock                                0.19s     0.23s      52.3M      74         0            0         0
internal/testkit/lyxbin                      0.19s     0.28s      45.5M      78         0            0         0
internal/verifyrun                           0.19s     0.30s      52.5M      79         0            0         0
internal/impactset                           0.19s     0.31s      53.5M      82         0            0         0
internal/batcher                             0.19s     0.30s      50.3M     199         0            0         0
internal/buildvcs                            0.19s     0.24s      46.9M     478         0            0         0
internal/testkit/plankit                     0.18s     0.27s      44.5M      66         0            0         0
internal/shedengine                          0.18s     0.29s      46.6M      90         0            0         0
internal/stencil                             0.17s     0.28s      46.5M      85         0            0         0
internal/friction                            0.17s     0.23s      50.1M      73         0            0         0
internal/loggerconfig                        0.16s     0.24s      47.0M      85         0            0         0
internal/testkit/stencilkit                  0.16s     0.24s      41.1M      72         0            0         0
tools/sandbox                                0.16s     0.23s      46.8M      90         0            0         0
internal/vscode                              0.16s     0.23s      44.8M      76         0            0         0
internal/testkit                             0.16s     0.22s      43.7M     106         0            0         0
internal/summaryparser                       0.16s     0.22s      42.4M      88         0            0         0
contracts/specs                              0.16s     0.23s      47.5M      84         0            0         0
internal/testkit/logcapture                  0.16s     0.22s      41.3M      72         0            0         0
internal/stencilstore                        0.16s     0.24s      48.3M      73         0            0         0
internal/state                               0.16s     0.22s      43.8M     101         0            0         0
internal/yamlengine                          0.16s     0.22s      43.8M      79         0            0         0
internal/weftname                            0.15s     0.20s      40.9M      70         0            0         0
tools/godocreflow                            0.15s     0.21s      39.2M      72         0            0         0
internal/proc                                0.15s     0.21s      48.1M      79         0            0         0
tools/mdreflow                               0.15s     0.22s      48.9M      68         0            0         0
internal/gitrepo/internal/gitoracle          0.15s     0.23s      47.8M      69         0            0         0
internal/testkit/scankit                     0.15s     0.20s      38.1M      67         0            0         0
internal/testkit/envelope                    0.15s     0.21s      41.0M      68         0            0         0
internal/shedcheck                           0.15s     0.23s      42.4M      83         0            0         0
internal/configengine                        0.15s     0.21s      48.7M      87         0            0         0
tools/internal/devbin                        0.15s     0.21s      40.5M      87         0            0         0
tools/wordswap                               0.15s     0.22s      50.5M      68         0            0         0
tools/deploy                                 0.15s     0.22s      48.2M      69         0            0         0
internal/output                              0.15s     0.21s      45.9M      68         0            0         0
internal/agentname                           0.15s     0.22s      41.9M      76         0            0         0
internal/testkit/llmkit                      0.14s     0.21s      41.8M      68         0            0         0
internal/segmentcolor                        0.14s     0.21s      46.5M      70         0            0         0
internal/modelspec                           0.14s     0.24s      48.3M      74         0            0         0
internal/shedrun                             0.14s     0.22s      51.9M      76         0            0         0
internal/testkit/locationkit                 0.14s     0.21s      50.8M      67         0            0         0
internal/editdirective                       0.14s     0.22s      54.0M      76         0            0         0
tools/codestats                              0.14s     0.22s      46.8M      83         0            0         0
internal/shell                               0.14s     0.21s      49.9M      67         0            0         0
internal/burlermarker                        0.14s     0.20s      41.6M     138         0            0         0
internal/fslink                              0.14s     0.21s      41.9M      70         0            0         0
internal/envsource                           0.14s     0.20s      50.9M      74         0            0         0
internal/fsx                                 0.13s     0.21s      45.7M      77         0            0         0
internal/discussionparser                    0.13s     0.20s      53.5M      82         0            0         0
internal/testkit/boardkit                    0.09s     0.27s      28.5M      36         0            0         0
internal/testkit/indexkit                    0.05s     0.00s       0.5M      31         0            0         0
internal/planindex                           0.05s     0.00s       2.5M      31         0            0         0
contracts/recipes                            0.05s     0.02s       9.0M      31         0            0         0
internal/lyxdirs                             0.02s     0.01s       6.1M      17         0            0         0
TOTAL                                      224.35s   178.96s              81127                   3003      1466

Leftover processes (pid argv):
  internal/reedcli: 3145250 /tmp/ttr-2652009940/TestSmokeWarmPath2172589440/001/lyx reed watchdog --hub-path /tmp/ttr-2652009940/TestSmokeWarmPath2172589440/003/warp-bare-LYXHUB --tmux tmux --shell bash
  internal/reedcli: 3146130 /tmp/ttr-2652009940/TestSmokeColdStart248267788/001/lyx reed watchdog --hub-path /tmp/ttr-2652009940/TestSmokeColdStart248267788/003/warp-bare-LYXHUB --tmux tmux --shell bash
  internal/reedcli: 3147087 sleep 300
  internal/reedcli: 3152518 /tmp/ttr-2652009940/TestSmokeLifecycle1803570383/001/lyx reed watchdog --hub-path /tmp/ttr-2652009940/TestSmokeLifecycle1803570383/003/warp-bare-LYXHUB --tmux tmux --shell bash

CPU and PEAK_MEM: the package's systemd scope cgroup (cpu.stat usage_usec, memory.peak), last sample before the scope ended.
A row marked rusage fell back to rusage because its scope could not be read.
PROCS: system-wide processes created while the package ran, so it counts anything else running on the machine.
Load average (1 min): 3.15 at start, 1.61 at end. A report reads "quiet machine" as a precondition.

Git processes by subcommand

SUBCOMMAND                         TOTAL   FIXTURE      CODE
------------------------------  --------  --------  --------
rev-parse                           1664       820       844
worktree                             492       363       129
diff                                 223        92       131
commit                               209       157        52
config                               194       171        23
add                                  193       124        69
rev-list                             174       123        51
upload-pack                          165       165         0
push                                 117        91        26
receive-pack                         117        91        26
for-each-ref                         111        91        20
clone                                110       109         1
pack-objects                         105        91        14
unpack-objects                       105        91        14
branch                               103        88        15
status                                66        28        38
reset                                 60        60         0
ls-remote                             56        56         0
symbolic-ref                          35        35         0
checkout                              32        32         0
remote                                31        31         0
checkout-index                        28        28         0
ls-files                              28        28         0
read-tree                             28        28         0
init                                  10         9         1
gc                                     6         0         6
tag                                    3         0         3
unknown                                2         0         2
rm                                     1         1         0
show                                   1         0         1
TOTAL                               4469      3003      1466

FIXTURE counts git that carried the fixture marker, CODE all other git.
The split is exact for a serial package and approximate where a hub build overlaps other tests of the same package, because the marker is process-wide during a build; compare before and after on the total.
```

## After

```yaml
commit: 9e15e5ed6
date: 2026-10-10
```

The three commands under [Regenerate](#regenerate) ran once each on the finished tree, serially, with the `llm` tier left out as above.
The machine was loaded: the load average (1 min) ran from 3.06 to 8.08 over the three runs on 12 threads, against 1.33 to 3.15 before, so wall figures read high and the CPU, git and process counts are the steadier comparison.
No package failed and none left a process behind.

### Tier 1 (untagged)

```
Resources  —  Tier 1 (offline)

PACKAGE                                       WALL       CPU   PEAK_MEM   PROCS  LEFTOVER  GIT_FIXTURE  GIT_CODE
----------------------------------------  --------  --------  ---------  ------  --------  -----------  --------
internal/loomcli                             2.01s     4.35s     232.7M    1531         0            0         0
cmd/lyx                                      1.85s     2.93s     212.2M     126         0            0         0
internal/websterengine                       1.76s     5.87s     208.4M    1011         0            0         3
internal/reedengine                          1.63s     0.75s     104.3M    1245         0            0         0
internal/orchcli                             1.42s     1.24s     119.8M    1554         0            0         0
internal/fabricengine                        1.35s     3.79s     224.6M     373         0            0         3
internal/loomshed                            1.08s     2.26s     157.2M     996         0            0         0
internal/lyxcwd                              1.07s     1.61s     193.8M    1127         0            0         0
internal/webstercli                          1.02s     2.09s     179.3M     984         0            0         0
internal/planglyph                           0.96s     2.41s     154.8M     758         0            0         0
internal/landingshed                         0.87s     1.85s     152.1M     675         0            0         0
internal/shuttleengine/claudeengine          0.80s     1.95s     144.7M     471         0            0         0
internal/burlercli                           0.76s     1.09s     117.1M     118         0            0         0
internal/shedcli                             0.76s     1.31s     174.3M     448         0            0         0
internal/loomrecipe                          0.75s     1.12s     114.9M     576         0            0         0
internal/gitkit                              0.73s     1.20s     176.2M     391         0            0         0
internal/shuttleengine                       0.73s     0.64s      97.5M     351         0            0         0
internal/orchengine                          0.72s     1.20s      92.6M     625         0            0         0
internal/shedadapters                        0.72s     1.07s     108.7M     527         0            0         0
internal/battencli                           0.69s     1.59s     135.4M     117         0            0         0
internal/treadleengine                       0.66s     1.35s     113.8M     286         0            0         0
internal/reedcli                             0.63s     0.77s      96.7M     139         0            0         0
internal/boardengine                         0.61s     1.76s     113.9M     144         0            0         0
internal/pairteardown                        0.60s     0.97s     101.4M     628         0            0         0
tools/tokencount                             0.59s     1.46s     127.5M     129         0            0         0
internal/shedrecipe                          0.56s     0.87s     124.4M     433         0            0         0
internal/fabriccli                           0.54s     0.87s     102.6M     300         0            0         0
internal/gatecli                             0.54s     0.91s     107.6M     196         0            0         0
internal/burlerengine                        0.54s     1.13s     101.4M      89         0            0         0
internal/loomengine                          0.53s     0.96s     101.8M     401         0            0         0
internal/shedbuild                           0.52s     0.80s     118.8M     255         0            0         0
cmd/testtiming                               0.50s     1.18s     115.7M     104         0            0         0
internal/configcli                           0.50s     1.07s      95.3M     122         0            0         0
internal/frictionengine                      0.49s     0.79s     100.7M     209         0            0         0
internal/cliwire                             0.49s     0.87s      95.6M      95         0            0         0
internal/configreg                           0.48s     0.85s      90.7M     101         0            0         0
internal/quarrycli                           0.48s     0.81s     123.0M     189         0            0         0
internal/battenrecipe                        0.47s     0.70s     105.6M      85         0            0         0
internal/mergeresolve                        0.46s     0.71s      96.7M     445         0            0         0
internal/hubgeom                             0.45s     0.74s     101.1M     302         0            0         0
internal/hubreconcile                        0.45s     0.80s     107.8M     523         0            0         0
internal/parentreview                        0.44s     0.66s      81.3M     361         0            0         0
internal/boardcli                            0.43s     0.71s      92.8M     103         0            0         0
internal/gateslot                            0.43s     0.60s      57.5M     120         0            0         0
internal/ideengine                           0.43s     0.75s      88.5M     554         0            0         0
contracts/stencils                           0.43s     0.75s     102.3M      73         0            0         0
internal/configsync                          0.42s     0.70s      88.7M     135         0            0         0
internal/seatengine                          0.42s     0.65s      92.7M     267         0            0         0
internal/idecli                              0.42s     0.68s      94.3M     485         0            0         0
internal/gitrepo                             0.41s     0.63s      87.8M     333         0            0         0
internal/testkit/envkit                      0.41s     0.64s      97.8M     391         0            0         0
internal/standalonegeom                      0.39s     0.71s      89.2M     238         0            0         0
internal/battenshed                          0.39s     0.62s      89.1M      72         0            0         0
internal/hubforge                            0.39s     0.67s      93.3M      79         0            0         0
internal/testkit/shedfake                    0.39s     0.64s      96.0M     593         0            0         0
internal/stencilcli                          0.38s     0.73s      88.0M     425         0            0         0
internal/preflightshed                       0.38s     0.72s     104.3M     105         0            0         0
internal/boardengine/boardtest               0.38s     0.80s      82.8M     119         0            0         0
internal/shuttlecli                          0.38s     0.66s     101.1M     201         0            0         0
internal/shedverbs                           0.36s     0.58s     100.6M     235         0            0         0
internal/githubclient                        0.35s     0.48s      67.7M     110         0            0         0
internal/statuscommit                        0.33s     0.55s      79.2M     412         0            0         0
internal/planparser                          0.32s     0.65s      91.4M      93         0            0         0
internal/shedtransient                       0.32s     0.49s      82.3M     163         0            0         0
internal/testkit/tmuxkit                     0.32s     0.48s      86.7M      84         0            0         0
internal/preflight                           0.30s     0.51s      88.2M      80         0            0         0
internal/testkit/shuttlefake                 0.29s     0.50s     100.1M      83         0            0         0
internal/impactset                           0.29s     0.51s      56.9M     325         0            0         0
internal/logger                              0.28s     0.52s     131.5M     123         0            0         0
internal/selfreportengine                    0.28s     0.43s      68.4M     323         0            0         0
internal/selfreportcli                       0.27s     0.45s      65.2M     237         0            0         0
internal/pattern                             0.27s     0.30s      52.8M     611         0            0         0
internal/gitexec                             0.27s     0.36s      47.4M     180         0            0         0
internal/verifyrun                           0.26s     0.37s      41.0M     534         0            0         0
internal/commentlint                         0.25s     0.47s      59.9M     102         0            0         0
internal/verifytree                          0.24s     0.43s      57.8M     186         0            0         0
internal/testkit/lyxbin                      0.23s     0.39s      56.4M     360         0            0         0
internal/lock                                0.22s     0.21s      41.9M      94         0            0         0
internal/clihelp                             0.20s     0.29s      50.1M     103         0            0         0
internal/shedengine                          0.20s     0.26s      43.0M     442         0            0         0
internal/parentdirective                     0.18s     0.26s      53.0M      82         0            0         0
internal/standalonestate                     0.18s     0.28s      49.1M     130         0            0         0
internal/stencil                             0.18s     0.25s      49.9M     321         0            0         0
internal/reedengine/render                   0.18s     0.26s      43.2M     285         0            0         0
internal/friction                            0.17s     0.27s      49.3M      77         0            0         0
internal/fsx                                 0.17s     0.18s      34.8M     105         0            0         0
internal/testkit/stencilkit                  0.17s     0.20s      43.2M     130         0            0         0
internal/gitrepo/internal/gitoracle          0.17s     0.27s      48.8M      71         0            0         0
internal/editdirective                       0.17s     0.21s      42.9M      57         0            0         0
internal/fslink                              0.16s     0.17s      37.9M      85         0            0         0
internal/envsource                           0.16s     0.17s      36.1M      74         0            0         0
internal/vscode                              0.16s     0.20s      42.5M     298         0            0         0
internal/configengine                        0.16s     0.22s      45.1M      69         0            0         0
internal/loggerconfig                        0.16s     0.20s      40.2M      66         0            0         0
internal/batcher                             0.16s     0.20s      46.3M      62         0            0         0
internal/summaryparser                       0.16s     0.19s      35.1M     269         0            0         0
internal/shell                               0.15s     0.17s      39.4M     206         0            0         0
internal/dotgit                              0.15s     0.17s      39.6M     131         0            0         0
internal/output                              0.14s     0.18s      44.6M      88         0            0         0
internal/shedcheck                           0.14s     0.19s      41.1M      99         0            0         0
internal/segmentcolor                        0.14s     0.17s      37.9M     192         0            0         0
contracts/specs                              0.14s     0.19s      45.6M      54         0            0         0
internal/burlermarker                        0.14s     0.20s      41.8M      59         0            0         0
internal/testkit                             0.14s     0.18s      42.4M     141         0            0         0
internal/stencilstore                        0.14s     0.21s      50.4M      99         0            0         0
internal/testkit/logcapture                  0.14s     0.20s      44.3M     118         0            0         0
internal/modelspec                           0.14s     0.21s      48.7M     102         0            0         0
internal/discussionparser                    0.14s     0.18s      41.7M      87         0            0         0
internal/buildinfo                           0.14s     0.19s      41.9M      77         0            0         0
internal/testkit/scankit                     0.14s     0.19s      40.8M     185         0            0         0
tools/sandbox                                0.13s     0.20s      45.5M      64         0            0         0
internal/state                               0.13s     0.20s      42.0M     148         0            0         0
internal/shedrun                             0.13s     0.19s      44.7M      88         0            0         0
tools/wordswap                               0.13s     0.20s      43.9M      76         0            0         0
tools/internal/devbin                        0.13s     0.19s      40.2M      94         0            0         0
internal/testkit/locationkit                 0.13s     0.19s      41.1M      93         0            0         0
internal/proc                                0.13s     0.20s      43.3M      63         0            0         0
tools/godocreflow                            0.13s     0.19s      40.3M      78         0            0         0
internal/buildvcs                            0.13s     0.17s      42.2M      55         0            0         0
internal/agentname                           0.12s     0.19s      43.7M      56         0            0         0
internal/yamlengine                          0.12s     0.20s      44.2M      66         0            0         0
internal/testkit/plankit                     0.12s     0.19s      42.2M      77         0            0         0
internal/testkit/envelope                    0.12s     0.19s      41.9M      71         0            0         0
internal/testkit/llmkit                      0.12s     0.18s      43.1M      61         0            0         0
tools/deploy                                 0.12s     0.13s      33.0M      55         0            0         0
tools/mdreflow                               0.12s     0.13s      37.3M      71         0            0         0
internal/weftname                            0.12s     0.13s      37.7M      56         0            0         0
tools/codestats                              0.12s     0.14s      36.1M      55         0            0         0
internal/testkit/boardkit                    0.07s     0.27s      30.9M      45         0            0         0
internal/testkit/indexkit                    0.05s     0.03s       9.4M      31         0            0         0
internal/planindex                           0.04s     0.00s       3.0M      32         0            0         0
contracts/recipes                            0.04s     0.00s       2.5M      31         0            0         0
internal/lyxdirs                             0.02s     0.01s       4.7M      29         0            0         0
TOTAL                                       53.07s    91.88s              34027                      0         6

CPU and PEAK_MEM: the package's systemd scope cgroup (cpu.stat usage_usec, memory.peak), last sample before the scope ended.
A row marked rusage fell back to rusage because its scope could not be read.
PROCS: system-wide processes created while the package ran, so it counts anything else running on the machine.
Load average (1 min): 8.08 at start, 5.98 at end. A report reads "quiet machine" as a precondition.

Git processes by subcommand

SUBCOMMAND                         TOTAL   FIXTURE      CODE
------------------------------  --------  --------  --------
status                                 3         0         3
unknown                                2         0         2
clone                                  1         0         1
TOTAL                                  6         0         6

FIXTURE counts git that carried the fixture marker, CODE all other git.
The split is exact for a serial package and approximate where a hub build overlaps other tests of the same package, because the marker is process-wide during a build; compare before and after on the total.
```

### Tier 2 (`integration`)

```
Resources  —  Tags: integration

PACKAGE                                       WALL       CPU   PEAK_MEM   PROCS  LEFTOVER  GIT_FIXTURE  GIT_CODE
----------------------------------------  --------  --------  ---------  ------  --------  -----------  --------
internal/fabricengine                       16.61s    95.89s     381.5M   47696         0        32785      3933
internal/testkit/lyxbin                      8.46s     7.70s     449.5M   18835         0            0         0
internal/webstercli                          8.00s     9.69s     188.0M   48009         0         1965       395
internal/battencli                           7.78s     7.98s     152.1M    3743         0         1047      1584
internal/loomcli                             4.34s     8.91s     245.1M    2921         0         1101       405
cmd/lyx                                      4.30s    10.35s     463.6M     690         0           78         7
internal/fabriccli                           4.00s     8.23s     117.1M    4137         0         1332      1802
internal/gitexec                             3.29s     0.41s      51.0M     226         0            0        32
internal/websterengine                       2.46s     5.29s     179.7M    9980         0          830       430
internal/landingshed                         2.46s     3.20s     119.8M    1647         0          335       354
internal/loomshed                            2.16s     3.54s     163.3M    1315         0           45        74
internal/orchcli                             1.96s     2.62s     109.8M     869         0          210        25
internal/gatecli                             1.85s     1.51s      99.4M     617         0           59         0
internal/shedcli                             1.83s     2.52s     184.2M    4802         0          318        29
internal/reedengine                          1.74s     0.84s     100.0M     873         0            0         0
internal/ideengine                           1.61s     2.08s     105.8M    1525         0          406       140
internal/planglyph                           1.55s     7.05s     277.1M    2609         0            0      1140
internal/hubreconcile                        1.52s     4.49s     107.3M    2389         0         1103       138
internal/verifyrun                           1.52s     0.31s      48.9M    3471         0            0         0
internal/verifytree                          1.20s     0.44s      52.6M    1812         0           28        79
internal/stencilcli                          1.07s     1.40s     105.8M     798         0           59        10
internal/gitrepo                             1.03s     3.41s      84.3M    1768         0          670       504
internal/lyxcwd                              1.03s     1.51s     196.0M     181         0           16         0
internal/hubforge                            0.97s     2.75s     101.0M    1242         0          792         6
internal/preflight                           0.94s     2.38s     111.9M    1850         0          272       123
internal/shedrecipe                          0.93s     1.16s     125.3M     541         0            0         0
internal/shedadapters                        0.93s     1.33s     114.9M    1757         0            0         0
internal/orchengine                          0.92s     1.33s      99.1M    1436         0            0         0
internal/shuttleengine                       0.91s     0.82s     107.8M    1148         0            0         0
internal/hubgeom                             0.90s     1.37s     102.9M     503         0          232        27
internal/preflightshed                       0.88s     1.62s      92.4M    1842         0          270        26
internal/configcli                           0.79s     1.47s     113.7M     336         0           59        86
internal/boardengine/boardtest               0.78s     1.57s     109.3M     348         0           59       105
internal/reedcli                             0.75s     0.80s     101.6M    1782         0            0         0
internal/quarrycli                           0.75s     0.99s     123.1M    2674         0            0         0
internal/testkit/envkit                      0.71s     0.86s     105.9M    2219         0            0         0
internal/idecli                              0.69s     1.02s     105.5M     451         0           59         1
internal/shuttleengine/claudeengine          0.68s     1.10s      95.6M     631         0            0         0
internal/loomrecipe                          0.68s     1.04s     107.7M     162         0            0         0
internal/boardengine                         0.67s     0.75s      93.2M     158         0            3        62
internal/burlercli                           0.67s     0.86s     108.5M      86         0            0         0
internal/pairteardown                        0.61s     0.85s     108.7M     656         0            0         0
internal/gitkit                              0.60s     0.92s     186.8M     143         0           47         5
internal/shedbuild                           0.56s     0.87s     110.4M     345         0            0         0
internal/loomengine                          0.55s     0.91s     102.3M     215         0            0         0
internal/treadleengine                       0.53s     0.84s      97.1M    1418         0            0         0
internal/battenrecipe                        0.53s     0.95s     109.5M      90         0            0         0
internal/burlerengine                        0.52s     0.90s      97.4M      81         0            0         0
internal/testkit/shedfake                    0.49s     0.69s     104.1M    1784         0            0         0
internal/shedverbs                           0.47s     0.70s     102.7M     459         0            0         0
internal/seatengine                          0.47s     0.70s     102.5M     281         0            0         0
internal/selfreportcli                       0.47s     0.55s      75.5M     408         0            0         0
internal/cliwire                             0.46s     0.78s      93.6M      81         0            0         0
internal/boardcli                            0.46s     0.75s      95.8M      92         0            0         1
internal/shuttlecli                          0.46s     0.71s      93.5M     622         0            0         0
internal/statuscommit                        0.45s     0.61s      81.3M     553         0            0         0
internal/parentreview                        0.44s     0.65s      85.5M     401         0            0         0
contracts/stencils                           0.43s     0.79s     114.8M      72         0            0         0
internal/shedtransient                       0.43s     0.67s      95.4M     221         0            0         0
internal/standalonegeom                      0.43s     0.63s      96.5M     521         0            0         0
tools/tokencount                             0.43s     0.77s      97.9M      84         0            8         0
internal/battenshed                          0.43s     0.74s      99.1M     127         0            0         0
internal/planparser                          0.42s     0.61s      98.5M     191         0            0         0
internal/configreg                           0.42s     0.73s      92.2M      71         0            0         0
cmd/testtiming                               0.42s     0.69s      91.4M     150         0            0         0
internal/mergeresolve                        0.41s     0.64s      97.2M      74         0            0         0
internal/testkit/shuttlefake                 0.39s     0.62s      81.8M     993         0            0         0
internal/testkit/tmuxkit                     0.38s     0.62s      86.1M     962         0            0         0
internal/frictionengine                      0.37s     0.60s     104.1M      83         0            0         0
internal/selfreportengine                    0.37s     0.52s      64.9M     141         0            0         0
internal/configsync                          0.34s     0.65s      91.0M      72         0            0         0
internal/shedengine                          0.32s     0.33s      45.9M     179         0            0         0
internal/githubclient                        0.32s     0.46s      67.8M     119         0            0         0
internal/impactset                           0.31s     0.39s      57.2M     253         0           24        11
internal/logger                              0.28s     0.45s     135.1M     141         0            0         1
internal/stencilstore                        0.28s     0.34s      47.2M     338         0            0         0
internal/gateslot                            0.27s     0.37s      58.1M      74         0            0         0
internal/testkit/plankit                     0.27s     0.27s      42.6M    1290         0            0         0
internal/testkit/llmkit                      0.27s     0.25s      39.0M    1122         0            0         0
internal/testkit/scankit                     0.26s     0.27s      38.5M    1219         0            0         0
internal/output                              0.22s     0.23s      40.0M     244         0            0         0
internal/commentlint                         0.22s     0.31s      56.5M      91         0           11         8
internal/state                               0.21s     0.22s      42.6M     574         0            0         0
internal/standalonestate                     0.21s     0.25s      51.0M     175         0            0         0
internal/modelspec                           0.21s     0.29s      46.0M      69         0            0         0
internal/pattern                             0.21s     0.26s      53.4M     247         0            0         0
internal/lock                                0.21s     0.22s      45.5M     122         0            0         0
internal/shedrun                             0.21s     0.21s      47.3M      85         0            0         0
internal/testkit/locationkit                 0.20s     0.22s      47.9M     488         0            0         0
internal/gitrepo/internal/gitoracle          0.20s     0.24s      44.9M     129         0            0         0
internal/testkit                             0.20s     0.24s      41.9M      91         0            0         0
internal/parentdirective                     0.19s     0.28s      46.2M     223         0            0         0
internal/stencil                             0.19s     0.25s      47.5M      76         0            0         0
internal/batcher                             0.18s     0.32s      51.1M      72         0            0         0
internal/testkit/stencilkit                  0.18s     0.26s      50.8M     498         0            0         0
internal/testkit/envelope                    0.18s     0.24s      40.1M     387         0            0         0
internal/testkit/logcapture                  0.18s     0.20s      39.9M     192         0            0         0
internal/summaryparser                       0.18s     0.24s      43.5M     167         0            0         0
internal/clihelp                             0.17s     0.22s      44.0M      69         0            0         0
internal/reedengine/render                   0.17s     0.17s      37.3M      58         0            0         0
internal/loggerconfig                        0.17s     0.19s      39.3M      95         0            0         0
internal/proc                                0.16s     0.18s      41.4M     336         0            0         0
internal/shell                               0.16s     0.23s      41.8M     304         0            0         0
internal/segmentcolor                        0.16s     0.17s      38.6M     158         0            0         0
internal/shedcheck                           0.16s     0.19s      40.9M     338         0            0         0
tools/sandbox                                0.15s     0.19s      41.3M      69         0            0         0
internal/configengine                        0.15s     0.22s      42.9M      64         0            0         0
internal/vscode                              0.15s     0.19s      39.3M     192         0            0         0
internal/editdirective                       0.14s     0.23s      48.8M      58         0            0         0
contracts/specs                              0.14s     0.21s      44.4M      59         0            0         0
tools/codestats                              0.14s     0.18s      42.5M      55         0            0         0
internal/buildinfo                           0.14s     0.20s      44.5M      55         0            0         0
internal/burlermarker                        0.14s     0.20s      42.5M      59         0            0         0
internal/buildvcs                            0.14s     0.20s      43.8M      55         0            0         0
internal/dotgit                              0.13s     0.21s      41.6M      63         0            0         0
internal/agentname                           0.13s     0.21s      41.7M      56         0            0         0
internal/discussionparser                    0.13s     0.21s      43.1M      63         0            0         0
internal/friction                            0.13s     0.22s      49.6M      59         0            0         0
internal/yamlengine                          0.13s     0.19s      43.3M      60         0            0         0
tools/internal/devbin                        0.13s     0.18s      43.6M      53         0            0         0
tools/wordswap                               0.13s     0.18s      41.6M      55         0            0         0
tools/mdreflow                               0.13s     0.18s      44.0M      60         0            0         0
tools/deploy                                 0.13s     0.19s      43.4M      55         0            0         0
tools/godocreflow                            0.13s     0.19s      45.2M      58         0            0         0
internal/fsx                                 0.13s     0.18s      43.3M      56         0            0         0
internal/weftname                            0.13s     0.19s      42.1M      75         0            0         0
internal/envsource                           0.12s     0.21s      41.9M      55         0            0         0
internal/fslink                              0.12s     0.19s      41.3M      74         0            0         0
internal/testkit/boardkit                    0.10s     0.19s      26.7M      49         0            0         0
internal/testkit/indexkit                    0.08s     0.09s      16.3M     354         0            0         0
internal/planindex                           0.05s     0.00s       2.2M      31         0            0         0
contracts/recipes                            0.04s     0.01s       6.7M      32         0            0         0
internal/lyxdirs                             0.03s     0.01s       5.7M      17         0            0         0
TOTAL                                      120.72s   248.08s             203484                  44223     11543

CPU and PEAK_MEM: the package's systemd scope cgroup (cpu.stat usage_usec, memory.peak), last sample before the scope ended.
A row marked rusage fell back to rusage because its scope could not be read.
PROCS: system-wide processes created while the package ran, so it counts anything else running on the machine.
Load average (1 min): 4.05 at start, 8.08 at end. A report reads "quiet machine" as a precondition.

Git processes by subcommand

SUBCOMMAND                         TOTAL   FIXTURE      CODE
------------------------------  --------  --------  --------
worktree                            8453      6532      1921
rev-parse                           5972      4583      1389
commit                              3401      2956       445
add                                 3387      2933       454
config                              3362      2881       481
rev-list                            3306      2494       812
for-each-ref                        2355      2055       300
diff                                2290      1114      1176
push                                2275      1975       300
receive-pack                        2269      1974       295
pack-objects                        2186      1966       220
unpack-objects                      2171      1952       219
branch                              1938      1492       446
status                              1892      1251       641
upload-pack                         1670      1308       362
ls-remote                            868       744       124
remote                               783       718        65
reset                                735       646        89
checkout                             722       704        18
init                                 717       648        69
ls-files                             665       507       158
show                                 488        43       445
read-tree                            473       436        37
checkout-index                       436       403        33
fetch                                410       212       198
clone                                401       356        45
merge-base                           343       134       209
ls-tree                              323       186       137
show-ref                             308       308         0
merge                                205       111        94
gc                                   162        83        79
stash                                150        80        70
tag                                  123        80        43
log                                   95        57        38
symbolic-ref                          81        69        12
switch                                63        41        22
clean                                 39        39         0
rm                                    35        32         3
merge-tree                            28        12        16
check-ignore                          25         9        16
cat-file                              21         9        12
unknown                               16         0        16
repack                                14        13         1
hash-object                           13         8         5
pull                                  12         3         9
write-tree                            12        12         0
prune-packed                           9         9         0
reflog                                 8         7         1
mv                                     6         6         0
_run_dashed_                           5         4         1
pack-refs                              5         4         1
prune                                  5         4         1
rerere                                 5         4         1
_query_                                4         4         0
commit-tree                            4         4         0
diff-index                             4         4         0
rebase                                 4         0         4
diff-tree                              3         0         3
remote-ext                             2         0         2
update-ref                             2         2         0
version                                2         0         2
_run_shell_alias_                      1         0         1
merge-ours                             1         1         0
notes                                  1         0         1
remote-curl                            1         0         1
update-index                           1         1         0
TOTAL                              55766     44223     11543

FIXTURE counts git that carried the fixture marker, CODE all other git.
The split is exact for a serial package and approximate where a hub build overlaps other tests of the same package, because the marker is process-wide during a build; compare before and after on the total.
```

### Tier 3 (`tmux`)

```
Resources  —  Tags: tmux

PACKAGE                                       WALL       CPU   PEAK_MEM   PROCS  LEFTOVER  GIT_FIXTURE  GIT_CODE
----------------------------------------  --------  --------  ---------  ------  --------  -----------  --------
internal/loomcli                            70.42s    18.15s     250.3M   12911         0          866       503
internal/reedcli                            40.90s    33.31s     416.0M   17529         0          670        33
internal/reedengine                         33.60s    23.57s     242.3M   10136         0            0         0
cmd/lyx                                      3.40s     5.16s     190.0M    4471         0            0         0
internal/pairteardown                        3.09s     3.62s     125.0M    1481         0          279       171
internal/websterengine                       1.83s     6.76s     206.2M     390         0            0         3
internal/orchcli                             1.39s     1.20s     109.4M     401         0            0         0
internal/webstercli                          1.15s     2.35s     172.5M     226         0            0         0
cmd/testtiming                               1.06s     1.69s     126.9M    2954         0            0         0
internal/fabricengine                        1.05s     3.90s     222.6M     145         0            0         3
internal/loomshed                            1.04s     2.35s     157.2M     455         0            0         0
internal/lyxcwd                              1.02s     1.52s     183.1M     262         0            0         0
internal/battencli                           0.90s     1.82s     133.3M    1558         0            0         0
internal/planglyph                           0.89s     2.75s     154.2M     166         0            0         0
internal/shedcli                             0.74s     1.36s     177.1M     130         0            0         0
internal/shuttleengine                       0.72s     0.67s      93.7M      86         0            0         0
internal/shuttleengine/claudeengine          0.71s     1.96s     139.5M     148         0            0         0
internal/burlerengine                        0.70s     1.84s     125.0M     192         0            0         0
internal/boardengine                         0.68s     1.87s     120.2M     648         0            0         0
contracts/stencils                           0.68s     0.99s     116.8M    1600         0            0         0
internal/orchengine                          0.67s     1.01s      94.1M     155         0            0         0
internal/shedadapters                        0.67s     1.01s     105.0M     154         0            0         0
internal/testkit/tmuxkit                     0.65s     0.76s      90.4M     159         0            0         0
internal/seatengine                          0.65s     1.57s     111.2M     143         0            0         0
internal/standalonegeom                      0.63s     1.01s     103.8M     239         0            0         1
internal/treadleengine                       0.63s     1.47s     113.0M     125         0            0         0
tools/tokencount                             0.63s     1.48s     124.8M     185         0            0         0
internal/loomrecipe                          0.62s     1.02s     118.6M      83         0            0         0
internal/boardcli                            0.59s     1.18s     114.8M     593         0            0         0
internal/burlercli                           0.57s     1.04s     104.5M     127         0            0         0
internal/shedrecipe                          0.56s     0.94s     114.1M      83         0            0         0
internal/gitkit                              0.54s     1.10s     182.9M     118         0            0         0
internal/landingshed                         0.53s     0.94s     112.5M     116         0            0         0
internal/battenrecipe                        0.52s     0.87s     113.7M     803         0            0         0
internal/cliwire                             0.51s     0.93s      93.9M     112         0            0         0
internal/quarrycli                           0.47s     0.77s     120.4M      95         0            0         0
internal/shedbuild                           0.47s     0.83s     108.2M      97         0            0         0
internal/configcli                           0.46s     0.89s     111.9M     113         0            0         0
internal/loomengine                          0.45s     0.88s     100.1M      84         0            0         0
internal/shuttlecli                          0.45s     0.93s     103.5M     114         0            0         0
internal/configreg                           0.44s     0.75s     111.1M      84         0            0         0
internal/fabriccli                           0.44s     0.84s      99.6M     105         0            0         0
internal/mergeresolve                        0.44s     0.74s     104.7M     196         0            0         0
internal/hubreconcile                        0.44s     0.88s      94.0M     248         0            0         0
internal/preflightshed                       0.42s     0.83s     100.9M     111         0            0         0
internal/hubforge                            0.42s     0.85s      98.1M     102         0            0         0
internal/battenshed                          0.41s     0.64s      91.4M     668         0            0         0
internal/gatecli                             0.41s     0.83s     106.4M     104         0            0         0
internal/ideengine                           0.41s     0.74s      88.4M     364         0            0         0
internal/idecli                              0.40s     0.72s      80.2M     254         0            0         0
internal/parentreview                        0.39s     0.68s      94.0M      71         0            0         0
internal/frictionengine                      0.39s     0.74s      94.5M      80         0            0         0
internal/boardengine/boardtest               0.38s     0.62s      85.8M     177         0            0         0
internal/stencilcli                          0.38s     0.79s      83.4M     105         0            0         0
internal/hubgeom                             0.37s     0.70s      79.8M     109         0            0         0
internal/planparser                          0.37s     0.63s      83.6M     104         0            0         0
internal/testkit/envkit                      0.37s     0.66s      92.5M      90         0            0         0
internal/preflight                           0.37s     0.66s      96.5M     138         0            0         0
internal/testkit/shedfake                    0.36s     0.62s      73.1M      89         0            0         0
internal/shedverbs                           0.36s     0.64s      93.1M      66         0            0         0
internal/configsync                          0.34s     0.59s      95.6M      75         0            0         0
internal/shedtransient                       0.33s     0.61s      89.8M      65         0            0         0
internal/gitrepo                             0.33s     0.66s      84.6M     104         0            0         0
internal/testkit/shuttlefake                 0.31s     0.54s      75.1M      63         0            0         0
internal/selfreportengine                    0.31s     0.43s      68.2M     133         0            0         0
internal/selfreportcli                       0.30s     0.46s      78.7M      97         0            0         0
internal/statuscommit                        0.30s     0.54s      84.5M      89         0            0         0
internal/gateslot                            0.29s     0.52s      57.9M     106         0            0         0
internal/githubclient                        0.28s     0.46s      67.3M      71         0            0         0
internal/impactset                           0.26s     0.44s      78.5M     250         0            0         0
internal/verifytree                          0.26s     0.47s      61.8M     108         0            0         0
internal/logger                              0.24s     0.45s     131.7M     111         0            0         0
internal/batcher                             0.24s     0.35s      48.3M     638         0            0         0
internal/gitexec                             0.23s     0.41s      61.4M     102         0            0         0
internal/commentlint                         0.23s     0.42s      53.3M      98         0            0         0
internal/standalonestate                     0.22s     0.44s      55.0M     111         0            0         0
internal/verifyrun                           0.22s     0.41s      57.9M     101         0            0         0
internal/testkit/lyxbin                      0.22s     0.37s      65.8M     110         0            0         0
contracts/specs                              0.20s     0.22s      44.4M     415         0            0         0
internal/pattern                             0.19s     0.29s      54.7M      70         0            0         0
internal/parentdirective                     0.19s     0.30s      53.3M      63         0            0         0
internal/lock                                0.18s     0.19s      44.5M      56         0            0         0
internal/reedengine/render                   0.18s     0.28s      46.6M      86         0            0         0
internal/shedengine                          0.17s     0.20s      44.0M      66         0            0         0
internal/clihelp                             0.17s     0.20s      44.5M      68         0            0         0
internal/agentname                           0.16s     0.18s      40.5M     392         0            0         0
internal/gitrepo/internal/gitoracle          0.16s     0.23s      48.1M      68         0            0         0
internal/testkit/logcapture                  0.16s     0.21s      37.1M      64         0            0         0
internal/stencilstore                        0.15s     0.22s      49.3M      66         0            0         0
internal/shedrun                             0.15s     0.20s      43.8M      87         0            0         0
tools/sandbox                                0.15s     0.23s      45.5M      73         0            0         0
internal/friction                            0.15s     0.23s      45.2M      58         0            0         0
internal/proc                                0.15s     0.20s      40.1M      61         0            0         0
internal/testkit/plankit                     0.15s     0.20s      38.6M      54         0            0         0
tools/mdreflow                               0.14s     0.20s      39.7M      67         0            0         0
internal/editdirective                       0.14s     0.23s      46.8M      58         0            0         0
internal/testkit                             0.14s     0.22s      41.3M      76         0            0         0
internal/testkit/stencilkit                  0.14s     0.23s      50.6M      67         0            0         0
tools/deploy                                 0.14s     0.20s      44.7M      54         0            0         0
internal/buildinfo                           0.14s     0.19s      43.5M     197         0            0         0
internal/output                              0.14s     0.19s      42.8M      64         0            0         0
internal/shell                               0.14s     0.20s      42.6M      59         0            0         0
internal/segmentcolor                        0.14s     0.18s      37.8M      61         0            0         0
internal/stencil                             0.14s     0.22s      46.4M      64         0            0         0
internal/configengine                        0.14s     0.21s      48.1M      61         0            0         0
tools/wordswap                               0.14s     0.19s      42.4M      94         0            0         0
internal/testkit/scankit                     0.14s     0.21s      43.3M      55         0            0         0
internal/fsx                                 0.14s     0.19s      43.7M      60         0            0         0
internal/modelspec                           0.14s     0.21s      48.4M      69         0            0         0
internal/vscode                              0.14s     0.23s      46.1M      61         0            0         0
internal/burlermarker                        0.13s     0.20s      45.0M      58         0            0         0
tools/godocreflow                            0.13s     0.21s      43.7M      79         0            0         0
internal/testkit/llmkit                      0.13s     0.19s      43.0M      54         0            0         0
internal/envsource                           0.13s     0.19s      43.7M      58         0            0         0
internal/dotgit                              0.13s     0.19s      42.2M      62         0            0         0
internal/loggerconfig                        0.13s     0.21s      48.7M      63         0            0         0
internal/summaryparser                       0.13s     0.21s      40.6M      58         0            0         0
internal/state                               0.13s     0.20s      43.8M      90         0            0         0
internal/shedcheck                           0.13s     0.20s      41.1M      55         0            0         0
internal/yamlengine                          0.13s     0.19s      40.9M      62         0            0         0
internal/fslink                              0.13s     0.21s      42.5M      58         0            0         0
tools/codestats                              0.13s     0.19s      42.2M      55         0            0         0
tools/internal/devbin                        0.13s     0.20s      42.6M      53         0            0         0
internal/discussionparser                    0.13s     0.20s      44.0M      63         0            0         0
internal/testkit/locationkit                 0.12s     0.20s      41.4M      57         0            0         0
internal/buildvcs                            0.12s     0.18s      42.6M      60         0            0         0
internal/testkit/envelope                    0.12s     0.21s      41.4M      55         0            0         0
internal/weftname                            0.12s     0.18s      42.1M      63         0            0         0
internal/testkit/boardkit                    0.08s     0.28s      30.0M      37         0            0         0
contracts/recipes                            0.07s     0.00s       2.5M     223         0            0         0
internal/testkit/indexkit                    0.05s     0.01s       7.2M      30         0            0         0
internal/planindex                           0.04s     0.02s       9.0M      31         0            0         0
internal/lyxdirs                             0.02s     0.00s       4.7M      17         0            0         0
TOTAL                                      197.53s   171.34s              69639                   1815       714

CPU and PEAK_MEM: the package's systemd scope cgroup (cpu.stat usage_usec, memory.peak), last sample before the scope ended.
A row marked rusage fell back to rusage because its scope could not be read.
PROCS: system-wide processes created while the package ran, so it counts anything else running on the machine.
Load average (1 min): 5.29 at start, 3.06 at end. A report reads "quiet machine" as a precondition.

Git processes by subcommand

SUBCOMMAND                         TOTAL   FIXTURE      CODE
------------------------------  --------  --------  --------
worktree                             448       316       132
config                               199       175        24
diff                                 178        39       139
add                                  175       106        69
commit                               162       110        52
rev-list                             155       100        55
rev-parse                            134        64        70
push                                 124        97        27
receive-pack                         124        97        27
for-each-ref                         118        97        21
pack-objects                         112        97        15
unpack-objects                       112        97        15
upload-pack                           80        80         0
branch                                76        61        15
status                                67        29        38
ls-remote                             58        58         0
remote                                32        32         0
reset                                 32        32         0
ls-files                              30        29         1
checkout-index                        29        29         0
read-tree                             29        29         0
clone                                 23        22         1
init                                  10         9         1
gc                                     6         0         6
symbolic-ref                           6         6         0
checkout                               3         3         0
tag                                    3         0         3
unknown                                2         0         2
rm                                     1         1         0
show                                   1         0         1
TOTAL                               2529      1815       714

FIXTURE counts git that carried the fixture marker, CODE all other git.
The split is exact for a serial package and approximate where a hub build overlaps other tests of the same package, because the marker is process-wide during a build; compare before and after on the total.
```

## Comparison

Wall and CPU are the tier `TOTAL` rows; `rev-parse` is the census total, fixture and code summed.
The tier 1 after-census has no `rev-parse` row, so its figure is 0.

| Tier | Wall before | Wall after | CPU before | CPU after | `rev-parse` before | `rev-parse` after |
|---|---|---|---|---|---|---|
| 1 (untagged) | 58.70s | 53.07s | 125.43s | 91.88s | 25 | 0 |
| 2 (`integration`) | 128.63s | 120.72s | 330.30s | 248.08s | 26989 | 5972 |
| 3 (`tmux`) | 224.35s | 197.53s | 178.96s | 171.34s | 1664 | 134 |

The fixture split of the after-state `rev-parse` total is 0 fixture and 0 code in tier 1, 4583 fixture and 1389 code in tier 2, and 64 fixture and 70 code in tier 3; it sits beside the comparison and takes no part in it.
Every after run started at a higher load average than its before run, so a quiet-machine repeat of the commands under [Regenerate](#regenerate) is the fair wall-time comparison.
