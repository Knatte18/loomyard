# Batch: scheme-refinements

```yaml
task: "Refine SANDBOX-SUITE.md from the 2026-06-30 sandbox run"
batch: "scheme-refinements"
number: 1
cards: 6
verify: go test ./tools/sandbox/...
depends-on: []
```

## Batch Scope

This batch delivers the seven meta/doc refinement items from the discussion (issues #39,
#40, #41), spread across the six cards below — Card 1 groups three closely-related edits
(the Operating-model paragraph, the S4 reword, and the S6 verdict note). Every change is
an edit to the single
authored source file `tools/sandbox/test-scheme.md`, which `tools/sandbox/suite.go`
embeds and renders into `SANDBOX-SUITE.md` at launch. It is one batch because the edits
all live in one ~180-line markdown file and a single implementer holds the whole file in
context trivially. There is no external interface for a later batch to consume — this is
the only batch. No lyx Go code changes. The `# Sandbox test-scheme` H1 heading MUST be
preserved (pinned by `suite_test.go`); see Shared Decisions in `00-overview.md`.

## Cards

### Card 1: Operating-model paragraph + S4 reword + S6 verdict note

- **Context:**
  - `internal/boardengine/config.go`
  - `tools/sandbox/suite.go`
- **Edits:**
  - `tools/sandbox/test-scheme.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In the `## Pre-conditions` section, add an **Operating model** paragraph stating:
    lyx resolves against the current directory's own `_lyx/` and does **not** walk up to a
    parent; the hub host repo is initialized at its root, so the agent runs the entire
    session from there (cwd is fixed at the root); running a lyx command from a
    subdirectory that has not itself been initialized correctly reports
    `not initialized here; run "lyx init"` — that is expected behaviour, **not a
    finding**. Add the explanatory note that `lyx init` in a subdirectory would create
    `_lyx/` there and make lyx work in that subdir, but the agent must **not** scaffold
    nested `_lyx/` during a session. (The exact error string is sourced from
    `internal/boardengine/config.go`'s `LoadConfig`.)
  - Reword the S4 `**Watch:**` line. Replace the current text
    ``Watch: `lyx config` read/write. Does it find the right config scope from cwd?``
    with ``Watch: From the worktree root, does `lyx config` read/write the correct
    `_lyx/config/` and round-trip a value?`` — removing any directory-varying reading.
  - In the S6 `**Watch:**` line, append a sentence stating that a legible
    `not initialized` / "run from the initialized root"-style message is the `OK`
    (ergonomics-pass) outcome, not a `FAIL` — do not file it as a finding. This keeps S6
    consistent with the Operating-model paragraph and prevents re-creating the #35 false
    positive at S6.
- **Commit:** `docs(sandbox): state lyx operating model; fix S4/S6 subdir wording`

### Card 2: PowerShell JSON-quoting note

- **Context:**
  - `internal/boardcli/cli.go`
- **Edits:**
  - `tools/sandbox/test-scheme.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add a **PowerShell JSON-quoting** note to the `## Pre-conditions` section. State that
    when driving the suite from Windows PowerShell (the assumed session shell on Windows),
    backslash-escaping a JSON argument is the intuitive-but-wrong move and yields
    `{"error":"invalid json: invalid character '\\' looking for beginning of object key string","ok":false}`;
    the working form is a single-quoted string with literal inner double quotes, e.g.
    `lyx board upsert '{"slug":"s3-demo","title":"S3 demo"}'`. (The example mirrors the
    `upsert [json-payload]` help example in `internal/boardcli/cli.go`.)
  - Add a one-line pointer in S3 (where the agent adds a task to the board with a JSON
    payload) referencing the PowerShell JSON-quoting note in Pre-conditions.
- **Commit:** `docs(sandbox): add PowerShell JSON-quoting note for board upsert`

### Card 3: S2 — raw git is acceptable for host commits

- **Context:**
  - `docs/sandbox-hub.md`
- **Edits:**
  - `tools/sandbox/test-scheme.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Reword the S2 `**Watch:**` line. Remove the text framing a raw-`git` fallback as a
    gap (the sentences "Does lyx own this flow or do you fall back to raw `git`? Falling
    back is a gap in the lyx surface."). Replace with wording that the host is an ordinary
    git repo — committing host changes with plain `git` is acceptable and **not** a
    finding — and refocus the Watch on lyx's actual responsibility: host/weft coordination
    (junctions wired correctly, weft mirroring behaves). Keep the S2 Goal line unchanged.
- **Commit:** `docs(sandbox): S2 -- raw git is fine for host commits`

### Card 4: Board-durability note in S3

- **Context:**
  - `internal/boardcli/cli.go`
- **Edits:**
  - `tools/sandbox/test-scheme.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In S3, add a note that the board is **durable across sessions** — it starts non-empty
    (e.g. a `T1 "Test task from S3"` task persists from prior runs), so the agent must not
    assume a fresh board — and instruct the agent to **clean up the test tasks it creates**
    at session end. Add this note to S3 **only**; do **not** edit S1 (its "Hub
    orientation" text makes no fresh-board assumption). The note leans on the `lyx board
    list` subcommand (the agent observes the persisted `T1` task) and the `lyx board
    remove` subcommand (used to clean up the test tasks) — both defined in
    `internal/boardcli/cli.go`.
- **Commit:** `docs(sandbox): note board durability + test-task cleanup in S3`

### Card 5: Reframe S5 as an operator-only step

- **Context:**
  - `docs/sandbox-hub.md`
- **Edits:**
  - `tools/sandbox/test-scheme.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Remove the `### S5 -- Re-clone and reset idempotency` scenario block from the
    `## Scenarios` section (including its trailing `---` separator). Leave S6 numbered S6
    — do **not** renumber.
  - Add a new `## Operator steps` subsection, placed **before** `## Session log format`,
    describing the `sandbox.cmd -reset` idempotency check as an **operator-run** step
    performed before (and optionally after) the session, outside the agent transcript:
    clean teardown and rebuild, no stale junctions, no leftover state, no "directory in
    use" Windows handle errors. State why it is operator-only (it lives outside the hub
    and `-reset` would destroy the worktree the agent's shell is `cwd`-ed into — the
    black-box rule forbids it).
  - In the `## Session log format` fenced block, change the `S5:` line so it is
    operator-supplied/transcribed (e.g. `S5: <operator-supplied>`) rather than an agent
    verdict line; keep the S0–S4 and S6 lines as agent verdict lines. Update the prose
    after the block ("File one GitHub issue per WARN or FAIL finding...") only if it
    references S5 as an agent scenario.
- **Commit:** `docs(sandbox): reframe S5 as operator-only step`

### Card 6: Clarify "cwd-relpath mirroring" in Notes

- **Context:**
  - `docs/sandbox-hub.md`
- **Edits:**
  - `tools/sandbox/test-scheme.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Add a bullet to the `## Notes` section clarifying that the host
    `Knatte18/lyx-test` README's phrase "cwd-relpath mirroring" refers to **weft path
    mirroring** (how the weft worktree mirrors host subpaths), **not** to running lyx from
    subdirectories. Name the host README as the source of the phrase, since "cwd-relpath"
    does not otherwise appear anywhere in `test-scheme.md`.
- **Commit:** `docs(sandbox): clarify cwd-relpath mirroring is weft path mirroring`

## Batch Tests

This is a pure-markdown batch; the only runnable surface is the Go `//go:embed` of
`test-scheme.md` in `tools/sandbox/suite.go`. The frontmatter `verify: go test
./tools/sandbox/...` runs the `tools/sandbox` package tests — chiefly
`TestRenderScheme_ContainsHeaderAndBody` (which pins the `"Sandbox test-scheme"` heading)
and the embed compilation. Because every card preserves the H1 heading, the verify
confirms the embed still resolves and the heading is intact after the edits. There is no
finer-grained assertion on the scenario body to add — the suite content is operator-/
agent-facing prose, validated by review, not by unit tests.
