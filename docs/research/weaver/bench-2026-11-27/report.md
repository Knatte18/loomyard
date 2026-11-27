# Weaver against Webster on landed plans

2026-11-27.
The figures record runs on one machine and are not claiming to stay true.

## Plans

Each plan was implemented by Webster and landed.
A run is clean when its Webster row has one start and one end, with no machine sleep, restarted Master or recovery strand inside it.

| Plan | Base commit | Landed commit | Cards, Webster batches | Recorded Webster row | Main packages |
|---|---|---|---|---|---|
| `board-viewer-server` | `6d3a916f4` | `7bdb031ed` | 9 in 4+2+3 | 2026-11-19, 63m00s | `boardengine`, `boardserve`, `boardcli`, `vscode` |
| `launch-manifest-parity` | `40c66912c` | `3bbc92a0e` | 9 in 3+3+3 | 2026-11-17, 52m29s | `shuttleparity`, `shuttleengine`, `shuttlecli`, `heddleengine` |
| `gate-test-map-mode` | `5469ff91d` | `b92c95ee5` | 5 in 1+2+2 | 2026-11-17, 53m36s | `gatecli`, `testcover` |

Why these three:

- Each run is clean.
- Their main packages differ.
- Each base commit is an ancestor of `main`.
- `board-viewer-server` has the newest clean recorded run, so it is the calibration plan.

Newer runs left out:

- `dobby-run-watch-build`: the machine hibernated for over eight hours inside the Webster row.
- `async-run-pusher`: two Master sessions, and the row's history holds only the later start.
- `single-repo-namespace-layout`: the row stopped once, and one batch took two recoveries.
- `review-result-checks`: the plan-level verify failed once and the row did rework.
- `slack-machine-bridge-build`: one batch took a recovery.
- `adopt-foreground`: its base commit is no longer in the repository.
