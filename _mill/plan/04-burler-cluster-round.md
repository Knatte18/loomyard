# Batch: burler-cluster-round

```yaml
task: "Fork-based cluster review in burler"
batch: "burler-cluster-round"
number: 4
cards: 7
verify: go test ./internal/burlerengine/ ./internal/burlercli/ ./internal/perchengine/ ./internal/perchcli/
depends-on: [2, 3]
```

## Batch Scope

The cluster round itself, plus every schema site that names the old gate — one batch
because the `ClusterN`→`ClusterFan` swap is one compile unit: `burlerengine.Profile`
is embedded by value in `perchengine.Profile` and decoded by both CLIs, so splitting
would leave the repo unbuildable between batches. Cards 8–12 rework burlerengine
(profile, prompt, verdict, audit policy, CLI); cards 13–14 carry the mechanical perch
passthrough. External interface for batch 5: a profile with `cluster-fan: <name>` runs
the three-phase cluster round end-to-end and fails loud on any fork violation.
Batch-local decision: fan resolution happens inside `Profile.validate` (which gains a
`Config` parameter) so both entry paths — burlercli profiles and perch-built round
profiles — pass through one resolver before any strand spawns.

## Cards

### Card 8: Profile.ClusterFan replaces ClusterN

- **Context:**
  - `internal/burlerengine/config.go`
  - `internal/burlerengine/doc.go`
- **Edits:**
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/profile_test.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/engine_test.go`
  - `internal/burlerengine/smoke_round_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `profile.go`: delete `ClusterN int` and `ErrClusterUnsupported`;
  add `ClusterFan string` (doc comment: names a fan from burler.yaml; naming a fan IS
  what activates clustering and the fan's entry count IS the fork fan-out; empty =
  single-reviewer round, the default — clustering is never on unless a profile says
  so). Add unexported `clusterLenses []Lens` populated by validate. `validate` gains
  the config parameter — new signature `validate(worktreeRoot string, cfg Config)` —
  and when `ClusterFan != ""` calls `ResolveFan(cfg, p.ClusterFan)`, storing the result
  in `p.clusterLenses` and propagating its fail-loud errors; the old `ClusterN`
  negative/unsupported checks are removed. In `engine.go`: `Engine` gains a `cfg
  Config` field; `New(shuttle Shuttle, layout *hubgeometry.Layout, cfg Config)
  *Engine`; `Run` passes `e.cfg` into `p.validate` and sets
  `spec.ForkSubagents = p.ClusterFan != ""` when building the shuttle Spec. Update
  `profile_test.go` (replace the ClusterN negative/unsupported cases with a ClusterFan
  table: empty fan string skips resolution; unknown fan, unknown lens, over-cap fan
  propagate `ResolveFan` errors; happy path populates `clusterLenses` in fan order),
  `engine_test.go` (the `New` helper at its line ~80 gains a `Config{}` argument;
  add one case asserting a cluster profile's shuttle Spec has `ForkSubagents: true`
  and a plain profile false — extend the existing fake-shuttle spec-capture pattern),
  and `smoke_round_test.go` (compile only: `New` call gains a `Config{}` argument and
  its profile literal drops `cluster-n`).
- **Commit:** `burler: replace ClusterN gate with fan-resolved ClusterFan`

### Card 9: cluster prompt block

- **Context:**
  - `internal/burlerengine/config.go`
  - `internal/burlerengine/template.go`
  - `internal/stencil/stencil.go`
  - `docs/research/session-fork-spike.md`
