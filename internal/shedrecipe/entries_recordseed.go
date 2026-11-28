// entries_recordseed.go implements recordSeedEntry, the Constructor for the "RecordSeed" registry row:
// a Go row that seeds a loom run's decision record from its board entry's stored record, with no agent.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// recordSeedEntry is the Constructor for the "RecordSeed" registry row.
// It takes no Config key and returns loomshed.NewRecordSeed over Env.RecordSeed.
// The record, support log, discussion directory and seed paths must be absolute, and the slug non-empty.
// ReadStored, Commit and ReadSeed must be non-nil; the refusal names the Env field.
func recordSeedEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	const entry = LoomSeedEngine
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	deps := env.RecordSeed
	for _, path := range []struct{ field, value string }{
		{"RecordSeed.RecordPath", deps.RecordPath},
		{"RecordSeed.SupportLogPath", deps.SupportLogPath},
		{"RecordSeed.DiscussionDir", deps.DiscussionDir},
		{"RecordSeed.SeedPath", deps.SeedPath},
	} {
		if err := requireAbsRoot(entry, path.field, path.value); err != nil {
			return nil, err
		}
	}
	if err := requireNonEmpty(entry, "RecordSeed.Slug", deps.Slug); err != nil {
		return nil, err
	}
	for _, seam := range []struct {
		field string
		value any
	}{
		{"RecordSeed.ReadStored", deps.ReadStored},
		{"RecordSeed.Commit", deps.Commit},
		{"RecordSeed.ReadSeed", deps.ReadSeed},
	} {
		if err := requireSeam(entry, seam.field, seam.value); err != nil {
			return nil, err
		}
	}

	return loomshed.NewRecordSeed(row.Name, deps), nil
}
