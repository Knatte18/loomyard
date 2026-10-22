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
// The index's format, rework and dispatch validation append fabric-vocabulary-name (planparser.CheckFabricVocabularyName), a declared name the fabric-vocabulary scan would refuse.
// Dispatch drops its findings against completed cards.
// The index's format and rework validation also append card-fabric-reference and edit-test-file (planparser.CheckEditTestFile), which dispatch does not run.
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
//     Build-constraint partitioning reads `.go` candidate sets only: a candidate set holding any other file is never partitioned, and such a file is described as carrying no constraint, so a name such as `Foo_windows.cs` is never read as one.
//     A self glyph that two alphabets both match is an ambiguous answer with no candidates and a non-empty list of languages, and its detail names those languages and the way forward in place of an empty candidate list:
//     name the file as a slashed path self glyph, or a member as `lyx quarry glyphs` lists them.
//     The passes that raise glyph-rejected are resolve.go, create.go, deleteorder.go and donecheck.go — one per fail-closed status policy in the package;
//     each is named at its own bullet, and the double report one anomalous target can produce across two of them is accepted by design.
//   - create-already-exists (blocking), create-new-unit (informational) — the Create inversion
//     (create.go), over every Create group's own targets, handle-shaped or glyph-shaped alike.
//     create-already-exists covers found, multipart and ambiguous alike: all three mean a
//     declaration already occupies the name the card is creating.
//     An ambiguous target is create-already-exists whether or not its candidates are partitioned by build constraints, and its detail names each candidate's file and build constraint.
//   - delete-before-reference (blocking for a resolved reference, informational for a name-only method match) — a card that deletes a symbol whose reference a later card's Edit code still holds, so the delete must move after that card.
//     It is computed by callerCoverageFindings (callercoverage.go) through planGatePass at the plan gates, from the reference computation caller-uncovered shares, and by LaterDeleteReferences (deleteorder.go) scan-only at ValidateDispatch and for Webster.
//     Only Go files outside the directories the go tool ignores are searched; a package self glyph Delete keeps its quoted import-path match, over Go regions only.
//     A member reference that an Edit target of the deleting card or an earlier card also covers is not reported, since that card rewrites the code before the later card runs; the quoted import-path match keeps no such exemption.
//     glyph-rejected is additionally its own fail-closed arm of LaterDeleteReferences, for a Delete or Edit member whose answer it cannot read.
//     A member partitioned by build constraints is looked up through its candidates, which are its declarations.
//   - resign-head-mismatch (blocking) — nameResigns (resign.go), an Edit re-sign arrow whose head quarry.Name cannot name or names as a member other than the arrow's own glyph.
//     It reads plan text alone, so it runs wherever resolvePass does, ValidateDispatch included.
//     nameResigns names every arrow before the plan's batched resolve, and nameDeclarations (handle.go) is the package's one quarry.Name site, shared with CanonicalizeHandles, while resolveTargets stays the one resolve site.
//     A Go arrow is accepted when the minted id is its target's own.
//     A C# arrow is accepted when the minted id is its target's apart from the parameter list, with both written with parentheses or both without, since a re-sign changes the parameter list and so the id.
//     A C# arrow's way forward depends on how the head failed: correct the head to one quarry names under the owner chain, keep the parameter list or change the member's kind through a Delete and a Create, or use a Rename pair for a name change.
//   - resign-overload-collision (blocking) — resignCollisionFindings (resign.go), an accepted C# arrow whose minted id differs from its target and collides with an existing overload that resolves found, with a Create or Rename handle that binds to the same id, or with another arrow's minted id on a different target.
//     The finding is attributed to the later of the two cards and names the earlier, and it is lifted when a card in the span between the two declarations deletes the colliding member, renames it away or re-signs it away.
//     A card names a re-signed C# member by its old glyph until the re-sign is recorded.
//   - resign-not-done (blocking) — RebindResigns (resign.go), reached from the post-batch pass of record-batch and recover-batch: a recorded C# arrow whose minted id does not resolve found in the tree, so the card's edit does not match its arrow.
//     Fail closed, only a found answer shows the edit declares the arrow's head, and nothing is rewritten.
//     When every minted id resolves found, one plan.RewriteRefs call rewrites each old glyph to its minted id plan-wide, so a later card naming the member by its old glyph names the minted id.
//     A Go arrow, an arrow whose minted id is its target and a plan whose language opts out of glyphs do nothing.
//   - plan-language-mismatch (blocking) — languageMismatchFindings (languagemismatch.go), the resolve-backed half of the check planparser also makes from plan text: a go plan whose targets resolve into `.cs` files and no `.go` file.
//     It is plan-level, runs only when no card is done, and never fires when a card creates a declaration whose head opens with a Go keyword, or when a non-path target other than a Create target has no found or multipart answer, since that glyph's own finding stands.
//   - redundant-file-target (blocking) — planGatePass (plangate.go), a card listing a file self glyph beside a member glyph that resolves into that file.
//     A partitioned member's candidates are its declarations, so a file holding one of them counts.
//     It runs at the plan gates only (ValidateFormat, ValidateFormatAfter over its cards not done, Validate and so ValidateRework), never at ValidateDispatch.
//     The rule the pass exists for: a check whose verdict depends on state the run itself changes runs at the plan gates only, because dispatch re-validates a plan against a tree the run has already changed.
//   - resign-interface-method (blocking) — planGatePass (plangate.go), an Edit re-sign arrow on a member that resolves to an interface method, whose own spec is no declaration.
//     It reads the tree, so like redundant-file-target it runs at the plan gates only.
//   - caller-uncovered (blocking for a resolved reference, informational for a name-only method match) — callerCoverageFindings (callercoverage.go), reached through planGatePass: a deleted or re-signed member that Go code still references with no admissible card's target covering that code.
//     A partitioned member's candidates are its declarations, so each is a subject's span and a target's span alike.
//     It runs only when a subject exists and at the plan gates only.
//     A typesLoader (typesload.go) loads the root module's packages through go/packages, offline, with test variants and the integration, tmux and llm tags, under a timeout, and logged; an identifier whose used object is the member is a resolved reference.
//     An index built by NewSlottedIndex with a pool slots the load: it writes a wait record into the told wait directory, acquires a hub gate slot, drops the record once the acquire returns, starts the timeout only once the slot is held, runs go list under the slot's `-p` cap and releases the slot when the load returns.
//     NewIndex, standalone runs and a nil pool load unslotted.
//     The load is skipped when the worktree root holds no go.mod, and a failed or timed-out load is logged and leaves every file to the scan, never an error.
//     A slot that cannot be acquired, as over an absent or invalid gate.yaml or an invalid machine override of it, is a failed load: the plan gate degrades to the scan rather than refusing, and the run's next slotted gate refuses on the same file with its way forward.
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
//     A handle's alphabet is the Lang of planparser.ParseRef over its body.
//     A C# Create handle is `plan:<namespace>#<owner.member>`, and its declaration is named under that namespace with the owner chain as `Owner`, less the explicit-interface qualifier the head writes.
//     That qualifier is the longest trailing run of owner components the head spells, dotted and immediately before the declared name, compared in the canonical C# spelling of type arguments, so `IPair<string, int>.First` qualifies the owner component `IPair<string,int>`.
//     A body the C# alphabet rejects is one handle-name-failed and quarry is not asked to name it;
//     a head whose qualifier is spelled non-canonically and refused by quarry names the canonical spelling as the way forward.
//   - rename-old-unresolved (blocking) — renameDeclSource (handle.go), a Rename pair's Old side
//     that does not resolve found, so no declaration can be derived for its handle-shaped New side.
//     An old side declared once per build-constraint set stays unresolved, since a rename derives from exactly one declaration;
//     its detail names each candidate's file and constraint and the way forward: a Delete of the old member and one Create declaration of the new member's handle on a card that edits each of the old member's files, or an Edit of each file's body.
//     A C# symbol's declaration is derived by renameCSharpHead from its signature, under the old symbol's unit and owner chain, with the declared name token replaced by the new handle's name without its generic arity suffix.
//     A field, const or event gets a one-declarator head, and a member no identifier names, an operator, an indexer, a finalizer or a constructor, is rename-old-unresolved with the way forward of dropping the pair.
//   - bind-count-mismatch (blocking) — BindHandles (handle.go), a card whose own handles (its
//     Create declarations AND any Rename pair's still-handle-shaped New side, per planparser.OwnedHandles)
//     the record-batch delta matched fewer of than it owns.
//   - plan-references-deleted-symbol (blocking), rename-candidate (informational) — DetectDrift
//     (drift.go), the exact-tier and evidence-tier halves of drift detection.
//   - scope-outside-plan (informational) — ScopeGuard (scope.go), a symbol a completed batch's
//     delta touched outside its own cards' targets (a file target covers every symbol in that
//     file, a package target every symbol in a file directly in its directory), and a changed file
//     the delta gives no symbols for that no card target covers, file-granular: a target's file or
//     package directory lets a symbol-less change through.
//     ScopeGuard receives the plan, so it parses each target in the plan's alphabet.
//   - create-not-done, delete-not-done, rename-not-done (all blocking) — DoneChecks (donecheck.go):
//     a Create target that still does not resolve, a Delete target that still does, or a Rename
//     pair whose old side still resolves or whose new side still does not, after the batch that was
//     supposed to build, remove, or rename it.
//     A Create target under the worktree root's git-ignored `.scratch/` is a transient draft its card may remove once read, so it gets no done-check;
//     a `.scratch/` below the root, a Rename new side and every Delete target keep theirs.
//     A target still path-shaped after canonicalization, one no glyph spells such as a path whose directory holds a space, is judged on disk instead of resolved.
//     Its path must exist for a Create or Rename new side and must not for a Delete or Rename old side (planparser.IsDiskCheckedTarget).
//     Each detail carries the lookup: the resolve key looked up beside the card's own spelling, quarry's status, for not_found whether the unit exists, and for ambiguous each candidate's file and build constraint.
//     A Create or Rename new side answered not_found also names the files declaring its unit and every declaration of its name elsewhere in the worktree, from one glyphs listing taken only when such a finding is due.
//     An ambiguous answer whose candidates sit in files no single build environment selects together counts as resolved, so a Create or a Rename new side over such a member lands;
//     the Delete direction and a Rename old side keep blocking on any surviving declaration.
//     glyph-rejected is additionally DoneChecks' own
//     fail-closed arm for a done-check answer outside quarry's four-value status vocabulary,
//     mirroring the Create inversion's.
package planglyph
