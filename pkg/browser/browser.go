// Package browser provides shared infrastructure for browser-based UI tools.
// It starts a local HTTP server on a random loopback port and opens the system
// browser — a pattern reused by user interaction tools and pkg/confirm.
package browser

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// OpenFn is called whenever a URL should be opened in the system browser.
// Override in tests to suppress real browser windows:
//
//	browser.OpenFn = func(string) {}
var OpenFn = func(url string) {
	_ = TryOpen(url)
}

// Serve starts a local HTTP server on a random loopback port.
// Returns the bound port, a shutdown function, and any error.
// The caller must eventually call shutdown to release the port.
func Serve(mux *http.ServeMux) (port int, shutdown func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, fmt.Errorf("browser.Serve: %w", err)
	}
	port = ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	shutdown = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	return port, shutdown, nil
}

// Open opens url in the system default browser.
// It is a best-effort call; errors are silently ignored.
func Open(url string) { OpenFn(url) }

// TryOpen opens url in the system default browser and reports whether the
// platform-specific launch command was started successfully.
func TryOpen(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "linux":
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("MIR_SOCKET") == "" {
			return fmt.Errorf("no graphical Linux session detected")
		}
		return exec.Command("xdg-open", url).Start()
	default:
		return fmt.Errorf("unsupported platform %q", runtime.GOOS)
	}
}

// ServeAndOpen starts a local HTTP server and immediately opens
// http://127.0.0.1:<port>/ in the browser. Returns the port and shutdown func.
func ServeAndOpen(mux *http.ServeMux) (port int, shutdown func(), err error) {
	port, shutdown, err = Serve(mux)
	if err != nil {
		return 0, nil, err
	}
	Open(fmt.Sprintf("http://127.0.0.1:%d/", port))
	return port, shutdown, nil
}
