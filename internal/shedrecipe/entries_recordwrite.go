// entries_recordwrite.go implements recordWriteEntry, the Constructor for the "RecordWrite" registry row:
// a Go row that writes a loom run's decision record and support log from its board entry, with no agent.

package shedrecipe

import (
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// recordWriteEntry is the Constructor for the "RecordWrite" registry row: it takes no Config key and returns loomshed.NewRecordWrite over Env.RecordWrite.
// The record, support log and discussion directory paths must be absolute, and ReadBody, ReadStored and Commit must be non-nil; the refusal names the Env field.
func recordWriteEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	const entry = "RecordWrite"
	if err := configRejectUnknown(cfg); err != nil {
		return nil, err
	}
	deps := env.RecordWrite
	for _, path := range []struct{ field, value string }{
		{"RecordWrite.RecordPath", deps.RecordPath},
		{"RecordWrite.SupportLogPath", deps.SupportLogPath},
		{"RecordWrite.DiscussionDir", deps.DiscussionDir},
	} {
		if err := requireAbsRoot(entry, path.field, path.value); err != nil {
			return nil, err
		}
	}
	for _, seam := range []struct {
		field string
		value any
	}{
		{"RecordWrite.ReadBody", deps.ReadBody},
		{"RecordWrite.ReadStored", deps.ReadStored},
		{"RecordWrite.Commit", deps.Commit},
	} {
		if err := requireSeam(entry, seam.field, seam.value); err != nil {
			return nil, err
		}
	}

	return loomshed.NewRecordWrite(row.Name, deps), nil
}
