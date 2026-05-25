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
	if err := ValidateGroups([]string{GroupSystem, GroupFile}); err != nil {
		t.Fatalf("known groups should validate: %v", err)
	}
	err := ValidateGroups([]string{"nope", "alsobad"})
	if err == nil {
		t.Fatal("expected error for unknown groups")
	}
	if !strings.Contains(err.Error(), "alsobad") || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("error should list both bad names: %v", err)
	}
}

func TestRegisterWithDisableGroupsSystem(t *testing.T) {
	cap := &captureRegistrar{}
	Register(cap, WithDisableGroups(GroupSystem))

	for _, name := range cap.names {
		if name == "http_request" || name == "get_system_info" {
			t.Fatalf("disabled group leaked tool: %s", name)
		}
	}
	// Sanity: a non-system tool still registered.
	if !contains(cap.names, "read_file") {
		t.Fatal("read_file should still be registered when only system disabled")
	}
}

func TestRegisterWithDisableGroupsMultiple(t *testing.T) {
	cap := &captureRegistrar{}
	Register(cap, WithDisableGroups(GroupSystem, GroupUser))

	for _, banned := range []string{"http_request", "ask_user", "notify_user"} {
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

func TestRegisterDefaultRegistersCoreTools(t *testing.T) {
	cap := &captureRegistrar{}
	Register(cap)

	for _, want := range []string{"read_file", "grep_files", "run_command", "http_request"} {
		if !contains(cap.names, want) {
			t.Fatalf("expected %s to register by default", want)
		}
	}
}

func TestServerInstructionsExcludingGroups(t *testing.T) {
	full := ServerInstructions()
	if !strings.Contains(full, "System") {
		t.Fatal("full instructions should contain system section")
	}

	noSystem := ServerInstructionsExcludingGroups(GroupSystem)
	if strings.Contains(noSystem, "http_request") {
		t.Fatal("excluding system group should remove its tools")
	}
	// Intro must remain.
	if !strings.Contains(noSystem, "PERFORMANCE:") {
		t.Fatal("intro section should always be retained")
	}
	// Other groups remain.
	if !strings.Contains(noSystem, "File operations") {
		t.Fatal("file section should remain when only system excluded")
	}
}
