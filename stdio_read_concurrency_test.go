package bmcptools

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/bearaujus/bmcptools/pkg/toolname"
)

func TestStdioConcurrentReadFileResponses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()
	defer serverIn.Close()
	defer clientOut.Close()
	defer clientIn.Close()
	defer serverOut.Close()

	s := mcpserver.NewMCPServer(
		ServerName,
		Version,
		mcpserver.WithToolCapabilities(false),
	)
	RegisterFile(s)

	stdio := mcpserver.NewStdioServer(s)
	errCh := make(chan error, 1)
	go func() {
		errCh <- stdio.Listen(ctx, serverIn, serverOut)
	}()

	c := mcpclient.NewClient(transport.NewIO(clientIn, clientOut, io.NopCloser(strings.NewReader(""))))
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "test-client",
				Version: "1.0.0",
			},
			Capabilities: mcp.ClientCapabilities{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	paths := makeReadConcurrencyFiles(t)
	var wg sync.WaitGroup
	errs := make([]error, len(paths))
	for i, path := range paths {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			req := mcp.CallToolRequest{}
			req.Params.Name = toolname.ReadFile
			req.Params.Arguments = map[string]any{
				"path":              path,
				"show_line_numbers": true,
				"max_bytes":         float64(60000),
			}
			result, err := c.CallTool(callCtx, req)
			if err != nil {
				errs[i] = err
				return
			}
			if result == nil || result.IsError {
				errs[i] = fmt.Errorf("unexpected result: %#v", result)
			}
		}(i, path)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stdio server did not stop")
	}
}

func TestSubprocessConcurrentReadFileResponses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	exe := buildTestServer(t, ctx)
	runSubprocessConcurrentReadFileResponses(t, ctx, exe, "--disable=user")
}

func TestSubprocessReadFileNotStarvedBySlowTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	exe := buildTestServer(t, ctx)
	c, err := mcpclient.NewStdioMCPClient(exe, nil, "--disable=user")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "test-client",
				Version: "1.0.0",
			},
			Capabilities: mcp.ClientCapabilities{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	command, shell := slowCommand()
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := mcp.CallToolRequest{}
			req.Params.Name = toolname.RunCommand
			req.Params.Arguments = map[string]any{
				"command":          command,
				"shell":            shell,
				"timeout_seconds":  float64(5),
				"max_output_bytes": float64(1),
			}
			result, err := c.CallTool(ctx, req)
			if err != nil {
				errs[i] = err
				return
			}
			if result == nil || result.IsError {
				errs[i] = fmt.Errorf("unexpected result: %#v", result)
			}
		}(i)
	}
	time.Sleep(200 * time.Millisecond)

	readCtx, readCancel := context.WithTimeout(ctx, time.Second)
	defer readCancel()
	req := mcp.CallToolRequest{}
	req.Params.Name = toolname.ReadFile
	req.Params.Arguments = map[string]any{
		"path":              makeReadConcurrencyFiles(t)[0],
		"show_line_numbers": true,
		"max_bytes":         float64(60000),
	}
	result, err := c.CallTool(readCtx, req)
	if err != nil {
		t.Fatalf("read_file was starved by slow tools: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("unexpected read_file result: %#v", result)
	}

	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("slow call %d failed: %v", i, err)
		}
	}
}

func buildTestServer(t *testing.T, ctx context.Context) string {
	t.Helper()

	exe := filepath.Join(t.TempDir(), "bmcptools-test")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	build := exec.CommandContext(ctx, "go", "build", "-o", exe, "./cmd/bmcptools")
	build.Env = os.Environ()
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build test server: %v\n%s", err, output)
	}
	return exe
}

func slowCommand() (command, shell string) {
	if runtime.GOOS == "windows" {
		return "Start-Sleep -Seconds 2; Write-Output done", "powershell"
	}
	return "sleep 2; echo done", "sh"
}

func runSubprocessConcurrentReadFileResponses(t *testing.T, ctx context.Context, exe string, args ...string) {
	t.Helper()

	c, err := mcpclient.NewStdioMCPClient(exe, nil, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "test-client",
				Version: "1.0.0",
			},
			Capabilities: mcp.ClientCapabilities{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	paths := makeReadConcurrencyFiles(t)
	var wg sync.WaitGroup
	errs := make([]error, len(paths))
	for i, path := range paths {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			req := mcp.CallToolRequest{}
			req.Params.Name = toolname.ReadFile
			req.Params.Arguments = map[string]any{
				"path":              path,
				"show_line_numbers": true,
				"max_bytes":         float64(60000),
			}
			result, err := c.CallTool(callCtx, req)
			if err != nil {
				errs[i] = err
				return
			}
			if result == nil || result.IsError {
				errs[i] = fmt.Errorf("unexpected result: %#v", result)
			}
		}(i, path)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
}

func makeReadConcurrencyFiles(t *testing.T) []string {
	t.Helper()

	dir := t.TempDir()
	paths := make([]string, 6)
	for i := range paths {
		var b strings.Builder
		for line := 1; line <= 700; line++ {
			fmt.Fprintf(&b, "file=%d line=%d abcdefghijklmnopqrstuvwxyz\n", i, line)
		}
		path := filepath.Join(dir, fmt.Sprintf("file-%d.txt", i))
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		paths[i] = path
	}
	return paths
}
