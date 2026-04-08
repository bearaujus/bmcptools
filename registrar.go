package bmcptools

import "github.com/bearaujus/bmcptools/internal/toolreg"

// ToolRegistrar is the minimal interface needed to register MCP tools.
// It decouples Register functions from the concrete *server.MCPServer,
// making them easier to test and future-proof against SDK changes.
type ToolRegistrar = toolreg.ToolRegistrar
