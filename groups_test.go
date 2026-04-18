package bmcptools

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// captureRegistrar collects every registered tool name for assertions.
type captureRegistrar struct{ names []string }

func (c *captureRegistrar) AddTool(t mcp.Tool, _ server.ToolHandlerFunc) {
	c.names = append(c.names, t.Name)
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func TestAllGroupsHasNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, g := range AllGroups() {
		if seen[g] {
			t.Fatalf("duplicate group: %s", g)
		}
		seen[g] = true
	}
}

func TestValidateGroups(t *testing.T) {
	if err := ValidateGroups(nil); err != nil {
		t.Fatalf("nil should be valid: %v", err)
	}
	if err := ValidateGroups([]string{GroupBinance, GroupFile}); err != nil {
		t.Fatalf("known groups should validate: %v", err)
	}
	err := ValidateGroups([]string{"binance", "nope", "alsobad"})
	if err == nil {
		t.Fatal("expected error for unknown groups")
	}
	if !strings.Contains(err.Error(), "alsobad") || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("error should list both bad names: %v", err)
	}
}

func TestRegisterWithDisableGroupsBinance(t *testing.T) {
	cap := &captureRegistrar{}
	Register(cap, WithDisableGroups(GroupBinance))

	for _, name := range cap.names {
		if strings.HasPrefix(name, "binance_") {
			t.Fatalf("disabled group leaked tool: %s", name)
		}
	}
	// Sanity: a non-binance tool still registered.
	if !contains(cap.names, "read_file") {
		t.Fatal("read_file should still be registered when only binance disabled")
	}
}

func TestRegisterWithDisableGroupsMultiple(t *testing.T) {
	cap := &captureRegistrar{}
	Register(cap, WithDisableGroups(GroupBinance, GroupSystem, GroupUser))

	for _, banned := range []string{"binance_", "http_request", "ask_user", "notify_user"} {
		for _, name := range cap.names {
			if strings.HasPrefix(name, banned) || name == banned {
				t.Fatalf("disabled tool registered: %s", name)
			}
		}
	}
	if !contains(cap.names, "read_file") || !contains(cap.names, "list_directory") {
		t.Fatal("file/dir groups should remain registered")
	}
}

func TestRegisterDefaultRegistersBinance(t *testing.T) {
	cap := &captureRegistrar{}
	Register(cap)

	found := false
	for _, name := range cap.names {
		if strings.HasPrefix(name, "binance_") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected binance_* tools to register by default")
	}
}

func TestServerInstructionsExcludingGroups(t *testing.T) {
	full := ServerInstructions()
	if !strings.Contains(full, "Binance USDT-M Futures") {
		t.Fatal("full instructions should contain binance section")
	}

	noBinance := ServerInstructionsExcludingGroups(GroupBinance)
	if strings.Contains(noBinance, "Binance USDT-M Futures") {
		t.Fatal("excluding binance group should remove its section header")
	}
	// Intro must remain.
	if !strings.Contains(noBinance, "PERFORMANCE:") {
		t.Fatal("intro section should always be retained")
	}
	// Other groups remain.
	if !strings.Contains(noBinance, "File operations") {
		t.Fatal("file section should remain when only binance excluded")
	}
}
