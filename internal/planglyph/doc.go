// Package planglyph is the sole owner of every quarry.Repo call (Open, Resolve, EncloseAt, DeltaGit).
// It is also the sole owner of the package-level quarry.Name.
// It runs the resolve-backed validation pass layered on top of internal/planparser's pure checks.
//
// planglyph derives no path of its own and never imports internal/lymcwd.
// Its validation entry points take worktreeRoot as planparser.Validate(plan, worktreeRoot) does.
// They take it because quarry.Open needs an absolute root.
//
// planglyph composes internal/planparser's pure checks rather than reimplementing any of them.
// The format gate and the approval gate append the resolve-backed findings and the plan-gate-only checks.
// ValidateDispatch appends the resolve-backed findings only.
// A check whose verdict depends on state the run itself changes runs at the plan gates only.
// The gate for a plan some of whose cards webster already ran is the index's ValidateFormat, told those cards as done.
//
// A caller's answer is a three-way split.
// Pure findings come from planparser and are stamped SeverityBlocking.
// Resolve findings come from this package's passes, with either severity.
// The third part is an infrastructure error, ErrQuarryUnavailable.
// It is returned alongside whatever pure findings were already collected.
// Degrading to format-only validation would let a plan look validated when it was not.
//
// The finding types and ErrQuarryUnavailable are declared in internal/planindex, which links no tree-sitter;
// this package aliases them, so errors.Is matches under either name.
// NewIndex returns the real planindex.Index.
// A package that calls the code index receives it from the CLI layer instead of importing planglyph.
//
// The list below is the canonical list of every resolve-backed Finding.Check ID this package can raise.
// It parallels the plan gate's check catalogue in contracts/specs/loom-plan-checks-spec.md.
//
//   - glyph-not-found, glyph-ambiguous, glyph-rejected: the resolve status policy, blocking,
//     over every glyph target that neither a Create group owns nor a Rename pair names as its New side.
//     glyph-rejected is also the fail-closed arm for an answer no policy understands.
//     A member whose declarations sit in files no single build environment selects together is one member,
//     so a partitioned ambiguous answer resolves.
//     A self glyph that two alphabets both match is ambiguous, and its detail names the way forward.
//   - create-already-exists (blocking), create-new-unit (informational): the Create inversion over Create targets.
//     A found, multipart or ambiguous Create target means a declaration already occupies the name.
//   - delete-before-reference (blocking for a resolved reference, informational for a name-only method match):
//     a card that deletes a symbol whose reference a later card's Edit or Extends code still holds.
//   - resign-head-mismatch (blocking): an Edit re-sign arrow whose head quarry.Name cannot name,
//     or names as a member other than the arrow's own glyph.
//     It reads plan text alone, so it runs wherever the resolve pass does.
//   - resign-overload-collision (blocking): an accepted C# arrow whose minted id collides with another declaration.
//   - resign-not-done (blocking): a recorded C# arrow whose minted id does not resolve found in the tree.
//     It fails closed and rewrites nothing; when every minted id resolves, RebindResigns rewrites the old glyphs.
//   - plan-language-mismatch (blocking): a go plan whose targets resolve into `.cs` files and no `.go` file.
//   - redundant-file-target (blocking): a card lists a file self glyph beside a member glyph that resolves into it.
//   - resign-interface-method (blocking): an Edit re-sign arrow on a member that resolves to an interface method.
//   - caller-uncovered (blocking for a resolved reference, informational for a name-only method match):
//     a deleted or re-signed member that Go code still references with no admissible card's target covering that code.
//     It reports none when plan.Part is set, because the verdict rests on cards outside the part.
//     A failed or timed-out types load degrades the plan gate to the scan, never to an error.
//   - delete-target-gone (informational): a Delete target of a pending card that is already absent.
//   - handle-name-failed, handle-canonical-collision (both blocking): CanonicalizeHandles,
//     a declaration that fails to parse, or two draft handles that canonicalize to the same glyph.
//   - rename-old-unresolved (blocking): a Rename pair's Old side that does not resolve found,
//     so no declaration can be derived for its handle-shaped New side.
//   - bind-count-mismatch (blocking): BindHandles, a card whose record-batch delta matched fewer handles than it owns.
//   - plan-references-deleted-symbol (blocking), rename-candidate (informational): DetectDrift,
//     the exact-tier and evidence-tier halves of drift detection.
//   - later-target-removed (blocking): RemovedTargets, a symbol a batch's delta deleted that a pending card targets.
//   - scope-outside-plan (informational): ScopeGuard, a symbol a completed batch's delta touched outside its targets.
//   - create-not-done, delete-not-done, rename-not-done (all blocking): DoneChecks,
//     a Create, Delete or Rename target judged after the batch that was supposed to build, remove or rename it.
//     signature-mismatch (blocking) replaces the not_found finding when the unit declares the name with other signatures.
//     A Create target under the root's git-ignored `.scratch/` is a transient draft and gets no done-check.
package planglyph
