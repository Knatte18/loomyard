// check.go declares Finding, Check, which judges a parsed parts file against the decision record's ids, and Validate, which runs Parse and Check over the files on disk.

package partsparser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
)

// Finding is one violation the parts check found.
// Part names the part, or the shared spot's path, the finding is about; Detail ends in its way forward.
type Finding struct {
	Check  string
	Part   string
	Detail string
}

const (
	checkPartsMissing        = "parts-missing"
	checkPartsMalformed      = "parts-malformed"
	checkPartsNone           = "parts-none"
	checkPartsOverMax        = "parts-over-max"
	checkPartNameInvalid     = "part-name-invalid"
	checkPartNameRepeated    = "part-name-repeated"
	checkPartNameArchive     = "part-name-archive"
	checkPartDependencyOrder = "part-dependency-order"
	checkSpotNeederUnknown   = "spot-needer-unknown"
	checkSpotExportOrder     = "spot-export-order"
	checkExportClaimedTwice  = "export-claimed-twice"
	checkSpotInArea          = "spot-in-area"
	checkDecisionUnnamed     = "decision-unnamed"
	checkDecisionUnknown     = "decision-unknown"
)

// decisionHolder is a part or shared spot, by its name or path, with the decision ids it names.
type decisionHolder struct {
	label     string
	decisions []string
}

// Check judges parts against the record's decision ids and the most parts max allows, and returns one Finding per violation.
func Check(parts *Parts, decisionIDs []string, max int) []Finding {
	var findings []Finding
	add := func(check, part, detail string) {
		findings = append(findings, Finding{Check: check, Part: part, Detail: detail})
	}

	if len(parts.Parts) == 0 {
		add(checkPartsNone, "", "the file holds no part; way forward: write at least one part, a single part covering the whole task when it does not split")
	}
	if len(parts.Parts) > max {
		add(checkPartsOverMax, "", fmt.Sprintf("the file holds %d parts and at most %d are allowed; way forward: merge parts until at most %d remain", len(parts.Parts), max, max))
	}

	index := map[string]int{}
	for i, part := range parts.Parts {
		if err := seatengine.ValidateSeatName(part.Name); err != nil {
			add(checkPartNameInvalid, part.Name, fmt.Sprintf("part name %q is not usable: %v; way forward: rename the part to a lower-case name that starts with a letter", part.Name, err))
		}
		if earlier, repeated := index[part.Name]; repeated {
			add(checkPartNameRepeated, part.Name, fmt.Sprintf("part name %q is already held by part %d; way forward: rename one of the two parts", part.Name, earlier+1))
		} else {
			index[part.Name] = i
		}
		if strings.HasPrefix(part.Name, ArchivePrefix) {
			add(checkPartNameArchive, part.Name, fmt.Sprintf("part name %q begins with %q, so its directory would be skipped as an earlier archive; way forward: rename the part", part.Name, ArchivePrefix))
		}
	}

	for i, part := range parts.Parts {
		for _, dependency := range part.DependsOn {
			if at, known := index[dependency]; !known || at >= i {
				add(checkPartDependencyOrder, part.Name, fmt.Sprintf("part %q depends on %q, which is no earlier part; way forward: move the depended-on part above it in the file or drop the dependency", part.Name, dependency))
			}
		}
	}

	exportOwner := map[string]int{}
	for i, part := range parts.Parts {
		for _, export := range part.Exports {
			if _, claimed := exportOwner[export]; !claimed {
				exportOwner[export] = i
			}
		}
	}

	for _, spot := range parts.Spots {
		firstNeeder := -1
		for _, name := range spot.NeededBy {
			at, known := index[name]
			if !known {
				add(checkSpotNeederUnknown, spot.Path, fmt.Sprintf("shared spot %q is needed by %q, which is no part; way forward: name a part of the file", spot.Path, name))
				continue
			}
			if firstNeeder < 0 || at < firstNeeder {
				firstNeeder = at
			}
		}
		if firstNeeder < 0 {
			continue
		}
		for _, use := range spot.Uses {
			if owner, exported := exportOwner[use]; exported && owner > firstNeeder {
				add(checkSpotExportOrder, spot.Path, fmt.Sprintf("shared spot %q uses %q, exported by part %q, which comes after %q, the first part that needs the spot; way forward: reorder the parts or move the dependency into a part", spot.Path, use, parts.Parts[owner].Name, parts.Parts[firstNeeder].Name))
			}
		}
	}

	claimant := map[string]string{}
	reported := map[string]bool{}
	claim := func(ref, owner string) {
		first, claimed := claimant[ref]
		if !claimed {
			claimant[ref] = owner
			return
		}
		if first != owner && !reported[ref] {
			reported[ref] = true
			add(checkExportClaimedTwice, owner, fmt.Sprintf("%q is exported by both %s and %s; way forward: keep the export on one of them", ref, first, owner))
		}
	}
	for _, part := range parts.Parts {
		for _, export := range part.Exports {
			claim(export, fmt.Sprintf("part %q", part.Name))
		}
	}
	for _, spot := range parts.Spots {
		for _, export := range spot.Exports {
			claim(export, fmt.Sprintf("shared spot %q", spot.Path))
		}
	}

	for _, spot := range parts.Spots {
		for _, part := range parts.Parts {
			if slices.Contains(part.Area, spot.Path) {
				add(checkSpotInArea, part.Name, fmt.Sprintf("part %q lists the shared spot %q as a file entry of its area; way forward: drop it from the area, since the chair writes the spot", part.Name, spot.Path))
			}
		}
	}

	named := map[string]bool{}
	var holders []decisionHolder
	for _, part := range parts.Parts {
		holders = append(holders, decisionHolder{part.Name, part.Decisions})
	}
	for _, spot := range parts.Spots {
		holders = append(holders, decisionHolder{spot.Path, spot.Decisions})
	}
	for _, id := range decisionIDs {
		for _, holder := range holders {
			if slices.Contains(holder.decisions, id) {
				named[id] = true
			}
		}
		if !named[id] {
			add(checkDecisionUnnamed, "", fmt.Sprintf("decision %s of the record is named by no part or shared spot; way forward: name it in the part or shared spot that carries it", id))
		}
	}
	for _, holder := range holders {
		for _, id := range holder.decisions {
			if !slices.Contains(decisionIDs, id) {
				add(checkDecisionUnknown, holder.label, fmt.Sprintf("%q names decision %s, which the record does not hold; way forward: drop or correct the id", holder.label, id))
			}
		}
	}
	return findings
}

