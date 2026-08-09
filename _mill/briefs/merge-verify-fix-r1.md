# Verify-Fix Brief

The verify command `go build ./... && go test ./...` failed after a merge.
Your job is to diagnose the failures and fix the code so the verify command passes.

## Verify Output

```
ok  	github.com/Knatte18/loomyard/cmd/lyx	(cached)
?   	github.com/Knatte18/loomyard/cmd/testtiming	[no test files]
ok  	github.com/Knatte18/loomyard/internal/batcher	(cached)
ok  	github.com/Knatte18/loomyard/internal/boardcli	(cached)
ok  	github.com/Knatte18/loomyard/internal/boardengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/boardengine/boardtest	(cached)
ok  	github.com/Knatte18/loomyard/internal/burlercli	(cached)
ok  	github.com/Knatte18/loomyard/internal/burlerengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/clihelp	(cached)
ok  	github.com/Knatte18/loomyard/internal/configcli	(cached)
ok  	github.com/Knatte18/loomyard/internal/configengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/configreg	(cached)
ok  	github.com/Knatte18/loomyard/internal/configsync	(cached)
ok  	github.com/Knatte18/loomyard/internal/envsource	(cached)
ok  	github.com/Knatte18/loomyard/internal/fabriccli	(cached) [no tests to run]
ok  	github.com/Knatte18/loomyard/internal/fabricengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/fslink	(cached)
ok  	github.com/Knatte18/loomyard/internal/fsx	(cached)
ok  	github.com/Knatte18/loomyard/internal/gitexec	(cached) [no tests to run]
ok  	github.com/Knatte18/loomyard/internal/githubclient	(cached)
ok  	github.com/Knatte18/loomyard/internal/gitignore	(cached)
ok  	github.com/Knatte18/loomyard/internal/gitrepo	(cached)
ok  	github.com/Knatte18/loomyard/internal/idecli	(cached) [no tests to run]
ok  	github.com/Knatte18/loomyard/internal/ideengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/lock	(cached)
ok  	github.com/Knatte18/loomyard/internal/logger	(cached)
ok  	github.com/Knatte18/loomyard/internal/loomengine	(cached)
--- FAIL: TestEnforcement_MarkdownLinks (0.01s)
    --- FAIL: TestEnforcement_MarkdownLinks/repo (0.01s)
        docslink_test.go:416: broken markdown link: docs/reference/plan-format.md:176  missing file  ../../manifest/designs/scout-redesign.md
        docslink_test.go:416: broken markdown link: docs/reference/plan-format.md:337  missing file  ../../manifest/designs/scout-redesign.md
        docslink_test.go:419: stale allowlist entry, delete it: docs/reference/discussion-format.md -> plan-format.md
        docslink_test.go:419: stale allowlist entry, delete it: docs/reference/plan-format-v3.md -> plan-format.md
        docslink_test.go:419: stale allowlist entry, delete it: docs/reference/status-schema.md -> plan-format.md
        docslink_test.go:419: stale allowlist entry, delete it: manifest/designs/loom.md -> ../../docs/reference/plan-format.md
        docslink_test.go:419: stale allowlist entry, delete it: docs/reference/plan-format-v3.md -> ../../manifest/designs/scout-redesign.md
FAIL
FAIL	github.com/Knatte18/loomyard/internal/lyxcwd	0.115s
?   	github.com/Knatte18/loomyard/internal/lyxdirs	[no test files]
ok  	github.com/Knatte18/loomyard/internal/lyxtest	(cached)
ok  	github.com/Knatte18/loomyard/internal/modelspec	(cached)
ok  	github.com/Knatte18/loomyard/internal/output	(cached)
ok  	github.com/Knatte18/loomyard/internal/pattern	(cached)
ok  	github.com/Knatte18/loomyard/internal/perchcli	(cached)
ok  	github.com/Knatte18/loomyard/internal/perchengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/planparser	(cached)
ok  	github.com/Knatte18/loomyard/internal/proc	(cached)
ok  	github.com/Knatte18/loomyard/internal/reedcli	(cached)
ok  	github.com/Knatte18/loomyard/internal/reedengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/reedengine/render	(cached)
ok  	github.com/Knatte18/loomyard/internal/scoutcli	(cached)
ok  	github.com/Knatte18/loomyard/internal/scoutengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/selfreportcli	(cached)
ok  	github.com/Knatte18/loomyard/internal/selfreportengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/shell	(cached)
ok  	github.com/Knatte18/loomyard/internal/shuttlecli	(cached)
ok  	github.com/Knatte18/loomyard/internal/shuttleengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/state	(cached)
ok  	github.com/Knatte18/loomyard/internal/stencil	(cached)
ok  	github.com/Knatte18/loomyard/internal/tokenvocab	(cached)
ok  	github.com/Knatte18/loomyard/internal/treadleengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/vscode	(cached)
ok  	github.com/Knatte18/loomyard/internal/webstercli	(cached)
ok  	github.com/Knatte18/loomyard/internal/websterengine	(cached)
ok  	github.com/Knatte18/loomyard/internal/weftname	(cached)
ok  	github.com/Knatte18/loomyard/internal/yamlengine	(cached)
ok  	github.com/Knatte18/loomyard/tools/deploy	(cached)
ok  	github.com/Knatte18/loomyard/tools/godocreflow	(cached)
ok  	github.com/Knatte18/loomyard/tools/internal/devbin	(cached)
ok  	github.com/Knatte18/loomyard/tools/mdreflow	(cached)
ok  	github.com/Knatte18/loomyard/tools/sandbox	(cached)
ok  	github.com/Knatte18/loomyard/tools/wordswap	(cached)
FAIL
```

## Merge Diff

