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

func TestAllToolsHasNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range AllTools() {
		if seen[name] {
			t.Fatalf("duplicate tool: %s", name)
		}
		seen[name] = true
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

func TestValidateToolNames(t *testing.T) {
	if err := ValidateToolNames(nil); err != nil {
		t.Fatalf("nil should be valid: %v", err)
	}
	if err := ValidateToolNames([]string{ToolAskUser, ToolReadFile, ToolRunCommand}); err != nil {
		t.Fatalf("known tools should validate: %v", err)
	}
	err := ValidateToolNames([]string{"bad_tool", "also_bad"})
	if err == nil {
		t.Fatal("expected error for unknown tools")
	}
	if !strings.Contains(err.Error(), "also_bad") || !strings.Contains(err.Error(), "bad_tool") {
		t.Fatalf("error should list both bad names: %v", err)
	}
}

func TestToolsForGroup(t *testing.T) {
	userTools := ToolsForGroup(GroupUser)
	if !contains(userTools, ToolAskUser) || !contains(userTools, ToolNotifyUser) {
		t.Fatalf("expected user tools to include ask_user and notify_user, got %v", userTools)
	}
	if got := ToolsForGroup("missing"); got != nil {
		t.Fatalf("unknown group should return nil, got %v", got)
	}
	if group, ok := ToolGroup(ToolAskUser); !ok || group != GroupUser {
		t.Fatalf("expected %s to map to %s, got %q ok=%v", ToolAskUser, GroupUser, group, ok)
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

func TestRegisterWithDisableGroupsAndExcludeTools(t *testing.T) {
	cap := &captureRegistrar{}
	Register(cap, WithDisableGroups(GroupSystem), WithExcludeTools(ToolAskUser))

	if contains(cap.names, ToolAskUser) {
		t.Fatalf("excluded tool registered: %s", ToolAskUser)
	}
	if contains(cap.names, ToolHTTPRequest) {
		t.Fatalf("disabled group tool registered: %s", ToolHTTPRequest)
	}
	if !contains(cap.names, ToolNotifyUser) {
		t.Fatalf("user group should remain active for non-excluded tools: %s", ToolNotifyUser)
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

	noMulti := ServerInstructionsExcludingGroups(GroupMulti)
	if strings.Contains(noMulti, "read_multiple_files") || strings.Contains(noMulti, "Batch file operations") {
		t.Fatal("excluding multi group should remove batch-file instructions")
	}

	noExec := ServerInstructionsExcludingGroups(GroupExec)
	if strings.Contains(noExec, "get_env") {
		t.Fatal("excluding exec group should remove environment instructions")
	}
}
