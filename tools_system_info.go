package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func getSystemInfoHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var sb strings.Builder

	hostname, _ := os.Hostname()
	fmt.Fprintf(&sb, "OS:       %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if hostname != "" {
		fmt.Fprintf(&sb, "Hostname: %s\n", hostname)
	}
	fmt.Fprintf(&sb, "CPUs:     %d logical core(s)\n", runtime.NumCPU())

	if model := cpuModel(); model != "" {
		fmt.Fprintf(&sb, "CPU:      %s\n", model)
	}
	if mem, err := memoryInfo(); err == nil {
		fmt.Fprintf(&sb, "\n── Memory ──\n%s", mem)
	}
	if disk, err := diskInfo(); err == nil {
		fmt.Fprintf(&sb, "\n── Disk ──\n%s", disk)
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func cpuModel() string {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "linux":
		out, err := exec.Command("sh", "-c", `grep -m1 "model name" /proc/cpuinfo | cut -d: -f2`).Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

func memoryInfo() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		totalOut, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return "", err
		}
		totalBytes, _ := strconv.ParseInt(strings.TrimSpace(string(totalOut)), 10, 64)

		vmOut, err := exec.Command("vm_stat").Output()
		if err != nil {
			return "", err
		}
		pageSize := int64(16384) // default Apple Silicon page size
		vals := map[string]int64{}
		for _, line := range strings.Split(string(vmOut), "\n") {
			for _, key := range []string{"Pages free", "Pages inactive", "Pages speculative"} {
				if strings.HasPrefix(line, key) {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						v, _ := strconv.ParseInt(strings.TrimRight(parts[len(parts)-1], "."), 10, 64)
						vals[key] = v
					}
				}
			}
		}
		freePages := vals["Pages free"] + vals["Pages inactive"] + vals["Pages speculative"]
		freeBytes := freePages * pageSize
		usedBytes := totalBytes - freeBytes
		return fmt.Sprintf("  Total: %s\n  Used:  %s  (%.1f%%)\n  Free:  %s\n",
			formatBytes(totalBytes),
			formatBytes(usedBytes), float64(usedBytes)/float64(totalBytes)*100,
			formatBytes(freeBytes),
		), nil

	case "linux":
		out, err := exec.Command("free", "-b").Output()
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "Mem:") {
				fields := strings.Fields(line)
				if len(fields) >= 4 {
					total, _ := strconv.ParseInt(fields[1], 10, 64)
					used, _ := strconv.ParseInt(fields[2], 10, 64)
					free, _ := strconv.ParseInt(fields[3], 10, 64)
					return fmt.Sprintf("  Total: %s\n  Used:  %s  (%.1f%%)\n  Free:  %s\n",
						formatBytes(total),
						formatBytes(used), float64(used)/float64(total)*100,
						formatBytes(free),
					), nil
				}
			}
		}
	case "windows":
		out, err := exec.Command("powershell", "-Command",
			"$m = Get-CimInstance Win32_OperatingSystem; "+
				"Write-Output (\"Total:\"+$m.TotalVisibleMemorySize+\" Free:\"+$m.FreePhysicalMemory)").Output()
		if err != nil {
			return "", err
		}
		return "  " + strings.TrimSpace(string(out)) + " KB\n", nil
	}
	return "", fmt.Errorf("unsupported OS")
}

func diskInfo() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin", "linux":
		cmd = exec.Command("df", "-h", "-P")
	case "windows":
		cmd = exec.Command("powershell", "-Command",
			"Get-PSDrive -PSProvider FileSystem | "+
				"Select-Object Name,@{N='Used(GB)';E={[math]::Round($_.Used/1GB,1)}},@{N='Free(GB)';E={[math]::Round($_.Free/1GB,1)}} | "+
				"Format-Table -AutoSize | Out-String")
	default:
		return "", fmt.Errorf("unsupported OS")
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(out), "\n")
	var kept []string
	if runtime.GOOS != "windows" {
		for _, line := range lines {
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "Filesystem") {
				kept = append(kept, "  "+line)
				continue
			}
			fs := strings.Fields(line)
			if len(fs) == 0 {
				continue
			}
			skip := false
			for _, prefix := range []string{"devfs", "tmpfs", "map ", "none", "udev"} {
				if strings.HasPrefix(fs[0], prefix) {
					skip = true
					break
				}
			}
			if !skip {
				kept = append(kept, "  "+line)
			}
		}
	} else {
		for _, line := range lines {
			kept = append(kept, "  "+line)
		}
	}
	return strings.Join(kept, "\n") + "\n", nil
}

// formatBytes converts a byte count to a human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
