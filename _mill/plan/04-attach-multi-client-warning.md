# Batch: attach-multi-client-warning

```yaml
task: "Reed attach dot-fill render artifact on resize and cross-client mouse move"
batch: "attach-multi-client-warning"
number: 4
cards: 3
verify: go test -count=1 ./internal/reedengine/
depends-on: []
```

## Batch Scope

This batch adds reed's first production client-awareness: `AttachArgv`'s pre-flight lists the session's currently attached clients and emits one `logger.Warn` per client whose size differs from the size this attach was told.
It is the primary deliverable for the cross-client trigger, which the `root-cause-model` decision places in the **uncovered** subset — dots no repaint mechanism can remove, because tmux is correctly padding real estate with nothing behind it.
The warning does not remove the artifact;
it removes the bewilderment, which is the residual's actual operator cost.

It is one batch because the pure parser, the engine method, and the `AttachArgv` wiring are one feature over two files, and it depends on nothing else in this plan: it touches neither the hook array nor the smoke suite.

Batch-local decisions beyond `## Shared Decisions`:

- The warning **never** blocks, **never** changes the argv, and **never** reaches the JSON envelope.
  `attach`'s envelope is reserved for pre-flight aborts, and this is not an abort;
  printing to the operator's terminal is useless because stdio is about to be handed to tmux and anything printed is immediately overwritten.
- Cardinality is **one line per differing client**, not one aggregate line, and no line at all for a client whose size matches.
  The line's whole job is to name the specific other terminal the operator should go look at, which an aggregate count would drop.
- The pure parser lives in `internal/reedengine/attach.go` beside its only consumer rather than in `internal/reedengine/windowsize.go`, whose stated subject is window size and the hook array.

## Cards

### Card 15: the client-list parser

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/overlay.go`
- **Edits:**
  - `internal/reedengine/attach.go`
  - `internal/reedengine/attach_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/reedengine/attach.go`:

  ```go
  type attachedClient struct {
  	Name   string
  	Width  int
  	Height int
  }

  func parseClientList(out string) []attachedClient
  ```

  `parseClientList` parses a `list-clients -F '#{client_name} #{client_width} #{client_height}'` answer, one client per line.
  It performs no I/O and no logging.
  Follow `parseWindowSize`'s strictness discipline per line: a line yields a client only when it has exactly three whitespace-separated fields and both size fields parse as strictly positive integers.
  Every other line shape — blank, one field, two fields, four or more, non-numeric, zero, negative — is skipped rather than reported, because one malformed line among several must not discard the well-formed clients beside it.
  Trim the whole answer before splitting so a trailing newline yields no phantom entry.
  It returns an empty slice, never nil-versus-empty ambiguity, when no client parses.

  Add a table test in `internal/reedengine/attach_test.go` mirroring `TestParseWindowSize`'s shape, covering: the empty answer, one client, several clients, a malformed line among well-formed ones, trailing whitespace, a zero size field, and a negative size field.
- **Commit:** `feat(reed): parse tmux list-clients answers into name/size triples`

### Card 16: the attach-time mismatch warning

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/probe.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/attachgeometry_integration_test.go`
- **Edits:**
  - `internal/reedengine/attach.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/reedengine/attach.go`:

  ```go
  func (e *Engine) warnMismatchedClientsLocked(cols, rows int)
  ```

  It issues `e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()), "-F", "#{client_name} #{client_width} #{client_height}")`.
  The target is `exactSessionTarget` — the bare `=<name>` form — because `list-clients -t` takes a session target;
  `exactSessionWindowTarget`'s trailing colon exists solely for tmux's window/pane parsers, and the same form is used by the repaint entry's own enumeration and by `internal/reedengine/attachgeometry_integration_test.go`'s existing client-attach poll.
  The `=` prefix is what stops tmux prefix-matching a sibling worktree's session on the shared per-hub server.

  On a round-trip error it logs one `logger.Warn` naming the socket, the session, and the error, and returns.
  It never returns an error and never introduces a degrade path — a failed listing warns and continues, exactly as the geometry pins do.

  Otherwise it feeds the answer to `parseClientList` and, for each parsed client whose `Width != cols` or `Height != rows`, emits exactly one `logger.Warn` naming the client, its size, and the size this attach was told.
  A client whose size matches produces no line, so the common single-client case stays silent.
  The message must say plainly what the operator will see: that another client is attached at a different size, that tmux must pick one window size, and that the mismatched client shows tmux padding until it is resized or detached.

  Wire the call into `AttachArgv`'s existing `withOpLock` closure, immediately after `e.requireSessionLocked()` and before `e.pinGeometryOptionsLocked()`.
  That position is deliberate and is worth its own sentence in the doc comment: it is before any mutation, and it is ahead of every in-closure degrade return, so the warning still fires on an attach whose chain is later suppressed — which is precisely the attach an operator is most likely to be confused by.
  Add no new early return, no new error path, and no change to either returned argv.

  Add nothing to `requiredSubcommands` in `internal/reedengine/probe.go`: a failing or unsupported `list-clients` logs one warning and emits no multi-client warning, and the argv is unaffected.
- **Commit:** `feat(reed): warn at attach time when a differently-sized client is already attached`

### Card 17: AttachArgv coverage for the warning

- **Context:**
  - `internal/reedengine/attach.go`
  - `internal/reedengine/logcapture_test.go`
  - `internal/reedengine/windowsize.go`
- **Edits:**
  - `internal/reedengine/attach_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend the `attachScript` fixture struct with `listClients string` and `listClientsErr error`, and add a `case "list-clients":` branch to `newAttachHook` that records the call into the recorder and answers from those fields.
  Answering from the existing `default:` branch would leave the new call invisible to the sequence assertions, so give it its own branch.

  Add tests using `captureLogOutput` to assert on the emitted warnings:
  - a same-size existing client produces no warning line;
  - a different-size existing client produces exactly one warning line naming that client and both sizes;
  - three attached clients of which two differ produce exactly two lines, one naming each differing client, and none for the matching one;
  - a `list-clients` error produces a warning and no behaviour change.

  In every one of those four cases assert the returned argv is byte-identical to what the same script produces today — the argv is the contract and the warning is a side effect that must never perturb it.
  Reuse `assertBareArgv` and the `TestAttachArgv_ChainedShape` expectations rather than re-deriving them.

  Add one further case: a script whose `windowSize` suppresses the chain still emits the warning, proving the call sits ahead of the in-closure degrade returns.

  Confirm the existing tests in this file still pass unchanged in shape — `TestAttachArgv_NeverMutatesTheSessionOrPersistsState` in particular must keep seeing no mutation, since `list-clients` is a read-only query and belongs in neither `mutationCalls` nor `setOptionCalls`.
- **Commit:** `test(reed): cover the attach-time multi-client warning's cardinality and argv invariance`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/reedengine/`, the package both edited files live in.
It covers the new `parseClientList` table test and every `AttachArgv` case in `internal/reedengine/attach_test.go`, including the pre-existing argv, guard, and hook-install tests this batch must leave passing unchanged.

The package is named explicitly rather than running `./...`: nothing outside `internal/reedengine` reads `parseClientList` or `warnMismatchedClientsLocked`, and the whole-repo regression is covered by the `pipeline.done_gate` run at task end.
No smoke tag is needed — both edited files are untagged, and the warning's observable surface is a `logger` line a Tier 1 test captures directly via `captureLogOutput`.
