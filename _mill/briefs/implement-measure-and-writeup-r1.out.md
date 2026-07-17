Both cards are committed and verified.

{"status":"success","commit_sha":"bd670d62","session_id":"dc018ac2-265d-4dd6-a8c8-4969a2e86367"}

Card-count self-check: 2 of 2 cards committed (Card 6: a756c116; Card 7: bd670d62), plus 4 supporting commits under the surprise-file protocol (two plan-extension commits, two harness bugfix commits).

- Card 6 (a756c116): created .lsp.json. ENABLE_LSP_TOOL could not be toggled on the running session; recorded docs-only characterization to .scratch/codeintel/cc-native.md (gitignored) for Card 7.
- Surprise-file fixes (plan extended first): gopackages.go resolveSymbol pointer-wrap bug for interface types; callgraph.go transitiveCallers origin-matching bug (byte-identical caller sets across unrelated targets). Plan file 03-measure-and-writeup.md extended under Card 7 Edits before each fix.
- Card 7 (bd670d62): created docs/research/codeintel-spike.md. Ran all arms across 5 benchmark symbols, verdict: ADOPT in-process go/packages+go/types references; callers/call-hierarchy needs rebuild on Uses/Defs (100% false-negative on generic-instantiation calls); DEFER callgraph (CHA/RTA over-approximate 70-225x vs VTA). Raw data under .scratch/codeintel/ (gitignored).

`go build ./tools/codeintel-poc/` passes; no tracked files left dirty.
