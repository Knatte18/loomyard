Both cards (18 and 19) committed, matching the batch's two declared cards exactly: 2 of 2 cards committed. All complete.

I completed both cards in this batch (`05-docs.md`):

- **Card 18** - extended `internal/reedengine/doc.go`'s package doc comment (`Load-bearing behavioral assumptions` list) with 9 new bullets covering the dot-fill mechanism, the stale-paint/uncovered split, the `window-size latest` rejection rationale, the actual repaint-entry disposition (none shipped, per the batch-2 measurement gate), the two acceptance criteria, the would-be repaint entry's independence from `watchdog` with the verified call-site map, the Windows story, the attach-time multi-client warning, and the `requiredSubcommands` exemption. I verified every factual claim (call sites, guard ordering, `SkipFocus` early return) directly against `attach.go`, `windowsize.go`, `apply.go`, `lifecycle.go`, and `probe.go` before writing it. Item 10 (candidate 1's shell-mechanics detail) was correctly omitted since no candidate was accepted and `internal/shell` has no `ForEachLine`/`LineVarRef`.
  Commit: `3c2b7033b`

- **Card 19** - updated the `### W5` scenario in `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md` with the confirmed mechanism, what actually shipped (no repaint entry, so the watchdog round-trip smear rather than a flicker), explicit cross-client repro steps, and the OK-not-FAIL disposition for the uncovered-real-estate case, following the repo's semantic-line-break markdown convention.
  Commit: `dfcca2e54`

Verify (`go test -count=1 ./internal/reedengine/ ./cmd/lyx/`) passed on the first attempt. Working tree is clean except the untracked brief file itself.

2 of 2 cards committed - all complete.

{"status":"success","commit_sha":"dfcca2e546f55c2338a7fb4dbd0d38ef7af15970","session_id":"480eccac-ca22-45bd-a2da-39ab0c2b1fa9","cards_done":[18,19]}
