# PATTERN-markdown-link-integrity

Every inline markdown link in a `.md` file under `manifest/` or `docs/` resolves, file part and `#anchor`.

- `manifest/` and `docs/` name scan sources only, not valid targets.
- The allowlist is keyed by `(file, target)`, and each entry names its owning task.