```diff
diff --git a/CONSTRAINTS.md b/CONSTRAINTS.md
index 8e27d861..a144a021 100644
--- a/CONSTRAINTS.md
+++ b/CONSTRAINTS.md
@@ -220,6 +220,29 @@ a human or any tool outside LYX keeps ordinary git in their warp worktree, untou
   every other `fabricengine` caller remains a review obligation.
   The agent half is machine-checked for webster runs by `fabricengine.RefScanner` (a fork or Master Bash command matching a fabric-driving command spelling or the weft sibling worktree path is a hard, round-failing violation).
 
+## Markdown Link Integrity
+
+Every inline markdown link (`[text](target)`) in a `.md` file under `manifest/` or `docs/` resolves — both its file part and, for a `.md` target carrying one, its `#anchor`.
+
+- **The root restriction is source-side only.**
+  `manifest/` and `docs/` name which files are *scanned* for outgoing links;
+  they do not restrict where those links may *point*.
+  Every link target is resolved wherever it lands in the repo, and any `.md` target gets its `#anchor` resolved too, whether that target sits inside `manifest/`/`docs/` or not.
+  Reading the root restriction as licence to skip anchor resolution for an out-of-root target would silently un-guard `finalize.md`'s `../../CONSTRAINTS.md#fabric-git-invariant-warp--weft` link and the `../../internal/*/doc.go` targets this task creates.
+- **A file-layout convenience, not an ownership claim.**
+  The enforcing test lives in `internal/lyxcwd` (`docslink_test.go`'s `TestEnforcement_MarkdownLinks`), reusing that package's `repoRootForEnforcement` and `walkEnforcementRoots` helpers.
+  That placement is a file-layout convenience, not an ownership claim on markdown links by `internal/lyxcwd` — the Cwd Resolution Invariant scopes that package to cwd resolution and nothing else, exactly the caveat the Fabric Vocabulary Invariant above already states for its own test.
+- **What the machine check does and does not reach — stated honestly, not implying full coverage.**
+  Not reached: external `http`/`https`/`mailto` URLs, never fetched;
+  reference-style links (`[text][ref]`) and `<...>` autolinks, out of grammar by decision, not by oversight;
+  link-shaped text inside fenced code blocks, deliberately skipped;
+  prose mentions of a filename that are not markdown links — `manifest/roadmap.md:98`'s `scout-redesign.md` reference is a live example this task leaves standing;
+  and `.md` files outside `manifest/` and `docs/` as **scan sources**, so `README.md`, `CLAUDE.md`, and `internal/**/*.md` have their own outgoing links checked by nobody.
+- **The allowlist's self-expiring contract.**
+  Keyed by `(file, target)`, never by line number, with every entry naming its owning task.
+  An entry whose key is not matched by any break in a scan is reported as deletable — this covers both a link that was fixed and a keyed file that was renamed or deleted away, since neither case is ever visited by the scan again.
+- **Enforced by** `internal/lyxcwd/docslink_test.go` (`TestEnforcement_MarkdownLinks`).
+
 ## Review Round Invariant
 
 One review+fix round (burler now, hardener later) follows: A-before-B (review fully written to disk before any target file is touched);
diff --git a/README.md b/README.md
index dbadc630..8c02a83e 100644
--- a/README.md
+++ b/README.md
@@ -90,7 +90,7 @@ All commands print JSON: `{"ok":true, ...}` on success, `{"ok":false,"error":"..
 
 **In progress (design):**
 
-- **loom** — the phased orchestrator (Preflight → Discussion → Plan → Webster → Raddle → Finalize), each producing phase gated by a `perch` review.
+- **loom** — the phased orchestrator (Preflight → Discussion → Plan → Webster → Finalize), each producing phase gated by a `perch` review.
   Preflight is built;
   Discussion, Plan, the phase-machine skeleton, Finalize, and session bootstrap are still being built out.
 
diff --git a/docs/shared-libs/README.md b/docs/shared-libs/README.md
index 157be25b..e8d98a3e 100644
--- a/docs/shared-libs/README.md
+++ b/docs/shared-libs/README.md
@@ -9,7 +9,7 @@ It carries *no* domain logic.
 The command *sequences* (which git calls, which lock files, which config keys) stay in the modules.
 Each shared lib also carries its own deep tests, so it is vetted plumbing, not an untested dependency.
 
-See [roadmap.md](../roadmap.md) milestones 2–3 for the extraction order.
+See [roadmap.md](../../manifest/roadmap.md) milestones 2–3 for the extraction order.
 
 ## Libraries
 
diff --git a/internal/lyxcwd/docslink_test.go b/internal/lyxcwd/docslink_test.go
new file mode 100644
index 00000000..dd4fd9fb
--- /dev/null
+++ b/internal/lyxcwd/docslink_test.go
@@ -0,0 +1,599 @@
+// docslink_test.go guards markdown link and anchor integrity under manifest/ and docs/: every
+// inline markdown link's file part and #anchor must resolve somewhere in the repo. Its placement in
+// internal/lyxcwd is a file-layout convenience reusing repoRootForEnforcement and
+// walkEnforcementRoots from enforcement_test.go, not an ownership claim on markdown links by that
+// package — see CONSTRAINTS.md's Markdown Link Integrity invariant.
+
+package lyxcwd
+
+import (
+	"os"
+	"path/filepath"
+	"regexp"
+	"strconv"
+	"strings"
+	"testing"
+	"unicode"
+)
+
+// inlineLinkPattern matches an inline markdown link "[text](target)". Reference-style links
+// ("[text][ref]") and angle-bracket autolinks ("<https://...>") do not match this pattern and are
+// therefore out of grammar, per the Link grammar decision recorded in _mill/discussion.md.
+var inlineLinkPattern = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)\)`)
+
+// atxHeadingPattern matches an ATX heading line ("#" through "######" followed by a space) after
+// leading whitespace has been trimmed.
+var atxHeadingPattern = regexp.MustCompile(`^#{1,6} `)
+
+// docsLinkSlug implements GitHub's heading-slug rule: strip a leading run of "#" characters and the
+// single following space, delete every backtick, lowercase the remainder, delete every rune that is
+// not a Unicode letter, a Unicode digit, "_", "-", or a space, then replace each remaining space
+// with "-". The deletion step is a deletion, not a replacement — an em-dash surrounded by spaces
+// leaves the two spaces behind, which become a double hyphen once spaces are replaced.
+func docsLinkSlug(heading string) string {
+	s := heading
+
+	i := 0
+	for i < len(s) && s[i] == '#' {
+		i++
+	}
+	s = s[i:]
+	s = strings.TrimPrefix(s, " ")
+
+	s = strings.ReplaceAll(s, "`", "")
+	s = strings.ToLower(s)
+
+	var kept strings.Builder
+	for _, r := range s {
+		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == ' ' {
+			kept.WriteRune(r)
+		}
+	}
+
+	return strings.ReplaceAll(kept.String(), " ", "-")
+}
+
+// docsLink is one inline markdown link found in a document, carrying its 1-based source line and
+// its raw, unresolved target string.
+type docsLink struct {
+	Line   int
+	Target string
+}
+
+// docsLinkExtract returns every inline markdown link in data, in document order, with its 1-based
+// line number. It tracks fenced code blocks (both ``` and ~~~ fences, opened and closed at line
+// start allowing up to three leading spaces) and skips every link-shaped match found inside a
+// fence, since a fenced example of a broken link is documentation, not a broken link.
+func docsLinkExtract(data []byte) []docsLink {
+	var links []docsLink
+
+	inFence := false
+	var fenceChar byte
+
+	for i, line := range strings.Split(string(data), "\n") {
+		if marker, isFence := fenceMarker(line); isFence {
+			if !inFence {
+				inFence = true
+				fenceChar = marker
+			} else if marker == fenceChar {
+				inFence = false
+			}
+			continue
+		}
+		if inFence {
+			continue
+		}
+
+		for _, match := range inlineLinkPattern.FindAllStringSubmatch(line, -1) {
+			links = append(links, docsLink{Line: i + 1, Target: match[2]})
+		}
+	}
+
+	return links
+}
+
+// fenceMarker reports whether line opens or closes a fenced code block (allowing up to three
+// leading spaces before the fence), returning the fence character ('`' or '~') when it does.
+func fenceMarker(line string) (byte, bool) {
+	trimmed := strings.TrimLeft(line, " ")
+	if len(line)-len(trimmed) > 3 {
+		return 0, false
+	}
+	if strings.HasPrefix(trimmed, "```") {
+		return '`', true
+	}
+	if strings.HasPrefix(trimmed, "~~~") {
+		return '~', true
+	}
+	return 0, false
+}
+
+// docsLinkHeadingAnchors returns the set of anchors for every ATX heading in data, in document
+// order, skipping headings inside fenced code blocks. Each heading's text is passed through
+// docsLinkSlug, and GitHub's duplicate-disambiguation suffixes are applied: the first occurrence of
+// a slug is bare, the second gets "-1", the third "-2", and so on.
+func docsLinkHeadingAnchors(data []byte) map[string]bool {
+	anchors := make(map[string]bool)
+	seen := make(map[string]int)
+
+	inFence := false
+	var fenceChar byte
+
+	for _, line := range strings.Split(string(data), "\n") {
+		if marker, isFence := fenceMarker(line); isFence {
+			if !inFence {
+				inFence = true
+				fenceChar = marker
+			} else if marker == fenceChar {
+				inFence = false
+			}
+			continue
+		}
+		if inFence {
+			continue
+		}
+
+		trimmed := strings.TrimLeft(line, " ")
+		if len(line)-len(trimmed) > 3 || !atxHeadingPattern.MatchString(trimmed) {
+			continue
+		}
+
+		base := docsLinkSlug(trimmed)
+		occurrence := seen[base]
+		seen[base] = occurrence + 1
+		if occurrence == 0 {
+			anchors[base] = true
+			continue
+		}
+		anchors[base+"-"+strconv.Itoa(occurrence)] = true
+	}
+
+	return anchors
+}
+
+// TestDocsLinkSlug covers GitHub's heading-slug rules against literal data, including the three
+// worked examples from _mill/discussion.md's "Link-checker implementation notes" and a fourth case
+// for the Fabric Git Invariant heading, since card 5 links depend on exactly that slug.
+func TestDocsLinkSlug(t *testing.T) {
+	tests := []struct {
+		name    string
+		heading string
+		want    string
+	}{
+		{
+			name:    "phase machine em-dash heading",
+			heading: "## The phase machine — a flat producer list, no predefined slots",
+			want:    "the-phase-machine--a-flat-producer-list-no-predefined-slots",
+		},
+		{
+			name:    "summary artifact backtick and slash heading",
+			heading: "## The summary artifact — `_lyx/webster/summary.md`",
+			want:    "the-summary-artifact--_lyxwebstersummarymd",
+		},
+		{
+			name:    "when it runs colon heading",
+			heading: "## When it runs: deferred to merge-time, not mid-task",
+			want:    "when-it-runs-deferred-to-merge-time-not-mid-task",
+		},
+		{
+			name:    "fabric git invariant parens and plus heading",
+			heading: "## Fabric Git Invariant (warp + weft)",
+			want:    "fabric-git-invariant-warp--weft",
+		},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := docsLinkSlug(tt.heading)
+			if got != tt.want {
+				t.Errorf("docsLinkSlug(%q) = %q; want %q", tt.heading, got, tt.want)
+			}
+		})
+	}
+}
+
+// TestDocsLinkExtract covers the inline-link grammar over literal data: plain links, links whose
+// text carries backticks, multiple links on one line, fence skipping for both fence styles, and
+// that reference-style links plus autolinks are silently ignored as out of grammar.
+func TestDocsLinkExtract(t *testing.T) {
+	tests := []struct {
+		name string
+		data string
+		want []docsLink
+	}{
+		{
+			name: "plain inline link",
+			data: "See [loom.md](loom.md) for details.",
+			want: []docsLink{{Line: 1, Target: "loom.md"}},
+		},
+		{
+			name: "link text with backticks",
+			data: "See [`internal/fabricengine`](../../internal/fabricengine/doc.go).",
+			want: []docsLink{{Line: 1, Target: "../../internal/fabricengine/doc.go"}},
+		},
+		{
+			name: "two links on one line",
+			data: "[a](a.md) and [b](b.md)",
+			want: []docsLink{{Line: 1, Target: "a.md"}, {Line: 1, Target: "b.md"}},
+		},
+		{
+			name: "link inside backtick fence is skipped",
+			data: "text\n```\n[a](a.md)\n```\nmore",
+			want: nil,
+		},
+		{
+			name: "link inside tilde fence is skipped",
+			data: "text\n~~~\n[a](a.md)\n~~~\nmore",
+			want: nil,
+		},
+		{
+			name: "reference-style link and autolink are ignored",
+			data: "[text][ref] and <https://example.com>\n\n[ref]: target.md",
+			want: nil,
+		},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := docsLinkExtract([]byte(tt.data))
+			if len(got) != len(tt.want) {
+				t.Fatalf("docsLinkExtract(%q) = %v; want %v", tt.data, got, tt.want)
+			}
+			for i := range got {
+				if got[i] != tt.want[i] {
+					t.Errorf("docsLinkExtract(%q)[%d] = %v; want %v", tt.data, i, got[i], tt.want[i])
+				}
+			}
+		})
+	}
+}
+
+// TestDocsLinkHeadingAnchors covers anchor-set construction over literal data: distinct headings,
+// duplicate-slug disambiguation up to three occurrences, and that a "#"-prefixed line inside a
+// fence never counts as a heading.
+func TestDocsLinkHeadingAnchors(t *testing.T) {
+	tests := []struct {
+		name string
+		data string
+		want map[string]bool
+	}{
+		{
+			name: "distinct headings",
+			data: "# One\n## Two\n### Three\n",
+			want: map[string]bool{"one": true, "two": true, "three": true},
+		},
+		{
+			name: "two identically-slugging headings",
+			data: "## Foo\n## Foo\n",
+			want: map[string]bool{"foo": true, "foo-1": true},
+		},
+		{
+			name: "three identically-slugging headings",
+			data: "## Foo\n## Foo\n## Foo\n",
+			want: map[string]bool{"foo": true, "foo-1": true, "foo-2": true},
+		},
+		{
+			name: "hash-prefixed line inside fence is not a heading",
+			data: "```\n# Not A Heading\n```\n## Real Heading\n",
+			want: map[string]bool{"real-heading": true},
+		},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := docsLinkHeadingAnchors([]byte(tt.data))
+			if len(got) != len(tt.want) {
+				t.Fatalf("docsLinkHeadingAnchors(%q) = %v; want %v", tt.data, got, tt.want)
+			}
+			for k := range tt.want {
+				if !got[k] {
+					t.Errorf("docsLinkHeadingAnchors(%q) missing anchor %q; got %v", tt.data, k, got)
+				}
+			}
+		})
+	}
+}
+
+// docsLinkKey identifies one (file, target) link instance for allowlisting purposes. File is the
+// repoRoot-relative, slash-normalized path of the file the link was found in; Target is the raw
+// target string exactly as written in the source.
+type docsLinkKey struct {
+	File   string
+	Target string
+}
+
+// docsLinkBreak is one unresolved markdown link found by a scan.
+type docsLinkBreak struct {
+	File   string
+	Line   int
+	Target string
+	Reason string // "missing file" or "missing anchor"
+}
+
+// docsLinkResolve resolves one split link target against the repo tree rooted at repoRoot, where
+// relPath and data identify the containing file the link was found in. It returns "missing file" or
+// "missing anchor" when the target does not resolve, or "" when it does. A same-file fragment
+// (filePart == "") resolves against data's own headings; otherwise filePart is resolved relative to
+// the containing file's directory, its existence on disk is checked, and — only when it exists,
+// ends in ".md", and fragment is non-empty — fragment is resolved against that target file's own
+// headings. A target that exists but does not end in ".md" has its existence checked and no anchor
+// check attempted.
+func docsLinkResolve(repoRoot, relPath string, data []byte, filePart, fragment string, hasFragment bool) string {
+	if filePart == "" {
+		if hasFragment && fragment != "" && !docsLinkHeadingAnchors(data)[fragment] {
+			return "missing anchor"
+		}
+		return ""
+	}
+
+	targetAbs := filepath.Clean(filepath.Join(repoRoot, filepath.Dir(filepath.FromSlash(relPath)), filepath.FromSlash(filePart)))
+	info, err := os.Stat(targetAbs)
+	if err != nil {
+		return "missing file"
+	}
+	// A target that resolves to a directory (e.g. a trailing-slash link to a directory listing)
+	// exists but is never a .md file, so no anchor check applies to it either.
+	if info.IsDir() || !strings.HasSuffix(targetAbs, ".md") || !hasFragment || fragment == "" {
+		return ""
+	}
+
+	targetData, readErr := os.ReadFile(targetAbs)
+	if readErr != nil {
+		return "missing file"
+	}
+	if !docsLinkHeadingAnchors(targetData)[fragment] {
+		return "missing anchor"
+	}
+	return ""
+}
+
+// docsLinkScan walks every ".md" file under roots (repoRoot-relative, "." for the whole tree) via
+// walkEnforcementRoots, extracts every inline link, and resolves each one against the repo tree.
+// The root restriction is source-side only: roots names which files are scanned for outgoing links
+// and never restricts where a target may point — every target is resolved wherever it lands in the
+// repo, including the #anchor of any ".md" target whether or not that target is itself inside roots.
+// breaks is every unresolved link whose docsLinkKey is not present in allow; unmatched is every
+// allow key that no break in this run — allowlisted or not — matched, which is how a stale allowlist
+// entry (its link now resolves, or its keyed file was renamed or deleted away) is reported.
+func docsLinkScan(t *testing.T, repoRoot string, roots []string, allow map[docsLinkKey]string) (breaks []docsLinkBreak, unmatched []docsLinkKey) {
+	t.Helper()
+
+	matched := make(map[docsLinkKey]bool)
+
+	walkEnforcementRoots(t, repoRoot, roots, []string{".md"}, func(relPath string, data []byte) {
+		for _, link := range docsLinkExtract(data) {
+			target := link.Target
+			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") {
+				continue
+			}
+
+			filePart, fragment, hasFragment := strings.Cut(target, "#")
+			reason := docsLinkResolve(repoRoot, relPath, data, filePart, fragment, hasFragment)
+			if reason == "" {
+				continue
+			}
+
+			key := docsLinkKey{File: relPath, Target: target}
+			matched[key] = true
+			if _, ok := allow[key]; ok {
+				continue
+			}
+			breaks = append(breaks, docsLinkBreak{File: relPath, Line: link.Line, Target: target, Reason: reason})
+		}
+	})
+
+	for key := range allow {
+		if !matched[key] {
+			unmatched = append(unmatched, key)
+		}
+	}
+
+	return breaks, unmatched
+}
+
+// docsLinkAllowlist is the self-expiring allowlist of known-broken links this task leaves for other
+// tasks to fix, per _mill/discussion.md's allowlist-is-keyed-and-self-expiring decision. It is keyed
+// by (file, target) and never by line number; every entry names its owning task; and an entry whose
+// key is not matched by any break in a scan is reported by docsLinkScan as deletable.
+// 7 entries covering 8 link instances: docs/reference/plan-format-v3.md carries the
+// scout-redesign.md target twice, and the (file, target) key collapses both into one entry, which
+// is intended.
+var docsLinkAllowlist = map[docsLinkKey]string{
+	{File: "docs/reference/discussion-format.md", Target: "plan-format.md"}:                        "task B -- resolves when plan-format-v3.md is renamed to plan-format.md",
+	{File: "docs/reference/plan-format-v3.md", Target: "plan-format.md"}:                           "task B -- same",
+	{File: "docs/reference/status-schema.md", Target: "plan-format.md"}:                            "task B -- same",
+	{File: "manifest/designs/loom.md", Target: "../../docs/reference/plan-format.md"}:              "task B -- same",
+	{File: "docs/reference/plan-format-v3.md", Target: "../../manifest/designs/scout-redesign.md"}: "task B owns this file; the target fix is the one this task applies elsewhere",
+	{File: "docs/overview.md", Target: "../CONSTRAINTS.md#package-naming"}:                         "chain A -> B -> E; E is last owner",
+	{File: "manifest/designs/loom.md", Target: "../../docs/overview.md#hub-geometry-invariants"}:   "chain B -> C -> E; E is last owner",
+}
+
+// TestEnforcement_MarkdownLinks is the permanent guard behind the Markdown Link Integrity invariant:
+// every inline markdown link in a .md file under manifest/ or docs/ must resolve, both its file part
+// and its #anchor.
+func TestEnforcement_MarkdownLinks(t *testing.T) {
+	t.Run("repo", func(t *testing.T) {
+		breaks, unmatched := docsLinkScan(t, repoRootForEnforcement(t), []string{"manifest", "docs"}, docsLinkAllowlist)
+
+		for _, b := range breaks {
+			t.Errorf("broken markdown link: %s:%d  %s  %s", b.File, b.Line, b.Reason, b.Target)
+		}
+		for _, u := range unmatched {
+			t.Errorf("stale allowlist entry, delete it: %s -> %s", u.File, u.Target)
+		}
+	})
+
+	// writeTree materializes files (each keyed by a slash-separated path relative to the tree
+	// root) under a fresh t.TempDir() and returns the tree's absolute root. None of these paths
+	// may contain "testdata" -- walkEnforcementRoots skips any directory whose name contains that
+	// substring, which would make the built fixture walk to zero files and pass vacuously.
+	writeTree := func(t *testing.T, files map[string]string) string {
+		t.Helper()
+		root := t.TempDir()
+		for relPath, content := range files {
+			full := filepath.Join(root, filepath.FromSlash(relPath))
+			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
+				t.Fatalf("mkdir for %s: %v", relPath, err)
+			}
+			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
+				t.Fatalf("write %s: %v", relPath, err)
+			}
+		}
+		return root
+	}
+
+	t.Run("relative link to existing file with no fragment resolves", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md": "[b](b.md)\n",
+			"b.md": "# B\n",
+		})
+		breaks, unmatched := docsLinkScan(t, root, []string{"."}, nil)
+		if len(breaks) != 0 || len(unmatched) != 0 {
+			t.Errorf("docsLinkScan() breaks=%v unmatched=%v; want none", breaks, unmatched)
+		}
+	})
+
+	t.Run("relative link to missing file produces missing file break", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md": "[b](b.md)\n",
+		})
+		breaks, _ := docsLinkScan(t, root, []string{"."}, nil)
+		if len(breaks) != 1 || breaks[0].Reason != "missing file" {
+			t.Errorf("docsLinkScan() breaks=%v; want one missing file break", breaks)
+		}
+	})
+
+	t.Run("fragment matching a heading in the target file resolves", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md": "[b](b.md#some-heading)\n",
+			"b.md": "## Some Heading\n",
+		})
+		breaks, _ := docsLinkScan(t, root, []string{"."}, nil)
+		if len(breaks) != 0 {
+			t.Errorf("docsLinkScan() breaks=%v; want none", breaks)
+		}
+	})
+
+	t.Run("fragment with no matching heading produces missing anchor break", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md": "[b](b.md#no-such-heading)\n",
+			"b.md": "## Some Heading\n",
+		})
+		breaks, _ := docsLinkScan(t, root, []string{"."}, nil)
+		if len(breaks) != 1 || breaks[0].Reason != "missing anchor" {
+			t.Errorf("docsLinkScan() breaks=%v; want one missing anchor break", breaks)
+		}
+	})
+
+	t.Run("same-file fragment resolves against the containing file's own headings", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md": "## Some Heading\n\n[self](#some-heading)\n",
+		})
+		breaks, _ := docsLinkScan(t, root, []string{"."}, nil)
+		if len(breaks) != 0 {
+			t.Errorf("docsLinkScan() breaks=%v; want none", breaks)
+		}
+	})
+
+	t.Run("http https and mailto targets are skipped and never produce a break", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md": "[h](http://example.com/x) [s](https://example.com/y) [m](mailto:a@example.com)\n",
+		})
+		breaks, _ := docsLinkScan(t, root, []string{"."}, nil)
+		if len(breaks) != 0 {
+			t.Errorf("docsLinkScan() breaks=%v; want none", breaks)
+		}
+	})
+
+	t.Run("allowlisted pair produces no break and leaves unmatched empty", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md": "[b](b.md)\n",
+		})
+		allow := map[docsLinkKey]string{
+			{File: "a.md", Target: "b.md"}: "test",
+		}
+		breaks, unmatched := docsLinkScan(t, root, []string{"."}, allow)
+		if len(breaks) != 0 {
+			t.Errorf("docsLinkScan() breaks=%v; want none (allowlisted)", breaks)
+		}
+		if len(unmatched) != 0 {
+			t.Errorf("docsLinkScan() unmatched=%v; want none", unmatched)
+		}
+	})
+
+	t.Run("stale allowlist entry whose link now resolves is reported in unmatched", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md": "[b](b.md)\n",
+			"b.md": "# B\n",
+		})
+		allow := map[docsLinkKey]string{
+			{File: "a.md", Target: "b.md"}: "test",
+		}
+		breaks, unmatched := docsLinkScan(t, root, []string{"."}, allow)
+		if len(breaks) != 0 {
+			t.Errorf("docsLinkScan() breaks=%v; want none", breaks)
+		}
+		if len(unmatched) != 1 || unmatched[0] != (docsLinkKey{File: "a.md", Target: "b.md"}) {
+			t.Errorf("docsLinkScan() unmatched=%v; want the now-resolved entry reported stale", unmatched)
+		}
+	})
+
+	t.Run("stale allowlist entry whose keyed file no longer exists is reported in unmatched", func(t *testing.T) {
+		// The renamed-away case: the allowlisted file is not present in this tree at all, so the
+		// walk never visits it and never produces a matching break. A naive "does the link now
+		// resolve" staleness check would never catch this; docsLinkScan's "was this key matched
+		// by any break in this run" definition catches it because it was never matched.
+		root := writeTree(t, map[string]string{
+			"other.md": "# Other\n",
+		})
+		allow := map[docsLinkKey]string{
+			{File: "renamed-away.md", Target: "b.md"}: "test",
+		}
+		breaks, unmatched := docsLinkScan(t, root, []string{"."}, allow)
+		if len(breaks) != 0 {
+			t.Errorf("docsLinkScan() breaks=%v; want none", breaks)
+		}
+		if len(unmatched) != 1 || unmatched[0] != (docsLinkKey{File: "renamed-away.md", Target: "b.md"}) {
+			t.Errorf("docsLinkScan() unmatched=%v; want the renamed-away entry reported stale", unmatched)
+		}
+	})
+
+	t.Run("link-shaped text inside fences is ignored end-to-end", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md": "text\n```\n[missing](no-such-file.md)\n```\nmore\n~~~\n[missing2](also-no-such-file.md)\n~~~\n",
+		})
+		breaks, _ := docsLinkScan(t, root, []string{"."}, nil)
+		if len(breaks) != 0 {
+			t.Errorf("docsLinkScan() breaks=%v; want none (fenced links ignored)", breaks)
+		}
+	})
+
+	t.Run("two identically-slugging headings both resolve via foo and foo-1", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md": "[first](b.md#foo) [second](b.md#foo-1)\n",
+			"b.md": "## Foo\n## Foo\n",
+		})
+		breaks, _ := docsLinkScan(t, root, []string{"."}, nil)
+		if len(breaks) != 0 {
+			t.Errorf("docsLinkScan() breaks=%v; want none", breaks)
+		}
+	})
+
+	t.Run("non-md target existence is checked with no anchor check attempted", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md":   "[doc](doc.go#nonexistent-anchor)\n",
+			"doc.go": "package p\n",
+		})
+		breaks, _ := docsLinkScan(t, root, []string{"."}, nil)
+		if len(breaks) != 0 {
+			t.Errorf("docsLinkScan() breaks=%v; want none -- non-.md target with fragment skips anchor check", breaks)
+		}
+	})
+
+	t.Run("missing non-md target still produces a missing file break", func(t *testing.T) {
+		root := writeTree(t, map[string]string{
+			"a.md": "[gone](gone.go)\n",
+		})
+		breaks, _ := docsLinkScan(t, root, []string{"."}, nil)
+		if len(breaks) != 1 || breaks[0].Reason != "missing file" {
+			t.Errorf("docsLinkScan() breaks=%v; want one missing file break", breaks)
+		}
+	})
+}
diff --git a/manifest/designs/finalize.md b/manifest/designs/finalize.md
index d8dfae47..7f512b6d 100644
--- a/manifest/designs/finalize.md
+++ b/manifest/designs/finalize.md
@@ -1,6 +1,6 @@
 # Finalize — Shed's merge-back step
 
