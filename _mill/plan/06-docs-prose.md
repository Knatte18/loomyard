# Batch: docs-prose

```yaml
task: "Rename hub container suffix from -HUB to -LYXHUB"
batch: "docs-prose"
number: 6
cards: 4
verify: go test ./internal/lyxcwd/...
depends-on: [1, 5]
```

## Batch Scope

The five documents under `docs/` and `manifest/` that name the hub container suffix: the canonical hub-path document, the sandbox how-to, the overview's sandbox paragraph, the shared-library page listing the policed geometry tokens, and one design document whose measured public-surface census quotes the constant's value.
It also adds the one piece of operator-facing procedure this task owes — how to dispose of a container that still carries the retired suffix, which is not self-evident from the code and is the clean break's single sharp edge.

It is one batch because all five are prose read by the same audience, and because the procedure and the substitutions have to be written against each other: the procedure only makes sense next to the path it replaces.
It depends on batch 5 as well as batch 1, because the sandbox documents quote the fixture container name that batch 5 renames, and the two must agree.

Batch-local decision beyond `## Shared Decisions`: the migration procedure lands here and in the constraints file only.
See `### Decision: Migration prose placement` in the overview.

## Cards

### Card 18: Update the canonical hub-path document and record the disposal procedure

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/reedengine/server.go`
  - `tools/sandbox/main.go`
- **Edits:**
  - `docs/sandbox-hub.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Substitute `-LYXHUB` for `-HUB` at all seven hits, each of which names the fixture container directory:
  the sentence under `## Hub Location and Structure` giving the Windows and POSIX clone destinations and spelling out how the name is composed from the warp basename plus the suffix;
  the directory-tree fence immediately below it;
  the first-build step that computes the hub path;
  the reset step that removes the existing hub directory;
  and the three later steps that locate the hub warp repository, including the one naming the git exclude file the suite writes into.
  The composed-name sentence also names the derivation helper it credits — leave that credit as written and change only the suffix.

  Then add a new subsection at the end of `## Hub Location and Structure`, after the directory-tree fence and before `## Prerequisites`.
  Title it so it reads as operator procedure rather than as background — for example `### Disposing of a pre-rename container`.
  It must record, in substance:
  - A container created before the rename still works.
    Discovery is structural, so it resolves normally;
    the only degradation is the display name derived from its basename.
  - It is never renamed in place.
    The portal links and launcher scripts inside it were materialised against its absolute path, and the tmux socket key is derived from that same absolute path, so moving the directory breaks a working container rather than migrating it.
  - The reset flag does not reach it.
    Both the collision guard and the reset teardown key on the container path the code derives, which now carries the new suffix, so a re-clone neither refuses nor removes the old directory — it silently creates a second, parallel container for the same warp, with its own board, its own links, and its own tmux socket, while the old one keeps running.
  - The procedure is therefore, in order: push or abandon any outstanding work inside the old container, stop its reed server with `lyx reed down`, remove the old directory by hand, then run `lyx fabric clone` to create the new one.
    State plainly that the reset flag is not the tool for this.

  Confirm each claim before writing it: the collision guard and reset teardown in `internal/fabricengine/clone.go`, the suffix the container path is derived with in `internal/fabricengine/junctionnames.go`, and the socket-key derivation in `internal/reedengine/server.go`.
  Read `tools/sandbox/main.go` to confirm the fixture container name this document quotes matches the constant batch 5 renamed.

  Follow the repository's semantic-line-break markdown rule throughout: one sentence per line, breaking inside a long sentence only at an internal independent-clause boundary, with plain newlines.
- **Commit:** `docs(sandbox): rename the hub container to -LYXHUB and record disposal`

### Card 19: Update the sandbox how-to and the overview

- **Context:**
  - `tools/sandbox/main.go`
- **Edits:**
  - `docs/sandbox-howto.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  One substitution in each file, both naming the fixture container directory by its full path.

  - `docs/sandbox-howto.md` — the first-time clone instruction giving the destination path.
  - `docs/overview.md` — the sandbox paragraph stating where the fixture lives on disk and which binary it exercises.

  Neither file gains any migration prose;
  the disposal procedure is recorded once, in the canonical hub-path document card 18 edits.
  Keep the surrounding sentences byte-identical apart from the suffix, and do not re-wrap the paragraphs.
- **Commit:** `docs(sandbox): update how-to and overview to -LYXHUB`

### Card 20: Update the policed-token list

- **Context:**
  - `internal/lyxcwd/enforcement_test.go`
- **Edits:**
  - `docs/shared-libs/lyxcwd.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The paragraph describing `TestEnforcement_GeometryLiterals` enumerates the policed geometry path tokens as an inline list.
  Replace the retired token in that list with `-LYXHUB`, keeping the list's order and the rest of the sentence — including the dual-owner parenthetical — exactly as written.

  Read `internal/lyxcwd/enforcement_test.go` and confirm the rendered list matches the `geometryToken` switch case list there token for token before committing.
  This document is a live description of the current test, so it is updated with the test rather than left as a record of what the test once policed.
- **Commit:** `docs(lyxcwd): update the policed geometry token list to -LYXHUB`

### Card 21: Update the measured public-surface census

- **Context:**
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The sentence listing the measured hub-layout identifiers on the package's public surface quotes each constant's present value, and quotes `HubSuffix` with the retired one.
  Change that quoted value to `"-LYXHUB"`, leaving every identifier name and every other quoted value untouched.

  This is a live census — its claim is about the code as it stands, so leaving it would make the document false.
  Confirm the new value against `internal/fabricengine/junctionnames.go` before writing it.
- **Commit:** `docs(manifest): update the measured HubSuffix value to -LYXHUB`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` is the right scope for a documentation batch in this repository, because both repo-wide markdown gates live in that package's untagged tier.

`TestEnforcement_MarkdownLinks` in `internal/lyxcwd/docslink_test.go` walks every markdown file under `docs/` and `manifest/` and resolves each inline link's file part and heading anchor.
That is the direct guard for this batch: four of the five edited files sit under those two roots, and card 18 adds a new heading, which is exactly the kind of edit that can strand an anchor link elsewhere in the tree.

`TestEnforcement_GeometryLiterals` in the same package runs alongside it and re-confirms, at this batch's boundary, that no production file picked up a stray literal while the documentation was being rewritten.

The fifth edited file is also under `docs/`, so the whole batch is covered.
The prose itself — whether the disposal procedure is correct and complete — is reviewed rather than tested;
there is no runnable surface for operator procedure.
