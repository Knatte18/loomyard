// Package shedtransient is the one place that translates a lower-level package's own failure
// classification into shedengine's transient mark. shedengine stays stdlib-only and the leaf
// packages (gitexec, githubclient, shuttleengine) gain no shedengine import; the producer boundary
// (the classifier shedbuild.NewShed tells a Shed) and the bootstrap boundary (the PreStep hooks)
// share this single mapping.
//
// Import allowlist: stdlib, shedengine, shuttleengine, gitexec and githubclient, and nothing else;
// seam_enforcement_test.go enforces it.
package shedtransient
