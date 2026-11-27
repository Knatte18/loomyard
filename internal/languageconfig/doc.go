// Package languageconfig owns the hub-wide _lym/config/language.yaml:
// the skills every code-writing or code-reviewing role loads after its agents.yaml list,
// and the hub's structure-aware rename tools.
//
// The module has two keys, skills and rename_tools, each an ordered list of strings.
// Each is classified Guard, so a worktree's local override cannot set it.
// lym hard-codes no language: the template's lists are empty, and the operator names the hub's skills and rename tools.
//
// A `verify_tiers` key is Guard-classified:
// a load drops it from the stacked document, and reconcile prunes it from the hub file.
// The hub's tiers and gate lists live in gate.yaml.
//
// Load degrades to every list empty when no board directory is told or when the hub holds no language.yaml.
// A standalone run, which has no board directory told, reads no worktree file.
// An existing hub file without rename_tools loads with the empty list.
// The layered load fills a missing template key.
// An empty skills list leaves every role's skills as without the module.
// It never checks that a named skill exists.
//
// A content failure is an unparseable file, a local override holding any key, or a value that is not a list of strings.
// An entry that is empty, has leading or trailing whitespace, or in rename_tools holds a line break is one too.
// A content failure returns an error marked configengine.ErrInvalid that wraps ErrUnusable.
// The error names the key where there is one and ends with the way forward:
// fix it with "lym config language" from the prime.
// A file that is present but unreadable returns the plain read error.
//
// Each rename_tools entry is one rule naming the hub's structure-aware rename tool for one language and how to run it.
// BulletListValue renders the list as the marker value the Webster implementer and recovery prompts fill:
// the entries verbatim and in order as "- " bullets, or "none" for the empty list.
// The empty list means no tool, so a plan with a symbol Rename pair is refused.
//
// A Go hub sets rename_tools to a rule built on gopls, such as:
//
//   - For a Go symbol, run `gopls rename -w <file>:#<offset> <new name>` for each symbol pair from the worktree root.
//
// A Python-based rename tool needs shuttle's deny_python off, since the shell guard's python row refuses it otherwise.
//
// A role's own skills come from agents.yaml; shuttle loads the language skills after them, dropping repeats.
// Load never resolves cwd itself: the caller hands it the directories (see PATTERN-cwd-resolution).
package languageconfig