-> **Status: Design — not built. Planned, combined with the `Shed` task** (see `manifest/roadmap.md`) — building `Shed`'s skeleton and its Finalize step happen together, the same reasoning as the combined `Treadle` + `perch`-rewrite task. Renamed from `loom-finalize.md`: Finalize is **`Shed`'s** literally-shared code (identical for `loom` and `Hardener`, not a swappable per-instance slot the way Preflight and the producer are — see [shed.md](shed.md)), not loom-specific, though originally split out of [loom.md](loom.md) as a substantial, self-contained phase spec worth its own file. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), when this lands the durable parts fold into the relevant package doc and this file is deleted.
+> **Status: Design — not built. Planned, combined with the `Shed` task** (see `manifest/roadmap.md`) — building `Shed`'s skeleton and its Finalize step happen together, the same reasoning as the combined `Treadle` + `perch`-rewrite task. Renamed from `loom-finalize.md`: `Finalize` is an ordinary producer that both `loom`'s and `Hardener`'s producer lists name — one definition, named twice, never copied, and never something `Shed` special-cases (see [shed.md](shed.md)) — not loom-specific, though originally split out of [loom.md](loom.md) as a substantial, self-contained phase spec worth its own file. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), when this lands the durable parts fold into the relevant package doc and this file is deleted.
 
 ## What it does
 
