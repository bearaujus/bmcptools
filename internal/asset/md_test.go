package asset

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMdJSRenderer(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found, skipping md.js tests")
	}

	_, thisFile, _, _ := runtime.Caller(0)
	testScript := filepath.Join(filepath.Dir(thisFile), "html", "md_test.js")

	cmd := exec.Command("node", testScript)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("md.js tests failed: %v", err)
	}
}
