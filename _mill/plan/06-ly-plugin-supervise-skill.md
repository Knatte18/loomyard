# Batch: ly-plugin-supervise-skill

```yaml
task: "lyx loom step + external supervisor skill"
batch: "ly-plugin-supervise-skill"
number: 6
cards: 3
verify: go test ./internal/lyxcwd/...
depends-on: [5]
```

## Batch Scope

This batch delivers the operator-facing half of the task: a new `plugins/ly` Claude Code plugin holding one skill, `ly-supervise`, registered in the marketplace, plus the `loom.md` skills-table row that stops claiming `/ly-*` skills are thin wrappers over `lyx loom run`.
It is one batch because the plugin scaffolding is meaningless without the skill it exists to hold, and the skills-table row is the doc surface that becomes false the moment the skill ships.
It has no Go code dependency; the `depends-on: [5]` edge exists to serialise the shared `manifest/designs/loom.md` edit, per `## Shared Decisions`' loom-md-edits-are-serialised-through-the-batch-chain.

Batch-local decision: the skill gets no test harness. Its correctness bar is the manual end-to-end pass described in `## Batch Tests` below, which is the operator's to run — an LLM skill body has no runnable surface, and inventing one would be a fiction.

## Cards

### Card 16: the ly plugin scaffolding

- **Context:**
  - `plugins/scribe/.claude-plugin/plugin.json`
  - `plugins/scribe/skills/INDEX.md`
  - `.claude-plugin/marketplace.json`
  - `docs/overview.md`
- **Edits:**
  - `.claude-plugin/marketplace.json`
- **Creates:**
  - `plugins/ly/.claude-plugin/plugin.json`
  - `plugins/ly/skills/INDEX.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `plugins/ly/.claude-plugin/plugin.json` copying `plugins/scribe/.claude-plugin/plugin.json`'s shape: `name` `"ly"`, a one-line `description` naming it as the operator surface over `lyx`'s loom verbs, `version` `"1.0.0"`, `license` `"Apache-2.0"`, and `author` `{"name": "Knatte18"}`. Omit the `hooks` key — `scribe` carries it because it ships a session-start hook, and `ly` ships none.

  Create `plugins/ly/skills/INDEX.md` as a table of skill links, matching `plugins/scribe/skills/INDEX.md`'s shape: an H1 naming the plugin's skills, then a two-column `| Skill | Description |` table with one row, `[ly-supervise](ly-supervise/SKILL.md)`, whose description matches the skill's own frontmatter `description`. State below the table that `ly-supervise` is explicit-invocation-only and is never started by a model on its own.

  In `.claude-plugin/marketplace.json`, append a third entry to the `plugins` array with `name` `"ly"`, the same `description` as the plugin manifest, `version` `"1.0.0"`, `author` `{"name": "Knatte18"}`, `source` `"./plugins/ly"`, and `category` `"productivity"` — matching the existing `prowler` and `scribe` entries' key order and shape exactly. Keep the file valid JSON with the repo's existing two-space indentation.

  Per `## Shared Decisions`' no-version-bumps rule, both version fields are `1.0.0` and no existing plugin's version changes. `docs/overview.md` pins `ly` as the skill-plugin name and `/ly-*` as the skill naming, so neither `lyx` nor `loom-step` is available as a name here.
- **Commit:** `feat(ly): add the ly plugin scaffolding and register it in the marketplace`

### Card 17: the ly-supervise skill

- **Context:**
  - `plugins/ly/skills/INDEX.md`
  - `plugins/scribe/skills/handoff/SKILL.md`
  - `plugins/scribe/skills/conversation/SKILL.md`
  - `internal/loomcli/step.go`
  - `internal/loomcli/status.go`
  - `internal/loomshed/interruptpolicy.go`
  - `manifest/designs/loom-step.md`
  - `CLAUDE.md`
