# Batch: text-references

```yaml
task: "loom's status file can conflict on the landing merge"
batch: "text-references"
number: 2
cards: 4
verify: go test ./contracts/... ./internal/loomengine/... ./internal/shedengine/... ./internal/loomshed/... ./internal/lyxcwd/...
depends-on: [1]
```

## Batch Scope

This batch updates every reference that *names* loom's status file as text rather than resolving it through a constructor — the class the Go call-site trace is structurally blind to.
One of these is behavioral, not drift: `contracts/stencils/loom/loom-rubric-webster-review.md` is live agent prompt text wired into two recipe rows, and it instructs the Webster-Review agent to raise a BLOCKING finding and review nothing when the status file cannot be read.
Left stale, it would force a spurious BLOCKING into every Webster-Review round of every future loom run, so the review segment would never converge.
A second stencil, `contracts/stencils/loom/loom-template-discussion.md`, fences the Discussion agent off a directory that no longer exists while leaving the real status directory unfenced.

It depends on batch 1 because it describes the post-move state; nothing in it changes behavior of Go code, and no batch consumes anything from it.

## Cards

### Card 8: Retarget the Webster-Review rubric's status-file path

- **Context:**
  - `contracts/recipes/loom-recipe.yaml`
  - `contracts/stencils/rubric_test.go`
  - `contracts/stencils/registry_test.go`
- **Edits:**
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the "Determining the review range" section, change both occurrences of the path the agent is told to read from the old durable location to `.lyx/loom/status.json`: step 1's "Read ... and take `product.parent`, the branch this run started from", and the sentence below step 2 beginning "If ... cannot be read, or its `product.parent` is empty or absent, raise a BLOCKING finding".
  Change nothing else in the section — the two steps stay read-only, the BLOCKING-on-unreadable disposition stays, and the "Silently reviewing a guessed range is a worse failure than an honest block" sentence stays.
  Introduce no `warp` or `weft` token: this file is walked by `TestEnforcement_FabricVocabulary`'s markdown pass over `contracts/stencils/`, which fails on either bare token outside the owner set.
  The stencil is wired as `rubric_stencil: loom-rubric-webster-review` on the `Webster-Bouncer` and `Webster-Burler` rows of `contracts/recipes/loom-recipe.yaml`; that wiring is unchanged and needs no edit here.
  Record, without acting on it, that an operator's already-seeded board copy of this rubric does not pick the edit up on its own — the board's stencils tree is seeded on first run and persists, and `lyx stencil`'s existing promote/sync surface is what reconciles it.
- **Commit:** `fix(stencils): point the Webster-Review rubric at the new status path`

### Card 9: Retarget the Discussion stencil's fence and its pinning assertion

- **Context:**
  - `contracts/stencils/rubric_test.go`
- **Edits:**
  - `contracts/stencils/loom/loom-template-discussion.md`
  - `contracts/stencils/discussiontemplate_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In the read-only fence list, retarget the bullet that currently reads ``- **`_lyx/loom/`** — the phase machine's status file. The driver owns it; a write from here corrupts orchestration state.`` so it names `.lyx/loom/` instead, keeping the rest of the bullet's wording as it stands.
  After the move that directory is where the live status file is, and the old one no longer exists — leaving the fence unchanged would warn the agent off an empty path while leaving the real one unguarded.
  Leave the `_lyx/config/` and `_lyx/plan/` bullets alone; both still exist.
  In `TestLoomTemplateDiscussion_FencesWhatItMayWrite`'s table, change the `{"fences the phase machine's status file", ...}` row's `phrase` field from the backtick-wrapped old directory literal to the backtick-wrapped `.lyx/loom/` literal, leaving the row's `name` field and every other row unchanged.
  Both edits land in this one card and therefore in one commit: the assertion pins the exact literal, so the stencil edit fails that test until the assertion moves with it, and splitting them across cards would leave a red commit between them.
- **Commit:** `fix(stencils): retarget the Discussion fence to the new status directory`

### Card 10: Update the recipe's tool-use justification comment

- **Context:**
  - `contracts/stencils/loom/loom-rubric-webster-review.md`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  On the `Webster-Burler` row, the `tool-use: true` justification comment says the round "reads _lyx/loom/status.json and runs read-only git to obtain the diff, which is its entire subject".
  Update the path it names to the new `.lyx` location so the comment matches the rubric card 8 edits.
  This is a comment only: `fix-scope`, `tool-use`, and every other key on the row are unchanged, and no recipe row's behavior changes.
- **Commit:** `docs(recipes): update the Webster-Burler tool-use comment's status path`

