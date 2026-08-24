# Card 3 — json-emission

**Custom:**
- `boardcli.emitJSON`
- `//internal/output/emit.go`

**Uses:**
- `list.go`

**Intent:** Introduce `emitJSON`, a new helper in a new file, marshaling each row through `output.Ok` when `--json` is set.
