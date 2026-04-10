package user

import (
	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/dialog"
)

type userConfig struct {
	dialogHTML string
	chatHTML   string
	restHTML   string
}

func (c *userConfig) dialogHTMLSource() string {
	if c.dialogHTML != "" {
		return c.dialogHTML
	}
	return asset.HTML("dialog")
}

func (c *userConfig) chatHTMLSource() string {
	if c.chatHTML != "" {
		return c.chatHTML
	}
	return asset.HTML("chat")
}

func (c *userConfig) restHTMLSource() string {
	if c.restHTML != "" {
		return c.restHTML
	}
	return asset.HTML("rest")
}

// Option configures the user tool group at registration time.
type Option func(*userConfig)

// WithDialogTemplate overrides the default ask_user dialog HTML.
// The template must satisfy the dialog.DialogTemplate contract.
func WithDialogTemplate(t dialog.DialogTemplate) Option {
	return func(c *userConfig) { c.dialogHTML = t.HTML() }
}

// WithChatTemplate overrides the default open_chat HTML.
// The template must satisfy the dialog.ChatTemplate contract.
func WithChatTemplate(t dialog.ChatTemplate) Option {
	return func(c *userConfig) { c.chatHTML = t.HTML() }
}

// WithRestTemplate overrides the default rest HTML.
// The template must satisfy the dialog.RestTemplate contract.
func WithRestTemplate(t dialog.RestTemplate) Option {
	return func(c *userConfig) { c.restHTML = t.HTML() }
}
