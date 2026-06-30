# Discussion: Fix lyx CLI defects + host-commit gap from the sandbox run

```yaml
task: Fix lyx CLI defects + host-commit gap from the sandbox run
slug: lyx-sandbox-fixes
status: discussing
parent: main
```

## Problem

The 2026-06-30 sandbox run (a Claude agent driving `lyx.exe` black-box against the
`Knatte18/lyx-test` hub per `tools/sandbox/SANDBOX-SUITE.md`) filed four GitHub issues:
#35 (subdirectory invocation), #36 (terse/unguided errors, three sub-points), #37
(inconsistent JSON path separators between `warp list` and `warp pairs`), and #38 (no
lyx-owned host-commit flow, filed as an enhancement). #35 was fully retracted by the
filing agent in its own follow-up comments — its premise (lyx should walk up from a
subdirectory like git) was a misunderstanding of lyx's by-design single-root binding —
and its only valid residual is already covered by #36.

Why now: #36 points 1 and 2 (no `--help` hint on unknown command/flag; JSON-shaped
errors even outside `--json`) react to a 2-day-old, deliberate, tested design decision
(`7817b67`, "CLI help & error ergonomics", 2026-06-28) that made every CLI error
JSON-shaped on stdout regardless of `--json`, specifically to eliminate a prior
inconsistency (domain errors were JSON, cobra-level errors were plain text). The filer
was a Claude sandbox-testing agent, not a human reading a terminal — the human-ergonomics
case for plain text doesn't hold the same way it would for a person typing commands by
hand, so this task declines those two sub-points rather than reversing the recent pass.
#36 point 3 (a raw git `fatal:` string leaking through unwrapped) and #37 (the separator
mismatch) are genuine implementation defects, independent of that design question, and
are fixed in code. #38 is declined: `docs/overview.md` already documents the host repo
as an ordinary, developer-maintained repo, with lyx-owned tooling reserved for the weft
side specifically because weft is the unusual "piggyback" repo.

## Scope

**In:**

