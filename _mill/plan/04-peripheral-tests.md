# Batch: peripheral-tests

```yaml
task: "Rename hub container suffix from -HUB to -LYXHUB"
batch: "peripheral-tests"
number: 4
cards: 6
verify: go test ./cmd/lyx/... ./internal/hubgeom/... ./internal/logger/... ./internal/loomengine/... ./internal/lyxcwd/... ./internal/tokenvocab/... ./internal/weftname/... ./internal/reedengine/... && go test -tags integration -run TestResolve_FromWorktreeRoot ./internal/lyxcwd/... && go vet -tags smoke ./internal/reedcli/...
depends-on: [1]
```

## Batch Scope

The suffix sweep across every package outside `internal/fabricengine`: sixteen test files in nine packages, almost all of them synthetic hub-path fixtures that never touch disk.
Three sites need judgement rather than substitution — the reed socket-key boundary cases, whose intent must be re-checked against a four-byte-longer suffix;
one conceptual-prose comment that means the hub but not the suffix, and is reworded rather than substituted;
and the two recorded historical captures elsewhere in the repository, which this batch deliberately does not touch.

It is one batch because every card is the same mechanical judgement applied package by package, and because the three exceptions are easier to get right when read against the mechanical majority rather than in isolation.
It depends on batch 1 only, and shares no file with any other batch.

Batch-local decision beyond `## Shared Decisions`: the smoke-tagged file is verified by `go vet -tags smoke` rather than by running the smoke suite.
See `## Batch Tests`.

## Cards

### Card 10: Sweep `cmd/lyx`, `internal/hubgeom`, and `internal/logger` fixtures

- **Context:**
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/notransients_test.go`
  - `internal/hubgeom/hubgeom_test.go`
  - `internal/hubgeom/webstergeom_test.go`
  - `internal/logger/logsdir_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Every hit in these five files is a synthetic hub-container path fixture standing for the real suffix.
  Substitute `-LYXHUB` for `-HUB` in each, changing nothing else.

  - `cmd/lyx/constructoranchoring_test.go` — two `hub := filepath.Join("home", "user", "repo-HUB")` fixtures.
  - `cmd/lyx/notransients_test.go` — one fixture of the same shape.
  - `internal/hubgeom/hubgeom_test.go` — two `hub := filepath.Join(root, "some-hub-HUB")` fixtures.
  - `internal/hubgeom/webstergeom_test.go` — one fixture of the same shape.
  - `internal/logger/logsdir_test.go` — two location literals whose hub field is `filepath.Join("home", "user", "repo-HUB")`.

  These fixtures are string-only and must stay that way: none of them may be turned into a directory that is created on disk.
- **Commit:** `test(hub): sweep cmd/lyx, hubgeom, and logger fixtures to -LYXHUB`

### Card 11: Sweep `internal/loomengine` fixtures

- **Context:**
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/loomengine/config_test.go`
  - `internal/loomengine/discussionpath_test.go`
  - `internal/loomengine/friction_test.go`
  - `internal/loomengine/loomstatus_test.go`
  - `internal/loomengine/review_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  All twenty-one hits across these five files have the identical shape — a location literal whose hub field is `filepath.Join("home", "user", "repo-HUB")` — and every one stands for the real suffix.
  Substitute `-LYXHUB` for `-HUB` in each.

  The counts to expect, so a missed line is visible: three in `internal/loomengine/config_test.go`, five in `internal/loomengine/discussionpath_test.go`, two in `internal/loomengine/friction_test.go`, ten in `internal/loomengine/loomstatus_test.go`, one in `internal/loomengine/review_test.go`.
  Change no assertion, no expected path built from the fixture, and no test name — the fixtures feed path derivations that carry the container basename through unchanged, so the existing expectations follow automatically.
- **Commit:** `test(loomengine): sweep hub-path fixtures to -LYXHUB`

### Card 12: Sweep `internal/lyxcwd` geometry and resolution tests

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/lyxcwd/geometry_test.go`
  - `internal/lyxcwd/lyxcwd_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/lyxcwd/geometry_test.go`, one table case carries the container path twice — once as the `hub` input `"/repos/loomyard-HUB"`, and once as the first argument of the `filepath.Join` expression that builds its `want` value.
  Both are the real suffix as a synthetic fixture;
  substitute `-LYXHUB` in both.

  In `internal/lyxcwd/lyxcwd_test.go`, the only hit is inside the comment above the `RepoName` assertion, which explains that the fixture's container carries no hub suffix so nothing is trimmed.
  Substitute the new suffix in that comment.
  The assertion line below it derives its expectation through `fabricengine.HubSuffix` and needs no edit — it updated itself when the constant moved.

  Add no new case to either file.
  The retired-suffix degradation is already pinned by the untagged trimming test batch 1 creates, so repeating it in an integration-tagged file would only duplicate coverage.
- **Commit:** `test(lyxcwd): sweep geometry and resolution fixtures to -LYXHUB`

### Card 13: Sweep `internal/tokenvocab` and `internal/weftname`

- **Context:**
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/tokenvocab/tokenvocab_test.go`
  - `internal/weftname/weftname_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/tokenvocab/tokenvocab_test.go`, nine of the ten hits are the container path `"/hub/loomyard-HUB"` appearing as a `HubPath` field on a context literal, as a `want` string, or inside a `want` map — every one a synthetic fixture standing for the real suffix.
  Substitute `-LYXHUB` in all nine.
  The tenth hit is the comment explaining that the display token renders whatever `RepoName` holds regardless of how it was derived, and which names the trim derivation by its suffix;
  substitute the new suffix there too, since it names the derivation as it behaves today.

  In `internal/weftname/weftname_test.go`, the single `nested_container` table case carries the container path twice, as the input `"/repos/loomyard-HUB"` and inside the `want` expression;
  substitute `-LYXHUB` in both.

  No assertion changes: these tests carry the hub path through as an opaque string and never trim it.
