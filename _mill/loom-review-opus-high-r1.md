# `loom` — crucible round 2 review (tag `opus-high-r1`)

> Independent, clean-room review + fix round against `loom`, per `_mill/loom-review-prompt.md`.
> Worktree `/home/knatte/Code/loomyard/wts/loom-crucible-hardening-round2`, branch `loom-crucible-hardening-round2`.
> This file is written incrementally during Job 1 and committed after each meaningful append.

## Executive summary

_(written last)_

## Did a full live pipeline run complete this round?

_(answered when the live driving concludes — see "What was tested")_

## Scope assessment

_(pending)_

## Findings

_(recorded provisionally as they are spotted; severity ordering finalized last)_

## Docs & operability findings

_(pending)_

## What was tested

### Environment check (first, per the prompt's "check for a genuine environment gap FIRST")

```
which lyx claude tmux git go
```
- `lyx` → `/home/knatte/.local/bin/lyx` (deployed dev binary present)
- `claude` → `/home/knatte/.local/bin/claude`, version `2.1.231 (Claude Code)`
- `tmux` → `/usr/bin/tmux`, version `3.6`
- `go` → `go1.26.0 linux/amd64`
- `ps aux | grep -i tmux` at session start: **zero** tmux processes — clean baseline.

No environment gap. Real substrate driving is available.

### Hermetic gates (baseline, before any edit)

```
go build ./...                       -> rc=0, 2.7s
go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/... \
       ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... \
       ./internal/shedrecipe/... ./internal/shedbuild/... ./internal/hubgeom/...
                                     -> rc=0, no diagnostics
go test -count=5 <same nine packages> ./cmd/lyx/...
                                     -> rc=0, 10 packages `ok`, zero FAIL, zero panic
```
Baseline is green.
