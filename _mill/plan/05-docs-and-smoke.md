# Batch: docs-and-smoke

```yaml
task: "Fork-based cluster review in burler"
batch: "docs-and-smoke"
number: 5
cards: 3
verify: go test ./...
depends-on: [4]
```

## Batch Scope

Closes the task: the as-built documentation the CLAUDE.md task-completion rule demands
(cards 15–16) and the opt-in real-engine cluster smoke that proves the mechanism live —
including the one assumption the spike never verified, that PreToolUse hooks fire inside
fork subagents (card 17). Batch-local decision: the full-repo `go test ./...` verify is
justified here as the task's terminal gate — earlier batches ran scoped; this catches
cross-package regressions (pinned help trees, enforcement guards, tier-purity walkers)
before handoff.

## Cards

### Card 15: burlerengine package doc rewritten as-built

- **Context:**
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/cluster.go`
  - `internal/burlerengine/config.go`
  - `docs/research/session-fork-spike.md`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/burlerengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the "Cluster fan-out (not yet)" section with an as-built
  "Cluster fan-out (fork subagents)" section covering: the fan model (`cluster-fan`
  names a fan from seed-only burler.yaml; fan length = fork count, cap `maxClusterN`;
  never default); the three-phase round shape (explore → spawn N unnamed lens forks in
  one message + handler holistic review → consolidate with origin labels and a
  Rejected section, all inside job A — A-before-B intact); the fork discipline
  (read-only, no git, no output files, no nested Agent) and its two enforcement layers
  (session hook + `auditClusterRound` over `AuditForks` facts, exact-N or
  `ErrClusterForksMissing`); the run mechanics (forks run in the handler's session and
  model — no model-per-fork; `CLAUDE_CODE_FORK_SUBAGENT=1` rides the launch line
  because the mux server env is scrubbed); version pinning (staged-rollout flag,
  Claude Code v2.1.117+; forks must stay unnamed — named forks silently lose context
  ≤2.1.206); and timeout guidance (no auto-scaling; forks queue under the CC
  concurrency cap min(16, cores−2) on low-core hosts — set a longer per-run timeout
  for wide fans; serialization never breaks exact-N, it surfaces as timeout). Also
  update the Profile/RunOpts paragraph's field list (`ClusterFan` replaces
  `ClusterN`). Mention that cluster rounds weaken nothing about weft-blindness: forks
  write nothing at all.
- **Commit:** `burler: rewrite package doc cluster section as-built`

### Card 16: overview, roadmap, CONSTRAINTS, sandbox suite

- **Context:**
  - `internal/burlerengine/doc.go`
  - `_mill/discussion.md`
- **Edits:**
  - `docs/overview.md`
  - `docs/roadmap.md`
  - `CONSTRAINTS.md`
  - `tools/sandbox/SANDBOX-BURLER-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `docs/overview.md`: the burler module entry (its line ~280) gains
  one sentence on fork-subagent cluster review (`cluster-fan`, fan config in seed-only
  burler.yaml); the execution-stack row (line ~317) already says "(+cluster)" — leave
  unless inaccurate. `docs/roadmap.md`: milestone 11's burler half notes cluster
  fan-out is now ✅ Done via fork subagents (link the burlerengine package doc);
  milestone 24 (own-window strand anchoring) is rewritten to drop "unlocks burler's
  cluster-N" — it no longer gates cluster review (fork subagents run in the handler's
  pane); keep it as an independent mux enhancement, noting live per-reviewer pane
  visibility as its remaining cluster-adjacent value; under "Deferred burler
  enhancements" mark the cluster entry done-by-fork-subagents (pointing at the
  package doc) and update the "generic tools-restriction" entry's premise (cluster
  reviewers are forks inside the handler session, not separate sessions — the entry's
  milestone-24 gating is stale). `CONSTRAINTS.md`: extend the Review Round Invariant
  statement with one sentence: in a cluster round the fork reports, holistic review,
  and consolidation are all part of A — the consolidated review is fully on disk
  before B touches a file — and fork reviewers are read-only (no writes, no git,
  enforced by the fork audit); keep the enforcement pointer accurate
  (`template_test.go` now also pins the cluster statements, per batch 4 card 9).
  `SANDBOX-BURLER-SUITE.md`: the S1 profile line (its line ~142) drops `cluster-n: 0`
  for `cluster-fan` omitted/empty; the invalid-profile scenario (line ~190) replaces
  the `cluster-n: 1` unsupported-cluster expectation with `cluster-fan: no-such-fan`
  expecting a validate error naming the unknown fan; keep the `**Covers:** burler`
  tagging intact.
- **Commit:** `docs: record fork-based cluster review across overview, roadmap, constraints, sandbox suite`

### Card 17: cluster smoke tests (opt-in, real engine)

- **Context:**
  - `internal/burlerengine/smoke_round_test.go`
  - `internal/burlerengine/testmain_test.go`
  - `internal/burlerengine/cluster.go`
  - `internal/burlerengine/config.go`
  - `internal/shuttleengine/claudeengine/settings.go`
- **Edits:** none
- **Creates:**
  - `internal/burlerengine/smoke_cluster_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Mirror `smoke_round_test.go`'s build tag, opt-in env gating,
  fixture/hub setup, and engine wiring. Construct the burler `Config` directly in Go
  (no burler.yaml seeding needed): `Lenses` with a `generic` text plus one
  deliberately misbehaving lens whose text instructs the fork to attempt spawning a
  subagent via the Agent tool; `Fans` with `clean: [generic, generic]` and
  `rogue: [generic, <misbehaving>]`. Two tests, generous explicit `RunOpts.Timeout`
  (forks serialize under the CC concurrency cap on low-core hosts — a slow runner must
  surface as timeout, never be misread as shortfall): (1) `clean` fan round reaches
  `OutcomeDone`, `Result.ForkAudit` carries exactly 2 forks with zero
  `AgentCalls`/`WriteCalls`, and the review file parses with at least one
  `origin:`-labelled finding or a well-formed consolidated structure; (2) `rogue` fan
  round fails `Engine.Run` with the fork-violation hard error (assert the error
  message names the Agent-call violation), AND the rogue fork's transcript under the
  session's `subagents/` directory contains the `steerAgentNonForkDeny` steer text —
  the empirical proof that PreToolUse hooks fire INSIDE fork subagents, the one spike
  assumption this task must verify. State in the test's doc comment that assertion
  (2b) is the load-bearing one: if a Claude Code update stops firing hooks in forks,
  this is the test that says so.
- **Commit:** `burler: add opt-in cluster smoke tests (clean fan + rogue-fork violation)`

## Batch Tests

Frontmatter `verify: go test ./...` — the terminal repo-wide gate (justification in
Batch Scope: earlier batches were package-scoped; this catches pinned-set and
enforcement-guard regressions anywhere). The smoke file itself is tag-gated and does
not run under the verify command; it is executed on demand like the existing burler
smoke (documented opt-in env), so the verify stays fast and offline.
