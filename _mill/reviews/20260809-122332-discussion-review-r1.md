MILL_REVIEW_BEGIN
# Review: plan-format: drop the v3 suffix and sweep every reference by script

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Anthropic Claude Opus-class model (system reports claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [NIT:consistency] Bare-`v3` residue contradicts the manifest's rejected alternative
**Demoted-from:** BLOCKING
**Section:** `## Scope` → **Out**, first bullet
**Issue:** `shed-followups.md:169` explicitly rejects "renaming the file but keeping in-text `v3` as a historical label — the suffix is exactly what is being retired", yet the discussion declares exactly that class out of scope; no Decision records this as an override, and after the v2 erasure the renamed doc still says "v3 keeps lyx's own established `What:` name" (`:69`) inside a file titled "Plan format".
**Fix:** Add a Decision that either overrides `:169` explicitly (and adds it to the override-note list) or brings the surviving bare-`v3` sites — at minimum the renamed doc's own body — into scope.

### [NIT:decision] Override-note list omits two recorded overrides
**Demoted-from:** BLOCKING
**Section:** `### shed-followups-override-notes` → *Required notes*
**Issue:** The Problem section states "every override … must also be written back into that file", but the three required notes cover only the six-pattern set, the exclusion set, the v2 erasure and `:209–210`; `constraints-needs-no-change` (overrides `:188`) and `yaml-v3-exclusion-is-structural` (overrides `:199`'s "This task's script names the exclusion explicitly") get no note.
**Fix:** Either extend note 1 to cover `:188` and `:199` (and `:169` per the finding above), or restate the rule as "overrides that affect downstream tasks", so the criterion matches the deliverable.

### [NIT:consistency] Deleting `:5` leaves an orphan blockquote continuation line
**Section:** `### plan-format-5-in-scope`
**Issue:** The blockquote spans `:3–:5`; `:4` is a bare `>` separator that becomes a trailing empty quote line once only `:5` is deleted.
**Fix:** State that `:4`'s bare `>` goes with `:5`, leaving `:3` as the sole blockquote line.

### [NIT:scope] Gate 2's shed-followups filter is a content match, not a path match
**Section:** `## Testing` → *Acceptance gate*, step 2
**Issue:** `| grep -v 'shed-followups.md'` drops any output line whose *content* mentions that filename, so a genuine pattern hit in another file that also cites `shed-followups.md` is silently exempted.
**Fix:** Anchor the exclusion on the path field (e.g. `--exclude=shed-followups.md`, or `grep -v '^\./manifest/designs/shed-followups\.md:'`).

### [NIT:decision] No stated disposition for shed-followups' now-dangling links
**Section:** `### exclusion-set`
**Issue:** Excluding the file leaves `:53`, `:74`, `:192`, `:214` pointing at `plan-format-v3.md`, which will not exist; gate 8 only covers links "touched by the sweep", so this outcome is never accepted in writing.
**Fix:** State explicitly that stale references inside the excluded historical spec are accepted, and that the override notes are where a reader learns the file moved.

### [NIT:consistency] Manifest's "ten Go files" vs this task's 32
**Section:** `### yaml-v3-exclusion-is-structural`
**Issue:** `shed-followups.md:197` says `gopkg.in/yaml.v3` "appears in ten Go files"; the verified count is 32, and the discussion asserts 32 without noting the manifest's stale figure.
**Fix:** Note the corrected count alongside the other override notes, so the next reader of `:197` is not misled.

## Verdict

REQUEST_CHANGES
Bare-`v3` residue and incomplete override notes need explicit disposition before planning.
MILL_REVIEW_END
