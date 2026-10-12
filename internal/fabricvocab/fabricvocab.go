// fabricvocab.go holds the fabric-vocabulary predicates and their data.
// That is the side tokens, the owner set, the doc roots and the bare-token checks.

package fabricvocab

import (
	"regexp"
	"strings"
)

// ConfigsyncOwnerDir is the owner row with a narrower carve-out than the rest.
// internal/configsync may name the side tokens in string literals and comments, but not in identifiers.
// Its literals are the on-disk legacy config filenames its migration must read by name.
const ConfigsyncOwnerDir = "internal/configsync"

// owners is the set of directories permitted to use the bare side tokens.
// It governs the bare-token rule only; the fabric-sense host-phrase rule applies inside these directories too.
// internal/hubforge is a directory of non-test .go files that names fabric's own geometry,
// so it owns the tokens the same way internal/fabricengine does.
// internal/fabricvocab spells the tokens it bans.
var owners = map[string]bool{
	"internal/fabricengine": true,
	"internal/fabriccli":    true,
	"internal/weftname":     true,
	"internal/gitkit":       true,
	"internal/boardengine":  true,
	ConfigsyncOwnerDir:      true,
	"internal/hubforge":     true,
	"internal/fabricvocab":  true,
}

// docRoots are the roots whose .md, .yaml and .go files the bare-token scan covers.
var docRoots = map[string]bool{"contracts": true, "docs": true, "plugins": true, "crucible": true, "cmd": true}

// hostGeometryIdentifiers are the fabric-geometry identifiers that compound the retired host noun with a repo or geometry noun.
// They are matched case-insensitively against an identifier's name.
var hostGeometryIdentifiers = map[string]bool{
	"hostbranch":    true,
	"hostlayoutfor": true,
	"hostreason":    true,
	"hostjunction":  true,
	"hostclean":     true,
}

// sideTokens are the two fabric side tokens, lower case.
var sideTokens = []string{"weft", "warp"}

// packageNamePattern matches the side-named name package's spelling as a whole word.
var packageNamePattern = regexp.MustCompile(`\bweftname\b`)

// recordsRepoNamePattern matches the proper name of this repository's public records repository as a whole word.
// The repo-root docs link that repository by name and URL; the name is not a side-token use.
var recordsRepoNamePattern = regexp.MustCompile(`\bloomyard-weft\b`)

// SideTokens returns a fresh copy of the two fabric side tokens,
// so a test outside the owner set can build a side-token name without spelling one.
func SideTokens() []string {
	return append([]string(nil), sideTokens...)
}

// BareToken reports whether s contains a side token anywhere, case-insensitively, as a substring.
// Substring matching is deliberate: it catches the token inside a camelCase identifier, not only a standalone word.
func BareToken(s string) bool {
	return BareTokenIn(s, false)
}

// BareTokenIn is BareToken with the side-named name package's spelling stripped first when stripPackageName is set.
// That serves a file whose import of that package another rule already governs.
func BareTokenIn(s string, stripPackageName bool) bool {
	if !containsSideToken(strings.ToLower(s)) {
		return false
	}
	if stripPackageName {
		s = packageNamePattern.ReplaceAllString(s, "")
	}
	return containsSideToken(strings.ToLower(s))
}

// BareTokenInRootDoc is BareToken for a repo-root markdown file:
// it strips the records repository's proper name first, so that name passes and every other side token still fails.
func BareTokenInRootDoc(s string) bool {
	return BareToken(recordsRepoNamePattern.ReplaceAllString(s, ""))
}

func containsSideToken(lower string) bool {
	for _, token := range sideTokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

// IsHostGeometryIdentifier reports whether name, case-insensitively, is a fabric-geometry identifier that compounds the retired host noun.
func IsHostGeometryIdentifier(name string) bool {
	return hostGeometryIdentifiers[strings.ToLower(name)]
}

// IsOwner reports whether the slash-separated directory dir is in the owner set.
func IsOwner(dir string) bool {
	return owners[dir]
}

// SkipsBareCheck reports whether dir is exempt from the bare-token check entirely:
// every owner row except ConfigsyncOwnerDir, whose narrower carve-out FailsBareCheck applies.
func SkipsBareCheck(dir string) bool {
	return owners[dir] && dir != ConfigsyncOwnerDir
}

// FailsBareCheck applies the owner-set carve-outs to a file's bare-token hits.
// ConfigsyncOwnerDir passes on a literal-or-comment-only hit and fails on an identifier hit.
// Every other directory reaching it is a non-owner, so it fails on either kind of hit.
func FailsBareCheck(dir string, bareIdent, bareLiteralOrComment bool) bool {
	if dir == ConfigsyncOwnerDir {
		return bareIdent
	}
	return bareIdent || bareLiteralOrComment
}

// IsDocRoot reports whether first, a path's first segment, is a doc root.
func IsDocRoot(first string) bool {
	return docRoots[first]
}
