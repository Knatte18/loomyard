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
// ValidateFormat calls planparser.ValidateFormat and Validate calls planparser.Validate, converts every finding, and appends the same resolve-backed findings on top via one shared resolvePass and then planGatePass — no check exists in both packages.
// ValidateDispatch shares resolvePass but never runs planGatePass, whose checks run at the plan gates only.
// ValidateFormatAfter is the plan gates' form for a plan some of whose cards webster already ran, the cards the index's ValidateFormat is told are done:
// ValidateDispatch's answer with those cards completed, plus planGatePass over the cards not done, so a done card's landed work is never reported as a defect of the plan.
// With no done card it is ValidateFormat.
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
// The finding types and ErrQuarryUnavailable are declared in internal/planindex, which links no tree-sitter;
// this package aliases them, so errors.Is matches under either name.
// NewIndex returns the real planindex.Index, which a package that calls the code index receives from the CLI layer instead of importing planglyph (index.go).
//
// Every resolve-backed Finding.Check ID this package can raise, the canonical list a caller checks
// a claim against without reading Go source — the parallel this package owes planparser's own
// exhaustively numbered "Validation checks" section in contracts/specs/loom-plan-spec.md, since
// nothing enumerated these anywhere else:
//
//   - glyph-not-found, glyph-ambiguous, glyph-rejected — the resolve status policy (resolve.go),
//     blocking, over every glyph target that neither a Create group owns nor a Rename pair names as
//     its New side (a rename destination only exists after the card runs, mirroring planparser's
//     own path-missing rule that never checks Pairs.New). glyph-rejected is additionally the
//     Create inversion's own fail-closed arm (create.go), since a Create target is excluded from
//     the status policy above and would otherwise have no reader at all for an answer neither
//     policy understands.
//     A member whose every declaration sits in files no single build environment selects together is one member, so a partitioned ambiguous answer resolves;
//     the detail of an unpartitioned one names each candidate's file and build constraint.
//     The passes that raise glyph-rejected are resolve.go, create.go, deleteorder.go and donecheck.go — one per fail-closed status policy in the package;
//     each is named at its own bullet, and the double report one anomalous target can produce across two of them is accepted by design.
//   - create-already-exists (blocking), create-new-unit (informational) — the Create inversion
//     (create.go), over every Create group's own targets, handle-shaped or glyph-shaped alike.
//     create-already-exists covers found, multipart and ambiguous alike: all three mean a
//     declaration already occupies the name the card is creating.
//     An ambiguous target is create-already-exists whether or not its candidates are partitioned by build constraints, and its detail names each candidate's file and build constraint.
//   - delete-before-reference (blocking) — LaterDeleteReferences (deleteorder.go), a card that deletes a symbol whose reference a later card's Edit code still holds, so the delete must move after that card.
//     glyph-rejected is additionally its own fail-closed arm, for a Delete or Edit member whose answer it cannot read.
//   - resign-head-mismatch (blocking) — CanonicalizeHandles (handle.go), an Edit re-sign arrow whose head quarry.Name cannot name or names as a member other than the arrow's own glyph.
//     It reads plan text alone, so it runs wherever resolvePass does, ValidateDispatch included.
//   - redundant-file-target (blocking) — planGatePass (plangate.go), a card listing a file self glyph beside a member glyph that resolves into that file.
//     It runs at the plan gates only (ValidateFormat, ValidateFormatAfter over its cards not done, Validate and so ValidateRework), never at ValidateDispatch.
//     The rule the pass exists for: a check whose verdict depends on state the run itself changes runs at the plan gates only, because dispatch re-validates a plan against a tree the run has already changed.
//   - resign-interface-method (blocking) — planGatePass (plangate.go), an Edit re-sign arrow on a member that resolves to an interface method, whose own spec is no declaration.
//     It reads the tree, so like redundant-file-target it runs at the plan gates only.
//   - caller-uncovered (blocking for a resolved reference, informational for a name-only method match) — callerCoverageFindings (callercoverage.go), reached through planGatePass: a deleted or re-signed member that Go code still references with no admissible card's target covering that code.
//     It runs only when a subject exists and at the plan gates only.
//     A typesLoader (typesload.go) loads the root module's packages through go/packages, offline, with test variants and the integration, tmux and llm tags, under a timeout, and logged; an identifier whose used object is the member is a resolved reference.
//     An index built by NewSlottedIndex with a pool slots the load: it writes a wait record into the told wait directory, acquires a hub gate slot, drops the record once the acquire returns, starts the timeout only once the slot is held, runs go list under the slot's `-p` cap and releases the slot when the load returns.
//     NewIndex, standalone runs and a nil pool load unslotted.
//     The load is skipped when the worktree root holds no go.mod, and a failed or timed-out load is logged and leaves every file to the scan, never an error.
//     A slot that cannot be acquired, as over an absent or invalid gate.yaml, is a failed load: the plan gate degrades to the scan rather than refusing, and the run's next slotted gate refuses on the same file with its way forward.
//     The scan walks every Go file under the worktree root by token, nested modules included, over each file the load did not check and each line of a checked file that has an identifier without type information.
//     In the scan a package-level member is resolved by import path: a reference is its bare identifier inside the declaring package, and elsewhere an import of the member's import path (aliased or dotted included) followed by the identifier.
//     A method is matched by name alone, except after an imported package's name, and a subject's name-only matches collapse into one informational finding naming every file.
//     The import-name exclusion also drops a method call on a local variable that shadows an imported package's name, in the scan only.
//     A call through an interface whose method set holds the method is no longer reported once the load answers.
//     The untagged test tier replaces defaultTypesLoader with a loader that fails without spawning, so only the integration tier runs go list.
//     A re-signed member admits its own card's targets; a deleted member admits its own card and every earlier one, and its reference inside a later card's Edit code stays delete-before-reference's.
//   - delete-target-gone (informational) — downgradeGoneDeleteTargets (planglyph.go), a Delete target of a pending card that is already absent, reported by ValidateDispatch alone, and so by ValidateFormatAfter, once a batch is begun or done instead of the blocking path-missing or glyph-not-found finding for it.
//   - handle-name-failed, handle-canonical-collision (both blocking) — CanonicalizeHandles
//     (handle.go), a declaration that fails to parse or two draft handles that canonicalize to the
//     same glyph.
//   - rename-old-unresolved (blocking) — renameDeclSource (handle.go), a Rename pair's Old side
//     that does not resolve found, so no declaration can be derived for its handle-shaped New side.
//   - bind-count-mismatch (blocking) — BindHandles (handle.go), a card whose own handles (its
//     Create declarations AND any Rename pair's still-handle-shaped New side, per cardOwnHandles)
//     the record-batch delta matched fewer of than it owns.
//   - plan-references-deleted-symbol (blocking), rename-candidate (informational) — DetectDrift
//     (drift.go), the exact-tier and evidence-tier halves of drift detection.
//   - scope-outside-plan (informational) — ScopeGuard (scope.go), a symbol a completed batch's
//     delta touched outside its own cards' declared targets.
//   - create-not-done, delete-not-done, rename-not-done (all blocking) — DoneChecks (donecheck.go):
//     a Create target that still does not resolve, a Delete target that still does, or a Rename
//     pair whose old side still resolves or whose new side still does not, after the batch that was
//     supposed to build, remove, or rename it. glyph-rejected is additionally DoneChecks' own
//     fail-closed arm for a done-check answer outside quarry's four-value status vocabulary,
//     mirroring the Create inversion's.
package planglyph
