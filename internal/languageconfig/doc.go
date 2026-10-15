// Package languageconfig owns the hub-wide _lyx/config/language.yaml, the skills every code-writing or code-reviewing role loads after its own.
//
// The module has one key, skills, an ordered list of skill names, classified Guard so a worktree's local override cannot set it.
// lyx hard-codes no language: the template's list is empty, and the operator names the hub's skills.
//
// Load degrades to the empty list when no board directory is told (a standalone run reads no worktree file) or when the hub holds no language.yaml.
// An empty list leaves every role's skills exactly as without the module.
// It never checks that a named skill exists.
//
// A content failure (an unparseable file, a local override holding skills, a skills value that is not a list of strings, an empty entry or one with leading or trailing whitespace) returns an error marked configengine.ErrInvalid that wraps ErrUnusable, names the key and ends with the way forward: fix it with "lyx config language" from the prime.
// A file that is present but unreadable returns the plain read error.
//
// AppendSkills merges a role's own skill list with the language list: the role's skills in order, then each language skill not already present.
// Load never resolves cwd itself: the caller hands it the directories (see PATTERN-cwd-resolution).
package languageconfig
