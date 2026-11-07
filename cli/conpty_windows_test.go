//go:build windows

package main

import "testing"

func TestWindowsEnvironmentBlockIsSortedAndDoubleNulTerminated(t *testing.T) {
	block := windowsEnvironmentBlock([]string{"Z_PORT=2", "A_PORT=1"})
	if len(block) < 3 {
		t.Fatalf("environment block is too short: %d", len(block))
	}
	if block[len(block)-1] != 0 || block[len(block)-2] != 0 {
		t.Fatalf("environment block is not double-NUL terminated")
	}
	if block[0] != 'A' {
		t.Fatalf("environment block is not sorted: first code unit %d", block[0])
	}
}
