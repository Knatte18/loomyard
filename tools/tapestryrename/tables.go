// tables.go holds the rename as data: the file moves, the token substitutions and the excluded paths.
// It also holds substitute, which applies the substitution table to one file's text.

package main

import "strings"

// move is one file rename, both paths slash-separated and relative to the repository root.
type move struct {
	from string
	to   string
}

// moves lists the files whose names say loom for the tapestry recipe.
var moves = []move{
	{"contracts/recipes/loom-recipe.yaml", "contracts/recipes/tapestry-recipe.yaml"},
	{"internal/loomshed/loompreflight.go", "internal/loomshed/tapestrypreflight.go"},
	{"internal/loomshed/loompreflight_test.go", "internal/loomshed/tapestrypreflight_test.go"},
}

// identifierSubstitutions maps each identifier or file-name token that names the tapestry recipe onto its new spelling.
// The match is case exact.
var identifierSubstitutions = map[string]string{
	"RecipeLoom":      "RecipeTapestry",
	"LoomRecipe":      "TapestryRecipe",
	"loom-recipe":     "tapestry-recipe",
	"LoomRetiredRows": "TapestryRetiredRows",
	"Loom-Preflight":  "Tapestry-Preflight",
	"LoomPreflight":   "TapestryPreflight",
	"loomPreflight":   "tapestryPreflight",
	"loompreflight":   "tapestrypreflight",
}

// phraseSubstitutions maps each prose phrase that says loom for the tapestry recipe onto its new wording.
// The phrases are written in lower case, and each also applies with its first letter upper-cased.
var phraseSubstitutions = map[string]string{
	"loom run":      "tapestry run",
	"loom runs":     "tapestry runs",
	"loom recipe":   "tapestry recipe",
	"loom recipes":  "tapestry recipes",
	"loom task":     "tapestry task",
	"loom tasks":    "tapestry tasks",
	"loom and darn": "tapestry and darn",
	"loom or darn":  "tapestry or darn",
	"loom, darn":    "tapestry, darn",
	"darn and loom": "darn and tapestry",
	"darn or loom":  "darn or tapestry",
}

// valueSubstitutions maps each fragment that gives the tapestry recipe the value `loom` onto its new spelling.
// The match is case exact.
// Each fragment carries the context that tells the recipe name from the `loom` config module, which keeps its name:
// a seed's `recipe` key;
// a `--recipe` flag;
// a board task's recipe;
// a run profile's recipe block;
// or a test helper or table that takes a recipe.
var valueSubstitutions = map[string]string{
	`--recipe loom`:                                      `--recipe tapestry`,
	`"--recipe", "loom"`:                                 `"--recipe", "tapestry"`,
	`"recipe":"loom"`:                                    `"recipe":"tapestry"`,
	`task["recipe"] != "loom"`:                           `task["recipe"] != "tapestry"`,
	`want [bug]/loom`:                                    `want [bug]/tapestry`,
	`Recipe: "loom"`:                                     `Recipe: "tapestry"`,
	`seed.Recipe != "loom"`:                              `seed.Recipe != "tapestry"`,
	`seed.Recipe, "loom")`:                               `seed.Recipe, "tapestry")`,
	`seeded with recipe "loom"`:                          `seeded with recipe "tapestry"`,
	`seedText("loom")`:                                   `seedText("tapestry")`,
	`seedBoardTask(t, h, slug, "loom")`:                  `seedBoardTask(t, h, slug, "tapestry")`,
	`seedBoardTask(t, h, "b", "loom")`:                   `seedBoardTask(t, h, "b", "tapestry")`,
	`newSeedChildDeps("loom"`:                            `newSeedChildDeps("tapestry"`,
	`typeAtWiring: "loom"`:                               `typeAtWiring: "tapestry"`,
	`wantRecipe:  "loom"`:                                `wantRecipe:  "tapestry"`,
	`"some-slug", "loom"`:                                `"some-slug", "tapestry"`,
	`seedRecipe("loom")`:                                 `seedRecipe("tapestry")`,
	`recipes["loom"]`:                                    `recipes["tapestry"]`,
	`"step", "loom", "run"`:                              `"step", "tapestry", "run"`,
	`{"loom", loomcli.BootstrapVerb`:                     `{"tapestry", loomcli.BootstrapVerb`,
	`{"batten", "darn", "loom", "pawl"}`:                 `{"batten", "darn", "pawl", "tapestry"}`,
	`exactly batten, darn, loom and pawl`:                `exactly batten, darn, pawl and tapestry`,
	`{"loom", RecipeLoom`:                                `{"tapestry", RecipeTapestry`,
	`{RecipeBatten, RecipeDarn, RecipeLoom, RecipePawl}`: `{RecipeBatten, RecipeDarn, RecipePawl, RecipeTapestry}`,
	`recipe = "loom"`:                                    `recipe = "tapestry"`,
	`{"loom": 2, "darn": 1}`:                             `{"tapestry": 2, "darn": 1}`,
	`"loom %d"`:                                          `"tapestry %d"`,
	`| loom |`:                                           `| tapestry |`,
	`{"loom": "solid"`:                                   `{"tapestry": "solid"`,
	`"light:\n  loom:\n    loom: {plan_max_bounces: 1, code_max_bounces: 1, merge_max_bounces: 1}`: `"light:\n  tapestry:\n    loom: {plan_max_bounces: 1, code_max_bounces: 1, merge_max_bounces: 1}`,
	`lie over the loom block`:                            `lie over the tapestry block`,
	"`\"docs\"`, \"no loom block\"":                      "`\"docs\"`, \"no tapestry block\"",
	`1, 0, []string{"loom"}}`:                            `1, 0, []string{"tapestry"}}`,
	`[]string{"loom", "darn", "pawl"}`:                   `[]string{"tapestry", "darn", "pawl"}`,
	`loom, err := profiles.Settings(tt.profile, "loom")`: `tapestry, err := profiles.Settings(tt.profile, "tapestry")`,
	`keys := loom["loom"]`:                               `keys := tapestry["loom"]`,
	`{"loom": loom["loom"]`:                              `{"tapestry": tapestry["loom"]`,
	`profiles.Settings("docs", "loom")`:                  `profiles.Settings("docs", "tapestry")`,
	`Settings(docs, loom) error = nil; want a refusal for the missing loom block`: `Settings(docs, tapestry) error = nil; want a refusal for the missing tapestry block`,
	`"docs", "loom", []string{`: `"docs", "tapestry", []string{`,
	`profiles.Default("loom")`:  `profiles.Default("tapestry")`,
}