// Validate is the one parts check the parts gate and `lym loom validate-parts` run: it reads the parts file under anchorPath, parses it, reads the decision record at recordPath and checks the parts against its decision ids and the most parts max allows.
// An absent parts file is a parts-missing finding and a file Parse refuses is a parts-malformed finding.
// Any read error, an absent record included, is returned: the record is Discussion-Write's committed output, which the Parts-Write agent cannot write.
func Validate(anchorPath, recordPath string, max int) ([]Finding, error) {
	record, err := os.ReadFile(recordPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("the decision record %s is absent, and the Parts-Write agent cannot write it; run lym loom validate-discussion and tell the parent: %w", recordPath, err)
		}
		return nil, fmt.Errorf("read the decision record %s: %w", recordPath, err)
	}
	data, err := os.ReadFile(File(anchorPath))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []Finding{{Check: checkPartsMissing, Detail: fmt.Sprintf("%s does not exist; way forward: write the parts file there in the format of contracts/specs/loom-parts-spec.md", DirRel()+"/"+fileName)}}, nil
		}
		return nil, fmt.Errorf("read the parts file %s: %w", File(anchorPath), err)
	}
	parts, err := Parse(data)
	if err != nil {
		return []Finding{{Check: checkPartsMalformed, Detail: err.Error()}}, nil
	}
	return Check(parts, discussionparser.DecisionIDs(record), max), nil
}
