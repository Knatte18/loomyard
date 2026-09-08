# `loom` — independent review, ROUND 5 (`opus-medium-r5`) — SAFETY PASS

Reviewer tag: `opus-medium-r5` (Opus 5, medium effort).
Worktree: `/home/hanf/Code/loomyard/wts/crucible-loom-glyph-hardening` (confirmed with `git rev-parse --show-toplevel`).
Branch: `crucible-loom-glyph-hardening` (confirmed with `git branch --show-current`).
HEAD at review start: `3cf37eea8`.

Clean-room: this report's findings were formed WITHOUT reading any `_mill/loom-review-*` file
(no prior round's review, no fixer report, no `loom-review-HANDOFF.md`). The design docs,
`CONSTRAINTS.md`, `CLAUDE.md` and the code were read; prior-round material was consulted only
after the findings list below was complete and committed.

## Executive summary

_(filled in at the end of Job 1 — see "Verdict" below)_

## What was tested

Appended as each command/scenario returned.

### Hermetic gates (all green, at HEAD `3cf37eea8`, before any edit)

| Command | Result |
|---|---|
| `CGO_ENABLED=1 go build ./...` | clean, no output |
| `CGO_ENABLED=1 go vet` over the 17 packages the prompt names | exit 0, no diagnostics |
| `CGO_ENABLED=1 go test -count=5` over those 17 packages + `./cmd/lyx/...` | all `ok`, exit 0 |
| `CGO_ENABLED=1 go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/webstercli/... ./internal/burlercli/...` | all `ok`, exit 0 |

### Binary/PATH agreement

`which lyx` found NOTHING on this host at review start — no stale binary, no binary at all.
Deployed with `CGO_ENABLED=1 go run ./tools/deploy` → `Deployed lyx @ 3cf37eea8 (25541 KB) /home/hanf/go/bin/lyx`,
which is on `PATH`. (`lyx --version` is not a flag this CLI carries; the deploy tool's own
`@ <sha>` line is the agreement evidence.)

### Substrate-leak probe (this is where finding R5-1 came from)

Baseline `ps aux | grep -E 'tmux|claude'` after the hermetic gates showed two orphan
`tmux -L lyx-<hash8> new-session -d -s 001-<hash8> -c /tmp/TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate<N>/001 ... bash`
servers, whose `-c` cwd is a `t.TempDir()` that no longer exists.

Isolated reproduction:

```
pgrep -af "tmux -L lyx-" | wc -l                       # -> 3 (2 orphans + the probe's own shell)
CGO_ENABLED=1 go test -tags integration -count=1 \
  -run 'TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate' ./internal/webstercli/
# ok  github.com/Knatte18/loomyard/internal/webstercli  0.177s
pgrep -af "tmux -L lyx-"                               # -> a THIRD live server, socket lyx-dc9e259e
```

Every socket under `/tmp/tmux-1000/` was then classified live-vs-dead with `tmux -L <n> ls`:
27 sockets, of which exactly the three from this one test were LIVE. Every other lyx test that
boots a tmux server (`lyx-001-*`, `lyx-contract-*`, `lyx-TestDeadHeaderPane*`,
`lyx-TestRemoveStrand_*`) had torn its server down and left only a dead socket file behind.

`internal/burlercli`'s same-named integration test does NOT leak: `burlercli`'s `run` verb
refuses on the missing `--profile` flag *before* `c.reedUp()` is reached
(`internal/burlercli/run.go:120-127` runs the flag check ahead of everything), so no server boots.

## Findings

Severity-ranked. `CONFIRMED` = reproduced or proven by reading a definite code path;
`PLAUSIBLE` = reasoned but not driven.