- **Commit:** `test(tokenvocab): sweep hub-path fixtures to -LYXHUB`

### Card 14: Sweep and re-check the reed socket-key boundary cases

- **Context:**
  - `internal/reedengine/server.go`
- **Edits:**
  - `internal/reedengine/server_test.go`
  - `internal/reedengine/state_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Substitute `-LYXHUB` for `-HUB` at every one of the ten hits across these two files — the hub-container fixtures in `TestServerName_Deterministic`, `TestServerName_SocketSafe`, `TestServerName_SocketSafeForAHubAtTheFilesystemRoot`, `TestServerName_BoundedForALongHubBasename`, `TestServerName_DistinctForDistinctHubsSharingBasename`, and `TestServerName_HasHubBasenameAndPrefix`;
  the two socket-key strings in `TestValidateToldTmuxIdentity_SocketKey`'s table;
  and the `Socket` field fixture in `internal/reedengine/state_test.go`.
  The expected-prefix string in `TestServerName_HasHubBasenameAndPrefix` must move with its fixture, so the prefix it asserts still matches the container basename it derives from.

  Then re-check, rather than assume, that two cases still test what they were written to test, and record the conclusion as a short comment beside each if it is not already evident:
  - The long-basename case in `TestServerName_BoundedForALongHubBasename` builds its basename by repeating a character two hundred times and appending the suffix.
    The bound it asserts is computed from `maxSocketSafeBaseBytes`, so a four-byte-longer suffix changes nothing about the assertion — the input was already far past the cap and still is.
    Confirm this against `internal/reedengine/server.go` and leave the assertion as written.
  - The socket-key table cases assert that a derived hub-mode key and a key containing dots are accepted.
    Neither is length-sensitive, so both survive the substitution unchanged.

  Change no assertion shape and add no new case: the cap exists precisely to absorb long container names, and the hash half preserves identity, so the four extra bytes need no new coverage.
- **Commit:** `test(reedengine): sweep socket-key fixtures to -LYXHUB`

### Card 15: Reword the conceptual per-hub comment in `internal/reedcli`

- **Context:**
  - `internal/reedengine/server.go`
- **Edits:**
  - `internal/reedcli/smoke_teardown_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The doc comment on `TestSmokeDownInOneWorktreeLeavesSiblingSessionAlive` contains the phrase "the tmux server identity is per-HUB (the -L socket derives from the hub)".
  This names the hub as a concept — one tmux server per hub, shared by sibling worktrees — not the container directory's name suffix, so substituting the new suffix there would produce nonsense.

  Reword it to lowercase "per-hub", leaving the rest of the sentence and the whole of the surrounding comment untouched.
  The result must read "the tmux server identity is per-hub (the -L socket derives from the hub)".

  This is the only hit in the file and the only conceptual-prose case in the repository.
  Change nothing executable — this file is smoke-tagged and its assertions are out of scope.
- **Commit:** `test(reedcli): reword per-HUB comment to per-hub`

## Batch Tests

`verify` is three chained invocations, covering three tiers.

`go test ./cmd/lyx/... ./internal/hubgeom/... ./internal/logger/... ./internal/loomengine/... ./internal/lyxcwd/... ./internal/tokenvocab/... ./internal/weftname/... ./internal/reedengine/...` runs the untagged tier of every package this batch edits.
That is where thirteen of the sixteen edited files live, and where every assertion that consumes a swept fixture runs.
`internal/lyxcwd` is in the list twice over: for `internal/lyxcwd/geometry_test.go`, and because `TestEnforcement_GeometryLiterals` walks the whole repository source tree and is the standing guard that no production file picked up a stray literal.

`go test -tags integration -run TestResolve_FromWorktreeRoot ./internal/lyxcwd/...` covers `internal/lyxcwd/lyxcwd_test.go`, which is integration-tagged.
Building the tagged binary compiles the file, and the filter runs the one test whose comment card 12 edits and whose `RepoName` assertion derives through the constant.
The rest of the tagged suite is unchanged by this batch and is covered task-wide by `pipeline.done_gate`, configured as `go test ./... && go test -tags integration ./...`.

`go vet -tags smoke ./internal/reedcli/...` covers `internal/reedcli/smoke_teardown_test.go`, which is smoke-tagged.
Card 15's change there is a single word inside a doc comment, so compilation under the smoke tag is the entire risk surface, and `go vet` proves it.
Running the smoke suite itself is deliberately not done here: it drives real tmux servers across multiple worktrees and takes minutes, it is not part of `pipeline.done_gate` either, and nothing this batch changes could affect its outcome.

No new coverage is owed by this batch.
Every card is a vocabulary substitution over synthetic fixtures or comments;
the one behavioural question the suffix raises — the socket-key length boundary — is answered in card 14 by re-checking the existing cases rather than by adding one, because `maxSocketSafeBaseBytes` already absorbs inputs far longer than either suffix.
