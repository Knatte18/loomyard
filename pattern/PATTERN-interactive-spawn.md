# PATTERN-interactive-spawn

Every agent lym spawns runs as an interactive session in a tmux pane, placed by reed and driven by shuttle, never as a headless `claude -p` or `claude --print` process.
Interactive sessions keep subscription coverage, while headless use is moving to API billing, so the reason is economic rather than technical.

- The scan in `cmd/lym/interactivespawn_test.go` parses every production Go file of the module, `tools/` included, and reports three shapes:
  a call that takes a `-p`, `--print` or `--print=…` string literal and another argument naming claude, which is a string literal whose base name is `claude` or `claude.exe`, or an identifier, selector or call whose name holds `claude` in any case;
  a string literal whose whitespace tokens, quote characters trimmed, name claude and then carry a print flag;
  and any string literal under `internal/shuttleengine/claudeengine` carrying a print-flag token, since that engine builds claude's launch lines from flag fragments.
- Comments are never read, so documentation naming the forbidden form passes.
- The scan's allowlist is keyed by file and admits every finding in that file, with a reason naming why the flag there launches no agent.
- Bound: the scan sees static shapes only, so a flag built at runtime, or a launch line assembled from fragments outside the claude engine's directory, passes.
- Bound: test files are not scanned, so a test may hold the forbidden text as data.
