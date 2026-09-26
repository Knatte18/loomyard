// Package shedverbs implements the generic cobra verb bodies -- run, step, status, and pause --
// shared by every module that arms a *shedengine.Shed onto a CLI subtree. It owns the four verb
// bodies and nothing else: it derives no path of its own, imports no resolver, and never calls
// os.Getwd or internal/lyxcwd -- every path this package touches reaches it told, through a Spec
// the arming module fills. It also imports no <module>cli package, which is what keeps a consuming
// module's own imports acyclic: internal/shedcli (or any other CLI module wiring shedverbs.Verbs
// onto its own subtree) can safely import shedverbs, but shedverbs never imports back.
// The one resolving import it admits is internal/logger, for boundary logging and the two
// sink-location accessors, per the Shed Verb-Set Invariant.
package shedverbs
