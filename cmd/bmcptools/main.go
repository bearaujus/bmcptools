package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	bmcptools "github.com/bearaujus/bmcptools"
	"github.com/mark3labs/mcp-go/server"
)

// serverVersion is overridden at build time via -ldflags "-X main.serverVersion=<tag>".
// Falls back to "dev" when building without ldflags (e.g. go run . locally).
var serverVersion = "dev"

const usage = `bmcptools — MCP server with developer tools.

Usage:
  bmcptools [flags]

Flags:
  --disable=GROUPS    Comma-separated tool groups to NOT register.
                      Example: --disable=user,system
                      Also reads env var BMCPTOOLS_DISABLE (flag wins).
  --exclude-tools=TOOLS
                      Comma-separated tool names to NOT register.
                      Example: --exclude-tools=ask_user,update_dialog,cancel_ask_user
                      Also reads env var BMCPTOOLS_EXCLUDE_TOOLS (flag wins).
  --list-groups       Print available groups and exit.
  --list-tools        Print available tool names and exit.
  --version           Print version and exit.
  -h, --help          Print this help and exit.

By default ALL tool groups and tools are registered. Excluded groups/tools are
also stripped from the server instructions sent to the AI.
`

func main() {
	bmcptools.Version = serverVersion

	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	disableFlag := flag.String("disable", "", "comma-separated tool groups to disable")
	excludeToolsFlag := flag.String("exclude-tools", "", "comma-separated tool names to exclude")
	listGroups := flag.Bool("list-groups", false, "print available groups and exit")
	listTools := flag.Bool("list-tools", false, "print available tool names and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(serverVersion)
		return
	}
	if *listGroups {
		fmt.Println("Available tool groups:")
		for _, g := range bmcptools.AllGroups() {
			fmt.Printf("  %s\n", g)
		}
		return
	}
	if *listTools {
		fmt.Println("Available tools:")
		for _, g := range bmcptools.AllGroups() {
			fmt.Printf("  %s:\n", g)
			for _, name := range bmcptools.ToolsForGroup(g) {
				fmt.Printf("    %s\n", name)
			}
		}
		return
	}

	disableRaw := *disableFlag
	if disableRaw == "" {
		disableRaw = os.Getenv("BMCPTOOLS_DISABLE")
	}
	disabled := splitCSV(disableRaw)
	if err := bmcptools.ValidateGroups(disabled); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	excludeToolsRaw := *excludeToolsFlag
	if excludeToolsRaw == "" {
		excludeToolsRaw = os.Getenv("BMCPTOOLS_EXCLUDE_TOOLS")
	}
	excludedTools := splitCSV(excludeToolsRaw)
	if err := bmcptools.ValidateToolNames(excludedTools); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	instructions := bmcptools.ServerInstructionsWithExclusions(disabled, excludedTools)

	s := server.NewMCPServer(
		bmcptools.ServerName,
		bmcptools.Version,
		server.WithToolCapabilities(false),
		server.WithInstructions(instructions),
	)

	var opts []bmcptools.Option
	if len(disabled) > 0 {
		opts = append(opts, bmcptools.WithDisableGroups(disabled...))
	}
	if len(excludedTools) > 0 {
		opts = append(opts, bmcptools.WithExcludeTools(excludedTools...))
	}
	bmcptools.Register(s, opts...)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
