MILL_REVIEW_BEGIN
# Review: Relocate producer prompt files into a stencils/ directory

```yaml
duration_s: 348.0
verdict: REQUEST_CHANGES
reviewer_model: fable
reviewer_self_id: Fable 5 (claude-fable-5)
reviewed_file: _mill/discussion.md
date: 2026-08-14
```

## Findings

### [BLOCKING:design] Stamp rule assumes a banner implementer-body.md lacks
**Section:** `### stamp-format-and-edit-detection`, `### compose-strips-every-banner`, `## Technical context`, `## Q&A log`
**Issue:** "All 15 files open with such a banner today" is false — `internal/websterengine/implementer-body.md:1` opens with the `# Webster implementer job` heading, no `<!-- -->` block (the 15th banner-carrying file under `internal/` is reed's `header-template.md`, not a stencil). So "folded into the existing banner block rather than added as a second one" is unimplementable for that file, and the "pre-existing second-banner leak" does not exist: `joinTemplateAssets` (`internal/websterengine/render.go:60-65`) joins a banner-carrying prefix with a bannerless body, and `stencil.Fill` strips the only banner present — nothing leaks until this task stamps the body.
**Fix:** (1) In `stamp-format-and-edit-detection`, replace "a stamp line inside the leading `<!-- ... -->` banner it already has, of the form `<!-- lyx-stencil: sha256=<hex> -->` (folded into the existing banner block rather than added as a second one)" with "a stamp line of the form `<!-- lyx-stencil: sha256=<hex> -->` inside the file's leading `<!-- ... -->` banner — folded into the existing banner where one exists, or written as a new leading banner for `implementer-body.md`, the one default that ships without one". (2) In `## Technical context`, replace "All 15 files open with such a banner today." with "14 of the 15 open with such a banner today; `implementer-body.md` does not, so seeding creates its banner there.", and replace "The composed result contains two banner comments, only the first of which `stripLeadingComment` removes. That is existing behaviour, but this task makes it actively harmful and therefore must fix it" with "Today the composed result carries only the prefix's banner, which Fill strips; once seeding stamps `implementer-body.md` it would carry two, and only the first is stripped — this task creates that hazard and must fix it". (3) In `compose-strips-every-banner`, replace "so the second file's banner already reaches the LLM verbatim today" with "so a banner on the second file would reach the LLM verbatim", and replace "and the pre-existing double-banner leak is fixed by the same change rather than left standing beside it" with "and the general stripper also hardens any future banner-carrying asset". (4) In the Q&A entry "Once `implementer-body.md` carries a stamp…", replace "which also fixes the pre-existing second-banner leak" with "which is what makes the stamp safe to add". The decision itself stands unchanged — only its factual premise moves.

### [BLOCKING:design] Marker-failure direction is backwards; validate has no basis
**Section:** `### invalid-stencil-handling` (knock-ons in `## Testing`, `## Scope`)
**Issue:** "`stencil.Fill` requires every top-level marker to be present, so an edit that deletes a marker breaks a producer mid-run" inverts the real semantics: `unfilledTopLevelMarkers` (`internal/stencil/stencil.go:39-43,82`) errors only when the *template* carries a marker the producer's values leave unfilled — an added/renamed/typo'd marker breaks Fill, while a deleted marker fills cleanly and silently drops that content from the prompt. The decision also never states what `validate` compares against to detect a bad marker, and the values each producer supplies live in the engines, unreachable from `stencilcli`.
**Fix:** Replace the rationale sentence with: "`stencil.Fill` fails when the template carries a top-level marker the producer's values do not fill, so an edit that adds or renames a marker breaks a producer mid-run — while an edit that deletes a marker fills cleanly and silently drops that content, the invisibility class this task exists to remove. `validate` therefore compares each body's top-level marker set against its shipped default's, both recoverable via the registry: a marker absent from the default is an error (it will break Fill), a default marker missing from the body is a warning (legal customisation, but content-dropping)." Default-marker-set is the right basis because producer values are engine-internal, and it matches `ValidateHeader`'s direction — it errors on an unknown top-level token (`internal/reedengine/header_test.go:51-55`). Knock-ons: in `## Testing`, replace "a stencil whose body no longer fills (a deleted top-level marker) fails validation with the offending name" with "a stencil whose body adds a marker unknown to its shipped default fails validation with the offending name; one that deletes a default marker is reported as a warning"; in `## Scope`, extend "Exporting `internal/stencil`'s leading-comment stripper" to also name exporting a top-level-marker lister (today unexported inside `unfilledTopLevelMarkers`), which `validate` needs.

### [NIT:consistency] Registry consumer claim contradicts Reconcile signature
**Demoted-from:** BLOCKING
**Section:** `### stencils-is-a-go-package` vs `### seeding-trigger`
**Issue:** "`stencilstore` is its only consumer; no engine imports the `stencils` package directly" contradicts `Reconcile(baseDir, registry)` — a caller-supplied registry means the composition root and `stencilcli` (`sync`/`list`/`validate` all need the name set and defaults) obtain the registry from the `stencils` package themselves, so they are consumers too; both sentences cannot hold.
**Fix:** Keep the parameterised signature — it is what lets stencilstore's edit-detection tests run against a fake registry and a bare `t.TempDir()`, preserving the Tier-1 rationale in `stencilstore-ownership` — and in `stencils-is-a-go-package` replace "`stencilstore` is its only consumer; no engine imports the `stencils` package directly, so treadle's allowlist needs the one `internal/stencilstore` entry and no second one" with "`internal/stencilstore` and the composition roots that hand it the registry — `cmd/lyx`'s root pre-run and `stencilcli` — are its only consumers; no engine imports it, and treadle calls only `stencilstore.Read(baseDir, name)`, which needs no registry, so treadle's allowlist needs the one `internal/stencilstore` entry and no second one." No other section restates the only-consumer claim, so no further knock-on.

### [NIT:decision] Board .gitattributes lifecycle unstated
**Section:** `### stamp-format-and-edit-detection` (knock-ons in `## Testing`, `### cli-surface`)
**Issue:** "The board's stencils tree is seeded with its own `.gitattributes`" states creation only — not whether it is re-seeded when deleted, edit-detected (it cannot carry an `<!-- -->` stamp), registry-listed, or covered by the seeding commit's positive pathspec.
**Fix:** Append to that bullet: "The `.gitattributes` file is seed-if-absent only, mirroring configsync's `SeedOnly` behaviour: rewritten when missing, never when present, never stamped, never in the registry, invisible to `list`/`validate`/`diff`, and always included in the seeding commit's positive pathspec." Seed-if-absent is the right choice because LF-normalised hashing already keeps the mechanism correct without it, so a second edit-detection scheme for a non-markdown file buys nothing. Knock-ons: `## Testing`'s "Seeding concurrency" bullet gains "and that the pathspec also covers the seeded `.gitattributes`"; `cli-surface`'s "`list` names all 15" then stays exactly true.

### [NIT:decision] Dev-stamp variable has no named home
**Section:** `### seeding-trigger` (knock-on in `## Testing`)
**Issue:** "sets a package-level string via `-ldflags -X` at build time" names neither the package nor the variable, and the mode's route into the reconcile pass is unstated.
**Fix:** Replace with: "`tools/deploy -dev` sets `var buildChannel string` in `package main` (`cmd/lyx/main.go`) via `-ldflags \"-X main.buildChannel=dev\"` — `tools/deploy/main.go` passes no `-ldflags` today, so this is a new flag on that build path — and the composition root threads the resulting mode into `stencilstore.Reconcile` as an explicit argument; `stencilstore` itself never reads build identity." `package main` is the right home because only the root pre-run consumes it, and an explicit mode argument keeps stencilstore's dev/prod tests hermetic. Knock-on: `## Testing`'s "Dev/prod seeding" bullet should note the assertions drive the mode argument, not a stamped binary.

## Verdict

REQUEST_CHANGES
Two verified false premises about source behaviour and one internal contradiction must be corrected.
MILL_REVIEW_END
