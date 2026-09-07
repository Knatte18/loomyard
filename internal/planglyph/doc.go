// doc.go documents internal/planglyph, the sole owner of every quarry.Repo call (Open, Resolve,
// DeltaGit) and of the package-level quarry.Name, and of the resolve-backed validation pass layered
// on top of internal/planparser's pure checks.
//
// planglyph derives no path of its own and never imports internal/lyxcwd: its two entry points,
// ValidateFormat and Validate, take worktreeRoot exactly as planparser.Validate(plan, worktreeRoot)
// does, per the told-geometry-for-planglyph Shared Decision — quarry.Open needs an absolute root,
// which is exactly why the root is told rather than derived.
//
// planglyph composes internal/planparser's pure checks rather than reimplementing any of them:
// ValidateFormat calls planparser.ValidateFormat and Validate calls planparser.Validate, converts
// every finding, and appends the same resolve-backed findings on top via one shared resolvePass —
// no check exists in both packages.
//
// A caller's own answer is therefore a three-way split, never a two-way one: pure findings (from
// planparser, stamped SeverityBlocking), resolve findings (from this package's own passes, either
// severity), and an infrastructure error that is neither of the two — ErrQuarryUnavailable,
// returned alongside whatever pure findings were already collected, so a quarry outage is never
// mistaken for a clean answer. Rejected, and worth stating so it is not reintroduced: degrading to
// format-only validation with a warning, the exact failure mode where a plan looks validated and
// was not; and making the error informational everywhere, which makes the outage invisible at
// precisely the boundaries whose whole job is to be mechanical. See repo.go and planglyph.go.
//
// Every resolve-backed Finding.Check ID this package can raise, the canonical list a caller checks
// a claim against without reading Go source — the parallel this package owes planparser's own
// exhaustively numbered "Validation checks" section in contracts/specs/loom-plan-spec.md, since
// nothing enumerated these anywhere else:
//
//   - glyph-not-found, glyph-ambiguous, glyph-rejected — the resolve status policy (resolve.go),
//     blocking, over every glyph target that neither a Create group owns nor a Rename pair names as
//     its New side (a rename destination only exists after the card runs, mirroring planparser's
//     own path-missing rule that never checks Pairs.New).
//   - create-already-exists (blocking), create-new-unit (informational) — the Create inversion
//     (create.go), over every Create group's own targets, handle-shaped or glyph-shaped alike.
//   - containment-file-overlap (blocking) — the resolve-backed containment tier (containment.go),
//     the member-vs-file overlap the syntactic tier cannot see.
//   - handle-name-failed, handle-canonical-collision (both blocking) — CanonicalizeHandles
//     (handle.go), a declaration that fails to parse or two draft handles that canonicalize to the
//     same glyph.
//   - rename-old-unresolved (blocking) — renameDeclSource (handle.go), a Rename pair's Old side
//     that does not resolve found, so no declaration can be derived for its handle-shaped New side.
//   - bind-count-mismatch (blocking) — BindHandles (handle.go), a card whose declared handles the
//     record-batch delta matched fewer of than it declared.
//   - plan-references-deleted-symbol (blocking), rename-candidate (informational) — DetectDrift
//     (drift.go), the exact-tier and evidence-tier halves of drift detection.
//   - scope-outside-plan (informational) — ScopeGuard (scope.go), a symbol a completed batch's
//     delta touched outside its own cards' declared targets.
//   - create-not-done, delete-not-done (both blocking) — DoneChecks (donecheck.go), a Create target
//     that still does not resolve or a Delete target that still does, after the batch that was
//     supposed to build or remove it.
package planglyph
