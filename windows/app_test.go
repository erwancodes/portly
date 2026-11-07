//go:build windows

package main

import (
	"path/filepath"
	"testing"
)

func TestBundledCLICandidatesSkipGUIExecutable(t *testing.T) {
	gui := filepath.FromSlash(`C:/Users/Test User/Portly/Portly.exe`)
	candidates := bundledCLICandidates(gui)
	if len(candidates) != 2 {
		t.Fatalf("candidates=%v", candidates)
	}
	if candidates[0] != filepath.FromSlash(`C:/Users/Test User/Portly/bin/portly.exe`) {
		t.Fatalf("first candidate=%q", candidates[0])
	}
	if candidates[1] != filepath.FromSlash(`C:/Users/Test User/Portly/cli/portly.exe`) {
		t.Fatalf("second candidate=%q", candidates[1])
	}
}
