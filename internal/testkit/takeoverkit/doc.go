// Package takeoverkit writes another machine's takeover entry into a run's status file.
// It serves the tests of fabricengine and pairteardown that plant a takeover on origin.
// The kit imports only encoding/json, os and internal/shedengine; the caller commits and pushes the file.
package takeoverkit