- **Edits:** none
- **Creates:**
  - `plugins/ly/skills/ly-supervise/SKILL.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Open with YAML frontmatter carrying `name: ly-supervise`, a one-line `description`, and `disable-model-invocation: true` — the skill is a deliberate operator action and must never be started by a model on its own.

  Write the body so a cold agent can drive a loom task with no other context. It must carry **no phase knowledge**: it never names a producer, never predicts what comes next, and never decides what runs. Where a decision depends on which row is current, it branches on the policy word Go hands it, never on a row name. Per `## Shared Decisions`' interrupt-policy-is-go-supplied rule, a producer name must not appear anywhere in this file.

  Cover, in order:

  **Preconditions.** The session's current working directory must be the task worktree root, because `lyx loom step` derives everything from cwd (`lyxcwd.Resolve` requires a git worktree root) — verify this before the first step rather than discovering it as a resolve failure mid-loop. Tell the operator to open `lyx reed attach` in a side terminal before starting, as a second, independent watching layer.

  **The pre-loop baseline.** Take one `lyx loom status` read before the first step and record its `current_producer` and `history_length` as the baseline. State that the baseline always exists for this reason: the interrupted-step branch compares against it, and the very first step of a session — or the first step after an operator re-invokes past the iteration cap — has no prior step envelope to compare with.

  **How to invoke a step.** Never as a blocking foreground shell call. Launch it in the background with stdout redirected to a per-step file under `.scratch/ly-supervise/step-<n>.json`, then wait for that process to exit and read the envelope from the file. Explain why: a single step blocks for a whole producer call, loom's LLM rows are minutes-to-an-hour agent spawns, and every agent shell tool caps a foreground call in the single-digit minutes — so a foreground call would be killed mid-producer on exactly the rows the supervisor exists to watch. Note that redirecting to a file also keeps each envelope out of the transcript until the skill chooses to read it, which matters across a long loop. Note that `.scratch/` is relative to the same cwd, is already gitignored repo-wide, and is the mandated scratch location — never the OS temp directory.

  **The loop.** A hard cap of **40 steps**. Each iteration: invoke a step, read its envelope, read the artifact its `output` names when non-empty, and continue only while `continue` is true. Record what 40 is: loom's list is seventeen rows, a review round inside a segment costs two steps, and a run taking three review rounds in each of three segments walks thirty-five, rounded up. State plainly that 40 is a typical-run margin and not the mechanical ceiling, which is far higher — the six review rows carry their own budget of five bounces and the three validator rows bounce at the inherited default of ten, putting the worst case near a hundred. On reaching the cap, stop and report rather than failing anything; loom's own state is untouched by the cap, and the operator re-invokes the skill to continue.

  **Stopping.** On any envelope whose `continue` is false, and on any error envelope, stop looping and hand back to the operator with a report. Never clear `state`, never edit the status file, never re-seed, never push. State the reason: a non-running state means a Go gate concluded a human is needed, and an LLM deciding otherwise is the judgment-override the whole design avoids.

  **Error envelopes.** An error envelope carries a `kind` key with exactly one of five values. On `producer`, re-invoke the step **once**; if the retry also errors, stop and report. On every other kind, hand straight back with no retry — none can be fixed by running the same command again, and a retry would read to the operator as though the skill were trying something. Name the remedy the envelope itself gives for the busy kind: the operator has a driver running and must pause it. State the hard limit on cleanup: the skill never kills reed panes, removes strands, deletes lock files, touches git, or edits any task artifact. It reports debris; the operator resolves it.

  **Interrupted invocations.** An invocation that was killed, timed out, or exited without writing a parseable envelope is a distinct case from an error envelope and writes no envelope of its own. On detecting one, read `lyx loom status` once and branch on that single read: if `current_producer` or `history_length` differs from the baseline, or `state` is no longer running, the producer finished and only the invocation died — continue from the fresh status and do not count the step twice. If nothing differs, `state` is still running, and the status envelope's `interrupt_policy` is `reinvoke`, re-invoke the step for the same row; this is loom's own designed crash-resume, not a retry of something unknown. If nothing differs and the policy is `handback`, stop and hand back, saying plainly that a live agent may still be running in its pane and that a later re-invocation would restart it rather than attach to it. Cap the re-invoking branch at **two consecutive** interrupted-and-re-invoked steps against the same row; on a third, stop and hand back, because something is wrong with the invocation mechanism itself rather than with the run. State that outside the handback branch no orphaned-agent warning is printed, because there is no orphan — the next step attaches to the agent rather than abandoning it.

  **Self-report.** The skill may call `lyx selfreport create` only after the loop has stopped — terminal state, blocked, hand-back, or iteration cap — never between steps, and at most once per supervised run. It drafts the title and body, shows both to the operator, and fires only on explicit operator approval. The body goes in via `-b -` on stdin and states the row's policy-relevant facts, the envelope fields that were surprising, and the artifact path it read. The default `bug` label stands for a defect; `--label enhancement` is for friction that is not a defect. Record why the gate exists: the verb files a real public issue through the GitHub API, an outward-facing and hard-to-reverse act.

  **Operator choices.** Any point where the skill offers the operator a choice must present it as a numbered text list, one option per line in the form `1) Label — description`, never a mouse-driven prompt.

  Write the whole file in semantic line breaks per `CLAUDE.md`: one sentence per line, breaking inside a long sentence only at an internal independent-clause boundary, using plain newlines and never trailing double-spaces or a backslash.
