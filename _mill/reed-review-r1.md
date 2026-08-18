# reed — independent review, round 1 (opus-medium-r1)

Round tag: `opus-medium-r1`.
Subject: `internal/reedengine`, `internal/reedcli`, `internal/hubgeom`, and reed's `cmd/lyx` integration, at branch `reed-shuttle-crucible-hardening` (post wave-1 commit `b98ee2ba`).
Merge bar for this round: correctness in the NORMAL single-instance flow.
An N×-concurrent stress suite is a diagnostic amplifier, not a merge blocker on its own.

## What was tested

Log appended as each command/scenario returned.

### Hermetic gates (baseline, before any edit)

| Command | Result |
|---|---|
| `go build ./...` | clean |
| `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...` | clean |
| `go test -count=1 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...` | all ok |

### Static / import-graph checks

| Check | Command | Observation |
|---|---|---|
| hubgeom one-way told direction | `go list -deps ./internal/reedengine \| grep hubgeom` | no match — `reedengine` does not reach `hubgeom`. Also structurally impossible: `hubgeom` imports `reedengine`, and Go forbids import cycles, so the direction is self-enforcing rather than merely unviolated today. |
| reedengine direct imports of `lyxcwd`/`fabricengine` | `grep -rn 'lyxcwd\|fabricengine' internal/reedengine/*.go` (non-test) | only comment/doc mentions; no production import. Note: `internal/lyxcwd` IS still in `go list -deps ./internal/reedengine`, reached transitively through `internal/logger`. |
| `hubgeom` importers | `grep -rn internal/hubgeom --include=*.go .` | `reedcli`, `shuttlecli`, `burlercli`, `perchcli`, `webstercli` + smoke tests. No engine imports it. |

## Findings

Severity legend: BLOCKING / MEDIUM / LOW / NIT. Status: CONFIRMED (reproduced or traced end to end) vs PLAUSIBLE.

(populated below as the review proceeds)
</content>
</invoke>
