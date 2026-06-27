{"status":"success","commit_sha":"07789e246f69acc41ca7f0c48aa31030f891760f","session_id":"06d0612f-4eaf-4399-80b3-c3a828e37455"}

All three cards implemented and verified:

- Card 9 (internal/ide/cli.go): Added Command() with PersistentPreRunE resolving cwd+layout into a closure var; spawn RunE uses args[0] for slug (cobra-stripped); menu RunE closes over l; no-arg listing requires no git repo; RunCLI delegates to clihelp.Execute.

- Card 10 (internal/weft/cli.go): Added Command() with hidden persistent --weft-path flag; PersistentPreRunE enters bypass mode when flag is set (non-push subcommands -> output.Err + clihelp.Abort), otherwise resolves cwd->layout->config->pathspec into closure vars; push RunE branches on bypass bool; all other subcommands close over resolved vars; RunCLI delegates to clihelp.Execute.

- Card 11 (internal/ide/cli_test.go, internal/weft/cli_test.go): Updated no-arg assertions to expect exit 0 + subcommand listing; updated unknown-subcommand assertions to check "unknown command" substring; TestRunCLI_WeftPathPushOnly JSON envelope assertion preserved verbatim; all pre-existing behavior tests left intact.

{"status":"success","commit_sha":"07789e246f69acc41ca7f0c48aa31030f891760f","session_id":"06d0612f-4eaf-4399-80b3-c3a828e37455"}
