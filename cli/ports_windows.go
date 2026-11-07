//go:build windows

package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type listener struct {
	Port    int
	PID     int
	Command string
	User    string
}

type stopOutcome struct {
	Description     string
	ProcessID       *int
	DockerContainer *dockerContainer
}

func occupantOf(port int) *listener {
	out, err := exec.Command("netstat.exe", "-ano", "-p", "tcp").Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.EqualFold(fields[0], "TCP") || !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		if netstatPort(fields[1]) != port {
			continue
		}
		pid, err := strconv.Atoi(fields[4])
		if err != nil || pid <= 0 {
			continue
		}
		return &listener{Port: port, PID: pid, Command: commandForPID(pid), User: currentUser()}
	}
	return nil
}

func netstatPort(address string) int {
	colon := strings.LastIndex(address, ":")
	if colon < 0 || colon == len(address)-1 {
		return 0
	}
	port, _ := strconv.Atoi(address[colon+1:])
	return port
}

func isListening(port int) bool { return occupantOf(port) != nil }

func stopOccupant(port int, expectedPID *int) (stopOutcome, error) {
	occ := occupantOf(port)
	if occ == nil {
		return stopOutcome{}, fmt.Errorf("The port is already free.")
	}
	if expectedPID != nil && occ.PID != *expectedPID {
		return stopOutcome{}, fmt.Errorf("The listener changed before Portly could stop it. Refresh and try again.")
	}
	if container := dockerContainerPublishing(port); container != nil {
		if err := stopDockerContainer(*container); err != nil {
			return stopOutcome{}, err
		}
		return stopOutcome{Description: "Docker container " + container.displayName(), DockerContainer: container}, nil
	}
	if isDockerDaemonCommand(occ.Command) {
		return stopOutcome{}, fmt.Errorf("Refusing to signal the Docker daemon. Stop the published container instead.")
	}
	forced := false
	if err := terminatePID(occ.PID); err != nil {
		if forceErr := forceTerminatePID(occ.PID); forceErr != nil {
			return stopOutcome{}, fmt.Errorf("Process %d refused the stop request: %v; force stop failed: %w", occ.PID, err, forceErr)
		}
		forced = true
	}
	pid := occ.PID
	description := fmt.Sprintf("%s (pid %d)", occ.Command, occ.PID)
	if forced {
		description += " (forced)"
	}
	return stopOutcome{Description: description, ProcessID: &pid}, nil
}

func commandForPID(pid int) string {
	cmd := exec.Command("tasklist.exe", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	reader := csv.NewReader(strings.NewReader(string(out)))
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(row) == 0 || strings.HasPrefix(row[0], "INFO:") {
			continue
		}
		return row[0]
	}
	return "unknown"
}

func currentUser() string {
	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}
	return os.Getenv("USER")
}
