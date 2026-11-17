# Open no beam files by default and clarify Markdown preview requirements

The `ide.yaml` template now sets `open_files.beam: []`, so opening a beam no longer opens its README by default.
Every place defaults to an empty file list; explicit hub and local file choices still apply.
The existing configuration test expectations and package and overview documentation follow the new default.

`render_markdown` remains enabled by default.
Its `*.md` association uses the editor ID that [VS Code documents](https://code.visualstudio.com/docs/languages/markdown#_set-the-markdown-preview-as-the-default-editor).
The installed Markdown extension declares that ID, and the hub workspace carries the association.
The launcher passes the workspace or folder and requested files without disabling extensions.
There is no evidence here that the association itself needs replacing or removal.

The reported window failure was not reproduced, so its cause remains unconfirmed.
[VS Code's bisect implementation](https://github.com/microsoft/vscode/blob/main/src/vs/workbench/services/extensionManagement/browser/extensionBisect.ts) selects enabled user extensions.
The bisect notification alone therefore does not establish that the built-in Markdown provider was disabled.
The overview now explains checking Markdown Language Features, ending an active bisect, and reopening a source tab as preview.

The existing `TestLoadConfig` rows cover empty defaults, local overrides, and explicit hub file lists.
`go build ./internal/ideconfig`, `go vet ./internal/ideconfig`, formatting, and whitespace checks passed.
The `darn-write` gate reported tier0, tier1, and tier2 passing before the final comment-only corrections.

Go comments flagged by the gate now use shorter sentences and sentence boundaries instead of fixed-column wrapping.
These corrections change no executable code or test assertions.
`lym loom lint-comments` passes on the corrected tree.
