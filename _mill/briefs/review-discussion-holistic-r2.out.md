MILL_REVIEW_BEGIN
# Review: prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-29
```

## Findings

### [BLOCKING:design] Non-recursive listing's blob entries have no disposition
**Section:** §Decisions "Truncated fallback walks with `recursive=1` per subtree"
**Issue:** The fallback lists the root tree non-recursively and then only "emit[s] that subtree's blobs", so root-level files (`README.md`, `Makefile`) present in the non-recursive listing are never said to be emitted — a silently partial list, contradicting the "never silently partial" promise the decision itself invokes.
**Fix:** State explicitly that blob entries in every non-recursive listing (root and any re-listed subtree) are emitted, and add a fixture asserting a root-level file appears on the truncated path.

### [BLOCKING:design] Truncated subtree's children enumerated from a truncated response
**Section:** §Decisions "Truncated fallback ..."
**Issue:** For a subtree that itself returns `truncated: true`, the design says "push its own subtrees onto the worklist" — but that subtree list comes from the truncated response, which is by definition incomplete, so subtrees past the cap are lost; at the root the design correctly re-fetches non-recursively, and the same step is missing one level down.
**Fix:** Specify that any node reporting `truncated: true` is re-listed non-recursively before its children are enqueued, uniformly at root and at depth.

### [BLOCKING:design] `[path]` → tree-SHA resolution mechanism unspecified
**Section:** §Decisions "Optional `[path]` argument is a subtree scope"
**Issue:** On the truncated path the script must "resolve that directory to its tree SHA", but no mechanism is given for a nested path (`drivers/net/ethernet`) — per-component non-recursive descent, contents API, or something else; the stated "27 calls to 2" saving is also inconsistent with the algorithm, which needs branch-resolve + root recursive + root non-recursive + subtree ≥ 4 for even a top-level path.
**Fix:** Name the resolution method and its per-component call cost, and correct the call-count claim to match it.

### [BLOCKING:consistency] Stub `gh` cats JSON but the script consumes `--jq` output
**Section:** §Testing, "The stub `gh`"
**Issue:** The stub is specified to "`cat` a canned JSON file", yet every fetch in §Decisions reads a `--jq`-transformed `#trunc\t…` / `type\tsha\tpath` TSV stream; a stub that ignores `--jq` and emits raw JSON does not exercise the script's actual input format.
**Fix:** Say whether fixtures are pre-transformed TSV keyed by the jq expression, or the stub delegates JSON+expression to real `gh`/gojq — and note the consequent loss of coverage of the jq expression itself.

### [NIT:consistency] `docs/benchmarks/` cited for a claim it does not make
**Section:** §Problem
**Issue:** "Turn count, not tool latency, is the dominant driver of agent wall-clock time in this project (see `docs/benchmarks/`)" — the four files there cover board writes, test-suite timing, fixture copy, and running tests; none measures agent turns.
**Fix:** Drop the citation or point at the actual source; the design rationale stands without it.

### [NIT:design] Stack worklist vs. claimed traversal order
**Section:** §Decisions "Traversal-order output, no post-hoc sort"
**Issue:** A bash array "used as a stack" pops siblings in reverse push order, so output is deterministic but not the git-sorted order the rationale implies.
**Fix:** State the expected sibling order the fixtures will assert (queue-order vs. LIFO), so tests pin the intended one.

## Verdict

REQUEST_CHANGES
Fallback completeness, path resolution, and stub-`gh` format need resolving before planning.
MILL_REVIEW_END
