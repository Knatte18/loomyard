Working tree is clean. All three cards are committed. Verify passed.

{"status":"success","commit_sha":"8126a9e","session_id":"ac10d92a-849b-4636-92f6-edcef95e8577"}

The batch is complete:

- Card 15 (internal/muxpoc/cli.go): Added Command() with persistent flags on the parent, a PersistentPreRunE that resolves worktree root via paths.Resolve into a closure cfg, six subcommands each with RunE closing over cfg, and RunCLI delegating to clihelp.Execute.

- Card 16 (internal/warp/warp.go): Added Command() with nine subcommands, per-verb local flags using the self-referencing closure pattern, arg-index-shift rebinding, and RunCLI delegating to clihelp.Execute.

- Card 17 (internal/muxpoc/cli_test.go, internal/warp/warp_test.go): Split the shared muxpoc table guard into three separate tests; updated warp's UnknownSubcommand from JSON unmarshal to "unknown command" substring assertion.

{"status":"success","commit_sha":"8126a9e","session_id":"ac10d92a-849b-4636-92f6-edcef95e8577"}
