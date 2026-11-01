// join.go implements JoinParts, the plan format's write path for a plan joined from the plans of the parts of a split task, and ExportMet, the test of whether a part's plan delivers an export.

package planparser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrJoinRefused is wrapped by every error JoinParts returns for a defect in the parts, raised before any file is written.
var ErrJoinRefused = errors.New("planparser: join refused")

// JoinPart is one part of a split task: its name, the directory holding its plan, and its exports as the parts file spells them.
type JoinPart struct {
	Name    string
	Dir     string
	Exports []string
}

// JoinedCard says where one card of a part landed in the joined plan.
type JoinedCard struct {
	Part string
	From int
	To   int
	File string
}

// JoinResult lists the joined plan's cards in their new order.
type JoinResult struct {
	Cards []JoinedCard
}

// canonicalRenameMechanic is the `## Rename mechanic` section body every plan with a Rename card carries.
const canonicalRenameMechanic = `1. For a file pair (both sides file self glyphs or paths), run ` + "`git mv <old> <new>`" + ` FIRST, before any other change to the moved file.
2. Then make ONLY surgical edits (package declaration, imports, identifier
   retargeting) — no unrelated rewrites.
3. A genuinely new file with no predecessor belongs in a separate ` + "`Create`" + ` card, never folded
   into the ` + "`Rename`" + ` pair.
4. Never write the relocated file from scratch and delete the original — that loses
   git history exactly as an unstructured create+delete pair would.
5. For a symbol pair, run the hub's structure-aware rename tool the implementer prompt names (` + "`language.yaml` `rename_tools`" + `), never a text or regex replace, then grep for zero remaining old-name references and confirm the symbol's doc comment opening names the new name.`

// topLevelCardFileRe matches the name of a card file directly under a plan directory.
var topLevelCardFileRe = regexp.MustCompile(`^\d+-.+\.md$`)

// refuseJoin builds the error JoinParts returns for a defect in the parts.
func refuseJoin(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrJoinRefused}, args...)...)
}

// JoinParts joins the plans of parts into the one plan under planDir, titled title, and returns where every card landed.
// Each part is parsed with ParsePlan, in the order given, which is the parts file's order.
// Before it writes anything it refuses, with an error wrapping ErrJoinRefused that names the part to fix and the reason, when a part's plan does not parse, two parts' overviews differ in `language:` or `root:`, a part's export is not met by its plan, or a `plan:` handle or new file path is a Create target or Rename New side of cards of two parts.
// Otherwise it numbers the parts' cards into one sequence from 1 in part order, rewriting each card's heading, `Commit:` prefix and the ids of the cards of its own part, removes every top-level card file and the overview that planDir held, leaving subdirectories in place, writes the joined cards, and writes the overview last.
// The overview carries a framing paragraph naming the parts for the chair to rewrite, the Card Index, each part's shared decisions under `### Part <name>`, and the canonical rename mechanic when any part has one.
// A read or write failure is returned without wrapping ErrJoinRefused; running the join again rewrites the whole plan directory from the parts.
func JoinParts(planDir, title string, parts []JoinPart) (*JoinResult, error) {
	plans := make([]*Plan, len(parts))
	for i, part := range parts {
		plan, err := ParsePlan(part.Dir)
		if err != nil {
			return nil, refuseJoin("part %q: its plan in %s does not parse: %w; way forward: fix part %q's plan, then run lym loom join-parts again", part.Name, part.Dir, err, part.Name)
		}
		plans[i] = plan
	}
	if err := checkJoinable(parts, plans); err != nil {
		return nil, err
	}

	result := &JoinResult{}
	cardFiles := map[string][]byte{}
	var indexLines []string
	next := 1
	for i, part := range parts {
		plan := plans[i]
		moves := make(map[int]cardIDMove, len(plan.Cards))
		for j, card := range plan.Cards {
			moves[card.Number] = cardIDMove{slug: card.Slug, to: next + j}
		}
		for _, card := range plan.Cards {
			to := moves[card.Number].to
			data, err := os.ReadFile(filepath.Join(part.Dir, cardFileName(card.Number, card.Slug)))
			if err != nil {
				return nil, fmt.Errorf("planparser: join: read part %q's card %d: %w; way forward: run lym loom join-parts again once the file is readable", part.Name, card.Number, err)
			}
			text, _ := rewriteCardIDs(string(moveCardBytes(data, card.Number, to)), cardFileName(to, card.Slug), moves)
			file := cardFileName(to, card.Slug)
			cardFiles[file] = []byte(text)
			indexLines = append(indexLines, fmt.Sprintf("%d — %s — %s", to, card.Slug, card.Summary))
			result.Cards = append(result.Cards, JoinedCard{Part: part.Name, From: card.Number, To: to, File: file})
		}
		next += len(plan.Cards)
	}

	overview := joinedOverview(title, parts, plans, indexLines)
	if err := writeJoinedPlan(planDir, cardFiles, overview); err != nil {
		return nil, fmt.Errorf("planparser: join: %w; way forward: run lym loom join-parts again, which rewrites the whole plan directory from the parts, since a join that failed wrote no complete plan", err)
	}
	return result, nil
}

