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
)

// ServerName is the MCP server identifier.
const ServerName = "bmcptools"

// ServerInstructions is the system-level prompt loaded from the embedded asset.
var ServerInstructions = asset.ServerInstructions()

// UserOption configures the user interaction tool group (ask_user, open_chat, rest).
// It is a public alias for the internal option type so external consumers never need
// to import internal/tool/user directly.
type UserOption = user.Option

// UserWithDialogTemplate overrides the HTML used for ask_user dialogs.
// Use dialog.NewDialogTemplate to build and validate the template first.
func UserWithDialogTemplate(t dialog.DialogTemplate) UserOption { return user.WithDialogTemplate(t) }

// UserWithChatTemplate overrides the HTML used for open_chat windows.
func UserWithChatTemplate(t dialog.ChatTemplate) UserOption { return user.WithChatTemplate(t) }

// UserWithRestTemplate overrides the HTML used for rest pages.
func UserWithRestTemplate(t dialog.RestTemplate) UserOption { return user.WithRestTemplate(t) }

// Option configures the root Register call.
type Option func(*serverConfig)

type serverConfig struct {
	userOpts []UserOption
}

// WithUserOptions passes options to the user tool group (e.g., custom HTML templates).
func WithUserOptions(opts ...UserOption) Option {
	return func(c *serverConfig) { c.userOpts = append(c.userOpts, opts...) }
}

// Register registers all bmcptools tool groups with s.
// This is the primary entry point for embedding bmcptools into a custom MCP server.
func Register(s ToolRegistrar, opts ...Option) {
	cfg := &serverConfig{}
	for _, o := range opts {
		o(cfg)
	}
	user.Register(s, cfg.userOpts...)
	file.Register(s)
	dir.Register(s)
	search.Register(s)
	exec.Register(s)
	multi.Register(s)
	system.Register(s)
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