- `internal/hubgeometry/hubgeometry.go`: stop embedding raw git stderr into the wrapped
  `ErrNotAGitRepo` message (root cause of #36 point 3).
- Strip the redundant message prefixes that four call sites add on top of
  `hubgeometry.Resolve()`'s already-self-describing error: `internal/idecli/cli.go:59`,
  `internal/initcli/initcli.go:84`, `internal/configcli/configcli.go:185`,
  `internal/muxpoccli/cli.go:100` (the last of which currently double-states
  "not a git repository" — same defect class, not a separate bug).
- `internal/warpengine/status.go` (`PairStatus`), `prune.go` (`PruneEntry`),
  `reconcile.go` (`ReconcilePairResult`): normalize `HostWorktree`/`WeftWorktree` to
  forward-slash via `filepath.ToSlash` only at the JSON-struct-literal boundary, matching
  `warp list`'s existing (already-correct) forward-slash output (#37).
- `tools/sandbox/SANDBOX-SUITE.md`: tighten scenario S2 (explicit — no host-commit
  tooling is intentional, not a gap to re-file) and S6 (explicit — the JSON error
  envelope is the deliberate machine contract; judge legibility by whether the message
  identifies the problem, not by prose/hint presence; a leaked raw subprocess line is
  still a legitimate finding).
- Closing #36 (partially), #37, and #38 on GitHub via `gh issue close` with a comment —
  a manual/finalize-time action, not implementation work, but should happen as part of
  wrapping up this task.

**Out:**

- No change to the JSON-always error envelope behavior. No `--json`-vs-plain-text
  branching, no `hint` field added to the envelope. #36 points 1 and 2 declined outright.
- No did-you-mean / fuzzy command-suggestion logic.
- No new host-commit CLI surface: no `lyx host` module, no `lyx warp commit`. #38
  declined outright.
- No further action on #35 beyond what #36 already covers — it is self-retracted.
- No broader `SANDBOX-SUITE.md` pass beyond scenarios S2 and S6.

## Decisions

### JSON-always errors stay as-is

- Decision: Decline #36 points 1 & 2 with no code change. The JSON error envelope
  remains unconditional, regardless of `--json`.
- Rationale: `CONSTRAINTS.md`'s "Errors are JSON" rule was introduced 2 days prior
  specifically to eliminate an inconsistency where domain errors were JSON but
  cobra-level errors were plain text — i.e. callers couldn't predict the error shape.
  The filer of #36 was a Claude sandbox-testing agent, which parses JSON natively, so
  the human-ergonomics argument for plain text doesn't apply to the actual consumer that
  hit this. Reopening the dual-shape contract would reintroduce the exact bug class the
  recent pass fixed, for a complaint that doesn't generalize to a real user.
- Rejected: Adding a `"hint"` field to the JSON envelope (low-risk, additive) — rejected
  because the underlying complaint doesn't apply to its actual filer. Full reversal to
  plain-text-unless-`--json` — rejected as a regression risk for no real benefit.

### Host-commit feature declined

- Decision: Close #38 as won't-fix / by-design. No `lyx host` module, no
  `lyx warp commit`.
- Rationale: The host repo is an ordinary git repo by design —
  `docs/overview.md` states the host is "the project's source of truth, maintained by
  developers," with lyx-owned tooling reserved for the weft side because weft is the
  unusual "piggyback" repo lyx actually controls. Building lyx-owned commit tooling for
  an intentionally-ordinary repo cuts against that split. Scenario S2 already carried
  language acknowledging raw-git host commits as acceptable before #38 was filed, but
  that didn't stop the (correctly WARN/enhancement-bucketed, but still unwanted)
  suggestion — S2 needs to be explicit that this is declined, not merely tolerated.
- Rejected: `lyx warp commit` (commit host + sync weft in one step, the issue's own
  suggestion) — rejected per the above. A new `lyx host` module mirroring weft's shape —
  rejected as disproportionate surface for a declined feature.

### Raw git stderr leak fixed at the source plus call sites

- Decision: `hubgeometry.go:96` (`Resolve()`, the `exitCode != 0` branch) changes from
  `fmt.Errorf("%w: %s", ErrNotAGitRepo, stderr)` to bare `ErrNotAGitRepo` — no appended
  text at all. The sentinel's own message ("not a git repository") is self-describing
  once the raw git stderr is dropped; per the earlier decision to decline adding any
  hint/guidance text to error messages, nothing is added back in its place.
  `hubgeometry.go:93` (the `err != nil` branch, a Go-level subprocess-spawn failure —
  e.g. the `git` binary missing — wrapped as `fmt.Errorf("%w: %v", ErrNotAGitRepo, err)`)
  is explicitly OUT of scope and unchanged: that `err` is Go's own exec-layer error, not
  git's stderr output, so it does not reproduce the "fatal: ..." leak #36 point 3
  describes, and the diagnostic content (why the subprocess itself couldn't launch) is
  useful, not noise. The four call sites that add their own redundant prefix on top of
  `hubgeometry.Resolve()`'s error — `idecli/cli.go:59` (`"failed to resolve layout: %v"`),
  `initcli/initcli.go:84` (same), `configcli.go:185` (`"resolve layout: %v"`),
  `muxpoccli/cli.go:100` (`"not a git repository: %v"`) — drop that prefix and pass the
  (now-clean) error straight through, matching the existing no-prefix call sites in
  `warpcli/warp.go` and `weftcli/cli.go`.
- Rationale: The #36 point 3 repro
  (`"not a git repository: fatal: not a git repository (or any of the parent
  directories): .git"`) comes directly from
  `fmt.Errorf("%w: %s", ErrNotAGitRepo, stderr)` embedding git's raw fatal text.
  `muxpoccli` compounds this into a triple-statement. Fixing only the literal repro path
  (`warp pairs`, which already calls bare `err.Error()`) would leave the identical defect
  live at four other call sites.
- Rejected: Fixing only `hubgeometry.go` and leaving the four call-site prefixes in
  place — rejected because `muxpoccli`'s double-statement is the same bug, not a
  separate one, and leaving it would mean the fix is incomplete by inspection.

### Path-separator normalization at the JSON boundary

- Decision: In `warpengine/status.go` (`PairStatus`), `prune.go` (`PruneEntry`), and
  `reconcile.go` (`ReconcilePairResult`), apply `filepath.ToSlash` to
  `HostWorktree`/`WeftWorktree` only when building the JSON-tagged struct fields. The
  OS-native `hostPath`/`weftPath` locals (built via `filepath.FromSlash` +
  `filepath.Clean`) are left untouched everywhere they're used for `os.Stat`, git
  subprocess calls, junction-health checks, etc.
- Rationale: `warp list` already emits forward-slash paths because it passes git's raw
  `worktree list --porcelain` output (always forward-slash, regardless of OS) straight
  through. `status.go`, `prune.go`, and `reconcile.go` each independently call
  `filepath.FromSlash` on that same `entry.Path`, producing OS-native (backslash on
  Windows) output in their JSON fields instead — that's the entire #37 mismatch. The
  OS-native form is still required internally for real filesystem/git operations on
  Windows, so the fix must apply only at serialization, not internally.
- Rejected: Converting `hostPath`/`weftPath` to forward-slash everywhere including
  internal use — rejected because Windows filesystem/git calls require OS-native
  separators; converting internally risks breaking `os.Stat`/git-subprocess calls.

### SANDBOX-SUITE.md hardened against re-filing declined findings

- Decision: Tighten scenario S2 to state explicitly that the absence of host-commit
  tooling is intentional and must not be re-filed as an enhancement. Tighten scenario S6
  to state explicitly that the JSON-shaped error envelope is the deliberate
  machine-parseable contract — legibility means the message identifies the problem, not
  that it reads as human prose with a hint — while clarifying that leaked raw subprocess
  output (e.g. a bare git `fatal:` line) remains a legitimate finding.
- Rationale: S2's existing host/raw-git language predates #38 and didn't stop the
  enhancement filing. S6's current wording ("Does lyx say what to do, or just fail?")
  invited exactly the #36 points 1/2 framing. Both need to be explicit enough that a
  future sandbox run doesn't regenerate either finding, while still catching genuine
  regressions (e.g. if the raw-git-leak fix in this task were ever reverted).
- Rejected: A broader `SANDBOX-SUITE.md` pass — explicitly scoped to S2 + S6 only per
  direction; no other scenario touched.

### #35 requires no action

- Decision: No code or doc change beyond what #36 already covers.
- Rationale: #35 was filed, then fully retracted by the filing agent across three
  follow-up comments; its premise was a misunderstanding of lyx's by-design
  single-root binding, not a defect. The only residual point it raised (terse errors,
  raw git fatal leak) is already covered by #36 in this task.
- Rejected: N/A — no live work item here.

## Technical context

- `internal/output/output.go`: `Ok`/`Err` JSON envelope helpers. `Err` already does
  `strings.TrimSpace(msg)` before marshalling — unaffected by this task; the hubgeometry
  fix changes what string is passed in, not this helper's behavior.
- `internal/clihelp/exec.go`: `RunRoot`/`Execute` wrap all cobra-level errors in the
  JSON envelope at a single seam; `GroupRunE` rejects unknown subcommands. No changes
  needed — confirms JSON-always is enforced centrally, consistent with declining #36
  points 1 & 2.
- `internal/hubgeometry/hubgeometry.go`: owns `ErrNotAGitRepo` and `Resolve()`. Lines
  89–97 are the fix site for #36 point 3. Per `CONSTRAINTS.md`'s Hub Geometry Invariant,
  `Resolve()` is the only legal way to obtain a `*Layout` — the fix must stay inside this
  function, not be worked around at call sites.
- Call sites needing prefix-stripping: `internal/idecli/cli.go:59`,
  `internal/initcli/initcli.go:84`, `internal/configcli/configcli.go:185` (note:
  `configcli.go` has a second `Resolve()` call at line ~284–286 that already does bare
  `err.Error()` — only line 185 needs fixing), `internal/muxpoccli/cli.go:100`.
- `internal/warpengine/status.go`, `prune.go`, `reconcile.go`: each independently loops
  `hubgeometry.List(...)` entries and does
  `hostPath := filepath.FromSlash(entry.Path); hostPath = filepath.Clean(hostPath)`
  before building a JSON-tagged result struct. `cleanup.go` has the same `FromSlash`
  pattern but `CleanupBranchEntry` carries no path field, so it's unaffected and out of
  scope. `list.go` (the `warp list` command) never calls `FromSlash` — it is already the
  forward-slash reference behavior the other three should match.
- `hubgeometry.List()` is the shared source of `entry.Path` for all of the above; it
  wraps `git worktree list --porcelain`, which git always emits with forward slashes
  regardless of OS.
- `tools/sandbox/SANDBOX-SUITE.md`: scenario S2 (~lines 123–133) and S6 (~lines
  165–175) are the two blocks to edit.

## Constraints

- **Hub Geometry Invariant**: any change inside `internal/hubgeometry` must keep
  `Resolve()` as the sole path-resolution entry point; raw `os.Getwd`/
  `git rev-parse --show-toplevel` stay banned outside hubgeometry and `cmd/lyx/main.go`
  (enforced by `enforcement_test.go`). The geometry-literal ban doesn't apply here — this
  task touches error-message strings and path-serialization, not geometry-token
  construction.
- **CLI / Cobra Invariant — "Errors are JSON"**: explicitly preserved, not touched, by
  the decision to decline #36 points 1 & 2. All message-text changes must keep flowing
  through `output.Err` / the existing JSON envelope — no new plain-text path is
  introduced anywhere.
- **CLI / Cobra Invariant — help-prose review obligation**: this task changes observable
  error-message text (not flags/defaults/behavior), so the reviewer should spot-check
  that no `Short`/`Long` text quotes the old leaky error format verbatim. None found
  during this discussion's exploration, but mill-plan/reviewer should re-verify against
  the diff as written.
- **lyxtest Leaf Invariant**: not implicated — no test changes here touch
  `internal/lyxtest`.

## Testing

- `internal/hubgeometry/hubgeometry_test.go`: existing tests asserting
  `errors.Is(err, hubgeometry.ErrNotAGitRepo)` must keep passing (the `%w` sentinel wrap
  is preserved — only the appended detail text changes). TDD candidate: write an
  assertion that the message no longer contains raw git stderr substrings (e.g.
  `"fatal:"`), confirm it fails against the current `%s`-interpolated stderr, then apply
  the fix.
- `internal/muxpoccli`, `internal/idecli`, `internal/initcli`, `internal/configcli`:
  each package's existing not-a-git-repo / resolve-failure tests likely assert on
  substrings of the old (prefixed) message — these need deliberate updating to the new
  single-statement message, not just left passing incidentally. Grep each package's
  `_test.go` for `"not a git repository"` / `"failed to resolve layout"` /
  `"resolve layout"` substring assertions before editing the production code.
- `internal/warpengine`'s status/prune/reconcile tests (confirm exact test file names at
  plan time): add or update assertions that `HostWorktree`/`WeftWorktree` JSON fields
  contain no backslash. Note for mill-plan: a plain `go test ./...` on a Unix CI runner
  would not catch the original separator bug (Unix already produces forward-slash
  naturally) — the assertion needs to check for the absence of `\` explicitly, not rely
  on OS-dependent behavior, so it's meaningful on both platforms. Critically, the
  existing tests in this area compare `filepath.Clean(jsonField) == filepath.Clean(expected)`
  (or similar `Clean`-wrapped comparisons) — on Windows, `filepath.Clean` re-normalizes
  forward slashes back to backslash, so a `Clean`-wrapped comparison would pass both
  before and after the fix and never actually exercise the separator guarantee. The new
  (or updated) assertions must check the raw JSON field string directly — e.g.
  `!strings.Contains(jsonField, "\\")` — never through a `filepath.Clean`/`filepath.FromSlash`
  wrapper that would silently re-introduce the platform-native separator before the
  comparison runs.
- No test changes needed for `internal/output` or `internal/clihelp` — JSON-always
  behavior is unchanged.
- `tools/sandbox/SANDBOX-SUITE.md` changes are documentation-only; the suite itself is
  agent-driven, not scripted (per the file's own "What this is" section), so no
  automated test applies.

## Q&A log

- **Q:** How should #36's conflict with `CONSTRAINTS.md`'s JSON-always-errors invariant
  be resolved? **A:** Keep JSON-always exactly as-is; decline #36 points 1 & 2 entirely,
  no code change (fix only point 3). The filer was a Claude agent, not a human at a
  terminal, so the human-ergonomics case for plain text doesn't hold; reopening the
  dual-shape contract would regress the 2-day-old ergonomics pass.
- **Q:** Should lyx add did-you-mean suggestions for unknown commands? **A:** No — out
  of scope, not requested by the issue.
- **Q:** Should this task build a host-commit feature (#38)? **A:** No — declined as
  by-design; host is an ordinary repo, weft is the special piggyback repo lyx owns
  tooling for.
- **Q:** Should a `hint` field still be added to the JSON error envelope as a smaller
  compromise? **A:** No — decline outright, no code change for #36 points 1 & 2.
- **Q:** What's the `SANDBOX-SUITE.md` update scope? **A:** S2 + S6 only, tightened to
  explicitly preempt re-filing of the declined findings.
- **Q:** Scope of the #36 point 3 fix — just `hubgeometry.go`, or also the four call
  sites with redundant prefixes? **A:** Both — `hubgeometry.go` plus stripping the four
  redundant call-site prefixes (`idecli`, `initcli`, `configcli:185`, `muxpoccli`).
- **Q:** Mechanics of the #37 fix — normalize everywhere or just at the JSON boundary?
  **A:** JSON boundary only (`filepath.ToSlash` on `HostWorktree`/`WeftWorktree` when
  building the JSON struct fields); OS-native locals stay untouched for filesystem/git
  operations.
