package main

import (
	"fmt"
	"os"

	"github.com/agnivo/agnivo/apps/deployer/internal/app"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

func main() {
	if err := bootstrap.Run("deployer", app.Register); err != nil {
		fmt.Fprintf(os.Stderr, "deployer: fatal: %v\n", err)
		os.Exit(1)
	}
}
