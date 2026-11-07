//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const foreverTaskName = "Portly"

type foreverState struct {
	Enabled          bool   `json:"enabled"`
	Loaded           bool   `json:"loaded"`
	Label            string `json:"label"`
	Unit             string `json:"unit"`
	DaemonExecutable string `json:"daemonExecutable"`
}

func foreverBackendName() string { return "Windows Task Scheduler" }

func taskSchedulerAvailable() bool {
	_, err := exec.LookPath("schtasks.exe")
	return err == nil
}

func taskQuery() (string, error) {
	return commandOutput("/Query", "/TN", foreverTaskName, "/FO", "CSV", "/NH")
}

func currentForeverState() foreverState {
	exe, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	query, err := taskQuery()
	loaded := err == nil && strings.Contains(strings.ToLower(query), "running")
	return foreverState{
		Enabled:          err == nil,
		Loaded:           loaded,
		Label:            foreverTaskName,
		Unit:             `\` + foreverTaskName,
		DaemonExecutable: exe,
	}
}

func foreverEnable(c *portlyClient) (foreverState, error) {
	if !taskSchedulerAvailable() {
		return foreverState{}, fmt.Errorf("Windows Task Scheduler is not available on this host")
	}
	exe, err := os.Executable()
	if err != nil {
		return foreverState{}, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	active := snapshotActiveServers(c)
	if c.reachable(time.Second) {
		_ = c.request("POST", "quit", map[string]any{}, &actionResponse{}, false)
		waitUntilUnreachable(c)
	}
	// /TR receives one argument, so quoting the executable protects paths such
	// as C:\Users\John Doe\Tools\portly.exe without invoking a shell.
	command := fmt.Sprintf(`"%s" daemon`, strings.ReplaceAll(exe, `"`, `\"`))
	if out, err := commandOutput("/Create", "/SC", "ONLOGON", "/TN", foreverTaskName, "/TR", command, "/F"); err != nil {
		return foreverState{}, fmt.Errorf("could not register Portly in Task Scheduler: %s", strings.TrimSpace(out))
	}
	if out, err := commandOutput("/Run", "/TN", foreverTaskName); err != nil {
		return foreverState{}, fmt.Errorf("could not start Portly from Task Scheduler: %s", strings.TrimSpace(out))
	}
	if !waitForAPI(c) {
		return foreverState{}, fmt.Errorf("Task Scheduler registered Portly, but its control API did not become reachable")
	}
	restoreServers(c, active)
	return currentForeverState(), nil
}

func foreverDisable(c *portlyClient) (foreverState, error) {
	active := snapshotActiveServers(c)
	if out, err := commandOutput("/Delete", "/TN", foreverTaskName, "/F"); err != nil && !strings.Contains(strings.ToLower(out), "cannot find") {
		return foreverState{}, fmt.Errorf("could not remove Portly from Task Scheduler: %s", strings.TrimSpace(out))
	}
	if len(active) > 0 {
		if err := c.launchDaemonIfNeeded(); err != nil {
			return foreverState{}, fmt.Errorf("launch at login was disabled, but Portly could not be relaunched: %w", err)
		}
		restoreServers(c, active)
	}
	return currentForeverState(), nil
}

func commandOutput(args ...string) (string, error) {
	out, err := exec.Command("schtasks.exe", args...).CombinedOutput()
	return string(out), err
}

func snapshotActiveServers(c *portlyClient) []string {
	if !c.reachable(time.Second) {
		return nil
	}
	var status PortlyStatus
	if err := c.request("GET", "status", nil, &status, false); err != nil {
		return nil
	}
	var ids []string
	for _, project := range status.Projects {
		for _, server := range project.Servers {
			if server.State != StateStopped && server.State != StateFailed {
				ids = append(ids, server.ID)
			}
		}
	}
	return ids
}

func restoreServers(c *portlyClient, ids []string) {
	for _, id := range ids {
		_ = c.post("start", targetRequest{Server: &id}, &actionResponse{})
	}
}

func waitUntilUnreachable(c *portlyClient) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && c.reachable(300*time.Millisecond) {
		time.Sleep(200 * time.Millisecond)
	}
}

func waitForAPI(c *portlyClient) bool {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if c.reachable(500 * time.Millisecond) {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}
