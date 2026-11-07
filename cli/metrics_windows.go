//go:build windows

package main

import (
	"encoding/json"
	"os/exec"
	"strings"
)

type processMetrics struct {
	CPUPercent          float64
	MemoryBytes         uint64
	ResidentMemoryBytes uint64
	ProcessCount        int
}

type procRecord struct {
	PID        int
	ParentPID  int
	RSSBytes   uint64
	Footprint  uint64
	CPUPercent float64
	Command    string
}

func sampleMetrics(rootPIDs map[int]struct{}) map[int]processMetrics {
	records := listProcesses()
	parent := map[int]int{}
	for _, rec := range records {
		parent[rec.PID] = rec.ParentPID
	}
	owner := func(pid int) int {
		seen := map[int]bool{}
		current := pid
		for current > 0 && !seen[current] {
			seen[current] = true
			if _, ok := rootPIDs[current]; ok {
				return current
			}
			next, ok := parent[current]
			if !ok {
				return 0
			}
			current = next
		}
		return 0
	}
	totals := map[int]processMetrics{}
	for _, rec := range records {
		root := owner(rec.PID)
		if root == 0 {
			continue
		}
		m := totals[root]
		m.CPUPercent += rec.CPUPercent
		m.MemoryBytes += rec.Footprint
		m.ResidentMemoryBytes += rec.RSSBytes
		m.ProcessCount++
		totals[root] = m
	}
	return totals
}

type windowsProcessRecord struct {
	PID        int     `json:"pid"`
	ParentPID  int     `json:"parentPid"`
	Memory     uint64  `json:"memory"`
	CPUPercent float64 `json:"cpuPercent"`
	Name       string  `json:"name"`
}

func listProcesses() []procRecord {
	powershell := windowsPowerShell()
	if powershell == "" {
		return nil
	}
	// Win32_PerfFormattedData_PerfProc_Process provides a point-in-time CPU
	// percentage and the process parent relationship without scraping tasklist.
	// WorkingSetPrivate is the Windows equivalent used for Portly's managed
	// memory totals; it is reported as private working set, not macOS footprint.
	script := `$ErrorActionPreference = 'SilentlyContinue'; @(Get-CimInstance Win32_PerfFormattedData_PerfProc_Process | Where-Object { $_.IDProcess -gt 0 } | ForEach-Object { [PSCustomObject]@{ pid = [int]$_.IDProcess; parentPid = [int]$_.CreatingProcessID; memory = [uint64]$_.WorkingSetPrivate; cpuPercent = [double]$_.PercentProcessorTime; name = [string]$_.Name } }) | ConvertTo-Json -Compress`
	out, err := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script).Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return nil
	}
	var rows []windowsProcessRecord
	if err := json.Unmarshal(out, &rows); err != nil {
		var row windowsProcessRecord
		if singleErr := json.Unmarshal(out, &row); singleErr != nil {
			return nil
		}
		rows = []windowsProcessRecord{row}
	}
	recs := make([]procRecord, 0, len(rows))
	for _, row := range rows {
		if row.PID <= 0 {
			continue
		}
		recs = append(recs, procRecord{
			PID:        row.PID,
			ParentPID:  row.ParentPID,
			RSSBytes:   row.Memory,
			Footprint:  row.Memory,
			CPUPercent: row.CPUPercent,
			Command:    row.Name,
		})
	}
	return recs
}

func windowsPowerShell() string {
	for _, candidate := range []string{"pwsh.exe", "powershell.exe"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}
