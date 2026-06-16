Perfect! I have successfully completed the batch. Here's what was done:

## Summary of Changes

**Card 7: Swap muxpoc to fsx.AtomicWrite**

I edited `internal/muxpoc/state.go` to:
1. Replace the import `"github.com/Knatte18/loomyard/internal/board"` with `"github.com/Knatte18/loomyard/internal/fsx"`
2. Changed line 108 from `board.AtomicWrite(cwd, stateRelPath, string(content))` to `fsx.AtomicWrite(cwd, stateRelPath, string(content))`

The changes are minimal and behaviour-preserving, as both functions have identical signatures.

{"status":"success","commit_sha":"8b98ce40fde7398f4304c9443a0e0ff164018d33","session_id":"ca5b8819-5009-4a15-b47e-6ae9e081bee6"}