- **Edits:**
  - `internal/burlerengine/prompt.go`
  - `internal/burlerengine/prompt_test.go`
  - `internal/burlerengine/review-prompt-template.md`
  - `internal/burlerengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one top-level marker `{{.cluster_rules}}` to
  `review-prompt-template.md` as a new `## Cluster rules` section between the
  sequencing-rule and fix-everything-rule sections, and extend the template's
  review-file-format section with one static sentence: in a cluster round each finding
  carries an `origin:` key (`lens:<name>` or `handler`). Keep the template free of
  `{{if}}`/`{{range}}` (stencil requires every marker non-empty — same constraint the
  template's header comment documents; update that comment's marker count). In
  `prompt.go` add `clusterRulesBlock(p *Profile) string`, wired into `composePrompt`'s
  values map. Non-cluster value: explicit prose — "This is a single-reviewer round —
  no cluster forks. Do the full review yourself." Cluster value, composed from
  `p.clusterLenses`, must state (phrase for the agent, in the template's voice):
  (a) phase structure — after exploring the target fully, spawn ALL fork reviewers in
  a SINGLE message via the Agent tool with `subagent_type: "fork"`, one per lens
  listed below, never passing a `name` (named forks silently lose inherited context);
  (b) the per-lens fork prompt = a fixed boilerplate plus that lens's emphasis text,
  where the boilerplate requires: prefer inherited context and fetch only what the
  lens needs; READ-ONLY discipline — never Write/Edit/delete any file, never run any
  git command, never touch the two round output files, never call the Agent tool
  (forks cannot nest); return findings ONLY as the fork's final message, each with
  severity, location, and a one-line summary; (c) while forks run, the handler does
  its own HOLISTIC review — architecture, cross-file invariants, CONSTRAINTS-fit —
  and prepares ground truths plus the severity rubric for judging; (d) consolidation —
  judge every fork finding and the handler's own findings with equal skepticism,
  dedup across lenses, tag every kept finding's frontmatter with `origin:`
  (`lens:<name>` or `handler`), move false positives to a `## Rejected` prose section
  below the frontmatter with a one-line reason each (rejected items never appear in
  `findings:`), order by severity; the consolidated review is the ONE review file and
  must be fully on disk before job B touches anything (A-before-B: consolidation is
  part of A). Lens texts are rendered as a list: `- <name>: <text>`. Extend
  `prompt_test.go` (cluster block present with every lens name and both boilerplate
  ban phrases at cluster; non-cluster prose otherwise; all markers non-empty both
  ways) and `template_test.go` (pin the new statements: single-message spawn, unnamed
  forks, read-only/no-git fork discipline, consolidation-before-B, origin labels,
  Rejected section — following the file's existing pin style).
- **Commit:** `burler: compose cluster-rules prompt block from resolved fan lenses`

### Card 10: Finding.Origin

- **Context:**
  - `internal/burlerengine/review-prompt-template.md`
- **Edits:**
  - `internal/burlerengine/verdict.go`
  - `internal/burlerengine/verdict_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `Origin string` with yaml tag `origin` to `Finding` in
  `verdict.go`, doc comment: optional free-text provenance label (`lens:<name>` /
  `handler`) written by cluster rounds; never validated, never required —
  `ParseReview` accepts files with and without it (backward compatibility with every
  existing review file). No `validateFindings` change. `verdict_test.go`: one case
  with `origin:` on some findings asserting it parses through, one asserting an
  origin-less file still parses identically to today.
- **Commit:** `burler: carry optional origin label on parsed findings`

### Card 11: cluster audit policy in Engine.Run

- **Context:**
  - `internal/shuttleengine/forkaudit.go`
  - `internal/shuttleengine/run.go`
  - `internal/burlerengine/profile.go`
- **Edits:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/engine_test.go`
- **Creates:**
  - `internal/burlerengine/cluster.go`
  - `internal/burlerengine/cluster_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `cluster.go` defines `var ErrClusterForksMissing = errors.New(...)`
  (message: the cluster round did not produce exactly the requested number of fork
  reviewers; no "burler: " prefix, matching the deleted sentinel's prefix rationale)
  and `func auditClusterRound(audit *shuttleengine.ForkAudit, wantN int) (warnings
  []string, err error)` enforcing, in order, the discussion's violation classes —
  hard errors (burler-prefixed, first failure returned): nil audit or
  `len(audit.Forks) != wantN` → wrap `ErrClusterForksMissing` with `%w` and the
  requested/actual counts ("requested %d, spawned %d"); any fork with
  `AgentCalls > 0` (an Agent ATTEMPT in a fork transcript is a violation even if the
  hook denied it — the fork disobeyed the ban); any fork with `WriteCalls > 0`; any
  fork with a `BashCommands` entry matching the package-level
  `var mutatingGitPattern = regexp.MustCompile(...)` (matches a `git` invocation whose
  subcommand is one of add, commit, push, pull, fetch, merge, rebase, reset, restore,
  rm, mv, checkout, switch, stash, apply, cherry-pick, tag, branch — word-boundary
  match on the command string, tolerant of leading paths/`&&`/`;` chaining);
  `audit.NamedSpawns > 0` (named forks silently lose inherited context — silent
  quality degradation is the rejected class). Warnings (returned, never failing): a
  fork with `ReportReturned == false`. In `engine.go` `Run`: extend `Result` with
  `ForkAudit *shuttleengine.ForkAudit` and `ClusterWarnings []string`; after the done
  classification and BEFORE the review-file read, when `p.ClusterFan != ""` copy
  `shuttleResult.ForkAudit` onto the Result and call
  `auditClusterRound(shuttleResult.ForkAudit, len(p.clusterLenses))` — a policy error
  returns the populated-so-far Result with a wrapped hard error (same fail-loud shape
  as the verdict-parse failure path); warnings land in `Result.ClusterWarnings`.
  `cluster_test.go`: table over the full taxonomy (exact-N pass; shortfall/zero/nil →
  `errors.Is(err, ErrClusterForksMissing)` and the counts in the message; each
  violation class; the git-pattern matrix including non-mutating `git log`/`git diff`
  commands that must NOT match and a chained `cd x && git commit` that must);
  `engine_test.go`: a fake-shuttle done Result carrying a violating audit fails Run,
  a clean one passes with warnings copied through, and a non-cluster profile never
  invokes the policy.
- **Commit:** `burler: enforce fork audit policy on cluster rounds`

### Card 12: burlercli cluster-fan wiring

- **Context:**
  - `internal/burlerengine/config.go`
  - `internal/burlerengine/engine.go`
- **Edits:**
  - `internal/burlercli/cli.go`
  - `internal/burlercli/run.go`
  - `internal/burlercli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `run.go`: `profileYAML` drops `ClusterN int` (`cluster-n`) for
  `ClusterFan string` (`cluster-fan`); `decodeProfile` maps it; the `Long` example
  profile replaces `cluster-n: 0` with `cluster-fan: ""` plus one comment-line noting
  that naming a fan from burler.yaml activates cluster review with one fork per fan
  entry; the success envelope gains `"clusterWarnings": result.ClusterWarnings` and
  `"forkCount": len(result.ForkAudit.Forks)` (0 when nil — guard the nil). `cli.go`:
  `PersistentPreRunE` loads `burlerengine.LoadConfig(layout.Cwd)` after the shuttle
  config (its only error today is a read/decode failure — absent file is zero Config)
  and passes it to `burlerengine.New(runner, layout, burlerCfg)`; update the wiring
  doc comments. `cli_test.go`: update the decode tests for the key swap (including
  the strict-decode rejection of the now-unknown `cluster-n` key — assert the error
  names it) and the envelope shape.
- **Commit:** `burlercli: profile cluster-fan key and burler config wiring`

### Card 13: perchengine passthrough

- **Context:**
  - `internal/burlerengine/profile.go`
- **Edits:**
  - `internal/perchengine/profile.go`
  - `internal/perchengine/roundfiles.go`
  - `internal/perchengine/roundfiles_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `perchengine/profile.go` replace the burler content field
  `ClusterN int` with `ClusterFan string` (keep the field-group doc comment's
  carried-1:1-into-every-round statement; validation stays burler's job inside the
  first round's `Engine.Run`, unchanged posture). `buildRoundProfile` in
  `roundfiles.go` maps `ClusterFan: p.ClusterFan`. `roundfiles_test.go`: update the
  passthrough assertion to the new field.
- **Commit:** `perch: pass cluster-fan through to round profiles`

### Card 14: perchcli passthrough and wiring

- **Context:**
  - `internal/burlerengine/config.go`
  - `internal/burlercli/run.go`
- **Edits:**
  - `internal/perchcli/cli.go`
  - `internal/perchcli/run.go`
  - `internal/perchcli/run_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `perchcli/run.go`: the profile YAML mirror swaps `cluster-n` →
  `cluster-fan` (string) and its mapping; the `Long` example updates the same way as
  burlercli's (card 12). `perchcli/cli.go`: load `burlerengine.LoadConfig(layout.Cwd)`
  and pass it to the `burlerengine.New(runner, layout, burlerCfg)` call at its line
  ~130. `run_test.go`: decode-table updates for the key swap.
- **Commit:** `perchcli: cluster-fan profile key and burler config wiring`

## Batch Tests

`go test ./internal/burlerengine/ ./internal/burlercli/ ./internal/perchengine/
./internal/perchcli/` — the four packages this batch edits: profile/fan-resolution
tables, prompt cluster-block and template pins, origin parsing, the full audit-policy
taxonomy in `cluster_test.go`, both CLIs' decode/envelope tables, and the perch
passthrough. The scope spans four packages because the field swap is one compile unit
(see Batch Scope).
