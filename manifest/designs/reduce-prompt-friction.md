# reduce prompt friction — trivial destructive-looking commands shouldn't stall autonomy

> **Status: Speculative, not scoped.** Prompted by `fabric`'s testing hitting frequent
> permission prompts for `rm`-and-similar commands against test/fixture directories. Not yet a
> plan — this file holds the investigation done so far so it isn't re-derived from scratch. Per
> the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), if this is ever
> picked up the durable parts fold into the owning package's doc when it lands; if abandoned,
> this file is simply deleted.

## The problem

Any Bash command that looks destructive (`rm -rf` chief among them) triggers a permission
prompt, even when it's routine test/fixture cleanup rather than a real risk. `fabric`'s testing
— differential warp/weft-vs-fabric fixtures, sandbox test repos — generates a lot of this,
stalling autonomous work. Same root complaint that led to the `sed` rule already added to global
`CLAUDE.md`: a command genuinely useful for agent workflows keeps colliding with a permission
gate meant for actually risky operations.

## Investigation so far (loomyard's own codebase, not fabric's in-progress branch)

Checked whether the codebase under-uses Go's `t.TempDir()` (auto-cleaned, no `rm` ever needed)
in favor of manual `os.MkdirTemp`/`os.RemoveAll`, which was the first suspect:

- **Not generally true** — `t.TempDir()` is already used in 133 files across the repo. The
  pattern is well-established.
- **A handful of real, small inconsistencies exist anyway:**
  - `internal/lyxtest/lyxtest.go:145,191,241` — three functions use raw `os.MkdirTemp`, while
    other functions in the *same file* (435, 470, 536, 588) correctly use `tb.TempDir()`.
  - `internal/warpengine/clone_integration_test.go:63` and
    `internal/weftengine/sync_test.go:172` — one-off manual `os.RemoveAll` in files that
    otherwise consistently use `t.TempDir()`.
  - These are cheap, mechanical fixes (swap to `t.TempDir()`) but are not believed to be the
    main driver of what `fabric` is hitting.
- **Prime suspect: `tools/sandbox/main.go` runs outside `go test` entirely** — it's a
  standalone CLI tool, so `t.TempDir()` (a `testing.T`/`testing.B` method) isn't available to it
  at all, by construction. Its test-repo directories are created and torn down via real
  `os.RemoveAll` (already present as a testability seam, `var removeAll = os.RemoveAll` in
  `tools/sandbox/main.go:50`), which is inherently outside Go's automatic test cleanup.
  `fabric`'s differential tests and its new `SANDBOX-FABRIC-SUITE.md` scenarios likely lean on
  this same sandbox-style fixture model, not just `go test`-internal fixtures — which would
  explain the prompt volume better than the lyxtest inconsistencies above.

## Relationship to `dev-test-binary`

Overlapping surface (`tools/sandbox/main.go`) but a distinct concern: `dev-test-binary` is about
*which binary* the sandbox suite exercises (dev vs. production install). This item is about *how
the sandbox suite's own fixture directories get created and destroyed* — specifically, whether
that cleanup can happen entirely inside the Go tool's own execution (so no external Bash `rm` is
ever needed by the driving agent) rather than requiring a separate shell command after the tool
runs.

## Possible directions (open, not decided)

1. **Mechanical cleanup** of the three known `lyxtest`/test-file inconsistencies — cheap,
   uncontroversial, but not expected to be the main fix.
2. **Make the sandbox suite fully self-cleaning end-to-end** — every sandbox run tears down its
   own fixture directories from within the Go tool itself (already partially true via the
   `removeAll` seam), so an agent driving it never needs to issue its own follow-up `rm` command
   at all. This is the direction most likely to actually remove the prompts, since it removes the
   Bash `rm` invocation rather than trying to make it less scary.
3. **Fallback CLAUDE.md policy note**, analogous to the `sed` rule, for whatever destructive-
   looking cleanup genuinely can't be pushed inside Go tooling (if any remains after direction 2).

## Related

- [dev-test-binary.md](dev-test-binary.md) — overlapping `tools/sandbox/main.go` surface, distinct
  concern (which binary, not how fixtures are torn down).
- `internal/lyxtest/lyxtest.go`, `tools/sandbox/main.go` — the files investigated above.