// checkJoinable refuses parts whose plans cannot be joined: a language or root that differs, an export no card delivers, or a handle or new path two parts create.
func checkJoinable(parts []JoinPart, plans []*Plan) error {
	for i := 1; i < len(parts); i++ {
		if plans[i].Language != plans[0].Language {
			return refuseJoin("parts %q and %q differ in language: %q and %q; way forward: set part %q's overview to `language: %s`, then run lym loom join-parts again", parts[0].Name, parts[i].Name, plans[0].Language, plans[i].Language, parts[i].Name, plans[0].Language)
		}
		if plans[i].Root != plans[0].Root {
			return refuseJoin("parts %q and %q differ in root: %q and %q; way forward: set part %q's overview to `root: %s`, then run lym loom join-parts again", parts[0].Name, parts[i].Name, plans[0].Root, plans[i].Root, parts[i].Name, plans[0].Root)
		}
	}
	for i, part := range parts {
		for _, export := range part.Exports {
			if !ExportMet(plans[i], export) {
				return refuseJoin("part %q: no card creates, re-signs or deletes its export %q; way forward: add the card to part %q's plan, then run lym loom join-parts again", part.Name, export, part.Name)
			}
		}
	}
	creator := map[string]string{}
	for i, part := range parts {
		for _, ref := range createdRefs(plans[i]) {
			if first, claimed := creator[ref]; claimed && first != part.Name {
				return refuseJoin("parts %q and %q both create %q; way forward: keep the creating card in one part and have the other use it, then run lym loom join-parts again", first, part.Name, ref)
			}
			creator[ref] = part.Name
		}
	}
	return nil
}

// createdRefs returns the plan's Create targets and Rename New sides that name something new: a `plan:` handle as written, and any other as its disk path.
func createdRefs(plan *Plan) []string {
	var refs []string
	add := func(ref string) {
		if IsHandleRef(ref) {
			refs = append(refs, ref)
		} else if diskPath, ok := diskPathForRef(plan, ref); ok {
			refs = append(refs, diskPath)
		}
	}
	for _, card := range plan.Cards {
		for _, group := range card.TargetGroups {
			switch group.Type {
			case CardTypeCreate:
				for _, ref := range group.Refs {
					add(ref)
				}
			case CardTypeRename:
				for _, pair := range group.Pairs {
					add(pair.New)
				}
			}
		}
	}
	return refs
}

