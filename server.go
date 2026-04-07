package bmcptools

import (
	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/internal/tool/dir"
	"github.com/bearaujus/bmcptools/internal/tool/exec"
	"github.com/bearaujus/bmcptools/internal/tool/file"
	"github.com/bearaujus/bmcptools/internal/tool/multi"
	"github.com/bearaujus/bmcptools/internal/tool/search"
	"github.com/bearaujus/bmcptools/internal/tool/system"
	"github.com/bearaujus/bmcptools/internal/tool/user"
)

// ServerName is the MCP server identifier.
const ServerName = "bmcptools"

// ServerInstructions is the system-level prompt loaded from the embedded asset.
var ServerInstructions = asset.ServerInstructions()

// Register registers all bmcptools tool groups with s.
// This is the primary entry point for embedding bmcptools into a custom MCP server.
func Register(s ToolRegistrar) {
	user.Register(s)
	file.Register(s)
	dir.Register(s)
	search.Register(s)
	exec.Register(s)
	multi.Register(s)
	system.Register(s)
}
