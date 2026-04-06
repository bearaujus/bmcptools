package main

import (
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"
)

const (
	serverName    = "bmcptools"
	serverVersion = "2.2.0"
)

func main() {
	s := server.NewMCPServer(
		serverName,
		serverVersion,
		server.WithToolCapabilities(false),
	)

	registerUserTools(s)
	registerFileTools(s)
	registerDirTools(s)
	registerSearchTools(s)
	registerExecTools(s)
	registerMultiTools(s)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
