// Package shedtransient is the one place that translates a lower-level package's own failure classification into shedengine's transient mark.
// shedengine stays stdlib-only and the leaf packages (gitexec, githubclient, proc, shuttleengine) gain no shedengine import;
// the producer boundary (the classifier shedbuild.NewShed tells a Shed) and the bootstrap boundary (the PreStep hooks) share this single mapping.
//
// A spawn the host refused for EAGAIN is a host-resources failure, and so is an agent death that follows such a refusal in the same process within two minutes, read from proc's spawn-refusal record;
// a death with no refusal in that window stays a hard failure.
//
// Import allowlist: stdlib, shedengine, shuttleengine, gitexec, githubclient and proc, and nothing else;
// seam_enforcement_test.go enforces it.
package shedtransient
