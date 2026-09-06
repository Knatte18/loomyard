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
package planglyph
