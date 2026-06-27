package main

import (
	"fmt"
	"os"

	"github.com/agnivo/agnivo/apps/worker/internal/app"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("worker", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "worker: fatal: %v\n", err)
		os.Exit(1)
	}
}
