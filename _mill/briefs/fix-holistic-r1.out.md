All three findings fixed, all 6 verify commands pass.

Summary of fixes applied:
- Finding 1 (go.mod // indirect): ran `go mod tidy` to move github.com/spf13/cobra and github.com/spf13/pflag from the indirect to direct require block. Committed as eeaedfb.
- Finding 2 (board b == nil guards): removed all 11 dead `if b == nil { return 0 }` guards across every board subcommand. These were unreachable because clihelp.WrapRun already calls ShouldAbort before invoking the inner closure. Committed as 969b167.
- Finding 3 (weft cmd.Flags() vs cmd.InheritedFlags()): switched the --weft-path read in PersistentPreRunE from cmd.Flags().GetString to cmd.InheritedFlags().GetString, with an explanatory comment. Committed as 906007d.

{"status":"success","commit_sha":"906007d74895f314e5a06c4c0f044cd6fe5dc827","session_id":"695ac7e4-c440-43c5-b664-a4ade8563d5e"}
