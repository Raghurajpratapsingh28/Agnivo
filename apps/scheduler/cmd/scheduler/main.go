package main

import (
	"fmt"
	"os"

	"github.com/agnivo/agnivo/apps/scheduler/internal/app"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("scheduler", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "scheduler: fatal: %v\n", err)
		os.Exit(1)
	}
}
