//go:build windows

package main

import "testing"

func TestNetstatPort(t *testing.T) {
	cases := map[string]int{
		"0.0.0.0:5173":   5173,
		"127.0.0.1:3000": 3000,
		"[::]:7737":      7737,
		"[::1]:8080":     8080,
		"bad":            0,
	}
	for input, want := range cases {
		if got := netstatPort(input); got != want {
			t.Fatalf("netstatPort(%q)=%d, want %d", input, got, want)
		}
	}
}
