package system

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type processInfo struct {
	PID     int
	Name    string
	CPU     float64
	Mem     float64
	Command string
}

func listProcessesHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter := strings.ToLower(strings.TrimSpace(req.GetString("filter", "")))
	sortBy := strings.ToLower(strings.TrimSpace(req.GetString("sort_by", "pid")))
	limit := int(req.GetFloat("limit", 50))
	if limit <= 0 {
		limit = 50
	}

	procs, err := listProcesses()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list processes failed: %v", err)), nil
	}

	if filter != "" {
		filtered := procs[:0]
		for _, p := range procs {
			if strings.Contains(strings.ToLower(p.Name), filter) ||
				strings.Contains(strings.ToLower(p.Command), filter) {
				filtered = append(filtered, p)
			}
		}
		procs = filtered
	}

	switch sortBy {
	case "cpu":
		sort.Slice(procs, func(i, j int) bool { return procs[i].CPU > procs[j].CPU })
	case "mem":
		sort.Slice(procs, func(i, j int) bool { return procs[i].Mem > procs[j].Mem })
	default:
		sort.Slice(procs, func(i, j int) bool { return procs[i].PID < procs[j].PID })
	}

	total := len(procs)
	if total > limit {
		procs = procs[:limit]
	}

	if len(procs) == 0 {
		msg := "No processes found"
		if filter != "" {
			msg += fmt.Sprintf(" matching %q", filter)
		}
		return mcp.NewToolResultText(msg + "."), nil
	}

	var sb strings.Builder
	maxNameLen := 4
	for _, p := range procs {
		if len(p.Name) > maxNameLen {
			maxNameLen = len(p.Name)
		}
	}
	maxCmdLen := 60
	nameColFmt := fmt.Sprintf("%%-%ds", maxNameLen)
	cpuHeader, memHeader := "CPU%", "MEM%"
	if runtime.GOOS == "windows" {
		cpuHeader, memHeader = "CPU(s)", "MEM(MB)"
	}
	header := fmt.Sprintf("%-8s "+nameColFmt+" %7s %8s  %s\n", "PID", "NAME", cpuHeader, memHeader, "COMMAND")
	fmt.Fprint(&sb, header)
	fmt.Fprintln(&sb, strings.Repeat("\u2500", 8+1+maxNameLen+1+7+1+8+2+maxCmdLen))
	for _, p := range procs {
		cmd := p.Command
		if len(cmd) > maxCmdLen {
			cmd = cmd[:maxCmdLen-3] + "..."
		}
		fmt.Fprintf(&sb, "%-8d "+nameColFmt+" %7.1f %8.1f  %s\n", p.PID, p.Name, p.CPU, p.Mem, cmd)
	}
	fmt.Fprintf(&sb, "\nShowing %d of %d processes", len(procs), total)
	if filter != "" {
		fmt.Fprintf(&sb, " matching %q", filter)
	}
	sb.WriteByte('.')

	return mcp.NewToolResultText(sb.String()), nil
}

func listProcesses() ([]processInfo, error) {
	switch runtime.GOOS {
	case "darwin", "linux":
		return listProcessesPosix()
	case "windows":
		return listProcessesWindows()
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func listProcessesPosix() ([]processInfo, error) {
	out, err := exec.Command("ps", "axo", "pid,pcpu,pmem,comm,command").Output()
	if err != nil {
		return nil, err
	}
	var procs []processInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		cpu, _ := strconv.ParseFloat(fields[1], 64)
		mem, _ := strconv.ParseFloat(fields[2], 64)
		name := filepath.Base(fields[3])
		command := strings.Join(fields[4:], " ")
		procs = append(procs, processInfo{PID: pid, Name: name, CPU: cpu, Mem: mem, Command: command})
	}
	return procs, nil
}

func listProcessesWindows() ([]processInfo, error) {
	out, err := exec.Command("powershell", "-Command",
		"Get-Process | Select-Object Id,Name,CPU,WorkingSet | ConvertTo-Csv -NoTypeInformation").Output()
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(bytes.NewReader(out))
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse process CSV: %w", err)
	}
	var procs []processInfo
	for _, fields := range records[1:] { // skip header row
		if len(fields) < 4 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		name := fields[1]
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		memBytes, _ := strconv.ParseFloat(fields[3], 64)
		procs = append(procs, processInfo{PID: pid, Name: name, CPU: cpu, Mem: memBytes / 1024 / 1024, Command: name})
	}
	return procs, nil
}
