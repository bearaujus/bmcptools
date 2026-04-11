package user

import (
	"github.com/bearaujus/bmcptools/internal/asset"
	"github.com/bearaujus/bmcptools/pkg/dialog"
)

type userConfig struct {
	dialogHTML string
	restHTML   string
}

func (c *userConfig) dialogHTMLSource() string {
	if c.dialogHTML != "" {
		return c.dialogHTML
	}
	return asset.HTML("dialog")
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

// WithRestTemplate overrides the default rest HTML.
// The template must satisfy the dialog.RestTemplate contract.
func WithRestTemplate(t dialog.RestTemplate) Option {
	return func(c *userConfig) { c.restHTML = t.HTML() }
}
