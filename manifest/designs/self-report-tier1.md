# self-report Tier 1 — Go-detected structural anomalies

> **Status: Done.** See each package's own documentation for as-built detail — `internal/loomengine`'s `anomaly.go`/`anomalybody.go` and `internal/loomcli`'s `selfreport.go`.

## What shipped

`lyx loom drive` now detects and files five Tier-1 structural anomalies directly off loom's own status file and its Bouncer ledgers, with no LLM call and no session watching: a crash-resume (a driver that died mid-run, observed at the next drive's entry), an escalation-to-human halt (a blocked run with no `OnStuck` target), a bounce-budget-exhausted halt, a producer-hard-failure halt (a failed run), and a recurring finding (a ledger entry still open after three or more rounds — one less than a segment's own bounce budget, since the Bouncer's seed call permanently consumes one unit of it).

Each trigger renders a deterministic title discriminator so a re-observed condition never mints a second issue: the three halt kinds key on the current producer and its own count of prior successful (`done`) history entries, the crash-resume keys on the producer and the history length at the moment it was observed, and the recurring finding keys on the Bouncer row name plus the ledger entry's own key (row name included because a ledger key is scoped to its own segment's run directory, with nothing making it unique across the discussion, plan, and webster segments).

Detection runs as a four-step filing pass after every `shed.Run` call: collapse the detected anomalies to one per distinct title (keeping the fullest rounds list when a recurring finding is carried forward across several ledger files), filter against a machine-local filed-title marker, file one GitHub issue per surviving anomaly via the shipped `selfreport` primitive (`selfreportengine.CreateIssue`, `DefaultLabels()`), and record each title in the marker immediately after its own filing call succeeds. Losing the marker — a fresh clone, a fabric re-wire — costs at most one duplicate issue, which is why it is deliberately machine-local rather than durable.

The crash-resume trigger carries one exclusion beyond its own signature: a completed `lyx loom step` leaves the status file byte-identical to a mid-run driver death (state running, run lock free, history non-empty), so `step` records a machine-local clean-handoff marker (`.lyx/loom/step-handoff.json`: the persisted history length and state) after every completed step, and the next `drive`'s entry observation consumes that marker — one-shot, deleted on read — and suppresses the crash-resume when it matches.
An operator handing a supervised task to a driver therefore files nothing, while a step killed mid-producer (which never writes the marker) and a driver that died after appending history (which outgrows it) both still report.
Losing the marker costs at most one spurious issue, the same machine-local trade the filed-title marker makes.

**Tier 1 fires on the `drive` path only, and `lyx loom step` is deliberately exempt.** The detection step hangs off `shed.Run`, and `step` calls `shed.Step`, so a task driven entirely through the step-loop supervisor never auto-files an anomaly — including a crash-resume, which on that path is a condition the supervisor is itself watching for (see [loom-step.md](loom-step.md) and the `ly-supervise` skill's interrupted-invocation branch). This is the same scoping argument [self-report-tier2.md](self-report-tier2.md) makes for its own aggregation pass: when a live session is supervising, it notices and files with an operator in the loop, rather than a primitive the supervisor calls up to forty times per run minting public issues on its own.
It is stated here because it is not inferable: a reader of the sentence above can reasonably take "`lyx loom drive`" as naming the implementation site rather than excluding a sibling verb. Since `lyx loom run` spawns `drive` as its detached driver, the exemption is narrower than it sounds — every unsupervised run is covered.

The `selfreport` key in `loom.yaml` (default `true`) gates the whole step, on by default; a run in CI or against a fork must set it `false`, exactly as its own template comment says, or it will file into the upstream `Knatte18/loomyard` issue tracker. Every failure in the detect-and-file path — a filing call, a marker read, a marker write — degrades to a warning and never changes `drive`'s outcome, its exit code, or either envelope: this is a diagnostics side-channel on a long autonomous run, and a GitHub outage or an unresolvable token must never turn a completed loom run into a reported failure.

## Related

- [loom.md](loom.md) — the phase machine and status-file contract this reads from.
- [self-report-tier2.md](self-report-tier2.md) — the sibling tier for friction an agent notices within its own scoped task, which Go cannot detect structurally.
