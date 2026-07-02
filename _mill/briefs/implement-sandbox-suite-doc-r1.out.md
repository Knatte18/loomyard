No tracked modifications outstanding. All three cards are committed and pushed, and `go test ./tools/sandbox/...` passes.

Summary of work in this batch (`01-sandbox-suite-doc.md`):
- `C:\Code\loomyard\wts\sandbox-suite-expand\tools\sandbox\SANDBOX-SUITE.md`
  - Card 1 (commit `b5ae456`): renamed S6 → S5, extended the `ref` range and Session-log-format list to S0–S8.
  - Card 2 (commit `7367640`): added S6 (Subfolder init), S7 (Weft lifecycle), S8 (Warp introspection) scenarios each with a durability note, extended S4's Watch for the sandbox-native `--set`/`--print`/`reconcile` round-trip, and rewrote the Operating-model note to name S6 as the sole nested-`_lyx` exception.
  - Card 3 (commit `03b0221`): added `**Covers:**` tags — S3→board, S4→config, S6→init, S7→weft, S8→warp — leaving S0/S1/S2/S5 untagged.

{"status":"success","commit_sha":"03b0221276b7df4ad31ee9c37688b039e208a115","session_id":"b7b62898-4164-42af-8b65-727af08fa87f"}