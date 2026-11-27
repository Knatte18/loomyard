// Package fabriccore holds fabric's rules, as pure functions over what fabric has read.
// A rule's input is the snapshot, the facts read from the hub and from origin,
// and its output is an answer that refuses, asks for another read phase, or lists actions to run.
// The words are "snapshot" and "action list"; neither is called a plan.
//
// The package holds the snapshot, phase, action list and outcome types every rule reads and returns,
// and it is the home of every fabric rule over them.
//
// It imports only the standard library and internal/fabriclayout.
// It runs no git, starts no process, touches no filesystem and takes no lock.
// What reads a snapshot's facts and runs an action list lives in internal/fabricengine.
package fabriccore
