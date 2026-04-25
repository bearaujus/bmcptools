package confirm_test

import (
	"context"
	"fmt"
	"time"

	"github.com/bearaujus/bmcptools/pkg/confirm"
)

func Example_usage() {
	// Ask opens a browser dialog and blocks until the user responds.
	// On Linux, it returns an error (browser dialogs require macOS/Windows).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	confirmed, _, err := confirm.Ask(ctx, "Delete files?", "This will remove all .tmp files from the project.")
	if err != nil {
		fmt.Printf("dialog error: %v\n", err)
		return
	}
	if confirmed {
		fmt.Println("user confirmed")
	} else {
		fmt.Println("user cancelled")
	}
}
