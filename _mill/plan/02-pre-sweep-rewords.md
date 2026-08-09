# Batch: pre-sweep-rewords

```yaml
task: "Rename the fabric host vocabulary to warp, and name the composite repo Fabric"
batch: "pre-sweep-rewords"
number: 2
cards: 2
verify: go test ./internal/fabricengine/... ./internal/boardengine/... ./internal/configcli/... ./internal/builderengine/... ./tools/sandbox/... ./internal/lyxcwd/...
depends-on: [1]
```

## Batch Scope

Every hand edit that must land **before** `wordswap` is pointed at anything.
Two kinds: the four English verb-sense `host` occurrences that sit inside files batch 3 will sweep (reworded to a synonym so the tool cannot mis-swap them and so the word does not survive in the fabric packages in any sense), and the two fabric-sense occurrences in non-owner **production** files (hand-reworded to a neutral phrase, because those files must never be passed to the tool at all).

This batch is why batch 3's `-skip` set can stay as small as one pattern.
It is also a hard ordering constraint, not a convenience: if `internal/builderengine/spawn.go` reached `wordswap`, its line 9 "the plain host filesystem" is bare machine-sense `host` with a clean token boundary on both sides, so the tool would swap it silently rather than report it — and a bare `warp` token in a non-owner production file fails `internal/lyxcwd/enforcement_test.go:883` immediately.

No identifier changes here, and no behaviour changes.
Every edit in this batch is a comment.

## Cards

### Card 6: reword the four verb-sense hits inside files the sweep will touch

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/coalesce.go`
  - `internal/boardengine/board.go`
  - `tools/sandbox/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the English verb sense of "host" at four sites with a synonym, changing nothing else on those lines.
  These are the only standalone English `host`/`hosts`/`hosting`/`hosted` occurrences in any file batch 3 sweeps, verified by exhaustive grep.

  `internal/fabricengine/coalesce.go` line 1 — the package-level file comment currently opens:

```
// coalesce.go hosts the generic loop-until-clean coalescing primitive coalescePush and fabric's own
```

  Change `hosts` to `holds`.

  `internal/boardengine/board.go` line 23 — currently:

```
// and GitHub wiki rendering (an intermediate idea) requires whichever repo hosts the wiki to be
```

  Change `hosts` to `holds`.

  `internal/boardengine/board.go` line 26 — currently:

```
// wiki-hosting repo would have had to go public just to render board's front page.
```

  Change `wiki-hosting` to `wiki-serving`.

  `tools/sandbox/main.go` line 32 — currently:

```
	// lyx-test-HUB above -- the dedicated hub hosts fabric's stricter
```

  Change `hosts` to `carries`.

  Do NOT touch the fabric-sense occurrences in these same files — `board.go:15` ("a host branch"), `:17` ("the host's own default branch") and `:25` ("the host/warp repo", "the host repo's") stay as they are and are renamed mechanically by batch 3.
  Leaving them is what keeps this card a pure verb-sense reword.
- **Commit:** `docs(fabric): reword verb-sense "host" ahead of the host->warp sweep`

### Card 7: hand-reword the two fabric-sense hits in non-owner production files

- **Context:**
  - `internal/lyxcwd/enforcement_test.go`
  - `_mill/discussion.md`
  - `internal/buildercli/poll.go`
- **Edits:**
  - `internal/configcli/configcli.go`
  - `internal/builderengine/spawn.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Reword the two fabric-sense comment lines that live in non-owner production files to a **neutral** phrase.
  Neither replacement may contain the token `warp` or `weft` in any case: `internal/lyxcwd/enforcement_test.go`'s `bareVocabularyToken` matches those as bare substrings anywhere in an identifier, string literal or comment, and `failsBareVocabularyCheck` fails any non-owner directory that carries one.
  "Fabric" is safe — the bare-token rule covers `weft` and `warp` only.

  `internal/configcli/configcli.go` line 269 — currently:

```
	// Compute baseDir as the host _lyx parent: the worktree root joined with the relative path.
```

  Reword to drop the retired qualifier, e.g. `// Compute baseDir as the enclosing _lyx parent: the worktree root joined with the relative path.`

  `internal/builderengine/spawn.go` line 446 — currently:

```
			// The anchor is this HEAD: the host commit immediately before
```

  Reword to drop the retired qualifier, e.g. `// The anchor is this HEAD: the commit immediately before`.

  Change nothing else in `internal/builderengine/spawn.go`.
  Its lines 9 ("the plain host filesystem"), 178 ("a downed reed session hosts no live strand"), 236 ("to the plain host filesystem only") and 277 ("cannot host a live strand") are machine-sense and verb-sense respectively and must survive verbatim — that is why this file is hand-edited end to end rather than swept.

  Do NOT edit `internal/buildercli/poll.go`.
  Its line 321 ("a live pane hosting an idle agent process") is verb-sense in a non-owner production file and is left untouched by this task entirely;
  the file is excluded from batch 3's sweep set for the same reason.
- **Commit:** `docs(cli,builder): reword fabric-sense "host" in non-owner production comments`

## Batch Tests

`verify` runs the test packages that own, or machine-check, the five edited files: `internal/fabricengine`, `internal/boardengine`, `internal/configcli`, `internal/builderengine` and `tools/sandbox` for their own suites, plus `internal/lyxcwd` because `TestEnforcement_FabricVocabulary` lives there and is the check card 7's "no bare `warp`/`weft` in non-owner production" requirement is written against.

Every edit in this batch is inside a comment, so no test asserts on the changed text and all six packages should stay green unchanged.
The value of the gate is the negative case: if card 7's rewording reached for `warp` instead of a neutral phrase, `internal/lyxcwd`'s `TestEnforcement_FabricVocabulary` tree-scan fails here rather than five batches later.

The overview's module-wide `verify: go build ./...` covers the rest of the tree at the batch boundary.
