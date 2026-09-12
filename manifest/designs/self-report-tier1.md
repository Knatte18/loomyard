# self-report Tier 1 — Go-detected structural anomalies

> **Status: Done.** See each package's own documentation for as-built detail — `internal/loomengine`'s `anomaly.go`/`anomalybody.go` and `internal/loomcli`'s `selfreport.go`.

## What shipped

`lyx loom drive` now detects and files five Tier-1 structural anomalies directly off loom's own status file and its Bouncer ledgers, with no LLM call and no session watching: a crash-resume (a driver that died mid-run, observed at the next drive's entry), an escalation-to-human halt (a blocked run with no `OnStuck` target), a bounce-budget-exhausted halt, a producer-hard-failure halt (a failed run), and a recurring finding (a ledger entry still open after three or more rounds — one less than a segment's own bounce budget, since the Bouncer's seed call permanently consumes one unit of it).

Each trigger renders a deterministic title discriminator so a re-observed condition never mints a second issue: the three halt kinds key on the current producer and its own count of prior successful (`done`) history entries, the crash-resume keys on the producer and the history length at the moment it was observed, and the recurring finding keys on the Bouncer row name plus the ledger entry's own key (row name included because a ledger key is scoped to its own segment's run directory, with nothing making it unique across the discussion, plan, and webster segments).

Detection runs as a four-step filing pass after every `shed.Run` call: collapse the detected anomalies to one per distinct title (keeping the fullest rounds list when a recurring finding is carried forward across several ledger files), filter against a machine-local filed-title marker, file one GitHub issue per surviving anomaly via the shipped `selfreport` primitive (`selfreportengine.CreateIssue`, `DefaultLabels()`), and record each title in the marker immediately after its own filing call succeeds. Losing the marker — a fresh clone, a fabric re-wire — costs at most one duplicate issue, which is why it is deliberately machine-local rather than durable.

The `selfreport` key in `loom.yaml` (default `true`) gates the whole step, on by default; a run in CI or against a fork must set it `false`, exactly as its own template comment says, or it will file into the upstream `Knatte18/loomyard` issue tracker. Every failure in the detect-and-file path — a filing call, a marker read, a marker write — degrades to a warning and never changes `drive`'s outcome, its exit code, or either envelope: this is a diagnostics side-channel on a long autonomous run, and a GitHub outage or an unresolvable token must never turn a completed loom run into a reported failure.

## Related

- [loom.md](loom.md) — the phase machine and status-file contract this reads from.
- [self-report-tier2.md](self-report-tier2.md) — the sibling tier for friction an agent notices within its own scoped task, which Go cannot detect structurally.
