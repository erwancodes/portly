//go:build windows

package main

import "testing"

func TestForeverUsesWindowsTaskScheduler(t *testing.T) {
	if foreverBackendName() != "Windows Task Scheduler" {
		t.Fatalf("backend=%q", foreverBackendName())
	}
	state := currentForeverState()
	if state.Unit != `\Portly` {
		t.Fatalf("unit=%q", state.Unit)
	}
}
