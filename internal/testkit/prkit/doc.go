// Package prkit is a scripted fake of pullrequest.API, shared by tests that drive GitHub through one stub.
// Each method has one scripted answer list and one recorded call list, guarded by a mutex.
// It asserts nothing.
package prkit
