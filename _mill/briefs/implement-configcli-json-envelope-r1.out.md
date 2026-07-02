All 5 cards (Card 7–11) of batch `configcli-json-envelope` are implemented, committed, and pushed. Verify command `go test -tags integration ./internal/configcli/...` passes. Working tree is clean.

Key files touched:
- `C:\Code\loomyard\wts\config-set-key-loss\internal\configcli\configcli.go` — `setModule` and `editOne` now emit `output.Ok` JSON envelopes; `setModule`'s envelope carries `"preserved"` only when non-empty; `--set` help text documents preserve-and-warn + `reconcile`.
- `C:\Code\loomyard\wts\config-set-key-loss\internal\configcli\configcli_test.go` — new `assertJSONOkContains` helper, updated existing success-path tests, and three new tests (`TestDispatchSet_PreservesUnrecognizedKeyReportsWarning`, `TestDispatchSet_CleanFileNoPreservedField`, `TestDispatchSet_PreservedKeyDetectedByReconcile`).
- `C:\Code\loomyard\wts\config-set-key-loss\internal\configcli\configcli_integration_test.go` — `TestE2ESyncIntegration` now also asserts the JSON envelope shape (`ok:true`, `module:"warp"`).

Commits (in order): `9cd458d` (Cards 7+8 combined, same file), `d172ef4` (Card 9), `92f512b` (Card 10), `c163c14` (Card 11).

{"status":"success","commit_sha":"c163c144383e80ddee81f45f66c6ab433cf0818c","session_id":"f7bed8de-6e4a-4f50-8170-68ea2f954f4e"}

{"status":"success","commit_sha":"c163c144383e80ddee81f45f66c6ab433cf0818c","session_id":"f7bed8de-6e4a-4f50-8170-68ea2f954f4e"}
