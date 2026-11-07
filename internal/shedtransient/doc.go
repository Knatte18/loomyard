// Package shedtransient translates a lower-level package's own failure classification into shedengine's transient mark.
// It is the one place that does so.
// shedengine stays stdlib-only, and the leaf packages whose failures it translates import no shedengine.
// The producer boundary and the bootstrap boundary share this single mapping.
// The producer boundary is the classifier shedbuild.NewShed tells a Shed.
// The bootstrap boundary is the PreStep hooks.
//
// A spawn the host refused for EAGAIN is a host-resources failure.
// So is an agent death that follows such a refusal in the same process within two minutes.
// The refusal is read from proc's spawn-refusal record.
// A death with no refusal in that window stays a hard failure.
//
// Import allowlist: stdlib, shedengine, shuttleengine, gitexec, githubclient and proc, and nothing else;
// seam_enforcement_test.go enforces it.
package shedtransient
