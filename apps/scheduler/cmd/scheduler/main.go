package main

import (
	"fmt"
	"os"

	"github.com/Raghurajpratapsingh28/Agnivo/apps/scheduler/internal/app"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("scheduler", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "scheduler: fatal: %v\n", err)
		os.Exit(1)
	}
}
