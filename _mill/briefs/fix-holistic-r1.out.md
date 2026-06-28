All two NIT findings were fixed, committed, and all verify commands passed (exit 0 across all 5 non-null batch verifies).

{"status":"success","commit_sha":"567fddd27578844b47dfff2f0c48c26308ad90e6","session_id":"31c08e36-9d53-4407-8863-b21da22c8bdb"}

Files changed:
- internal/muxpoc/cli_test.go — TestRunCLIUnknownFlagFails now parses the JSON envelope and asserts ok=false plus "unknown flag" in the error field.
- internal/weft/cli_test.go — corrected the backwards execution-order comment in TestRunCLI_UnknownSubcommand.

{"status":"success","commit_sha":"567fddd27578844b47dfff2f0c48c26308ad90e6","session_id":"31c08e36-9d53-4407-8863-b21da22c8bdb"}
