# Roadmap: mhgo

This document captures the long-term direction and deferred work for `mhgo`. It is a living document — later tasks maintain and refine this roadmap as the project evolves.

## Deferred work

### Creating and cloning the board repository

`mhgo init` currently scaffolds only the configuration layer (`_mhgo/`, commented
`board.yaml`, managed `.gitignore` block). In the future, `init` will grow to
handle repository creation and cloning, analogous to mill-setup Phases 1–3:

- Creating a local board repo from scratch (initializing git, building the
  initial directory structure)
- Cloning an existing board repo from a remote URL
- Full end-to-end project setup as a top-level experience

Today, the board directory is auto-created on first write, and the user manually
makes it a git repo (`git init`, `git remote add`, etc.) for push to work.

### Machine-local configuration overrides

The `.mhgo/board.yaml` layer provides gitignored, machine-local overrides of the
team-wide `_mhgo/board.yaml`. Future enhancements will include:

- A wizard or seeding utility to generate `.mhgo/board.yaml` stubs with
  environment-specific defaults (e.g., custom board directory for a specific
  machine or development environment)
- Persistence helpers for non-YAML config (e.g., credentials, API tokens,
  temporary feature flags)

### Verify / doctor subcommand

A `verify` or `doctor` subcommand to diagnose the state of a mhgo installation:

- Check that `_mhgo/` is present and readable
- Validate the syntax and schema of `board.yaml` files
- Verify the board repository is initialized and accessible
- Detect stale lock files or incomplete operations
- Suggest remediation steps for common issues

### Future modules beyond board

`mhgo` is designed to grow beyond the `board` module. Future modules may include:

- A `mill` orchestrator module (bringing millpy/Millhouse orchestration into the
  Go binary)
- Additional task/project-tracking capabilities
- Integration modules for external services

### Claude Code plugin packaging

Packaging `mhgo` as a Claude Code plugin to integrate task tracking and board
rendering into the IDE, once the Go binary is stable and the module architecture
is proven.

## Explicitly out of scope

The following are **not** planned for `mhgo` and remain in the Python/millpy domain:

- All millpy plumbing not applicable to a Go binary: junctions, hardlinks,
  portals, `PYTHONPATH`, venv setup, `MILL_PYTHON` environment variables
- The millpy wiki daemon and associated socket/RPC infrastructure
- VS Code workspace color schemes and project-local customizations
- Heuristic inference of `Home.md` content shape and wiki-URL derivation
- Task templating and bulk-generation helpers (future millpy feature, not mhgo)

## Maintenance

This roadmap is a shared reference for future tasks. When implementing new
features or fixes:

- Add any deferred work to the appropriate section above
- Update section descriptions to reflect progress
- Close a section when it is fully delivered
- Do **not** enumerate future task entries (use the task list in the hub's issue
  tracker for that — avoid redundant data)