@@ -8,8 +8,8 @@
 Go-first: the happy path (no conflicts) is pure Go — squash, push, done, zero LLM cost.
 An LLM is spawned only on merge conflict (during merge-in from parent, or the merge to parent itself), escalating to a **fresh, higher-capability model in a clean session** (see `internal/websterengine`'s package documentation) — not a `/model` switch inside a polluted one.
 
-Mostly wiring on top of the already-built `warp` mechanics (absorbed into `fabric` once that lands — see [fabric.md](fabric.md));
-worktree/branch/junction/portal teardown is explicitly **out of scope** — that's `warp cleanup`'s (future: `fabric`'s) existing, separate job, which cannot run from inside the worktree being removed, the same reason `mill-cleanup` runs from the hub, never a task worktree.
+Mostly wiring on top of the already-built `fabric` mechanics (see [`internal/fabricengine`](../../internal/fabricengine/doc.go));
+worktree/branch/junction/portal teardown is explicitly **out of scope** — that's `lyx fabric cleanup`'s existing, separate job, which cannot run from inside the worktree being removed, the same reason `mill-cleanup` runs from the hub, never a task worktree.
 
 ## Two merge targets, not one — warp and weft, handled differently
 
@@ -23,7 +23,7 @@ Merge-back is not a single git-merge operation — it is two, with genuinely dif
 
 ## Only Raddle forwards from child weft to parent weft — not `_lyx`
 
-`_lyx` is committed into every task's own weft branch **by design** (see `internal/fabricengine`'s package documentation and CONSTRAINTS.md's Weft Git Invariant) — it is the per-task session/orchestration state,
+`_lyx` is committed into every task's own weft branch **by design** (see `internal/fabricengine`'s package documentation and the [Fabric Git Invariant (warp + weft)](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft)) — it is the per-task session/orchestration state,
 and it is correct for it to live there for the task's own lifetime.
 It was never meant to propagate to parent, though — merge-back only forwards **Raddle**'s regenerated output (see [raddle.md](raddle.md#when-it-runs-deferred-to-merge-time-not-mid-task) for when that regeneration actually runs) using a **narrowed pathspec**: `fabric.CommitWeft` already accepts an arbitrary pathspec (it is not hardwired to `_lyx` — `internal/fabricengine/weftgit.go`'s `CommitWeft` takes the pathspec as a parameter,
 and the fabric config's own `pathspec` key is whitespace-separated, so a hub can already name several directories at once), so the merge-back commit simply calls it with `["_lyx"]` — raddle and PATTERN content are both inside `_lyx` now, which is precisely why the earlier per-directory scoping (`_raddle` vs. `_pattern`) is obsolete.
@@ -31,6 +31,13 @@ No new exclusion mechanism is needed — this is a call-site decision, not an ar
 
 Note: since Raddle and (eventually) `scout`'s own index are both pure functions of the current source code, they **regenerate** at merge-time rather than being merged/diffed across branches at all (see [raddle.md](raddle.md) for the reasoning) — so in practice the weft-side document-driven conflict path above is expected to matter mainly for genuinely hand/LLM-authored weft content like `PATTERN.md`, not for Raddle's own output.
 
+## Raddle regeneration — part of the merge, not a step before it
+
+Raddle-regeneration is scoped as part of the Finalize merge itself, not a separate producer or a reserved phase slot of its own, per [shed.md](shed.md) and [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots): updating Raddle before the merge is impractical given merge-conflict risk, so regenerating it happens inside the merge's own critical section instead.
+The merge lock Finalize takes must span that whole critical section as one atomic unit — read the parent's current HEAD, run the leaf-fork and `Overview.md` regeneration against it, and commit the result via `SyncWeft` — never released and re-acquired partway through.
+See [raddle.md](raddle.md) for the regeneration mechanics themselves — the parallel-fork structure, the `Overview.md` sequencing, and the `SyncWeft` commit shape all live there, not here.
+This is the currently-landed shape of the fold, not the only one considered: an alternative giving Raddle its own `Shed` producer, with merge-in and locking lifted into `Shed` itself, surfaced during this task's discussion and remains a candidate for a future task.
+
 ## PR creation, when configured
 
 If `require_pr_to_base` is set, the PR title/body is dumped **verbatim** from the prose summary artifact webster adds to its final action (see [webster-contract.md](../../docs/reference/webster-contract.md#the-summary-artifact--_lyxwebstersummarymd)) — no dedicated LLM call needed in Finalize itself, since that summary is the only artifact with full oversight of what was actually built, including deviations from the original plan.
@@ -42,11 +49,12 @@ If `require_pr_to_base` is set, the PR title/body is dumped **verbatim** from th
 
 ## Related
 
-- [shed.md](shed.md) — the generic outer phase-FSM Finalize is the last step of;
-  both `loom` and the Someday `Hardener` share this exact code.
+- [shed.md](shed.md) — the generic outer phase-FSM `Finalize` is a producer within;
+  both `loom`'s and the Someday `Hardener`'s producer lists name the same `Finalize` definition, never a copy.
 - [loom.md](loom.md) — the mature, already-detailed phase machine this doc was originally split out of;
-  `Shed` hasn't been extracted from it yet (see that doc's own naming note).
-- [raddle.md](raddle.md) — the merge-time regeneration decision and merge-lock scope Finalize's Raddle-regeneration step must honor.
+  `shed.md` owns `Shed`'s generic mechanism, while `loom.md` owns `loom`'s own concrete producer list built on top of it, per `shed.md`'s own split of authority.
+- [raddle.md](raddle.md) — the regeneration mechanics (parallel-fork structure, `Overview.md` sequencing, `SyncWeft` commit shape) the section above points at;
+  the fold decision itself now lives in this doc's own "Raddle regeneration" section above, not in this bullet.
 - [webster-contract.md](../../docs/reference/webster-contract.md#the-summary-artifact--_lyxwebstersummarymd) — the summary artifact Finalize consumes verbatim for PR bodies;
   `internal/websterengine`'s package documentation covers the escalation pattern Finalize mirrors.
-- [fabric.md](fabric.md) — the mechanics Finalize wires on top of, incl. `CommitWeft`'s pathspec parameter and `Warp-SHA` correspondence tracking.
+- [`internal/fabricengine`](../../internal/fabricengine/doc.go) — the mechanics Finalize wires on top of, incl. `CommitWeft`'s pathspec parameter and `Warp-SHA` correspondence tracking.
diff --git a/manifest/designs/raddle.md b/manifest/designs/raddle.md
index 42e28e5e..0675dfaa 100644
--- a/manifest/designs/raddle.md
+++ b/manifest/designs/raddle.md
@@ -1,10 +1,11 @@
 # raddle — codeguide's woven-in successor (Someday, deprioritized)
 
-> **Status: Design partially exists, not scheduled.** Deprioritized — not required to land a first `loom` plan. Already has a reserved-but-unbuilt phase slot between Webster and Finalize (see [loom.md](loom.md#the-phase-machine)). This doc covers the parts of raddle's design settled during the vacation-time discussion, not the whole module.
+> **Status: Design partially exists, not scheduled.** Deprioritized — not required to land a first `loom` plan. Raddle-regeneration is folded into `Finalize`'s own contract, not a reserved phase slot of its own (see [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots)). This doc covers the parts of raddle's design settled during the vacation-time discussion, not the whole module.
 
 ## What it is
 
-Raddle is codeguide's weaving-vocabulary successor, living in `weft`: an always-run step after Webster (deliberately not the implementer's job — implementers, busy with code, forget the docs) that generates docs over the diff a plan produced, building heavily on Millhouse's `codeguide-update`.
+Raddle is codeguide's weaving-vocabulary successor, living in `weft`: it generates docs over the diff a plan produced, building heavily on Millhouse's `codeguide-update`, deliberately not the implementer's job — implementers, busy with code, forget the docs.
+See [When it runs](#when-it-runs-deferred-to-merge-time-not-mid-task) below for when regeneration actually happens.
 
 ## Geometry — where raddle content lives
 
@@ -51,8 +52,8 @@ This collapses the two potential runs into one and guarantees the output describ
 If another task's merge landed in parent partway through regeneration, the docs would be stale against the HEAD they're about to be committed onto.
 Same "advance only on confirmed success" discipline the `Warp-SHA` trailer mechanism uses elsewhere in `fabric` for recording a baseline, extended to cover the compute step, not just the write step.
 
-**Open, not yet decided:** whether this removes raddle's reserved phase slot between Webster and Finalize in [loom.md](loom.md#the-phase-machine) entirely (folding regeneration into the Finalize/Merge step instead), or whether an earlier, non-authoritative mid-task run stays for human visibility before PR.
-Not resolved here.
+**Decided:** raddle has no reserved phase slot of its own in [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) — regeneration is folded into `Finalize`'s own contract instead, landed at [shed.md](shed.md) and [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots).
+See [finalize.md](finalize.md#raddle-regeneration--part-of-the-merge-not-a-step-before-it) for Finalize's side of the contract.
 
 ## Staleness tracking, via `fabric`
 
@@ -82,5 +83,6 @@ Master (and any fork inheriting its context) must treat raddle content as "how t
 ## Related
 
 - [`internal/fabricengine`](../../internal/fabricengine/doc.go) — the `Warp-SHA`/`Snapshot` trailer and `SyncWeft` mechanics this design relies on.
-- [loom.md](loom.md) — where raddle's phase slot sits in the phase machine.
+- [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots) — the flat producer list Raddle has no slot of its own in;
+  regeneration is folded into `Finalize`'s contract instead.
 - The `internal/boardengine` package documentation — `PATTERN.md` (raddle's neighbor in `weft`) mentioned there.
diff --git a/manifest/designs/self-report.md b/manifest/designs/self-report.md
index 6337654c..f8ff7a29 100644
--- a/manifest/designs/self-report.md
+++ b/manifest/designs/self-report.md
@@ -27,7 +27,7 @@ It cannot notice a systemic problem that only shows up across several phases —
 ## Aggregation and the reflection step
 
 Go collects every Tier 2 note emitted during a run (it reads every phase's output file regardless) and, at a natural end point — Finalize, or a `stuck` escalation — spawns **one** dedicated reflection agent over the aggregated dossier.
-This mirrors the `Raddle` pattern (see [loom.md](loom.md#the-phase-machine)): a fresh-context agent reading only the accumulated notes, not carrying the baggage of having "been there" for the whole run.
+This mirrors the `Raddle` pattern (see [loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots)): a fresh-context agent reading only the accumulated notes, not carrying the baggage of having "been there" for the whole run.
 That agent makes the actual self-report judgment call (worth filing? one issue or several? title/body?) and invokes the shipped `lyx selfreport create` primitive to do the actual filing.
 
 ## Relationship to the shipped `selfreport` module
diff --git a/manifest/designs/semantic-index.md b/manifest/designs/semantic-index.md
index a2571b3e..c729f338 100644
--- a/manifest/designs/semantic-index.md
+++ b/manifest/designs/semantic-index.md
@@ -1,11 +1,11 @@
 # semantic-index — semantic search over docstrings and descriptive text
 
-> **Status: Speculative, not scoped.** Inspired by [Enzyme](https://www.enzyme.garden/blog/an-lsp-for-your-notes), a semantic search system for personal note vaults. This is the "deferred idea" [scout-redesign.md](scout-redesign.md) refers to ("a semantic/conceptual index... a separate, further-out idea, not part of this proposal") and the relationship-table row from the original scout proposal ("have we written something conceptually similar, without shared vocabulary? — embeddings + temporal-decay weighting; not part of this proposal") — now named, not yet designed in depth. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), if this is ever picked up the durable parts fold into the owning package's doc when it lands; if abandoned, this file is simply deleted.
+> **Status: Speculative, not scoped.** Inspired by [Enzyme](https://www.enzyme.garden/blog/an-lsp-for-your-notes), a semantic search system for personal note vaults. This is the "deferred idea" [`internal/scoutengine`](../../internal/scoutengine/doc.go)'s own design proposal referred to ("a semantic/conceptual index... a separate, further-out idea, not part of this proposal") and the relationship-table row from the original scout proposal ("have we written something conceptually similar, without shared vocabulary? — embeddings + temporal-decay weighting; not part of this proposal") — now named, not yet designed in depth. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), if this is ever picked up the durable parts fold into the owning package's doc when it lands; if abandoned, this file is simply deleted.
 
 ## The problem this responds to
 
 Grep/text-search finds literal keyword matches.
-It cannot answer "find code that does X" when the code implementing X uses none of the words a caller would naturally search for — e.g. "show me the error-handling patterns in this codebase" when error handling is spelled out in prose inside docstrings and comments but never literally contains the word "error" everywhere it matters. `scout` (see [scout-redesign.md](scout-redesign.md)) doesn't solve this either — it answers "what exactly references/defines this symbol," a precise, compiler-derived question, not "what have we conceptually written that's similar to this."
+It cannot answer "find code that does X" when the code implementing X uses none of the words a caller would naturally search for — e.g. "show me the error-handling patterns in this codebase" when error handling is spelled out in prose inside docstrings and comments but never literally contains the word "error" everywhere it matters. `scout` (see [`internal/scoutengine`](../../internal/scoutengine/doc.go)) doesn't solve this either — it answers "what exactly references/defines this symbol," a precise, compiler-derived question, not "what have we conceptually written that's similar to this."
 
 ## Core mechanism, adapted from Enzyme
 
@@ -51,7 +51,7 @@ None of these three replace either of the others — different question, differe
 
 ## Related
 
-- [scout-redesign.md](scout-redesign.md) — the precise, compiler-derived sibling;
-  named this as an out-of-scope, deferred idea.
+- [`internal/scoutengine`](../../internal/scoutengine/doc.go) — the precise, compiler-derived sibling;
+  scout's own design proposal named this as an out-of-scope, deferred idea.
 - [raddle.md](raddle.md) — the curated-narrative sibling.
 - [`internal/gitrepo`](../../internal/gitrepo/doc.go) — plausible source of the temporal-decay recency signal.
diff --git a/manifest/designs/webster-parallel-execution.md b/manifest/designs/webster-parallel-execution.md
index c398a7ec..4b4df838 100644
--- a/manifest/designs/webster-parallel-execution.md
+++ b/manifest/designs/webster-parallel-execution.md
@@ -51,10 +51,10 @@ Only the *executor that actually runs the width* (this entry) remains parked.
 ## Relationship to scout (Part B of the retired draft)
 
 The retired `websterv2.md` draft also had a Part B — structured impact lookup via `go/packages`/`gopls` (find-all-references as a Go verb instead of LLM-driven grep).
-That idea is superseded, not lost: it's the direct ancestor of the [scout](scout-redesign.md) proposal, which generalizes it to a multi-language, daemon-based design.
+That idea is superseded, not lost: it's the direct ancestor of [`internal/scoutengine`](../../internal/scoutengine/doc.go), which generalizes it to a multi-language, daemon-based design.
 
 ## Related
 
 - `internal/websterengine`'s package documentation — the sequential model this would extend.
 - [plan-format.md](../../docs/reference/plan-format.md) — already captures the cheap win (`depends-on`).
-- [scout-redesign.md](scout-redesign.md) — Part B's successor.
+- [`internal/scoutengine`](../../internal/scoutengine/doc.go) — Part B's successor.

```

## Instructions

1. Read the failing tests and the source files they exercise.
2. Fix the root cause of the failures.
   Do not modify tests unless they are genuinely wrong due to the merge (e.g. a test asserted against a value that the merge legitimately changed).
3. Re-run `go build ./... && go test ./...` after each fix attempt using `git -C /home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix` for git commands.
4. Commit each fix attempt with a clear commit message.
5. Self-fix up to `3` times.
   If the verify command still fails after `3` attempts, stop and report stuck.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

**`commit_sha` MUST be the full SHA from `git rev-parse HEAD` -- never the abbreviated form (`git rev-parse --short HEAD`) or a `git log --oneline` hash.**

On success:

{"status":"success","commit_sha":"<last-HEAD-sha>"}

After exhausting fix rounds:

{"status":"stuck","stuck_type":"verify","reason":"<one-line description of what still fails>","commit_sha":"<last-HEAD-sha>"}

Anything other than this JSON object on the last line is a protocol violation;
the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost.
Do not wrap the JSON in a code fence;
do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob.
Use `git -C /home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix` for git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/plan-format-drop-v3-suffix`.
