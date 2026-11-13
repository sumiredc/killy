package protocol

import "testing"

// Deliberately failing: proves the go job fails on a failing test (#14). This branch is never merged.
func TestCIProbeFails(t *testing.T) {
	t.Fatal("deliberate failure for #14")
}