// ExportMet reports whether a card of the plan creates the export (a Create target or Rename New side), re-signs it (an Edit arrow target) or deletes it (a Delete target).
// A path export is met by its disk path, a glyph exactly, and a `plan:` handle exactly or by the declaration it names: a handle whose body ParseRef reads with the same Unit and Name as the export's.
// The resolve pass canonicalizes a declared handle by spelling its owner chain from the declaration head, so the canonical spelling on disk still meets an export written as drafted.
func ExportMet(plan *Plan, export string) bool {
	for _, card := range plan.Cards {
		for _, group := range card.TargetGroups {
			switch group.Type {
			case CardTypeCreate, CardTypeDelete:
				for _, ref := range group.Refs {
					if refsMatch(plan, ref, export) {
						return true
					}
				}
			case CardTypeRename:
				for _, pair := range group.Pairs {
					if refsMatch(plan, pair.New, export) {
						return true
					}
				}
			case CardTypeEdit:
				for _, resign := range group.Resigns {
					if resign.Target == export {
						return true
					}
				}
			}
		}
	}
	return false
}

// refsMatch reports whether ref names what export names: the same string, the same disk path, or two handles that read to one unit and name.
func refsMatch(plan *Plan, ref, export string) bool {
	if ref == export {
		return true
	}
	if IsHandleRef(ref) || IsHandleRef(export) {
		refBody, refOK := HandleBody(ref)
		exportBody, exportOK := HandleBody(export)
		if !refOK || !exportOK {
			return false
		}
		refGlyph, refErr := ParseRef(plan, refBody)
		exportGlyph, exportErr := ParseRef(plan, exportBody)
		return refErr == nil && exportErr == nil && refGlyph.Unit == exportGlyph.Unit && refGlyph.Name == exportGlyph.Name
	}
	refPath, refOK := diskPathForRef(plan, ref)
	exportPath, exportOK := diskPathForRef(plan, export)
	return refOK && exportOK && refPath == exportPath
}

// joinedOverview renders the joined plan's 00-overview.md.
func joinedOverview(title string, parts []JoinPart, plans []*Plan, indexLines []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "format: %d\napproved: false\n", RecognizedFormat)
	if plans[0].Root != "" {
		fmt.Fprintf(&b, "root: %s\n", plans[0].Root)
	}
	fmt.Fprintf(&b, "language: %s\n", plans[0].Language)
	fmt.Fprintf(&b, "---\n\n# Plan: %s\n\n", title)

	names := make([]string, len(parts))
	for i, part := range parts {
		names[i] = "`" + part.Name + "`"
	}
	fmt.Fprintf(&b, "This plan joins the plans of the parts %s, in that order.\n\n", strings.Join(names, ", "))

	b.WriteString(cardIndexHeading + "\n\n")
	for _, line := range indexLines {
		b.WriteString(line + "\n")
	}

	var decisions strings.Builder
	renameMechanic := false
	for i, plan := range plans {
		if plan.SharedDecisions != "" {
			fmt.Fprintf(&decisions, "### Part %s\n\n%s\n\n", parts[i].Name, plan.SharedDecisions)
		}
		renameMechanic = renameMechanic || plan.RenameMechanic != ""
	}
	if decisions.Len() > 0 {
		fmt.Fprintf(&b, "\n%s\n\n%s", sharedDecisionsHeading, strings.TrimRight(decisions.String(), "\n")+"\n")
	}
	if renameMechanic {
		fmt.Fprintf(&b, "\n%s\n\n%s\n", renameMechanicHeading, canonicalRenameMechanic)
	}
	return b.String()
}

// writeJoinedPlan replaces the top-level card files and overview of planDir with cards and overview, writing the overview last.
func writeJoinedPlan(planDir string, cards map[string][]byte, overview string) error {
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(planDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || (entry.Name() != overviewFileName && !topLevelCardFileRe.MatchString(entry.Name())) {
			continue
		}
		if err := os.Remove(filepath.Join(planDir, entry.Name())); err != nil {
			return err
		}
	}
	for name, data := range cards {
		if err := os.WriteFile(filepath.Join(planDir, name), data, 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(planDir, overviewFileName), []byte(overview), 0o644)
}
