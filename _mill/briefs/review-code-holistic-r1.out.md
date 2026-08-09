MILL_REVIEW_BEGIN
# Review: Scope the Shed producer-model rewrite into buildable tasks — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-09
```

## Findings

### [BLOCKING] Stray `</content>` tag leaked into three staged task bodies and their published wiki pages
**Location:** `_mill/followup/C-format-docs-name-producers.md:80`, `_mill/followup/D-raddle-finalize-fold-and-link-repair.md:63`, `_mill/followup/E-shed-model-contradiction-sweep.md:119`
**Issue:** Each file ends with a literal `</content>` line after its last real sentence. Per the shared decision, "everything after that block's closing fence is the task body, published verbatim" — and it was: the tag is already in the published wiki pages, confirmed in `wiki/tasks.json`'s stored `body` fields for tasks 167 (`format-docs-name-producers`), 168 (`raddle-finalize-fold-and-link-repair`), and 169 (`shed-model-contradiction-sweep`). Tasks 165 (A), 166 (B), and 170 (F) do not carry the tag, so this is isolated to C/D/E.
**Fix:** Strip the trailing `</content>` line from the three staged files and re-run the batched upsert (idempotent by slug, per card 7) to overwrite the corrupted wiki bodies.

### [NIT] F's body drops half of the "carry rejected alternatives from both decisions" instruction
**Location:** `_mill/followup/F-batcher-standalone-split.md:19-23`
**Issue:** Card 3 instructs carrying rejected alternatives from both `batchifier-splits-out-of-webster` and `batcher-extracts-standalone-now-absorbed-by-shed-later`. The body's Rejected-alternatives list only carries the three from the second decision, omitting the first decision's "dropping Batchifier to preserve the shipped framing" and "surfacing it as unresolved" (`discussion.md:63`).
**Fix:** Add the two missing rejected alternatives from `batchifier-splits-out-of-webster`.

### [NIT] A regroups discussion's Doc-retirement inventory under "Code deletion" instead of transcribing its own section boundaries
**Location:** `_mill/followup/A-builder-retire.md:42-49`
**Issue:** `discussion.md:202-217` places README.md, docs/sandbox-howto.md, sandbox/builder-suite.cmd, .gitattributes, comment-only residue, roadmap.md's Done item, and status-schema.md under *Doc retirement*. A's body instead files all of these under item "1. Code deletion," leaving its own "2. Doc retirement" narrower than the source. Content is complete — every site is present somewhere — but the grouping diverges from the "transcribe, do not re-derive" decision's own section boundaries.
**Fix:** Move those bullets into the "Doc retirement" numbered step to match `discussion.md`'s own grouping.

## Verdict

REQUEST_CHANGES
Three of six published task bodies are corrupted by a trailing `</content>` artifact; fix before handoff.
MILL_REVIEW_END
