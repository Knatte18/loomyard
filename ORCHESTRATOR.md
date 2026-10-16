# ORCHESTRATOR.md — orch-only rules for loomyard

These rules add to the orch's role for this repository, and lyx renders the file into the orch's role file at every delivery.

## Production

An exception to `CLAUDE.md`'s Production rule: after your own pushed fix, on a clean `main`, you run `update-plugins.sh` yourself and say so in your report.

## Watching runs

The operator watches each run in its own terminal window, never in VS Code.
VS Code opens only when the operator asks for it, through `lyx ide` or `batten.yaml`'s `open_ide`.