### Card 11: Update the Go doc comments naming or characterising the status file

- **Context:**
  - `internal/loomengine/config.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomengine/status.go`
  - `internal/loomengine/report.go`
  - `internal/loomshed/seed.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/loomengine/status.go`, the package doc comment says "CheckSeed validates _lyx/loom/status.json coherence over told absolute paths" — update the path to the new `.lyx` location.
  In `internal/loomengine/report.go`, update the five occurrences of the old path in the loom-specific check block: the block's own lead-in ("declared here because internal/preflight has no notion of ...") and the four `CheckID` constant comments on `CheckSeedMissing`, `CheckSeedUnreadable`, `CheckSeedIncoherent`, and `CheckHalfFinished`.
  Each is a path mention only; no verdict semantics change.
  In `internal/loomshed/seed.go`, update the file-header comment's "nothing else in production writes _lyx/loom/status.json today" to name the new path.
  While in that file, leave `Seed`'s own `os.MkdirAll` on the status lock's parent directory in place and leave its explanatory comment as it stands: after the move the status file and its lock share one parent, so `internal/state`'s own `MkdirAll` would cover it, but the call runs first and is harmless, and removing it would couple this function to that ordering.
  In `internal/shedengine/shed.go`, the `StatusPath` field comment reads "StatusPath is the durable status file; it is told and never derived."
  Drop the durability claim and keep the told-never-derived half — `Shed` is generic, so it has no standing to say which side of the durable/ephemeral line its caller's status file sits on.
  In `internal/shedengine/doc.go`, rewrite the "Told, never derived" paragraph's caller obligation, currently "The caller is responsible for supplying paths that already obey the Durable-vs-Ephemeral State Invariant in CONSTRAINTS.md -- the status file durable, both locks never-tracked transients -- because Shed cannot and does not choose either location."
  Keep the obligation itself and the invariant reference, and delete the parenthetical that assigns each of the three paths to a side.
  Do not replace it with "the status file ephemeral" — that would mislead the next product in the other direction, which is exactly the failure being corrected.
  Introduce no `warp` or `weft` token in any of these five files: none of their directories is in `TestEnforcement_FabricVocabulary`'s owner set, and the Go pass covers production files under `internal/`.
- **Commit:** `docs(loom): update status-file doc comments for the new location`

## Batch Tests

`verify:` runs the packages whose tests pin this batch's text.
`./contracts/...` covers `discussiontemplate_test.go`, whose `TestLoomTemplateDiscussion_FencesWhatItMayWrite` is the Tier-1 marker assertion card 9 moves, plus `rubric_test.go` and `registry_test.go`, which validate the stencil registry and the rubric markers card 8 edits around.
`./internal/lyxcwd/...` runs `TestEnforcement_FabricVocabulary`, whose markdown pass walks `contracts/stencils/` and whose Go pass walks `internal/` — the machine check that none of these prose edits leaked a `warp`/`weft` token.
`./internal/loomengine/...`, `./internal/shedengine/...`, and `./internal/loomshed/...` are compile-and-run coverage for the four packages whose doc comments card 11 rewrites; comment-only edits cannot change behavior, so these exist to catch an accidental code edit rather than to prove anything new.
The recipe edit in card 10 is a YAML comment and is covered by the recipe-parsing tests that already run under `./internal/...`; no new case is added for it.
