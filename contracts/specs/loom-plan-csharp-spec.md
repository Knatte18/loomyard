# Loom plan C# spec — a plan under `language: csharp`

> **Status: Contract — pinned.** This doc pins what **plan-format**, defined in `loom-plan-spec.md`, adds for a plan whose overview sets `language: csharp`. The writer sets it when the code the cards change is C#.

## The alphabet

Each ref, and each `plan:` handle's body, is read in the C# glyph alphabet.

- A ref both the C# and the Go alphabet parse is C#.
- A ref only the Go alphabet parses is accepted only as a self glyph.
  A path is spelled as a self glyph in the Go unit alphabet; any other is `glyph-malformed`.
- A member glyph is `<namespace>#<owner.member>`.
  A method's, constructor's or indexer's glyph carries its parameter list: `Glyphchain.Sharp#Renderer.Draw(int)`.
  Copy it from a quarry answer with its list, since a spelling without one names a member that has none.
- A `.cs` file is a plain path such as `src/Renderer.cs`, canonicalized to its file self glyph.
- A slash-free self glyph such as `Glyphchain.Sharp#` names a namespace, never a path.
  A repository-root file or directory named without a slash is therefore not checked on disk:
  spell it as a slashed path or a member.
- A ref several alphabets match is answered ambiguous with the matching languages;
  name the file as a slashed path self glyph, or a member glyph.

## Handles

A handle is `plan:<namespace>#<owner.member>`, and its declaration head is C# source declaring one symbol:
`public void Draw(int width)` for `plan:Glyphchain.Sharp#Renderer.Draw(int)`.
The owner chain names the types that hold the member.
A member declared through an explicit interface writes the interface as a qualifier before its name.
The qualifier is the trailing owner components.
The handle `plan:Glyphchain.Sharp#Renderer.IPair<string,int>.First` takes this head:
`int IPair<string,int>.First { get; }`.
A head spells the qualifier's type arguments canonically:
no space after a comma, and the keyword for a built-in type.
`IPair<string, int>.` is refused with the canonical spelling named.
A body the C# alphabet rejects is `handle-name-failed`.

A `Rename` pair's `New` side is a handle.
Its declaration is derived from the resolved `Old` symbol with the name replaced.
A member no identifier names — an operator, an indexer, a finalizer or a constructor — cannot be renamed this way.

## Re-signs

A re-sign arrow's head is C# source, and:

- declares the same member, with the same namespace, owner chain and name, changing only its parameter list;
- keeps a member with a parameter list one with a list, and one without, without,
  so a change of kind goes through a `Delete` and a `Create`;
- mints no id of an overload that exists or that another card declares (`resign-overload-collision`).

A later card names the re-signed member by its old glyph until the batch that re-signs it is recorded;
webster then rewrites the old glyph to the new one plan-wide.
A later card naming the member by its old glyph therefore runs in a later batch.

## Checks

- `resign-head-mismatch` accepts an arrow when `quarry.Name` answers the glyph's own namespace, owner chain and name.
  The parameter list is free to change.
  Way forward: correct the head to one `Name` accepts under the owner chain.
  Use the canonical explicit-interface qualifier when that is what it refused.
  Keep the parameter list, or change the member's kind through a `Delete` and a `Create`.
  Use a `Rename` pair for a name change.
- `rename-namespace-pair` — a pair of two self glyphs whose old side has no slash and names nothing on disk.
  An example is `Glyphchain.Sharp# -> Glyphchain.Core#`.
  A file pair runs `git mv`, so a namespace cannot be renamed this way.
  A path an earlier card creates is no finding.
  Way forward: rename the namespace through `Edit` cards on the files that declare it and refer to it.
  Or correct the old side to a root file that exists.
  Or move the pair after the card that creates its old side.
- `resign-overload-collision` *(planglyph)* — an accepted arrow whose head mints an id already taken:
  the id of an existing overload, of a `Create` or `Rename` handle, or of another arrow.
  The finding is attributed to the later card, naming the earlier.
  It is lifted when a card between them deletes, renames or re-signs the colliding member away.
  Way forward: change the head to a free signature, or remove or re-sign the other declaration on an earlier card.
- `resign-not-done` — at webster's record-batch and recover-batch only:
  a recorded arrow whose minted id does not resolve in the tree, so the card's edit does not match its arrow.
  Way forward: make the edit match the head.
  Or correct the head to the signature the card wrote and run `lym webster rebaseline --card <N>`.

A `csharp` plan skips the checks that read Go source or the Go toolchain:
`verify-nested-module`, `verify-module-wide`, `verify-package-missing`;
the `.go` file-unit rule of `handle-malformed`;
`delete-before-reference`, `caller-uncovered`, `rename-leaves-test`;
`test-tier-unstated`, `seam-test-unlisted`, `doc-go-unlisted`;
and the Go package-directory mapping of a unit.
