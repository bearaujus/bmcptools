package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"
)

const serverName = "bmcptools"

// serverVersion is overridden at build time via -ldflags "-X main.serverVersion=<tag>".
// Falls back to "dev" when building without ldflags (e.g. go run . locally).
var serverVersion = "dev"

//go:embed assets/descriptions/server_instructions.txt
var serverInstructions string

func main() {
	s := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithToolCapabilities(false),
		server.WithInstructions(serverInstructions),
	)

	registerUserTools(s)
	registerFileTools(s)
	registerDirTools(s)
	registerSearchTools(s)
	registerExecTools(s)
	registerMultiTools(s)
	registerSystemTools(s)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
