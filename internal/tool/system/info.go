package system

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
		fmt.Fprintf(&sb, "Memory:   %s\n", mem)
	}
	if disk, err := diskInfo(); err == nil {
		fmt.Fprintf(&sb, "Disk:\n%s", disk)
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
		data, err := os.ReadFile("/proc/cpuinfo")
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if !strings.HasPrefix(line, "model name") {
					continue
				}
				if _, value, ok := strings.Cut(line, ":"); ok {
					return strings.TrimSpace(value)
				}
			}
		}
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			"(Get-CimInstance Win32_Processor | Select-Object -First 1 -ExpandProperty Name)").Output()
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
		pageSize := int64(4096)
		if pageOut, err := exec.Command("sysctl", "-n", "hw.pagesize").Output(); err == nil {
			if parsed, parseErr := strconv.ParseInt(strings.TrimSpace(string(pageOut)), 10, 64); parseErr == nil && parsed > 0 {
				pageSize = parsed
			}
		}
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
		return memorySummary(totalBytes, usedBytes, freeBytes), nil

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
					return memorySummary(total, used, free), nil
				}
			}
		}
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			"$m=Get-CimInstance Win32_OperatingSystem; "+
				"$total=[int64]$m.TotalVisibleMemorySize*1KB; "+
				"$free=[int64]$m.FreePhysicalMemory*1KB; "+
				"$used=$total-$free; "+
				"'{0},{1},{2}' -f $total,$used,$free").Output()
		if err != nil {
			return "", err
		}
		fields := strings.Split(strings.TrimSpace(string(out)), ",")
		if len(fields) == 3 {
			total, _ := strconv.ParseInt(fields[0], 10, 64)
			used, _ := strconv.ParseInt(fields[1], 10, 64)
			free, _ := strconv.ParseInt(fields[2], 10, 64)
			return memorySummary(total, used, free), nil
		}
	}
	return "", fmt.Errorf("unsupported OS")
}

func diskInfo() (string, error) {
	switch runtime.GOOS {
	case "darwin", "linux":
		out, err := exec.Command("df", "-k", "-P").Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(out), "\n")
		var kept []string
		for _, line := range lines[1:] {
			if line == "" {
				continue
			}
			fs := strings.Fields(line)
			if len(fs) < 6 {
				continue
			}
			skip := false
			for _, prefix := range []string{"devfs", "tmpfs", "map", "none", "udev"} {
				if strings.HasPrefix(fs[0], prefix) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			usedKB, _ := strconv.ParseInt(fs[2], 10, 64)
			freeKB, _ := strconv.ParseInt(fs[3], 10, 64)
			kept = append(kept, fmt.Sprintf("  %s: %s used, %s free", fs[5], formatBytes(usedKB*1024), formatBytes(freeKB*1024)))
		}
		return strings.Join(kept, "\n") + "\n", nil
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			"Get-CimInstance Win32_LogicalDisk -Filter \"DriveType=3\" | "+
				"ForEach-Object { '{0},{1},{2}' -f $_.DeviceID,([int64]$_.Size),([int64]$_.FreeSpace) }").Output()
		if err != nil {
			return "", err
		}
		var kept []string
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Split(line, ",")
			if len(fields) != 3 {
				continue
			}
			size, _ := strconv.ParseInt(fields[1], 10, 64)
			free, _ := strconv.ParseInt(fields[2], 10, 64)
			used := size - free
			drive := strings.TrimSuffix(fields[0], ":")
			kept = append(kept, fmt.Sprintf("  %s: %s used, %s free", drive, formatBytes(used), formatBytes(free)))
		}
		return strings.Join(kept, "\n") + "\n", nil
	default:
		return "", fmt.Errorf("unsupported OS")
	}
}

func memorySummary(total, used, free int64) string {
	percent := 0.0
	if total > 0 {
		percent = float64(used) / float64(total) * 100
	}
	return fmt.Sprintf("%s used / %s total (%.1f%%), %s free",
		formatBytes(used), formatBytes(total), percent, formatBytes(free))
}

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
