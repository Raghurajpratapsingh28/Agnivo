package main

import (
	"fmt"
	"os"

	"github.com/agnivo/agnivo/apps/builder/internal/app"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("builder", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "builder: fatal: %v\n", err)
		os.Exit(1)
	}
}
