package main

import (
	"fmt"
	"os"

	bmcptools "github.com/bearaujus/bmcptools"
	"github.com/mark3labs/mcp-go/server"
)

// serverVersion is overridden at build time via -ldflags "-X main.serverVersion=<tag>".
// Falls back to "dev" when building without ldflags (e.g. go run . locally).
var serverVersion = "dev"

func main() {
	s := server.NewMCPServer(
		bmcptools.ServerName,
		serverVersion,
		server.WithToolCapabilities(false),
		server.WithInstructions(bmcptools.ServerInstructions),
	)

	bmcptools.Register(s)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
