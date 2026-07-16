# Batch: header-text-pipeline

```yaml
task: "Built-in operator console pane in mux"
batch: header-text-pipeline
number: 3
cards: 6
verify: go test ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...
depends-on: [2]
```

## Batch Scope

Delivers the end-to-end header-text vertical: the `header:` config block (both GOOS templates), the
embedded default header template, `Engine.HeaderText()`/`ValidateHeader()` composing `tokenvocab`, and
the `lyx mux header` verb (default enveloped + `--blocking`). This is one batch because it is a single
cohesive feature — produce the header text — none of whose pieces are independently shippable. External
interface consumed by batch 4: `Engine.HeaderText()` (to launch/validate the pane) and the
`lyx mux header --blocking` command line. Batch-local decision: the default verb mode emits the
`internal/output` envelope (which is JSON by nature — reconciling discussion.md's "`--json` available"
note without a redundant flag); only `--blocking` is envelope-exempt.

## Cards

### Card 8: Header config block in Config and both GOOS templates

- **Context:**
  - `internal/muxengine/template.go`
  - `internal/configengine/config.go`
- **Edits:**
  - `internal/muxengine/config.go`
  - `internal/muxengine/template_posix.yaml`
  - `internal/muxengine/template_windows.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `config.go` add a nested `Header HeaderConfig` field (`yaml:"header"`) to
  `Config`, and a `type HeaderConfig struct { Template string ` + "`yaml:\"template\"`" + `; HeightRows
  int ` + "`yaml:\"height_rows\"`" + ` }`. Document that `Template` empty means "use the embedded
  default" (card 9) and `HeightRows` defaults to 1. Add the matching `header:` block (with `template:
  ""` and `height_rows: 1`) to BOTH `template_posix.yaml` and `template_windows.yaml` so
  `configengine.Load`'s strict template validation accepts the key on every GOOS — omitting either
  file ships one platform without the key. Note in the struct godoc that a hub whose `mux.yaml`
  predates this field needs `lyx config reconcile` to adopt it (matching the `DebugLog`/`Mouse`
  precedent).
- **Commit:** `feat(muxengine): add header config block to Config and both GOOS templates`

### Card 9: Embedded default header template asset

- **Context:**
  - `internal/muxengine/template_posix.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/header-template.md`
  - `internal/muxengine/headertemplate.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `header-template.md` — the default header text template — containing a
  leading `<!-- ... -->` banner comment (stripped by `stencil.Fill`) documenting its tokens, followed
  by the one-line body `hub: {{.hub}}`. Create `headertemplate.go` with `//go:embed header-template.md`
  and `func HeaderTemplate() []byte` returning the embedded bytes. Name the asset `header-template.md`
  (NOT `template.yaml`, which is the config-template convention) per the `builderengine`
  `*-template.md` precedent.
- **Commit:** `feat(muxengine): embed default header text template`

### Card 10: Engine.HeaderText and eager ValidateHeader

- **Context:**
  - `internal/muxengine/lock.go`
  - `internal/muxengine/config.go`
  - `internal/muxengine/headertemplate.go`
  - `internal/tokenvocab/render.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/header.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (e *Engine) HeaderText() (string, error)` in `header.go`: select the
  template bytes (`[]byte(e.cfg.Header.Template)` when non-empty, else `HeaderTemplate()`), build
  `tokenvocab.Ctx{Layout: e.layout}`, call `tokenvocab.Render(template, ctx)`, and return the string
  (surfacing any `stencil` unfilled-marker error). Add `func (e *Engine) ValidateHeader() error` that
  calls `HeaderText()` and returns its error (discarding the text) — the eager, loud validation hook
  batch 4 runs at boot. Both are provider-invariant (no Claude specifics) and read only `e.cfg` /
  `e.layout`.
- **Commit:** `feat(muxengine): add HeaderText and ValidateHeader over tokenvocab`

### Card 11: `lyx mux header` verb (default + --blocking)

- **Context:**
  - `internal/muxcli/attach.go`
  - `internal/muxcli/status.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/muxcli/cli.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:**
  - `internal/muxcli/header.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (c *muxCLI) headerCmd() *cobra.Command` (Use `"header"`, non-empty
  `Short`, a `Long` with an example and the two-mode explanation) with a `--blocking` bool flag. RunE:
  guard `clihelp.ShouldAbort`; call `c.eng.HeaderText()`; on error emit `output.Err` + `clihelp.SetExit`
  (both modes fail loudly pre-print). On success: if `--blocking`, write the raw text to stdout then
  block forever (e.g. `select {}` / block on a never-closed channel) — the pane keepalive, the one
  envelope-exempt tail; if not `--blocking`, emit the text via `output.Ok(out, ...)` (enveloped,
  smoke-testable) and return. Register `c.headerCmd()` in `Command()`'s `parent.AddCommand(...)` call.
  Add `"header"` to the pinned mux `wantSubs` list in `cmd/lyx/helptree_test.go:86`
  (`{"up", "add", "remove", "status", "attach", "resume", "down"}` → add `"header"`); grep
  `cmd/lyx/*_test.go` for any other pinned mux-subcommand list and update it too.
- **Commit:** `feat(muxcli): add lyx mux header verb with default and --blocking modes`

### Card 12: CONSTRAINTS + docs for the envelope exemption

- **Context:**
  - `internal/muxcli/header.go`
  - `internal/muxcli/attach.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CONSTRAINTS.md`'s "Interactive-handoff exception" bullet (under CLI / Cobra
  Invariant), extend the scope to name `mux header --blocking` as a self-displaying keepalive sub-case
  alongside `ide menu` and `mux attach`: the default `mux header` stays fully on the envelope, only the
  `--blocking` print-then-block tail is exempt, and everything fallible (template render) runs pre-flight
  on the envelope. Update the `docs/overview.md#modules` mux rationale (where `mux attach`'s exemption is
  documented) to mention the `header --blocking` exemption with the same reasoning.
- **Commit:** `docs(muxcli): record header --blocking envelope exemption`

### Card 13: Header pipeline tests

- **Context:**
  - `internal/muxengine/header.go`
  - `internal/muxengine/config.go`
  - `internal/muxengine/headertemplate.go`
  - `internal/muxcli/cli_test.go`
- **Edits:** none
- **Creates:**
  - `internal/muxengine/header_test.go`
  - `internal/muxcli/header_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `header_test.go` (muxengine, hermetic): build an `Engine` via `muxengine.New(cfg,
  layout)` with `Config`/`*hubgeometry.Layout` literals (no `Resolve`/tmux spawn). Assert:
  `HeaderText()` with an empty `cfg.Header.Template` renders the embedded default (`hub: <layout.Hub>`);
  a non-empty `cfg.Header.Template` (`repo: {{.repo}}`) renders from config; `ValidateHeader()` returns
  an error for a template with an unknown top-level token (`{{.slug}}`) and nil for a good one.
  `header_test.go` (muxcli): construct the command via `c.headerCmd()` and assert `Use == "header"`, a
  non-empty `Short`, and that the `--blocking` flag is registered — pure command construction, no
  PreRunE/spawn. Do NOT run the `--blocking` path inline (it blocks by design).
- **Commit:** `test(mux): cover HeaderText, ValidateHeader, and the header verb wiring`

## Batch Tests

`verify: go test ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...` runs the hermetic
HeaderText/config tests (card 13), the header-verb wiring test (card 13), and the updated
`cmd/lyx/helptree_test.go`/`drift_test.go` pinned-set + Short guards (card 11). All untagged/fast. The
full enveloped command end-to-end (PreRunE → HeaderText) and the `--blocking` pane are covered by the
mux smoke suite in batch 4, not here.
