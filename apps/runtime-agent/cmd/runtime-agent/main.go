package main

import (
	"fmt"
	"os"

	"github.com/Raghurajpratapsingh28/Agnivo/apps/runtime-agent/internal/app"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("runtime-agent", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "runtime-agent: fatal: %v\n", err)
		os.Exit(1)
	}
}
