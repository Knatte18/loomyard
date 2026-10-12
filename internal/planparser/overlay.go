// overlay.go implements Overlay, a read path over a plan directory that answers from proposed contents first and from disk second, so a plan can be parsed, checked and canonicalized in memory without a byte written to `_lyx/plan/`.
// A Plan built through an Overlay carries it, and Plan.ReadFile, Plan.RewriteRefs and Plan.Reparse follow it.

package planparser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// ErrOverlayName is the sentinel NewOverlay wraps when a proposed name is neither 00-overview.md nor a card file the Card Index lists.
var ErrOverlayName = errors.New("planparser: overlay name is not a plan file")

// ErrOverlayFileMissing is the sentinel NewOverlay wraps when a proposed card file is listed in the Card Index but absent on disk.
var ErrOverlayFileMissing = errors.New("planparser: overlay card file is missing on disk")

// Overlay is a plan directory with proposed full contents laid over its files.
// It adds and removes no file: only 00-overview.md and the card files the Card Index on disk lists can be overlaid.
type Overlay struct {
	planDir string
	files   map[string][]byte
}

// NewOverlay returns the Overlay of planDir holding files, proposed full contents keyed by plan file name.
// A name that is not 00-overview.md or a card file the overview on disk lists wraps ErrOverlayName, and a listed card file that is absent on disk wraps ErrOverlayFileMissing.
// An overview on disk that does not parse is returned as its parse error.
func NewOverlay(planDir string, files map[string]string) (*Overlay, error) {
	onDisk := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(planDir, name)) }
	overview, err := readOverview(planDir, onDisk)
	if err != nil {
		return nil, err
	}

	listed := make(map[string]bool, len(overview.entries))
	for _, entry := range overview.entries {
		listed[cardFileName(entry.Number, entry.Slug)] = true
	}

	proposed := make(map[string][]byte, len(files))
	for name, content := range files {
		switch {
		case name == overviewFileName:
		case listed[name]:
			if _, err := os.Stat(filepath.Join(planDir, name)); err != nil {
				return nil, fmt.Errorf("%w: %q", ErrOverlayFileMissing, name)
			}
		default:
			return nil, fmt.Errorf("%w: %q is neither %s nor a card file the Card Index lists", ErrOverlayName, name, overviewFileName)
		}
		proposed[name] = []byte(content)
	}
	return &Overlay{planDir: planDir, files: proposed}, nil
}

// Parse parses the overlaid plan, as ParsePlanFrom does, and records the overlay on the returned Plan.
func (o *Overlay) Parse() (*Plan, error) {
	plan, err := ParsePlanFrom(o.planDir, o.readFile)
	if err != nil {
		return nil, err
	}
	plan.overlay = o
	return plan, nil
}

// ReadFile returns the contents of the plan file name: the proposed contents when the plan was parsed through an Overlay that holds name, and the file under Dir otherwise.
func (p *Plan) ReadFile(name string) ([]byte, error) {
	if p.overlay != nil {
		return p.overlay.readFile(name)
	}
	return os.ReadFile(filepath.Join(p.Dir, name))
}

// RewriteRefs substitutes refs across the plan's card files as the package-level RewriteRefs does, in memory when the plan was parsed through an Overlay and on disk otherwise.
func (p *Plan) RewriteRefs(subs map[string]string) error {
	if p.overlay != nil {
		return rewriteRefsIn(p.Dir, subs, p.overlay.readFile, p.overlay.writeFile)
	}
	return RewriteRefs(p.Dir, subs)
}

// Reparse parses the plan again from its source: the Overlay when the plan was parsed through one, and Dir on disk otherwise.
func (p *Plan) Reparse() (*Plan, error) {
	if p.overlay != nil {
		return p.overlay.Parse()
	}
	return ParsePlan(p.Dir)
}

// readFile returns the proposed contents of name when it is overlaid, and the file under the plan directory otherwise.
func (o *Overlay) readFile(name string) ([]byte, error) {
	if content, ok := o.files[name]; ok {
		return slices.Clone(content), nil
	}
	return os.ReadFile(filepath.Join(o.planDir, name))
}

// writeFile stores data as the proposed contents of name in memory and writes nothing to disk.
func (o *Overlay) writeFile(name string, data []byte) error {
	o.files[name] = slices.Clone(data)
	return nil
}
