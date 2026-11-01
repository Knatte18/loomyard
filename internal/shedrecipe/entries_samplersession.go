// entries_samplersession.go implements the SamplerSession registry entry: it refuses every config key and builds the Sampler-Session row over Env.Sampler.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// samplerSessionKeyWayForward is the way forward of a SamplerSession row that carries a config key.
const samplerSessionKeyWayForward = "way forward: remove the key; a SamplerSession row takes no config"

// samplerSessionEntry is the Constructor for the "SamplerSession" registry row.
// The row takes no config key: the session's agent comes from the sampler module's settings and its paths and seams from Env.Sampler.
// It validates the two absolute record paths and every seam of Env.Sampler but Watch, then builds the row over Env.Sampler.
func samplerSessionEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg); err != nil {
		return nil, fmt.Errorf("%w; row %q: %s", err, row.Name, samplerSessionKeyWayForward)
	}
	sampler := env.Sampler
	for _, path := range []struct{ field, value string }{
		{"Sampler.RecordPath", sampler.RecordPath},
		{"Sampler.RecordLockPath", sampler.RecordLockPath},
	} {
		if err := requireAbsRoot("SamplerSession", path.field, path.value); err != nil {
			return nil, err
		}
	}
	seams := []struct {
		field string
		seam  any
	}{
		{"Sampler.Spec", sampler.Spec},
		{"Sampler.Sessions", sampler.Sessions},
		{"Sampler.SessionLive", sampler.SessionLive},
		{"Sampler.RegisterSession", sampler.RegisterSession},
		{"Sampler.Stamp", sampler.Stamp},
		{"Sampler.DirtyPaths", sampler.DirtyPaths},
		{"Sampler.CheckRecord", sampler.CheckRecord},
		{"Sampler.MailAgent", sampler.MailAgent},
		{"Sampler.PauseCovered", sampler.PauseCovered},
		{"Sampler.Now", sampler.Now},
		{"Sampler.Sleep", sampler.Sleep},
	}
	for _, s := range seams {
		if err := requireSeam("SamplerSession", s.field, s.seam); err != nil {
			return nil, err
		}
	}
	return loomshed.NewSamplerSession(row.Name, sampler), nil
}