// pathSubstitutions maps a slash-separated path relative to the root onto substitutions that apply only in that file.
// Each substitution keeps its sorted list sorted once a name in it is renamed.
var pathSubstitutions = map[string]map[string]string{
	"internal/shedrecipe/registry_test.go": {
		"\t\t\t\"LoomPreflight\",\n\t\t\t\"MultiLLM\",\n": "\t\t\t\"MultiLLM\",\n",
		"\t\t\t\"Stub\",\n\t\t\t\"WorktreeCreate\",\n":    "\t\t\t\"Stub\",\n\t\t\t\"TapestryPreflight\",\n\t\t\t\"WorktreeCreate\",\n",
	},
}

// excludedPaths lists the slash-separated path prefixes substitute never reads.
// They cover version control and run records;
// the measurement reports, which record past runs;
// this program and the earlier rename program, whose texts describe their own runs;
// and the archived README.
var excludedPaths = []string{
	".git/",
	"_lym/",
	".lym/",
	"docs/research/",
	"tools/tapestryrename/",
	"tools/wordswap/",
	"README.old.md",
}

// substitute replaces in text every whole-token occurrence of a key of keys with its value.
// At each position the longest key wins.
// A key starting with a lower-case letter starts a token only after a non-alphanumeric byte or at the text's start.
// Any other key starts a token anywhere, such as at a camelCase word start.
// A token ends at the end of the text, before a non-alphanumeric byte, or before an upper-case letter.
// A key ending with a non-alphanumeric byte ends a token anywhere.
// No key starts right after `lym `, so a verb such as `lym loom run` keeps its name.
// The output is never re-scanned, so a replacement is never matched again.
func substitute(text string, keys map[string]string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		key, ok := matchAt(text, i, keys)
		if !ok {
			out.WriteByte(text[i])
			i++
			continue
		}
		out.WriteString(keys[key])
		i += len(key)
	}
	return out.String()
}

// substitutionKeys merges the tables that apply to the file at the slash-separated path rel:
// the identifier and value tables, every phrase in its lower and its capitalised form, and the path's own table.
func substitutionKeys(rel string) map[string]string {
	keys := map[string]string{}
	for _, table := range []map[string]string{identifierSubstitutions, valueSubstitutions, pathSubstitutions[rel]} {
		for from, to := range table {
			keys[from] = to
		}
	}
	for from, to := range phraseSubstitutions {
		keys[from] = to
		keys[capitalise(from)] = capitalise(to)
	}
	return keys
}

// matchAt returns the longest key that occurs in text at i as a whole token.
func matchAt(text string, i int, keys map[string]string) (string, bool) {
	best := ""
	for key := range keys {
		if len(key) <= len(best) || !strings.HasPrefix(text[i:], key) {
			continue
		}
		if !tokenStart(text, i, key) || !tokenEnd(text, i+len(key), key) {
			continue
		}
		best = key
	}
	return best, best != ""
}

// tokenStart reports whether key may start a token at i in text.
func tokenStart(text string, i int, key string) bool {
	if strings.HasSuffix(text[:i], "lym ") {
		return false
	}
	return i == 0 || !isAlphanumeric(text[i-1]) || !isLower(key[0])
}

// tokenEnd reports whether key may end a token at end in text.
func tokenEnd(text string, end int, key string) bool {
	return !isAlphanumeric(key[len(key)-1]) || end == len(text) || !isAlphanumeric(text[end]) || isUpper(text[end])
}

// capitalise upper-cases the first byte of s, an ASCII letter.
func capitalise(s string) string {
	return strings.ToUpper(s[:1]) + s[1:]
}

func isAlphanumeric(b byte) bool {
	return isUpper(b) || isLower(b) || (b >= '0' && b <= '9')
}

func isUpper(b byte) bool {
	return b >= 'A' && b <= 'Z'
}

func isLower(b byte) bool {
	return b >= 'a' && b <= 'z'
}
