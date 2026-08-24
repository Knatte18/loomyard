# Card 3 — json-emission

**Custom:**
- `boardcli.emitJSON`
- `//internal/output/emit.go`

**Uses:**
- `list.go`

**Intent:** Introduce `emitJSON`, a new helper in a new file, marshaling each row through
`output.Ok` when `--json` is set. A Custom card because it spans a brand-new symbol and a
brand-new file at once and is called from `list.go`, neither cleanly a Create nor an Edit.
