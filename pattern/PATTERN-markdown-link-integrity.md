# PATTERN-markdown-link-integrity

Every inline markdown link in a `.md` file under `docs/` resolves, file part and `#anchor`.

- `docs/` names a scan source only, not a valid target.
- The allowlist is keyed by `(file, target)`, and each entry names its owning task.