- **Commit:** `feat(ly): add the ly-supervise skill driving lyx loom step in a supervised loop`

### Card 18: correct loom.md's /ly-* skills table row

- **Context:**
  - `plugins/ly/skills/ly-supervise/SKILL.md`
  - `plugins/ly/skills/INDEX.md`
  - `manifest/designs/loom-step.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `manifest/designs/loom.md`'s skill-layer table has a `/ly-*` skills row whose right-hand cell reads "over `lyx loom run`". That is now false: the first shipped `/ly-*` skill wraps `lyx loom step`, not `run`. Rewrite the cell to name `ly-supervise` as the first shipped skill, describe it as the supervised step-loop over `lyx loom step`, and say it carries no phase knowledge of its own. Keep the cell on one line — the repo's markdown convention keeps table cells to one line each, unlike prose.

  If any inline link is added, its file part and `#anchor` must resolve, per the Markdown Link Integrity invariant enforced by `internal/lyxcwd`'s `docslink_test.go`, which is in this batch's verify scope.
- **Commit:** `docs(loom): point the /ly-* skills row at ly-supervise and lyx loom step`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` runs `docslink_test.go`, the Markdown Link Integrity guard over `manifest/` and `docs/`, which is the only mechanical check any file in this batch is subject to. No Go production code changes here.

Nothing in the tree tests `.claude-plugin/marketplace.json` or a plugin manifest, so card 16's JSON validity is the implementer's own responsibility — parse the file after editing rather than assuming.

The skill itself is not unit-testable and gets no test harness, per this batch's scope decision. Its correctness bar is a manual end-to-end pass the operator runs after the task lands: drive one real task from an unseeded worktree to a terminal or blocked state with `/ly:ly-supervise`, with `lyx reed attach` open alongside, confirming the loop reads each step's `output` artifact, halts on the first non-running state, and never mutates state itself. Three behaviours a happy path never reaches must be exercised deliberately in that pass: a step spanning a real multi-minute LLM row, confirming the backgrounded invocation survives where a foreground call would not; an invocation killed mid-producer on a row whose policy is `reinvoke`, confirming the skill sees nothing moved, re-invokes, and that the re-invocation attaches to the live agent rather than spawning a second one — checked against the reed panes, not just the envelope; the same kill on the row whose policy is `handback`, confirming the skill stops and says a live agent may remain; an interruption of the session's very first step, confirming the skill read its pre-loop baseline and takes the policy from `lyx loom status` rather than from a prior envelope it does not have; and the same kill against a producer that finishes anyway, confirming the skill notices the status moved and continues rather than double-counting. The `lyx selfreport create` path is verified by drafting an issue and declining at the confirmation prompt, confirming nothing is filed.
