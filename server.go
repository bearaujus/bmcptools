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
	"github.com/bearaujus/bmcptools/pkg/connector"
	"github.com/bearaujus/bmcptools/pkg/dialog"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ServerName is the MCP server identifier.
const ServerName = "bmcptools"

// Version is the bmcptools module version.
// The standalone binary sets this via cmd/main.go ldflags.
// Library consumers can read it after import.
var Version = "dev"

// Group identifiers for selective registration. Each constant matches the
// directory name under internal/tool/ AND the marker used in
// internal/asset/descriptions/server_instructions.txt.
const (
	GroupUser   = "user"
	GroupFile   = "file"
	GroupMulti  = "multi"
	GroupDir    = "dir"
	GroupSearch = "search"
	GroupExec   = "exec"
	GroupSystem = "system"
)

// ServerInstructions returns the full server instructions covering all tool groups.
func ServerInstructions() string { return asset.ServerInstructions() }

// ServerInstructionsForGroups returns server instructions filtered to only
// the specified groups. Valid group names: "user", "file", "multi", "dir",
// "search", "exec", "system". The intro section is always included.
// Passing no groups returns the full instructions (same as ServerInstructions).
func ServerInstructionsForGroups(groups ...string) string {
	return asset.ServerInstructionsForGroups(groups...)
}

// ServerInstructionsExcludingGroups returns server instructions with the
// specified groups removed. The intro section is always kept.
// Passing no groups returns the full instructions.
func ServerInstructionsExcludingGroups(groups ...string) string {
	return asset.ServerInstructionsExcludingGroups(groups...)
}

// ServerInstructionsExcludingTools returns server instructions with the
// specified tool blocks removed. Passing no tools returns the full instructions.
func ServerInstructionsExcludingTools(names ...string) string {
	return asset.ServerInstructionsExcludingTools(names...)
}

// ServerInstructionsWithExclusions returns server instructions with the
// specified groups and tools removed. The intro section is always kept.
func ServerInstructionsWithExclusions(groups []string, tools []string) string {
	return asset.ServerInstructionsWithExclusions(groups, tools)
}

// UserOption configures the user interaction tool group (ask_user, open_chat, rest).
// It is a public alias for the internal option type so external consumers never need
// to import internal/tool/user directly.
type UserOption = user.Option

// UserWithDialogTemplate overrides the HTML used for ask_user dialogs.
// Use dialog.NewDialogTemplate to build and validate the template first.
func UserWithDialogTemplate(t dialog.DialogTemplate) UserOption { return user.WithDialogTemplate(t) }

// UserWithRestTemplate overrides the HTML used for rest pages.
func UserWithRestTemplate(t dialog.RestTemplate) UserOption { return user.WithRestTemplate(t) }

// Option configures the root Register call.
type Option func(*serverConfig)

type serverConfig struct {
	userOpts      []UserOption
	exclude       map[string]bool
	disableGroups map[string]bool
}

// WithUserOptions passes options to the user tool group (e.g., custom HTML templates).
func WithUserOptions(opts ...UserOption) Option {
	return func(c *serverConfig) { c.userOpts = append(c.userOpts, opts...) }
}

// WithExcludeTools prevents the listed tools from being registered.
// Use tool name constants from pkg/toolname (e.g. toolname.CompressFiles).
func WithExcludeTools(names ...string) Option {
	return func(c *serverConfig) {
		if c.exclude == nil {
			c.exclude = make(map[string]bool)
		}
		for _, n := range names {
			c.exclude[n] = true
		}
	}
}

// WithDisableGroups prevents whole tool groups from being registered.
// Use the Group* constants (GroupSystem, GroupUser, ...). Unknown group
// names are silently ignored at registration; use ValidateGroups beforehand
// for strict validation (e.g. when parsing CLI flags).
func WithDisableGroups(groups ...string) Option {
	return func(c *serverConfig) {
		if c.disableGroups == nil {
			c.disableGroups = make(map[string]bool)
		}
		for _, g := range groups {
			c.disableGroups[g] = true
		}
	}
}

// filteringRegistrar wraps a ToolRegistrar and silently drops excluded tools.
type filteringRegistrar struct {
	inner   ToolRegistrar
	exclude map[string]bool
}

func (f *filteringRegistrar) AddTool(tool mcp.Tool, handler server.ToolHandlerFunc) {
	if f.exclude[tool.Name] {
		return
	}
	f.inner.AddTool(tool, handler)
}

// Register registers all bmcptools tool groups with s.
// This is the primary entry point for embedding bmcptools into a custom MCP server.
func Register(s ToolRegistrar, opts ...Option) {
	cfg := &serverConfig{}
	for _, o := range opts {
		o(cfg)
	}

	var reg ToolRegistrar = s
	if len(cfg.exclude) > 0 {
		reg = &filteringRegistrar{inner: s, exclude: cfg.exclude}
	}

	if !cfg.disableGroups[GroupUser] {
		user.Register(reg, cfg.userOpts...)
	}
	if !cfg.disableGroups[GroupFile] {
		file.Register(reg)
	}
	if !cfg.disableGroups[GroupDir] {
		dir.Register(reg)
	}
	if !cfg.disableGroups[GroupSearch] {
		search.Register(reg)
	}
	if !cfg.disableGroups[GroupExec] {
		exec.Register(reg)
	}
	if !cfg.disableGroups[GroupMulti] {
		multi.Register(reg)
	}
	if !cfg.disableGroups[GroupSystem] {
		system.Register(reg)
	}
}

// RegisterFile registers only the file tool group.
func RegisterFile(s ToolRegistrar) { file.Register(s) }

// RegisterDir registers only the directory tool group.
func RegisterDir(s ToolRegistrar) { dir.Register(s) }

// RegisterSearch registers only the search tool group.
func RegisterSearch(s ToolRegistrar) { search.Register(s) }

// RegisterExec registers only the exec tool group.
func RegisterExec(s ToolRegistrar) { exec.Register(s) }

// RegisterSystem registers only the system tool group.
func RegisterSystem(s ToolRegistrar) { system.Register(s) }

// RegisterMulti registers only the multi-file tool group.
func RegisterMulti(s ToolRegistrar) { multi.Register(s) }

// RegisterUser registers only the user interaction tool group.
// opts allows overriding default HTML templates for dialogs.
func RegisterUser(s ToolRegistrar, opts ...UserOption) { user.Register(s, opts...) }

// RegisterConnectors registers one or more external connectors.
// Each connector provides its own tool group via the Connector interface.
// c.Name() returns a short identifier (e.g. "lark") useful for logging:
//
//	log.Printf("registering connector: %s", c.Name())
func RegisterConnectors(s ToolRegistrar, connectors ...connector.Connector) {
	for _, c := range connectors {
		c.Register(s)
	}
}
