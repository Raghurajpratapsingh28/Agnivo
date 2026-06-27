package main

import (
	"fmt"
	"os"

	"github.com/agnivo/agnivo/apps/runtime-agent/internal/app"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("runtime-agent", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "runtime-agent: fatal: %v\n", err)
		os.Exit(1)
	}
}
