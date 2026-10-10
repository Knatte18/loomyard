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

The last card fills this section.
