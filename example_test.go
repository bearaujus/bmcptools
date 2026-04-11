package bmcptools_test

import (
	"fmt"

	bmcptools "github.com/bearaujus/bmcptools"
	"github.com/bearaujus/bmcptools/pkg/toolname"
	"github.com/mark3labs/mcp-go/server"
)

func Example_register() {
	s := server.NewMCPServer(
		bmcptools.ServerName,
		bmcptools.Version,
		server.WithToolCapabilities(false),
		server.WithInstructions(bmcptools.ServerInstructions()),
	)

	bmcptools.Register(s) // registers all 41 tools
	fmt.Println("registered all tools")
	// Output: registered all tools
}

func Example_registerSelectiveGroups() {
	s := server.NewMCPServer(bmcptools.ServerName, bmcptools.Version,
		server.WithInstructions(bmcptools.ServerInstructionsForGroups("file", "search")),
	)

	bmcptools.RegisterFile(s)
	bmcptools.RegisterSearch(s)
	fmt.Println("registered file + search groups")
	// Output: registered file + search groups
}

func Example_withExcludeTools() {
	s := server.NewMCPServer(bmcptools.ServerName, bmcptools.Version)

	bmcptools.Register(s, bmcptools.WithExcludeTools(
		toolname.CompressFiles,
		toolname.ExtractArchive,
		toolname.CreateSymlink,
	))
	fmt.Println("registered tools (excluding compress, extract, symlink)")
	// Output: registered tools (excluding compress, extract, symlink)
}
