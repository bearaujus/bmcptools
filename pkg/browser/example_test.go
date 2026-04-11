package browser_test

import (
	"fmt"
	"net/http"

	"github.com/bearaujus/bmcptools/pkg/browser"
)

func Example_serve() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<h1>Hello</h1>")
	})

	port, shutdown, err := browser.Serve(mux)
	if err != nil {
		panic(err)
	}
	defer shutdown()

	fmt.Printf("server listening on port %d\n", port)
	// (port is dynamic, so no Output assertion)
}
