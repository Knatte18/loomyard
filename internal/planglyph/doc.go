// Package planglyph is the sole owner of every quarry.Repo call (Open, Resolve, EncloseAt, DeltaGit).
// It is also the sole owner of the package-level quarry.Name.
// It runs the resolve-backed validation pass layered on top of internal/planparser's pure checks.
//
// planglyph derives no path of its own and never imports internal/lymcwd.
// Its two entry points, ValidateFormat and Validate, take worktreeRoot as planparser.Validate(plan, worktreeRoot) does:
// quarry.Open needs an absolute root, so the root is told rather than derived.
//
// planglyph composes internal/planparser's pure checks rather than reimplementing any of them.
// No check exists in both packages.
// ValidateFormat calls planparser.ValidateFormat and Validate calls planparser.Validate;
// each converts every finding,
// and each appends the same resolve-backed findings via one shared resolvePass and then planGatePass.
// ValidateDispatch shares resolvePass but never runs planGatePass, whose checks run at the plan gates only.
// ValidateFormatAfter is the plan gates' form for a plan some of whose cards webster already ran.
// Those are the cards the index's ValidateFormat is told are done.
// It returns ValidateDispatch's answer with those cards completed, plus planGatePass over the cards not done,
// so a done card's landed work is never reported as a defect of the plan.
// With no done card it is ValidateFormat.
//
// A caller's answer is a three-way split.
// Pure findings come from planparser and are stamped SeverityBlocking.
// Resolve findings come from this package's passes, with either severity.
// The third part is an infrastructure error.
// That error is ErrQuarryUnavailable, returned alongside whatever pure findings were already collected,
// so a quarry outage is never mistaken for a clean answer.
// Degrading to format-only validation with a warning would let a plan look validated when it was not;
// an informational error would hide the outage at the boundaries whose job is to be mechanical.
//
// The finding types and ErrQuarryUnavailable are declared in internal/planindex, which links no tree-sitter;
// this package aliases them, so errors.Is matches under either name.
// The index's format, rework and dispatch validation append fabric-vocabulary-name.
// That finding (planparser.CheckFabricVocabularyName) reports a declared name the fabric-vocabulary scan would refuse.
// Dispatch drops its findings against completed cards.
// The index's format and rework validation also append card-fabric-reference and edit-test-file.
// Dispatch does not run those two; edit-test-file is planparser.CheckEditTestFile.
// edit-test-file admits a test file quarry's toc lists with no symbol, since no member glyph can name the edit there.
// NewIndex returns the real planindex.Index.
// A package that calls the code index receives it from the CLI layer instead of importing planglyph.
//
// The list below is the canonical list of every resolve-backed Finding.Check ID this package can raise.
// It parallels the plan gate's check catalogue in contracts/specs/loom-plan-checks-spec.md.
//
//   - glyph-not-found, glyph-ambiguous, glyph-rejected — the resolve status policy, blocking,
//     over every glyph target that neither a Create group owns nor a Rename pair names as its New side.
//     A rename destination exists only after the card runs, as planparser's path-missing rule never checks Pairs.New.
//     glyph-rejected is also the fail-closed arm of the Create inversion:
//     a Create target is excluded from the status policy,
//     so an answer neither policy understands would otherwise have no reader.
//     Under a part scope resolvePass treats Part.Paths as forthcoming:
//     a self glyph of a file another part creates is left out of the status policy as a Create target is.
//     A member whose every declaration sits in files no single build environment selects together is one member,
//     so a partitioned ambiguous answer resolves;
//     the detail of an unpartitioned one names each candidate's file and build constraint.
//     Build-constraint partitioning reads `.go` candidate sets only:
//     a candidate set holding any other file is never partitioned,
//     and its files are described as carrying no constraint, so a name such as `Foo_windows.cs` is never read as one.
//     A self glyph that two alphabets both match is an ambiguous answer that lists languages and no candidates.
//     Its detail names those languages and the way forward:
//     name the file as a slashed path self glyph, or a member as `lym quarry glyphs` lists them.
//     resolveVerdict holds the policy shared by statusFindings and the index's ResolveGlyphs,
//     so a glyph resolves for one exactly when it resolves for the other.
//     ResolveGlyphs backs the discussion citation check and the plan seam-test and `doc.go` checks.
//     Each fail-closed status policy in the package raises glyph-rejected at its own bullet below,
//     and the double report one anomalous target can produce across two of them is accepted by design.
//   - create-already-exists (blocking), create-new-unit (informational) — the Create inversion,
//     over every Create group's own targets, handle-shaped or glyph-shaped alike.
//     create-already-exists covers found, multipart and ambiguous alike:
//     all three mean a declaration already occupies the name the card is creating.
//     An ambiguous target is create-already-exists whether or not build constraints partition its candidates,
//     and its detail names each candidate's file and build constraint.
//   - delete-before-reference (blocking for a resolved reference, informational for a name-only method match) —
//     a card that deletes a symbol whose reference a later card's Edit or Extends code still holds,
//     so the delete must move after that card.
//     The delete-order edit regions cover Extends groups as they cover Edit groups.
//     callerCoverageFindings computes it through planGatePass at the plan gates,
//     from the reference computation caller-uncovered shares;
//     LaterDeleteReferences computes it scan-only at ValidateDispatch and for Webster.
//     Only Go files outside the directories the go tool ignores are searched;
//     a package self glyph Delete keeps its quoted import-path match, over Go regions only.
//     A plan gate reads the tree as it stands.
//     It first sees at ValidateDispatch a reference that an earlier card writes during the run.
//     The same holds for a reference inside a later card's Edit target that is still a handle.
//     A member reference that an Edit target of the deleting card or an earlier card also covers is not reported;
//     that card rewrites the code before the later card runs.
//     The quoted import-path match keeps no such exemption.
//     LaterDeleteReferences raises glyph-rejected for a Delete or Edit member whose answer it cannot read.
//     A member partitioned by build constraints is looked up through its candidates, which are its declarations.
//   - resign-head-mismatch (blocking) — nameResigns,
//     an Edit re-sign arrow whose head quarry.Name cannot name or names as a member other than the arrow's own glyph.
//     It reads plan text alone, so it runs wherever resolvePass does, ValidateDispatch included.
//     nameResigns names every arrow before the plan's batched resolve.
//     nameDeclarations is the package's one quarry.Name site, shared with CanonicalizeHandles,
//     and resolveTargets is its one resolve site.
//     A Go arrow is accepted when the minted id is its target's own.
//     A C# arrow is accepted when the minted id is its target's apart from the parameter list;
//     both ids are written with parentheses or both without, since a re-sign changes the parameter list and so the id.
//     A C# arrow's way forward depends on how the head failed:
//     correct the head to one quarry names under the owner chain;
//     keep the parameter list or change the member's kind through a Delete and a Create;
//     or use a Rename pair for a name change.
//   - resign-overload-collision (blocking) — resignCollisionFindings:
//     an accepted C# arrow whose minted id differs from its target and collides with one of these:
//     an existing overload that resolves found;
//     a Create or Rename handle that binds to the same id;
//     or another arrow's minted id on a different target.
//     The finding is attributed to the later of the two cards and names the earlier;
//     it is lifted when a card between the two declarations deletes, renames away or re-signs away the colliding member.
//     A card names a re-signed C# member by its old glyph until the re-sign is recorded.
//   - resign-not-done (blocking) — RebindResigns, reached from the post-batch pass of record-batch and recover-batch:
//     a recorded C# arrow whose minted id does not resolve found in the tree.
//     The card's edit then does not match its arrow.
//     It fails closed, because only a found answer shows the edit declares the arrow's head, and it rewrites nothing.
//     When every minted id resolves found, one plan.RewriteRefs call rewrites every old glyph to its minted id.
//     The rewrite is plan-wide, so a later card naming the member by its old glyph names the minted id.
//     A Go arrow, an arrow whose minted id is its target and a plan whose language opts out of glyphs do nothing.
//   - plan-language-mismatch (blocking) — languageMismatchFindings,
//     the resolve-backed half of the check planparser also makes from plan text:
//     a go plan whose targets resolve into `.cs` files and no `.go` file.
//     It is plan-level and runs only when no card is done.
//     It never fires when a card creates a declaration whose head opens with a Go keyword.
//     It also never fires when a non-path target other than a Create target has no found or multipart answer;
//     that glyph's own finding stands.
//   - redundant-file-target (blocking) — planGatePass:
//     a card lists a file self glyph beside a member glyph that resolves into that file.
//     A partitioned member's candidates are its declarations, so a file holding one of them counts.
//     It runs at the plan gates only: ValidateFormat, ValidateFormatAfter, Validate and so ValidateRework.
//     ValidateFormatAfter runs it over its cards not done, and ValidateDispatch never runs it.
//     A check whose verdict depends on state the run itself changes runs at the plan gates only;
//     dispatch re-validates a plan against a tree the run has already changed.
//   - resign-interface-method (blocking) — planGatePass:
//     an Edit re-sign arrow on a member that resolves to an interface method, whose own spec is no declaration.
//     It reads the tree, so like redundant-file-target it runs at the plan gates only.
//   - caller-uncovered (blocking for a resolved reference, informational for a name-only method match) —
//     callerCoverageFindings, reached through planGatePass:
//     a deleted or re-signed member that Go code still references with no admissible card's target covering that code.
//     A partitioned member's candidates are its declarations, so each is a subject's span and a target's span alike.
//     It runs only when a subject exists and at the plan gates only.
//     planGatePass reports none when plan.Part is set;
//     the verdict rests on cards outside the part, and the joined plan's plan gate checks it.
//     A typesLoader loads the root module's packages through go/packages, offline, under a timeout, and logs the load.
//     The load includes test variants and the tier0 through tier3 tags.
//     An identifier whose used object is the member is a resolved reference.
//     An index built by NewSlottedIndex with a pool slots the load.
//     It writes a wait record into the told wait directory and acquires a hub gate slot;
//     the record is dropped once the acquire returns.
//     The timeout starts only once the slot is held.
//     go list runs under the slot's `-p` cap, and the slot is released when the load returns.
//     NewIndex, standalone runs and a nil pool load unslotted.
//     The load is skipped when the worktree root holds no go.mod.
//     A failed or timed-out load is logged and leaves every file to the scan, never an error.
//     A slot that cannot be acquired, as over an absent or invalid gate.yaml or machine override, is a failed load:
//     the plan gate degrades to the scan rather than refusing,
//     and the run's next slotted gate refuses on the same file with its way forward.
//     The scan walks every Go file under the worktree root by token, nested modules included.
//     It covers each file the load did not check.
//     It also covers each line of a checked file that has an identifier without type information.
//     In the scan a package-level member is resolved by import path:
//     a reference is its bare identifier inside the declaring package;
//     elsewhere it is an import of the member's import path (aliased or dotted included) followed by the identifier.
//     A method is matched by name alone, except after an imported package's name,
//     and a subject's name-only matches collapse into one informational finding naming every file.
//     The import-name exclusion also drops a method call on a local variable that shadows an imported package's name.
//     This happens in the scan only.
//     A call through an interface whose method set holds the method is not reported once the load answers.
//     The untagged test tier replaces defaultTypesLoader with a loader that fails without spawning;
//     only the integration tier runs go list.
//     A re-signed member admits its own card's targets; a deleted member admits its own card and every earlier one,
//     and its reference inside a later card's Edit code stays delete-before-reference's.
//   - delete-target-gone (informational) — downgradeGoneDeleteTargets:
//     a Delete target of a pending card that is already absent.
//     Only ValidateDispatch reports it, and so ValidateFormatAfter once a batch is begun or done.
//     It replaces the blocking path-missing or glyph-not-found finding for that target.
//   - handle-name-failed, handle-canonical-collision (both blocking) — CanonicalizeHandles:
//     a declaration that fails to parse, or two draft handles that canonicalize to the same glyph.
//     A handle's alphabet is the Lang of planparser.ParseRef over its body.
//     A C# Create handle is `plan:<namespace>#<owner.member>`.
//     Its declaration takes that namespace and the owner chain as `Owner`, less the head's explicit-interface qualifier.
//     The qualifier is the longest run of owner components that the head spells dotted before the declared name.
//     Components are compared in the canonical C# spelling of type arguments,
//     so `IPair<string, int>.First` qualifies the owner component `IPair<string,int>`.
//     A body the C# alphabet rejects is one handle-name-failed, and quarry is not asked to name it.
//     A head whose qualifier quarry refuses for non-canonical spelling names the canonical spelling as the way forward.
//   - rename-old-unresolved (blocking) — renameDeclSource, a Rename pair's Old side that does not resolve found,
//     so no declaration can be derived for its handle-shaped New side.
//     An old side declared once per build-constraint set stays unresolved:
//     a rename derives from exactly one declaration.
//     Its detail names each candidate's file and constraint, and the way forward.
//     The way forward is a Delete of the old member and one Create declaration of the new member's handle.
//     The card with the Delete edits each of the old member's files.
//     The alternative is an Edit of each file's body.
//     renameCSharpHead derives a C# symbol's declaration from its signature,
//     under the old symbol's unit and owner chain.
//     It replaces the declared name token with the new handle's name without its generic arity suffix.
//     A field, const or event gets a one-declarator head.
//     A member that no identifier names is rename-old-unresolved, with dropping the pair as the way forward.
//     Such members are an operator, an indexer, a finalizer and a constructor.
//   - bind-count-mismatch (blocking) — BindHandles:
//     a card whose record-batch delta matched fewer handles than the card owns.
//     Its own handles are its Create declarations and any Rename pair's still-handle-shaped New side.
//     planparser.OwnedHandles lists them.
//     Under a go plan, a handle the delta matches only in its directory's other package binds to that glyph.
//     That package is `<dir>_test` for `<dir>` and the reverse (planparser.GoPackageCounterpart).
//   - plan-references-deleted-symbol (blocking), rename-candidate (informational) — DetectDrift,
//     the exact-tier and evidence-tier halves of drift detection.
//     A rename the batch's implementer reports is repaired as an exact-tier one is, under the amendment tier `reported`,
//     and its old glyph gets neither finding;
//     a reported new glyph that does not resolve is the post-repair resolve's blocking finding.
//   - later-target-removed (blocking) — RemovedTargets:
//     a symbol a batch's delta deleted that a pending card targets or uses.
//     It is not reported when quarry renamed the symbol exactly;
//     when the batch's implementer reported its rename;
//     or when a card of the batch declares it as a Delete target or a Rename pair's old side.
//     It reads and writes nothing, and its finding names any evidence-tier rename candidates.
//   - scope-outside-plan (informational) — ScopeGuard:
//     a symbol a completed batch's delta touched outside its own cards' targets.
//     A file target covers every symbol in that file;
//     a package target covers every symbol in a file directly in its directory.
//     It also reports a changed file for which the delta gives no symbols and no card target covers.
//     That check is file-granular: a target's file or package directory lets a symbol-less change through.
//     ScopeGuard receives the plan, so it parses each target in the plan's alphabet.
//   - create-not-done, delete-not-done, rename-not-done (all blocking) — DoneChecks:
//     a Create target that still does not resolve;
//     a Delete target that still does;
//     or a Rename pair whose old side still resolves or whose new side still does not;
//     all judged after the batch that was supposed to build, remove, or rename it.
//     A Create target under the root's git-ignored `.scratch/` is a transient draft that its card may remove once read,
//     so it gets no done-check;
//     a `.scratch/` below the root, a Rename new side and every Delete target keep theirs.
//     A target still path-shaped after canonicalization is judged on disk instead of resolved.
//     No glyph spells such a target, as with a path whose directory holds a space.
//     Its path must exist for a Create or Rename new side,
//     and it must not exist for a Delete or Rename old side (planparser.IsDiskCheckedTarget).
//     Each detail carries the lookup: the resolve key beside the card's own spelling, and quarry's status.
//     For not_found it says whether the unit exists.
//     For ambiguous it names each candidate's file and build constraint.
//     Under a go plan, a Create or Rename new side answered not_found is done in one case:
//     the worktree's glyphs listing holds its member in its directory's other package.
//     A plan cannot tell which package a new test file declares.
//     The finding for a Create or Rename new side answered not_found also names how many files declare its unit.
//     It also names the nearest few declarations of its name elsewhere in the worktree.
//     Both come from one glyphs listing, taken only when such a finding is due.
//     signature-mismatch (blocking) replaces that finding when the unit declares the name with other signatures only.
//     It leads with each declared signature and its file.
//     An ambiguous answer whose candidates sit in files no single build environment selects together counts as resolved,
//     so a Create or a Rename new side over such a member lands;
//     the Delete direction and a Rename old side keep blocking on any surviving declaration.
//     DoneChecks raises glyph-rejected for a done-check answer outside quarry's status vocabulary,
//     as the Create inversion does.
package planglyph
