# Batch: go-comment-sweep

```yaml
task: "Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise"
batch: "go-comment-sweep"
number: 3
cards: 3
verify: go test ./internal/loomengine/... ./internal/loomshed/... ./internal/shuttleengine/... ./internal/webstercli/... ./internal/websterengine/... ./internal/frictionengine/... ./internal/friction/... ./internal/shedadapters/...
depends-on: [2]
```

## Per-file sweep rule

Identical to batch 1's rule of the same name, and it governs this batch too: a file in a card's `Edits:` is swept **whole**, and the sites named in `Requirements:` are landmarks pinning the hard judgment calls, never the boundary.
Sweep for verb names (`lyx loom run`/`lyx loom drive` and bare backticked `` `run` ``/`` `drive` `` naming a verb), pre-move filename citations (`run.go` -> `start.go`, `drive.go` -> `run.go`), the identifiers `runCmd`/`driveCmd`/`RunAliasCommand` including test function names, and `ly-supervise` -> `ly-drive`.

Leave the noun sense alone — "run" meaning "an execution", and "driver"/"drives" as ordinary English — per the batch-local decision below.

## Batch Scope

This batch sweeps the verb names out of Go comments in packages outside `internal/loomcli` — the counterpart gotcha the discussion names, where genuine verb hits live in packages a file-by-file scope would not think to enumerate.
Every change here is comment text, so nothing in this batch can break a compile;
it is separated from batch 1 so the atomic rename's diff stays readable and so this batch's judgment-heavy classification work is reviewed on its own.

The cards are split by classification difficulty rather than by package: card 10 is the straightforward bootstrap-sense rewrites, card 11 is the contrast prose that must be rewritten as sentences, and card 12 is the test-comment and fixture surface.

Batch-local decision: the disposition-2 leave list is enforced per card, not just stated once.
`internal/burlercli/wiring.go`, `internal/githubclient/doc.go`, `internal/landingshed/deps.go`, `internal/selfreportengine/selfreport.go`, and `internal/webstercli/wiring.go`'s standalone-webster comment all use "run" as a *noun* meaning "an execution" and are outside this batch's `Edits:` entirely, except `webstercli/wiring.go`, which card 10 edits for a different line and must leave its noun usage alone.

## Cards

### Card 10: bootstrap-sense comment rewrites

- **Context:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/run.go`
- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/loomshed/seed.go`
  - `internal/frictionengine/spec.go`
  - `internal/webstercli/wiring.go`
  - `internal/websterengine/strand.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Each hit below names the bootstrap verb and becomes `lyx loom start`;
  read the surrounding sentence before editing, because several of these files also contain the noun sense that must survive untouched.
  In `internal/loomengine/config.go`, the comment about the run lock reading as free so that "a second `lyx loom run`" spawns a second driver names the bootstrap.
  In `internal/loomshed/seed.go`, the comment stating that the malformed-JSON shape made `lyx loom run` refuse on the envelope names the bootstrap;
  the adjacent sentence saying `lyx loom drive` was already correct because it never calls `Seed` names the foreground verb and becomes `lyx loom run`, which makes this file a two-sided edit that must be read as a pair rather than substituted.
  In `internal/frictionengine/spec.go`, the comment stating that `lyx loom run` is by definition the unattended path names the bootstrap.
  In `internal/webstercli/wiring.go`, the comment about a directive under `lyx loom run`, where Master drives the batch loop, names the bootstrap;
  the separate comment stating that a standalone webster run is not a loom run uses "run" as a noun twice and must be left exactly as it is.
  In `internal/websterengine/strand.go`, the comment describing the log "the one `lyx loom run` points an operator at" names the bootstrap, since it is the bootstrap that reports the detached driver's log path.
- **Commit:** `docs(engines): name lyx loom start in the bootstrap-sense comments`

### Card 11: contrast prose rewritten as sentences

- **Context:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/run.go`
- **Edits:**
  - `internal/loomengine/seed.go`
  - `internal/shuttleengine/attach.go`
  - `internal/friction/friction.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Every hit in this card is prose whose meaning turns on the two verbs being different, so each must be rewritten as a sentence and never token-substituted — a mechanical swap collapses both sides onto one name and makes the sentence name one verb twice.
  In `internal/loomengine/seed.go`, the comment about a poisoned status file that made both `"lyx loom run"` and `"lyx loom drive"` refuse on the envelope must end up naming `"lyx loom start"` and `"lyx loom run"`, with the "both" still reading correctly as two distinct verbs.
  In `internal/shuttleengine/attach.go`, the comment stating that an absent `reed.json` is recreated in-band by `"lyx reed up"`, or simply by `"lyx loom run"` and `"lyx loom drive"`, which both call `reed.Up()` themselves, becomes `"lyx loom start"` and `"lyx loom run"`;
  the "both" is the whole point of that sentence.
  The later sentence in the same file naming `"lyx loom drive"` over a truncated `reed.json` in a crucible-round finding becomes `"lyx loom run"`.
  `"lyx reed up"` is a different module's verb and is unchanged.
  In `internal/friction/friction.go`, the comment stating that a failure must never fail `lyx loom run` or `lyx loom drive` becomes `lyx loom start` or `lyx loom run`, again keeping the two sides distinct.
  Change no code in these files.
- **Commit:** `docs(engines): rewrite the two-verb contrast comments for the new names`

### Card 12: test comments and fixtures

- **Context:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/run.go`
  - `plugins/ly/skills/INDEX.md`
- **Edits:**
  - `internal/loomengine/seedownership_test.go`
  - `internal/loomshed/seed_test.go`
  - `internal/shedadapters/bouncer_seed_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomengine/seedownership_test.go`, the reproduction comment stating that a poisoned status file made both `"lyx loom run"` and `"lyx loom drive"` refuse on the envelope is the same two-verb contrast card 11 handles in the non-test files: rewrite it as a sentence naming `"lyx loom start"` and `"lyx loom run"`, keeping both sides distinct.
  In `internal/loomshed/seed_test.go`, the two comments describing how escalating the decode failure made `lyx loom run` refuse on the envelope both name the bootstrap and become `lyx loom start`.
  In `internal/shedadapters/bouncer_seed_test.go`, the comment referring to what "the ly-supervise skill tells operators cannot happen" names the skill and becomes `ly-drive`, matching the rename batch 4 performs;
  this is a retrospective record describing the tree as it stands, so it is rewritten outright rather than glossed.
  These are comment-only edits in untagged Tier 1 suites — change no assertion, no fixture value, and no test name, and add no new test.
- **Commit:** `docs(tests): rewrite the verb and skill names in engine test comments`

## Batch Tests

`verify:` runs the eight packages this batch edits.
Every change is comment text, so no assertion changes behaviour;
the command's job is to prove the batch broke nothing, and running the owning packages is the cheapest honest way to do that.
The whole set completes in roughly two seconds on this tree, well within the per-batch scoping default — no unbounded whole-suite run is needed or justified.

The repo-wide regression surface for a comment sweep is covered separately by `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) at task completion.
