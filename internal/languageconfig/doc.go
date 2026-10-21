// Package languageconfig owns the hub-wide _lyx/config/language.yaml: the skills every code-writing or code-reviewing role loads after its own and the hub's structure-aware rename tools.
//
// The module has two keys, skills and rename_tools, each an ordered list of strings and each classified Guard so a worktree's local override cannot set it.
// lyx hard-codes no language: the template's lists are empty, and the operator names the hub's skills and rename tools.
//
// `verify_tiers` is a retired key, Guard-classified: a load drops it from the stacked document and reconcile prunes it from the hub file.
// The hub's tiers and gate lists live in gate.yaml, and the prompts that once rendered `verify_tiers` render them from there.
//
// Load degrades to every list empty when no board directory is told (a standalone run reads no worktree file) or when the hub holds no language.yaml.
// An existing hub file without rename_tools loads with the empty list, because the layered load fills a missing template key.
// An empty skills list leaves every role's skills exactly as without the module.
// It never checks that a named skill exists.
//
// A content failure (an unparseable file, a local override holding any key, a value that is not a list of strings, an empty entry, one with leading or trailing whitespace, or a rename_tools entry holding a line break) returns an error marked configengine.ErrInvalid that wraps ErrUnusable, names the key where there is one and ends with the way forward: fix it with "lyx config language" from the prime.
// A file that is present but unreadable returns the plain read error.
//
// Each rename_tools entry is one rule naming the hub's structure-aware rename tool for one language and how to run it.
// BulletListValue renders the list as the marker value the Webster implementer and recovery prompts fill: the entries verbatim and in order as "- " bullets, or "none" for the empty list.
// The empty list means no tool, so a plan with a symbol Rename pair is refused.
//
// A Go hub sets rename_tools to a rule built on gopls, such as:
//
//   - For a Go symbol, run `gopls rename -w <file>:#<offset> <new name>` for each symbol pair from the worktree root.
//
// A Python-based rename tool needs shuttle's deny_python off, since the shell guard's python row refuses it otherwise.
//
// AppendSkills merges a role's own skill list with the language list: the role's skills in order, then each language skill not already present.
// Load never resolves cwd itself: the caller hands it the directories (see PATTERN-cwd-resolution).
package languageconfig
