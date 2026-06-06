package multi

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bearaujus/bmcptools/internal/helper"
)

const (
	defaultPathExistsBatchLimit      = 500
	defaultMultipleFileInfoPathLimit = 100
	defaultReadMultipleTotalBytes    = 1 * 1024 * 1024
	defaultFindReplaceMaxFileSize    = 10 * 1024 * 1024
	defaultFindReplaceTotalDiffBytes = 256 * 1024
)

func writeLimitedPathList(sb *strings.Builder, paths []string, limit int) {
	if limit <= 0 || len(paths) <= limit {
		for _, p := range paths {
			fmt.Fprintf(sb, "  %s\n", p)
		}
		return
	}
	for _, p := range paths[:limit] {
		fmt.Fprintf(sb, "  %s\n", p)
	}
	fmt.Fprintf(sb, "  ... and %s more\n", helper.Pluralize(len(paths)-limit, "path"))
}

func batchLimit(req mcp.CallToolRequest, defaultLimit int) int {
	limit := int(req.GetFloat("limit", float64(defaultLimit)))
	if limit < 0 {
		return defaultLimit
	}
	return limit
}

func pathExistsBatchHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	paths := req.GetStringSlice("paths", nil)
	if len(paths) == 0 {
		return mcp.NewToolResultError("paths is required (array of file/directory paths)"), nil
	}

	limit := batchLimit(req, defaultPathExistsBatchLimit)
	total := len(paths)
	truncated := false
	if limit > 0 && len(paths) > limit {
		paths = paths[:limit]
		truncated = true
	}

	var sb strings.Builder
	existsCount := 0
	missingCount := 0
	for i, p := range paths {
		if i > 0 {
			sb.WriteString("\n")
		}
		info, err := os.Lstat(p)
		if err != nil {
			fmt.Fprintf(&sb, "%s: false", p)
			missingCount++
			continue
		}
		existsCount++
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		}
		if info.Mode()&os.ModeSymlink != 0 {
			kind = "symlink"
		}
		fmt.Fprintf(&sb, "%s: %s (%s)", p, kind, helper.HumanizeBytes(info.Size()))
	}
	fmt.Fprintf(&sb, "\n\nSummary: checked %d of %d path(s); %d exist, %d missing/inaccessible.",
		len(paths), total, existsCount, missingCount)
	if truncated {
		fmt.Fprintf(&sb, " Output truncated after %d paths; increase limit or set limit=0 for all.", limit)
	}
	return mcp.NewToolResultText(sb.String()), nil
}
