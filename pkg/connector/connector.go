// Package connector defines the Connector interface for extending bmcptools
// with external tool groups (e.g. Lark, Slack, Notion integrations).
//
// # Usage
//
// Implement the Connector interface in your package:
//
//	type LarkConnector struct { client *lark.Client }
//
//	func (c *LarkConnector) Name() string { return "lark" }
//
//	func (c *LarkConnector) Register(s toolreg.ToolRegistrar) {
//	    s.AddTool(mcp.NewTool("lark_send_message", ...), c.sendMessageHandler)
//	}
//
// Then register with bmcptools:
//
//	bmcptools.RegisterConnectors(s, lark.New(cfg))
package connector

import "github.com/bearaujus/bmcptools/pkg/toolreg"

// Connector is implemented by any package that contributes additional MCP tools.
// Connectors are registered via bmcptools.RegisterConnectors at server startup.
type Connector interface {
	// Name returns a short, unique identifier for this connector (e.g. "lark").
	Name() string

	// Register adds the connector's tools to s.
	Register(s toolreg.ToolRegistrar)
}
