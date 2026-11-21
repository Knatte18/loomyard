# PATTERN-code-conventions

Writing code: a package's `doc.go` documents only that package; a name or path that can go stale is one named constant; no language, engine or code branch is hard-coded, and a branch lym owns in its records namespace is one named constant in fabric.

## Go documentation

Rule: Writing a package's `doc.go`: it documents the package; a rule other code must follow is a PATTERN entry, never a `doc.go` paragraph.

## Named constants

Rule: Writing a name or path that can go stale: one named constant, declared once and referenced everywhere, never a repeated literal.

## No hard-coded choice

Rule: Choosing a language, engine or code branch: lym hard-codes none; it is read from config, the repository's git state or a table row keyed by name.

lym runs on any repository, so no code path assumes one language, one agent engine or one branch.

- **Language.** A language is a row in a table keyed by its name, read through that table; no code path branches on a language literal outside it.
  A new language is a new row, never a new branch in the callers.
- **Engine.** An agent engine is chosen by config and reached through shuttle's provider seam; no caller names an engine.
- **Branch.** The rule governs code branches: a code branch name is read from git or from lym's own records,
  such as the parent branch, the default branch or the pairing's suffixed name.
  No code path names `main` or any other code branch.
  A branch lym itself owns inside its records namespace is a named constant defined once in fabric, never a literal at a use site.
  Two are such: the one-repository board branch `main` under `refs/lym/heads/`, and the `lym-bolt` branch of the retired layout that the clone's refusal names.
  Neither is read from git by design, and only fabric knows the hub layout.
- An existing literal of any of the three is debt: a change that touches it moves it into the table, config or git read, and never adds another.
