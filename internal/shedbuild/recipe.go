// recipe.go declares this package's two exported types: Recipe, the decoded recipe document, and
// Row, one producer entry within it.

package shedbuild

// Recipe is a decoded recipe document: the whole graph of producer rows plus the graph metadata
// Build never consumes.
//
// Entry and Terminals are graph metadata Build never consumes, surfaced because shedcheck.Check
// takes both as told arguments and refuses to infer either.
type Recipe struct {
	Version   int      `yaml:"version"`
	Entry     string   `yaml:"entry"`
	Terminals []string `yaml:"terminals"`
	Producers []Row    `yaml:"producers"`
}

// Row is one producer entry in a Recipe: the row's identity, its engine name, its portable
// configuration, its routing, and its bounce budget.
//
// Config is the decoded sub-map passed to the row's engine untouched -- never normalised,
// lowercased, or key-walked -- because each registry entry owns its own key validation.
type Row struct {
	Name       string         `yaml:"name"`
	Engine     string         `yaml:"engine"`
	Config     map[string]any `yaml:"config"`
	OnDone     string         `yaml:"on_done"`
	OnStuck    string         `yaml:"on_stuck"`
	Segment    string         `yaml:"segment"`
	MaxBounces int            `yaml:"max_bounces"`
}
